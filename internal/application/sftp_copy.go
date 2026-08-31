package application

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// Cross- and same-side copy orchestration behind the SFTP panel's
// copy/paste: local→local (CopyLocal, see sftp_local.go), remote→remote
// (CopyWithinHost), local→remote upload and remote→local download
// (UploadPath/DownloadPath). The *Path variants accept a file OR a directory
// tree; single files behave exactly like the plain UploadFromFile /
// DownloadToFile they delegate to (per-file size limits included).

// treeProgress accumulates bytes across the sequential per-file transfers of
// one tree and reports them against the tree's total size. Per-file
// callbacks report absolute in-file positions; the delta against that file's
// previous position advances the aggregate counter.
type treeProgress struct {
	done, total, last int64
	report            SftpProgress
}

func newTreeProgress(total int64, report SftpProgress) *treeProgress {
	return &treeProgress{total: total, report: report}
}

// forFile returns a progress callback scoped to the next file. Files are
// transferred sequentially, so resetting `last` here is safe.
func (t *treeProgress) forFile() SftpProgress {
	if t == nil || t.report == nil {
		return nil
	}
	t.last = 0
	return func(n, _ int64) {
		t.done += n - t.last
		t.last = n
		t.report(t.done, t.total)
	}
}

// ── remote → remote ─────────────────────────────────────────────────────

// remoteFileOp is one file to copy within a host.
type remoteFileOp struct {
	src, dst string
	size     int64
}

// remoteTreePlan lists the mkdirs (parents first) and file copies of a
// remote→remote tree copy, plus the aggregate byte total.
type remoteTreePlan struct {
	dirs  []string
	files []remoteFileOp
	total int64
}

// CopyWithinHost copies a remote file or directory tree to another path on
// the same host, streaming every file through the SFTP client. Copying a
// directory into its own subtree is refused.
func (s *SftpService) CopyWithinHost(ctx context.Context, hostID, srcPath, dstPath string, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return err
	}
	size, isDir, err := s.client.StatPath(host, c, srcPath)
	if err != nil {
		return err
	}
	chunk := int64(s.transferConfig().ChunkKB) * 1024

	if !isDir {
		tp := newTreeProgress(size, progress)
		return s.client.CopyRemote(ctx, host, c, srcPath, dstPath, chunk, tp.forFile())
	}
	if remoteContains(srcPath, dstPath) || remoteContains(dstPath, srcPath) {
		return fmt.Errorf("cannot copy %q into itself", srcPath)
	}
	plan, err := s.planRemoteTree(ctx, host, c, srcPath, dstPath)
	if err != nil {
		return err
	}
	tp := newTreeProgress(plan.total, progress)
	for _, dir := range plan.dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.client.Mkdir(host, c, dir); err != nil {
			return err
		}
	}
	for _, op := range plan.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.client.CopyRemote(ctx, host, c, op.src, op.dst, chunk, tp.forFile()); err != nil {
			return err
		}
	}
	return nil
}

// planRemoteTree walks srcDir via ListDir, mapping every entry to its
// destination under dstDir.
func (s *SftpService) planRemoteTree(ctx context.Context, host domain.Host, c domain.Credentials, srcDir, dstDir string) (*remoteTreePlan, error) {
	plan := &remoteTreePlan{dirs: []string{dstDir}}
	if err := s.planRemoteDir(ctx, host, c, srcDir, dstDir, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *SftpService) planRemoteDir(ctx context.Context, host domain.Host, c domain.Credentials, srcDir, dstDir string, plan *remoteTreePlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := s.client.ListDir(host, c, srcDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		sp := joinRemote(srcDir, e.Name)
		dp := joinRemote(dstDir, e.Name)
		if e.IsDir {
			plan.dirs = append(plan.dirs, dp)
			if err := s.planRemoteDir(ctx, host, c, sp, dp, plan); err != nil {
				return err
			}
		} else {
			plan.files = append(plan.files, remoteFileOp{src: sp, dst: dp, size: e.Size})
			plan.total += e.Size
		}
	}
	return nil
}

// ── local → remote (upload) / remote → local (download) ─────────────────

// UploadPath uploads a local file or directory tree to remotePath. Directory
// uploads mirror the source layout on the remote side; every file goes
// through the same staged, size-limited upload as a plain upload.
func (s *SftpService) UploadPath(ctx context.Context, hostID, localPath, remotePath string, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local %q: %w", localPath, err)
	}
	cfg := s.transferConfig()
	chunk := int64(cfg.ChunkKB) * 1024
	limit := int64(cfg.MaxUploadMB) * 1024 * 1024

	if !info.IsDir() {
		if info.Size() > limit {
			return fmt.Errorf("file is %d MB, exceeds the %d MB upload limit (adjust in Settings → Advanced)", info.Size()/1024/1024, cfg.MaxUploadMB)
		}
		tp := newTreeProgress(info.Size(), progress)
		return s.client.UploadFromFile(ctx, host, c, localPath, remotePath, chunk, tp.forFile())
	}

	// Plan the local tree: remote mkdirs (WalkDir yields parents first) and
	// per-file uploads with the aggregate byte total.
	type upOp struct {
		local, remote string
		size          int64
	}
	var (
		dirs  []string
		files []upOp
		total int64
	)
	err = filepath.WalkDir(localPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(localPath, p)
		if err != nil {
			return err
		}
		rp := remotePath
		if rel != "." {
			rp = joinRemote(remotePath, filepath.ToSlash(rel))
		}
		if d.IsDir() {
			dirs = append(dirs, rp)
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, upOp{local: p, remote: rp, size: fi.Size()})
		total += fi.Size()
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk local %q: %w", localPath, err)
	}

	tp := newTreeProgress(total, progress)
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.client.Mkdir(host, c, dir); err != nil {
			return err
		}
	}
	for _, op := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if op.size > limit {
			return fmt.Errorf("file %q is %d MB, exceeds the %d MB upload limit (adjust in Settings → Advanced)", filepath.Base(op.local), op.size/1024/1024, cfg.MaxUploadMB)
		}
		if err := s.client.UploadFromFile(ctx, host, c, op.local, op.remote, chunk, tp.forFile()); err != nil {
			return err
		}
	}
	return nil
}

// DownloadPath downloads a remote file or directory tree to localPath,
// mirroring the remote layout locally; every file goes through the same
// staged, size-limited download as a plain download.
func (s *SftpService) DownloadPath(ctx context.Context, hostID, remotePath, localPath string, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return err
	}
	size, isDir, err := s.client.StatPath(host, c, remotePath)
	if err != nil {
		return err
	}
	cfg := s.transferConfig()
	chunk := int64(cfg.ChunkKB) * 1024
	limit := int64(cfg.MaxDownloadMB) * 1024 * 1024

	if !isDir {
		if size > limit {
			return fmt.Errorf("file is %d MB, exceeds the %d MB download limit (adjust in Settings → Advanced)", size/1024/1024, cfg.MaxDownloadMB)
		}
		tp := newTreeProgress(size, progress)
		return s.client.DownloadToFile(ctx, host, c, remotePath, localPath, chunk, tp.forFile())
	}

	plan, err := s.planRemoteTree(ctx, host, c, remotePath, localPath)
	if err != nil {
		return err
	}
	// The plan builds POSIX remote-style destination paths rooted at dstDir
	// (the localPath string); rebase them onto the local filesystem before
	// touching disk. dstRoot is captured before the slice is mutated.
	dstRoot := plan.dirs[0]
	for i := range plan.dirs {
		plan.dirs[i] = rebaseDst(dstRoot, plan.dirs[i], localPath)
	}
	for i := range plan.files {
		plan.files[i].dst = rebaseDst(dstRoot, plan.files[i].dst, localPath)
	}

	tp := newTreeProgress(plan.total, progress)
	for _, dir := range plan.dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir local %q: %w", dir, err)
		}
	}
	for _, op := range plan.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if op.size > limit {
			return fmt.Errorf("file %q is %d MB, exceeds the %d MB download limit (adjust in Settings → Advanced)", path.Base(op.src), op.size/1024/1024, cfg.MaxDownloadMB)
		}
		if err := s.client.DownloadToFile(ctx, host, c, op.src, op.dst, chunk, tp.forFile()); err != nil {
			return err
		}
	}
	return nil
}

// ── small path helpers ───────────────────────────────────────────────────

// joinRemote joins a POSIX remote directory and entry name.
func joinRemote(dir, name string) string {
	return path.Join(dir, name)
}

// remoteContains reports whether child equals parent or lies underneath it
// (POSIX paths, case-sensitive like remote filesystems).
func remoteContains(parent, child string) bool {
	p := path.Clean(parent)
	ch := path.Clean(child)
	return ch == p || strings.HasPrefix(ch, p+"/")
}

// rebaseDst converts a planned remote-style destination path (dstRoot-prefixed,
// slash-separated) to an actual path rooted at actualRoot on the local
// filesystem.
func rebaseDst(dstRoot, planned, actualRoot string) string {
	rel := strings.TrimPrefix(path.Clean(planned), path.Clean(dstRoot))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return actualRoot
	}
	return filepath.Join(actualRoot, filepath.FromSlash(rel))
}

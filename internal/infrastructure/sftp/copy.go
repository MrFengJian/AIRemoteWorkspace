package sftp

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// Remote→remote copy support: the SFTP protocol has no server-side copy, so
// bytes flow network→network inside this process with the same fixed-buffer,
// .part-staged discipline as the other streaming transfers.

// StatPath reports a remote path's size and directory flag (tree planning and
// paste-side conflict checks).
func (m *Manager) StatPath(host domain.Host, creds domain.Credentials, remotePath string) (int64, bool, error) {
	sc, err := m.client(host, creds)
	if err != nil {
		return 0, false, err
	}
	info, err := sc.Stat(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return 0, false, fmt.Errorf("stat %q: %w", remotePath, err)
	}
	return info.Size(), info.IsDir(), nil
}

// CopyRemote streams one remote file to another path on the same host. chunk
// bytes is the buffer size (and progress granularity); ctx cancellation
// aborts cleanly, leaving dstPath untouched (bytes land in the .part file
// first, published by rename on completion).
func (m *Manager) CopyRemote(ctx context.Context, host domain.Host, creds domain.Credentials, srcPath, dstPath string, chunk int64, progress application.SftpProgress) error {
	if chunk <= 0 {
		chunk = 256 * 1024
	}
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	info, err := sc.Stat(srcPath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("stat %q: %w", srcPath, err)
	}
	r, err := sc.Open(srcPath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("open %q: %w", srcPath, err)
	}
	defer r.Close()

	partPath := dstPath + partSuffix
	w, err := sc.Create(partPath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("create %q: %w", partPath, err)
	}

	total := info.Size()
	if progress != nil {
		progress(0, total)
	}
	cr := &countingReader{r: r, ctx: ctx, total: total, chunk: chunk, progress: progress}
	buf := make([]byte, chunk)
	if _, err := io.CopyBuffer(w, cr, buf); err != nil {
		_ = w.Close()
		_ = sc.Remove(partPath)
		return fmt.Errorf("copy %q: %w", path.Base(srcPath), err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close remote %q: %w", partPath, err)
	}
	// Atomic publish, POSIX rename first (overwrites), basic rename as the
	// fallback for servers without the extension.
	if err := sc.PosixRename(partPath, dstPath); err != nil {
		if rmErr := sc.Rename(partPath, dstPath); rmErr != nil {
			return fmt.Errorf("publish %q: %w", dstPath, err)
		}
	}
	if progress != nil {
		progress(total, total)
	}
	return nil
}

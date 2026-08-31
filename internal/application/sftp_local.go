package application

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Local filesystem operations backing the SFTP panel's local pane. They
// mirror the remote operation set (list/mkdir/rename/delete) and reuse the
// same SftpEntry shape, so the UI can treat both panes alike. All paths are
// on the machine the app runs on; no SSH involvement.

// LocalDefaultDir returns the initial directory for the local pane (the
// user's home directory).
func (s *SftpService) LocalDefaultDir() (string, error) {
	return os.UserHomeDir()
}

// LocalListDir lists a local directory. An empty dir means the home
// directory. Entries are sorted dirs-first then by name, like remote lists.
func (s *SftpService) LocalListDir(dir string) ([]SftpEntry, error) {
	if strings.TrimSpace(dir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home: %w", err)
		}
		dir = home
	}
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}
	entries := make([]SftpEntry, 0, len(dirents))
	for _, de := range dirents {
		info, err := de.Info()
		if err != nil {
			continue // entry vanished between ReadDir and Info — skip it
		}
		entries = append(entries, SftpEntry{
			Name:    de.Name(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir:   info.IsDir(),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // directories first
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// LocalMkdir creates a single local directory (parents are NOT created,
// matching the remote Mkdir).
func (s *SftpService) LocalMkdir(path string) error {
	if err := os.Mkdir(path, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", path, err)
	}
	return nil
}

// LocalRename renames/moves a local path.
func (s *SftpService) LocalRename(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %q → %q: %w", oldPath, newPath, err)
	}
	return nil
}

// LocalDelete removes a local file or empty directory (same semantics as the
// remote DeleteFile — non-empty directories are refused).
func (s *SftpService) LocalDelete(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}

// CopyLocal copies a local file or directory tree (files are overwritten at
// the destination — the UI resolves same-name conflicts beforehand).
// Cancellation leaves a partial destination behind, like a failed Explorer
// copy. Copying a directory into its own subtree is refused.
func (s *SftpService) CopyLocal(ctx context.Context, srcPath, dstPath string, progress SftpProgress) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %q: %w", srcPath, err)
	}
	if info.IsDir() && (localContains(srcPath, dstPath) || localContains(dstPath, srcPath)) {
		return fmt.Errorf("cannot copy %q into itself", srcPath)
	}
	total := info.Size()
	if info.IsDir() {
		total = localTreeSize(srcPath)
	}
	tp := newTreeProgress(total, progress)
	return copyLocalEntry(ctx, srcPath, dstPath, tp)
}

// copyLocalEntry copies one file or recurses a directory tree, reporting
// aggregate progress through tp.
func copyLocalEntry(ctx context.Context, srcPath, dstPath string, tp *treeProgress) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %q: %w", srcPath, err)
	}
	if !info.IsDir() {
		return copyLocalFile(ctx, srcPath, dstPath, info.Size(), tp.forFile())
	}
	if err := os.MkdirAll(dstPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", dstPath, err)
	}
	dirents, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", srcPath, err)
	}
	for _, de := range dirents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyLocalEntry(ctx,
			filepath.Join(srcPath, de.Name()),
			filepath.Join(dstPath, de.Name()),
			tp,
		); err != nil {
			return err
		}
	}
	return nil
}

// copyLocalFile streams one local file to another, staged via
// "<dst>.airw-part" + rename so a failed copy never masquerades as complete.
func copyLocalFile(ctx context.Context, srcPath, dstPath string, size int64, progress SftpProgress) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %q: %w", srcPath, err)
	}
	defer f.Close()

	partPath := dstPath + ".airw-part"
	w, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", partPath, err)
	}

	if progress != nil {
		progress(0, size)
	}
	buf := make([]byte, 256*1024)
	var copied int64
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				_ = w.Close()
				_ = os.Remove(partPath)
				return fmt.Errorf("write %q: %w", dstPath, werr)
			}
			copied += int64(n)
			if progress != nil {
				progress(copied, size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = w.Close()
			_ = os.Remove(partPath)
			return fmt.Errorf("read %q: %w", srcPath, rerr)
		}
		if err := ctx.Err(); err != nil {
			_ = w.Close()
			_ = os.Remove(partPath)
			return err
		}
	}
	if err := w.Close(); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("close %q: %w", partPath, err)
	}
	if err := os.Rename(partPath, dstPath); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("publish %q: %w", dstPath, err)
	}
	if progress != nil {
		progress(size, size)
	}
	return nil
}

// localTreeSize sums file sizes under dir (the aggregate progress total).
func localTreeSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries contribute nothing, keep walking
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// localContains reports whether child equals parent or lies underneath it
// (case-insensitive on Windows, where paths are case-insensitive).
func localContains(parent, child string) bool {
	p, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	c, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	p = filepath.Clean(p)
	c = filepath.Clean(c)
	eq := p == c
	has := strings.HasPrefix(c, p+string(filepath.Separator))
	if runtime.GOOS == "windows" {
		eq = strings.EqualFold(p, c)
		has = strings.HasPrefix(strings.ToLower(c), strings.ToLower(p+string(filepath.Separator)))
	}
	return eq || has
}

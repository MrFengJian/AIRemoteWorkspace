package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// Streaming transfers (FileZilla-style): data moves disk↔network inside the
// Go process with a fixed-size buffer — memory usage is independent of file
// size, and only progress/cancel events cross the UI bridge. Atomicity comes
// from writing "<name>.airw-part" and renaming on completion, so a failed
// transfer never masquerades as a complete file (and the leftover .part is
// the natural resume point should resume be added later).

// partSuffix marks an in-progress transfer file.
const partSuffix = ".airw-part"

// countingReader wraps the local source of an upload: progress per chunk and
// cancellation between chunks.
type countingReader struct {
	r        io.Reader
	ctx      context.Context
	n, total int64
	chunk    int64
	last     int64
	progress application.SftpProgress
}

func (c *countingReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, fmt.Errorf("transfer cancelled: %w", err)
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.progress != nil && (c.n-c.last >= c.chunk || err == io.EOF) {
		c.progress(c.n, c.total)
		c.last = c.n
	}
	return n, err
}

// countingWriter wraps the local destination of a download.
type countingWriter struct {
	w        io.Writer
	ctx      context.Context
	n, total int64
	chunk    int64
	last     int64
	progress application.SftpProgress
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, fmt.Errorf("transfer cancelled: %w", err)
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	if c.progress != nil && c.n-c.last >= c.chunk {
		c.progress(c.n, c.total)
		c.last = c.n
	}
	return n, err
}

// UploadFromFile streams a local file to remotePath. chunk bytes is the
// buffer size (and progress granularity); ctx cancellation aborts cleanly,
// leaving remotePath untouched (data lands in the .airw-part file first).
func (m *Manager) UploadFromFile(ctx context.Context, host domain.Host, creds domain.Credentials, localPath, remotePath string, chunk int64, progress application.SftpProgress) error {
	if chunk <= 0 {
		chunk = 256 * 1024
	}
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local %q: %w", localPath, err)
	}
	defer f.Close()

	partPath := remotePath + partSuffix
	w, err := sc.Create(partPath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("create %q: %w", partPath, err)
	}

	total, _ := f.Seek(0, io.SeekEnd)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek %q: %w", localPath, err)
	}
	if progress != nil {
		progress(0, total)
	}
	cr := &countingReader{r: f, ctx: ctx, total: total, chunk: chunk, progress: progress}
	// io.CopyBuffer delegates to the sftp file's ReaderFrom, which pipelines
	// concurrent WRITE requests when the client has them enabled.
	buf := make([]byte, chunk)
	if _, err := io.CopyBuffer(w, cr, buf); err != nil {
		_ = w.Close()
		_ = sc.Remove(partPath)
		return fmt.Errorf("upload %q: %w", filepath.Base(localPath), err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close remote %q: %w", partPath, err)
	}
	// Atomic publish: prefer POSIX rename (overwrites atomically), fall back
	// to the basic rename on servers without the extension.
	if err := sc.PosixRename(partPath, remotePath); err != nil {
		if rmErr := sc.Rename(partPath, remotePath); rmErr != nil {
			return fmt.Errorf("publish %q: %w", remotePath, err)
		}
	}
	if progress != nil {
		progress(total, total)
	}
	return nil
}

// DownloadToFile streams remotePath to a local file. Same shape as
// UploadFromFile: .part staging, chunked progress, cancellation.
func (m *Manager) DownloadToFile(ctx context.Context, host domain.Host, creds domain.Credentials, remotePath, localPath string, chunk int64, progress application.SftpProgress) error {
	if chunk <= 0 {
		chunk = 256 * 1024
	}
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	info, err := sc.Stat(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("stat %q: %w", remotePath, err)
	}
	r, err := sc.Open(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("open %q: %w", remotePath, err)
	}
	defer r.Close()

	partPath := localPath + partSuffix
	f, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("create local %q: %w", partPath, err)
	}

	total := info.Size()
	if progress != nil {
		progress(0, total)
	}
	cw := &countingWriter{w: f, ctx: ctx, total: total, chunk: chunk, progress: progress}
	// io.CopyBuffer delegates to the sftp file's WriteTo (pipelined
	// concurrent READs on capable servers).
	buf := make([]byte, chunk)
	if _, err := io.CopyBuffer(cw, r, buf); err != nil {
		_ = f.Close()
		_ = os.Remove(partPath)
		return fmt.Errorf("download %q: %w", filepath.Base(remotePath), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close local %q: %w", partPath, err)
	}
	if err := os.Rename(partPath, localPath); err != nil {
		return fmt.Errorf("publish %q: %w", localPath, err)
	}
	if progress != nil {
		progress(total, total)
	}
	return nil
}

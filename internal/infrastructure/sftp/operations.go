package sftp

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// Entry is a remote filesystem entry (file or directory).
type Entry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`    // e.g. "-rw-r--r--"
	ModTime time.Time `json:"modTime"`
	IsDir   bool      `json:"isDir"`
}

// maxDownloadBytes caps whole-file downloads in the basic version. Larger
// transfers should use a streaming path (future work).
const maxDownloadBytes = 50 * 1024 * 1024 // 50 MB

// ListDir returns the entries of a remote directory, sorted (dirs first, then
// name). The path is resolved relative to the SFTP server's root.
func (m *Manager) ListDir(host domain.Host, creds domain.Credentials, dir string) ([]Entry, error) {
	sc, err := m.client(host, creds)
	if err != nil {
		return nil, err
	}
	infos, err := sc.ReadDir(dir)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, Entry{
			Name:    info.Name(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime(),
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

// DownloadFile reads a remote file into memory. Returns an error if the file
// exceeds maxDownloadBytes.
func (m *Manager) DownloadFile(host domain.Host, creds domain.Credentials, remotePath string) ([]byte, error) {
	sc, err := m.client(host, creds)
	if err != nil {
		return nil, err
	}
	info, err := sc.Stat(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return nil, fmt.Errorf("stat %q: %w", remotePath, err)
	}
	if info.Size() > maxDownloadBytes {
		return nil, fmt.Errorf("file is %d bytes, exceeds the %d-byte basic-download limit; use a dedicated SFTP client", info.Size(), maxDownloadBytes)
	}
	r, err := sc.Open(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return nil, fmt.Errorf("open %q: %w", remotePath, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", remotePath, err)
	}
	return data, nil
}

// UploadFile writes data to remotePath, overwriting if it exists.
func (m *Manager) UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte) error {
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	w, err := sc.Create(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("create %q: %w", remotePath, err)
	}
	defer w.Close()
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write %q: %w", remotePath, err)
	}
	return nil
}

// DeleteFile removes a remote file or empty directory.
func (m *Manager) DeleteFile(host domain.Host, creds domain.Credentials, remotePath string) error {
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	info, err := sc.Stat(remotePath)
	if err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("stat %q: %w", remotePath, err)
	}
	if info.IsDir() {
		if err := sc.RemoveDirectory(remotePath); err != nil {
			m.maybeDropOnErr(host.ID, err)
			return fmt.Errorf("remove dir %q: %w", remotePath, err)
		}
		return nil
	}
	if err := sc.Remove(remotePath); err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("remove %q: %w", remotePath, err)
	}
	return nil
}

// RenameFile renames/moves a remote path.
func (m *Manager) RenameFile(host domain.Host, creds domain.Credentials, oldPath, newPath string) error {
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	// posix rename works across dirs on the same filesystem; SFTP's Rename
	// maps to it. If the target exists some servers reject — surface the error.
	if err := sc.Rename(oldPath, newPath); err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("rename %q → %q: %w", oldPath, newPath, err)
	}
	return nil
}

// Mkdir creates a remote directory.
func (m *Manager) Mkdir(host domain.Host, creds domain.Credentials, remotePath string) error {
	sc, err := m.client(host, creds)
	if err != nil {
		return err
	}
	if err := sc.Mkdir(remotePath); err != nil {
		m.maybeDropOnErr(host.ID, err)
		return fmt.Errorf("mkdir %q: %w", remotePath, err)
	}
	return nil
}

// JoinPath is a convenience for callers building paths; kept here so the
// path semantics (POSIX) live next to the operations that use them.
func JoinPath(elems ...string) string {
	return path.Join(elems...)
}

// maybeDropOnErr drops the cached connection when an error suggests it is dead,
// so the next operation redials instead of reusing a broken channel.
func (m *Manager) maybeDropOnErr(hostID string, err error) {
	if err == nil {
		return
	}
	// EOF / connection reset / channel closed indicate a dead connection.
	msg := err.Error()
	if strings.Contains(msg, "EOF") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "channel closed") || strings.Contains(msg, "use of closed") {
		m.dropOnError(hostID)
	}
}

package application

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// RemoteEditService powers the SFTP "edit in local editor" flow: a remote
// file is downloaded to a per-host temp folder and opened with the OS
// default application; a watcher polls the local file and uploads it back
// automatically when the user saves (mtime change). Last write wins —
// concurrent remote edits are not detected. Credentials resolve inside the
// SftpService on every transfer (OS vault included).
type RemoteEditService struct {
	sftp *SftpService
	// onChanged fires after a successful auto-upload (editID, remotePath).
	onChanged func(editID, remotePath string)

	mu     sync.Mutex
	edits  map[string]*remoteEdit
	stopCh chan struct{}
}

type remoteEdit struct {
	id         string
	hostID     string
	remotePath string
	localPath  string
	lastMod    time.Time
}

// NewRemoteEditService wires the service to the SFTP client.
func NewRemoteEditService(sftp *SftpService) *RemoteEditService {
	return &RemoteEditService{
		sftp:   sftp,
		edits:  make(map[string]*remoteEdit),
		stopCh: make(chan struct{}),
	}
}

// SetOnChanged registers the post-auto-upload callback (wired to the Wails
// event bus by the interfaces layer after the app handle exists).
func (s *RemoteEditService) SetOnChanged(fn func(editID, remotePath string)) {
	s.onChanged = fn
}

// StopAll terminates every watcher (app shutdown).
func (s *RemoteEditService) StopAll() {
	close(s.stopCh)
}

// editIDOf is stable per host+path: editing the same file again reuses the
// same local file and watcher instead of stacking duplicates.
func editIDOf(hostID, remotePath string) string {
	sum := sha1.Sum([]byte(hostID + "\x00" + remotePath))
	return fmt.Sprintf("%x", sum[:])
}

// Begin downloads remotePath to the local temp folder and starts the
// auto-upload watcher. Returns the local path so the caller can open it
// with the OS default application.
func (s *RemoteEditService) Begin(hostID, remotePath string) (string, error) {
	id := editIDOf(hostID, remotePath)

	s.mu.Lock()
	if e, ok := s.edits[id]; ok {
		local := e.localPath
		s.mu.Unlock()
		return local, nil // already tracked — caller re-opens the editor
	}
	s.mu.Unlock()

	data, err := s.sftp.DownloadFile(hostID, domain.Credentials{}, remotePath, nil)
	if err != nil {
		return "", fmt.Errorf("download for edit: %w", err)
	}

	dir := filepath.Join(os.TempDir(), "ai-remote-edit", hostID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create edit dir: %w", err)
	}
	local := filepath.Join(dir, fmt.Sprintf("%s_%s", id[:10], filepath.Base(remotePath)))
	if err := os.WriteFile(local, data, 0o644); err != nil {
		return "", fmt.Errorf("write local copy: %w", err)
	}
	st, _ := os.Stat(local)
	edit := &remoteEdit{
		id:         id,
		hostID:     hostID,
		remotePath: remotePath,
		localPath:  local,
		lastMod:    st.ModTime(),
	}

	s.mu.Lock()
	s.edits[id] = edit
	s.mu.Unlock()

	go s.watch(edit)
	return edit.localPath, nil
}

// watch polls the local file and uploads it back on change, until the edit
// is stopped (app shutdown) or the local file disappears (user cleaned it).
// The poll cadence is deliberately coarse: mtime granularity on Windows can
// be ~1s, and saving in a big IDE takes longer than the upload anyway.
func (s *RemoteEditService) watch(edit *remoteEdit) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
		st, err := os.Stat(edit.localPath)
		if err != nil {
			return // local copy removed — stop watching
		}
		if !st.ModTime().After(edit.lastMod) {
			continue
		}
		data, err := os.ReadFile(edit.localPath)
		if err != nil {
			continue // mid-save read (e.g. editor swap) — retry next tick
		}
		if err := s.sftp.UploadFile(edit.hostID, domain.Credentials{}, edit.remotePath, data, nil); err != nil {
			continue // keep watching; the next save will retry the upload
		}
		edit.lastMod = st.ModTime()
		if s.onChanged != nil {
			s.onChanged(edit.id, edit.remotePath)
		}
	}
}

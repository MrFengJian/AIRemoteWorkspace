package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// FileEntryDTO is the frontend-facing remote filesystem entry.
type FileEntryDTO struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

// TransferProgressDTO carries upload/download progress to the frontend.
type TransferProgressDTO struct {
	Transferred int64 `json:"transferred"`
	Total       int64 `json:"total"`
}

// progressEmitInterval throttles progress events so a fast transfer does not
// flood the Wails event bus. The final 100% event is always sent.
const progressEmitInterval = 100 * time.Millisecond

// SftpService exposes remote file operations to the frontend.
//
// The frontend passes empty credentials: the backend resolves remembered
// secrets from the OS vault (Phase 5 SecretStore). If no remembered secret
// exists, the operation fails with an auth error the UI surfaces.
//
// Streaming transfers (StartDownload/StartUpload) run asynchronously in a
// backend goroutine: progress events arrive on "sftp:transfer:<id>" and a
// terminal event on "sftp:transfer:<id>:end" (data = "" on success,
// "cancelled", or the error text). CancelTransfer aborts by id.
type SftpService struct {
	svc *appsvc.SftpService
	app *wailsapp.App

	mu        sync.Mutex
	transfers map[string]context.CancelFunc
}

// NewSftpService wires the Wails SftpService to its application port.
func NewSftpService(svc *appsvc.SftpService) *SftpService {
	return &SftpService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (s *SftpService) ServiceName() string { return "SftpService" }

// ServiceStartup captures the Application handle so progress events can be
// emitted (same pattern as TerminalService).
func (s *SftpService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	s.app = wailsapp.Get()
	return nil
}

// ListDir returns the entries of a remote directory.
func (s *SftpService) ListDir(hostID, dir string) ([]FileEntryDTO, error) {
	entries, err := s.svc.ListDir(hostID, domain.Credentials{}, dir)
	if err != nil {
		return nil, fmt.Errorf("list dir: %w", err)
	}
	out := make([]FileEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, FileEntryDTO{
			Name:    e.Name,
			Size:    e.Size,
			Mode:    e.Mode,
			ModTime: e.ModTime,
			IsDir:   e.IsDir,
		})
	}
	return out, nil
}

// progressEmitter returns a throttled callback that emits progress events on
// "sftp:transfer:<id>". Safe for concurrent use within one transfer.
func (s *SftpService) progressEmitter(transferID string) appsvc.SftpProgress {
	if s.app == nil || transferID == "" {
		return nil
	}
	var mu sync.Mutex
	var last time.Time
	return func(transferred, total int64) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		done := total > 0 && transferred >= total
		if !done && now.Sub(last) < progressEmitInterval {
			return
		}
		last = now
		s.app.Event.Emit(
			fmt.Sprintf("sftp:transfer:%s", transferID),
			TransferProgressDTO{Transferred: transferred, Total: total},
		)
	}
}

// DownloadFile reads a remote file and returns its bytes. Progress events are
// emitted on "sftp:transfer:<transferID>" while the read runs.
func (s *SftpService) DownloadFile(hostID, remotePath, transferID string) ([]byte, error) {
	return s.svc.DownloadFile(hostID, domain.Credentials{}, remotePath, s.progressEmitter(transferID))
}

// UploadFile writes data to a remote path. Progress events are emitted on
// "sftp:transfer:<transferID>" while the write runs.
func (s *SftpService) UploadFile(hostID, remotePath string, data []byte, transferID string) error {
	return s.svc.UploadFile(hostID, domain.Credentials{}, remotePath, data, s.progressEmitter(transferID))
}

// registerTransfer records a running transfer's cancel func.
func (s *SftpService) registerTransfer(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	if s.transfers == nil {
		s.transfers = make(map[string]context.CancelFunc)
	}
	s.transfers[id] = cancel
	s.mu.Unlock()
}

func (s *SftpService) unregisterTransfer(id string) {
	s.mu.Lock()
	delete(s.transfers, id)
	s.mu.Unlock()
}

// runStreamTransfer executes one streaming transfer in the background and
// emits the terminal "sftp:transfer:<id>:end" event. start runs the
// cancellable work.
func (s *SftpService) runStreamTransfer(transferID string, start func(ctx context.Context) error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.registerTransfer(transferID, cancel)
	go func() {
		defer s.unregisterTransfer(transferID)
		err := start(ctx)
		msg := ""
		if err != nil {
			if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "cancelled") {
				msg = "cancelled"
			} else {
				msg = err.Error()
			}
		}
		if s.app != nil {
			s.app.Event.Emit(fmt.Sprintf("sftp:transfer:%s:end", transferID), msg)
		}
	}()
}

// StartDownload streams a remote file to a local path chosen via the native
// save dialog. Returns immediately; progress and completion arrive as
// events (see the type comment). The remote size is pre-checked against the
// configured ceiling — an oversized file fails with the limit named.
// The source may be a directory: the whole tree downloads then, mirroring
// the remote layout under localPath.
func (s *SftpService) StartDownload(hostID, remotePath, localPath, transferID string) error {
	s.runStreamTransfer(transferID, func(ctx context.Context) error {
		return s.svc.DownloadPath(ctx, hostID, remotePath, localPath, s.progressEmitter(transferID))
	})
	return nil
}

// StartUpload streams a local file (native-open dialog path) to a remote
// path, staging via ".airw-part" and renaming on completion. The source may
// be a directory: the whole tree uploads then, mirrored under remotePath.
func (s *SftpService) StartUpload(hostID, localPath, remotePath, transferID string) error {
	s.runStreamTransfer(transferID, func(ctx context.Context) error {
		return s.svc.UploadPath(ctx, hostID, localPath, remotePath, s.progressEmitter(transferID))
	})
	return nil
}

// CancelTransfer aborts a running streaming transfer by its id.
func (s *SftpService) CancelTransfer(transferID string) {
	s.mu.Lock()
	cancel, ok := s.transfers[transferID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// LocalExists reports whether a local path exists — the download-side
// same-name conflict check (the UI offers overwrite vs auto-rename).
func (s *SftpService) LocalExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// RemoteExists reports whether a remote path exists — the paste-side
// same-name conflict check for remote destinations.
func (s *SftpService) RemoteExists(hostID, remotePath string) (bool, error) {
	_, _, err := s.svc.StatPath(hostID, remotePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, appsvc.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// ── Local pane operations (the SFTP panel's local window) ────────────────

// ListLocalDir lists a local directory (empty dir → the user's home).
func (s *SftpService) ListLocalDir(dir string) ([]FileEntryDTO, error) {
	entries, err := s.svc.LocalListDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list local dir: %w", err)
	}
	out := make([]FileEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, FileEntryDTO{
			Name:    e.Name,
			Size:    e.Size,
			Mode:    e.Mode,
			ModTime: e.ModTime,
			IsDir:   e.IsDir,
		})
	}
	return out, nil
}

// LocalDefaultDir returns the local pane's initial directory (home).
func (s *SftpService) LocalDefaultDir() (string, error) {
	return s.svc.LocalDefaultDir()
}

// LocalMkdir creates a local directory.
func (s *SftpService) LocalMkdir(path string) error {
	return s.svc.LocalMkdir(path)
}

// LocalRename renames/moves a local path.
func (s *SftpService) LocalRename(oldPath, newPath string) error {
	return s.svc.LocalRename(oldPath, newPath)
}

// LocalDelete removes a local file or empty directory.
func (s *SftpService) LocalDelete(path string) error {
	return s.svc.LocalDelete(path)
}

// StartLocalCopy copies a local file or tree to another local path in the
// background (progress/cancel events as with transfers).
func (s *SftpService) StartLocalCopy(srcPath, dstPath, transferID string) error {
	s.runStreamTransfer(transferID, func(ctx context.Context) error {
		return s.svc.CopyLocal(ctx, srcPath, dstPath, s.progressEmitter(transferID))
	})
	return nil
}

// StartRemoteCopy copies a remote file or tree to another path on the same
// host in the background (progress/cancel events as with transfers).
func (s *SftpService) StartRemoteCopy(hostID, srcPath, dstPath, transferID string) error {
	s.runStreamTransfer(transferID, func(ctx context.Context) error {
		return s.svc.CopyWithinHost(ctx, hostID, srcPath, dstPath, s.progressEmitter(transferID))
	})
	return nil
}

// UploadClipboardImage decodes base64 image data and stores it as /tmp/<name>
// on the host, returning the path written. An empty hostID stores the file in
// the LOCAL temp dir instead (local terminal sessions) — the path is inserted
// into the terminal so the user can reference it right away.
func (s *SftpService) UploadClipboardImage(hostID, name, dataB64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty image data")
	}
	// Defend against path traversal in the caller-supplied name.
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, '/') {
		return "", fmt.Errorf("invalid image name %q", name)
	}

	if hostID == "" {
		localPath := filepath.Join(os.TempDir(), name)
		if err := os.WriteFile(localPath, data, 0o644); err != nil {
			return "", fmt.Errorf("write local image: %w", err)
		}
		return localPath, nil
	}
	remotePath := "/tmp/" + name
	if err := s.svc.UploadFile(hostID, domain.Credentials{}, remotePath, data, nil); err != nil {
		return "", err
	}
	return remotePath, nil
}

// DeleteFile removes a remote file or empty directory.
func (s *SftpService) DeleteFile(hostID, remotePath string) error {
	return s.svc.DeleteFile(hostID, domain.Credentials{}, remotePath)
}

// RenameFile renames/moves a remote entry.
func (s *SftpService) RenameFile(hostID, oldPath, newPath string) error {
	return s.svc.RenameFile(hostID, domain.Credentials{}, oldPath, newPath)
}

// Mkdir creates a remote directory.
func (s *SftpService) Mkdir(hostID, remotePath string) error {
	return s.svc.Mkdir(hostID, domain.Credentials{}, remotePath)
}

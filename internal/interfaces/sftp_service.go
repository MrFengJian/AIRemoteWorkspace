package interfaces

import (
	"context"
	"fmt"
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
// Downloads/uploads emit per-transfer progress events named
// "sftp:transfer:<id>" (Go → JS); the frontend supplies the id.
type SftpService struct {
	svc *appsvc.SftpService
	app *wailsapp.App
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

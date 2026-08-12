package interfaces

import (
	"context"
	"fmt"

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

// SftpService exposes remote file operations to the frontend.
//
// The frontend passes empty credentials: the backend resolves remembered
// secrets from the OS vault (Phase 5 SecretStore). If no remembered secret
// exists, the operation fails with an auth error the UI surfaces.
type SftpService struct {
	svc *appsvc.SftpService
}

// NewSftpService wires the Wails SftpService to its application port.
func NewSftpService(svc *appsvc.SftpService) *SftpService {
	return &SftpService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (s *SftpService) ServiceName() string { return "SftpService" }

// ServiceStartup runs when the service is registered with the app.
func (s *SftpService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error { return nil }

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

// DownloadFile reads a remote file and returns its bytes.
func (s *SftpService) DownloadFile(hostID, remotePath string) ([]byte, error) {
	return s.svc.DownloadFile(hostID, domain.Credentials{}, remotePath)
}

// UploadFile writes data to a remote path.
func (s *SftpService) UploadFile(hostID, remotePath string, data []byte) error {
	return s.svc.UploadFile(hostID, domain.Credentials{}, remotePath, data)
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

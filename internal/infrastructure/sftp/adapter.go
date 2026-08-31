package sftp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	sf "github.com/pkg/sftp"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// AppClient adapts a *Manager to the application.SftpClient interface,
// translating infrastructure sftp.Entry values into application.SftpEntry.
type AppClient struct{ m *Manager }

// NewAppClient wraps a Manager as an application.SftpClient.
func NewAppClient(m *Manager) application.SftpClient {
	return &AppClient{m: m}
}

// Compile-time interface check.
var _ application.SftpClient = (*AppClient)(nil)

func (a *AppClient) ListDir(host domain.Host, creds domain.Credentials, dir string) ([]application.SftpEntry, error) {
	entries, err := a.m.ListDir(host, creds, dir)
	if err != nil {
		return nil, err
	}
	out := make([]application.SftpEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, toAppEntry(e))
	}
	return out, nil
}

func (a *AppClient) DownloadFile(host domain.Host, creds domain.Credentials, remotePath string, progress application.SftpProgress) ([]byte, error) {
	return a.m.DownloadFile(host, creds, remotePath, progress)
}

func (a *AppClient) UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte, progress application.SftpProgress) error {
	return a.m.UploadFile(host, creds, remotePath, data, progress)
}

func (a *AppClient) DeleteFile(host domain.Host, creds domain.Credentials, remotePath string) error {
	return a.m.DeleteFile(host, creds, remotePath)
}

func (a *AppClient) RenameFile(host domain.Host, creds domain.Credentials, oldPath, newPath string) error {
	return a.m.RenameFile(host, creds, oldPath, newPath)
}

func (a *AppClient) Mkdir(host domain.Host, creds domain.Credentials, remotePath string) error {
	return a.m.Mkdir(host, creds, remotePath)
}

func (a *AppClient) StatSize(host domain.Host, creds domain.Credentials, remotePath string) (int64, error) {
	return a.m.StatSize(host, creds, remotePath)
}

func (a *AppClient) DownloadToFile(ctx context.Context, host domain.Host, creds domain.Credentials, remotePath, localPath string, chunk int64, progress application.SftpProgress) error {
	return a.m.DownloadToFile(ctx, host, creds, remotePath, localPath, chunk, progress)
}

func (a *AppClient) UploadFromFile(ctx context.Context, host domain.Host, creds domain.Credentials, localPath, remotePath string, chunk int64, progress application.SftpProgress) error {
	return a.m.UploadFromFile(ctx, host, creds, localPath, remotePath, chunk, progress)
}

func (a *AppClient) StatPath(host domain.Host, creds domain.Credentials, remotePath string) (int64, bool, error) {
	size, isDir, err := a.m.StatPath(host, creds, remotePath)
	return size, isDir, mapNotExist(err)
}

func (a *AppClient) CopyRemote(ctx context.Context, host domain.Host, creds domain.Credentials, srcPath, dstPath string, chunk int64, progress application.SftpProgress) error {
	return a.m.CopyRemote(ctx, host, creds, srcPath, dstPath, chunk, progress)
}

// mapNotExist translates the SFTP no-such-file status into the application's
// ErrNotExist sentinel so callers can test absence without string matching.
// pkg/sftp normalises Stat/Open misses into os.ErrNotExist (the StatusError
// itself never escapes), so both error shapes must be recognised.
func mapNotExist(err error) error {
	if err == nil {
		return nil
	}
	var status *sf.StatusError
	if (errors.As(err, &status) && status.FxCode() == sf.ErrSSHFxNoSuchFile) ||
		errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", application.ErrNotExist, err)
	}
	return err
}

func toAppEntry(e Entry) application.SftpEntry {
	return application.SftpEntry{
		Name:    e.Name,
		Size:    e.Size,
		Mode:    e.Mode,
		ModTime: e.ModTime.Format(time.RFC3339),
		IsDir:   e.IsDir,
	}
}

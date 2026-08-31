package application

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ai-remote/workspace/internal/domain"
)

// SftpProgress reports transfer progress. transferred/total are in bytes;
// total is 0 when the size is unknown.
type SftpProgress func(transferred, total int64)

// ErrNotExist is returned when a remote path does not exist (mapped by the
// infrastructure adapter from the SFTP no-such-file status code). The UI
// uses it to distinguish an absent paste target from a real failure.
var ErrNotExist = errors.New("remote entry does not exist")

// SftpClient is the port the SFTP service depends on. Implemented by
// infrastructure/sftp.Manager; kept as an interface for testability.
type SftpClient interface {
	ListDir(host domain.Host, creds domain.Credentials, dir string) ([]SftpEntry, error)
	DownloadFile(host domain.Host, creds domain.Credentials, remotePath string, progress SftpProgress) ([]byte, error)
	UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte, progress SftpProgress) error
	DeleteFile(host domain.Host, creds domain.Credentials, remotePath string) error
	RenameFile(host domain.Host, creds domain.Credentials, oldPath, newPath string) error
	Mkdir(host domain.Host, creds domain.Credentials, remotePath string) error
	// Streaming transfers (FileZilla-style): fixed-memory, cancellable,
	// staged via .part + rename. chunk bytes is the buffer size and progress
	// granularity.
	StatSize(host domain.Host, creds domain.Credentials, remotePath string) (int64, error)
	DownloadToFile(ctx context.Context, host domain.Host, creds domain.Credentials, remotePath, localPath string, chunk int64, progress SftpProgress) error
	UploadFromFile(ctx context.Context, host domain.Host, creds domain.Credentials, localPath, remotePath string, chunk int64, progress SftpProgress) error
	// Copy support: stat a path (tree planning) and stream one remote file
	// to another on the same host (the SFTP protocol has no server-side copy).
	StatPath(host domain.Host, creds domain.Credentials, remotePath string) (size int64, isDir bool, err error)
	CopyRemote(ctx context.Context, host domain.Host, creds domain.Credentials, srcPath, dstPath string, chunk int64, progress SftpProgress) error
}

// SftpEntry mirrors the remote filesystem entry for the application layer.
type SftpEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

// SftpService runs remote file operations, resolving credentials via the
// host service (so remembered OS-vault secrets are reused transparently).
type SftpService struct {
	client  SftpClient
	hosts   HostRepository
	secrets *SecretService
	// transferCfg supplies the current transfer tunables (global settings);
	// nil falls back to the built-in defaults.
	transferCfg func() domain.TransferConfig
}

// NewSftpService wires an SftpService. secrets and cfg may be nil.
func NewSftpService(client SftpClient, hosts HostRepository, secrets *SecretService, cfg func() domain.TransferConfig) *SftpService {
	return &SftpService{client: client, hosts: hosts, secrets: secrets, transferCfg: cfg}
}

// transferConfig returns effective settings with defaults for zero fields.
func (s *SftpService) transferConfig() domain.TransferConfig {
	cfg := domain.TransferConfig{}
	if s.transferCfg != nil {
		cfg = s.transferCfg()
	}
	if cfg.ChunkKB <= 0 {
		cfg.ChunkKB = 256
	}
	if cfg.MaxUploadMB <= 0 {
		cfg.MaxUploadMB = 4096
	}
	if cfg.MaxDownloadMB <= 0 {
		cfg.MaxDownloadMB = 4096
	}
	return cfg
}

// resolve loads a host and fills in remembered credentials for empty fields.
func (s *SftpService) resolve(hostID string, provided domain.Credentials) (domain.Host, domain.Credentials, error) {
	host, err := s.hosts.Get(hostID)
	if err != nil {
		return domain.Host{}, domain.Credentials{}, err
	}
	creds := provided
	if s.secrets != nil {
		if creds.Password == "" && host.AuthType == domain.AuthPassword {
			if v, err := s.secrets.GetHostSecret(hostID, SecretPassword); err == nil {
				creds.Password = string(v)
			}
		}
		if creds.KeyPassphrase == "" && host.AuthType == domain.AuthKey {
			if v, err := s.secrets.GetHostSecret(hostID, SecretPassphrase); err == nil {
				creds.KeyPassphrase = string(v)
			}
		}
	}
	return host, creds, nil
}

// ListDir lists a remote directory.
func (s *SftpService) ListDir(hostID string, creds domain.Credentials, dir string) ([]SftpEntry, error) {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return nil, err
	}
	return s.client.ListDir(host, c, dir)
}

// StatPath stats a remote path (size + directory flag), reporting
// application.ErrNotExist for absent paths.
func (s *SftpService) StatPath(hostID, remotePath string) (int64, bool, error) {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return 0, false, err
	}
	return s.client.StatPath(host, c, remotePath)
}

// DownloadFile downloads a remote file into memory, reporting progress.
func (s *SftpService) DownloadFile(hostID string, creds domain.Credentials, remotePath string, progress SftpProgress) ([]byte, error) {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return nil, err
	}
	return s.client.DownloadFile(host, c, remotePath, progress)
}

// UploadFile uploads data to a remote path, reporting progress.
func (s *SftpService) UploadFile(hostID string, creds domain.Credentials, remotePath string, data []byte, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return err
	}
	return s.client.UploadFile(host, c, remotePath, data, progress)
}

// DeleteFile removes a remote file or empty directory.
func (s *SftpService) DeleteFile(hostID string, creds domain.Credentials, remotePath string) error {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return err
	}
	return s.client.DeleteFile(host, c, remotePath)
}

// RenameFile renames a remote entry.
func (s *SftpService) RenameFile(hostID string, creds domain.Credentials, oldPath, newPath string) error {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return err
	}
	return s.client.RenameFile(host, c, oldPath, newPath)
}

// Mkdir creates a remote directory.
func (s *SftpService) Mkdir(hostID string, creds domain.Credentials, remotePath string) error {
	host, c, err := s.resolve(hostID, creds)
	if err != nil {
		return err
	}
	return s.client.Mkdir(host, c, remotePath)
}

// DownloadToFile streams a remote file to a local path. The remote size is
// pre-checked against the configured download ceiling so oversized files
// fail fast with the limit named (before any bytes move).
func (s *SftpService) DownloadToFile(ctx context.Context, hostID, remotePath, localPath string, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return err
	}
	cfg := s.transferConfig()
	size, err := s.client.StatSize(host, c, remotePath)
	if err != nil {
		return err
	}
	if limit := int64(cfg.MaxDownloadMB) * 1024 * 1024; size > limit {
		return fmt.Errorf("file is %d MB, exceeds the %d MB download limit (adjust in Settings → Advanced)", size/1024/1024, cfg.MaxDownloadMB)
	}
	return s.client.DownloadToFile(ctx, host, c, remotePath, localPath, int64(cfg.ChunkKB)*1024, progress)
}

// UploadFromFile streams a local file to a remote path, with the same
// fast-fail size pre-check on the local file.
func (s *SftpService) UploadFromFile(ctx context.Context, hostID, localPath, remotePath string, progress SftpProgress) error {
	host, c, err := s.resolve(hostID, domain.Credentials{})
	if err != nil {
		return err
	}
	cfg := s.transferConfig()
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local %q: %w", localPath, err)
	}
	if limit := int64(cfg.MaxUploadMB) * 1024 * 1024; info.Size() > limit {
		return fmt.Errorf("file is %d MB, exceeds the %d MB upload limit (adjust in Settings → Advanced)", info.Size()/1024/1024, cfg.MaxUploadMB)
	}
	return s.client.UploadFromFile(ctx, host, c, localPath, remotePath, int64(cfg.ChunkKB)*1024, progress)
}

// SftpError wraps an SFTP error with context for the UI.
func SftpError(op string, err error) error {
	return fmt.Errorf("sftp %s: %w", op, err)
}

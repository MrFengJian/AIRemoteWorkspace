package application

import (
	"fmt"

	"github.com/ai-remote/workspace/internal/domain"
)

// SftpProgress reports transfer progress. transferred/total are in bytes;
// total is 0 when the size is unknown.
type SftpProgress func(transferred, total int64)

// SftpClient is the port the SFTP service depends on. Implemented by
// infrastructure/sftp.Manager; kept as an interface for testability.
type SftpClient interface {
	ListDir(host domain.Host, creds domain.Credentials, dir string) ([]SftpEntry, error)
	DownloadFile(host domain.Host, creds domain.Credentials, remotePath string, progress SftpProgress) ([]byte, error)
	UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte, progress SftpProgress) error
	DeleteFile(host domain.Host, creds domain.Credentials, remotePath string) error
	RenameFile(host domain.Host, creds domain.Credentials, oldPath, newPath string) error
	Mkdir(host domain.Host, creds domain.Credentials, remotePath string) error
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
}

// NewSftpService wires an SftpService. secrets may be nil.
func NewSftpService(client SftpClient, hosts HostRepository, secrets *SecretService) *SftpService {
	return &SftpService{client: client, hosts: hosts, secrets: secrets}
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

// SftpError wraps an SFTP error with context for the UI.
func SftpError(op string, err error) error {
	return fmt.Errorf("sftp %s: %w", op, err)
}

package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/ai-remote/workspace/internal/domain"
)

// HostService implements host CRUD on top of a HostRepository. It is the port
// the Wails HostService delegates to.
type HostService struct {
	repo     HostRepository
	connect  ConnectionManager
	keyStore HostKeyRepository
	secrets  *SecretService
}

// NewHostService wires a HostService to its repository and connection manager.
// The connection manager is used by TestConnection. secrets may be nil (no
// keychain available — "remember password" is then disabled).
func NewHostService(repo HostRepository, connect ConnectionManager, keyStore HostKeyRepository, secrets *SecretService) *HostService {
	return &HostService{repo: repo, connect: connect, keyStore: keyStore, secrets: secrets}
}

// List returns all stored hosts.
func (s *HostService) List() ([]domain.Host, error) {
	return s.repo.List()
}

// Get returns a host by id.
func (s *HostService) Get(id string) (domain.Host, error) {
	return s.repo.Get(id)
}

// Secrets exposes the SecretService to the interface layer (for SaveCredentials).
func (s *HostService) Secrets() *SecretService { return s.secrets }

// CreateHostInput carries the user-editable fields for a new host.
// Credentials are supplied separately (at connect time), never stored.
type CreateHostInput struct {
	Name          string
	Host          string
	Port          int
	Username      string
	AuthType      domain.AuthType
	KeyPath       string // only when AuthType == AuthKey
	TerminalTheme string // per-host terminal colour scheme; "" = default
	Group         string // host group (test/stage/production/custom)
	Tags          []string
}

// Create validates and persists a new host, returning the stored Host with its
// generated ID.
func (s *HostService) Create(in CreateHostInput) (domain.Host, error) {
	if err := validateHostInput(in); err != nil {
		return domain.Host{}, err
	}
	port := in.Port
	if port == 0 {
		port = 22
	}
	h := domain.Host{
		ID:            newHostID(),
		Name:          in.Name,
		Host:          in.Host,
		Port:          port,
		Username:      in.Username,
		AuthType:      in.AuthType,
		TerminalTheme: in.TerminalTheme,
		Group:         in.Group,
		Tags:          in.Tags,
	}
	if err := s.repo.Save(h); err != nil {
		return domain.Host{}, err
	}
	return h, nil
}

// Update mutates an existing host's editable fields.
func (s *HostService) Update(id string, in CreateHostInput) (domain.Host, error) {
	if err := validateHostInput(in); err != nil {
		return domain.Host{}, err
	}
	existing, err := s.repo.Get(id)
	if err != nil {
		return domain.Host{}, err
	}
	port := in.Port
	if port == 0 {
		port = 22
	}
	existing.Name = in.Name
	existing.Host = in.Host
	existing.Port = port
	existing.Username = in.Username
	existing.AuthType = in.AuthType
	existing.TerminalTheme = in.TerminalTheme
	existing.Group = in.Group
	existing.Tags = in.Tags
	if err := s.repo.Save(existing); err != nil {
		return domain.Host{}, err
	}
	return existing, nil
}

// Delete removes a host by id and clears any remembered secrets for it
// (AGENT.md §9: avoid orphaned vault entries).
func (s *HostService) Delete(id string) error {
	if s.secrets != nil {
		if err := s.secrets.DeleteHostSecrets(id); err != nil {
			// Log-worthy but not fatal: the host row is the source of truth,
			// and a leftover vault entry is inert without it.
			_ = err
		}
	}
	return s.repo.Delete(id)
}

// EnsureOS detects the OS of the connected session and persists it on the host
// if not already recorded. It is read-only from the user's perspective — the
// OS field is never in the edit inputs. Failures are silent: a detection
// problem must never break an otherwise successful connection.
func (s *HostService) EnsureOS(hostID, sessionID string) {
	host, err := s.repo.Get(hostID)
	if err != nil || host.OS != "" {
		return // already known, or host gone
	}
	osID, err := s.connect.DetectOS(sessionID)
	if err != nil || osID == "" {
		return // silent failure
	}
	host.OS = osID
	_ = s.repo.Save(host) // best-effort persist
}

// ResolveCredentials returns credentials suitable for a connection attempt.
// If the caller supplied a full secret (password/passphrase), it is used as-is.
// Otherwise the remembered secret is loaded from the OS vault when available.
//
// This lets "remember password" work transparently: callers pass an empty
// Credentials and get the remembered one back.
func (s *HostService) ResolveCredentials(host domain.Host, provided domain.Credentials) (domain.Credentials, error) {
	creds := provided
	if s.secrets == nil {
		return creds, nil
	}
	if creds.Password == "" && host.AuthType == domain.AuthPassword {
		if v, err := s.secrets.GetHostSecret(host.ID, SecretPassword); err == nil {
			creds.Password = string(v)
		} else if !errors.Is(err, ErrSecretNotFound) {
			return creds, fmt.Errorf("load remembered password: %w", err)
		}
	}
	if creds.KeyPassphrase == "" && host.AuthType == domain.AuthKey {
		if v, err := s.secrets.GetHostSecret(host.ID, SecretPassphrase); err == nil {
			creds.KeyPassphrase = string(v)
		} else if !errors.Is(err, ErrSecretNotFound) {
			return creds, fmt.Errorf("load remembered passphrase: %w", err)
		}
	}
	return creds, nil
}

// TestConnection attempts a short-lived dial + PTY open against the host with
// the given credentials, then closes immediately. Returns nil on success.
//
// It reuses the ConnectionManager but tears the session down right away — we
// only care that the handshake + auth + pty-req succeed.
func (s *HostService) TestConnection(ctx context.Context, host domain.Host, creds domain.Credentials) error {
	noopEvents := noopSessionEvents{}
	sessionID, err := s.connect.OpenSession(ctx, host, creds, 80, 24, &noopEvents)
	if err != nil {
		return err
	}
	// Give the PTY a moment to establish, then close.
	return s.connect.Close(sessionID)
}

// noopSessionEvents absorbs session lifecycle events during a connection test.
type noopSessionEvents struct{}

func (noopSessionEvents) OnData(string, []byte) {}
func (noopSessionEvents) OnExit(string, error)  {}

func validateHostInput(in CreateHostInput) error {
	if in.Name == "" {
		return errors.New("name is required")
	}
	if in.Host == "" {
		return errors.New("host is required")
	}
	if in.Username == "" {
		return errors.New("username is required")
	}
	if in.AuthType == "" {
		return errors.New("auth type is required")
	}
	return nil
}

// newHostID returns a stable identifier. Uses crypto-strong random; defined
// here (not in domain) to keep domain free of infra deps. Implementation in
// id.go.
func newHostID() string { return newID() }

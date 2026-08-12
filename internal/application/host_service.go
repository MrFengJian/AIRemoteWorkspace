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
}

// NewHostService wires a HostService to its repository and connection manager.
// The connection manager is used by TestConnection.
func NewHostService(repo HostRepository, connect ConnectionManager, keyStore HostKeyRepository) *HostService {
	return &HostService{repo: repo, connect: connect, keyStore: keyStore}
}

// List returns all stored hosts.
func (s *HostService) List() ([]domain.Host, error) {
	return s.repo.List()
}

// Get returns a host by id.
func (s *HostService) Get(id string) (domain.Host, error) {
	return s.repo.Get(id)
}

// CreateHostInput carries the user-editable fields for a new host.
// Credentials are supplied separately (at connect time), never stored.
type CreateHostInput struct {
	Name     string
	Host     string
	Port     int
	Username string
	AuthType domain.AuthType
	KeyPath  string // only when AuthType == AuthKey
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
		ID:       newHostID(),
		Name:     in.Name,
		Host:     in.Host,
		Port:     port,
		Username: in.Username,
		AuthType: in.AuthType,
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
	if err := s.repo.Save(existing); err != nil {
		return domain.Host{}, err
	}
	return existing, nil
}

// Delete removes a host by id.
func (s *HostService) Delete(id string) error {
	return s.repo.Delete(id)
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
	closeErr := s.connect.Close(sessionID)
	if closeErr != nil && !errors.Is(closeErr, errSessionClosedGracefully) {
		return closeErr
	}
	return nil
}

// errSessionClosedGracefully is a light sentinel; Close on an already-exited
// session returns nil from the manager, so this is mostly defensive.
var errSessionClosedGracefully = fmt.Errorf("session already closed")

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

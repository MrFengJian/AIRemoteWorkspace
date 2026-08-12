package application

import (
	"errors"
	"fmt"
)

// SecretStore is the port for OS-backed credential storage. Implemented by
// infrastructure/secret; never backed by SQLite (AGENT.md §9).
//
// Implementations MUST return ErrSecretNotFound from Get when no entry exists.
type SecretStore interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

// SecretKind labels what a stored secret is used for.
type SecretKind string

const (
	SecretPassword   SecretKind = "password"
	SecretPassphrase SecretKind = "passphrase"
)

// ErrSecretNotFound is returned by SecretStore.Get when no entry exists.
// Infrastructure backends translate their platform-specific "not found" errors
// into this sentinel.
var ErrSecretNotFound = errors.New("secret not found")

// SecretService wraps a SecretStore with host-scoped key conventions. Keys
// follow the stable scheme: airemote:host:<hostID>:<kind>.
type SecretService struct {
	store SecretStore
}

// NewSecretService wires a SecretService to a backend store.
func NewSecretService(store SecretStore) *SecretService {
	return &SecretService{store: store}
}

// secretKey builds the keychain key for a host secret.
func secretKey(hostID string, kind SecretKind) string {
	return fmt.Sprintf("airemote:host:%s:%s", hostID, kind)
}

// SaveHostSecret stores a host secret in the OS credential vault.
func (s *SecretService) SaveHostSecret(hostID string, kind SecretKind, value []byte) error {
	if len(value) == 0 {
		// Empty value = clear any prior entry rather than store a blank.
		return s.DeleteHostSecret(hostID, kind)
	}
	if err := s.store.Set(secretKey(hostID, kind), value); err != nil {
		return fmt.Errorf("save %s secret: %w", kind, err)
	}
	return nil
}

// GetHostSecret retrieves a host secret. Returns ErrSecretNotFound if absent.
func (s *SecretService) GetHostSecret(hostID string, kind SecretKind) ([]byte, error) {
	return s.store.Get(secretKey(hostID, kind))
}

// DeleteHostSecret removes one host secret. Missing keys are not an error.
func (s *SecretService) DeleteHostSecret(hostID string, kind SecretKind) error {
	return s.store.Delete(secretKey(hostID, kind))
}

// DeleteHostSecrets removes all known host secrets (password + passphrase).
// Called when a host is deleted to avoid orphaned vault entries.
func (s *SecretService) DeleteHostSecrets(hostID string) error {
	for _, kind := range []SecretKind{SecretPassword, SecretPassphrase} {
		if err := s.DeleteHostSecret(hostID, kind); err != nil {
			return err
		}
	}
	return nil
}

// HasHostSecret reports whether any secret is stored for the host (either kind).
func (s *SecretService) HasHostSecret(hostID string) bool {
	for _, kind := range []SecretKind{SecretPassword, SecretPassphrase} {
		if _, err := s.GetHostSecret(hostID, kind); err == nil {
			return true
		}
	}
	return false
}

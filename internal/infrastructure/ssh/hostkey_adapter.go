package ssh

import (
	"errors"

	"github.com/ai-remote/workspace/internal/application"
)

// hostKeyStoreAdapter adapts an application.HostKeyRepository to the
// HostKeyStore interface this package expects (string-tuple returns).
type hostKeyStoreAdapter struct {
	repo application.HostKeyRepository
}

// FromHostKeyRepo wraps an application.HostKeyRepository as a HostKeyStore,
// translating application.ErrHostKeyNotFound into this package's ErrNotFound.
func FromHostKeyRepo(repo application.HostKeyRepository) HostKeyStore {
	return &hostKeyStoreAdapter{repo: repo}
}

func (a *hostKeyStoreAdapter) Get(hostID string) (string, string, error) {
	k, err := a.repo.Get(hostID)
	if err != nil {
		if errors.Is(err, application.ErrHostKeyNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return k.Algorithm, k.Fingerprint, nil
}

func (a *hostKeyStoreAdapter) Upsert(hostID, algorithm, fingerprint string) error {
	return a.repo.Upsert(hostID, algorithm, fingerprint)
}

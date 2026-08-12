//go:build darwin

package secret

import (
	"errors"

	"github.com/zalando/go-keyring"

	"github.com/ai-remote/workspace/internal/application"
)

// serviceName is the Keychain "service" under which all AI Remote Workspace
// secrets are filed. The per-secret key becomes the "account" (user).
const serviceName = "AI Remote Workspace"

// defaultStore is the macOS Keychain backend (via /usr/bin/security, no CGO).
var defaultStore application.SecretStore = &keyringStore{}

type keyringStore struct{}

func (keyringStore) Set(key string, value []byte) error {
	return keyring.Set(serviceName, key, string(value))
}

func (keyringStore) Get(key string) ([]byte, error) {
	v, err := keyring.Get(serviceName, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, application.ErrSecretNotFound
		}
		return nil, err
	}
	return []byte(v), nil
}

func (keyringStore) Delete(key string) error {
	err := keyring.Delete(serviceName, key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

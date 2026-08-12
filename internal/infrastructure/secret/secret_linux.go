//go:build linux

package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// serviceName is the Secret Service collection label under which all AI Remote
// Workspace secrets are filed. The per-secret key becomes the "account" (user).
const serviceName = "AI Remote Workspace"

// defaultStore is the Linux Secret Service backend (via godbus D-Bus, no CGO).
// Requires a running secret service provider (GNOME Keyring / KWallet).
var defaultStore Store = &keyringStore{}

type keyringStore struct{}

func (keyringStore) Set(key string, value []byte) error {
	return keyring.Set(serviceName, key, string(value))
}

func (keyringStore) Get(key string) ([]byte, error) {
	v, err := keyring.Get(serviceName, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrNotFound
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

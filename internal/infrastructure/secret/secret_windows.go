//go:build windows

package secret

import (
	"errors"

	"github.com/danieljoos/wincred"

	"github.com/ai-remote/workspace/internal/application"
)

// defaultStore is the Windows Credential Manager backend.
var defaultStore application.SecretStore = &wincredStore{}

type wincredStore struct{}

func (wincredStore) Set(key string, value []byte) error {
	c := wincred.NewGenericCredential(key)
	c.CredentialBlob = value
	// PersistLocalMachine (wincred default): visible to the same user on this
	// machine, survives reboots. Session-scoped persistence would vanish on
	// restart, defeating "remember password".
	return c.Write()
}

func (wincredStore) Get(key string) ([]byte, error) {
	c, err := wincred.GetGenericCredential(key)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil, application.ErrSecretNotFound
		}
		return nil, err
	}
	return c.CredentialBlob, nil
}

func (wincredStore) Delete(key string) error {
	c, err := wincred.GetGenericCredential(key)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil // deleting a missing key is not an error
		}
		return err
	}
	return c.Delete()
}

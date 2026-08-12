//go:build !windows && !darwin && !linux

package secret

import (
	"github.com/ai-remote/workspace/internal/application"
)

// defaultStore is a no-op on unsupported platforms (e.g. BSD, mobile). Callers
// should detect application.ErrSecretNotFound and fall back to in-memory-only
// credentials.
var defaultStore application.SecretStore = unsupportedStore{}

type unsupportedStore struct{}

func (unsupportedStore) Set(string, []byte) error   { return nil }
func (unsupportedStore) Get(string) ([]byte, error) { return nil, application.ErrSecretNotFound }
func (unsupportedStore) Delete(string) error        { return nil }

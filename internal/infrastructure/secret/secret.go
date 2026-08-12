// Package secret provides a cross-platform SecretStore backed by the OS
// credential vault (Windows Credential Manager, macOS Keychain, Linux Secret
// Service). Secrets never touch SQLite — only an opaque reference does
// (AGENT.md §8.1, §9).
//
// All backends build with CGO_ENABLED=0, keeping the single-binary promise.
package secret

import "errors"

// Store is the abstraction every backend implements. Methods must be safe for
// sequential use from the UI thread.
type Store interface {
	// Set stores value under key, overwriting any existing entry.
	Set(key string, value []byte) error
	// Get returns the value for key, or ErrNotFound if absent.
	Get(key string) ([]byte, error)
	// Delete removes the entry for key. Deleting a missing key is not an error.
	Delete(key string) error
}

// ErrNotFound indicates no entry exists for the given key.
var ErrNotFound = errors.New("secret: not found")

// ErrUnsupported indicates the platform has no SecretStore backend.
var ErrUnsupported = errors.New("secret: no credential store on this platform")

// Default returns the platform-native SecretStore, or an unsupportedStore on
// platforms without a backend. The concrete implementation is selected via
// build tags in secret_<platform>.go.
func Default() Store { return defaultStore }

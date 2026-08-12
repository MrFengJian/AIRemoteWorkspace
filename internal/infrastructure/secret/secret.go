// Package secret provides a cross-platform SecretStore backed by the OS
// credential vault (Windows Credential Manager, macOS Keychain, Linux Secret
// Service). Secrets never touch SQLite — only an opaque reference does
// (AGENT.md §8.1, §9).
//
// All backends build with CGO_ENABLED=0, keeping the single-binary promise.
//
// The port interface + sentinel errors live in internal/application; this
// package supplies platform-specific implementations (selected via build tags).
package secret

import "github.com/ai-remote/workspace/internal/application"

// Default returns the platform-native SecretStore. On unsupported platforms a
// no-op store is returned whose Get yields application.ErrSecretNotFound.
func Default() application.SecretStore { return defaultStore }

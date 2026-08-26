// Package application orchestrates business flows on top of domain types.
// It depends only on domain and on port interfaces defined here — never on
// concrete infrastructure (AGENT.md §7 layering).
package application

import (
	"context"
	"errors"

	"github.com/ai-remote/workspace/internal/domain"
)

// ConfigService is the port the Wails ConfigService implements.
// Keeping it as an interface here means the UI layer can be tested against a
// stub without a database.
type ConfigService interface {
	GetAppConfig() (domain.AppConfig, error)
	SetAppConfig(cfg domain.AppConfig) error
}

// ConfigRepository is the persistence port for AppConfig.
// Implemented by infrastructure/sqlite.ConfigRepo.
type ConfigRepository interface {
	Get() (domain.AppConfig, error)
	Set(cfg domain.AppConfig) error
}

// ProviderRepository is the persistence port for LLM model providers. Stored
// separately from AppConfig so whole-config saves can't clobber provider edits.
// Implemented by infrastructure/sqlite.ConfigRepo.
type ProviderRepository interface {
	GetProviders() ([]domain.ModelProvider, error)
	SetProviders(providers []domain.ModelProvider) error
}

// HostRepository is the persistence port for hosts.
type HostRepository interface {
	List() ([]domain.Host, error)
	Get(id string) (domain.Host, error)
	Save(h domain.Host) error
	Delete(id string) error
}

// HostKeyRepository persists recorded SSH host fingerprints.
type HostKeyRepository interface {
	// Get returns the recorded key for hostID, or ErrHostKeyNotFound if none.
	Get(hostID string) (domain.HostKey, error)
	// Upsert records or replaces the fingerprint (and refreshes last_seen).
	Upsert(hostID, algorithm, fingerprint string) error
}

// SessionEvents delivers PTY lifecycle events to the layer above (the Wails
// service, which forwards them over the event bus to xterm.js).
type SessionEvents interface {
	// OnData ships a chunk of remote stdout/stderr.
	OnData(sessionID string, data []byte)
	// OnExit reports the shell has exited; exitErr carries the wait result
	// (nil on a clean exit). Resources for the session are released after.
	OnExit(sessionID string, exitErr error)
	// OnProgress reports connection-establishment stages ("connect",
	// "handshake", "session") so the UI can show what the dial is doing.
	// Called before the session exists; implementations may no-op.
	OnProgress(sessionID, stage string)
}

// ConnectionManager owns live SSH connections and their PTY sessions.
type ConnectionManager interface {
	// OpenSession dials the host, authenticates, and starts an interactive
	// PTY shell. Remote output is delivered via events.OnData; termination
	// via events.OnExit. The returned sessionID names the live session.
	OpenSession(ctx context.Context, host domain.Host, creds domain.Credentials, cols, rows int, events SessionEvents) (sessionID string, err error)
	// WriteStdin forwards local input to a session's remote shell.
	WriteStdin(sessionID string, data []byte) error
	// Resize updates a session's PTY dimensions.
	Resize(sessionID string, cols, rows int) error
	// Close ends a session and frees its connection.
	Close(sessionID string) error
	// DetectOS inspects the session's host and returns the distro id
	// (e.g. "ubuntu"). Returns "" when detection failed.
	DetectOS(sessionID string) (string, error)
	// ExecInSessionCtx runs a one-shot command on the connection backing
	// sessionID (fresh non-interactive exec channel; the interactive PTY is
	// untouched). Used by the agent's ssh_exec tool and the monitor collector.
	ExecInSessionCtx(ctx context.Context, sessionID, cmd string) (string, error)
	// CloseAll tears down every active session (used on app shutdown).
	CloseAll() error
}

// ToolRegistry holds the available Tools and dispatches calls through the
// permission gate (Phase 4).
type ToolRegistry interface {
	Registered() []domain.Tool
}

// ConversationRepository persists agent chat history. Implemented by
// infrastructure/sqlite.ConversationRepo. Children (messages) are cleaned up
// with their conversation by the implementation.
type ConversationRepository interface {
	// List returns all conversations, newest first.
	List() ([]domain.Conversation, error)
	// Create stores a new conversation.
	Create(c domain.Conversation) error
	// Touch refreshes a conversation's UpdatedAt (after a new turn).
	Touch(conversationID string) error
	// ListMessages returns a conversation's user/assistant messages in order.
	ListMessages(conversationID string) ([]domain.ConversationMessage, error)
	// AppendMessage stores one message in a conversation.
	AppendMessage(conversationID string, role, content string) error
	// Delete removes a conversation and its messages.
	Delete(conversationID string) error
}

// Sentinel errors shared across the application layer. Use errors.Is to test.
var (
	// ErrHostKeyNotFound indicates no recorded fingerprint exists yet.
	ErrHostKeyNotFound = errors.New("host key not found")
)

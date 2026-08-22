package ssh

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// Manager implements application.ConnectionManager over real SSH connections.
//
// Each OpenSession dials a fresh *Client (one connection per session for now —
// simple and matches how users expect independent terminal tabs to behave).
type Manager struct {
	keyStore HostKeyStore // for host-key verification

	mu       sync.Mutex
	sessions map[string]*managedSession
}

type managedSession struct {
	id     string
	client *Client
	pty    *PtySession
	host   domain.Host
}

// ErrSessionNotFound is returned when a session id is unknown to the manager.
var ErrSessionNotFound = errors.New("session not found")

// NewManager builds a Manager. keyStore provides known_hosts verification.
func NewManager(keyStore HostKeyStore) *Manager {
	return &Manager{
		keyStore: keyStore,
		sessions: make(map[string]*managedSession),
	}
}

// Compile-time interface check.
var _ application.ConnectionManager = (*Manager)(nil)

// OpenSession dials, authenticates, and starts a PTY shell.
func (m *Manager) OpenSession(
	ctx context.Context,
	host domain.Host,
	creds domain.Credentials,
	cols, rows int,
	events application.SessionEvents,
) (string, error) {

	auth := Auth{
		Password:      creds.Password,
		KeyPath:       creds.KeyPath,
		KeyPassphrase: creds.KeyPassphrase,
		UseAgent:      creds.UseAgent,
	}
	opts := ConnectOptions{
		HostID:   host.ID,
		Host:     host.Host,
		Port:     host.Port,
		Username: host.Username,
	}

	client, err := Dial(opts, auth, m.keyStore)
	if err != nil {
		return "", err
	}

	sessionID := uuid.NewString()

	// Output handler routes PTY chunks to the events sink.
	onOutput := func(data []byte) {
		if events != nil {
			events.OnData(sessionID, data)
		}
	}

	pty, err := NewPtySession(client, cols, rows, onOutput)
	if err != nil {
		_ = client.Close()
		return "", err
	}

	ms := &managedSession{
		id:     sessionID,
		client: client,
		pty:    pty,
		host:   host,
	}

	m.mu.Lock()
	m.sessions[sessionID] = ms
	m.mu.Unlock()

	// Watch the session lifecycle: when Wait returns, emit OnExit and clean up.
	go func() {
		waitErr := pty.Wait()
		if events != nil {
			events.OnExit(sessionID, waitErr)
		}
		m.removeSession(sessionID)
	}()

	return sessionID, nil
}

// WriteStdin forwards input bytes to a session's PTY.
func (m *Manager) WriteStdin(sessionID string, data []byte) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	return ms.pty.WriteStdin(data)
}

// Resize updates a session's PTY dimensions.
func (m *Manager) Resize(sessionID string, cols, rows int) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	return ms.pty.Resize(cols, rows)
}

// Close ends a session, its PTY, and the underlying SSH client.
func (m *Manager) Close(sessionID string) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	_ = ms.pty.Close()
	err := ms.client.Close()
	m.removeSession(sessionID)
	return err
}

// ExecInSession runs a one-shot command on the SSH connection backing sessionID
// (opens a fresh non-interactive session — does not disturb the PTY). Returns
// combined stdout+stderr. Used by the Agent's ssh_exec tool.
func (m *Manager) ExecInSession(sessionID, cmd string) (string, error) {
	return m.ExecInSessionCtx(context.Background(), sessionID, cmd)
}

// ExecInSessionCtx is ExecInSession with cancellation: when ctx is done the
// exec session is closed, which terminates the remote command (the same way
// closing an interactive session does). Used so the agent's Stop button can
// interrupt long-running remote commands.
func (m *Manager) ExecInSessionCtx(ctx context.Context, sessionID, cmd string) (string, error) {
	ms, ok := m.session(sessionID)
	if !ok {
		return "", errSessionNotFound(sessionID)
	}
	sess, err := ms.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new exec session: %w", err)
	}
	defer sess.Close()

	type execResult struct {
		out []byte
		err error
	}
	done := make(chan execResult, 1)
	go func() {
		out, err := sess.CombinedOutput(cmd)
		done <- execResult{out, err}
	}()
	select {
	case r := <-done:
		return string(r.out), r.err
	case <-ctx.Done():
		// Closing the session kills the remote command; wait for the reader
		// goroutine so the session isn't used after Close returns.
		_ = sess.Close()
		<-done
		return "", ctx.Err()
	}
}

// HostOfSession returns the domain.Host associated with a session.
func (m *Manager) HostOfSession(sessionID string) (domain.Host, bool) {
	ms, ok := m.session(sessionID)
	if !ok {
		return domain.Host{}, false
	}
	return ms.host, true
}

// CloseAll tears down every active session.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var firstErr error
	for _, id := range ids {
		if err := m.Close(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) session(id string) (*managedSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms, ok := m.sessions[id]
	return ms, ok
}

func (m *Manager) removeSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// errSessionNotFound returns a descriptive error for an unknown session id.
func errSessionNotFound(id string) error {
	return fmt.Errorf("session %q not found", id)
}

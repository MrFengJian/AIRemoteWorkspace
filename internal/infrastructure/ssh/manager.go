package ssh

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// Manager implements application.ConnectionManager over real SSH connections.
//
// Each OpenSession dials a fresh *Client (one connection per session for now —
// simple and matches how users expect independent terminal tabs to behave).
// When a session's connection dies, the manager auto-reconnects it IN PLACE —
// same session id, fresh client + PTY — with exponential backoff, so the
// frontend tab (and its scrollback) survives the drop. A shell that exits
// normally while the link is healthy is NOT reconnected.
type Manager struct {
	keyStore HostKeyStore // for host-key verification

	mu       sync.Mutex
	sessions map[string]*managedSession
}

// Session auto-reconnect tuning (Phase 8): exponential backoff between
// redials, capped, with a hard attempt limit after which the session is
// reported dead to the frontend.
const (
	reconnectBase     = 2 * time.Second
	reconnectMax      = 30 * time.Second
	reconnectAttempts = 10
)

type managedSession struct {
	mu     sync.Mutex
	id     string
	client *Client
	pty    *PtySession
	host   domain.Host
	// Everything the auto-reconnect needs: resolved credentials (session
	// lifetime, same scope as the connection itself), the event sink, the
	// output handler, and the latest PTY size to apply on redial.
	creds    domain.Credentials
	events   application.SessionEvents
	onOutput OutputHandler
	cols     int
	rows     int
	// closing marks a user-initiated close: the watcher must not reconnect.
	// stopCh aborts an in-flight backoff sleep immediately.
	closing bool
	stopCh  chan struct{}
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

	// The session id is assigned before dialing so progress events can be
	// attributed to this session from the very first stage.
	sessionID := uuid.NewString()
	if events != nil {
		opts.OnProgress = func(stage string) {
			events.OnProgress(sessionID, stage)
		}
	}

	client, err := Dial(opts, auth, m.keyStore)
	if err != nil {
		return "", err
	}

	// Output handler routes PTY chunks to the events sink.
	onOutput := func(data []byte) {
		if events != nil {
			events.OnData(sessionID, data)
		}
	}

	if events != nil {
		events.OnProgress(sessionID, "session")
	}
	pty, err := NewPtySession(client, cols, rows, onOutput)
	if err != nil {
		_ = client.Close()
		return "", err
	}

	ms := &managedSession{
		id:       sessionID,
		client:   client,
		pty:      pty,
		host:     host,
		creds:    creds,
		events:   events,
		onOutput: onOutput,
		cols:     cols,
		rows:     rows,
		stopCh:   make(chan struct{}),
	}

	m.mu.Lock()
	m.sessions[sessionID] = ms
	m.mu.Unlock()

	// Watch the session lifecycle: on connection death, auto-reconnect in
	// place; on normal shell exit or give-up, emit OnExit and clean up.
	go m.watchSession(ms, pty)

	return sessionID, nil
}

// watchSession owns one PTY's lifecycle. When Wait returns it decides between
// three endings: user-closed (no reconnect), normal shell exit while the link
// is healthy (no reconnect), or a dead link (auto-reconnect with backoff —
// same session id, fresh client + PTY; the frontend tab never notices beyond
// the reconnecting events).
func (m *Manager) watchSession(ms *managedSession, pty *PtySession) {
	waitErr := pty.Wait()

	if ms.isClosing() {
		m.finishSession(ms, waitErr)
		return
	}
	// Healthy link + exited shell = the user (or remote admin) ended the
	// session on purpose; reconnecting would fight them.
	if ms.currentClient().Alive() {
		m.finishSession(ms, waitErr)
		return
	}

	lastErr := waitErr
	backoff := reconnectBase
	for attempt := 1; attempt <= reconnectAttempts; attempt++ {
		if ms.isClosing() {
			m.finishSession(ms, waitErr)
			return
		}
		if ms.events != nil {
			ms.events.OnReconnecting(ms.id, attempt)
		}
		select {
		case <-ms.stopCh:
			m.finishSession(ms, waitErr)
			return
		case <-time.After(backoff):
		}
		if ms.isClosing() {
			m.finishSession(ms, waitErr)
			return
		}

		client, err := m.redial(ms)
		if err != nil {
			lastErr = err
			backoff = min(backoff*2, reconnectMax)
			continue
		}
		cols, rows := ms.snapshotSize()
		newPty, err := NewPtySession(client, cols, rows, ms.snapshotOutput())
		if err != nil {
			_ = client.Close()
			lastErr = err
			backoff = min(backoff*2, reconnectMax)
			continue
		}
		// Success: swap client + PTY in place and watch the new pair. The
		// stream resumes on the same session id; attempt 0 signals success.
		m.swapSession(ms, client, newPty)
		if ms.events != nil {
			ms.events.OnReconnecting(ms.id, 0)
		}
		go m.watchSession(ms, newPty)
		return
	}

	// Attempts exhausted — surface the last error through the normal exit
	// path (the frontend marks the tab as failed, manual reconnect remains).
	m.finishSession(ms, lastErr)
}

// redial opens a fresh connection for the session's host with its resolved
// credentials. No progress callbacks — the UI sees reconnecting events
// instead.
func (m *Manager) redial(ms *managedSession) (*Client, error) {
	creds := ms.snapshotCreds()
	return Dial(ConnectOptions{
		HostID:   ms.host.ID,
		Host:     ms.host.Host,
		Port:     ms.host.Port,
		Username: ms.host.Username,
	}, Auth{
		Password:      creds.Password,
		KeyPath:       creds.KeyPath,
		KeyPassphrase: creds.KeyPassphrase,
		UseAgent:      creds.UseAgent,
	}, m.keyStore)
}

// ── managedSession accessors (all state guarded by ms.mu) ───────────────

func (ms *managedSession) isClosing() bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.closing
}

func (ms *managedSession) markClosing() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if !ms.closing {
		ms.closing = true
		close(ms.stopCh)
	}
}

func (ms *managedSession) currentClient() *Client {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.client
}

func (ms *managedSession) snapshotCreds() domain.Credentials {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.creds
}

func (ms *managedSession) snapshotOutput() OutputHandler {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.onOutput
}

// snapshotSize returns the latest requested PTY size — kept across a
// reconnect gap so the replacement PTY opens with the right dimensions.
func (ms *managedSession) snapshotSize() (cols, rows int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.cols, ms.rows
}

func (ms *managedSession) setSize(cols, rows int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.cols, ms.rows = cols, rows
}

// swapSession replaces a session's client + PTY after a successful redial.
func (m *Manager) swapSession(ms *managedSession, client *Client, pty *PtySession) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	_ = ms.client.Close() // the old link is dead regardless
	ms.client = client
	ms.pty = pty
}

// finishSession is the single exit path: report to the frontend, drop the
// session from the manager.
func (m *Manager) finishSession(ms *managedSession, waitErr error) {
	if ms.events != nil {
		ms.events.OnExit(ms.id, waitErr)
	}
	m.removeSession(ms.id)
}

// reconnectBackoff returns the sleep before the given attempt (1-based).
// Pure helper so the escalation stays unit-testable.
func reconnectBackoff(attempt int) time.Duration {
	d := reconnectBase
	for i := 1; i < attempt; i++ {
		d = min(d*2, reconnectMax)
	}
	return d
}

// WriteStdin forwards input bytes to a session's PTY. Keystrokes typed
// during a reconnect gap are dropped (the shell they belonged to is gone) —
// the error surfaces as ErrPtyClosed, which callers treat as transient.
func (m *Manager) WriteStdin(sessionID string, data []byte) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	return ms.pty.WriteStdin(data)
}

// Resize updates a session's PTY dimensions. The size is ALWAYS remembered —
// a resize landing in a reconnect gap is applied to the replacement PTY.
func (m *Manager) Resize(sessionID string, cols, rows int) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	ms.setSize(cols, rows)
	if err := ms.pty.Resize(cols, rows); err != nil {
		if errors.Is(err, ErrPtyClosed) {
			return nil // mid-reconnect; applied on the fresh PTY
		}
		return err
	}
	return nil
}

// Close ends a session, its PTY, and the underlying SSH client. Marks the
// session as user-closed FIRST so the lifecycle watcher does not treat the
// teardown as a dropped link and try to reconnect.
func (m *Manager) Close(sessionID string) error {
	ms, ok := m.session(sessionID)
	if !ok {
		return errSessionNotFound(sessionID)
	}
	ms.markClosing()
	_ = ms.pty.Close()
	err := ms.currentClient().Close()
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
	sess, err := ms.currentClient().NewSession()
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

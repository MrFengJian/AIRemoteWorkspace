// Package localpty runs interactive shells on the local machine over a
// pseudo-terminal — no SSH involved. It mirrors the session lifecycle of
// infrastructure/ssh.Manager (uuid-keyed sessions, SessionEvents callbacks)
// so the Wails TerminalService can route between the two by session-id
// prefix ("local-" belongs here).
//
// The go-pty package provides the cross-platform PTY: openpty on Unix,
// ConPTY on Windows — pure Go, no CGO (a project constraint).
package localpty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/aymanbagabas/go-pty"
	"github.com/google/uuid"

	"github.com/ai-remote/workspace/internal/application"
)

// SessionIDPrefix marks a session as local (routing key in TerminalService).
const SessionIDPrefix = "local-"

// ErrSessionNotFound is returned when a session id is unknown to the manager.
var ErrSessionNotFound = errors.New("local session not found")

// IsLocal reports whether a session id belongs to this manager.
func IsLocal(sessionID string) bool {
	return strings.HasPrefix(sessionID, SessionIDPrefix)
}

type localSession struct {
	id  string
	pty pty.Pty
	cmd *pty.Cmd
	// go-pty's conPty.Close is NOT idempotent: a second ClosePseudoConsole
	// runs on a freed handle (which the kernel may have reused), and on
	// Windows that kills the calling process with STATUS_FATAL_APP_EXIT —
	// closing a local terminal tab took the whole app down. The once guards
	// the two legitimate closers (user-triggered Close + the output pump's
	// teardown after the shell exits on its own).
	closeOnce sync.Once
}

/** closePty tears the PTY down exactly once per session. */
func (s *localSession) closePty() {
	s.closeOnce.Do(func() { _ = s.pty.Close() })
}

// Manager owns live local PTY sessions. Safe for concurrent use.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*localSession
}

// NewManager builds a local PTY manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*localSession)}
}

// Open starts an interactive local shell attached to a fresh PTY. Output is
// streamed via events.OnData; termination via events.OnExit — the same
// contract the SSH manager honours, so the frontend needs no changes.
func (m *Manager) Open(cols, rows int, events application.SessionEvents) (string, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	shell, shellArgs, err := detectShell()
	if err != nil {
		return "", err
	}

	p, err := pty.New()
	if err != nil {
		return "", fmt.Errorf("create pty: %w", err)
	}
	// go-pty takes (width=cols, height=rows).
	if err := p.Resize(cols, rows); err != nil {
		_ = p.Close()
		return "", fmt.Errorf("resize pty: %w", err)
	}

	sessionID := SessionIDPrefix + uuid.NewString()
	cmd := p.Command(shell, shellArgs...)
	cmd.Dir = homeDir()
	cmd.Env = shellEnv()

	if err := cmd.Start(); err != nil {
		_ = p.Close()
		return "", fmt.Errorf("start %s: %w", shell, err)
	}

	s := &localSession{id: sessionID, pty: p, cmd: cmd}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()

	// Pump PTY output to the event sink until EOF, then report the exit.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := p.Read(buf)
			if n > 0 && events != nil {
				events.OnData(sessionID, append([]byte(nil), buf[:n]...))
			}
			if rerr != nil {
				break
			}
		}
		waitErr := cmd.Wait()
		if events != nil {
			events.OnExit(sessionID, waitErr)
		}
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		// Also closes for the natural-exit path (user typed `exit`); a
		// no-op when Manager.Close already tore the PTY down.
		s.closePty()
	}()

	return sessionID, nil
}

// WriteStdin forwards user input to the local shell.
func (m *Manager) WriteStdin(sessionID string, data []byte) error {
	s, ok := m.session(sessionID)
	if !ok {
		return ErrSessionNotFound
	}
	_, err := s.pty.Write(data)
	return err
}

// Resize updates the local PTY dimensions (cols, rows).
func (m *Manager) Resize(sessionID string, cols, rows int) error {
	s, ok := m.session(sessionID)
	if !ok {
		return ErrSessionNotFound
	}
	return s.pty.Resize(cols, rows)
}

// Close terminates a local session. Safe to call multiple times. The session
// is dropped from the map FIRST so any in-flight WriteStdin/Resize fails
// fast instead of touching a freed ConPTY handle.
func (m *Manager) Close(sessionID string) error {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if !ok {
		return ErrSessionNotFound
	}
	s.closePty()
	return nil
}

// CloseAll tears down every local session (app shutdown).
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

func (m *Manager) session(id string) (*localSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// psUTF8Bootstrap runs at PowerShell startup: UTF-8 for the console (input +
// output code page 65001, so external tools and `type` decode correctly) and
// UTF-8 as the default encoding for file cmdlets (Get-Content/Set-Content/
// Out-File). Windows PowerShell 5.x otherwise defaults to the legacy ANSI
// code page (GBK on zh-CN), which garbles UTF-8 content. pwsh 7 already
// defaults to UTF-8 — the assignments are harmless there.
const psUTF8Bootstrap = `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;` +
	`[Console]::InputEncoding=[System.Text.Encoding]::UTF8;` +
	`$PSDefaultParameterValues['*:Encoding']='utf8'`

// detectShell picks the interactive shell for this OS:
// Windows prefers PowerShell (pwsh → powershell → COMSPEC/cmd); Unix uses the
// user's login shell with zsh/bash fallbacks.
func detectShell() (string, []string, error) {
	switch runtime.GOOS {
	case "windows":
		for _, cand := range []string{"pwsh.exe", "powershell.exe"} {
			if path, err := exec.LookPath(cand); err == nil {
				return path, []string{"-NoLogo", "-NoExit", "-Command", psUTF8Bootstrap}, nil
			}
		}
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec, []string{"/k", "chcp 65001>nul"}, nil
		}
		if path, err := exec.LookPath("cmd.exe"); err == nil {
			return path, []string{"/k", "chcp 65001>nul"}, nil
		}
		return "", nil, errors.New("no local shell found (tried pwsh, powershell, cmd)")
	default:
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell, []string{"-l"}, nil // login shell: pick up the user's profile
		}
		for _, cand := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
			if _, err := os.Stat(cand); err == nil {
				return cand, []string{"-l"}, nil
			}
		}
		return "", nil, errors.New("no local shell found (tried $SHELL, zsh, bash, sh)")
	}
}

// shellEnv returns the child environment; Unix shells get a color TERM.
func shellEnv() []string {
	env := os.Environ()
	if runtime.GOOS != "windows" {
		env = append(env, "TERM=xterm-256color")
	}
	return env
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

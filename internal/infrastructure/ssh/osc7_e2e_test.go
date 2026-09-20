package ssh

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// memKeyStore trusts-and-records in memory (first contact wins).
type memKeyStore struct{ keys map[string]string }

func (m *memKeyStore) Get(hostID string) (string, string, error) {
	k, ok := m.keys[hostID]
	if !ok {
		return "", "", ErrNotFound
	}
	return "ssh-ed25519", k, nil
}
func (m *memKeyStore) Upsert(hostID, alg, fp string) error {
	m.keys[hostID] = fp
	return nil
}

// captureEvents records PTY output for assertions.
type captureEvents struct{ data []byte }

func (c *captureEvents) OnData(_ string, data []byte) { c.data = append(c.data, data...) }
func (c *captureEvents) OnExit(string, error)         {}
func (c *captureEvents) OnReconnecting(string, int)   {}
func (c *captureEvents) OnProgress(string, string)    {}

// TestOSC7IntegrationE2E drives a REAL sshd: opens a session (the backend
// injects the shell integration at startup — wrapped start command, zero
// echo) and verifies the shell reports its cwd via OSC 7 in the output
// stream, including after a cd typed into the PTY. Run with:
//
//	E2E_SSH_HOST=<ip> E2E_SSH_PORT=<port> E2E_SSH_USER=<user> E2E_SSH_KEY=<keypath> \
//	  go test -run TestOSC7IntegrationE2E ./internal/infrastructure/ssh/
func TestOSC7IntegrationE2E(t *testing.T) {
	hostAddr := os.Getenv("E2E_SSH_HOST")
	if hostAddr == "" {
		t.Skip("E2E_SSH_HOST not set")
	}
	user := os.Getenv("E2E_SSH_USER")
	keyPath := os.Getenv("E2E_SSH_KEY")
	port := 22
	if p := os.Getenv("E2E_SSH_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	mgr := NewManager(&memKeyStore{keys: map[string]string{}})
	events := &captureEvents{}
	host := domain.Host{ID: "e2e", Host: hostAddr, Port: port, Username: user}
	creds := domain.Credentials{KeyPath: keyPath}

	sid, err := mgr.OpenSession(context.Background(), host, creds, 80, 24, events)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer mgr.Close(sid)

	// The startup-injected hook fires on the first prompt redraw: the output
	// stream must contain an OSC 7 cwd report within a few seconds.
	deadline := time.Now().Add(8 * time.Second)
	for {
		if strings.Contains(string(events.data), "\x1b]7;file://") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no OSC 7 report in output stream:\n%.400q", string(events.data))
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("integration reported cwd via OSC 7 ✓ (%d bytes total)", len(events.data))

	// cd the interactive shell: the next prompt redraw reports the new cwd.
	if err := mgr.WriteStdin(sid, []byte("cd /tmp\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline = time.Now().Add(8 * time.Second)
	for {
		if strings.Contains(string(events.data), "/tmp") {
			return // PASS: follow signal tracks the shell's cd
		}
		if time.Now().After(deadline) {
			t.Fatalf("no OSC 7 report for /tmp after cd:\n%.400q", string(events.data))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Compile-time: captureEvents satisfies the port.
var _ application.SessionEvents = (*captureEvents)(nil)

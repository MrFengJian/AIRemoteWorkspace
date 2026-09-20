package ssh

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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

// TestFollowCwdE2E drives a REAL sshd (WSL): opens a PTY session, cds the
// interactive shell via PTY input, and verifies FollowCwd reports the shell's
// cwd without ever writing to the session itself. Run with:
//
//	E2E_SSH_HOST=<ip> E2E_SSH_USER=<user> E2E_SSH_KEY=<keypath> \
//	  go test -run TestFollowCwdE2E ./internal/infrastructure/ssh/
func TestFollowCwdE2E(t *testing.T) {
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
	host := domain.Host{ID: "e2e", Host: hostAddr, Port: port, Username: user}
	creds := domain.Credentials{KeyPath: keyPath}

	sid, err := mgr.OpenSession(context.Background(), host, creds, 80, 24, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer mgr.Close(sid)
	time.Sleep(1500 * time.Millisecond) // let the shell finish motd/first prompt

	before, err := mgr.FollowCwd(context.Background(), sid)
	if err != nil || !strings.HasPrefix(before, "/") {
		t.Fatalf("initial FollowCwd = %q, %v (want an absolute path)", before, err)
	}
	t.Logf("initial cwd: %q", before)

	// cd the interactive shell via PTY input; give the prompt a moment.
	if err := mgr.WriteStdin(sid, []byte("cd /tmp\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	after, err := mgr.FollowCwd(context.Background(), sid)
	if err != nil {
		t.Fatalf("follow: %v", err)
	}
	if !strings.HasPrefix(after, "/tmp") {
		t.Fatalf("FollowCwd after cd = %q, want /tmp…", after)
	}
}

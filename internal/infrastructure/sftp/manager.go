// Package sftp provides remote file operations over the SSH File Transfer
// Protocol. It caches one SFTP client per host (reused across directory
// listings) and closes idle connections automatically.
//
// The package depends on infrastructure/ssh for dial + host-key verification,
// and on github.com/pkg/sftp for the SFTP protocol layer (no CGO).
package sftp

import (
	"fmt"
	"sync"
	"time"

	sf "github.com/pkg/sftp"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// idleTimeout is how long a cached SFTP connection is kept open without use
// before it is closed and removed from the cache.
const idleTimeout = 10 * time.Minute

// cachedConn bundles an SFTP client with the SSH client it rides on (both must
// be closed together) plus an idle timer.
type cachedConn struct {
	sftp   *sf.Client
	client *ssh.Client
	timer  *time.Timer
}

// stopTimer safely stops the idle timer; returns true if it actually fired
// (meaning the connection should be considered expired).
func (c *cachedConn) stopTimer() {
	if c.timer != nil {
		c.timer.Stop()
	}
}

// Manager caches SFTP clients per host. It is safe for concurrent use.
type Manager struct {
	dialer Dialer

	mu    sync.Mutex
	conns map[string]*cachedConn
}

// Dialer abstracts ssh.Dial so this package stays testable without real SSH.
// Implementations dial, authenticate, and return a live *ssh.Client.
type Dialer func(host domain.Host, creds domain.Credentials) (*ssh.Client, error)

// NewManager builds a Manager that dials via dialer.
func NewManager(dialer Dialer) *Manager {
	return &Manager{dialer: dialer, conns: make(map[string]*cachedConn)}
}

// client returns a cached SFTP client for the host, or dials a fresh one.
// Each successful get resets the idle timer.
func (m *Manager) client(host domain.Host, creds domain.Credentials) (*sf.Client, error) {
	m.mu.Lock()
	if c, ok := m.conns[host.ID]; ok {
		c.stopTimer()
		m.mu.Unlock()
		// Verify the connection is still alive with a cheap op.
		if _, err := c.sftp.Getwd(); err == nil {
			m.armIdleTimer(host.ID)
			return c.sftp, nil
		}
		// Stale connection — drop and redial.
		m.mu.Lock()
		m.dropLocked(host.ID)
		m.mu.Unlock()
	} else {
		m.mu.Unlock()
	}

	sshClient, err := m.dialer(host, creds)
	if err != nil {
		return nil, fmt.Errorf("sftp dial %s: %w", host.Host, err)
	}
	// sftp.NewClient needs the raw *golang.org/x/crypto/ssh.Client that our
	// ssh.Client wraps.
	sc, err := sf.NewClient(sshClient.SSHClient())
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("sftp init %s: %w", host.Host, err)
	}

	c := &cachedConn{sftp: sc, client: sshClient}
	m.mu.Lock()
	m.conns[host.ID] = c
	m.armIdleLocked(host.ID)
	m.mu.Unlock()
	return sc, nil
}

// armIdleLocked schedules idle closure; caller holds m.mu.
func (m *Manager) armIdleLocked(hostID string) {
	c := m.conns[hostID]
	if c == nil {
		return
	}
	c.timer = time.AfterFunc(idleTimeout, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.dropLocked(hostID)
	})
}

// armIdleTimer is the unlocked wrapper for external callers that just got a hit.
func (m *Manager) armIdleTimer(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.armIdleLocked(hostID)
}

// dropLocked closes and removes a cached connection; caller holds m.mu.
func (m *Manager) dropLocked(hostID string) {
	c, ok := m.conns[hostID]
	if !ok {
		return
	}
	c.stopTimer()
	_ = c.sftp.Close()
	_ = c.client.Close()
	delete(m.conns, hostID)
}

// dropOnError removes a cached connection after a failed operation (the
// connection may be dead). Best-effort.
func (m *Manager) dropOnError(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropLocked(hostID)
}

// Close closes every cached connection (used on app shutdown).
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.conns {
		m.dropLocked(id)
	}
	return nil
}

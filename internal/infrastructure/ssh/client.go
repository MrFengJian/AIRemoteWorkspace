// Package ssh wraps golang.org/x/crypto/ssh for the workspace's remote
// connection needs: dial with multiple auth methods, host-key verification,
// keepalive, and interactive PTY sessions.
//
// It is deliberately free of Wails/SQLite concerns — those layers inject
// dependencies (HostKeyStore, event emitters) via interfaces.
package ssh

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/ai-remote/workspace/internal/domain"
)

// ConnectOptions describes a single dial attempt.
type ConnectOptions struct {
	HostID    string // logical id for host-key lookups
	Host      string
	Port      int
	Username  string
	Timeout   time.Duration // dial timeout; 0 = 15s
	OnNewKey  func(alg, fp string)
	// OnProgress receives "connect" (TCP dial) and "handshake" (key
	// exchange + host-key verification + authentication) as they start —
	// fed to the UI's connection progress indicator. May be nil.
	OnProgress func(stage string)
	// Route (optional) is the pre-SSH path: jump chain and/or HTTP/SOCKS5
	// proxy, resolved by application.ProxyService. Nil = direct TCP.
	Route *domain.SshRoute
}

// Auth carries resolved credentials for a connection. Exactly one source is
// populated depending on the host's AuthType.
type Auth struct {
	Password      string
	KeyPath       string
	KeyPassphrase string
	UseAgent      bool
}

// Client is a live SSH client connection with a background keepalive.
type Client struct {
	ssh  *ssh.Client
	conn ssh.Conn // for keepalive global requests

	stopKeepalive chan struct{}
	closeOnce     sync.Once
}

// Dial connects to the host, authenticates, and starts a keepalive loop.
// The HostKeyStore governs known_hosts verification. When ConnectOptions
// carries a Route (jump chain / proxy), the connection is tunneled through
// it; each hop is host-key verified under its own host id.
func Dial(opts ConnectOptions, auth Auth, store HostKeyStore) (*Client, error) {
	if opts.Route != nil && (len(opts.Route.Jumps) > 0 || opts.Route.Proxy != nil) {
		return dialViaRoute(opts, auth, store)
	}
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}

	var onProgress func(stage string)
	if opts.OnProgress != nil {
		onProgress = opts.OnProgress
	}
	addr := formatAddr(opts.Host, opts.Port)
	return handshakeSSH(func(a string) (net.Conn, error) {
		if onProgress != nil {
			onProgress("connect")
		}
		return net.DialTimeout("tcp", a, opts.Timeout)
	}, addr, opts.HostID, opts.Username, auth, store, opts.Timeout, onProgress, opts.OnNewKey)
}

// NewSession opens a new SSH session on this client.
func (c *Client) NewSession() (*ssh.Session, error) {
	return c.ssh.NewSession()
}

// SSHClient returns the underlying *ssh.Client, for subsystems like SFTP that
// need to open channels on the same connection (pkg/sftp.NewClient).
func (c *Client) SSHClient() *ssh.Client {
	return c.ssh
}

// Close stops keepalive and closes the SSH client.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.stopKeepalive)
		err = c.ssh.Close()
	})
	return err
}

// aliveProbeTimeout bounds the connection-liveness probe used to tell a
// dropped link apart from a normally exited shell.
const aliveProbeTimeout = 3 * time.Second

// Alive reports whether the SSH connection is still usable: a keepalive
// global request must complete within the probe timeout. After a network
// drop (or Close) the request fails or hangs, so the session manager can
// distinguish "shell exited, link fine" (no auto-reconnect) from "link
// dead" (auto-reconnect).
func (c *Client) Alive() bool {
	errCh := make(chan error, 1)
	go func() {
		_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err == nil
	case <-time.After(aliveProbeTimeout):
		return false // link is wedged — treat as dead
	}
}

// startKeepalive sends keepalive@openssh.com every interval; on failure it
// closes the client (the next op will surface a closed-connection error,
// which the connection manager treats as "needs reconnect").
func (c *Client) startKeepalive(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-c.stopKeepalive:
				return
			case <-t.C:
				_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					_ = c.ssh.Close()
					return
				}
			}
		}
	}()
}

// buildAuth resolves an Auth into ssh.AuthMethod(s).
func buildAuth(auth Auth) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if auth.UseAgent {
		signers, err := agentSigners()
		if err == nil && len(signers) > 0 {
			methods = append(methods, ssh.PublicKeys(signers...))
		}
		// Agent missing is non-fatal if other methods are provided.
	}

	if auth.KeyPath != "" {
		signer, err := loadKey(auth.KeyPath, auth.KeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("load key %s: %w", auth.KeyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if auth.Password != "" {
		methods = append(methods, ssh.Password(auth.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no authentication method available (need password, key, or agent)")
	}
	return methods, nil
}

// loadKey reads a private key file, trying plain then passphrase-protected.
func loadKey(path, passphrase string) (ssh.Signer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(raw, []byte(passphrase))
	}
	// Try unencrypted first; if it's encrypted, return the specific error so
	// callers can prompt for a passphrase.
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil && isEncryptedKeyError(err) && passphrase == "" {
		return nil, fmt.Errorf("key is encrypted; passphrase required")
	}
	return signer, err
}

// isEncryptedKeyError detects "ssh: this private key is password protected".
func isEncryptedKeyError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "password protected") ||
		strings.Contains(err.Error(), "encrypted"))
}

// agentSigners connects to SSH_AUTH_SOCK and returns available signers.
// Returns an error if no agent is available (e.g. on Windows without one).
func agentSigners() ([]ssh.Signer, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		// ssh-agent also listens on a named pipe on Windows via Pageant-like
		// helpers; net.Dial("unix",...) won't find those. For Phase 2 we rely
		// on SSH_AUTH_SOCK (WSL/Cygwin/git-bash agents expose it).
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("connect ssh-agent: %w", err)
	}
	ag := agent.NewClient(conn)
	signers, err := ag.Signers()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return signers, nil
}

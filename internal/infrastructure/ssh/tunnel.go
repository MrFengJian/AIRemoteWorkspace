package ssh

// SSH tunnels (AGENT.md §16): one tunnel per host, managed by TunnelManager.
//
//   - A tunnel owns a DEDICATED SSH connection — closing terminal tabs never
//     breaks it, and multiple sessions on one host share the single tunnel
//     (Ensure is a no-op when the config already runs).
//   - A supervisor goroutine per tunnel owns the local listener and redials
//     the SSH connection with exponential backoff when it drops (auto-reconnect).
//   - Supported forms (domain.TunnelType): local port forward (`ssh -L`) and
//     dynamic SOCKS5 (`ssh -D`, CONNECT-only).

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// tunnel retry backoff: doubled per consecutive failure, capped.
const (
	tunnelBackoffBase = 2 * time.Second
	tunnelBackoffMax  = 30 * time.Second
	// How long Ensure waits for a replaced tunnel to release its port.
	tunnelStopGrace = 5 * time.Second
)

// channelDialer abstracts the far side of the tunnel: opening connections to
// targets reachable from the SSH server, a death signal so the supervisor
// notices a dropped connection, and (for the remote form) the server-side
// listener — without touching real SSH types.
type channelDialer interface {
	// DialTarget connects to addr ("host:port") from the server's side.
	DialTarget(addr string) (net.Conn, error)
	// ListenRemote asks the server to listen on addr (`ssh -R`); Accepts
	// yield the server-side connections. The listener's lifetime is bounded
	// by the underlying SSH connection.
	ListenRemote(addr string) (net.Listener, error)
	// Done closes when the underlying connection is gone.
	Done() <-chan struct{}
	Close() error
}

// clientDialer adapts the real SSH client.
type clientDialer struct{ c *Client }

func (d clientDialer) DialTarget(addr string) (net.Conn, error) {
	return d.c.SSHClient().Dial("tcp", addr)
}

func (d clientDialer) ListenRemote(addr string) (net.Listener, error) {
	return d.c.SSHClient().Listen("tcp", addr)
}

func (d clientDialer) Done() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_ = d.c.SSHClient().Wait() // returns when the connection closes
		close(done)
	}()
	return done
}

func (d clientDialer) Close() error { return d.c.Close() }

// TunnelManager keeps one tunnel per host RULE (a host may configure several)
// and reports state changes through the emitter wired via SetEmitter
// (nil-safe everywhere). Supervisors are keyed by hostID + rule key; Ensure
// reconciles the running set with the host's saved rules.
type TunnelManager struct {
	keyStore HostKeyStore
	// backoffBase overrides the retry backoff (0 = default 2s). Test hook.
	backoffBase time.Duration

	// injectable for tests; nil = real SSH dial
	dialFn func(host domain.Host, creds domain.Credentials) (channelDialer, error)

	mu      sync.Mutex
	emit    func(domain.TunnelStatus)
	tunnels map[string]*tunnelSupervisor // supKey(hostID, ruleKey) → supervisor
}

// supKey builds the map key for one host rule.
func supKey(hostID, ruleKey string) string { return hostID + "\x00" + ruleKey }

// NewTunnelManager builds a manager using real SSH dials with keyStore-backed
// host-key verification.
func NewTunnelManager(keyStore HostKeyStore) *TunnelManager {
	return &TunnelManager{keyStore: keyStore, tunnels: make(map[string]*tunnelSupervisor)}
}

func (m *TunnelManager) dialHost(host domain.Host, creds domain.Credentials) (channelDialer, error) {
	if m.dialFn != nil {
		return m.dialFn(host, creds)
	}
	client, err := Dial(ConnectOptions{
		HostID:   host.ID,
		Host:     host.Host,
		Port:     host.Port,
		Username: host.Username,
	}, Auth{
		Password:      creds.Password,
		KeyPath:       creds.KeyPath,
		KeyPassphrase: creds.KeyPassphrase,
		UseAgent:      creds.UseAgent,
	}, m.keyStore)
	if err != nil {
		return nil, err
	}
	return clientDialer{c: client}, nil
}

// Ensure reconciles the host's tunnels with its saved rule list — the single
// entry point used by the terminal auto-start, the settings save path, and
// the panel's manual start.
//
//   - rule already running with the SAME config → untouched (opening another
//     session on the host never duplicates tunnels);
//   - rule changed → the old supervisor is stopped (waiting for it to release
//     its port) and a replacement starts;
//   - rule removed / invalid → its tunnel stops;
//   - new rule → tunnel starts.
//
// A tunnel the user manually stopped is indistinguishable from a never-started
// one here: if the rule is still in the saved list, Ensure brings it back.
func (m *TunnelManager) Ensure(host domain.Host, creds domain.Credentials) {
	type rule struct {
		key string
		cfg domain.TunnelConfig
	}
	var wanted []rule
	for _, c := range host.Tunnels {
		if c.Valid() {
			wanted = append(wanted, rule{c.Key(), c})
		}
	}

	prefix := host.ID + "\x00"
	m.mu.Lock()
	var (
		keep     []*tunnelSupervisor // unchanged rules → refresh host name only
		replaced []*tunnelSupervisor // changed/removed rules → stop + wait
		starts   []rule              // new rules → start below
	)
	for k, sup := range m.tunnels {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		kept := false
		for _, w := range wanted {
			if k == supKey(host.ID, w.key) {
				kept = true
				keep = append(keep, sup)
				break
			}
		}
		if !kept {
			// Orphan the old supervisor so its shutdown report can't
			// overwrite the replacement's events, then stop it.
			sup.orphan()
			sup.stop()
			delete(m.tunnels, k)
			replaced = append(replaced, sup)
		}
	}
	for _, w := range wanted {
		if _, ok := m.tunnels[supKey(host.ID, w.key)]; !ok {
			starts = append(starts, w)
		}
	}
	m.mu.Unlock()

	// Refresh host names outside the lock (kept supervisors report changes).
	for _, sup := range keep {
		sup.setHostName(host.Name)
	}
	// Wait for replaced tunnels to release their ports — a replacement often
	// listens on the same port with a new target.
	for _, sup := range replaced {
		select {
		case <-sup.done:
		case <-time.After(tunnelStopGrace):
		}
	}
	// Start the missing rules.
	for _, w := range starts {
		sup := newTunnelSupervisor(host, w.cfg, creds, m.dialHost, m.report)
		if m.backoffBase > 0 {
			sup.retryBackoffBase = m.backoffBase
		}
		m.mu.Lock()
		if _, exists := m.tunnels[supKey(host.ID, w.key)]; exists {
			m.mu.Unlock()
			continue // a concurrent Ensure installed this rule meanwhile
		}
		m.tunnels[supKey(host.ID, w.key)] = sup
		m.mu.Unlock()
		go sup.run()
	}
}

// Stop halts every tunnel of the host and forgets the entries (the panel
// renders configured-but-stopped rules from the host record itself).
func (m *TunnelManager) Stop(hostID string) {
	m.mu.Lock()
	var sups []*tunnelSupervisor
	for k, sup := range m.tunnels {
		if strings.HasPrefix(k, hostID+"\x00") {
			sups = append(sups, sup)
			delete(m.tunnels, k)
		}
	}
	m.mu.Unlock()
	for _, sup := range sups {
		sup.stop()
	}
}

// Remove stops the host's tunnels and forgets the entries (host deleted).
func (m *TunnelManager) Remove(hostID string) {
	m.Stop(hostID)
}

// ListenPortsOf returns the listen ports of every supervisor known for the
// host — running or not. Used to exempt the host's own tunnels from the
// save-time port-conflict check (they are reconciled on save anyway).
func (m *TunnelManager) ListenPortsOf(hostID string) []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ports []int
	prefix := hostID + "\x00"
	for k, sup := range m.tunnels {
		if strings.HasPrefix(k, prefix) {
			ports = append(ports, sup.config.ListenPort)
		}
	}
	return ports
}

// StopAll tears down every tunnel (app shutdown).
func (m *TunnelManager) StopAll() {
	m.mu.Lock()
	sups := make([]*tunnelSupervisor, 0, len(m.tunnels))
	for _, s := range m.tunnels {
		sups = append(sups, s)
	}
	m.mu.Unlock()
	for _, s := range sups {
		s.stop()
	}
}

// Statuses returns the current status of every known tunnel.
func (m *TunnelManager) Statuses() []domain.TunnelStatus {
	m.mu.Lock()
	sups := make([]*tunnelSupervisor, 0, len(m.tunnels))
	for _, s := range m.tunnels {
		sups = append(sups, s)
	}
	m.mu.Unlock()

	out := make([]domain.TunnelStatus, 0, len(sups))
	for _, s := range sups {
		out = append(out, s.snapshot())
	}
	return out
}

// report pushes a supervisor's status to the wired emitter.
func (m *TunnelManager) report(s domain.TunnelStatus) {
	m.mu.Lock()
	emit := m.emit
	m.mu.Unlock()
	if emit != nil {
		emit(s)
	}
}

// SetEmitter wires the status callback after construction (the Wails
// TunnelService, which itself needs the manager at creation).
func (m *TunnelManager) SetEmitter(fn func(domain.TunnelStatus)) {
	m.mu.Lock()
	m.emit = fn
	m.mu.Unlock()
}

// tunnelSupervisor runs one tunnel: listener + redial loop + connection
// serving. All state changes flow through setStatus → manager.report.
type tunnelSupervisor struct {
	hostID     string
	sshHost    string
	sshPort    int
	sshUser    string
	config     domain.TunnelConfig
	creds    domain.Credentials
	dial     func(domain.Host, domain.Credentials) (channelDialer, error)
	report   func(domain.TunnelStatus)

	// retryBackoffBase is a field (not const) so tests can shorten it.
	retryBackoffBase time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
	done     chan struct{} // closed when run() has fully exited

	dmu    sync.Mutex    // guards dialer
	dialer channelDialer // live far side; nil while the link is down

	mu       sync.Mutex
	orphaned bool // replaced by Ensure — must not report anymore
	status   domain.TunnelStatus
}

func newTunnelSupervisor(
	host domain.Host,
	cfg domain.TunnelConfig,
	creds domain.Credentials,
	dial func(domain.Host, domain.Credentials) (channelDialer, error),
	report func(domain.TunnelStatus),
) *tunnelSupervisor {
	return &tunnelSupervisor{
		hostID:           host.ID,
		sshHost:          host.Host,
		sshPort:          host.Port,
		sshUser:          host.Username,
		config:           cfg,
		creds:            creds,
		dial:             dial,
		report:           report,
		retryBackoffBase: tunnelBackoffBase,
		stopCh:           make(chan struct{}),
		done:             make(chan struct{}),
		status: domain.TunnelStatus{
			HostID:   host.ID,
			HostName: host.Name,
			Key:      cfg.Key(),
			Config:   cfg,
			State:    domain.TunnelStarting,
		},
	}
}

func (s *tunnelSupervisor) stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// orphan marks the supervisor as replaced: it must stay silent from now on so
// its shutdown cannot overwrite the replacement's status events.
func (s *tunnelSupervisor) orphan() {
	s.mu.Lock()
	s.orphaned = true
	s.mu.Unlock()
}

func (s *tunnelSupervisor) setStatus(state domain.TunnelState, lastErr string, retries int) {
	s.mu.Lock()
	if s.orphaned {
		s.mu.Unlock()
		return
	}
	s.status.State = state
	s.status.LastError = lastErr
	s.status.Retries = retries
	snap := s.status
	s.mu.Unlock()
	s.report(snap)
}

func (s *tunnelSupervisor) setHostName(name string) {
	s.mu.Lock()
	if s.orphaned || s.status.HostName == name {
		s.mu.Unlock()
		return
	}
	s.status.HostName = name
	snap := s.status
	s.mu.Unlock()
	s.report(snap)
}

func (s *tunnelSupervisor) snapshot() domain.TunnelStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// run is the supervisor loop: claim the listen port (locally for the local/
// dynamic forms, on the server per connection for the remote form), then
// dial → hold → redial with backoff until stopped. A fatal local listen
// error (port taken) ends the supervisor in the error state.
func (s *tunnelSupervisor) run() {
	defer close(s.done)
	defer s.setStatus(domain.TunnelStopped, "", 0)

	// Local and dynamic listeners live on this machine and survive
	// reconnects; the remote form's listener lives on the SERVER and must be
	// re-requested after every redial (see the loop below).
	var ln net.Listener
	if s.config.Type != domain.TunnelRemote {
		var err error
		ln, err = net.Listen("tcp", net.JoinHostPort(s.config.ListenHost(), strconv.Itoa(s.config.ListenPort)))
		if err != nil {
			s.setStatus(domain.TunnelError, fmt.Sprintf("listen %s:%d: %v", s.config.ListenHost(), s.config.ListenPort, err), 0)
			return
		}
		defer ln.Close()

		// One accept loop for the supervisor's whole life. Connections that
		// arrive while the SSH link is down wait briefly for the reconnect in
		// handleConn instead of being dropped.
		go s.acceptLoop(ln)
	}

	backoff := s.retryBackoffBase
	retries := 0
	first := true
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		state := domain.TunnelReconnecting
		if first {
			state = domain.TunnelStarting
			first = false
		}
		cur := s.snapshot()
		s.setStatus(state, cur.LastError, retries)

		// The synthetic host carries the dial identity — Username MUST be
		// present, an empty user makes every auth method fail server-side.
		dialer, err := s.dial(domain.Host{
			ID:       s.hostID,
			Host:     s.sshHost,
			Port:     s.sshPort,
			Username: s.sshUser,
		}, s.creds)
		var rln net.Listener // server-side listener (remote form)
		if err == nil && s.config.Type == domain.TunnelRemote {
			rln, err = dialer.ListenRemote(net.JoinHostPort(s.config.ListenHost(), strconv.Itoa(s.config.ListenPort)))
			if err != nil {
				err = fmt.Errorf("remote listen on %s:%d: %w", s.config.ListenHost(), s.config.ListenPort, err)
			}
		}
		if err != nil {
			if dialer != nil {
				_ = dialer.Close()
			}
			retries++
			s.setStatus(domain.TunnelReconnecting, err.Error(), retries)
			if !s.sleep(backoff) {
				return
			}
			backoff = min(backoff*2, tunnelBackoffMax)
			continue
		}
		backoff = s.retryBackoffBase
		retries = 0

		if rln != nil {
			// Serve connections arriving on the server for as long as this
			// connection lives — the forwarded listener dies with it.
			go s.acceptLoop(rln)
		}

		s.setDialer(dialer)
		s.setStatus(domain.TunnelConnected, "", 0)

		// Park until the connection dies or the supervisor is stopped.
		select {
		case <-dialer.Done():
		case <-s.stopCh:
			if rln != nil {
				_ = rln.Close() // ends this round's accept loop
			}
			s.setDialer(nil)
			_ = dialer.Close()
			return
		}
		if rln != nil {
			_ = rln.Close()
		}
		s.setDialer(nil)
		_ = dialer.Close()

		// connection lost — cool down briefly so a link that dies instantly
		// cannot spin the loop without ever backing off
		if !s.sleep(s.retryBackoffBase) {
			return
		}
	}
}

// sleep waits for d or until stopped; reports whether to keep running.
func (s *tunnelSupervisor) sleep(d time.Duration) bool {
	select {
	case <-s.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

// acceptLoop accepts local connections until the listener closes (which only
// happens when the supervisor is winding down).
func (s *tunnelSupervisor) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

// dialerSnapshot returns the live channel dialer, or nil while the SSH link
// is down.
func (s *tunnelSupervisor) dialerSnapshot() channelDialer {
	s.dmu.Lock()
	defer s.dmu.Unlock()
	return s.dialer
}

func (s *tunnelSupervisor) setDialer(d channelDialer) {
	s.dmu.Lock()
	s.dialer = d
	s.dmu.Unlock()
}

// tunnelConnectWait caps how long a freshly accepted connection waits for the
// SSH link to come back during a reconnect gap before being dropped (the
// client retries; waiting forever would hang it).
const tunnelConnectWait = 5 * time.Second

// handleConn tunnels one accepted connection by form:
//   - remote: the connection originated on the SERVER — carry it to a target
//     reachable from THIS machine (plain local dial, no tunnel dialer);
//   - local:  forward to the configured target resolved from the server;
//   - dynamic: first complete a SOCKS5 CONNECT handshake with the local
//     client to learn the target, then dial it from the server.
func (s *tunnelSupervisor) handleConn(conn net.Conn) {
	defer conn.Close()

	if s.config.Type == domain.TunnelRemote {
		remote, err := net.Dial("tcp", net.JoinHostPort(s.config.TargetHost, strconv.Itoa(s.config.TargetPort)))
		if err != nil {
			return
		}
		defer remote.Close()
		pipe(conn, remote)
		return
	}

	var target string
	if s.config.Type == domain.TunnelDynamic {
		var err error
		if target, err = socks5Handshake(conn); err != nil {
			return // the handshake already told the client
		}
	} else {
		target = net.JoinHostPort(s.config.TargetHost, strconv.Itoa(s.config.TargetPort))
	}

	// The link may be mid-reconnect; give it a bounded chance to return.
	var dialer channelDialer
	deadline := time.Now().Add(tunnelConnectWait)
	for {
		if dialer = s.dialerSnapshot(); dialer != nil {
			break
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	remote, err := dialer.DialTarget(target)
	if err != nil {
		return
	}
	defer remote.Close()
	pipe(conn, remote)
}

// pipe copies both directions until either side closes.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}

// socks5Handshake implements the server side of a no-auth SOCKS5 CONNECT
// (the only form `ssh -D` clients need). Returns the requested target.
func socks5Handshake(conn net.Conn) (string, error) {
	const (
		ver5        = 0x05
		cmdConnect  = 0x01
		replyOK     = 0x00
		replyCmdErr = 0x07 // command not supported
		replyAtypEr = 0x08 // address type not supported
	)

	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", err
	}
	if head[0] != ver5 {
		return "", fmt.Errorf("socks5: unsupported version %d", head[0])
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte{ver5, 0x00}); err != nil { // no-auth
		return "", err
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return "", err
	}
	if req[0] != ver5 {
		return "", fmt.Errorf("socks5: unsupported request version %d", req[0])
	}
	if req[1] != cmdConnect {
		_, _ = conn.Write([]byte{ver5, replyCmdErr, 0, 1, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("socks5: only CONNECT is supported")
	}

	var host string
	switch req[3] {
	case 0x01: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case 0x03: // domain name
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return "", err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = string(b)
	case 0x04: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	default:
		_, _ = conn.Write([]byte{ver5, replyAtypEr, 0, 1, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("socks5: unsupported address type %d", req[3])
	}

	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return "", err
	}

	// Success reply (BND.ADDR/PORT zeroed — CONNECT clients ignore it).
	if _, err := conn.Write([]byte{ver5, replyOK, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port)))), nil
}

package ssh

import (
	"runtime"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// fakeDialer is one simulated SSH connection: DialTarget connects to the
// test's "remote server" regardless of the requested target (from the fake's
// vantage point every target resolves to the echo service), so the full local
// path (local client → listener → tunnel → remote server) is exercised and
// only the SSH transport is faked.
type fakeDialer struct {
	remoteAddr string

	mu     sync.Mutex
	closed bool
	doneCh chan struct{}
}

func (f *fakeDialer) DialTarget(string) (net.Conn, error) {
	return net.Dial("tcp", f.remoteAddr)
}

// ListenRemote simulates the server-side `ssh -R` listener with a real local
// net.Listen — connections landing on it "originate from the server".
func (f *fakeDialer) ListenRemote(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func (f *fakeDialer) Done() <-chan struct{} { return f.doneCh }

func (f *fakeDialer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.doneCh)
	}
	return nil
}

var errFakeDial = &tempErr{}

type tempErr struct{}

func (*tempErr) Error() string { return "fake dial failure" }

// fakeFactory hands out a FRESH fakeDialer per simulated connection — like a
// real SSH dial, each connection gets its own Done signal. It records the
// dial identity so tests can assert the supervisor dials with full
// credentials (the empty-username regression).
type fakeFactory struct {
	remoteAddr string

	mu       sync.Mutex
	fail     int // remaining dial attempts to fail
	latest   *fakeDialer
	dials    int
	lastUser string
}

func (f *fakeFactory) dial(h domain.Host, _ domain.Credentials) (channelDialer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dials++
	f.lastUser = h.Username
	if f.fail > 0 {
		f.fail--
		return nil, errFakeDial
	}
	fd := &fakeDialer{remoteAddr: f.remoteAddr, doneCh: make(chan struct{})}
	f.latest = fd
	return fd, nil
}

// lastUsername returns the Username of the most recent dial.
func (f *fakeFactory) lastUsername() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUser
}

func (f *fakeFactory) dialCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dials
}

// kill drops the current connection (simulates network loss).
func (f *fakeFactory) kill() {
	f.mu.Lock()
	fd := f.latest
	f.mu.Unlock()
	if fd != nil {
		fd.Close()
	}
}

// newTestManager builds a TunnelManager whose dialFn returns fresh fake
// dialers wired to remoteAddr, with a short retry backoff for fast tests.
func newTestManager(t *testing.T, remoteAddr string) (*TunnelManager, *fakeFactory) {
	t.Helper()
	ff := &fakeFactory{remoteAddr: remoteAddr}
	m := NewTunnelManager(nil)
	m.dialFn = ff.dial
	m.backoffBase = 10 * time.Millisecond
	return m, ff
}

// echoServer starts a TCP server that echoes one line back — stands in for a
// service on the remote network.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, _ = io.Copy(c, c)
				_ = c.Close()
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// waitState polls until the host's tunnel reaches the wanted state.
func waitState(t *testing.T, m *TunnelManager, hostID string, want domain.TunnelState) domain.TunnelStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range m.Statuses() {
			if st.HostID == hostID && st.State == want {
				return st
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("tunnel %s never reached state %q; statuses: %+v", hostID, want, m.Statuses())
	return domain.TunnelStatus{}
}

// dialEcho sends one line through the local tunnel port and expects the echo.
func dialEcho(t *testing.T, port int, payload string) string {
	t.Helper()
	var conn net.Conn
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("dial local tunnel port %d: %v", port, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	return string(buf)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestTunnelLocalForwardEndToEnd(t *testing.T) {
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)
	host := domain.Host{
		ID:   "h1",
		Name: "web-1",
		Host: "203.0.113.10",
		Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23456, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}

	m.Ensure(host, domain.Credentials{Password: "pw"})
	waitState(t, m, "h1", domain.TunnelConnected)

	if got := dialEcho(t, 23456, "ping-through-tunnel"); got != "ping-through-tunnel" {
		t.Fatalf("echo mismatch: %q", got)
	}

	// Multiple sessions on the same host: Ensure again with the same config
	// must be a no-op (no duplicate tunnel).
	m.Ensure(host, domain.Credentials{Password: "pw"})
	if n := len(m.Statuses()); n != 1 {
		t.Fatalf("expected 1 tunnel after duplicate Ensure, got %d", n)
	}
	if got := dialEcho(t, 23456, "still-alive"); got != "still-alive" {
		t.Fatalf("echo after dedupe: %q", got)
	}

	m.StopAll()
	waitState(t, m, "h1", domain.TunnelStopped)
}

func TestTunnelDedupesByHost(t *testing.T) {
	remote := echoServer(t)
	m, fd := newTestManager(t, remote)
	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23457, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}

	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)
	dialsAfterFirst := fd.dialCount()

	// Simulate several tabs opening sessions on the same host.
	for i := 0; i < 3; i++ {
		m.Ensure(host, domain.Credentials{})
	}
	if got := fd.dialCount(); got != dialsAfterFirst {
		t.Fatalf("duplicate Ensure re-dialed: %d → %d", dialsAfterFirst, got)
	}
	if n := len(m.Statuses()); n != 1 {
		t.Fatalf("expected 1 tunnel, got %d", n)
	}
	m.StopAll()
}

func TestTunnelConfigChangeRestarts(t *testing.T) {
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23458, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)

	// Change the remote target — the tunnel must restart on the same port.
	host.Tunnels[0].TargetPort = 6379
	m.Ensure(host, domain.Credentials{})
	st := waitState(t, m, "h1", domain.TunnelConnected)
	if st.Config.TargetPort != 6379 {
		t.Fatalf("tunnel not restarted with new config: %+v", st.Config)
	}
	if got := dialEcho(t, 23458, "after-restart"); got != "after-restart" {
		t.Fatalf("echo after restart: %q", got)
	}
	m.StopAll()
}

func TestTunnelDisableStops(t *testing.T) {
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23459, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)

	host.Tunnels = nil
	m.Ensure(host, domain.Credentials{}) // disabled → rules stopped and forgotten
	if n := len(m.Statuses()); n != 0 {
		t.Fatalf("expected no tunnel entries after disable, got %d", n)
	}
}

func TestTunnelAutoReconnect(t *testing.T) {
	remote := echoServer(t)
	m, fd := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23460, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)

	// Kill the connection — the supervisor must redial and reconnect.
	fd.kill()
	st := waitState(t, m, "h1", domain.TunnelConnected)
	if got := dialEcho(t, 23460, "post-reconnect"); got != "post-reconnect" {
		t.Fatalf("echo after reconnect: %q", got)
	}
	_ = st
	m.StopAll()
}

func TestTunnelDialFailuresReported(t *testing.T) {
	remote := echoServer(t)
	m, ff := newTestManager(t, remote)

	// First dial attempts fail, then succeed — the tunnel must recover and
	// surface the failure text on the way.
	ff.mu.Lock()
	ff.fail = 2
	ff.mu.Unlock()

	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23461, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)
	if got := ff.dialCount(); got < 3 {
		t.Fatalf("expected >= 3 dial attempts (2 failures + success), got %d", got)
	}
	m.StopAll()
}

func TestTunnelSocks5(t *testing.T) {
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "proxy-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelDynamic,
			ListenPort: 23462,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)

	// Minimal SOCKS5 CONNECT client (no-auth, domain-less IPv4 target).
	targetHost, targetPortStr, _ := net.SplitHostPort(remote)
	targetPort := 0
	for _, c := range targetPortStr {
		targetPort = targetPort*10 + int(c-'0')
	}
	conn, err := net.Dial("tcp", "127.0.0.1:23462")
	if err != nil {
		t.Fatalf("socks dial: %v", err)
	}
	defer conn.Close()

	// Greeting: version 5, one method (no-auth 0x00).
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("greeting: %v", err)
	}
	rep := make([]byte, 2)
	if _, err := io.ReadFull(conn, rep); err != nil {
		t.Fatalf("greeting reply: %v", err)
	}
	if rep[0] != 0x05 || rep[1] != 0x00 {
		t.Fatalf("unexpected method reply: %v", rep)
	}

	// Request: CONNECT to IPv4 target.
	ip := net.ParseIP(targetHost).To4()
	req := []byte{0x05, 0x01, 0x00, 0x01, ip[0], ip[1], ip[2], ip[3], 0, 0}
	binary.BigEndian.PutUint16(req[8:10], uint16(targetPort))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	ans := make([]byte, 10)
	if _, err := io.ReadFull(conn, ans); err != nil {
		t.Fatalf("connect reply: %v", err)
	}
	if ans[1] != 0x00 {
		t.Fatalf("socks CONNECT failed: reply code %d", ans[1])
	}

	if _, err := conn.Write([]byte("via-socks")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len("via-socks"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "via-socks" {
		t.Fatalf("echo mismatch: %q", string(buf))
	}
	m.StopAll()
}

func TestTunnelRemoteForwardEndToEnd(t *testing.T) {
	localTarget := echoServer(t) // "service on this machine"
	m, ff := newTestManager(t, localTarget)

	host := domain.Host{
		ID: "h1", Name: "rev-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelRemote,
			ListenPort: 23464, BindHost: "127.0.0.1",
			TargetHost: "127.0.0.1", TargetPort: portOf(localTarget),
		}},
	}
	m.Ensure(host, domain.Credentials{})
	waitState(t, m, "h1", domain.TunnelConnected)

	// Something connects to the SERVER's listen port; the tunnel carries it
	// back to the local echo service.
	if got := dialEcho(t, 23464, "from-server"); got != "from-server" {
		t.Fatalf("echo: %q", got)
	}

	// After a drop the `ssh -R` listener must be re-requested on the fresh
	// connection — the port must serve again once reconnected.
	ff.kill()
	waitState(t, m, "h1", domain.TunnelConnected)
	if got := dialEcho(t, 23464, "after-reconnect"); got != "after-reconnect" {
		t.Fatalf("echo after reconnect: %q", got)
	}

	m.StopAll()
	waitState(t, m, "h1", domain.TunnelStopped)
}

// portOf extracts the numeric port from a "host:port" address.
func portOf(addr string) int {
	_, p, _ := net.SplitHostPort(addr)
	n := 0
	for _, c := range p {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestTunnelMultipleRulesPerHost(t *testing.T) {
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "multi-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{
			{Enabled: true, Type: domain.TunnelLocal, ListenPort: 23465, TargetHost: "127.0.0.1", TargetPort: 5432},
			{Enabled: true, Type: domain.TunnelDynamic, ListenPort: 23466},
		},
	}
	m.Ensure(host, domain.Credentials{})
	waitCondition(t, m, "h1", func(sts []domain.TunnelStatus) bool {
		return len(sts) == 2 && sts[0].State == domain.TunnelConnected && sts[1].State == domain.TunnelConnected
	}, "both rules connected")

	// Both tunnels serve concurrently.
	if got := dialEcho(t, 23465, "rule-one"); got != "rule-one" {
		t.Fatalf("rule one echo: %q", got)
	}
	conn, err := net.Dial("tcp", "127.0.0.1:23466")
	if err != nil {
		t.Fatalf("rule two (socks) dial: %v", err)
	}
	_ = conn.Close()

	// Removing one rule stops only that tunnel; the other is untouched.
	host.Tunnels = host.Tunnels[:1]
	m.Ensure(host, domain.Credentials{})
	waitCondition(t, m, "h1", func(sts []domain.TunnelStatus) bool {
		return len(sts) == 1 && sts[0].State == domain.TunnelConnected
	}, "one rule left, still connected")
	if got := dialEcho(t, 23465, "survivor"); got != "survivor" {
		t.Fatalf("surviving rule echo: %q", got)
	}

	m.StopAll()
}

// waitCondition polls until the host's tunnel statuses satisfy cond.
func waitCondition(t *testing.T, m *TunnelManager, hostID string, cond func([]domain.TunnelStatus) bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var forHost []domain.TunnelStatus
		for _, st := range m.Statuses() {
			if st.HostID == hostID {
				forHost = append(forHost, st)
			}
		}
		if cond(forHost) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met (%s); statuses: %+v", desc, m.Statuses())
}

func TestTunnelDialsWithHostUsername(t *testing.T) {
	// Regression: the supervisor rebuilt the dial host without the Username,
	// so tunnels authenticated as the empty user and every server rejected
	// them ("attempted methods [none password], no supported methods remain").
	remote := echoServer(t)
	m, ff := newTestManager(t, remote)

	host := domain.Host{
		ID: "h1", Name: "auth-1", Host: "203.0.113.10", Port: 22,
		Username: "deploy",
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23467, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{Password: "pw"})
	waitState(t, m, "h1", domain.TunnelConnected)

	if got := ff.lastUsername(); got != "deploy" {
		t.Fatalf("tunnel dialed with username %q, want %q", got, "deploy")
	}
	m.StopAll()
}

func TestTunnelPortTakenFatals(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows listener sockets default to SO_REUSEADDR, so a second bind
		// on the same port silently succeeds instead of erroring.
		t.Skip("port-conflict detection is not observable on Windows")
	}
	remote := echoServer(t)
	m, _ := newTestManager(t, remote)

	// Occupy the port first.
	ln, err := net.Listen("tcp", "127.0.0.1:23463")
	if err != nil {
		t.Skipf("cannot bind test port: %v", err)
	}
	defer ln.Close()

	host := domain.Host{
		ID: "h1", Name: "db-1", Host: "203.0.113.10", Port: 22,
		Tunnels: []domain.TunnelConfig{{
			Enabled: true, Type: domain.TunnelLocal,
			ListenPort: 23463, TargetHost: "127.0.0.1", TargetPort: 5432,
		}},
	}
	m.Ensure(host, domain.Credentials{})
	st := waitState(t, m, "h1", domain.TunnelError)
	if st.LastError == "" {
		t.Fatal("expected a lastError explaining the port conflict")
	}
}

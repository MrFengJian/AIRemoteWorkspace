package ssh

// Route dialing (Phase 8 跳板机/代理): reach a host through a jump-host
// chain and/or an HTTP CONNECT / SOCKS5 proxy BEFORE its SSH handshake.
//
// The plan (domain.SshRoute) is resolved by application.ProxyService and
// attached to ConnectOptions.Route. Execution order:
//
//	proxy (or direct TCP) → jumps[0] → jumps[1] → … → target
//
// Every hop is a full SSH connection: host-key verified under ITS host id
// against the same known_hosts store, authenticated with its own resolved
// credentials, keepalive running — and the next connection is tunneled
// through a direct-tcpip channel on it (OpenSSH ProxyJump semantics, no
// shell is opened on the box).

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ai-remote/workspace/internal/domain"
)

// dialViaRoute executes a non-direct route: proxy transport, then the jump
// chain, then the final SSH handshake to the target. Any failure closes
// everything opened so far.
func dialViaRoute(opts ConnectOptions, auth Auth, store HostKeyStore) (*Client, error) {
	route := opts.Route
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	// Innermost transport: plain TCP, or the configured proxy.
	dial := func(addr string) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, timeout)
	}
	if route.Proxy != nil {
		p := route.Proxy
		switch p.Kind {
		case "http":
			dial = httpConnectDialer(p, timeout)
		case "socks5":
			dial = socks5Dialer(p, timeout)
		default:
			return nil, fmt.Errorf("unknown proxy kind %q", p.Kind)
		}
	}

	var chain []*Client // jump clients opened so far; all closed on failure
	// fail tears down everything opened so far (in reverse order) — the
	// chain stays alive ONLY when the whole route succeeds.
	fail := func() {
		for i := len(chain) - 1; i >= 0; i-- {
			_ = chain[i].Close()
		}
	}

	for i, hop := range route.Jumps {
		c, err := handshakeSSH(dial, hop.Addr, hop.HostID, hop.Username, authOf(hop.Creds), store, timeout, nil, nil)
		if err != nil {
			fail()
			return nil, fmt.Errorf("jump %d via %s: %w", i+1, hop.Addr, err)
		}
		// All previous hops stay alive as the tunnel for this one.
		chain = append(chain, c)
		last := c
		dial = func(addr string) (net.Conn, error) {
			return last.SSHClient().Dial("tcp", addr)
		}
	}

	target := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	c, err := handshakeSSH(dial, target, opts.HostID, opts.Username, auth, store, timeout, opts.OnProgress, opts.OnNewKey)
	if err != nil {
		fail()
		return nil, err
	}
	return c, nil
}

// basicAuth encodes user:password for the HTTP Proxy-Authorization header.
func basicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// handshakeSSH dials addr through dial, verifies the host key under hostID,
// authenticates, and wraps the result in the keepalive-running Client.
// onProgress (may be nil) receives the "handshake" stage once the transport
// is connected.
func handshakeSSH(
	dial func(addr string) (net.Conn, error),
	addr, hostID, username string,
	auth Auth,
	store HostKeyStore,
	timeout time.Duration,
	onProgress func(stage string),
	onNewKey func(alg, fp string),
) (*Client, error) {
	authMethods, err := buildAuth(auth)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback(store, hostID, onNewKey),
		Timeout:         timeout,
		Config: ssh.Config{
			Ciphers: []string{
				"chacha20-poly1305@openssh.com",
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
			},
			KeyExchanges: []string{
				"curve25519-sha256", "curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256", "ecdh-sha2-nistp384", "ecdh-sha2-nistp521",
			},
		},
	}

	conn, err := dial(addr)
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress("handshake")
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", addr, err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	c := &Client{
		ssh:           client,
		conn:          sshConn,
		stopKeepalive: make(chan struct{}),
	}
	c.startKeepalive(30 * time.Second)
	return c, nil
}

// authOf maps domain credentials onto the connection-layer Auth.
func authOf(creds domain.Credentials) Auth {
	return Auth{
		Password:      creds.Password,
		KeyPath:       creds.KeyPath,
		KeyPassphrase: creds.KeyPassphrase,
		UseAgent:      creds.UseAgent,
	}
}

// httpConnectDialer returns a dialer that opens targets through an HTTP
// CONNECT proxy (optionally with Basic proxy authentication).
func httpConnectDialer(p *domain.ProxyEndpoint, timeout time.Duration) func(addr string) (net.Conn, error) {
	return func(addr string) (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", p.Addr, timeout)
		if err != nil {
			return nil, fmt.Errorf("dial proxy %s: %w", p.Addr, err)
		}
		if deadlineErr := conn.SetDeadline(time.Now().Add(timeout)); deadlineErr != nil {
			_ = conn.Close()
			return nil, deadlineErr
		}

		req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
		if p.Username != "" {
			req += "Proxy-Authorization: Basic " + basicAuth(p.Username, p.Password) + "\r\n"
		}
		req += "\r\n"
		if _, err := io.WriteString(conn, req); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("send CONNECT: %w", err)
		}

		resp, err := httpReadResponse(conn)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("read CONNECT response: %w", err)
		}
		if len(resp) < 12 || resp[:9] != "HTTP/1.1 " && resp[:9] != "HTTP/1.0 " {
			_ = conn.Close()
			return nil, fmt.Errorf("unexpected proxy response: %.40q", resp)
		}
		status := resp[9:12]
		if status != "200" {
			_ = conn.Close()
			return nil, fmt.Errorf("proxy CONNECT %s refused: %s", addr, firstLine(resp))
		}
		// Tunnel established — clear the deadline; this is now a live pipe.
		if err := conn.SetDeadline(time.Time{}); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
}

// httpReadResponse reads the full HTTP response HEADERS from the proxy (the
// body is empty for CONNECT 200 responses; any bytes after the header blank
// line belong to the tunnel and must NOT be consumed — bufio buffers them,
// so the response is parsed without bufio to keep the conn pristine).
func httpReadResponse(conn net.Conn) (string, error) {
	var buf []byte
	chunk := make([]byte, 1)
	for len(buf) < 8192 {
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if len(buf) >= 4 && string(buf[len(buf)-4:]) == "\r\n\r\n" {
				return string(buf), nil
			}
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("proxy response too large")
}

func firstLine(resp string) string {
	for i, r := range resp {
		if r == '\r' || r == '\n' {
			return resp[:i]
		}
	}
	return resp
}

// socks5Dialer returns a dialer that opens targets through a SOCKS5 proxy
// (RFC 1928, CONNECT command, optional username/password auth, RFC 1929).
func socks5Dialer(p *domain.ProxyEndpoint, timeout time.Duration) func(addr string) (net.Conn, error) {
	return func(addr string) (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", p.Addr, timeout)
		if err != nil {
			return nil, fmt.Errorf("dial proxy %s: %w", p.Addr, err)
		}
		if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
			_ = conn.Close()
			return nil, err
		}

		// Greeting: offer no-auth, and user/pass when credentials exist.
		methods := []byte{0x00}
		if p.Username != "" {
			methods = append(methods, 0x02)
		}
		if _, err := conn.Write([]byte{0x05, byte(len(methods))}); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if _, err := conn.Write(methods); err != nil {
			_ = conn.Close()
			return nil, err
		}
		rep := make([]byte, 2)
		if _, err := io.ReadFull(conn, rep); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if rep[0] != 0x05 {
			_ = conn.Close()
			return nil, fmt.Errorf("socks5: unexpected version %d", rep[0])
		}
		switch rep[1] {
		case 0x00: // no-auth
		case 0x02: // username/password
			if p.Username == "" {
				_ = conn.Close()
				return nil, fmt.Errorf("socks5: proxy requires authentication")
			}
			if err := socks5Auth(conn, p.Username, p.Password); err != nil {
				_ = conn.Close()
				return nil, err
			}
		default:
			_ = conn.Close()
			return nil, fmt.Errorf("socks5: no acceptable auth method (0x%02x)", rep[1])
		}

		// CONNECT request — target resolved on the PROXY side, so always
		// send a domain name (ATYP=0x03) and let it resolve.
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			_ = conn.Close()
			return nil, fmt.Errorf("socks5: bad port in %q", addr)
		}
		req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
		req = append(req, host...)
		var portBytes [2]byte
		binary.BigEndian.PutUint16(portBytes[:], uint16(port))
		req = append(req, portBytes[:]...)
		if _, err := conn.Write(req); err != nil {
			_ = conn.Close()
			return nil, err
		}

		// Reply: VER REP RSV ATYP ADDR... — header 4 bytes + variable addr.
		head := make([]byte, 4)
		if _, err := io.ReadFull(conn, head); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if head[1] != 0x00 {
			_ = conn.Close()
			return nil, fmt.Errorf("socks5: CONNECT refused (code 0x%02x)", head[1])
		}
		var rest int
		switch head[3] {
		case 0x01: // IPv4
			rest = 4
		case 0x03: // domain
			lenBuf := make([]byte, 1)
			if _, err := io.ReadFull(conn, lenBuf); err != nil {
				_ = conn.Close()
				return nil, err
			}
			rest = int(lenBuf[0])
		case 0x04: // IPv6
			rest = 16
		default:
			_ = conn.Close()
			return nil, fmt.Errorf("socks5: bad address type 0x%02x", head[3])
		}
		if rest > 0 {
			if _, err := io.ReadFull(conn, make([]byte, rest+2)); err != nil { // addr + port
				_ = conn.Close()
				return nil, err
			}
		}

		if err := conn.SetDeadline(time.Time{}); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
}

// socks5Auth performs the RFC 1929 username/password subnegotiation.
func socks5Auth(conn net.Conn, username, password string) error {
	req := []byte{0x01, byte(len(username))}
	req = append(req, username...)
	req = append(req, byte(len(password)))
	req = append(req, password...)
	if _, err := conn.Write(req); err != nil {
		return err
	}
	rep := make([]byte, 2)
	if _, err := io.ReadFull(conn, rep); err != nil {
		return err
	}
	if rep[1] != 0x00 {
		return fmt.Errorf("socks5: authentication rejected")
	}
	return nil
}

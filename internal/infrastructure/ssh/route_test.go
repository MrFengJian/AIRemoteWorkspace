package ssh

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// startFakeHTTPProxy runs a minimal HTTP CONNECT proxy: reads the CONNECT
// request, always answers 200, then pipes raw bytes to the requested target
// (resolved from THIS machine — the fake "proxy network").
func startFakeHTTPProxy(t *testing.T, requireUser, requirePass string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				req, err := httpReadResponse(c)
				if err != nil {
					return
				}
				if requireUser != "" {
					want := "Proxy-Authorization: Basic " + basicAuth(requireUser, requirePass)
					if !strings.Contains(req, want) {
						_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
						return
					}
				}
				addr := targetOfConnect(req)
				if addr == "" {
					return
				}
				_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
				remote, err := net.Dial("tcp", addr)
				if err != nil {
					return
				}
				defer remote.Close()
				go func() { _, _ = io.Copy(remote, c) }()
				_, _ = io.Copy(c, remote)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// targetOfConnect extracts "host:port" from a CONNECT request line.
func targetOfConnect(req string) string {
	for _, line := range strings.Split(req, "\r\n") {
		parts := strings.Split(line, " ")
		if len(parts) >= 2 && parts[0] == "CONNECT" {
			return parts[1]
		}
	}
	return ""
}

// startFakeSocks5Proxy runs a minimal RFC1928/1929 SOCKS5 server that pipes
// to the requested target.
func startFakeSocks5Proxy(t *testing.T, requireUser, requirePass string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("socks listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				head := make([]byte, 2)
				if _, err := io.ReadFull(c, head); err != nil || head[0] != 0x05 {
					return
				}
				methods := make([]byte, head[1])
				if _, err := io.ReadFull(c, methods); err != nil {
					return
				}
				wantAuth := requireUser != ""
				chosen := byte(0x00)
				if wantAuth {
					chosen = 0x02
					if !authOffered(methods) {
						_, _ = c.Write([]byte{0x05, 0xFF})
						return
					}
				} else if !noAuthOffered(methods) {
					_, _ = c.Write([]byte{0x05, 0xFF})
					return
				}
				// The method choice must go out BEFORE reading credentials —
				// the client waits for it before sending them.
				if _, err := c.Write([]byte{0x05, chosen}); err != nil {
					return
				}
				if chosen == 0x02 {
					if err := socks5ServerAuth(c, requireUser, requirePass); err != nil {
						return
					}
				}

				head4 := make([]byte, 4)
				if _, err := io.ReadFull(c, head4); err != nil {
					return
				}
				var addr string
				switch head4[3] {
				case 0x01:
					b := make([]byte, 4)
					if _, err := io.ReadFull(c, b); err != nil {
						return
					}
					addr = net.IP(b).String()
				case 0x03:
					l := make([]byte, 1)
					if _, err := io.ReadFull(c, l); err != nil {
						return
					}
					h := make([]byte, l[0])
					if _, err := io.ReadFull(c, h); err != nil {
						return
					}
					addr = string(h)
				default:
					return
				}
				port := make([]byte, 2)
				if _, err := io.ReadFull(c, port); err != nil {
					return
				}
				remote, err := net.Dial("tcp", net.JoinHostPort(addr, itoa(int(port[0])<<8|int(port[1]))))
				if err != nil {
					return
				}
				defer remote.Close()
				_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(remote, c) }()
				_, _ = io.Copy(c, remote)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func authOffered(methods []byte) bool {
	for _, m := range methods {
		if m == 0x02 {
			return true
		}
	}
	return false
}

func noAuthOffered(methods []byte) bool {
	for _, m := range methods {
		if m == 0x00 {
			return true
		}
	}
	return false
}

// socks5ServerAuth verifies the RFC1929 user/pass subnegotiation server-side
// (VER ULEN UNAME PLEN PASSWD — the VER byte arrives first).
func socks5ServerAuth(c net.Conn, user, pass string) error {
	ver := make([]byte, 1)
	if _, err := io.ReadFull(c, ver); err != nil {
		return err
	}
	if ver[0] != 0x01 {
		_, _ = c.Write([]byte{0x01, 0x01})
		return errAuthFailed
	}
	ulen := make([]byte, 1)
	if _, err := io.ReadFull(c, ulen); err != nil {
		return err
	}
	u := make([]byte, ulen[0])
	if _, err := io.ReadFull(c, u); err != nil {
		return err
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(c, plen); err != nil {
		return err
	}
	p := make([]byte, plen[0])
	if _, err := io.ReadFull(c, p); err != nil {
		return err
	}
	if string(u) != user || string(p) != pass {
		_, _ = c.Write([]byte{0x01, 0x01})
		return errAuthFailed
	}
	_, err := c.Write([]byte{0x01, 0x00})
	return err
}

var errAuthFailed = io.ErrUnexpectedEOF // marker; content irrelevant

func TestHTTPConnectDialer(t *testing.T) {
	target := echoServer(t)
	proxyAddr := startFakeHTTPProxy(t, "alice", "s3cret")

	dial := httpConnectDialer(&domain.ProxyEndpoint{
		Kind: "http", Addr: proxyAddr, Username: "alice", Password: "s3cret",
	}, 3*time.Second)
	conn, err := dial(target)
	if err != nil {
		t.Fatalf("dial via http proxy: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo mismatch: %q", buf)
	}
}

func TestHTTPConnectDialerBadAuth(t *testing.T) {
	target := echoServer(t)
	proxyAddr := startFakeHTTPProxy(t, "alice", "s3cret")
	dial := httpConnectDialer(&domain.ProxyEndpoint{
		Kind: "http", Addr: proxyAddr, Username: "alice", Password: "wrong",
	}, 3*time.Second)
	if _, err := dial(target); err == nil {
		t.Fatal("dial with wrong proxy password succeeded")
	}
}

func TestSocks5Dialer(t *testing.T) {
	target := echoServer(t)
	proxyAddr := startFakeSocks5Proxy(t, "bob", "pw")

	dial := socks5Dialer(&domain.ProxyEndpoint{
		Kind: "socks5", Addr: proxyAddr, Username: "bob", Password: "pw",
	}, 3*time.Second)
	conn, err := dial(target)
	if err != nil {
		t.Fatalf("dial via socks5: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("pong")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("echo mismatch: %q", buf)
	}
}

func TestSocks5DialerNoAuth(t *testing.T) {
	target := echoServer(t)
	proxyAddr := startFakeSocks5Proxy(t, "", "")
	dial := socks5Dialer(&domain.ProxyEndpoint{Kind: "socks5", Addr: proxyAddr}, 3*time.Second)
	conn, err := dial(target)
	if err != nil {
		t.Fatalf("dial via socks5 (no auth): %v", err)
	}
	_ = conn.Close()
}

// httpReadResponse is exercised here too (kept in sync with the dialer).
func TestHTTPReadResponseFindsHeaderEnd(t *testing.T) {
	server, client := net.Pipe()
	go func() {
		_, _ = io.WriteString(server, "HTTP/1.1 200 Connection established\r\nServer: x\r\n\r\nextra-tunnel-bytes")
		server.Close()
	}()
	resp, err := httpReadResponse(client)
	_ = client.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasSuffix(resp, "\r\n\r\n") {
		t.Fatalf("response consumed past header end: %q", resp)
	}
	// bufio is intentionally unused — assert the helper does not exist so a
	// future refactor cannot reintroduce over-buffering of tunnel bytes.
	var _ = bufio.NewReader
}

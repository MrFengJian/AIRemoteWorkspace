package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// --- stubs ---

type stubHosts struct {
	hosts []domain.Host
}

func (h *stubHosts) List() ([]domain.Host, error) { return h.hosts, nil }

func (h *stubHosts) Get(id string) (domain.Host, error) {
	for _, x := range h.hosts {
		if x.ID == id {
			return x, nil
		}
	}
	return domain.Host{}, errors.New("not found")
}

func (h *stubHosts) ResolveCredentials(host domain.Host, provided domain.Credentials) (domain.Credentials, error) {
	return provided, nil
}

type stubSSH struct {
	opened  []string
	closed  []string
	execOut string
}

func (m *stubSSH) OpenSession(_ context.Context, host domain.Host, _ domain.Credentials, _, _ int, _ application.SessionEvents) (string, error) {
	m.opened = append(m.opened, host.ID)
	return "sess-" + host.ID, nil
}

func (m *stubSSH) ExecInSessionCtx(_ context.Context, _, _ string) (string, error) {
	return m.execOut, nil
}

func (m *stubSSH) Close(sessionID string) error {
	m.closed = append(m.closed, sessionID)
	return nil
}

type stubSFTP struct {
	writtenPath string
}

func (s *stubSFTP) DownloadFile(_ domain.Host, _ domain.Credentials, _ string, _ application.SftpProgress) ([]byte, error) {
	return []byte("file-data"), nil
}

func (s *stubSFTP) UploadFile(_ domain.Host, _ domain.Credentials, remotePath string, _ []byte, _ application.SftpProgress) error {
	s.writtenPath = remotePath
	return nil
}

type gateCall struct {
	sessionID string
	toolName  string
	perm      domain.Permission
}

type stubGate struct{ calls []gateCall }

func (g *stubGate) Check(_ context.Context, sessionID, toolName string, perm domain.Permission, _ string) error {
	g.calls = append(g.calls, gateCall{sessionID, toolName, perm})
	return nil
}

// newTestServer builds a server with one configured host, listening on a free
// port with the given config overrides applied.
func newTestServer(t *testing.T, mutate func(*domain.MCPConfig)) (*Server, *stubSSH, *stubGate) {
	t.Helper()
	free := freePort(t)
	hosts := &stubHosts{hosts: []domain.Host{{
		ID: "h1", Name: "web-01", Host: "10.0.0.5", Port: 22,
		Username: "root", AuthType: domain.AuthPassword,
	}}}
	ssh := &stubSSH{execOut: "load average: 0.10"}
	gate := &stubGate{}
	s := New(Deps{
		AppName: "AI Remote Workspace", AppVersion: "test",
		Hosts: hosts, SSH: ssh, SFTP: &stubSFTP{}, Gate: gate,
	})
	cfg := domain.MCPConfig{Enabled: true, Port: free, Token: "tok123"}
	if mutate != nil {
		mutate(&cfg)
	}
	s.ApplyConfig(cfg)
	t.Cleanup(s.Stop)
	return s, ssh, gate
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// connect opens a real MCP client session against the running server,
// injecting the bearer token on every request.
func connect(t *testing.T, port int, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	tr := &mcp.StreamableClientTransport{
		Endpoint: fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
		HTTPClient: &http.Client{Transport: authInjector{token: token}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cs, err := client.Connect(ctx, tr, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type authInjector struct{ token string }

func (a authInjector) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(r)
}

// --- tests ---

func TestAuthRequiresBearer(t *testing.T) {
	s, _, _ := newTestServer(t, nil)
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", s.Status().Port)

	// No token / wrong token → 401 from the auth layer.
	for _, tc := range []struct{ name, header string }{
		{"no header", ""},
		{"wrong token", "Bearer nope"},
	} {
		req, _ := http.NewRequest(http.MethodPost, url, nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", tc.name, resp.StatusCode)
		}
	}
}

func TestToolRoundTrip(t *testing.T) {
	s, ssh, gate := newTestServer(t, nil)
	cs := connect(t, s.Status().Port, "tok123")

	// The full Phase 6 tool set is exposed.
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := []string{"list_hosts", "connect_host", "exec_command", "read_file",
		"write_file", "upload", "download", "system_info"}
	got := map[string]bool{}
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %q missing from tools/list", name)
		}
	}

	// list_hosts renders the configured host.
	call, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_hosts"})
	if err != nil {
		t.Fatalf("list_hosts: %v", err)
	}
	if text := textOf(t, call); !strings.Contains(text, "web-01") || !strings.Contains(text, "id=h1") {
		t.Errorf("list_hosts output = %q, want host name + id", text)
	}

	// exec_command: READ-classified command auto-passes the gate, runs, and
	// the gate is keyed by the MCP pseudo-session naming the target host.
	call, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "exec_command",
		Arguments: map[string]any{"hostId": "h1", "command": "uptime"},
	})
	if err != nil {
		t.Fatalf("exec_command: %v", err)
	}
	if text := textOf(t, call); !strings.Contains(text, "load average") {
		t.Errorf("exec_command output = %q, want stub output", text)
	}
	if len(ssh.opened) != 1 || ssh.opened[0] != "h1" {
		t.Errorf("ssh sessions opened = %v, want [h1]", ssh.opened)
	}
	last := gate.calls[len(gate.calls)-1]
	if last.sessionID != "mcp:web-01" || last.toolName != "exec_command" || last.perm != domain.PermissionRead {
		t.Errorf("gate call = %+v, want mcp:web-01/exec_command/READ", last)
	}

	// write_file requires approval (WRITE tier) even via MCP.
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "write_file",
		Arguments: map[string]any{"name": "web-01", "path": "/tmp/x", "content": "hi"},
	}); err != nil {
		t.Fatalf("write_file: %v", err)
	}
	last = gate.calls[len(gate.calls)-1]
	if last.perm != domain.PermissionWrite || last.toolName != "write_file" {
		t.Errorf("write_file gate call = %+v, want WRITE", last)
	}

	// Unknown host → tool error (IsError) mentioning list_hosts.
	bad, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "exec_command",
		Arguments: map[string]any{"name": "nope", "command": "uptime"},
	})
	if err != nil {
		t.Fatalf("exec_command unknown host: %v", err)
	}
	if !bad.IsError || !strings.Contains(textOf(t, bad), "list_hosts") {
		t.Errorf("unknown host result = IsError:%v %q, want error with list_hosts hint", bad.IsError, textOf(t, bad))
	}
}

func TestFirstEnableGeneratesAndPersistsToken(t *testing.T) {
	free := freePort(t)
	var persisted domain.MCPConfig
	s := New(Deps{
		AppName: "app", AppVersion: "0",
		Hosts: &stubHosts{}, SSH: &stubSSH{}, SFTP: &stubSFTP{}, Gate: &stubGate{},
		Persist: func(c domain.MCPConfig) error { persisted = c; return nil },
	})
	s.ApplyConfig(domain.MCPConfig{Enabled: true, Port: free})
	defer s.Stop()

	st := s.Status()
	if st.Token == "" || len(st.Token) < 32 {
		t.Errorf("generated token too short: %q", st.Token)
	}
	if persisted.Token != st.Token {
		t.Errorf("persisted token %q != effective %q", persisted.Token, st.Token)
	}
	if !st.Running {
		t.Error("server not running after enable")
	}
}

func TestDisableStopsAndClosesSessions(t *testing.T) {
	s, ssh, _ := newTestServer(t, nil)
	cs := connect(t, s.Status().Port, "tok123")
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "connect_host",
		Arguments: map[string]any{"hostId": "h1"},
	}); err != nil {
		t.Fatalf("connect_host: %v", err)
	}

	// Disabling tears the listener down and releases MCP-owned sessions.
	s.ApplyConfig(domain.MCPConfig{Enabled: false, Port: s.Status().Port, Token: "tok123"})
	if st := s.Status(); st.Running {
		t.Error("server still running after disable")
	}
	if len(ssh.closed) == 0 || ssh.closed[0] != "sess-h1" {
		t.Errorf("closed sessions = %v, want [sess-h1]", ssh.closed)
	}
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

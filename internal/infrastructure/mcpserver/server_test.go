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
	opened    []string
	closed    []string
	execOut   string
	connected map[string]bool   // ConnectedHosts result (nil = none)
	live      map[string]string // hostID → live sessionID for SessionForHost
}

func (m *stubSSH) OpenSession(_ context.Context, host domain.Host, _ domain.Credentials, _, _ int, _ application.SessionEvents) (string, error) {
	m.opened = append(m.opened, host.ID)
	if m.live == nil {
		m.live = map[string]string{}
	}
	m.live[host.ID] = "sess-" + host.ID
	return "sess-" + host.ID, nil
}

func (m *stubSSH) ExecInSessionCtx(_ context.Context, _, _ string) (string, error) {
	return m.execOut, nil
}

func (m *stubSSH) Close(sessionID string) error {
	m.closed = append(m.closed, sessionID)
	return nil
}

func (m *stubSSH) ConnectedHosts() map[string]bool { return m.connected }

func (m *stubSSH) SessionForHost(hostID string) (string, bool) {
	sid, ok := m.live[hostID]
	return sid, ok
}

// stubMonitor serves canned monitor data and counts calls (cache-sharing is
// observable as one call serving repeated host_stats requests).
type stubMonitor struct {
	calls int
}

func (m *stubMonitor) GetOverview(context.Context, string) (domain.MonitorOverview, error) {
	m.calls++
	return domain.MonitorOverview{
		CPUPercent: 12.5, CPUCores: 4, CPUModel: "Intel Xeon",
		Load1: 0.5, Load5: 0.3, Load15: 0.2, UptimeSeconds: 90000,
		MemTotalKB: 8 * 1024 * 1024, MemUsedKB: 2 * 1024 * 1024, MemUsedPercent: 25,
		ProcessTotal: 210, ProcessRunning: 3,
		NetRxBytesPerSec: 1.5 * 1024 * 1024, NetTxBytesPerSec: 300 * 1024,
		Disks:     []domain.MonitorDiskUsage{{Device: "/dev/vda1", Mount: "/", TotalKB: 100 * 1024 * 1024, UsedKB: 45 * 1024 * 1024, UsedPercent: 45}},
		TCPStates: []domain.MonitorTCPState{{State: "ESTABLISHED", Count: 12}, {State: "LISTEN", Count: 8}},
	}, nil
}

func (m *stubMonitor) GetProcesses(context.Context, string) ([]domain.MonitorProcess, error) {
	m.calls++
	return []domain.MonitorProcess{
		{PID: 42, Name: "nginx", CommandLine: "nginx: master process", CPUPercent: 33.3, RSSKB: 64 * 1024},
		{PID: 7, Name: "node", CPUPercent: 1.2, RSSKB: 512 * 1024},
	}, nil
}

func (m *stubMonitor) GetPorts(context.Context, string) ([]domain.MonitorPort, error) {
	m.calls++
	return []domain.MonitorPort{
		{Proto: "tcp", Address: "0.0.0.0", Port: 8080, Count: 2},
		{Proto: "tcp6", Address: "::", Port: 22},
	}, nil
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
func newTestServer(t *testing.T, mutate func(*domain.MCPConfig)) (*Server, *stubSSH, *stubGate, *stubMonitor) {
	t.Helper()
	free := freePort(t)
	hosts := &stubHosts{hosts: []domain.Host{{
		ID: "h1", Name: "web-01", Host: "10.0.0.5", Port: 22,
		Username: "root", AuthType: domain.AuthPassword,
	}}}
	ssh := &stubSSH{execOut: "load average: 0.10"}
	gate := &stubGate{}
	mon := &stubMonitor{}
	s := New(Deps{
		AppName: "AI Remote Workspace", AppVersion: "test",
		Hosts: hosts, SSH: ssh, SFTP: &stubSFTP{}, Gate: gate, Monitor: mon,
	})
	cfg := domain.MCPConfig{Enabled: true, Port: free, Token: "tok123"}
	if mutate != nil {
		mutate(&cfg)
	}
	s.ApplyConfig(cfg)
	t.Cleanup(s.Stop)
	return s, ssh, gate, mon
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
		Endpoint:   fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
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
	s, _, _, _ := newTestServer(t, nil)
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
	s, ssh, gate, _ := newTestServer(t, nil)
	cs := connect(t, s.Status().Port, "tok123")

	// The full Phase 6 tool set is exposed.
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := []string{"list_hosts", "connect_host", "exec_command", "read_file",
		"write_file", "upload", "download", "system_info", "host_stats"}
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
	s, ssh, _, _ := newTestServer(t, nil)
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

// TestListHostsFilters 覆盖 list_hosts 的 group/status 过滤与状态标注。
func TestListHostsFilters(t *testing.T) {
	free := freePort(t)
	hosts := &stubHosts{hosts: []domain.Host{
		{ID: "h1", Name: "web-01", Host: "10.0.0.5", Port: 22, Username: "root",
			AuthType: domain.AuthPassword, Group: "production/web"},
		{ID: "h2", Name: "db-01", Host: "10.0.0.6", Port: 22, Username: "root",
			AuthType: domain.AuthPassword, Group: "production/db"},
		{ID: "h3", Name: "dev-01", Host: "10.0.0.7", Port: 22, Username: "root",
			AuthType: domain.AuthPassword, Group: "dev"},
	}}
	ssh := &stubSSH{execOut: "ok", connected: map[string]bool{"h1": true}}
	s := New(Deps{
		AppName: "AI Remote Workspace", AppVersion: "test",
		Hosts: hosts, SSH: ssh, SFTP: &stubSFTP{}, Gate: &stubGate{},
	})
	s.ApplyConfig(domain.MCPConfig{Enabled: true, Port: free, Token: "tok123"})
	t.Cleanup(s.Stop)
	cs := connect(t, free, "tok123")

	call := func(t *testing.T, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "list_hosts", Arguments: args,
		})
		if err != nil {
			t.Fatalf("list_hosts %v: %v", args, err)
		}
		return textOf(t, res)
	}

	// 无过滤：全部主机都带 status 标注。
	all := call(t, nil)
	for _, want := range []string{"web-01", "db-01", "dev-01", "status=connected", "status=disconnected", "group=production/web"} {
		if !strings.Contains(all, want) {
			t.Errorf("all hosts output missing %q:\n%s", want, all)
		}
	}

	// 分组子串过滤（大小写不敏感、支持嵌套路径前缀）。
	prod := call(t, map[string]any{"group": "PRODUCTION"})
	if !strings.Contains(prod, "web-01") || !strings.Contains(prod, "db-01") || strings.Contains(prod, "dev-01") {
		t.Errorf("group=PRODUCTION output = %q, want only production hosts", prod)
	}
	web := call(t, map[string]any{"group": "production/web"})
	if !strings.Contains(web, "web-01") || strings.Contains(web, "db-01") {
		t.Errorf("group=production/web output = %q, want only web-01", web)
	}

	// 连接状态过滤：只有 h1 有活动会话。
	conn := call(t, map[string]any{"status": "connected"})
	if !strings.Contains(conn, "web-01") || strings.Contains(conn, "db-01") || strings.Contains(conn, "dev-01") {
		t.Errorf("status=connected output = %q, want only web-01", conn)
	}
	disc := call(t, map[string]any{"status": "disconnected"})
	if !strings.Contains(disc, "db-01") || strings.Contains(disc, "web-01") {
		t.Errorf("status=disconnected output = %q, want db-01 but not web-01", disc)
	}

	// 组合过滤 + 非法状态值。
	both := call(t, map[string]any{"group": "production", "status": "connected"})
	if !strings.Contains(both, "web-01") || strings.Contains(both, "db-01") {
		t.Errorf("combined filter output = %q, want only web-01", both)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_hosts", Arguments: map[string]any{"status": "online"},
	}); err != nil {
		t.Fatalf("invalid status call: %v", err)
	} else if !res.IsError || !strings.Contains(textOf(t, res), "connected") {
		t.Errorf("invalid status = %+v, want tool error mentioning allowed values", res)
	}
}

// TestHostStatsNotConnected: the tool is scoped to hosts with a live session.
func TestHostStatsNotConnected(t *testing.T) {
	s, _, _, _ := newTestServer(t, nil)
	cs := connect(t, s.Status().Port, "tok123")

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "host_stats", Arguments: map[string]any{"hostId": "h1"},
	})
	if err != nil {
		t.Fatalf("host_stats: %v", err)
	}
	if !res.IsError || !strings.Contains(textOf(t, res), "connect_host") {
		t.Errorf("not-connected result = IsError:%v %q, want error hinting connect_host", res.IsError, textOf(t, res))
	}
}

// TestHostStatsSections: overview / processes / ports render their data, the
// gate sees a READ call, and an unknown section lists the valid values.
func TestHostStatsSections(t *testing.T) {
	s, ssh, gate, _ := newTestServer(t, nil)
	cs := connect(t, s.Status().Port, "tok123")

	// connect_host opens an MCP-owned session — host_stats then rides it.
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "connect_host", Arguments: map[string]any{"hostId": "h1"},
	}); err != nil {
		t.Fatalf("connect_host: %v", err)
	}
	if sid, ok := ssh.SessionForHost("h1"); !ok || sid != "sess-h1" {
		t.Fatalf("SessionForHost(h1) = %q %v, want sess-h1", sid, ok)
	}

	call := func(t *testing.T, section string) *mcp.CallToolResult {
		t.Helper()
		args := map[string]any{"hostId": "h1"}
		if section != "" {
			args["section"] = section
		}
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "host_stats", Arguments: args,
		})
		if err != nil {
			t.Fatalf("host_stats %q: %v", section, err)
		}
		return res
	}

	text := textOf(t, call(t, "")) // no section = overview
	for _, want := range []string{"web-01", "CPU: 12.5%", "4 cores", "Memory: 2.0G / 8.0G", "ESTABLISHED=12"} {
		if !strings.Contains(text, want) {
			t.Errorf("overview output missing %q:\n%s", want, text)
		}
	}
	if text = textOf(t, call(t, "processes")); !strings.Contains(text, "nginx") || !strings.Contains(text, "512.0M") {
		t.Errorf("processes output = %q, want process rows", text)
	}
	if text = textOf(t, call(t, "PORTS")); !strings.Contains(text, "*:8080 tcp (x2)") || !strings.Contains(text, "*:22 tcp6") {
		t.Errorf("ports output = %q, want listening sockets", text)
	}
	if last := gate.calls[len(gate.calls)-1]; last.toolName != "host_stats" || last.perm != domain.PermissionRead {
		t.Errorf("gate call = %+v, want host_stats/READ", last)
	}

	if res := call(t, "nope"); !res.IsError || !strings.Contains(textOf(t, res), "overview") {
		t.Errorf("invalid section = %+v, want tool error listing valid sections", res)
	}
}

// stubMonConn backs a real application.MonitorService so the cache-sharing
// behaviour (panel + MCP read one collection) is exercised end to end.
type stubMonConn struct {
	execs  int
	hostOf map[string]domain.Host
}

func (f *stubMonConn) OpenSession(context.Context, domain.Host, domain.Credentials, int, int, application.SessionEvents) (string, error) {
	return "", errors.New("not implemented")
}
func (f *stubMonConn) WriteStdin(string, []byte) error { return errors.New("not implemented") }
func (f *stubMonConn) Resize(string, int, int) error   { return errors.New("not implemented") }
func (f *stubMonConn) Close(string) error              { return nil }
func (f *stubMonConn) DetectOS(string) (string, error) { return "", nil }
func (f *stubMonConn) ExecInSessionStdin(context.Context, string, string, []byte) (string, error) {
	return "", errors.New("not implemented")
}
func (f *stubMonConn) CloseAll() error { return nil }
func (f *stubMonConn) HostOfSession(sid string) (domain.Host, bool) {
	h, ok := f.hostOf[sid]
	return h, ok
}
func (f *stubMonConn) ExecInSessionCtx(_ context.Context, _, script string) (string, error) {
	f.execs++
	if strings.Contains(script, "=CPUINFO") {
		return "=LOAD\n0.10 0.20 0.30 1/2 3\n", nil
	}
	return "", errors.New("unexpected script")
}

// TestHostStatsSharesCacheWithPanel: repeated MCP calls and a panel-style
// read (another session of the same host) hit one shared collection.
func TestHostStatsSharesCacheWithPanel(t *testing.T) {
	free := freePort(t)
	conn := &stubMonConn{hostOf: map[string]domain.Host{
		"sess-term": {ID: "h1"}, // the terminal tab's session
		"sess-h1":   {ID: "h1"}, // the MCP-owned session from connect_host
	}}
	mon := application.NewMonitorService(conn)
	ssh := &stubSSH{live: map[string]string{"h1": "sess-h1"}}
	s := New(Deps{
		AppName: "app", AppVersion: "test",
		Hosts: &stubHosts{hosts: []domain.Host{{ID: "h1", Name: "web-01"}}},
		SSH:   ssh, SFTP: &stubSFTP{}, Gate: &stubGate{}, Monitor: mon,
	})
	s.ApplyConfig(domain.MCPConfig{Enabled: true, Port: free, Token: "tok123"})
	t.Cleanup(s.Stop)
	cs := connect(t, free, "tok123")

	text := func(t *testing.T) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "host_stats", Arguments: map[string]any{"hostId": "h1"},
		})
		if err != nil {
			t.Fatalf("host_stats: %v", err)
		}
		if res.IsError {
			t.Fatalf("host_stats errored: %s", textOf(t, res))
		}
		return textOf(t, res)
	}

	first := text(t)
	second := text(t) // same host, same TTL window
	if first != second || conn.execs != 1 {
		t.Fatalf("MCP re-collected: execs=%d", conn.execs)
	}

	// A panel-style read through a different session of the same host shares
	// the entry too — no new exec round-trip.
	if _, err := mon.GetOverview(context.Background(), "sess-term"); err != nil {
		t.Fatalf("panel read: %v", err)
	}
	if conn.execs != 1 {
		t.Fatalf("execs = %d, want 1 across MCP + panel", conn.execs)
	}
}

func TestFormatStatsRenderers(t *testing.T) {
	o := domain.MonitorOverview{
		CPUPercent: 12.5, CPUCores: 4, CPUModel: " Intel Xeon ",
		Load1: 1, Load5: 2, Load15: 3, UptimeSeconds: 90000, // 1d1h
		MemTotalKB: 8 * 1024 * 1024, MemUsedKB: 2 * 1024 * 1024, MemUsedPercent: 25,
		SwapTotalKB: 2 * 1024 * 1024, SwapUsedKB: 512 * 1024,
		Disks: []domain.MonitorDiskUsage{
			{Device: "/dev/vda1", Mount: "/", UsedPercent: 45, TotalKB: 100 << 20, UsedKB: 45 << 20},
			{Device: "tmpfs", Mount: "/dev/shm", UsedPercent: 0},
		},
		NetRxBytesPerSec: 1572864, NetTxBytesPerSec: 307200, // 1.5MB/s / 300KB/s
		ProcessTotal: 210, ProcessRunning: 3,
		TCPStates: []domain.MonitorTCPState{{State: "ESTABLISHED", Count: 12}, {State: "LISTEN", Count: 8}},
		Kernel:    "5.15.0",
	}
	ov := formatOverview("web-01", o)
	for _, want := range []string{
		"Host web-01 — overview", "CPU: 12.5% (4 cores) — Intel Xeon",
		"Load: 1.00 2.00 3.00  uptime 1d1h", "swap 512.0M / 2.0G",
		"/dev/vda1 on /: 45%", "↓ 1.5MB/s  ↑ 300.0KB/s",
		"Processes: 210 total, 3 running", "TCP: ESTABLISHED=12, LISTEN=8", "Kernel: 5.15.0",
	} {
		if !strings.Contains(ov, want) {
			t.Errorf("overview missing %q:\n%s", want, ov)
		}
	}

	procs := formatProcesses("web-01", []domain.MonitorProcess{
		{PID: 42, Name: "nginx", CommandLine: "nginx: master process", CPUPercent: 33.3, RSSKB: 64 << 10},
		{PID: 7, Name: "node", CPUPercent: 1.2, RSSKB: 512 << 10},
	})
	for _, want := range []string{"By CPU", "42       33.3% cpu  64.0M", "— nginx: master process", "By memory", "7         1.2% cpu  512.0M"} {
		if !strings.Contains(procs, want) {
			t.Errorf("processes missing %q:\n%s", want, procs)
		}
	}
	if empty := formatProcesses("web-01", nil); !strings.Contains(empty, "no process data") {
		t.Errorf("empty processes = %q", empty)
	}

	ports := formatPorts("web-01", []domain.MonitorPort{
		{Proto: "tcp", Address: "0.0.0.0", Port: 8080, Count: 2},
		{Proto: "tcp6", Address: "::", Port: 22},
	})
	for _, want := range []string{"listening ports", "  *:22 tcp6", "  *:8080 tcp (x2)"} {
		if !strings.Contains(ports, want) {
			t.Errorf("ports missing %q:\n%s", want, ports)
		}
	}
	if empty := formatPorts("web-01", nil); !strings.Contains(empty, "no listening") {
		t.Errorf("empty ports = %q", empty)
	}
}

package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// fakeMonConn serves the canned section outputs and counts exec round-trips,
// so cache hits (no exec) are directly observable.
type fakeMonConn struct {
	execs  int
	fail   bool
	hostOf map[string]domain.Host // sessionID → host
}

func (f *fakeMonConn) OpenSession(context.Context, domain.Host, domain.Credentials, int, int, SessionEvents) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeMonConn) WriteStdin(string, []byte) error { return errors.New("not implemented") }
func (f *fakeMonConn) Resize(string, int, int) error   { return errors.New("not implemented") }
func (f *fakeMonConn) Close(string) error              { return nil }
func (f *fakeMonConn) DetectOS(string) (string, error) { return "", nil }
func (f *fakeMonConn) ExecInSessionStdin(context.Context, string, string, []byte) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeMonConn) CloseAll() error { return nil }
func (f *fakeMonConn) HostOfSession(sessionID string) (domain.Host, bool) {
	h, ok := f.hostOf[sessionID]
	return h, ok
}

func (f *fakeMonConn) ExecInSessionCtx(_ context.Context, _, script string) (string, error) {
	f.execs++
	if f.fail {
		return "", errors.New("connection lost")
	}
	switch {
	case strings.Contains(script, "=CPUINFO"):
		return overviewSample, nil
	case strings.Contains(script, "=PAGESIZE"):
		return processSample, nil
	case strings.Contains(script, "/proc/net/tcp"):
		return portsSample, nil
	}
	return "", errors.New("unexpected script")
}

func newCachedMonitor(t *testing.T) (*MonitorService, *fakeMonConn, func(time.Duration)) {
	t.Helper()
	conn := &fakeMonConn{hostOf: map[string]domain.Host{
		"sess-a": {ID: "h1"},
		"sess-b": {ID: "h1"}, // second terminal tab on the same host
		"sess-c": {ID: "h2"},
		// sessions absent from the map (local/host-less) scope by session id
	}}
	s := NewMonitorService(conn)
	s.cacheTTL = time.Minute // deterministic: only the fake clock moves time
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	advance := func(d time.Duration) { clock = clock.Add(d) }
	return s, conn, advance
}

// 面板与 MCP 通过不同 sessionID 读同一台主机时共享一份采集结果。
func TestMonitorCacheSharedAcrossSessionsOfOneHost(t *testing.T) {
	s, conn, _ := newCachedMonitor(t)
	ctx := context.Background()

	o1, err := s.GetOverview(ctx, "sess-a")
	if err != nil {
		t.Fatalf("GetOverview sess-a: %v", err)
	}
	o2, err := s.GetOverview(ctx, "sess-b") // same host, other tab/session
	if err != nil {
		t.Fatalf("GetOverview sess-b: %v", err)
	}
	if o2.CPUPercent != o1.CPUPercent || o2.MemTotalKB != o1.MemTotalKB || conn.execs != 1 {
		t.Fatalf("second call re-collected: execs=%d, cpu=%v/%v", conn.execs, o1.CPUPercent, o2.CPUPercent)
	}

	// Sections cache independently: one exec per section.
	if _, err = s.GetProcesses(ctx, "sess-a"); err != nil {
		t.Fatalf("GetProcesses: %v", err)
	}
	if _, err = s.GetPorts(ctx, "sess-a"); err != nil {
		t.Fatalf("GetPorts: %v", err)
	}
	if conn.execs != 3 {
		t.Fatalf("execs = %d, want 3 (overview+processes+ports)", conn.execs)
	}

	// A different host never sees h1's cached entries.
	if _, err = s.GetOverview(ctx, "sess-c"); err != nil {
		t.Fatalf("GetOverview h2: %v", err)
	}
	if conn.execs != 4 {
		t.Fatalf("execs = %d, want 4 (h2 overview was a fresh collect)", conn.execs)
	}
}

// TTL 过期后重新采集；采集失败不缓存，恢复后立即拿到数据。
func TestMonitorCacheTTLAndErrorPolicy(t *testing.T) {
	s, conn, advance := newCachedMonitor(t)
	ctx := context.Background()

	if _, err := s.GetOverview(ctx, "sess-a"); err != nil {
		t.Fatalf("warm cache: %v", err)
	}

	// Within TTL: cached. After TTL: re-collected.
	advance(s.cacheTTL)
	if _, err := s.GetOverview(ctx, "sess-a"); err != nil {
		t.Fatalf("after TTL: %v", err)
	}
	if conn.execs != 2 {
		t.Fatalf("execs = %d, want 2 after TTL expiry", conn.execs)
	}

	// Errors are not cached: failing calls always retry, and a later
	// success repopulates the entry. (Advance past the fresh window first,
	// otherwise the re-collected entry from the line above still serves.)
	advance(s.cacheTTL)
	conn.fail = true
	if _, err := s.GetOverview(ctx, "sess-a"); err == nil {
		t.Fatal("expected collection error while fail=true")
	}
	if _, err := s.GetOverview(ctx, "sess-a"); err == nil {
		t.Fatal("second failing call must also hit the connection (error not cached)")
	}
	if conn.execs != 4 {
		t.Fatalf("execs = %d, want 4 (each failed call re-collected)", conn.execs)
	}
	conn.fail = false
	if _, err := s.GetOverview(ctx, "sess-a"); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if conn.execs != 5 {
		t.Fatalf("execs = %d, want 5 after recovery", conn.execs)
	}
}

// 无主机归属的会话（本地终端等）按 sessionID 各自成 scope，互不影响。
func TestMonitorCacheSessionlessScopedBySession(t *testing.T) {
	s, conn, _ := newCachedMonitor(t)
	ctx := context.Background()

	// "anon-*" sessions are absent from the fake's host map → own scope.
	if _, err := s.GetPorts(ctx, "anon-1"); err != nil {
		t.Fatalf("anon-1: %v", err)
	}
	if _, err := s.GetPorts(ctx, "anon-2"); err != nil {
		t.Fatalf("anon-2: %v", err)
	}
	if conn.execs != 2 {
		t.Fatalf("execs = %d, want 2 (sessionless sessions do not share)", conn.execs)
	}
	if _, err := s.GetPorts(ctx, "anon-1"); err != nil {
		t.Fatalf("anon-1 again: %v", err)
	}
	if conn.execs != 2 {
		t.Fatalf("execs = %d, want 2 (local-1 hit its own cache)", conn.execs)
	}
}

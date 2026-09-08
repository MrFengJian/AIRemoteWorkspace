package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestSessionLogService(t *testing.T) *SessionLogService {
	t.Helper()
	// Manual dir + lenient cleanup: t.TempDir FAILS the test when its
	// RemoveAll hits a transient Windows file lock (AV/indexer) right after
	// a log file was written; freshly written logs must not flake tests.
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("sesslog-test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dataDirs := NewDataDirService(dir, filepath.Join(os.TempDir(), fmt.Sprintf("sesslog-ptr-%d", time.Now().UnixNano())), nil)
	return NewSessionLogService(dataDirs)
}

func TestSessionLogStartWriteStop(t *testing.T) {
	s := newTestSessionLogService(t)

	info, err := s.Start("sess-1", "web-prod")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !info.Enabled || info.Path == "" {
		t.Fatalf("unexpected info after start: %+v", info)
	}
	if host := filepath.Base(filepath.Dir(info.Path)); host != "web-prod" {
		t.Fatalf("log file not under host folder, got dir %q (file %s)", host, filepath.Base(info.Path))
	}

	s.Write("sess-1", []byte("hello "))
	s.Write("sess-1", []byte("world\n"))
	stopped := s.Stop("sess-1")
	if stopped.Enabled {
		t.Fatal("stopped info still reports enabled")
	}

	raw, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(raw)
	for _, want := range []string{"session log started", "hello world", "session log stopped"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q; got:\n%s", want, out)
		}
	}
}

func TestSessionLogDisabledWriteIsNoop(t *testing.T) {
	s := newTestSessionLogService(t)

	// Writes and stops for unknown sessions must be safe no-ops.
	s.Write("ghost", []byte("ignored"))
	if info := s.Stop("ghost"); info.Enabled || info.Path != "" {
		t.Fatalf("stop of unknown session returned %+v", info)
	}
	if info := s.Status("ghost"); info.Enabled {
		t.Fatal("unknown session reports enabled")
	}
}

func TestSessionLogStopIsIdempotent(t *testing.T) {
	s := newTestSessionLogService(t)
	if _, err := s.Start("sess-1", "db-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	s.Stop("sess-1")
	// A second stop (tab close racing the exit event) must not error or
	// report enabled again.
	if info := s.Stop("sess-1"); info.Enabled {
		t.Fatalf("double stop returned %+v", info)
	}
}

func TestSessionLogConcurrentSessionsSeparateFiles(t *testing.T) {
	s := newTestSessionLogService(t)
	a, err := s.Start("sess-a", "web-1")
	if err != nil {
		t.Fatalf("start a: %v", err)
	}
	b, err := s.Start("sess-b", "web-1")
	if err != nil {
		t.Fatalf("start b: %v", err)
	}
	if a.Path == b.Path {
		t.Fatal("two sessions got the same log file")
	}
	s.Write("sess-a", []byte("from-a\n"))
	s.Write("sess-b", []byte("from-b\n"))
	s.Stop("sess-a")
	s.Stop("sess-b")

	rawA, _ := os.ReadFile(a.Path)
	rawB, _ := os.ReadFile(b.Path)
	if !strings.Contains(string(rawA), "from-a") || strings.Contains(string(rawA), "from-b") {
		t.Fatalf("session A log mixed content:\n%s", rawA)
	}
	if !strings.Contains(string(rawB), "from-b") || strings.Contains(string(rawB), "from-a") {
		t.Fatalf("session B log mixed content:\n%s", rawB)
	}
}

func TestSanitizeLogSegment(t *testing.T) {
	cases := map[string]string{
		`web/prod:01`:            "web-prod-01",
		`a<b>c"d|e?f`:            "a-b-c-d-e-f",
		"  服务器 01.":              "服务器 01",
		"":                       "session",
		`..\..\evil`:             "-..-evil", // leading dots trimmed (Windows)
		strings.Repeat("x", 200): strings.Repeat("x", 80),
	}
	for in, want := range cases {
		if got := sanitizeLogSegment(in); got != want {
			t.Fatalf("sanitizeLogSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

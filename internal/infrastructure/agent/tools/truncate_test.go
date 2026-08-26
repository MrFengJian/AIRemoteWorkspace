package tools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapOutputShort(t *testing.T) {
	for _, s := range []string{"", "hello", strings.Repeat("x", maxToolOutput)} {
		if got := capOutputAt(s, maxToolOutput); got != s {
			t.Errorf("capOutputAt(len %d) modified output", len(s))
		}
	}
}

func TestCapOutputLong(t *testing.T) {
	s := strings.Repeat("a", maxToolOutput*3)
	got := capOutputAt(s, maxToolOutput)
	if !strings.Contains(got, "output truncated") {
		t.Fatalf("missing truncation marker")
	}
	if !strings.HasPrefix(got, "aaaa") {
		t.Errorf("head not preserved")
	}
	if !strings.HasSuffix(got, "aaaa") {
		t.Errorf("tail not preserved")
	}
	// Roughly bounded: head + tail + marker must stay near the cap.
	if len(got) > maxToolOutput+256 {
		t.Errorf("capped output too large: %d", len(got))
	}
}

func TestCapOutputRuneSafe(t *testing.T) {
	// Multi-byte runes everywhere — cuts must land on rune boundaries.
	s := strings.Repeat("你好世界", maxToolOutput/4+100)
	got := capOutputAt(s, maxToolOutput)
	if !utf8.ValidString(got) {
		t.Errorf("capped output contains invalid UTF-8")
	}
	if !strings.Contains(got, "output truncated") {
		t.Errorf("missing truncation marker")
	}
}

func TestCapOutputOmittedCount(t *testing.T) {
	s := strings.Repeat("b", maxToolOutput+1000)
	got := capOutputAt(s, maxToolOutput)
	want := "output truncated: 1000 bytes omitted"
	if !strings.Contains(got, want) {
		t.Errorf("omitted count mismatch: want marker %q in output", want)
	}
}

func TestCapOutputCustomLimit(t *testing.T) {
	// The limit is caller-configurable (global agent settings).
	s := strings.Repeat("c", 10_000)
	got := capOutputAt(s, 1024)
	if !strings.Contains(got, "output truncated") {
		t.Errorf("custom limit not applied")
	}
	if len(got) > 1024+128 {
		t.Errorf("custom limit exceeded: %d", len(got))
	}
	// Non-positive limit falls back to the package default.
	if capOutputAt("x", 0) != "x" {
		t.Errorf("fallback limit broke short input")
	}
}

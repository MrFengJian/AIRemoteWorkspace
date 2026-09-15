package application

import (
	"sync"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// collectRunner runs a login script against a scripted output timeline and
// returns everything the runner wrote. Chunks are fed with small delays so
// the await loop observes them like a real pump would.
func collectRunner(t *testing.T, steps []domain.LoginStep, chunks []string) []string {
	t.Helper()
	var mu sync.Mutex
	var sent []string
	r := NewLoginScriptRunner(steps, func(s string) error {
		mu.Lock()
		sent = append(sent, s)
		mu.Unlock()
		return nil
	})
	r.stepTimeout = 300 * time.Millisecond
	go r.Run()
	for _, c := range chunks {
		r.Feed([]byte(c))
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case <-r.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	return sent
}

func TestLoginScriptWaitsAndSends(t *testing.T) {
	sent := collectRunner(t,
		[]domain.LoginStep{
			{Expect: "Password:", Send: "s3cret"},
			{Expect: ">", Send: "enable"},
		},
		[]string{"login: admin\r\n", "Passw", "ord:\r\n", "> "},
	)
	// The Password prompt spans two chunks — the accumulator must match it.
	if len(sent) != 2 || sent[0] != "s3cret\r" || sent[1] != "enable\r" {
		t.Fatalf("unexpected writes: %q", sent)
	}
}

func TestLoginScriptStepWithoutExpectSendsImmediately(t *testing.T) {
	sent := collectRunner(t,
		[]domain.LoginStep{{Expect: "", Send: "cd /data"}},
		nil,
	)
	if len(sent) != 1 || sent[0] != "cd /data\r" {
		t.Fatalf("unexpected writes: %q", sent)
	}
}

func TestLoginScriptTimeoutAbortsSilently(t *testing.T) {
	sent := collectRunner(t,
		[]domain.LoginStep{
			{Expect: "never-appears", Send: "x"},
			{Expect: "", Send: "should-not-run"},
		},
		[]string{"unrelated output"},
	)
	if len(sent) != 0 {
		t.Fatalf("timed-out script must not write; got %q", sent)
	}
}

func TestLoginScriptEmptyStepsDoNothing(t *testing.T) {
	sent := collectRunner(t, nil, []string{"anything"})
	if len(sent) != 0 {
		t.Fatalf("empty script wrote %q", sent)
	}
	// A step with both fields empty is a no-op too.
	sent = collectRunner(t, []domain.LoginStep{{Expect: "", Send: ""}}, []string{"x"})
	if len(sent) != 0 {
		t.Fatalf("empty step wrote %q", sent)
	}
}

func TestLoginScriptStopAborts(t *testing.T) {
	r := NewLoginScriptRunner(
		[]domain.LoginStep{{Expect: "never", Send: "x"}},
		func(string) error { return nil },
	)
	r.stepTimeout = 10 * time.Second
	go r.Run()
	time.Sleep(20 * time.Millisecond)
	r.Stop()
	select {
	case <-r.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("Stop did not abort the runner")
	}
}

// TestLoginScriptLongOutputBeforeExpect ensures a long session cannot wedge
// the runner while waiting for a late Expect (the accumulator keeps only a
// tail, and the match still lands after it).
func TestLoginScriptLongOutputBeforeExpect(t *testing.T) {
	r := NewLoginScriptRunner(
		[]domain.LoginStep{{Expect: "END", Send: "ok"}},
		func(string) error { return nil },
	)
	r.stepTimeout = 2 * time.Second
	go r.Run()
	for i := 0; i < 100; i++ {
		r.Feed(make([]byte, 1024)) // 100KB of filler
		time.Sleep(2 * time.Millisecond)
	}
	r.Feed([]byte("END"))
	select {
	case <-r.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("runner hung on long output")
	}
}

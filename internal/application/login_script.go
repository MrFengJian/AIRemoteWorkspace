package application

import (
	"strings"
	"sync"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// LoginScriptRunner executes a host's login script (expect 序列) against a
// live PTY session: for each step, optionally wait until the Expect
// substring appears in the output stream, then write Send + carriage return.
//
// The runner consumes output chunks fed from the session's output pump
// (Feed) from a goroutine (Run); it never touches the terminal itself —
// writes go through the caller-supplied function (the connection manager's
// WriteStdin). A step whose Expect never appears within the timeout aborts
// the whole script SILENTLY: a login script is best-effort automation, and
// a mismatch must never disturb the interactive session.
type LoginScriptRunner struct {
	steps []domain.LoginStep
	write func(string) error
	// stepTimeout bounds each Expect wait (0 = default 15s).
	stepTimeout time.Duration

	chunks   chan []byte
	stopCh   chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// NewLoginScriptRunner wires a runner for the given steps. write delivers
// keystrokes to the session.
func NewLoginScriptRunner(steps []domain.LoginStep, write func(string) error) *LoginScriptRunner {
	return &LoginScriptRunner{
		steps:       steps,
		write:       write,
		stepTimeout: 15 * time.Second,
		chunks:      make(chan []byte, 256),
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Feed hands one output chunk to the runner. Never blocks the pump: when the
// buffer is full (a runaway Expect with chatty output), chunks are dropped —
// substring matching is best-effort by design.
func (r *LoginScriptRunner) Feed(chunk []byte) {
	select {
	case r.chunks <- chunk:
	default:
	}
}

// Stop aborts the runner (session closed / app shutdown).
func (r *LoginScriptRunner) Stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

// Done resolves when Run has exited.
func (r *LoginScriptRunner) Done() <-chan struct{} { return r.done }

// Run executes the script. Runs until the steps finish, the session stops,
// or an Expect wait times out.
func (r *LoginScriptRunner) Run() {
	defer close(r.done)

	var buf []byte
	for _, step := range r.steps {
		if step.Expect != "" && !r.await(step.Expect, &buf) {
			return // timed out or stopped mid-wait
		}
		if step.Send != "" {
			select {
			case <-r.stopCh:
				return
			default:
			}
			// A short inter-step pause lets the shell settle before the
			// next line lands (prompt redraw, MOTD, …).
			if !r.sleep(120 * time.Millisecond) {
				return
			}
			if err := r.write(step.Send + "\r"); err != nil {
				return
			}
		}
	}
}

// await waits until the Expect substring appears in the accumulated output
// (or the timeout / stop hits). Keeps only the tail of the buffer so a long
// session cannot grow it without bound. Returns whether it matched.
func (r *LoginScriptRunner) await(expect string, buf *[]byte) bool {
	deadline := time.Now().Add(r.stepTimeout)
	for {
		// Substring check over the accumulated stream.
		if strings.Contains(string(*buf), expect) {
			// Keep a tail: the next Expect could span a chunk boundary.
			const keep = 4096
			if len(*buf) > keep {
				*buf = (*buf)[len(*buf)-keep:]
			}
			return true
		}
		select {
		case <-r.stopCh:
			return false
		case chunk, ok := <-r.chunks:
			if !ok {
				return false
			}
			*buf = append(*buf, chunk...)
		case <-time.After(time.Until(deadline)):
			return false
		}
	}
}

// sleep waits for d or until stopped; reports whether to keep running.
func (r *LoginScriptRunner) sleep(d time.Duration) bool {
	select {
	case <-r.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

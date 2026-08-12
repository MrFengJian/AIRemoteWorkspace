package ssh

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// OutputHandler receives PTY output chunks. The ConnectionManager wires this
// to a Wails event emitter scoped per session.
type OutputHandler func(data []byte)

// PtySession wraps an interactive SSH shell session with a remote PTY.
//
// Output is streamed asynchronously to an OutputHandler; input is written
// synchronously via WriteStdin. Resize propagates WindowChange to the server.
type PtySession struct {
	session  *ssh.Session
	stdin    io.WriteCloser
	client   *Client
	onOutput OutputHandler

	wg        sync.WaitGroup
	closeOnce sync.Once
	closed    bool
	mu        sync.Mutex
}

// NewPtySession opens a PTY shell session on client.
func NewPtySession(client *Client, cols, rows int, onOutput OutputHandler) (*PtySession, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new ssh session: %w", err)
	}

	// Request a PTY. xterm-256color is the de-facto default; most servers
	// support it and $TERM-dependent tools (vim, htop) render correctly.
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Start the remote shell. After this, output begins streaming.
	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	ps := &PtySession{
		session:  sess,
		stdin:    stdin,
		client:   client,
		onOutput: onOutput,
	}

	// Stream stdout + stderr into the handler until both EOF.
	ps.wg.Add(2)
	go ps.pump(stdout)
	go ps.pump(stderr)
	return ps, nil
}

// pump copies r into the output handler in 4KB chunks. Reduces event count
// vs line-buffering while staying responsive for interactive typing.
func (ps *PtySession) pump(r io.Reader) {
	defer ps.wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 && ps.onOutput != nil {
			ps.onOutput(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			return
		}
	}
}

// WriteStdin sends user input (keystrokes) to the remote shell.
func (ps *PtySession) WriteStdin(data []byte) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return errors.New("session closed")
	}
	_, err := ps.stdin.Write(data)
	return err
}

// Resize tells the remote PTY the new window dimensions.
func (ps *PtySession) Resize(cols, rows int) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return errors.New("session closed")
	}
	return ps.session.WindowChange(rows, cols)
}

// Wait blocks until the remote shell exits and all pumps finish, returning the
// session's wait error (nil on clean exit).
func (ps *PtySession) Wait() error {
	err := ps.session.Wait()
	ps.wg.Wait()
	return err
}

// Close ends the session and stdin pipe. Safe to call multiple times.
func (ps *PtySession) Close() error {
	var err error
	ps.closeOnce.Do(func() {
		ps.mu.Lock()
		ps.closed = true
		ps.mu.Unlock()
		err = ps.session.Close()
		_ = ps.stdin.Close()
	})
	return err
}

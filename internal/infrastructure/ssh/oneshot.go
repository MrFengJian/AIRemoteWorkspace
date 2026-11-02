package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	"github.com/ai-remote/workspace/internal/domain"
)

// OneShotRunner executes one-shot commands on hosts WITHOUT an open terminal
// session (install-plan probing). It reuses the same dial chain as terminal
// sessions — host resolution → credentials → jump/proxy route → Dial with
// host-key verification — never a second execution channel (计划 §FR-009).
// Each Exec dials a short-lived connection and closes it.
type OneShotRunner struct {
	keyStore HostKeyStore
	routes   func(domain.Host) (*domain.SshRoute, error)
	resolve  func(hostID string) (domain.Host, domain.Credentials, error)
}

// NewOneShotRunner wires the runner: routes (may be nil = direct dials)
// resolves the jump/proxy path; resolve returns the host + its resolved
// credentials (including remembered secrets).
func NewOneShotRunner(
	keyStore HostKeyStore,
	routes func(domain.Host) (*domain.SshRoute, error),
	resolve func(hostID string) (domain.Host, domain.Credentials, error),
) *OneShotRunner {
	return &OneShotRunner{keyStore: keyStore, routes: routes, resolve: resolve}
}

// Exec runs cmd on the host and returns its combined output and exit code.
// stdin (may be nil) is piped to the command's stdin — the appdeploy layer
// feeds the script to `bash -l -s` through it (no quoting/escaping needed).
// err is nil for non-zero remote exits (the code carries the outcome); only
// transport/auth/context failures return an error (code -1).
func (r *OneShotRunner) Exec(ctx context.Context, hostID, cmd string, stdin []byte, timeout time.Duration) (string, int, error) {
	host, creds, err := r.resolve(hostID)
	if err != nil {
		return "", -1, fmt.Errorf("解析主机/凭据失败: %w", err)
	}
	var route *domain.SshRoute
	if r.routes != nil {
		route, err = r.routes(host)
		if err != nil {
			return "", -1, fmt.Errorf("解析连接路由失败: %w", err)
		}
	}
	client, err := Dial(ConnectOptions{
		HostID:   host.ID,
		Host:     host.Host,
		Port:     host.Port,
		Username: host.Username,
		Route:    route,
		Timeout:  15 * time.Second,
	}, Auth{
		Password:      creds.Password,
		KeyPath:       creds.KeyPath,
		KeyPassphrase: creds.KeyPassphrase,
		UseAgent:      creds.UseAgent,
	}, r.keyStore)
	if err != nil {
		return "", -1, fmt.Errorf("连接主机失败: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("打开执行通道失败: %w", err)
	}
	defer sess.Close()
	if len(stdin) > 0 {
		sess.Stdin = bytes.NewReader(stdin)
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// Remote-side bound: the command is killed even if the context is gone
	// before we notice (探测脚本自身有界，这里是双保险)。
	type execResult struct {
		out []byte
		err error
	}
	done := make(chan execResult, 1)
	go func() {
		out, err := sess.CombinedOutput(cmd)
		done <- execResult{out, err}
	}()

	deadline := time.After(timeout)
	select {
	case res := <-done:
		code := 0
		if res.err != nil {
			var exitErr *cryptossh.ExitError
			if errors.As(res.err, &exitErr) {
				code = exitErr.ExitStatus()
			} else {
				return string(res.out), -1, fmt.Errorf("远程执行失败: %w", res.err)
			}
		}
		return string(res.out), code, nil
	case <-deadline:
		_ = sess.Close()
		<-done
		return "", -1, fmt.Errorf("命令超时（%s）", timeout)
	case <-ctx.Done():
		_ = sess.Close()
		<-done
		return "", -1, ctx.Err()
	}
}

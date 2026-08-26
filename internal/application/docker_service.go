package application

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// DockerService collects Docker runtime state for the panel by driving the
// docker CLI on the host behind a session: over the session's SSH exec
// channel for remote tabs, or directly (no shell — argv only) for local
// terminal sessions with Docker Desktop / a local engine. Docker's own
// `--format '{{json .}}'` keeps output machine-readable without any SDK.
type DockerService struct {
	connect ConnectionManager
}

// NewDockerService wires a DockerService to the connection manager.
func NewDockerService(connect ConnectionManager) *DockerService {
	return &DockerService{connect: connect}
}

// Sentinel errors the frontend maps to specific hints (vs. a generic failure).
var (
	// ErrDockerUnavailable: the docker CLI is not installed on the target host.
	ErrDockerUnavailable = errors.New("docker cli not available")
	// ErrDockerDaemonDown: the CLI exists but cannot reach the daemon.
	ErrDockerDaemonDown = errors.New("docker daemon unreachable")
)

// runDocker executes `docker <args...>` against the session's host and
// translates common failure modes into the sentinel errors above.
func (s *DockerService) runDocker(ctx context.Context, sessionID string, args ...string) (string, error) {
	var out string
	var err error
	if isLocalSessionID(sessionID) {
		out, err = runLocalArgs(ctx, "docker", args...)
	} else {
		if s.connect == nil {
			return "", errors.New("docker: no connection manager")
		}
		cmd := "docker"
		for _, a := range args {
			cmd += " " + shellQuote(a)
		}
		out, err = s.connect.ExecInSessionCtx(ctx, sessionID, cmd)
	}
	if err != nil {
		if isDockerCLIMissing(out, err) {
			return "", ErrDockerUnavailable
		}
		if isDaemonDown(out) {
			return "", ErrDockerDaemonDown
		}
		return out, fmt.Errorf("docker: %w", err)
	}
	return out, nil
}

// isDockerCLIMissing recognizes the "executable not found" family across
// shells and platforms (bash "command not found", cmd "not recognized",
// zsh "command not found", Go's exec.Error on the local path).
func isDockerCLIMissing(out string, err error) bool {
	if err == nil {
		return false
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return true
	}
	o := strings.ToLower(out)
	return strings.Contains(o, "command not found") ||
		strings.Contains(o, "is not recognized") ||
		strings.Contains(o, "not found:docker") ||
		strings.Contains(o, "commandnotfound")
}

// isDaemonDown recognizes daemon connection failures (CLI present, engine
// not running / socket unreachable).
func isDaemonDown(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "cannot connect to the docker daemon") ||
		strings.Contains(o, "error during connect") ||
		strings.Contains(o, "the docker daemon is not running")
}

// runLocalArgs executes a command locally without any shell (argv only), so
// format strings like `{{json .}}` need no platform-specific quoting.
func runLocalArgs(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// shellQuote wraps s in single quotes for remote shell consumption, escaping
// embedded quotes POSIX-style.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// GetInfo returns the Docker overview (server version + counters).
func (s *DockerService) GetInfo(ctx context.Context, sessionID string) (domain.DockerInfo, error) {
	verOut, err := s.runDocker(ctx, sessionID, "version", "--format", "{{json .}}")
	if err != nil {
		return domain.DockerInfo{}, err
	}
	infoOut, err := s.runDocker(ctx, sessionID, "info", "--format", "{{json .}}")
	if err != nil {
		// `docker version` succeeded (CLI + daemon reachable) — treat info
		// quirks as non-fatal and keep whatever version gave us.
		infoOut = ""
	}
	return parseDockerInfo(verOut, infoOut), nil
}

// ListContainers returns the container list (all=false: running only).
func (s *DockerService) ListContainers(ctx context.Context, sessionID string, all bool) ([]domain.DockerContainer, error) {
	args := []string{"ps", "--format", "{{json .}}"}
	if all {
		args = append(args, "-a")
	}
	out, err := s.runDocker(ctx, sessionID, args...)
	if err != nil {
		return nil, err
	}
	return parseDockerContainers(out), nil
}

// GetContainerStats returns one-shot resource usage per container.
func (s *DockerService) GetContainerStats(ctx context.Context, sessionID string) ([]domain.DockerContainerStats, error) {
	out, err := s.runDocker(ctx, sessionID, "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	return parseDockerStats(out), nil
}

// ListImages returns the local image list.
func (s *DockerService) ListImages(ctx context.Context, sessionID string) ([]domain.DockerImage, error) {
	out, err := s.runDocker(ctx, sessionID, "images", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	return parseDockerImages(out), nil
}

// ListNetworks returns the docker networks (`docker network ls`).
func (s *DockerService) ListNetworks(ctx context.Context, sessionID string) ([]domain.DockerNetwork, error) {
	out, err := s.runDocker(ctx, sessionID, "network", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	return parseDockerNetworks(out), nil
}

// InspectNetwork returns one network's detail (IPAM pools + attached
// containers). `network` may be a name or ID.
func (s *DockerService) InspectNetwork(ctx context.Context, sessionID, network string) (domain.DockerNetworkDetail, error) {
	if strings.TrimSpace(network) == "" {
		return domain.DockerNetworkDetail{}, errors.New("docker: empty network reference")
	}
	out, err := s.runDocker(ctx, sessionID, "network", "inspect", "--format", "{{json .}}", network)
	if err != nil {
		return domain.DockerNetworkDetail{}, err
	}
	detail, ok := parseDockerNetworkInspect(out)
	if !ok {
		return domain.DockerNetworkDetail{}, errors.New("docker: network detail not parseable")
	}
	return detail, nil
}

// dockerLogTailLimits bounds the log window the panel may request.
const (
	dockerLogTailMin = 10
	dockerLogTailMax = 1000
	// dockerLogByteCap bounds the payload sent to the UI (--tail already
	// bounds by lines; this is the belt-and-braces byte ceiling).
	dockerLogByteCap = 256 << 10
)

// GetLogs returns the last `tail` log lines (with docker timestamps) of one
// container. The container reference is shell-quoted; a name, ID prefix, or
// compose-style name works.
func (s *DockerService) GetLogs(ctx context.Context, sessionID, container string, tail int) (string, error) {
	if strings.TrimSpace(container) == "" {
		return "", errors.New("docker: empty container reference")
	}
	if tail < dockerLogTailMin {
		tail = 100
	}
	if tail > dockerLogTailMax {
		tail = dockerLogTailMax
	}
	out, err := s.runDocker(ctx, sessionID, "logs", "--tail", fmt.Sprintf("%d", tail), "-t", container)
	if err != nil {
		return "", err
	}
	return capString(out, dockerLogByteCap), nil
}

// dockerContainerActions is the closed allowlist the panel may trigger —
// ContainerAction never passes an arbitrary verb through to the CLI.
var dockerContainerActions = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"pause": true, "unpause": true, "kill": true,
}

// ContainerAction performs a lifecycle action on one container. Actions are
// restricted to the allowlist above; the container reference is quoted.
func (s *DockerService) ContainerAction(ctx context.Context, sessionID, container, action string) (string, error) {
	if !dockerContainerActions[action] {
		return "", fmt.Errorf("docker: action %q not allowed", action)
	}
	if strings.TrimSpace(container) == "" {
		return "", errors.New("docker: empty container reference")
	}
	return s.runDocker(ctx, sessionID, action, container)
}

// capString truncates s to at most max bytes (head-preserved, marker in the
// middle). Rune-safe at the tail cut.
func capString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - 64 // leave room for the marker
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n…[%d bytes truncated]…\n", len(s)-cut)
}

func isRuneStart(b byte) bool {
	return b&0xC0 != 0x80
}

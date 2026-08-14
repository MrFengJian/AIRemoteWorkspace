package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// localExec runs a command on the local machine. The permission tier is
// derived from the command content — destructive commands (rm, shutdown, dd…)
// require explicit user approval before execution.
func (ss *sessionToolSet) localExec(ctx context.Context, a localExecArgs) (string, error) {
	perm := classifyCommand(a.Command)
	if err := ss.gateCheck(ctx, "local_exec", perm, a); err != nil {
		return "", err
	}
	return runLocal(ctx, a.Command)
}

// localReadFile reads a local file (READ permission).
func (ss *sessionToolSet) localReadFile(ctx context.Context, a readPathArgs) (string, error) {
	if err := ss.gateCheck(ctx, "local_read_file", domain.PermissionRead, a); err != nil {
		return "", err
	}
	data, err := os.ReadFile(a.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", a.Path, err)
	}
	return string(data), nil
}

// sshExec runs a command on the remote host. The permission tier is derived
// from the command content — dangerous commands (rm, shutdown, iptables,
// docker prune…) require explicit user approval before execution.
func (ss *sessionToolSet) sshExec(ctx context.Context, a sshExecArgs) (string, error) {
	perm := classifyCommand(a.Command)
	if err := ss.gateCheck(ctx, "ssh_exec", perm, a); err != nil {
		return "", err
	}
	if ss.ssh == nil {
		return "", fmt.Errorf("ssh manager not available")
	}
	out, err := ss.ssh.ExecInSession(ss.sessionID, a.Command)
	if err != nil {
		return out, fmt.Errorf("ssh exec: %w", err)
	}
	return out, nil
}

// sshReadFile reads a remote file via SFTP (READ).
func (ss *sessionToolSet) sshReadFile(ctx context.Context, a sshReadFileArgs) (string, error) {
	if err := ss.gateCheck(ctx, "ssh_read_file", domain.PermissionRead, a); err != nil {
		return "", err
	}
	host, creds, err := ss.resolver(ss.sessionID)
	if err != nil {
		return "", err
	}
	data, err := ss.sftp.DownloadFile(host, creds, a.Path, nil)
	if err != nil {
		return "", fmt.Errorf("sftp read %s: %w", a.Path, err)
	}
	return string(data), nil
}

// sshWriteFile writes content to a remote file via SFTP (WRITE — needs approval).
func (ss *sessionToolSet) sshWriteFile(ctx context.Context, a sshWriteFileArgs) (string, error) {
	if err := ss.gateCheck(ctx, "ssh_write_file", domain.PermissionWrite, a); err != nil {
		return "", err
	}
	host, creds, err := ss.resolver(ss.sessionID)
	if err != nil {
		return "", err
	}
	if err := ss.sftp.UploadFile(host, creds, a.Path, []byte(a.Content), nil); err != nil {
		return "", fmt.Errorf("sftp write %s: %w", a.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), nil
}

// upload copies a local file to the remote host (WRITE — needs approval).
func (ss *sessionToolSet) upload(ctx context.Context, a uploadArgs) (string, error) {
	if err := ss.gateCheck(ctx, "upload", domain.PermissionWrite, a); err != nil {
		return "", err
	}
	data, err := os.ReadFile(a.LocalPath)
	if err != nil {
		return "", fmt.Errorf("read local %s: %w", a.LocalPath, err)
	}
	host, creds, err := ss.resolver(ss.sessionID)
	if err != nil {
		return "", err
	}
	if err := ss.sftp.UploadFile(host, creds, a.RemotePath, data, nil); err != nil {
		return "", fmt.Errorf("upload to %s: %w", a.RemotePath, err)
	}
	return fmt.Sprintf("uploaded %s → %s (%d bytes)", a.LocalPath, a.RemotePath, len(data)), nil
}

// download copies a remote file to the local machine (READ).
func (ss *sessionToolSet) download(ctx context.Context, a downloadArgs) (string, error) {
	if err := ss.gateCheck(ctx, "download", domain.PermissionRead, a); err != nil {
		return "", err
	}
	host, creds, err := ss.resolver(ss.sessionID)
	if err != nil {
		return "", err
	}
	data, err := ss.sftp.DownloadFile(host, creds, a.RemotePath, nil)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", a.RemotePath, err)
	}
	if err := os.WriteFile(a.LocalPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write local %s: %w", a.LocalPath, err)
	}
	return fmt.Sprintf("downloaded %s → %s (%d bytes)", a.RemotePath, a.LocalPath, len(data)), nil
}

// runLocal executes a shell command on the local machine with a timeout.
func runLocal(ctx context.Context, command string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	if isWindows() {
		cmd = exec.CommandContext(cctx, "cmd", "/c", command)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isWindows() bool {
	// runtime.GOOS == "windows" — kept simple without importing runtime here
	// since the local exec path is secondary to the remote SSH tools.
	return os.PathSeparator == '\\'
}

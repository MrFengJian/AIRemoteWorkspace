// Package tools implements the Agent's Tool set (AGENT.md §13). Each tool is
// an eino InvokableTool built via utils.InferTool, wired to the SSH/SFTP/local
// exec infrastructure. A PermissionGate runs before every call.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// PermissionGate is called before a tool executes. For READ it returns nil
// (auto-approve). For WRITE/DANGEROUS it blocks until the user approves or
// denies — returning nil to proceed, or an error to abort the tool call.
//
// The implementation lives in the application layer (AgentRuntime) and bridges
// to the frontend via Wails events.
type PermissionGate interface {
	// Check asks for approval. toolName/perm describe the action; argsJSON is
	// the raw arguments the LLM supplied (for display). Returns nil on approval.
	Check(ctx context.Context, sessionID, toolName string, perm domain.Permission, argsJSON string) error
}

// SftpFileOps is the subset of the SFTP manager the file tools need.
type SftpFileOps interface {
	DownloadFile(host domain.Host, creds domain.Credentials, remotePath string) ([]byte, error)
	UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte) error
}

// Deps bundles the infrastructure a ToolSet needs.
type Deps struct {
	SSH  *ssh.Manager
	SFTP SftpFileOps
}

// CredsResolver returns credentials for a session's host (so SFTP tools can
// reuse the OS-vault-remembered password). Same as application.ResolveCredentials.
type CredsResolver func(sessionID string) (domain.Host, domain.Credentials, error)

// ToolSet holds the built eino tools for one agent session.
type ToolSet struct {
	tools    []tool.BaseTool
	resolver CredsResolver
	ssh      *ssh.Manager
	sftp     SftpFileOps
	gate     PermissionGate
}

// NewToolSet builds the 7 tools, each capturing sessionID at call time.
func NewToolSet(deps Deps, resolver CredsResolver, gate PermissionGate) (*ToolSet, error) {
	ts := &ToolSet{
		resolver: resolver,
		ssh:      deps.SSH,
		sftp:     deps.SFTP,
		gate:     gate,
	}
	if err := ts.build(); err != nil {
		return nil, err
	}
	return ts, nil
}

// Tools returns the eino tool list for the agent config.
func (ts *ToolSet) Tools() []tool.BaseTool { return ts.tools }

// sessionCtx carries the session ID through the tool call. We inject it via
// the context passed to the agent's Stream call — but eino's InferTool closure
// doesn't receive the agent ctx at the tool level in all versions. To stay
// robust we set the sessionID on the ToolSet per chat invocation.
func (ts *ToolSet) withSession(sessionID string) *sessionToolSet {
	return &sessionToolSet{ToolSet: ts, sessionID: sessionID}
}

// sessionToolSet is a per-chat snapshot with a fixed sessionID.
type sessionToolSet struct {
	*ToolSet
	sessionID string
}

// BuildForSession returns a fresh tool list bound to sessionID, for one chat.
func (ts *ToolSet) BuildForSession(sessionID string) ([]tool.BaseTool, error) {
	ss := ts.withSession(sessionID)
	return ss.build()
}

// gateCheck is the shorthand every tool calls before executing.
func (ss *sessionToolSet) gateCheck(ctx context.Context, name string, perm domain.Permission, args any) error {
	if ss.gate == nil {
		return nil
	}
	argsJSON, _ := json.Marshal(args)
	return ss.gate.Check(ctx, ss.sessionID, name, perm, string(argsJSON))
}

// build constructs all 7 tools for this sessionToolSet.
func (ss *sessionToolSet) build() ([]tool.BaseTool, error) {
	var built []tool.BaseTool

	// 1. local_exec
	t1, err := utils.InferTool(
		"local_exec",
		"Execute a shell command on the LOCAL machine and return combined stdout+stderr. Use for local diagnostics.",
		ss.localExec,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_exec: %w", err)
	}
	built = append(built, t1)

	// 2. local_read_file
	t2, err := utils.InferTool(
		"local_read_file",
		"Read a file from the LOCAL machine and return its contents as text.",
		ss.localReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_read_file: %w", err)
	}
	built = append(built, t2)

	// 3. ssh_exec
	t3, err := utils.InferTool(
		"ssh_exec",
		"Execute a shell command on the REMOTE host (the currently connected SSH session) and return combined stdout+stderr. Use for remote diagnostics like 'uptime', 'df -h', 'free -m', 'ps aux'.",
		ss.sshExec,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_exec: %w", err)
	}
	built = append(built, t3)

	// 4. ssh_read_file
	t4, err := utils.InferTool(
		"ssh_read_file",
		"Read a file from the REMOTE host and return its contents as text.",
		ss.sshReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_read_file: %w", err)
	}
	built = append(built, t4)

	// 5. ssh_write_file
	t5, err := utils.InferTool(
		"ssh_write_file",
		"Write text content to a file on the REMOTE host. Overwrites if the file exists. Requires user approval.",
		ss.sshWriteFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_write_file: %w", err)
	}
	built = append(built, t5)

	// 6. upload
	t6, err := utils.InferTool(
		"upload",
		"Upload a LOCAL file to the REMOTE host. Requires user approval.",
		ss.upload,
	)
	if err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	built = append(built, t6)

	// 7. download
	t7, err := utils.InferTool(
		"download",
		"Download a REMOTE file to the LOCAL machine.",
		ss.download,
	)
	if err != nil {
		return nil, fmt.Errorf("build download: %w", err)
	}
	built = append(built, t7)

	return built, nil
}

// build on the outer ToolSet builds tools without a session — used only to
// validate construction at startup. Real usage goes through BuildForSession.
func (ts *ToolSet) build() error {
	_, err := ts.withSession("").build()
	return err
}

// --- argument structs (jsonschema inferred from field tags) ---

type localExecArgs struct {
	Command string `json:"command" jsonschema:"description=shell command to run locally,required"`
}

type readPathArgs struct {
	Path string `json:"path" jsonschema:"description=absolute file path,required"`
}

type sshExecArgs struct {
	Command string `json:"command" jsonschema:"description=shell command to run on the remote host,required"`
}

type sshReadFileArgs struct {
	Path string `json:"path" jsonschema:"description=remote file path,required"`
}

type sshWriteFileArgs struct {
	Path    string `json:"path" jsonschema:"description=remote file path,required"`
	Content string `json:"content" jsonschema:"description=file contents to write,required"`
}

type uploadArgs struct {
	LocalPath  string `json:"localPath" jsonschema:"description=local file path,required"`
	RemotePath string `json:"remotePath" jsonschema:"description=remote destination path,required"`
}

type downloadArgs struct {
	RemotePath string `json:"remotePath" jsonschema:"description=remote file path,required"`
	LocalPath  string `json:"localPath" jsonschema:"description=local destination path,required"`
}

// ensure schema import is used (for tool type alignment).
var _ = schema.ToolInfo{}

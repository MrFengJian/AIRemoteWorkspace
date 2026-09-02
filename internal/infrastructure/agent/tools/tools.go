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

	"github.com/ai-remote/workspace/internal/application"
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

// SkillBackend lists and loads agent skills — SKILL.md files under the skills
// root, mirroring eino's adk/middlewares/skill Backend contract (List/Get).
// May be nil: the `skill` tool is then simply not offered to the model.
type SkillBackend interface {
	ListSkills() ([]domain.Skill, error)
	GetSkill(name string) (domain.Skill, error)
}

// SftpFileOps is the subset of the SFTP manager the file tools need. Progress
// callbacks exist for the UI transfer path; tools pass nil.
type SftpFileOps interface {
	DownloadFile(host domain.Host, creds domain.Credentials, remotePath string, progress application.SftpProgress) ([]byte, error)
	UploadFile(host domain.Host, creds domain.Credentials, remotePath string, data []byte, progress application.SftpProgress) error
}

// Deps bundles the infrastructure a ToolSet needs.
type Deps struct {
	SSH  *ssh.Manager
	SFTP SftpFileOps
	// OutputLimitBytes bounds a single tool result fed back to the model
	// (0 = package default, 64KB). Fed from the global agent settings.
	OutputLimitBytes int
	// Skills (may be nil) enables the `skill` tool — the model loads a
	// skill's instructions by name, eino skill-middleware style.
	Skills SkillBackend
}

// CredsResolver returns credentials for a session's host (so SFTP tools can
// reuse the OS-vault-remembered password). Same as application.ResolveCredentials.
type CredsResolver func(sessionID string) (domain.Host, domain.Credentials, error)

// ToolSet holds the built eino tools for one agent session.
type ToolSet struct {
	tools       []tool.BaseTool
	resolver    CredsResolver
	ssh         *ssh.Manager
	sftp        SftpFileOps
	gate        PermissionGate
	observer    RunObserver
	outputLimit int
	skills      SkillBackend
}

// NewToolSet builds the 7 tools, each capturing sessionID at call time. The
// observer (may be nil) receives per-invocation start/end events.
func NewToolSet(deps Deps, resolver CredsResolver, gate PermissionGate, observer RunObserver) (*ToolSet, error) {
	ts := &ToolSet{
		resolver:    resolver,
		ssh:         deps.SSH,
		sftp:        deps.SFTP,
		gate:        gate,
		observer:    observer,
		outputLimit: deps.OutputLimitBytes,
		skills:      deps.Skills,
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

// BuildLocalForSession returns only the two LOCAL tools, for agent chats on
// a local terminal session (no SSH host behind it). Same observer wiring.
func (ts *ToolSet) BuildLocalForSession(sessionID string) ([]tool.BaseTool, error) {
	ss := ts.withSession(sessionID)
	var built []tool.BaseTool

	t1, err := utils.InferTool(
		"local_exec",
		"Execute a shell command on the LOCAL machine and return combined stdout+stderr. Use for local diagnostics.",
		ss.localExec,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_exec: %w", err)
	}
	built = append(built, observe(t1, ss.sessionID, "local_exec", ss.observer))

	t2, err := utils.InferTool(
		"local_read_file",
		"Read a file from the LOCAL machine and return its contents as text.",
		ss.localReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_read_file: %w", err)
	}
	built = append(built, observe(t2, ss.sessionID, "local_read_file", ss.observer))

	// The skill tool is host-agnostic — available on local sessions too.
	if sk, err := ss.buildSkillTool(); err != nil {
		return nil, err
	} else if sk != nil {
		built = append(built, sk)
	}

	return built, nil
}

// gateCheck is the shorthand every tool calls before executing.
func (ss *sessionToolSet) gateCheck(ctx context.Context, name string, perm domain.Permission, args any) error {
	if ss.gate == nil {
		return nil
	}
	argsJSON, _ := json.Marshal(args)
	return ss.gate.Check(ctx, ss.sessionID, name, perm, string(argsJSON))
}

// capOutput bounds a tool result to the ToolSet's configured limit.
func (ss *sessionToolSet) capOutput(s string) string {
	return capOutputAt(s, ss.outputLimit)
}

// buildSkillTool builds the `skill` tool when a skill backend is wired —
// the model loads a skill's full instructions by name (eino skill
// middleware, inline mode). Returns nil when no backend is configured.
func (ss *sessionToolSet) buildSkillTool() (tool.BaseTool, error) {
	if ss.skills == nil {
		return nil, nil
	}
	skills, err := ss.skills.ListSkills()
	if err != nil {
		skills = nil // listing failed — still offer the tool, generic description
	}
	desc := "Load the full instructions of an available skill into this conversation. " +
		"Use it whenever the user asks to follow a skill, or before performing work covered by one.\nAvailable skills:"
	for _, s := range skills {
		desc += fmt.Sprintf("\n- %s: %s", s.Name, s.Description)
	}
	if len(skills) == 0 {
		desc += "\n- (none)"
	}
	t, err := utils.InferTool(
		"skill",
		desc,
		ss.loadSkill,
	)
	if err != nil {
		return nil, fmt.Errorf("build skill: %w", err)
	}
	return observe(t, ss.sessionID, "skill", ss.observer), nil
}

// loadSkill resolves a skill name to its markdown instructions (READ tier —
// skills live on the user's machine and only extend the conversation).
func (ss *sessionToolSet) loadSkill(ctx context.Context, a skillArgs) (string, error) {
	if ss.skills == nil {
		return "", fmt.Errorf("skills not available")
	}
	if err := ss.gateCheck(ctx, "skill", domain.PermissionRead, a); err != nil {
		return "", err
	}
	sk, err := ss.skills.GetSkill(a.Skill)
	if err != nil {
		return "", err
	}
	return sk.Content, nil
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
	built = append(built, observe(t1, ss.sessionID, "local_exec", ss.observer))

	// 2. local_read_file
	t2, err := utils.InferTool(
		"local_read_file",
		"Read a file from the LOCAL machine and return its contents as text.",
		ss.localReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_read_file: %w", err)
	}
	built = append(built, observe(t2, ss.sessionID, "local_read_file", ss.observer))

	// 3. ssh_exec
	t3, err := utils.InferTool(
		"ssh_exec",
		"Execute a shell command on the REMOTE host (the currently connected SSH session) and return combined stdout+stderr. Use for remote diagnostics like 'uptime', 'df -h', 'free -m', 'ps aux'.",
		ss.sshExec,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_exec: %w", err)
	}
	built = append(built, observe(t3, ss.sessionID, "ssh_exec", ss.observer))

	// 4. ssh_read_file
	t4, err := utils.InferTool(
		"ssh_read_file",
		"Read a file from the REMOTE host and return its contents as text.",
		ss.sshReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_read_file: %w", err)
	}
	built = append(built, observe(t4, ss.sessionID, "ssh_read_file", ss.observer))

	// 5. ssh_write_file
	t5, err := utils.InferTool(
		"ssh_write_file",
		"Write text content to a file on the REMOTE host. Overwrites if the file exists. Requires user approval.",
		ss.sshWriteFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build ssh_write_file: %w", err)
	}
	built = append(built, observe(t5, ss.sessionID, "ssh_write_file", ss.observer))

	// 6. upload
	t6, err := utils.InferTool(
		"upload",
		"Upload a LOCAL file to the REMOTE host. Requires user approval.",
		ss.upload,
	)
	if err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	built = append(built, observe(t6, ss.sessionID, "upload", ss.observer))

	// 7. download
	t7, err := utils.InferTool(
		"download",
		"Download a REMOTE file to the LOCAL machine.",
		ss.download,
	)
	if err != nil {
		return nil, fmt.Errorf("build download: %w", err)
	}
	built = append(built, observe(t7, ss.sessionID, "download", ss.observer))

	// 8. skill (only when a skill backend is wired)
	if sk, err := ss.buildSkillTool(); err != nil {
		return nil, err
	} else if sk != nil {
		built = append(built, sk)
	}

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

type skillArgs struct {
	Skill string `json:"skill" jsonschema:"description=skill name (see the tool description list),required"`
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

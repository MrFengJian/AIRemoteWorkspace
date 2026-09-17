// Package tools implements the Agent's Tool set (AGENT.md §13). Each tool is
// an eino InvokableTool built via utils.InferTool, wired to the SSH/SFTP/local
// exec infrastructure. A PermissionGate runs before every call.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	// ReadSkillFile returns one bundled file of a directory-form skill pack
	// (path relative to the skill directory, slash-separated).
	ReadSkillFile(name, path string) (string, error)
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

// allowed filters a built tool by an expert's allowlist: nil/empty = keep
// everything; otherwise the name must be listed. The skill tool bypasses the
// filter (it is the persona's knowledge channel, not a host capability).
func allowedTool(allowed map[string]bool, name string) bool {
	return len(allowed) == 0 || name == "skill" || allowed[name]
}

// BuildForSession returns a fresh tool list bound to sessionID, for one chat.
// allowed (may be nil) scopes the toolset to an expert's allowlist.
func (ts *ToolSet) BuildForSession(sessionID string, allowed map[string]bool) ([]tool.BaseTool, error) {
	ss := ts.withSession(sessionID)
	built, err := ss.build()
	if err != nil {
		return nil, err
	}
	out := make([]tool.BaseTool, 0, len(built))
	for _, e := range built {
		if allowedTool(allowed, e.name) {
			out = append(out, e.tool)
		}
	}
	return out, nil
}

// BuildLocalForSession returns only the two LOCAL tools, for agent chats on
// a local terminal session (no SSH host behind it). Same observer wiring and
// expert allowlist.
func (ts *ToolSet) BuildLocalForSession(sessionID string, allowed map[string]bool) ([]tool.BaseTool, error) {
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
	if allowedTool(allowed, "local_exec") {
		built = append(built, observe(t1, ss.sessionID, "local_exec", ss.observer))
	}

	t2, err := utils.InferTool(
		"local_read_file",
		"Read a file from the LOCAL machine and return its contents as text.",
		ss.localReadFile,
	)
	if err != nil {
		return nil, fmt.Errorf("build local_read_file: %w", err)
	}
	if allowedTool(allowed, "local_read_file") {
		built = append(built, observe(t2, ss.sessionID, "local_read_file", ss.observer))
	}

	// The skill tool is host-agnostic — available on local sessions too.
	if allowedTool(allowed, "skill") {
		if sk, err := ss.buildSkillTool(); err != nil {
			return nil, err
		} else if sk != nil {
			built = append(built, sk)
		}
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
		"Use it whenever the user asks to follow a skill, or before performing work covered by one. " +
		"Skills may bundle extra files (scripts, references) listed at the end of their instructions — " +
		"pass such a path via the `path` argument to read its content.\nAvailable skills:"
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
// skills live on the user's machine and only extend the conversation). With
// `path` set it instead returns one bundled file of the pack (directory-form
// skills: scripts, references), same READ tier.
func (ss *sessionToolSet) loadSkill(ctx context.Context, a skillArgs) (string, error) {
	if ss.skills == nil {
		return "", fmt.Errorf("skills not available")
	}
	if err := ss.gateCheck(ctx, "skill", domain.PermissionRead, a); err != nil {
		return "", err
	}
	if a.Path != "" {
		return ss.skills.ReadSkillFile(a.Skill, a.Path)
	}
	sk, err := ss.skills.GetSkill(a.Skill)
	if err != nil {
		return "", err
	}
	if len(sk.Files) == 0 {
		return sk.Content, nil
	}
	return sk.Content + "\n\n---\nBundled files in this skill pack (read one with the skill tool, " +
		"passing `skill` plus its `path`):\n" + strings.Join(sk.Files, "\n"), nil
}

// build constructs all tools for this sessionToolSet as (name, tool) pairs —
// the names drive the expert allowlist filter in BuildForSession.
func (ss *sessionToolSet) build() ([]builtTool, error) {
	var built []builtTool
	var err error
	var bt builtTool

	// 1. local_exec
	bt, err = inferBuiltin(ss, "local_exec",
		"Execute a shell command on the LOCAL machine and return combined stdout+stderr. Use for local diagnostics.",
		ss.localExec)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 2. local_read_file
	bt, err = inferBuiltin(ss, "local_read_file",
		"Read a file from the LOCAL machine and return its contents as text.",
		ss.localReadFile)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 3. ssh_exec
	bt, err = inferBuiltin(ss, "ssh_exec",
		"Execute a shell command on the REMOTE host (the currently connected SSH session) and return combined stdout+stderr. Use for remote diagnostics like 'uptime', 'df -h', 'free -m', 'ps aux'.",
		ss.sshExec)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 4. ssh_read_file
	bt, err = inferBuiltin(ss, "ssh_read_file",
		"Read a file from the REMOTE host and return its contents as text.",
		ss.sshReadFile)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 5. ssh_write_file
	bt, err = inferBuiltin(ss, "ssh_write_file",
		"Write text content to a file on the REMOTE host. Overwrites if the file exists. Requires user approval.",
		ss.sshWriteFile)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 6. upload
	bt, err = inferBuiltin(ss, "upload",
		"Upload a LOCAL file to the REMOTE host. Requires user approval.",
		ss.upload)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 7. download
	bt, err = inferBuiltin(ss, "download",
		"Download a REMOTE file to the LOCAL machine.",
		ss.download)
	if err != nil {
		return nil, err
	}
	built = append(built, bt)

	// 8. skill (only when a skill backend is wired)
	if sk, err := ss.buildSkillTool(); err != nil {
		return nil, err
	} else if sk != nil {
		built = append(built, builtTool{name: "skill", tool: sk})
	}

	return built, nil
}

// builtTool pairs a tool with its registration name.
type builtTool struct {
	name string
	tool tool.BaseTool
}

// inferBuiltin infers one tool's schema and wraps it with the observer. T is
// inferred from the tool function's args struct — it must stay a type
// parameter here (a plain `any` would break InferTool's inference).
func inferBuiltin[T any](ss *sessionToolSet, name, desc string, fn func(context.Context, T) (string, error)) (builtTool, error) {
	t, err := utils.InferTool(name, desc, fn)
	if err != nil {
		return builtTool{}, fmt.Errorf("build %s: %w", name, err)
	}
	return builtTool{name: name, tool: observe(t, ss.sessionID, name, ss.observer)}, nil
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
	// Path (optional) reads one bundled file of the pack instead of the
	// instructions — directory-form skills ship scripts/references.
	Path string `json:"path,omitempty" jsonschema:"description=bundled file path to read instead of the instructions (relative to the skill, e.g. scripts/foo.py)"`
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

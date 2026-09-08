package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	cryptossh "golang.org/x/crypto/ssh"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent/tools"
)

// registerTools wires the eight MCP tools (ROADMAP Phase 6) to the shared
// infrastructure. Permission mapping mirrors the built-in agent:
//
//	READ (auto)      list_hosts · connect_host · read_file · download · system_info
//	command-derived  exec_command (tools.ClassifyCommand)
//	WRITE (approval) write_file · upload
//
// Handlers return plain text content (LLM-facing, same as the agent tools);
// errors become MCP tool errors via the SDK.
func (s *Server) registerTools() {
	// 1. list_hosts — discover targets (READ).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_hosts",
		Description: "List the SSH hosts configured in AI Remote Workspace. Returns id, name, address, username, auth type, group, OS and tags. Use a returned id (or exact name) to target the other tools.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		if err := s.gateCheck(ctx, domain.Host{ID: "mcp", Name: "workspace"}, "list_hosts", domain.PermissionRead, nil); err != nil {
			return nil, nil, err
		}
		hosts, err := s.deps.Hosts.List()
		if err != nil {
			return nil, nil, fmt.Errorf("list hosts: %w", err)
		}
		if len(hosts) == 0 {
			return textResult("No hosts configured. Add one in the app first (Hosts view)."), nil, nil
		}
		var b strings.Builder
		for _, h := range hosts {
			fmt.Fprintf(&b, "- %s  %s@%s:%d  id=%s auth=%s", h.Name, h.Username, h.Host, h.Port, h.ID, h.AuthType)
			if h.Group != "" {
				fmt.Fprintf(&b, " group=%s", h.Group)
			}
			if h.OS != "" {
				fmt.Fprintf(&b, " os=%s", h.OS)
			}
			if len(h.Tags) > 0 {
				fmt.Fprintf(&b, " tags=%s", strings.Join(h.Tags, ","))
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil, nil
	})

	// 2. connect_host — verify connectivity / pre-open a session (READ).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "connect_host",
		Description: "Open a connection to a host and verify authentication works. Prefer calling this once before a series of exec/read/write calls. The connection is reused by later calls and closed when the MCP server stops.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a hostArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		if err := s.gateCheck(ctx, host, "connect_host", domain.PermissionRead, a); err != nil {
			return nil, nil, err
		}
		sid, err := s.ensureSession(ctx, host, creds)
		if err != nil {
			return nil, nil, fmt.Errorf("connect %s: %w", host.Name, err)
		}
		return textResult(fmt.Sprintf("connected to %s (%s@%s:%d), session %s",
			host.Name, host.Username, host.Host, host.Port, shortID(sid))), nil, nil
	})

	// 3. exec_command — run a command on a host (tier from the command).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "exec_command",
		Description: "Execute a shell command on a remote host and return combined stdout+stderr. Use for diagnostics like 'uptime', 'df -h', 'free -m', 'systemctl status'. Destructive commands require user approval in the app. Connects automatically if needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a execArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		perm := tools.ClassifyCommand(a.Command)
		if err := s.gateCheck(ctx, host, "exec_command", perm, a); err != nil {
			return nil, nil, err
		}
		sid, err := s.ensureSession(ctx, host, creds)
		if err != nil {
			return nil, nil, fmt.Errorf("connect %s: %w", host.Name, err)
		}
		cctx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		out, err := s.deps.SSH.ExecInSessionCtx(cctx, sid, a.Command)
		if err != nil {
			// A non-zero exit is a valid diagnostic result (stderr usually
			// explains it), not a tool failure — same policy as the agent.
			var exitErr *cryptossh.ExitError
			if errors.As(err, &exitErr) {
				return textResult(fmt.Sprintf("%s\n[exit status %d]", tools.CapOutputAt(out, outputLimit), exitErr.ExitStatus())), nil, nil
			}
			return nil, nil, fmt.Errorf("exec on %s: %w", host.Name, err)
		}
		return textResult(tools.CapOutputAt(out, outputLimit)), nil, nil
	})

	// 4. read_file — remote file via SFTP (READ).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "read_file",
		Description: "Read a file from a remote host via SFTP and return its contents as text.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a pathArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		if err := s.gateCheck(ctx, host, "read_file", domain.PermissionRead, a); err != nil {
			return nil, nil, err
		}
		data, err := s.deps.SFTP.DownloadFile(host, creds, a.Path, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s on %s: %w", a.Path, host.Name, err)
		}
		return textResult(tools.CapOutputAt(string(data), outputLimit)), nil, nil
	})

	// 5. write_file — remote file via SFTP (WRITE — approval).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "write_file",
		Description: "Write text content to a file on a remote host via SFTP. Overwrites the file if it exists. Requires user approval in the app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a writeArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		if err := s.gateCheck(ctx, host, "write_file", domain.PermissionWrite, a); err != nil {
			return nil, nil, err
		}
		if err := s.deps.SFTP.UploadFile(host, creds, a.Path, []byte(a.Content), nil); err != nil {
			return nil, nil, fmt.Errorf("write %s on %s: %w", a.Path, host.Name, err)
		}
		return textResult(fmt.Sprintf("wrote %d bytes to %s on %s", len(a.Content), a.Path, host.Name)), nil, nil
	})

	// 6. upload — local → remote (WRITE — approval).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "upload",
		Description: "Upload a file from this machine (where AI Remote Workspace runs) to a remote host. Requires user approval in the app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a transferArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		if err := s.gateCheck(ctx, host, "upload", domain.PermissionWrite, a); err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(a.LocalPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read local %s: %w", a.LocalPath, err)
		}
		if err := s.deps.SFTP.UploadFile(host, creds, a.RemotePath, data, nil); err != nil {
			return nil, nil, fmt.Errorf("upload to %s on %s: %w", a.RemotePath, host.Name, err)
		}
		return textResult(fmt.Sprintf("uploaded %s → %s:%s (%d bytes)", a.LocalPath, host.Name, a.RemotePath, len(data))), nil, nil
	})

	// 7. download — remote → local (READ).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "download",
		Description: "Download a file from a remote host to this machine (where AI Remote Workspace runs).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a transferArgs) (*mcp.CallToolResult, any, error) {
		host, creds, err := s.resolveHost(a.HostID, a.Name)
		if err != nil {
			return nil, nil, err
		}
		if err := s.gateCheck(ctx, host, "download", domain.PermissionRead, a); err != nil {
			return nil, nil, err
		}
		data, err := s.deps.SFTP.DownloadFile(host, creds, a.RemotePath, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("download %s from %s: %w", a.RemotePath, host.Name, err)
		}
		if err := os.WriteFile(a.LocalPath, data, 0o644); err != nil {
			return nil, nil, fmt.Errorf("write local %s: %w", a.LocalPath, err)
		}
		return textResult(fmt.Sprintf("downloaded %s:%s → %s (%d bytes)", host.Name, a.RemotePath, a.LocalPath, len(data))), nil, nil
	})

	// 8. system_info — ambient workspace info (READ).
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "system_info",
		Description: "Get information about the AI Remote Workspace instance itself: app version, platform, number of configured hosts and MCP server state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		if err := s.gateCheck(ctx, domain.Host{ID: "mcp", Name: "workspace"}, "system_info", domain.PermissionRead, nil); err != nil {
			return nil, nil, err
		}
		hosts, _ := s.deps.Hosts.List()
		st := s.Status()
		return textResult(fmt.Sprintf(
			"app=%s\nversion=%s\nplatform=%s/%s\ngo=%s\nhosts=%d\nmcp_enabled=%t\nmcp_running=%t\nmcp_url=%s",
			s.deps.AppName, s.deps.AppVersion, runtime.GOOS, runtime.GOARCH, runtime.Version(), len(hosts),
			st.Enabled, st.Running, st.URL,
		)), nil, nil
	})
}

// --- argument structs (schemas inferred by the SDK; fields without
// omitempty are required) ---

type noArgs struct{}

// hostArgs selects a host by id (preferred) or exact name.
type hostArgs struct {
	HostID string `json:"hostId,omitempty" jsonschema:"host id from list_hosts"`
	Name   string `json:"name,omitempty" jsonschema:"exact host name (alternative to hostId)"`
}

type execArgs struct {
	HostID  string `json:"hostId,omitempty" jsonschema:"host id from list_hosts"`
	Name    string `json:"name,omitempty" jsonschema:"exact host name (alternative to hostId)"`
	Command string `json:"command" jsonschema:"shell command to run on the remote host"`
}

type pathArgs struct {
	HostID string `json:"hostId,omitempty" jsonschema:"host id from list_hosts"`
	Name   string `json:"name,omitempty" jsonschema:"exact host name (alternative to hostId)"`
	Path   string `json:"path" jsonschema:"absolute remote file path"`
}

type writeArgs struct {
	HostID  string `json:"hostId,omitempty" jsonschema:"host id from list_hosts"`
	Name    string `json:"name,omitempty" jsonschema:"exact host name (alternative to hostId)"`
	Path    string `json:"path" jsonschema:"absolute remote file path"`
	Content string `json:"content" jsonschema:"file contents to write"`
}

type transferArgs struct {
	HostID     string `json:"hostId,omitempty" jsonschema:"host id from list_hosts"`
	Name       string `json:"name,omitempty" jsonschema:"exact host name (alternative to hostId)"`
	RemotePath string `json:"remotePath" jsonschema:"remote file path"`
	LocalPath  string `json:"localPath" jsonschema:"local file path on the machine running AI Remote Workspace"`
}

// --- shared handler plumbing ---

// resolveHost maps (hostId | name) to a host record with credentials
// resolved from the OS vault, so "remember password" hosts authenticate
// without any secret ever crossing the MCP boundary.
func (s *Server) resolveHost(hostID, name string) (domain.Host, domain.Credentials, error) {
	var host domain.Host
	switch {
	case hostID != "":
		h, err := s.deps.Hosts.Get(hostID)
		if err != nil {
			return domain.Host{}, domain.Credentials{}, fmt.Errorf("host %q not found — call list_hosts for valid ids", hostID)
		}
		host = h
	case name != "":
		hosts, err := s.deps.Hosts.List()
		if err != nil {
			return domain.Host{}, domain.Credentials{}, err
		}
		found := false
		for _, h := range hosts {
			if h.Name == name {
				host, found = h, true
				break
			}
		}
		if !found {
			return domain.Host{}, domain.Credentials{}, fmt.Errorf("no host named %q — call list_hosts first", name)
		}
	default:
		return domain.Host{}, domain.Credentials{}, fmt.Errorf("specify hostId or name (call list_hosts first)")
	}
	creds, err := s.deps.Hosts.ResolveCredentials(host, domain.Credentials{})
	if err != nil {
		return domain.Host{}, domain.Credentials{}, fmt.Errorf("resolve credentials for %s: %w", host.Name, err)
	}
	return host, creds, nil
}

// gateCheck routes a tool call through the shared PermissionGate. The gate
// session key "mcp:<hostName>" makes the approval dialog show which host an
// external agent is targeting (frontend falls back to parsing this key).
// The args JSON is enriched with the host name for the dialog display.
func (s *Server) gateCheck(ctx context.Context, host domain.Host, toolName string, perm domain.Permission, args any) error {
	if s.deps.Gate == nil {
		return nil
	}
	display := map[string]any{"host": host.Name}
	if b, err := json.Marshal(args); err == nil {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			for _, k := range []string{"command", "path", "remotePath", "localPath", "content"} {
				if v, ok := m[k]; ok {
					display[k] = v
				}
			}
		}
	}
	b, err := json.Marshal(display)
	if err != nil {
		b = []byte("{}")
	}
	return s.deps.Gate.Check(ctx, "mcp:"+host.Name, toolName, perm, string(b))
}

// ensureSession returns a live SSH session for the host, dialing one on
// first use. Sessions are MCP-owned: closed on server stop, and dropped from
// the cache when they exit so the next call reconnects.
func (s *Server) ensureSession(ctx context.Context, host domain.Host, creds domain.Credentials) (string, error) {
	s.mu.Lock()
	sid, ok := s.sessions[host.ID]
	s.mu.Unlock()
	if ok {
		return sid, nil
	}
	newID, err := s.deps.SSH.OpenSession(ctx, host, creds, 80, 24, mcpSessionEvents{s})
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.sessions[host.ID] = newID
	s.mu.Unlock()
	return newID, nil
}

// mcpSessionEvents discards PTY output (exec uses its own channels) and
// forgets the session on exit so a dead connection is re-dialed, not reused.
type mcpSessionEvents struct{ s *Server }

func (e mcpSessionEvents) OnData(string, []byte)      {}
func (e mcpSessionEvents) OnProgress(string, string)  {}
func (e mcpSessionEvents) OnReconnecting(string, int) {}

func (e mcpSessionEvents) OnExit(sessionID string, _ error) {
	e.s.mu.Lock()
	defer e.s.mu.Unlock()
	for hostID, sid := range e.s.sessions {
		if sid == sessionID {
			delete(e.s.sessions, hostID)
		}
	}
}

// textResult wraps text as the tool's only content item.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// shortID trims a session id for human-facing output.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

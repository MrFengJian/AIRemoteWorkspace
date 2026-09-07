// Package mcpserver exposes the workspace's host/SSH/SFTP capabilities as an
// MCP (Model Context Protocol) server so external AI agents — Claude, Codex,
// Cursor — can drive the same infrastructure the built-in agent uses
// (ROADMAP Phase 6).
//
// Transport: streamable HTTP bound to 127.0.0.1. The app is a GUI process, so
// stdio — the transport most desktop agents default to — is not an option;
// clients connect by URL (Claude/Cursor native, Codex/mcp-remote). Every
// request must present the bearer token from AppConfig.MCP.Token, and every
// mutating tool call flows through the shared PermissionGate, surfacing the
// same approval dialog in the app as the built-in agent's tool calls.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent/tools"
)

// outputLimit bounds one tool result (same default as the agent ToolSet).
const outputLimit = 64 << 10 // 64 KB

// execTimeout caps a single exec_command run; MCP clients have their own
// request timeouts, but a hung remote command must not pin a session forever.
const execTimeout = 120 * time.Second

// HostSource resolves host records and their remembered credentials.
// *application.HostService satisfies it.
type HostSource interface {
	List() ([]domain.Host, error)
	Get(id string) (domain.Host, error)
	ResolveCredentials(host domain.Host, provided domain.Credentials) (domain.Credentials, error)
}

// SessionManager is the subset of the SSH manager the MCP tools need.
// *ssh.Manager satisfies it.
type SessionManager interface {
	OpenSession(ctx context.Context, host domain.Host, creds domain.Credentials, cols, rows int, events application.SessionEvents) (sessionID string, err error)
	ExecInSessionCtx(ctx context.Context, sessionID, cmd string) (string, error)
	Close(sessionID string) error
}

// SftpFileOps is the remote-file subset of the SFTP manager (same shape the
// agent's file tools use). *sftp.Manager satisfies it.
type SftpFileOps = tools.SftpFileOps

// PermissionGate is called before every tool execution. *application.
// PermissionGate (the gate shared with the built-in agent) satisfies it.
type PermissionGate interface {
	Check(ctx context.Context, sessionID, toolName string, perm domain.Permission, argsJSON string) error
}

// Deps bundles the collaborators the MCP server needs. Everything except
// Persist is required.
type Deps struct {
	AppName    string
	AppVersion string
	Hosts      HostSource
	SSH        SessionManager
	SFTP       SftpFileOps
	Gate       PermissionGate
	// Persist saves a regenerated/auto-generated token back to the app
	// config. May be nil (tests); the token then survives only in memory.
	Persist func(domain.MCPConfig) error
}

// Server runs the local MCP endpoint. Create with New, drive with
// ApplyConfig (config load + every config save), inspect with Status.
type Server struct {
	deps Deps
	mcp  *mcp.Server
	auth http.Handler // auth middleware wrapped around the MCP handler

	mu    sync.Mutex
	cfg   domain.MCPConfig  // effective config (normalized; holds the token)
	srv   *http.Server      // non-nil while running
	err   string            // last start failure, "" when running/stopped clean
	sessions map[string]string // hostID → SSH session owned by this server
}

// New builds the server (not yet listening). The MCP tool set is registered
// once; ApplyConfig only controls the listener.
func New(deps Deps) *Server {
	s := &Server{deps: deps, sessions: map[string]string{}}
	s.mcp = mcp.NewServer(&mcp.Implementation{Name: deps.AppName, Version: deps.AppVersion}, nil)
	s.registerTools()
	s.auth = withAuth(s.token, mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcp }, nil,
	))
	return s
}

// ApplyConfig reconciles the listener with cfg: stops when disabled, (re)
// starts on enable/port change, generates + persists the token on first
// enable. Safe to call on every config save.
func (s *Server) ApplyConfig(cfg domain.MCPConfig) {
	if cfg.Port == 0 {
		cfg.Port = domain.DefaultMCPPort
	}

	s.mu.Lock()
	prev := s.cfg
	s.cfg = cfg
	token := s.cfg.Token
	s.mu.Unlock()

	if !cfg.Enabled {
		s.stop("disabled")
		return
	}

	// First enable with no token: generate one and persist it so client
	// configs survive restarts.
	if token == "" {
		tok := GenerateToken()
		s.mu.Lock()
		s.cfg.Token = tok
		s.mu.Unlock()
		if s.deps.Persist != nil {
			if err := s.deps.Persist(s.EffectiveConfig()); err != nil {
				log.Printf("mcp: persist generated token: %v", err)
			}
		}
	}

	// A listener with the same port + token keeps running as-is.
	s.mu.Lock()
	same := prev.Enabled && cfg.Enabled && prev.Port == cfg.Port &&
		prev.Token == s.cfg.Token && s.srv != nil
	s.mu.Unlock()
	if same {
		return
	}
	s.stop("reconfigured")
	if err := s.start(); err != nil {
		log.Printf("mcp: start: %v", err)
	}
}

// EffectiveConfig returns the config as currently in effect (with any
// auto-generated token applied).
func (s *Server) EffectiveConfig() domain.MCPConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Status reports the runtime state for the settings UI.
func (s *Server) Status() domain.MCPStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := domain.MCPStatus{
		Enabled: s.cfg.Enabled,
		Running: s.srv != nil,
		Port:    s.cfg.Port,
		Token:   s.cfg.Token,
		Error:   s.err,
	}
	if s.srv != nil || s.cfg.Enabled {
		st.URL = fmt.Sprintf("http://127.0.0.1:%d/mcp", st.Port)
	}
	return st
}

// RegenerateToken replaces the bearer token (old client configs stop
// working), persists it, and returns the refreshed status.
func (s *Server) RegenerateToken() domain.MCPStatus {
	s.mu.Lock()
	s.cfg.Token = GenerateToken()
	s.mu.Unlock()
	if s.deps.Persist != nil {
		if err := s.deps.Persist(s.EffectiveConfig()); err != nil {
			log.Printf("mcp: persist regenerated token: %v", err)
		}
	}
	return s.Status()
}

// Stop shuts the listener down and closes every MCP-owned SSH session.
func (s *Server) Stop() { s.stop("shutdown") }

// start binds 127.0.0.1:<port> and serves. Listening synchronously surfaces
// "port in use" to Status instead of racing the UI.
func (s *Server) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.err = err.Error()
		return err
	}
	srv := &http.Server{Handler: s.auth, ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv
	s.err = ""
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.mu.Lock()
			s.srv = nil
			s.err = err.Error()
			s.mu.Unlock()
			log.Printf("mcp: serve: %v", err)
		}
	}()
	return nil
}

// stop tears the listener down (idempotent) and releases MCP-owned sessions.
func (s *Server) stop(reason string) {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	own := make(map[string]string, len(s.sessions))
	for k, v := range s.sessions {
		own[k] = v
		delete(s.sessions, k)
	}
	s.mu.Unlock()

	if srv != nil {
		// Grace period for in-flight requests, then hard-close: MCP clients
		// hold open SSE streams, and plain Shutdown would wait on them for
		// the full grace period every time.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		_ = srv.Close()
		log.Printf("mcp: server stopped (%s)", reason)
	}
	for hostID, sid := range own {
		if err := s.deps.SSH.Close(sid); err != nil {
			log.Printf("mcp: close session for host %s: %v", hostID, err)
		}
	}
}

// token returns the current bearer token ("", while never configured, blocks
// every request — the auth middleware rejects on empty too).
func (s *Server) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Token
}

// GenerateToken returns a 256-bit random hex bearer token.
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("mcp: token generation: %v", err)) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}

// withAuth guards the MCP handler: only requests presenting the current
// bearer token pass (currentToken is re-read per request so token rotation
// applies immediately). Compared in constant time; the endpoint is
// additionally bound to 127.0.0.1, so this is defense in depth against other
// local processes/users, not against the network.
func withAuth(currentToken func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := currentToken()
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || got == "" ||
			subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ai-remote-workspace"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

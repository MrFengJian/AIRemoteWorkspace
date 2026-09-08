package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/infrastructure/localpty"
	ssh "github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// OpenSessionRequest carries what the frontend needs to start a terminal.
type OpenSessionRequest struct {
	HostID string         `json:"hostId"`
	Creds  CredentialsDTO `json:"creds"`
	Size   PtySizeDTO     `json:"size"`
	// ConnectID is a frontend-generated correlation id: connection-progress
	// events are emitted under "terminal:connect" carrying it while this
	// call is in flight (the session id doesn't exist until it returns).
	ConnectID string `json:"connectId"`
}

// PtySizeDTO is the initial terminal dimensions.
type PtySizeDTO struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// OpenSessionResult returns the new session id to the frontend.
type OpenSessionResult struct {
	SessionID string `json:"sessionId"`
}

// TerminalService owns terminal sessions and bridges PTY I/O to the Wails
// event bus. Per-session event names are namespaced by session id:
//
//	term:<id>:out  — remote→local output (Go → JS)
//	term:<id>:exit — shell exited        (Go → JS)
//
// Input flows via bound methods (WriteStdin/Resize) — more controllable than
// events for low-latency keystrokes. This matches the hybrid pattern the Wails
// v3 streaming research recommended (output via events, input via bindings).
//
// Sessions route by id: "local-"-prefixed ids go to the local PTY manager
// (interactive shell on the user's machine, no SSH); everything else to the
// SSH connection manager.
type TerminalService struct {
	app         *wailsapp.App
	hostSvc     *appsvc.HostService
	connManager appsvc.ConnectionManager
	localMgr    *localpty.Manager
	tunnels     *ssh.TunnelManager
	logs        *appsvc.SessionLogService

	mu sync.Mutex
}

// NewTerminalService wires the TerminalService. The *Application is injected
// via ServiceStartup (Wails constructs the app after services are registered).
// tunnels (may be nil) hosts the per-host SSH tunnels a session auto-starts.
// logs (may be nil) records session output to disk when the user enables it.
func NewTerminalService(hostSvc *appsvc.HostService, connManager appsvc.ConnectionManager, localMgr *localpty.Manager, tunnels *ssh.TunnelManager, logs *appsvc.SessionLogService) *TerminalService {
	return &TerminalService{hostSvc: hostSvc, connManager: connManager, localMgr: localMgr, tunnels: tunnels, logs: logs}
}

// ServiceName lets Wails register the service under a stable name.
func (t *TerminalService) ServiceName() string { return "TerminalService" }

// ServiceStartup captures the Application handle so we can emit events.
// Wails calls this with (ctx, options) — the App is obtained via the global
// accessor since ServiceOptions doesn't carry it.
func (t *TerminalService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	t.app = wailsapp.Get()
	return nil
}

// ServiceShutdown closes all live sessions on app exit (SSH + local).
func (t *TerminalService) ServiceShutdown() error {
	sshErr := t.connManager.CloseAll()
	localErr := t.localMgr.CloseAll()
	if sshErr != nil {
		return sshErr
	}
	return localErr
}

// OpenSession dials the host and starts an interactive PTY shell. Output is
// streamed over the per-session "term:<id>:out" event; termination over
// "term:<id>:exit".
func (t *TerminalService) OpenSession(req OpenSessionRequest) (OpenSessionResult, error) {
	events := &terminalEvents{app: t.app, connectID: req.ConnectID, logs: t.logs}

	host, err := t.hostSvc.Get(req.HostID)
	if err != nil {
		return OpenSessionResult{}, fmt.Errorf("open session: %w", err)
	}

	// Resolve credentials: use what the frontend sent; fall back to any
	// remembered secret from the OS vault when the frontend sent blanks.
	events.OnProgress("", "credentials")
	creds, err := t.hostSvc.ResolveCredentials(host, toDomainCreds(req.Creds))
	if err != nil {
		return OpenSessionResult{}, err
	}

	ctx := context.Background()
	sessionID, err := t.connManager.OpenSession(ctx, host, creds, req.Size.Cols, req.Size.Rows, events)
	if err != nil {
		return OpenSessionResult{}, err
	}

	// Auto-detect the host OS in the background (if not already recorded).
	// Read-only metadata for display; failures are silent and never break
	// the terminal session.
	if host.OS == "" {
		hostID := req.HostID
		sid := sessionID
		go t.hostSvc.EnsureOS(hostID, sid)
	}

	// Auto-start the host's SSH tunnels when configured ("打开标签页时自动拉起").
	// Ensure dedupes per rule, so many sessions share one tunnel per rule. It
	// runs in the background — dial failures surface as tunnel status events
	// and must never block or fail the terminal itself.
	if len(host.Tunnels) > 0 && t.tunnels != nil {
		tunnels := t.tunnels
		go tunnels.Ensure(host, creds)
	}

	return OpenSessionResult{SessionID: sessionID}, nil
}

// OpenLocalSession starts an interactive shell on the user's machine over a
// local PTY (Windows: PowerShell/cmd via ConPTY; Unix: the login shell via
// openpty). Same event contract as OpenSession.
func (t *TerminalService) OpenLocalSession(size PtySizeDTO) (OpenSessionResult, error) {
	events := &terminalEvents{app: t.app, logs: t.logs}
	sessionID, err := t.localMgr.Open(size.Cols, size.Rows, events)
	if err != nil {
		return OpenSessionResult{}, err
	}
	return OpenSessionResult{SessionID: sessionID}, nil
}

// WriteStdin forwards a keystroke/line to the session's shell (local or SSH).
func (t *TerminalService) WriteStdin(sessionID string, data []byte) error {
	var err error
	if localpty.IsLocal(sessionID) {
		err = t.localMgr.WriteStdin(sessionID, data)
	} else {
		err = t.connManager.WriteStdin(sessionID, data)
	}
	if err != nil {
		if errors.Is(err, ssh.ErrSessionNotFound) || errors.Is(err, localpty.ErrSessionNotFound) {
			log.Printf("WriteStdin: session %s not found, ignoring", sessionID)
			return nil
		}
		return err
	}
	return nil
}

// ResizeSession updates the PTY dimensions (local or SSH).
func (t *TerminalService) ResizeSession(sessionID string, size PtySizeDTO) error {
	var err error
	if localpty.IsLocal(sessionID) {
		err = t.localMgr.Resize(sessionID, size.Cols, size.Rows)
	} else {
		err = t.connManager.Resize(sessionID, size.Cols, size.Rows)
	}
	if err != nil {
		if errors.Is(err, ssh.ErrSessionNotFound) || errors.Is(err, localpty.ErrSessionNotFound) {
			log.Printf("ResizeSession: session %s not found, ignoring", sessionID)
			return nil
		}
		return err
	}
	return nil
}

// CloseSession ends a session and frees its resources (local or SSH).
func (t *TerminalService) CloseSession(sessionID string) error {
	// Close an active session log up front — the session's exit event also
	// stops it, but the tab-close path is the deterministic one for UI calls.
	if t.logs != nil {
		t.logs.Stop(sessionID)
	}
	var err error
	if localpty.IsLocal(sessionID) {
		err = t.localMgr.Close(sessionID)
	} else {
		err = t.connManager.Close(sessionID)
	}
	if err != nil {
		if errors.Is(err, ssh.ErrSessionNotFound) || errors.Is(err, localpty.ErrSessionNotFound) {
			log.Printf("CloseSession: session %s not found, ignoring", sessionID)
			return nil
		}
		return err
	}
	return nil
}

// terminalEvents implements application.SessionEvents, forwarding PTY output
// and exit onto the Wails event bus under term:<id>:out / :exit. The session
// id arrives as an OnData/OnExit argument (the connection manager assigns it
// before the pumps start), so no per-session state is held here. Connection
// progress stages additionally go out under "terminal:connect" tagged with
// the request's ConnectID so the UI can correlate them with the opening call.
// logs (may be nil) receives every output chunk for sessions the user is
// recording, and is closed out on exit.
type terminalEvents struct {
	app       *wailsapp.App
	connectID string
	logs      *appsvc.SessionLogService
}

// terminalConnectEvent is the payload of the "terminal:connect" progress
// event (see OpenSessionRequest.ConnectID).
type terminalConnectEvent struct {
	ConnectID string `json:"connectId"`
	Stage     string `json:"stage"`
}

func (te *terminalEvents) OnProgress(_, stage string) {
	if te.app == nil || te.connectID == "" {
		return
	}
	te.app.Event.Emit("terminal:connect", terminalConnectEvent{
		ConnectID: te.connectID,
		Stage:     stage,
	})
}

func (te *terminalEvents) OnData(sessionID string, data []byte) {
	if te.logs != nil {
		te.logs.Write(sessionID, data) // best-effort session logging; never fails the pump
	}
	if te.app == nil {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	te.app.Event.Emit(fmt.Sprintf("term:%s:out", sessionID), encoded)
}

// OnReconnecting forwards session auto-reconnect progress onto
// "term:<id>:reconnecting": attempt >= 1 before each redial, 0 on success.
func (te *terminalEvents) OnReconnecting(sessionID string, attempt int) {
	if te.app == nil {
		return
	}
	te.app.Event.Emit(fmt.Sprintf("term:%s:reconnecting", sessionID), map[string]int{"attempt": attempt})
}

func (te *terminalEvents) OnExit(sessionID string, exitErr error) {
	if te.logs != nil {
		te.logs.Stop(sessionID) // close the log file — the session is over
	}
	if te.app == nil {
		return
	}
	msg := ""
	if exitErr != nil {
		msg = exitErr.Error()
	}
	te.app.Event.Emit(fmt.Sprintf("term:%s:exit", sessionID), msg)
}

// ── Session logging (Phase 8 会话日志) ------------------------------------

// SessionLogInfoDTO reports whether a session is being recorded and where.
type SessionLogInfoDTO struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// StartSessionLog begins recording the session's output to
// <数据目录>/logs/<主机>/<时间>-<会话>.log and returns the file info.
func (t *TerminalService) StartSessionLog(sessionID, hostName string) (SessionLogInfoDTO, error) {
	if t.logs == nil {
		return SessionLogInfoDTO{}, fmt.Errorf("session log service not available")
	}
	info, err := t.logs.Start(sessionID, hostName)
	if err != nil {
		return SessionLogInfoDTO{}, err
	}
	return SessionLogInfoDTO{Enabled: info.Enabled, Path: info.Path}, nil
}

// StopSessionLog closes the session's log file.
func (t *TerminalService) StopSessionLog(sessionID string) (SessionLogInfoDTO, error) {
	if t.logs == nil {
		return SessionLogInfoDTO{}, fmt.Errorf("session log service not available")
	}
	info := t.logs.Stop(sessionID)
	return SessionLogInfoDTO{Enabled: info.Enabled, Path: info.Path}, nil
}

// GetSessionLog reports the session's current recording status.
func (t *TerminalService) GetSessionLog(sessionID string) (SessionLogInfoDTO, error) {
	if t.logs == nil {
		return SessionLogInfoDTO{}, nil
	}
	info := t.logs.Status(sessionID)
	return SessionLogInfoDTO{Enabled: info.Enabled, Path: info.Path}, nil
}

// OpenSessionLogDir opens the session-log base directory in the OS file
// browser (created on demand so the entry works before the first recording).
func (t *TerminalService) OpenSessionLogDir() error {
	if t.logs == nil {
		return fmt.Errorf("session log service not available")
	}
	dir := t.logs.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	return openInFileBrowser(dir)
}

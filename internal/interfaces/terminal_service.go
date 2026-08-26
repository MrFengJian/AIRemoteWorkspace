package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
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

	mu sync.Mutex
}

// NewTerminalService wires the TerminalService. The *Application is injected
// via ServiceStartup (Wails constructs the app after services are registered).
func NewTerminalService(hostSvc *appsvc.HostService, connManager appsvc.ConnectionManager, localMgr *localpty.Manager) *TerminalService {
	return &TerminalService{hostSvc: hostSvc, connManager: connManager, localMgr: localMgr}
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
	events := &terminalEvents{app: t.app, connectID: req.ConnectID}

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

	return OpenSessionResult{SessionID: sessionID}, nil
}

// OpenLocalSession starts an interactive shell on the user's machine over a
// local PTY (Windows: PowerShell/cmd via ConPTY; Unix: the login shell via
// openpty). Same event contract as OpenSession.
func (t *TerminalService) OpenLocalSession(size PtySizeDTO) (OpenSessionResult, error) {
	events := &terminalEvents{app: t.app}
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
type terminalEvents struct {
	app       *wailsapp.App
	connectID string
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
	if te.app == nil {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	te.app.Event.Emit(fmt.Sprintf("term:%s:out", sessionID), encoded)
}

func (te *terminalEvents) OnExit(sessionID string, exitErr error) {
	if te.app == nil {
		return
	}
	msg := ""
	if exitErr != nil {
		msg = exitErr.Error()
	}
	te.app.Event.Emit(fmt.Sprintf("term:%s:exit", sessionID), msg)
}

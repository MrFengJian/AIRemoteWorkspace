package interfaces

import (
	"context"
	"fmt"
	"sync"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
)

// OpenSessionRequest carries what the frontend needs to start a terminal.
type OpenSessionRequest struct {
	HostID string         `json:"hostId"`
	Creds  CredentialsDTO `json:"creds"`
	Size   PtySizeDTO     `json:"size"`
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
type TerminalService struct {
	app         *wailsapp.App
	hostSvc     *appsvc.HostService
	connManager appsvc.ConnectionManager

	mu sync.Mutex
}

// NewTerminalService wires the TerminalService. The *Application is injected
// via ServiceStartup (Wails constructs the app after services are registered).
func NewTerminalService(hostSvc *appsvc.HostService, connManager appsvc.ConnectionManager) *TerminalService {
	return &TerminalService{hostSvc: hostSvc, connManager: connManager}
}

// ServiceName lets Wails register the service under a stable name.
func (t *TerminalService) ServiceName() string { return "TerminalService" }

// ServiceStartup captures the Application handle so we can emit events.
func (t *TerminalService) ServiceStartup(app *wailsapp.App) error {
	t.app = app
	return nil
}

// ServiceShutdown closes all live sessions on app exit.
func (t *TerminalService) ServiceShutdown(_ *wailsapp.App) error {
	return t.connManager.CloseAll()
}

// OpenSession dials the host and starts an interactive PTY shell. Output is
// streamed over the per-session "term:<id>:out" event; termination over
// "term:<id>:exit".
func (t *TerminalService) OpenSession(req OpenSessionRequest) (OpenSessionResult, error) {
	host, err := t.hostSvc.Get(req.HostID)
	if err != nil {
		return OpenSessionResult{}, fmt.Errorf("open session: %w", err)
	}

	creds := toDomainCreds(req.Creds)
	events := &terminalEvents{app: t.app}

	ctx := context.Background()
	sessionID, err := t.connManager.OpenSession(ctx, host, creds, req.Size.Cols, req.Size.Rows, events)
	if err != nil {
		return OpenSessionResult{}, err
	}
	return OpenSessionResult{SessionID: sessionID}, nil
}

// WriteStdin forwards a keystroke/line to the session's remote shell.
func (t *TerminalService) WriteStdin(sessionID string, data []byte) error {
	return t.connManager.WriteStdin(sessionID, data)
}

// ResizeSession updates the remote PTY dimensions.
func (t *TerminalService) ResizeSession(sessionID string, size PtySizeDTO) error {
	return t.connManager.Resize(sessionID, size.Cols, size.Rows)
}

// CloseSession ends a session and frees its connection.
func (t *TerminalService) CloseSession(sessionID string) error {
	return t.connManager.Close(sessionID)
}

// terminalEvents implements application.SessionEvents, forwarding PTY output
// and exit onto the Wails event bus under term:<id>:out / :exit. The session
// id arrives as an OnData/OnExit argument (the connection manager assigns it
// before the pumps start), so no per-session state is held here.
type terminalEvents struct {
	app *wailsapp.App
}

func (te *terminalEvents) OnData(sessionID string, data []byte) {
	if te.app == nil {
		return
	}
	te.app.Event.Emit(fmt.Sprintf("term:%s:out", sessionID), string(data))
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

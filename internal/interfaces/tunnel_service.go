package interfaces

import (
	"context"
	"fmt"
	"net"
	"strconv"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
	ssh "github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// TunnelStatusDTO mirrors domain.TunnelStatus for the frontend.
type TunnelStatusDTO struct {
	HostID    string              `json:"hostId"`
	HostName  string              `json:"hostName"`
	Key       string              `json:"key"` // rule identity (domain.TunnelConfig.Key)
	Config    domain.TunnelConfig `json:"config"`
	State     string              `json:"state"`
	LastError string              `json:"lastError,omitempty"`
	Retries   int                 `json:"retries"`
}

// TunnelService exposes SSH tunnel control + status to the frontend. State
// changes stream on the "tunnel:status" event (the panel keeps a live view);
// ListTunnels returns the full snapshot (initial load).
type TunnelService struct {
	app     *wailsapp.App
	mgr     *ssh.TunnelManager
	hostSvc *appsvc.HostService
}

// NewTunnelService wires the Wails TunnelService to the tunnel manager.
func NewTunnelService(mgr *ssh.TunnelManager, hostSvc *appsvc.HostService) *TunnelService {
	return &TunnelService{mgr: mgr, hostSvc: hostSvc}
}

// ServiceName lets Wails register the service under a stable name.
func (t *TunnelService) ServiceName() string { return "TunnelService" }

// ServiceStartup captures the Application handle so we can emit events, and
// registers the service as the manager's status sink.
func (t *TunnelService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	t.app = wailsapp.Get()
	t.mgr.SetEmitter(t.EmitStatus)
	return nil
}

// ServiceShutdown tears down every tunnel on app exit.
func (t *TunnelService) ServiceShutdown() error {
	t.mgr.StopAll()
	return nil
}

// EmitStatus forwards a manager status update onto the Wails event bus
// (wired as the manager's emitter in ServiceStartup).
func (t *TunnelService) EmitStatus(s domain.TunnelStatus) {
	if t.app == nil {
		return
	}
	t.app.Event.Emit("tunnel:status", toTunnelStatusDTO(s))
}

// ListTunnels returns the status of every known tunnel (all hosts).
func (t *TunnelService) ListTunnels() ([]TunnelStatusDTO, error) {
	statuses := t.mgr.Statuses()
	out := make([]TunnelStatusDTO, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, toTunnelStatusDTO(s))
	}
	return out, nil
}

// StartTunnel ensures the host's tunnels are running per its saved rules,
// resolving remembered credentials from the OS vault for the connections.
func (t *TunnelService) StartTunnel(hostID string) error {
	host, err := t.hostSvc.Get(hostID)
	if err != nil {
		return err
	}
	hasValid := false
	for _, c := range host.Tunnels {
		if c.Valid() {
			hasValid = true
			break
		}
	}
	if !hasValid {
		return fmt.Errorf("host %s has no valid tunnel configuration", host.Name)
	}
	creds, err := t.hostSvc.ResolveCredentials(host, domain.Credentials{})
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}
	t.mgr.Ensure(host, creds)
	return nil
}

// StopTunnel stops the host's tunnels (the rules stay; the next session on
// the host or a manual start brings them back).
func (t *TunnelService) StopTunnel(hostID string) error {
	t.mgr.Stop(hostID)
	return nil
}

// CheckTunnelPorts reports which of the requested LOCAL listen ports are
// already occupied — the save-time conflict pre-check for the host form.
// Ports bound by this host's own tunnels are exempt (they belong to the host
// being edited and are reconciled on save); anything else holding the port —
// another process, or another host's tunnel — is reported.
func (t *TunnelService) CheckTunnelPorts(hostID string, ports []int) ([]int, error) {
	own := make(map[int]bool)
	for _, p := range t.mgr.ListenPortsOf(hostID) {
		own[p] = true
	}
	seen := make(map[int]bool)
	var busy []int
	for _, p := range ports {
		if p <= 0 || p > 65535 || own[p] || seen[p] {
			continue
		}
		seen[p] = true
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			busy = append(busy, p)
			continue
		}
		_ = ln.Close()
	}
	return busy, nil
}

func toTunnelStatusDTO(s domain.TunnelStatus) TunnelStatusDTO {
	return TunnelStatusDTO{
		HostID:    s.HostID,
		HostName:  s.HostName,
		Key:       s.Key,
		Config:    s.Config,
		State:     string(s.State),
		LastError: s.LastError,
		Retries:   s.Retries,
	}
}

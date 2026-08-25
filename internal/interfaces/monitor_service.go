package interfaces

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// --- MonitorService ------------------------------------------------------

// MonitorService exposes host metrics collection to the frontend. Collection
// runs over an existing terminal session's SSH connection (sessionID = the
// tab's session), so no extra dial or credentials are involved.
type MonitorService struct {
	svc *appsvc.MonitorService
}

// NewMonitorService wires the Wails MonitorService to its application port.
func NewMonitorService(svc *appsvc.MonitorService) *MonitorService {
	return &MonitorService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (m *MonitorService) ServiceName() string { return "MonitorService" }

// ServiceStartup runs when the service is registered with the app.
func (m *MonitorService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}

// monitorTimeout bounds one collection round: the scripts sleep 1s between
// samples plus channel/transfer overhead, so 20s leaves generous headroom
// for slow links while still surfacing hangs to the UI.
const monitorTimeout = 20 * time.Second

// GetOverview returns the host overview snapshot for a live SSH session.
func (m *MonitorService) GetOverview(sessionID string) (domain.MonitorOverview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorTimeout)
	defer cancel()
	return m.svc.GetOverview(ctx, sessionID)
}

// GetProcesses returns the live process list for a live SSH session.
func (m *MonitorService) GetProcesses(sessionID string) ([]domain.MonitorProcess, error) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorTimeout)
	defer cancel()
	return m.svc.GetProcesses(ctx, sessionID)
}

// GetPorts returns the listening-socket list for a live SSH session.
func (m *MonitorService) GetPorts(sessionID string) ([]domain.MonitorPort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), monitorTimeout)
	defer cancel()
	return m.svc.GetPorts(ctx, sessionID)
}

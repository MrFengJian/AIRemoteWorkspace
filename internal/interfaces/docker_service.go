package interfaces

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// --- DockerService -------------------------------------------------------

// DockerService exposes the Docker panel's data and control surface to the
// frontend. All reads and actions run against the host behind the given
// session (SSH exec channel) or the local docker CLI for local sessions.
type DockerService struct {
	svc *appsvc.DockerService
}

// NewDockerService wires the Wails DockerService to its application port.
func NewDockerService(svc *appsvc.DockerService) *DockerService {
	return &DockerService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (d *DockerService) ServiceName() string { return "DockerService" }

// ServiceStartup runs when the service is registered with the app.
func (d *DockerService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}

// dockerTimeout bounds one CLI round-trip. `docker stats --no-stream` takes
// ~2s to sample; lifecycle actions wait for the container to actually stop,
// so the ceiling stays generous.
const dockerTimeout = 30 * time.Second

// GetInfo returns the Docker overview (server version + counters) for a session.
func (d *DockerService) GetInfo(sessionID string) (domain.DockerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.GetInfo(ctx, sessionID)
}

// ListContainers returns the container list for a session (all=true includes
// stopped containers).
func (d *DockerService) ListContainers(sessionID string, all bool) ([]domain.DockerContainer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.ListContainers(ctx, sessionID, all)
}

// GetContainerStats returns one-shot resource usage per running container.
func (d *DockerService) GetContainerStats(sessionID string) ([]domain.DockerContainerStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.GetContainerStats(ctx, sessionID)
}

// ListImages returns the image list for a session.
func (d *DockerService) ListImages(sessionID string) ([]domain.DockerImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.ListImages(ctx, sessionID)
}

// GetLogs returns the last `tail` timestamped log lines of one container.
func (d *DockerService) GetLogs(sessionID, container string, tail int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.GetLogs(ctx, sessionID, container, tail)
}

// ContainerAction performs a lifecycle action (start/stop/restart/pause/
// unpause/kill) on one container.
func (d *DockerService) ContainerAction(sessionID, container, action string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	return d.svc.ContainerAction(ctx, sessionID, container, action)
}

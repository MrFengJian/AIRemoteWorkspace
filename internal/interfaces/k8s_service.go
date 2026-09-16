package interfaces

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// --- K8sService -----------------------------------------------------------

// K8sService exposes the Kubernetes panel's data and control surface to the
// frontend. All reads and actions run kubectl on the host behind the given
// session (SSH exec channel) or the local kubectl CLI for local sessions.
type K8sService struct {
	svc *appsvc.K8sService
}

// NewK8sService wires the Wails K8sService to its application port.
func NewK8sService(svc *appsvc.K8sService) *K8sService {
	return &K8sService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (k *K8sService) ServiceName() string { return "K8sService" }

// ServiceStartup runs when the service is registered with the app.
func (k *K8sService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}

// k8sTimeout bounds one CLI round-trip: kubectl against a remote API server
// over the host's network can be slower than local docker, but lifecycle
// actions use --wait=false, so the docker panel's generous ceiling still fits.
const k8sTimeout = 30 * time.Second

// GetClusterInfo returns the panel overview (versions, context, counters).
func (k *K8sService) GetClusterInfo(sessionID string) (domain.K8sClusterInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.GetClusterInfo(ctx, sessionID)
}

// ListNamespaces returns the namespaces for the panel's scope picker.
func (k *K8sService) ListNamespaces(sessionID string) ([]domain.K8sNamespace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListNamespaces(ctx, sessionID)
}

// ListNodes returns the cluster nodes (read-only).
func (k *K8sService) ListNodes(sessionID string) ([]domain.K8sNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListNodes(ctx, sessionID)
}

// ListWorkloads returns one workload kind's rows (deployments/statefulsets/
// daemonsets). An empty namespace means all namespaces.
func (k *K8sService) ListWorkloads(sessionID, kind, namespace string) ([]domain.K8sWorkload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListWorkloads(ctx, sessionID, kind, namespace)
}

// ListPods returns the pod rows for the namespace scope.
func (k *K8sService) ListPods(sessionID, namespace string) ([]domain.K8sPod, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListPods(ctx, sessionID, namespace)
}

// ListServices returns the service rows for the namespace scope.
func (k *K8sService) ListServices(sessionID, namespace string) ([]domain.K8sServiceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListServices(ctx, sessionID, namespace)
}

// ListEvents returns recent events, newest first.
func (k *K8sService) ListEvents(sessionID, namespace string) ([]domain.K8sEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ListEvents(ctx, sessionID, namespace)
}

// GetPodLogs returns the last `tail` timestamped log lines of one pod
// (optionally one of its containers — required for multi-container pods).
func (k *K8sService) GetPodLogs(sessionID, namespace, pod, container string, tail int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.GetPodLogs(ctx, sessionID, namespace, pod, container, tail)
}

// WorkloadAction performs a lifecycle action on one workload (scale with
// replicas for deployment/statefulset; rollout restart for all kinds).
func (k *K8sService) WorkloadAction(sessionID, kind, namespace, name, action string, replicas int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.WorkloadAction(ctx, sessionID, kind, namespace, name, action, replicas)
}

// DeletePod removes one pod (returns as soon as the deletion is accepted).
func (k *K8sService) DeletePod(sessionID, namespace, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.DeletePod(ctx, sessionID, namespace, name)
}

// GetResourceYAML returns one resource's live manifest for the YAML viewer
// (deployment/statefulset/daemonset/pod/service; concrete namespace required).
func (k *K8sService) GetResourceYAML(sessionID, kind, namespace, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.GetResourceYAML(ctx, sessionID, kind, namespace, name)
}

// ApplyResourceYAML submits an edited manifest via `kubectl apply -f -`
// (stdin); identity guards ensure it lands on the object it was fetched from.
func (k *K8sService) ApplyResourceYAML(sessionID, kind, namespace, name, doc string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k8sTimeout)
	defer cancel()
	return k.svc.ApplyResourceYAML(ctx, sessionID, kind, namespace, name, doc)
}

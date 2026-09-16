package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// K8sService collects Kubernetes state for the panel by driving the kubectl
// CLI on the host behind a session — the same shape as DockerService: over
// the session's SSH exec channel for remote tabs, or directly (argv only,
// no shell) for local terminal sessions. kubectl resolves its kubeconfig and
// context on that host; every list call uses `-o json` for machine-readable
// output. Scope: the common workload resources only (Deployments,
// StatefulSets, DaemonSets, Pods, Services, Events) — CRDs and other
// free-form types are deliberately not exposed.
type K8sService struct {
	connect ConnectionManager
}

// NewK8sService wires a K8sService to the connection manager.
func NewK8sService(connect ConnectionManager) *K8sService {
	return &K8sService{connect: connect}
}

// Sentinel errors the frontend maps to specific hints (vs. a generic failure).
var (
	// ErrKubectlUnavailable: the kubectl CLI is not installed on the target host.
	ErrKubectlUnavailable = errors.New("kubectl cli not available")
	// ErrClusterUnreachable: the CLI exists but cannot reach the cluster
	// (API server down, kubeconfig missing, auth failed).
	ErrClusterUnreachable = errors.New("k8s cluster unreachable")
)

// runKubectl executes `kubectl <args...>` against the session's host and
// translates common failure modes into the sentinel errors above.
func (s *K8sService) runKubectl(ctx context.Context, sessionID string, args ...string) (string, error) {
	return s.runKubectlStdin(ctx, sessionID, nil, args...)
}

// runKubectlStdin is runKubectl with data piped to the command's stdin
// (`kubectl apply -f -`). Streaming — no argument-size ceiling, and the
// payload never touches the remote disk.
func (s *K8sService) runKubectlStdin(ctx context.Context, sessionID string, stdin []byte, args ...string) (string, error) {
	var out string
	var err error
	if isLocalSessionID(sessionID) {
		out, err = runLocalArgsStdin(ctx, "kubectl", stdin, args...)
	} else {
		if s.connect == nil {
			return "", errors.New("kubectl: no connection manager")
		}
		cmd := "kubectl"
		for _, a := range args {
			cmd += " " + shellQuote(a)
		}
		out, err = s.connect.ExecInSessionStdin(ctx, sessionID, cmd, stdin)
	}
	return classifyKubectl(out, err)
}

// classifyKubectl translates common failure modes into the sentinel errors
// and appends the trimmed CLI output to generic failures — the UI can only
// show err.Error(), which for a failed exec is just "exit status 1", while
// kubectl's real reason (BadRequest, validation, not found…) lives in the
// output.
func classifyKubectl(out string, err error) (string, error) {
	if err != nil {
		if isKubectlCLIMissing(out, err) {
			return "", ErrKubectlUnavailable
		}
		if isClusterUnreachable(out) {
			return "", ErrClusterUnreachable
		}
		return out, fmt.Errorf("kubectl: %w: %s", err, errHint(out))
	}
	return out, nil
}

// errHint condenses the CLI output into a short single-line error suffix
// (kubectl's diagnostics usually live in the last lines).
func errHint(out string) string {
	hint := strings.TrimSpace(out)
	if hint == "" {
		return "no output"
	}
	hint = strings.ReplaceAll(hint, "\r", "")
	hint = strings.Join(strings.Fields(hint), " ")
	if len(hint) > 300 {
		hint = hint[:300] + "…"
	}
	return hint
}

// isKubectlCLIMissing recognizes the "executable not found" family (same
// shapes isDockerCLIMissing covers, matched on the combined output).
func isKubectlCLIMissing(out string, err error) bool {
	return isDockerCLIMissing(out, err)
}

// isClusterUnreachable recognizes kubectl's connection-failure texts: API
// server unreachable, no kubeconfig, and bad auth (401/403 surfaces here too
// — from the panel's perspective the cluster is equally unreachable).
func isClusterUnreachable(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "the connection to the server") ||
		strings.Contains(o, "connection refused") ||
		strings.Contains(o, "no configuration has been provided") ||
		strings.Contains(o, "unable to connect to the server") ||
		strings.Contains(o, "unauthorized") ||
		strings.Contains(o, "forbidden") ||
		strings.Contains(o, "context was not found") ||
		strings.Contains(o, "no auth plugin")
}

// k8sNameRe matches DNS-1123 subdomain names (namespaces, most resource
// names, container names) — the safe identifier set for kubectl arguments.
// It doubles as injection armor: shellQuote stops splitting, but an argument
// starting with `--` would still be parsed as a flag, so every user-visible
// identifier is validated before it reaches the command line.
var k8sNameRe = regexp.MustCompile(`^[a-zA-Z0-9]([-._a-zA-Z0-9]{0,251}[a-zA-Z0-9])?$`)

// checkK8sName validates one kubectl identifier (name/namespace/container).
func checkK8sName(field, v string) error {
	if v == "" {
		return fmt.Errorf("kubectl: empty %s", field)
	}
	if !k8sNameRe.MatchString(v) {
		return fmt.Errorf("kubectl: invalid %s %q", field, v)
	}
	return nil
}

// nsArgs renders the namespace scope: `-n <ns>` for a specific namespace,
// `--all-namespaces` when empty (the panel's "all" option).
func nsArgs(namespace string) ([]string, error) {
	if namespace == "" {
		return []string{"--all-namespaces"}, nil
	}
	if err := checkK8sName("namespace", namespace); err != nil {
		return nil, err
	}
	return []string{"-n", namespace}, nil
}

// GetClusterInfo returns the overview: versions + context (fatal) merged
// with node / pod / workload counters (best-effort — a failed sub-collection
// leaves its counters at zero rather than failing the overview).
func (s *K8sService) GetClusterInfo(ctx context.Context, sessionID string) (domain.K8sClusterInfo, error) {
	info := domain.K8sClusterInfo{}

	verOut, err := s.runKubectl(ctx, sessionID, "version", "-o", "json")
	if err != nil {
		return info, err
	}
	info = parseK8sVersion(verOut)

	if ctxOut, err := s.runKubectl(ctx, sessionID, "config", "current-context"); err == nil {
		info.Context = strings.TrimSpace(ctxOut)
	}
	if nodeOut, err := s.runKubectl(ctx, sessionID, "get", "nodes", "-o", "json"); err == nil {
		info.NodesReady, info.NodesTotal = parseK8sNodeCounts(nodeOut)
	}
	// Phase counts via a lightweight column projection — the full pod list
	// of a large cluster would be megabytes just for four counters.
	if podOut, err := s.runKubectl(ctx, sessionID, "get", "pods", "--all-namespaces",
		"-o", "custom-columns=PHASE:.status.phase", "--no-headers"); err == nil {
		countPodPhases(podOut, &info)
	}
	for kind, target := range map[string]*int{
		"deployments":  &info.Deployments,
		"statefulsets": &info.StatefulSets,
		"daemonsets":   &info.DaemonSets,
	} {
		if out, err := s.runKubectl(ctx, sessionID, "get", kind, "--all-namespaces",
			"-o", "custom-columns=N:.metadata.name", "--no-headers"); err == nil {
			*target = countNonEmptyLines(out)
		}
	}
	return info, nil
}

// ListNamespaces returns the namespace rows for the panel's picker.
func (s *K8sService) ListNamespaces(ctx context.Context, sessionID string) ([]domain.K8sNamespace, error) {
	out, err := s.runKubectl(ctx, sessionID, "get", "namespaces", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseK8sNamespaces(out), nil
}

// ListNodes returns the cluster nodes (read-only; scheduling-affecting
// actions like cordon/drain are deliberately not offered here), enriched
// with live CPU/memory usage from `kubectl top nodes` when the cluster's
// metrics-server answers — a top failure (API missing, RBAC) degrades to
// zero usage, not an error.
func (s *K8sService) ListNodes(ctx context.Context, sessionID string) ([]domain.K8sNode, error) {
	out, err := s.runKubectl(ctx, sessionID, "get", "nodes", "-o", "json")
	if err != nil {
		return nil, err
	}
	nodes := parseK8sNodes(out)
	if topOut, topErr := s.runKubectl(ctx, sessionID, "top", "nodes", "--no-headers"); topErr == nil {
		applyK8sNodeUsage(nodes, topOut)
	}
	return nodes, nil
}

// k8sWorkloadKinds is the closed allowlist of listable workload kinds —
// ListWorkloads never passes an arbitrary resource type through to the CLI.
var k8sWorkloadKinds = map[string]bool{
	"deployments":  true,
	"statefulsets": true,
	"daemonsets":   true,
}

// ListWorkloads returns one workload kind's rows (kind: the kubectl plural).
func (s *K8sService) ListWorkloads(ctx context.Context, sessionID, kind, namespace string) ([]domain.K8sWorkload, error) {
	if !k8sWorkloadKinds[kind] {
		return nil, fmt.Errorf("kubectl: workload kind %q not allowed", kind)
	}
	ns, err := nsArgs(namespace)
	if err != nil {
		return nil, err
	}
	args := append([]string{"get", kind, "-o", "json"}, ns...)
	out, err := s.runKubectl(ctx, sessionID, args...)
	if err != nil {
		return nil, err
	}
	return parseK8sWorkloads(kind, out), nil
}

// ListPods returns the pod rows for the namespace scope.
func (s *K8sService) ListPods(ctx context.Context, sessionID, namespace string) ([]domain.K8sPod, error) {
	ns, err := nsArgs(namespace)
	if err != nil {
		return nil, err
	}
	args := append([]string{"get", "pods", "-o", "json"}, ns...)
	out, err := s.runKubectl(ctx, sessionID, args...)
	if err != nil {
		return nil, err
	}
	return parseK8sPods(out), nil
}

// ListServices returns the service rows for the namespace scope.
func (s *K8sService) ListServices(ctx context.Context, sessionID, namespace string) ([]domain.K8sServiceInfo, error) {
	ns, err := nsArgs(namespace)
	if err != nil {
		return nil, err
	}
	args := append([]string{"get", "services", "-o", "json"}, ns...)
	out, err := s.runKubectl(ctx, sessionID, args...)
	if err != nil {
		return nil, err
	}
	return parseK8sServices(out), nil
}

// k8sEventLimit bounds the event list (newest kept).
const k8sEventLimit = 200

// k8sYAMLByteCap bounds the fetched / accepted YAML document (the belt-and-
// braces ceiling; typical resources are 2–10 KiB).
const k8sYAMLByteCap = 256 << 10

// k8sYAMLKinds is the closed allowlist of resources the panel may fetch as
// YAML or apply back — everything else (CRDs, cluster-scoped objects) is
// deliberately out of the panel's scope.
var k8sYAMLKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
	"pod":         true,
	"service":     true,
}

// GetResourceYAML returns one resource's live manifest (`kubectl get <kind>
// <name> -n <ns> -o yaml`). A concrete namespace is required — name-based
// retrieval cannot span namespaces; the frontend always sends the list row's.
func (s *K8sService) GetResourceYAML(ctx context.Context, sessionID, kind, namespace, name string) (string, error) {
	if !k8sYAMLKinds[kind] {
		return "", fmt.Errorf("kubectl: kind %q not allowed", kind)
	}
	if err := checkK8sName("namespace", namespace); err != nil {
		return "", err
	}
	if err := checkK8sName("name", name); err != nil {
		return "", err
	}
	out, err := s.runKubectl(ctx, sessionID, "get", kind, name, "-n", namespace, "-o", "yaml")
	if err != nil {
		return "", err
	}
	return capString(out, k8sYAMLByteCap), nil
}

// ApplyResourceYAML submits an edited manifest back to the cluster via
// `kubectl apply -f -` (stdin — no temp files, no argument-size ceiling).
// Before anything reaches the API server, the document's kind/name/namespace
// are checked against the object it was fetched from: a mangled YAML (or an
// edit that dropped the metadata block) is rejected instead of silently
// creating some other object.
func (s *K8sService) ApplyResourceYAML(ctx context.Context, sessionID, kind, namespace, name, doc string) (string, error) {
	if !k8sYAMLKinds[kind] {
		return "", fmt.Errorf("kubectl: kind %q not allowed", kind)
	}
	if err := checkK8sName("name", name); err != nil {
		return "", err
	}
	if namespace != "" {
		if err := checkK8sName("namespace", namespace); err != nil {
			return "", err
		}
	}
	trimmed := strings.TrimSpace(doc)
	if trimmed == "" {
		return "", errors.New("kubectl: empty yaml document")
	}
	if len(trimmed) > k8sYAMLByteCap {
		return "", fmt.Errorf("kubectl: yaml document too large (%d bytes, cap %d)", len(trimmed), k8sYAMLByteCap)
	}

	docKind, docName, docNs := parseYAMLIdentity(trimmed)
	if docKind == "" || docName == "" {
		return "", errors.New("kubectl: 无法从 YAML 识别 kind / metadata.name——请保留顶层 kind 与 metadata 段后再应用 / cannot read kind and metadata.name from the YAML")
	}
	if !strings.EqualFold(docKind, kind) {
		return "", fmt.Errorf("kubectl: YAML 的 kind 是 %s，与目标 %s 不一致 / YAML kind %s does not match target %s", docKind, kind, docKind, kind)
	}
	if docName != name {
		return "", fmt.Errorf("kubectl: YAML 的 metadata.name 是 %s，与目标 %s 不一致 / YAML name %s does not match target %s", docName, name, docName, name)
	}
	if docNs != "" && namespace != "" && docNs != namespace {
		return "", fmt.Errorf("kubectl: YAML 的 metadata.namespace 是 %s，与目标 %s 不一致 / YAML namespace %s does not match target %s", docNs, namespace, docNs, namespace)
	}
	if namespace == "" {
		if docNs == "" {
			return "", errors.New("kubectl: 缺少目标命名空间 / target namespace missing")
		}
		if err := checkK8sName("namespace", docNs); err != nil {
			return "", err
		}
		namespace = docNs
	}

	args := []string{"apply", "-f", "-", "-n", namespace}
	out, err := s.runKubectlStdin(ctx, sessionID, []byte(trimmed), args...)
	if err != nil {
		return "", err
	}
	return capString(out, k8sLogByteCap), nil
}

// YAML identity probes: top-level `kind:` and the metadata block's
// name/namespace (exactly two-space indent — the shape `kubectl get -o yaml`
// emits; deeper name fields in templates/labels never match).
var (
	yamlKindRe     = regexp.MustCompile(`(?m)^kind:[ \t]*["']?([A-Za-z0-9.]+)["']?[ \t]*$`)
	yamlMetaNameRe = regexp.MustCompile(`(?m)^  name:[ \t]*["']?([^"'\s]+)["']?[ \t]*$`)
	yamlMetaNsRe   = regexp.MustCompile(`(?m)^  namespace:[ \t]*["']?([^"'\s]+)["']?[ \t]*$`)
)

// parseYAMLIdentity extracts (kind, metadata.name, metadata.namespace) from
// a manifest. namespace is "" when the document omits it.
func parseYAMLIdentity(doc string) (kind, name, namespace string) {
	if m := yamlKindRe.FindStringSubmatch(doc); m != nil {
		kind = strings.ToLower(m[1])
	}
	if m := yamlMetaNameRe.FindStringSubmatch(doc); m != nil {
		name = m[1]
	}
	if m := yamlMetaNsRe.FindStringSubmatch(doc); m != nil {
		namespace = m[1]
	}
	return kind, name, namespace
}

// ListEvents returns recent events, newest first.
func (s *K8sService) ListEvents(ctx context.Context, sessionID, namespace string) ([]domain.K8sEvent, error) {
	ns, err := nsArgs(namespace)
	if err != nil {
		return nil, err
	}
	args := append([]string{"get", "events", "-o", "json", "--sort-by=.lastTimestamp"}, ns...)
	out, err := s.runKubectl(ctx, sessionID, args...)
	if err != nil {
		return nil, err
	}
	events := parseK8sEvents(out)
	if len(events) > k8sEventLimit {
		events = events[len(events)-k8sEventLimit:]
	}
	return events, nil
}

// k8sLogTailLimits bounds the log window the panel may request (mirrors the
// docker panel's caps).
const (
	k8sLogTailMin = 10
	k8sLogTailMax = 1000
	k8sLogByteCap = 256 << 10
)

// GetPodLogs returns the last `tail` log lines (with timestamps) of one pod,
// optionally one container of it (required by kubectl when the pod runs
// multiple containers — the frontend's container picker guarantees that).
//
// kubectl logs has no --all-namespaces flag, so an empty namespace (the
// panel's "all" scope) resolves the pod's actual namespace first via a
// field-selector lookup; the frontend normally sends the pod's own namespace
// straight from its list row anyway.
func (s *K8sService) GetPodLogs(ctx context.Context, sessionID, namespace, pod, container string, tail int) (string, error) {
	if err := checkK8sName("pod", pod); err != nil {
		return "", err
	}
	if namespace == "" {
		var err error
		namespace, err = s.resolvePodNamespace(ctx, sessionID, pod)
		if err != nil {
			return "", err
		}
	} else if err := checkK8sName("namespace", namespace); err != nil {
		return "", err
	}
	if tail < k8sLogTailMin {
		tail = 100
	}
	if tail > k8sLogTailMax {
		tail = k8sLogTailMax
	}
	args := []string{"logs", pod, "--tail", fmt.Sprintf("%d", tail), "--timestamps", "-n", namespace}
	if container != "" {
		if err := checkK8sName("container", container); err != nil {
			return "", err
		}
		args = append(args, "-c", container)
	}
	out, err := s.runKubectl(ctx, sessionID, args...)
	if err != nil {
		return "", err
	}
	return capString(out, k8sLogByteCap), nil
}

// resolvePodNamespace finds the namespace of one pod by name across the
// cluster (field-selector lookup — cheap and exact). Used when the panel's
// namespace scope is "all" and no explicit namespace is available.
func (s *K8sService) resolvePodNamespace(ctx context.Context, sessionID, pod string) (string, error) {
	out, err := s.runKubectl(ctx, sessionID, "get", "pods", "--all-namespaces",
		"--field-selector", "metadata.name="+pod, "-o", "json")
	if err != nil {
		return "", err
	}
	if ns := firstPodNamespace(out); ns != "" {
		return ns, nil
	}
	return "", fmt.Errorf("kubectl: pod %q not found in cluster", pod)
}

// k8sWorkloadActions is the closed allowlist the panel may trigger —
// WorkloadAction never passes an arbitrary verb through to the CLI.
var k8sWorkloadActions = map[string]bool{
	"scale":   true,
	"restart": true,
}

// k8sScalableKinds: DaemonSets have no replicas semantics (kubectl scale
// rejects them), so scale is only valid for these kinds.
var k8sScalableKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
}

// WorkloadAction performs a lifecycle action on one workload:
//   - scale   → `kubectl scale <kind>/<name> -n <ns> --replicas <n>`
//     (deployment/statefulset only, replicas ≥ 0; 0 = 停止)
//   - restart → `kubectl rollout restart <kind> <name> -n <ns>`
//
// Names/identifiers are validated; nothing free-form reaches the command.
func (s *K8sService) WorkloadAction(ctx context.Context, sessionID, kind, namespace, name, action string, replicas int) (string, error) {
	if !k8sWorkloadActions[action] {
		return "", fmt.Errorf("kubectl: action %q not allowed", action)
	}
	if !k8sWorkloadKinds[kind] {
		return "", fmt.Errorf("kubectl: workload kind %q not allowed", kind)
	}
	if err := checkK8sName("name", name); err != nil {
		return "", err
	}
	ns, err := nsArgs(namespace)
	if err != nil {
		return "", err
	}
	singular := strings.TrimSuffix(kind, "s")
	switch action {
	case "scale":
		if !k8sScalableKinds[singular] {
			return "", fmt.Errorf("kubectl: scale not supported for %s", singular)
		}
		if replicas < 0 {
			return "", errors.New("kubectl: replicas must be >= 0")
		}
		args := append([]string{"scale", singular + "/" + name, "--replicas", fmt.Sprintf("%d", replicas)}, ns...)
		return s.runKubectl(ctx, sessionID, args...)
	default: // restart
		args := append([]string{"rollout", "restart", singular, name}, ns...)
		return s.runKubectl(ctx, sessionID, args...)
	}
}

// DeletePod removes one pod (`--wait=false`: the CLI returns as soon as the
// deletion is accepted instead of blocking through the grace period, which
// would otherwise trip the service timeout). Recreating a stuck pod is the
// panel's "restart" for bare pods; destructive mass operations are out of
// scope by design.
func (s *K8sService) DeletePod(ctx context.Context, sessionID, namespace, name string) (string, error) {
	if err := checkK8sName("pod", name); err != nil {
		return "", err
	}
	ns, err := nsArgs(namespace)
	if err != nil {
		return "", err
	}
	args := append([]string{"delete", "pod", name, "--wait=false"}, ns...)
	return s.runKubectl(ctx, sessionID, args...)
}

// countPodPhases folds the PHASE column projection into the info counters.
func countPodPhases(out string, info *domain.K8sClusterInfo) {
	for _, l := range strings.Split(out, "\n") {
		switch strings.TrimSpace(l) {
		case "Running":
			info.PodsRunning++
		case "Pending":
			info.PodsPending++
		case "Succeeded":
			info.PodsSucceeded++
		case "Failed":
			info.PodsFailed++
		case "":
		default:
			info.PodsUnknown++
		}
	}
}

// countNonEmptyLines counts the non-blank lines of a --no-headers projection.
func countNonEmptyLines(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

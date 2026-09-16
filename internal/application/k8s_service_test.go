package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// fakeK8sConn records every ExecInSessionCtx command so tests can assert the
// exact kubectl invocation; other ConnectionManager methods are unusable stubs.
type fakeK8sConn struct {
	cmds   []string
	stdins [][]byte
	outFor func(cmd string) (string, error)
}

func (f *fakeK8sConn) OpenSession(context.Context, domain.Host, domain.Credentials, int, int, SessionEvents) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeK8sConn) WriteStdin(string, []byte) error { return errors.New("not implemented") }
func (f *fakeK8sConn) Resize(string, int, int) error   { return errors.New("not implemented") }
func (f *fakeK8sConn) Close(string) error              { return nil }
func (f *fakeK8sConn) DetectOS(string) (string, error) { return "", errors.New("not implemented") }
func (f *fakeK8sConn) ExecInSessionCtx(ctx context.Context, sessionID, cmd string) (string, error) {
	return f.ExecInSessionStdin(ctx, sessionID, cmd, nil)
}
func (f *fakeK8sConn) ExecInSessionStdin(_ context.Context, _ string, cmd string, stdin []byte) (string, error) {
	f.cmds = append(f.cmds, cmd)
	f.stdins = append(f.stdins, stdin)
	return f.outFor(cmd)
}
func (f *fakeK8sConn) CloseAll() error { return nil }

var _ ConnectionManager = (*fakeK8sConn)(nil)

// `kubectl logs` has no --all-namespaces: an empty namespace (the panel's
// "all" scope) must resolve the pod's namespace first, then fetch with -n.
func TestGetPodLogsResolvesNamespaceWhenScopeIsAll(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(cmd string) (string, error) {
			if strings.Contains(cmd, "field-selector") {
				return `{"items":[{"metadata":{"name":"web-x7k2","namespace":"prod"}}]}`, nil
			}
			return "2024-06-01T00:00:00Z log line 1\n2024-06-01T00:00:01Z log line 2", nil
		},
	}
	s := &K8sService{connect: conn}

	out, err := s.GetPodLogs(context.Background(), "sess-1", "", "web-x7k2", "", 200)
	if err != nil {
		t.Fatalf("GetPodLogs: %v", err)
	}
	if !strings.Contains(out, "log line 2") {
		t.Fatalf("log content missing: %q", out)
	}
	if len(conn.cmds) != 2 {
		t.Fatalf("expected lookup + logs commands, got %d: %v", len(conn.cmds), conn.cmds)
	}
	if !strings.Contains(conn.cmds[0], "--all-namespaces") ||
		!strings.Contains(conn.cmds[0], "metadata.name=web-x7k2") {
		t.Fatalf("lookup command wrong: %q", conn.cmds[0])
	}
	logsCmd := conn.cmds[1]
	if strings.Contains(logsCmd, "--all-namespaces") {
		t.Fatalf("logs command must never carry --all-namespaces: %q", logsCmd)
	}
	if !strings.Contains(logsCmd, "'-n' 'prod'") || !strings.Contains(logsCmd, "'logs' 'web-x7k2'") {
		t.Fatalf("logs command wrong: %q", logsCmd)
	}
}

// An explicit namespace goes straight to a single logs call.
func TestGetPodLogsExplicitNamespace(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(string) (string, error) { return "logs", nil },
	}
	s := &K8sService{connect: conn}
	if _, err := s.GetPodLogs(context.Background(), "sess-1", "prod", "web", "nginx", 100); err != nil {
		t.Fatalf("GetPodLogs: %v", err)
	}
	if len(conn.cmds) != 1 {
		t.Fatalf("expected a single command, got %v", conn.cmds)
	}
	cmd := conn.cmds[0]
	if strings.Contains(cmd, "--all-namespaces") || !strings.Contains(cmd, "'-n' 'prod'") ||
		!strings.Contains(cmd, "'-c' 'nginx'") {
		t.Fatalf("logs command wrong: %q", cmd)
	}
}

// Failures carry kubectl's real output, not just "exit status 1".
func TestGetPodLogsErrorCarriesOutput(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(string) (string, error) {
			return "Error from server (BadRequest): container \"nginx\" in pod \"web\" is waiting to start",
				errors.New("exit status 1")
		},
	}
	s := &K8sService{connect: conn}
	_, err := s.GetPodLogs(context.Background(), "sess-1", "prod", "web", "nginx", 100)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Error from server (BadRequest)") {
		t.Fatalf("error should carry the kubectl output, got: %v", err)
	}
}

// A pod the lookup cannot find fails with a clear message.
func TestGetPodLogsUnknownPod(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(string) (string, error) { return `{"items":[]}`, nil },
	}
	s := &K8sService{connect: conn}
	_, err := s.GetPodLogs(context.Background(), "sess-1", "", "ghost", "", 100)
	if err == nil || !strings.Contains(err.Error(), "not found in cluster") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestParseYAMLIdentity(t *testing.T) {
	doc := `apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "3"
  creationTimestamp: "2024-03-01T00:00:00Z"
  name: web
  namespace: prod
spec:
  replicas: 3
  template:
    metadata:
      labels:
        app: web
      name: web-deep
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
status: {}
`
	kind, name, ns := parseYAMLIdentity(doc)
	if kind != "deployment" || name != "web" || ns != "prod" {
		t.Fatalf("identity wrong: %q %q %q", kind, name, ns)
	}

	// Missing metadata block → empty identity (apply guard rejects).
	if k, n, _ := parseYAMLIdentity("kind: Pod\nspec: {}\n"); k != "pod" || n != "" {
		t.Fatalf("missing name should yield empty name: %q %q", k, n)
	}
}

func TestApplyResourceYAMLGuards(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(string) (string, error) { return "deployment.apps/web configured", nil },
	}
	s := &K8sService{connect: conn}
	ctx := context.Background()

	const goodDoc = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
spec:
  replicas: 2
`

	// Happy path: apply goes through stdin (`-f -`), doc untouched.
	out, err := s.ApplyResourceYAML(ctx, "sess-1", "deployment", "prod", "web", goodDoc)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "configured") {
		t.Fatalf("apply output wrong: %q", out)
	}
	cmd := conn.cmds[len(conn.cmds)-1]
	if !strings.Contains(cmd, "'apply' '-f' '-'") || !strings.Contains(cmd, "'-n' 'prod'") {
		t.Fatalf("apply command wrong: %q", cmd)
	}
	stdin := conn.stdins[len(conn.stdins)-1]
	if string(stdin) != strings.TrimSpace(goodDoc) {
		t.Fatalf("stdin should be the yaml doc: %q", string(stdin))
	}

	// Kind mismatch (a pod doc for a deployment target) → rejected, no call.
	before := len(conn.cmds)
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "pod", "prod", "web", goodDoc); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("kind mismatch should be rejected, got %v", err)
	}
	// Name mismatch → rejected.
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "deployment", "prod", "api", goodDoc); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("name mismatch should be rejected, got %v", err)
	}
	// Namespace mismatch → rejected.
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "deployment", "default", "web", goodDoc); err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("namespace mismatch should be rejected, got %v", err)
	}
	// Identity-less doc → rejected.
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "deployment", "prod", "web", "replicas: 2\n"); err == nil {
		t.Fatal("identity-less doc should be rejected")
	}
	// Disallowed kind → rejected even before parsing.
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "secret", "prod", "x", goodDoc); err == nil {
		t.Fatal("disallowed kind should be rejected")
	}
	// Empty doc → rejected.
	if _, err := s.ApplyResourceYAML(ctx, "sess-1", "deployment", "prod", "web", "   \n"); err == nil {
		t.Fatal("empty doc should be rejected")
	}
	if len(conn.cmds) != before {
		t.Fatalf("rejected applies must not reach the CLI: %d extra calls", len(conn.cmds)-before)
	}
}

func TestGetResourceYAMLCommand(t *testing.T) {
	conn := &fakeK8sConn{
		outFor: func(string) (string, error) {
			return "apiVersion: v1\nkind: Service\nmetadata:\n  name: web\n", nil
		},
	}
	s := &K8sService{connect: conn}
	out, err := s.GetResourceYAML(context.Background(), "sess-1", "service", "prod", "web")
	if err != nil {
		t.Fatalf("GetResourceYAML: %v", err)
	}
	if !strings.Contains(out, "kind: Service") {
		t.Fatalf("yaml content wrong: %q", out)
	}
	if cmd := conn.cmds[0]; !strings.Contains(cmd, "'get' 'service' 'web'") || !strings.Contains(cmd, "'-o' 'yaml'") {
		t.Fatalf("get command wrong: %q", cmd)
	}

	// Namespace is mandatory (name-based retrieval cannot span namespaces).
	if _, err := s.GetResourceYAML(context.Background(), "sess-1", "pod", "", "web"); err == nil {
		t.Fatal("empty namespace should be rejected")
	}
	// Disallowed kinds (CRDs etc.) never reach the CLI.
	if _, err := s.GetResourceYAML(context.Background(), "sess-1", "crontabs", "prod", "x"); err == nil {
		t.Fatal("disallowed kind should be rejected")
	}
}

// The action/kind/identifier guards run before any CLI call, so a bare
// service (nil connection manager) is enough to exercise them.
func TestWorkloadActionGuards(t *testing.T) {
	s := &K8sService{}
	ctx := context.Background()

	cases := []struct {
		kind, ns, name, action string
		replicas               int
		wantErr                string
	}{
		{"deployments", "default", "web", "kill", 1, "action"},                 // unknown action
		{"replicasets", "default", "web", "restart", 0, "kind"},                // kind outside the panel scope
		{"pods", "default", "web", "scale", 1, "kind"},                         // singular check rides on the kind allowlist
		{"daemonsets", "default", "ds", "scale", 2, "scale"},                   // daemonsets have no replicas semantics
		{"deployments", "default", "--kubeconfig=/x", "restart", 0, "invalid"}, // flag injection
		{"deployments", "--all", "web", "scale", 1, "invalid"},                 // namespace must be a real name
		{"deployments", "default", "web", "scale", -1, "replicas"},             // negative replicas
	}
	for _, c := range cases {
		_, err := s.WorkloadAction(ctx, "sess-1", c.kind, c.ns, c.name, c.action, c.replicas)
		if err == nil {
			t.Errorf("%+v should be rejected", c)
			continue
		}
		if !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%+v: error %q should mention %q", c, err.Error(), c.wantErr)
		}
	}
}

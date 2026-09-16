package application

import (
	"context"
	"strings"
	"testing"
)

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

package application

import (
	"strings"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// Fixtures use realistic kubectl `-o json` shapes (metadata/spec/status).

func TestParseK8sVersion(t *testing.T) {
	out := `{"clientVersion":{"major":"1","minor":"28","gitVersion":"v1.28.2"},"kustomizeVersion":"v5","serverVersion":{"major":"1","minor":"27","gitVersion":"v1.27.3"}}`
	info := parseK8sVersion(out + "\nstderr noise")
	if info.ClientVersion != "v1.28.2" || info.ServerVersion != "v1.27.3" {
		t.Fatalf("versions wrong: %+v", info)
	}
	// Unparsable output degrades to an empty info, not a panic.
	if got := parseK8sVersion("not json at all"); got.ServerVersion != "" || got.ClientVersion != "" {
		t.Fatalf("bad output should degrade: %+v", got)
	}
}

const k8sNodesFixture = `{"items":[
 {"metadata":{"name":"cp-1","creationTimestamp":"2024-01-01T00:00:00Z","labels":{"node-role.kubernetes.io/control-plane":""}},
  "status":{"conditions":[{"type":"MemoryPressure","status":"False"},{"type":"Ready","status":"True"}],
   "addresses":[{"type":"Hostname","address":"cp-1"},{"type":"InternalIP","address":"10.0.0.1"}],
   "nodeInfo":{"kubeletVersion":"v1.27.3","osImage":"Ubuntu 22.04"}}},
 {"metadata":{"name":"worker-1","creationTimestamp":"2024-02-01T00:00:00Z","labels":{}},
  "status":{"conditions":[{"type":"Ready","status":"False"}],
   "addresses":[{"type":"InternalIP","address":"10.0.0.2"}],
   "nodeInfo":{"kubeletVersion":"v1.27.3","osImage":"Ubuntu 22.04"}}}
]}`

func TestParseK8sNodes(t *testing.T) {
	nodes := parseK8sNodes(k8sNodesFixture)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	// Sorted by name: cp-1 first, worker-1 second.
	if nodes[0].Name != "cp-1" || nodes[1].Name != "worker-1" {
		t.Fatalf("order wrong: %+v", nodes)
	}
	n := nodes[0]
	if n.Status != "Ready" || n.InternalIP != "10.0.0.1" || n.Roles != "control-plane" ||
		n.Version != "v1.27.3" || !strings.Contains(n.OS, "Ubuntu") {
		t.Fatalf("cp-1 fields wrong: %+v", n)
	}
	if nodes[1].Status != "NotReady" || nodes[1].Roles != "" {
		t.Fatalf("worker-1 fields wrong: %+v", nodes[1])
	}

	ready, total := parseK8sNodeCounts(k8sNodesFixture)
	if ready != 1 || total != 2 {
		t.Fatalf("counts wrong: %d/%d", ready, total)
	}
}

func TestParseK8sNamespaces(t *testing.T) {
	out := `{"items":[
	 {"metadata":{"name":"kube-system"},"status":{"phase":"Active"}},
	 {"metadata":{"name":"default"},"status":{"phase":"Active"}},
	 {"metadata":{"name":"old"},"status":{"phase":"Terminating"}}]}`
	ns := parseK8sNamespaces(out)
	if len(ns) != 3 || ns[0].Name != "default" || ns[2].Status != "Terminating" {
		t.Fatalf("namespaces wrong: %+v", ns)
	}
}

const k8sDeploymentsFixture = `{"items":[
 {"metadata":{"name":"web","namespace":"prod","creationTimestamp":"2024-03-01T00:00:00Z"},
  "spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"nginx","image":"nginx:1.25"},{"name":"sidecar","image":"busybox"}]}}},
  "status":{"replicas":3,"readyReplicas":2,"updatedReplicas":3,"availableReplicas":2}},
 {"metadata":{"name":"api","namespace":"prod"},
  "spec":{"template":{"spec":{"containers":[{"name":"api","image":"api:v2"}]}}},
  "status":{"replicas":1,"readyReplicas":1}}]}`

func TestParseK8sWorkloads(t *testing.T) {
	rows := parseK8sWorkloads("deployments", k8sDeploymentsFixture)
	if len(rows) != 2 {
		t.Fatalf("expected 2 workloads, got %d", len(rows))
	}
	// Sorted by namespace then name: api first, web second.
	w := rows[1]
	if w.Kind != "Deployment" || w.Namespace != "prod" || w.Name != "web" {
		t.Fatalf("identity wrong: %+v", w)
	}
	if w.Replicas != 3 || w.Ready != 2 || w.Updated != 3 || w.Available != 2 {
		t.Fatalf("counts wrong: %+v", w)
	}
	if len(w.Images) != 2 || w.Images[0] != "nginx:1.25" {
		t.Fatalf("images wrong: %v", w.Images)
	}
	if rows[0].Name != "api" || rows[0].Ready != 1 {
		t.Fatalf("api row wrong: %+v", rows[0])
	}

	ds := parseK8sWorkloads("daemonsets", `{"items":[
	 {"metadata":{"name":"node-exp","namespace":"kube-system"},
	  "spec":{"template":{"spec":{"containers":[{"name":"agent","image":"agent:1"}]}}},
	  "status":{"desiredNumberScheduled":5,"numberReady":4,"updatedNumberScheduled":5,"numberAvailable":4}}]}`)
	if len(ds) != 1 {
		t.Fatalf("expected 1 daemonset, got %d", len(ds))
	}
	d := ds[0]
	if d.Kind != "DaemonSet" || d.Replicas != 5 || d.Ready != 4 || d.Updated != 5 || d.Available != 4 {
		t.Fatalf("daemonset counts wrong: %+v", d)
	}
}

const k8sPodsFixture = `{"items":[
 {"metadata":{"name":"web-x7k2","namespace":"prod","creationTimestamp":"2024-05-01T00:00:00Z"},
  "spec":{"nodeName":"worker-1","containers":[{"name":"nginx"},{"name":"sidecar"}]},
  "status":{"phase":"Running","podIP":"10.244.1.5",
   "containerStatuses":[{"name":"nginx","ready":true,"restartCount":2,"state":{"running":{}}},
                        {"name":"sidecar","ready":false,"restartCount":3,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
 {"metadata":{"name":"job-done","namespace":"batch"},
  "spec":{"nodeName":"worker-1","containers":[{"name":"runner"}]},
  "status":{"phase":"Succeeded","containerStatuses":[{"name":"runner","ready":false,"restartCount":0}]}},
 {"metadata":{"name":"pending-pod","namespace":"prod"},
  "spec":{"containers":[{"name":"app"}]},
  "status":{"phase":"Pending"}}]}`

func TestParseK8sPods(t *testing.T) {
	pods := parseK8sPods(k8sPodsFixture)
	if len(pods) != 3 {
		t.Fatalf("expected 3 pods, got %d", len(pods))
	}
	// Sorted by namespace then name: batch/job-done, prod/pending-pod, prod/web-x7k2.
	if pods[0].Name != "job-done" || pods[1].Name != "pending-pod" || pods[2].Name != "web-x7k2" {
		t.Fatalf("order wrong: %s %s %s", pods[0].Name, pods[1].Name, pods[2].Name)
	}
	p := pods[2]
	if p.Status != "CrashLoopBackOff" {
		t.Fatalf("waiting reason should surface as status, got %q", p.Status)
	}
	if p.Ready != "1/2" || p.Restarts != 5 || p.IP != "10.244.1.5" || p.Node != "worker-1" {
		t.Fatalf("pod fields wrong: %+v", p)
	}
	if len(p.Containers) != 2 || p.Containers[0] != "nginx" {
		t.Fatalf("container names wrong: %v", p.Containers)
	}
	// Pending with no container statuses displays ContainerCreating.
	if pods[1].Status != "ContainerCreating" {
		t.Fatalf("pending status wrong: %q", pods[1].Status)
	}
	if pods[0].Status != "Succeeded" {
		t.Fatalf("succeeded status wrong: %q", pods[0].Status)
	}
}

func TestParseK8sServices(t *testing.T) {
	out := `{"items":[
	 {"metadata":{"name":"web","namespace":"prod","creationTimestamp":"2024-01-05T00:00:00Z"},
	  "spec":{"type":"NodePort","clusterIP":"10.96.0.10","selector":{"app":"web"},
	   "ports":[{"port":80,"nodePort":30080,"protocol":"TCP"},{"port":443,"protocol":"TCP"}]}},
	 {"metadata":{"name":"ingress-lb","namespace":"ingress"},
	  "spec":{"type":"LoadBalancer","clusterIP":"10.96.0.11","ports":[{"port":80,"protocol":"TCP"}]},
	  "status":{"loadBalancer":{"ingress":[{"ip":"203.0.113.7"}]}}}]}`
	svcs := parseK8sServices(out)
	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	// Sorted by namespace then name: ingress/ingress-lb first, prod/web second.
	s := svcs[1]
	if s.Name != "web" || s.Type != "NodePort" || s.Ports != "80:30080/TCP,443/TCP" || s.ClusterIP != "10.96.0.10" {
		t.Fatalf("service fields wrong: %+v", s)
	}
	if s.Selector["app"] != "web" {
		t.Fatalf("selector wrong: %v", s.Selector)
	}
	if svcs[0].ExternalIP != "203.0.113.7" || svcs[0].Type != "LoadBalancer" {
		t.Fatalf("external ip wrong: %+v", svcs[0])
	}
}

func TestParseK8sEvents(t *testing.T) {
	out := `{"items":[
	 {"metadata":{"namespace":"prod"},"involvedObject":{"kind":"Pod","name":"web-x7k2"},"reason":"BackOff",
	  "message":"Back-off restarting failed container","type":"Warning","count":7,
	  "lastTimestamp":"2024-06-02T10:00:00Z"},
	 {"metadata":{"namespace":"prod"},"involvedObject":{"kind":"Deployment","name":"web"},"reason":"ScalingReplicaSet",
	  "message":"Scaled up replica set to 3","type":"Normal","count":1,"lastTimestamp":"2024-06-01T09:00:00Z"}]}`
	events := parseK8sEvents(out)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Newest first.
	if events[0].Reason != "BackOff" || events[1].Reason != "ScalingReplicaSet" {
		t.Fatalf("sort wrong: %+v", events)
	}
	e := events[0]
	if e.Object != "Pod/web-x7k2" || e.Count != 7 || e.Type != "Warning" {
		t.Fatalf("event fields wrong: %+v", e)
	}
}

func TestHumanizeAge(t *testing.T) {
	now := time.Now()
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "10s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{12 * 24 * time.Hour, "12d"},
		{400 * 24 * time.Hour, "1y"},
	}
	for _, c := range cases {
		ts := now.Add(-c.ago).UTC().Format(time.RFC3339)
		if got := humanizeAge(ts); got != c.want {
			t.Errorf("age(%s) = %q, want %q", ts, got, c.want)
		}
	}
	if got := humanizeAge(""); got != "" {
		t.Errorf("empty timestamp should yield empty age, got %q", got)
	}
	if got := humanizeAge("garbage"); got != "" {
		t.Errorf("bad timestamp should yield empty age, got %q", got)
	}
}

func TestCheckK8sName(t *testing.T) {
	valid := []string{"default", "kube-system", "web-x7k2", "app.example.com", "My_App.1"}
	for _, v := range valid {
		if err := checkK8sName("name", v); err != nil {
			t.Errorf("%q should be valid, got %v", v, err)
		}
	}
	// Flag injection and shell metacharacters must be rejected.
	invalid := []string{"", "--kubeconfig=/etc/passwd", "-n", "a b", "a;b", "a'b", "x$(y)", "a\nb"}
	for _, v := range invalid {
		if err := checkK8sName("name", v); err == nil {
			t.Errorf("%q should be rejected", v)
		}
	}
}

func TestCountPodPhasesAndLines(t *testing.T) {
	var info domain.K8sClusterInfo
	countPodPhases("Running\nPending\nRunning\nFailed\nSucceeded\nEvicted\n\n", &info)
	if info.PodsRunning != 2 || info.PodsPending != 1 || info.PodsFailed != 1 ||
		info.PodsSucceeded != 1 || info.PodsUnknown != 1 {
		t.Fatalf("phase counts wrong: %+v", info)
	}
	if n := countNonEmptyLines("web\napi\n\nnginx\n"); n != 3 {
		t.Fatalf("line count wrong: %d", n)
	}
}

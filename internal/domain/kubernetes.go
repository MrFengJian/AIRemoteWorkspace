package domain

// Kubernetes data models, collected by invoking the kubectl CLI on the host
// behind the session (over the SSH exec channel) or on the local machine,
// always with `-o json` output — no SDK, no direct API connection. The panel
// covers the common workload resources only; CRDs and other free-form types
// are deliberately out of scope.

// K8sClusterInfo is the panel overview: kubectl version merged with node /
// pod / workload counters. Best-effort fields degrade to empty / 0 when a
// sub-collection fails (only the version call is fatal).
type K8sClusterInfo struct {
	ClientVersion string `json:"clientVersion"`
	ServerVersion string `json:"serverVersion"`
	// Context is the kubeconfig context kubectl resolves on the host.
	Context     string `json:"context"`
	NodesReady  int    `json:"nodesReady"`
	NodesTotal  int    `json:"nodesTotal"`
	PodsRunning   int `json:"podsRunning"`
	PodsPending   int `json:"podsPending"`
	PodsSucceeded int `json:"podsSucceeded"`
	PodsFailed    int `json:"podsFailed"`
	PodsUnknown   int `json:"podsUnknown"`
	Deployments   int `json:"deployments"`
	StatefulSets  int `json:"statefulSets"`
	DaemonSets    int `json:"daemonSets"`
}

// K8sNode is one cluster node (`kubectl get nodes -o json`), enriched with
// live usage from `kubectl top nodes` when the cluster's metrics-server is
// available (usage fields stay empty otherwise — the UI degrades to a hint).
type K8sNode struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "Ready" | "NotReady" | "Unknown"
	Roles      string `json:"roles"`  // comma-joined, e.g. "control-plane"
	Version    string `json:"version"`
	InternalIP string `json:"internalIp"`
	OS         string `json:"os"` // OS image, e.g. "Ubuntu 22.04"
	Age        string `json:"age"`
	// Allocatable + capacity resources from the node status (raw kubectl
	// values, e.g. "4" cores / "15880Mi"). Visible even without metrics.
	AllocatableCPU string `json:"allocatableCpu"`
	AllocatableMem string `json:"allocatableMem"`
	CapacityCPU    string `json:"capacityCpu"`
	CapacityMem    string `json:"capacityMem"`
	PodCapacity    string `json:"podCapacity"` // allocatable pods
	// Live usage (kubectl top). CPUUsed/MemUsed keep kubectl's formatted
	// values ("1200m", "8192Mi"); percents are numbers for the usage bars.
	CPUUsed    string  `json:"cpuUsed"`
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    string  `json:"memUsed"`
	MemPercent float64 `json:"memPercent"`
}

// K8sWorkload is one workload-controller row. Replicas/Ready/Updated/
// Available follow `kubectl get` columns; for DaemonSets, Replicas/Ready map
// to desired/ready scheduled counts and Updated/Available stay 0 (no scale
// semantics on the kind).
type K8sWorkload struct {
	Kind      string   `json:"kind"` // "Deployment" | "StatefulSet" | "DaemonSet"
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Replicas  int      `json:"replicas"`
	Ready     int      `json:"ready"`
	Updated   int      `json:"updated"`
	Available int      `json:"available"`
	Age       string   `json:"age"`
	Images    []string `json:"images"`
}

// K8sPod is one pod row. Status prefers a concrete waiting reason (e.g.
// "CrashLoopBackOff", "ContainerCreating") over the bare phase, matching
// what `kubectl get pods` shows.
type K8sPod struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Status     string   `json:"status"` // "Running" | "Pending" | "CrashLoopBackOff" | "Evicted" | …
	Ready      string   `json:"ready"`  // "1/2"
	Restarts   int      `json:"restarts"`
	Age        string   `json:"age"`
	Node       string   `json:"node"`
	IP         string   `json:"ip"`
	Containers []string `json:"containers"` // spec.container names (log picker)
}

// K8sServiceInfo is one service row (`kubectl get services -o json`).
type K8sServiceInfo struct {
	Namespace  string            `json:"namespace"`
	Name       string            `json:"name"`
	Type       string            `json:"type"` // "ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
	ClusterIP  string            `json:"clusterIp"`
	ExternalIP string            `json:"externalIp"`
	Ports      string            `json:"ports"` // "80:30080/TCP,443/TCP"
	Age        string            `json:"age"`
	Selector   map[string]string `json:"selector"`
}

// K8sEvent is one cluster event, newest first.
type K8sEvent struct {
	Namespace     string `json:"namespace"`
	Type          string `json:"type"` // "Normal" | "Warning"
	Reason        string `json:"reason"`
	Object        string `json:"object"` // "Pod/nginx-abc"
	Message       string `json:"message"`
	Count         int    `json:"count"`
	LastTimestamp string `json:"lastTimestamp"`
	Age           string `json:"age"`
}

// K8sNamespace is one namespace row for the panel's namespace picker.
type K8sNamespace struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "Active" | "Terminating"
}

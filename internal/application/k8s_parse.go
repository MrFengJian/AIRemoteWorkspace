package application

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// k8s_parse.go turns kubectl's `-o json` documents (occasionally interleaved
// with stderr noise, since exec channels are combined) into domain models.
// The shapes below are lean projections of the stable Kubernetes API objects
// — unknown fields are ignored by encoding/json, and unparsable items are
// skipped rather than failing the whole list.

// k8sListDoc is the envelope of every `kubectl get -o json` call.
type k8sListDoc struct {
	Items []json.RawMessage `json:"items"`
}

// jsonDoc extracts the outermost {...} block of out — combined exec output
// may carry stderr noise around the JSON document (the same tolerance the
// docker panel's line filter provides). Returns nil when no block exists.
func jsonDoc(out string) []byte {
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return nil
	}
	return []byte(out[start : end+1])
}

// k8sObjectMeta is the metadata slice every object carries.
type k8sObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp string            `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
}

// parseK8sVersion decodes `kubectl version -o json` into the info header.
func parseK8sVersion(out string) domain.K8sClusterInfo {
	info := domain.K8sClusterInfo{}
	var doc struct {
		ClientVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if json.Unmarshal(jsonDoc(out), &doc) != nil {
		return info
	}
	info.ClientVersion = doc.ClientVersion.GitVersion
	info.ServerVersion = doc.ServerVersion.GitVersion
	return info
}

// parseK8sNodeCounts returns (ready, total) from `kubectl get nodes -o json`.
func parseK8sNodeCounts(out string) (ready, total int) {
	nodes := parseK8sNodes(out)
	for _, n := range nodes {
		if n.Status == "Ready" {
			ready++
		}
	}
	return ready, len(nodes)
}

type k8sNodeItem struct {
	Metadata k8sObjectMeta `json:"metadata"`
	Status   struct {
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		Allocatable map[string]string `json:"allocatable"`
		NodeInfo struct {
			KubeletVersion string `json:"kubeletVersion"`
			OSImage        string `json:"osImage"`
		} `json:"nodeInfo"`
	} `json:"status"`
}

// parseK8sNodes decodes the node list.
func parseK8sNodes(out string) []domain.K8sNode {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	nodes := make([]domain.K8sNode, 0, len(list.Items))
	for _, raw := range list.Items {
		var it k8sNodeItem
		if json.Unmarshal(raw, &it) != nil || it.Metadata.Name == "" {
			continue
		}
		n := domain.K8sNode{
			Name:           it.Metadata.Name,
			Status:         "Unknown",
			OS:             it.Status.NodeInfo.OSImage,
			Version:        it.Status.NodeInfo.KubeletVersion,
			Age:            humanizeAge(it.Metadata.CreationTimestamp),
			AllocatableCPU: it.Status.Allocatable["cpu"],
			AllocatableMem: it.Status.Allocatable["memory"],
		}
		var roles []string
		for label := range it.Metadata.Labels {
			if suffix, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok && suffix != "" {
				roles = append(roles, suffix)
			}
		}
		sort.Strings(roles)
		n.Roles = strings.Join(roles, ",")
		for _, c := range it.Status.Conditions {
			if c.Type == "Ready" {
				if c.Status == "True" {
					n.Status = "Ready"
				} else {
					n.Status = "NotReady"
				}
			}
		}
		for _, a := range it.Status.Addresses {
			if a.Type == "InternalIP" {
				n.InternalIP = a.Address
			}
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

// parseK8sNamespaces decodes the namespace list for the picker.
func parseK8sNamespaces(out string) []domain.K8sNamespace {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	out2 := make([]domain.K8sNamespace, 0, len(list.Items))
	for _, raw := range list.Items {
		var it struct {
			Metadata k8sObjectMeta `json:"metadata"`
			Status   struct {
				Phase string `json:"phase"`
			} `json:"status"`
		}
		if json.Unmarshal(raw, &it) != nil || it.Metadata.Name == "" {
			continue
		}
		out2 = append(out2, domain.K8sNamespace{Name: it.Metadata.Name, Status: it.Status.Phase})
	}
	sort.Slice(out2, func(i, j int) bool { return out2[i].Name < out2[j].Name })
	return out2
}

// k8sContainerNames extracts spec.container names (the log picker).
func k8sContainerNames(containers []struct {
	Name string `json:"name"`
}) []string {
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		names = append(names, c.Name)
	}
	return names
}

type k8sWorkloadItem struct {
	Metadata k8sObjectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name  string `json:"name"`
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas               int `json:"replicas"`
		ReadyReplicas          int `json:"readyReplicas"`
		UpdatedReplicas        int `json:"updatedReplicas"`
		AvailableReplicas      int `json:"availableReplicas"`
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		UpdatedNumberScheduled int `json:"updatedNumberScheduled"`
		NumberAvailable        int `json:"numberAvailable"`
	} `json:"status"`
}

// kindDisplayName maps the kubectl plural to the display kind.
func kindDisplayName(plural string) string {
	switch plural {
	case "deployments":
		return "Deployment"
	case "statefulsets":
		return "StatefulSet"
	case "daemonsets":
		return "DaemonSet"
	default:
		return plural
	}
}

// parseK8sWorkloads decodes one workload kind's list. DaemonSets derive
// Replicas/Ready from desired/ready scheduled counts and leave Updated/
// Available at their status fields (they have no spec.replicas semantics).
func parseK8sWorkloads(kind, out string) []domain.K8sWorkload {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	rows := make([]domain.K8sWorkload, 0, len(list.Items))
	for _, raw := range list.Items {
		var it k8sWorkloadItem
		if json.Unmarshal(raw, &it) != nil || it.Metadata.Name == "" {
			continue
		}
		w := domain.K8sWorkload{
			Kind:      kindDisplayName(kind),
			Namespace: it.Metadata.Namespace,
			Name:      it.Metadata.Name,
			Age:       humanizeAge(it.Metadata.CreationTimestamp),
		}
		images := make([]string, 0, len(it.Spec.Template.Spec.Containers))
		for _, c := range it.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		w.Images = images
		switch kind {
		case "daemonsets":
			w.Replicas = it.Status.DesiredNumberScheduled
			w.Ready = it.Status.NumberReady
			w.Updated = it.Status.UpdatedNumberScheduled
			w.Available = it.Status.NumberAvailable
		default:
			if it.Spec.Replicas != nil {
				w.Replicas = *it.Spec.Replicas
			} else {
				w.Replicas = it.Status.Replicas
			}
			w.Ready = it.Status.ReadyReplicas
			w.Updated = it.Status.UpdatedReplicas
			w.Available = it.Status.AvailableReplicas
		}
		rows = append(rows, w)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Namespace != rows[j].Namespace {
			return rows[i].Namespace < rows[j].Namespace
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

type k8sPodItem struct {
	Metadata k8sObjectMeta `json:"metadata"`
	Spec     struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		Reason            string `json:"reason"`
		PodIP             string `json:"podIP"`
		ContainerStatuses []struct {
			Name        string `json:"name"`
			Ready       bool   `json:"ready"`
			RestartCount int   `json:"restartCount"`
			State       struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

// parseK8sPods decodes the pod list. The display status prefers the concrete
// reason (Evicted / CrashLoopBackOff / ImagePullBackOff / ContainerCreating)
// over the bare phase — the same thing `kubectl get pods` shows.
func parseK8sPods(out string) []domain.K8sPod {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	pods := make([]domain.K8sPod, 0, len(list.Items))
	for _, raw := range list.Items {
		var it k8sPodItem
		if json.Unmarshal(raw, &it) != nil || it.Metadata.Name == "" {
			continue
		}
		status := it.Status.Phase
		if it.Status.Reason != "" {
			status = it.Status.Reason
		} else {
			for _, cs := range it.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
					status = cs.State.Waiting.Reason
					break
				}
			}
			if status == "Pending" && len(it.Status.ContainerStatuses) == 0 {
				status = "ContainerCreating"
			}
		}
		ready, restarts := 0, 0
		for _, cs := range it.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
			restarts += cs.RestartCount
		}
		pods = append(pods, domain.K8sPod{
			Namespace:  it.Metadata.Namespace,
			Name:       it.Metadata.Name,
			Status:     status,
			Ready:      fmt.Sprintf("%d/%d", ready, len(it.Spec.Containers)),
			Restarts:   restarts,
			Age:        humanizeAge(it.Metadata.CreationTimestamp),
			Node:       it.Spec.NodeName,
			IP:         it.Status.PodIP,
			Containers: k8sContainerNames(it.Spec.Containers),
		})
	}
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	return pods
}

type k8sServiceItem struct {
	Metadata k8sObjectMeta `json:"metadata"`
	Spec     struct {
		Type      string            `json:"type"`
		ClusterIP string            `json:"clusterIP"`
		Selector  map[string]string `json:"selector"`
		Ports []struct {
			Port     int    `json:"port"`
			NodePort int    `json:"nodePort"`
			Protocol string `json:"protocol"`
		} `json:"ports"`
		ExternalIPs []string `json:"externalIPs"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

// parseK8sServices decodes the service list; Ports renders kubectl-style
// ("80:30080/TCP,443/TCP").
func parseK8sServices(out string) []domain.K8sServiceInfo {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	svcs := make([]domain.K8sServiceInfo, 0, len(list.Items))
	for _, raw := range list.Items {
		var it k8sServiceItem
		if json.Unmarshal(raw, &it) != nil || it.Metadata.Name == "" {
			continue
		}
		var portParts []string
		for _, p := range it.Spec.Ports {
			proto := p.Protocol
			if proto == "" {
				proto = "TCP"
			}
			if p.NodePort > 0 {
				portParts = append(portParts, fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, proto))
			} else {
				portParts = append(portParts, fmt.Sprintf("%d/%s", p.Port, proto))
			}
		}
		var external []string
		for _, ing := range it.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				external = append(external, ing.IP)
			} else if ing.Hostname != "" {
				external = append(external, ing.Hostname)
			}
		}
		external = append(external, it.Spec.ExternalIPs...)
		svcs = append(svcs, domain.K8sServiceInfo{
			Namespace:  it.Metadata.Namespace,
			Name:       it.Metadata.Name,
			Type:       it.Spec.Type,
			ClusterIP:  it.Spec.ClusterIP,
			ExternalIP: strings.Join(external, ","),
			Ports:      strings.Join(portParts, ","),
			Age:        humanizeAge(it.Metadata.CreationTimestamp),
			Selector:   it.Spec.Selector,
		})
	}
	sort.Slice(svcs, func(i, j int) bool {
		if svcs[i].Namespace != svcs[j].Namespace {
			return svcs[i].Namespace < svcs[j].Namespace
		}
		return svcs[i].Name < svcs[j].Name
	})
	return svcs
}

type k8sEventItem struct {
	Metadata       k8sObjectMeta `json:"metadata"`
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Count   int    `json:"count"`
	// lastTimestamp covers core events; eventTime covers the newer
	// EventTime-based ones. Either may be absent.
	LastTimestamp  string `json:"lastTimestamp"`
	FirstTimestamp string `json:"firstTimestamp"`
	EventTime      string `json:"eventTime"`
}

// parseK8sEvents decodes the event list, newest first (the --sort-by hint is
// server-side; the client re-sorts defensively so the panel never shows a
// reversed window).
func parseK8sEvents(out string) []domain.K8sEvent {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return nil
	}
	events := make([]domain.K8sEvent, 0, len(list.Items))
	for _, raw := range list.Items {
		var it k8sEventItem
		if json.Unmarshal(raw, &it) != nil || it.Reason == "" && it.Message == "" {
			continue
		}
		last := it.LastTimestamp
		if last == "" {
			last = it.EventTime
		}
		if last == "" {
			last = it.FirstTimestamp
		}
		events = append(events, domain.K8sEvent{
			Namespace:     it.Metadata.Namespace,
			Type:          it.Type,
			Reason:        it.Reason,
			Object:        strings.TrimSpace(it.InvolvedObject.Kind + "/" + it.InvolvedObject.Name),
			Message:       it.Message,
			Count:         it.Count,
			LastTimestamp: last,
			Age:           humanizeAge(last),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].LastTimestamp > events[j].LastTimestamp
	})
	return events
}

// firstPodNamespace returns the namespace of the first pod in a
// `kubectl get pods -o json` document (the namespace-resolution lookup used
// by the log viewer when the panel scope is "all").
func firstPodNamespace(out string) string {
	var list k8sListDoc
	if json.Unmarshal(jsonDoc(out), &list) != nil {
		return ""
	}
	for _, raw := range list.Items {
		var it struct {
			Metadata k8sObjectMeta `json:"metadata"`
		}
		if json.Unmarshal(raw, &it) == nil && it.Metadata.Namespace != "" {
			return it.Metadata.Namespace
		}
	}
	return ""
}

// applyK8sNodeUsage merges `kubectl top nodes --no-headers` output
// (NAME CPU(cores) CPU% MEMORY(bytes) MEMORY%) into the node rows. Noise
// lines (warnings, the header itself) are skipped; nodes without a top row
// keep zero usage — the UI then shows a metrics-unavailable hint instead of
// misleading zero bars.
func applyK8sNodeUsage(nodes []domain.K8sNode, topOut string) {
	usage := make(map[string]k8sNodeUsage)
	for _, line := range strings.Split(topOut, "\n") {
		f := strings.Fields(line)
		if len(f) != 5 || f[0] == "NAME" {
			continue
		}
		usage[f[0]] = k8sNodeUsage{
			CPUUsed:    f[1],
			CPUPercent: parsePercent(f[2]),
			MemUsed:    f[3],
			MemPercent: parsePercent(f[4]),
		}
	}
	for i := range nodes {
		if u, ok := usage[nodes[i].Name]; ok {
			nodes[i].CPUUsed = u.CPUUsed
			nodes[i].CPUPercent = u.CPUPercent
			nodes[i].MemUsed = u.MemUsed
			nodes[i].MemPercent = u.MemPercent
		}
	}
}

// k8sNodeUsage is one row of `kubectl top nodes`.
type k8sNodeUsage struct {
	CPUUsed    string
	CPUPercent float64
	MemUsed    string
	MemPercent float64
}

// parsePercent parses "30%" (and bare numbers) into a float; junk → 0.
func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// humanizeAge renders a creation timestamp kubectl-style (35s / 5m / 3h /
// 12d / 1y). Empty or unparseable timestamps yield "".
func humanizeAge(timestamp string) string {
	if timestamp == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

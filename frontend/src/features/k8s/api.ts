// K8s feature API — thin wrappers over the generated K8sService bindings.
// Reads and actions run kubectl on the backend: over the session's SSH exec
// channel for remote tabs, or the local kubectl CLI for local terminals.
import { K8sService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

/** Workload kinds the panel lists (backend allowlist; kubectl plurals). */
export type K8sWorkloadKind = "deployments" | "statefulsets" | "daemonsets";

/** Workload lifecycle actions the panel may trigger (backend allowlist). */
export type K8sAction = "scale" | "restart";

export const k8sApi = {
  clusterInfo: (sessionID: string) => K8sService.GetClusterInfo(sessionID),
  namespaces: (sessionID: string) => K8sService.ListNamespaces(sessionID),
  nodes: (sessionID: string) => K8sService.ListNodes(sessionID),
  workloads: (sessionID: string, kind: K8sWorkloadKind, namespace: string) =>
    K8sService.ListWorkloads(sessionID, kind, namespace),
  pods: (sessionID: string, namespace: string) => K8sService.ListPods(sessionID, namespace),
  services: (sessionID: string, namespace: string) => K8sService.ListServices(sessionID, namespace),
  events: (sessionID: string, namespace: string) => K8sService.ListEvents(sessionID, namespace),
  logs: (sessionID: string, namespace: string, pod: string, container: string, tail: number) =>
    K8sService.GetPodLogs(sessionID, namespace, pod, container, tail),
  workloadAction: (
    sessionID: string,
    kind: string,
    namespace: string,
    name: string,
    action: K8sAction,
    replicas: number,
  ) => K8sService.WorkloadAction(sessionID, kind, namespace, name, action, replicas),
  deletePod: (sessionID: string, namespace: string, name: string) =>
    K8sService.DeletePod(sessionID, namespace, name),
};

export type K8sErrorKind = "notInstalled" | "clusterUnreachable" | "generic";

/**
 * Map a backend error to a hint category. The sentinel messages come from
 * internal/application/k8s_service.go (ErrKubectlUnavailable /
 * ErrClusterUnreachable).
 */
export function k8sErrorKind(e: unknown): K8sErrorKind {
  const msg = String((e as Error)?.message ?? e ?? "").toLowerCase();
  if (msg.includes("kubectl cli not available")) return "notInstalled";
  if (msg.includes("k8s cluster unreachable")) return "clusterUnreachable";
  return "generic";
}

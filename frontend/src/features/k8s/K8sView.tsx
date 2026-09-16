import { useEffect, useMemo, useRef, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Copy,
  FileCode,
  FileText,
  Play,
  RefreshCw,
  RotateCw,
  Ship,
  SlidersHorizontal,
  Square,
  TerminalSquare,
  Trash2,
} from "lucide-react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type {
  K8sClusterInfo,
  K8sEvent,
  K8sNode,
  K8sPod,
  K8sServiceInfo,
  K8sWorkload,
} from "@/../bindings/github.com/ai-remote/workspace/internal/domain";
import { k8sApi, k8sErrorKind, type K8sAction } from "@/features/k8s/api";
import { ContextMenu, type MenuItem } from "@/components/ui/ContextMenu";
import { ResourceYAMLDialog, type YamlTarget } from "@/features/k8s/ResourceYAMLDialog";
import { insertToTerminal } from "@/lib/insertTerminal";
import { toast } from "@/lib/toast";
import { useConfirm } from "@/lib/useConfirm";
import { cn } from "@/lib/utils";

type SubTab = "overview" | "workloads" | "pods" | "services" | "events" | "logs";
type EventFilter = "all" | "warning";

interface K8sViewProps {
  /** The terminal session (tab) whose cluster access (kubectl) is shown. */
  embeddedSessionID: string;
}

const LOG_TAILS = [100, 200, 500, 1000];

/** Actions that visibly disrupt the workload — confirmed before running. */
const CONFIRMED_ACTIONS: ReadonlySet<K8sAction> = new Set(["restart"]);

/**
 * Kubernetes panel (right side, next to Files/Agent/Docker): cluster
 * overview with nodes, workload controllers (Deployments / StatefulSets /
 * DaemonSets) with scale + rollout actions, pods with logs and one-pod
 * recreation, services, and recent events. Everything runs kubectl on the
 * backend through the session's SSH exec channel (or the local kubectl CLI
 * for local terminals); a missing CLI or unreachable cluster degrades to a
 * calm hint. CRDs and other free-form types are deliberately out of scope.
 */
export function K8sView({ embeddedSessionID }: K8sViewProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<SubTab>("workloads");
  const [intervalSec, setIntervalSec] = useState(60);

  // Shared refresh interval with the host monitor panel (global setting).
  useEffect(() => {
    ConfigService.GetAppConfig()
      .then((cfg) => {
        if (cfg.monitorIntervalSeconds > 0) setIntervalSec(cfg.monitorIntervalSeconds);
      })
      .catch(() => {});
  }, []);

  const intervalMs = Math.max(5, intervalSec) * 1000;
  const refetchOpts = { refetchInterval: intervalMs } as const;

  // Namespace scope: "" = all namespaces. Component state, so it survives
  // tab switches and session switches like the docker panel's sub-tab.
  const [namespace, setNamespace] = useState("");
  const [eventFilter, setEventFilter] = useState<EventFilter>("warning");
  const [logPod, setLogPod] = useState("");
  const [logContainer, setLogContainer] = useState("");
  const [logTail, setLogTail] = useState(200);
  /** YAML viewer/editor target (workload, pod, or service row). */
  const [yamlTarget, setYamlTarget] = useState<YamlTarget | null>(null);
  /** Panel context menu target (see buildMenuItems). */
  const [menu, setMenu] = useState<
    | { x: number; y: number; kind: "workload"; workload: K8sWorkload }
    | { x: number; y: number; kind: "pod"; pod: K8sPod }
    | { x: number; y: number; kind: "service"; service: K8sServiceInfo }
    | { x: number; y: number; kind: "event"; event: K8sEvent }
    | { x: number; y: number; kind: "node"; node: K8sNode }
    | { x: number; y: number; kind: "background" }
    | null
  >(null);

  /** Copy text with a quiet confirmation. */
  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.info(t("k8s.copied"));
    } catch {
      /* clipboard unavailable */
    }
  };

  /** Insert a kubectl command into the active terminal (review + Enter). */
  const insertCommand = (cmd: string) => {
    if (insertToTerminal(cmd)) {
      toast.info(t("k8s.insertedToTerminal"));
    } else {
      toast.info(t("k8s.noTerminal"));
    }
  };

  // Namespaces for the scope picker: cheap, cached, no polling — a failed
  // listing just leaves the picker with the "all" option (the underlying
  // queries surface the real problem anyway).
  const nsQ = useQuery({
    queryKey: ["k8s-namespaces", embeddedSessionID],
    queryFn: () => k8sApi.namespaces(embeddedSessionID),
    staleTime: 60_000,
  });

  const infoQ = useQuery({
    queryKey: ["k8s-info", embeddedSessionID],
    queryFn: () => k8sApi.clusterInfo(embeddedSessionID),
    enabled: tab === "overview",
    ...refetchOpts,
  });
  const nodesQ = useQuery({
    queryKey: ["k8s-nodes", embeddedSessionID],
    queryFn: () => k8sApi.nodes(embeddedSessionID),
    enabled: tab === "overview",
    // Keep the previous list visible while a refresh / namespace switch is
    // in flight — no loading flash on cached data (docker-panel feel).
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  const workloadsEnabled = tab === "workloads";
  const deployQ = useQuery({
    queryKey: ["k8s-workloads", embeddedSessionID, "deployments", namespace],
    queryFn: () => k8sApi.workloads(embeddedSessionID, "deployments", namespace),
    enabled: workloadsEnabled,
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  const stsQ = useQuery({
    queryKey: ["k8s-workloads", embeddedSessionID, "statefulsets", namespace],
    queryFn: () => k8sApi.workloads(embeddedSessionID, "statefulsets", namespace),
    enabled: workloadsEnabled,
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  const dsQ = useQuery({
    queryKey: ["k8s-workloads", embeddedSessionID, "daemonsets", namespace],
    queryFn: () => k8sApi.workloads(embeddedSessionID, "daemonsets", namespace),
    enabled: workloadsEnabled,
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  // The logs tab needs the pod list for its pickers, so pods are fetched for
  // both tabs.
  const podsEnabled = tab === "pods" || tab === "logs";
  const podsQ = useQuery({
    queryKey: ["k8s-pods", embeddedSessionID, namespace],
    queryFn: () => k8sApi.pods(embeddedSessionID, namespace),
    enabled: podsEnabled,
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  const servicesQ = useQuery({
    queryKey: ["k8s-services", embeddedSessionID, namespace],
    queryFn: () => k8sApi.services(embeddedSessionID, namespace),
    enabled: tab === "services",
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  const eventsQ = useQuery({
    queryKey: ["k8s-events", embeddedSessionID, namespace],
    queryFn: () => k8sApi.events(embeddedSessionID, namespace),
    enabled: tab === "events",
    placeholderData: keepPreviousData,
    ...refetchOpts,
  });
  // Logs always target the pod's OWN namespace (from its list row), never
  // the panel scope: `kubectl logs` has no --all-namespaces, and a row picked
  // under "all namespaces" otherwise resolves to an invalid command.
  const logPodInfo = (podsQ.data ?? []).find((p) => p.name === logPod);
  const logNamespace = logPodInfo?.namespace || namespace;
  const logsQ = useQuery({
    queryKey: ["k8s-logs", embeddedSessionID, logNamespace, logPod, logContainer, logTail],
    queryFn: () => k8sApi.logs(embeddedSessionID, logNamespace, logPod, logContainer, logTail),
    enabled: tab === "logs" && logPod !== "",
    // Logs are pull-only: no auto-refresh loop.
  });

  const workloads: K8sWorkload[] = useMemo(() => {
    const rows = [...(deployQ.data ?? []), ...(stsQ.data ?? []), ...(dsQ.data ?? [])];
    rows.sort((a, b) => {
      if (a.namespace !== b.namespace) return a.namespace.localeCompare(b.namespace);
      if (a.kind !== b.kind) return a.kind.localeCompare(b.kind);
      return a.name.localeCompare(b.name);
    });
    return rows;
  }, [deployQ.data, stsQ.data, dsQ.data]);

  const activeError =
    tab === "overview"
      ? infoQ.error ?? nodesQ.error
      : tab === "workloads"
        ? deployQ.error ?? stsQ.error ?? dsQ.error
        : tab === "pods" || tab === "logs"
          ? tab === "pods"
            ? podsQ.error
            : podsQ.error ?? logsQ.error
          : tab === "services"
            ? servicesQ.error
            : tab === "events"
              ? eventsQ.error
              : undefined;

  // Remember each workload's last non-zero replica count (per panel mount),
  // so "启动" restores the previous size instead of guessing 1.
  const lastReplicas = useRef(new Map<string, number>());
  useEffect(() => {
    for (const w of workloads) {
      if (w.replicas > 0) lastReplicas.current.set(`${w.namespace}/${w.kind}/${w.name}`, w.replicas);
    }
  }, [workloads]);

  // When the pod list arrives and the picker selection vanished (state
  // change, removal), fall back to the first pod and its first container.
  // Skipped while the list is a placeholder from the previous namespace —
  // resetting against stale data would fire a doomed logs fetch.
  useEffect(() => {
    if (podsQ.isPlaceholderData) return;
    const list = podsQ.data ?? [];
    if (list.length > 0 && !list.some((p) => p.name === logPod)) {
      setLogPod(list[0].name);
      setLogContainer(list[0].containers?.[0] ?? "");
    }
  }, [podsQ.data, podsQ.isPlaceholderData]); // eslint-disable-line react-hooks/exhaustive-deps

  // Multi-container pods need an explicit -c; keep the picker in sync.
  const selectedPod = (podsQ.data ?? []).find((p) => p.name === logPod);
  useEffect(() => {
    if (podsQ.isPlaceholderData) return;
    if (selectedPod && !(selectedPod.containers ?? []).includes(logContainer)) {
      setLogContainer(selectedPod.containers?.[0] ?? "");
    }
  }, [selectedPod, podsQ.isPlaceholderData]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Lifecycle actions ─────────────────────────────────────────────
  const { askConfirm, askPrompt } = useConfirm();
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<{ key: string; action: string } | null>(null);

  const workloadMut = useMutation({
    mutationFn: (v: { kind: string; namespace: string; name: string; action: K8sAction; replicas: number }) =>
      k8sApi.workloadAction(embeddedSessionID, v.kind, v.namespace, v.name, v.action, v.replicas),
    onSettled: async () => {
      setPending(null);
      await queryClient.invalidateQueries({ queryKey: ["k8s-workloads", embeddedSessionID] });
      await queryClient.invalidateQueries({ queryKey: ["k8s-info", embeddedSessionID] });
      await queryClient.invalidateQueries({ queryKey: ["k8s-pods", embeddedSessionID] });
    },
  });

  const deletePodMut = useMutation({
    mutationFn: (v: { namespace: string; name: string }) =>
      k8sApi.deletePod(embeddedSessionID, v.namespace, v.name),
    onSettled: async () => {
      setPending(null);
      await queryClient.invalidateQueries({ queryKey: ["k8s-pods", embeddedSessionID] });
      await queryClient.invalidateQueries({ queryKey: ["k8s-info", embeddedSessionID] });
    },
  });

  const runWorkloadAction = async (w: K8sWorkload, action: K8sAction, replicas?: number) => {
    if (CONFIRMED_ACTIONS.has(action)) {
      const ok = await askConfirm({
        title: t(`k8s.action_${action}`),
        message: t("k8s.actionConfirm", { action: t(`k8s.action_${action}`), name: w.name }),
        confirmLabel: t(`k8s.action_${action}`),
      });
      if (!ok) return;
    }
    const kind = `${w.kind}s`.toLowerCase(); // display kind → kubectl plural
    const target = action === "scale" ? replicas ?? 0 : 0;
    setPending({ key: `${w.namespace}/${w.name}`, action });
    workloadMut.mutate({ kind, namespace: w.namespace, name: w.name, action, replicas: target });
  };

  /** 停止: scale to 0. 启动: restore the last non-zero replica count (≥1). */
  const stopWorkload = (w: K8sWorkload) => runWorkloadAction(w, "scale", 0);
  const startWorkload = (w: K8sWorkload) => {
    const last = lastReplicas.current.get(`${w.namespace}/${w.kind}/${w.name}`) ?? 1;
    void runWorkloadAction(w, "scale", Math.max(1, last));
  };
  const restartWorkload = (w: K8sWorkload) => runWorkloadAction(w, "restart");

  const setReplicas = async (w: K8sWorkload) => {
    const input = await askPrompt({
      title: t("k8s.setReplicasTitle", { name: w.name }),
      message: t("k8s.setReplicasMsg"),
      initialValue: String(w.replicas || 1),
      confirmLabel: t("common.save"),
    });
    if (input === null) return;
    const n = Math.round(Number(input));
    if (!Number.isFinite(n) || n < 0) {
      toast.error(t("k8s.invalidReplicas"));
      return;
    }
    void runWorkloadAction(w, "scale", n);
  };

  const deletePod = async (p: K8sPod) => {
    const ok = await askConfirm({
      title: t("k8s.deletePodTitle"),
      message: t("k8s.deletePodConfirm", { name: p.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    setPending({ key: `${p.namespace}/${p.name}`, action: "deletePod" });
    deletePodMut.mutate({ namespace: p.namespace, name: p.name });
  };

  const openLogs = (p: K8sPod) => {
    setLogPod(p.name);
    setLogContainer(p.containers?.[0] ?? "");
    setTab("logs");
  };

  /** Open the YAML viewer/editor for one resource (kind: kubectl singular). */
  const openYAML = (kind: string, namespace: string, name: string) => {
    setYamlTarget({ kind, namespace, name });
  };

  /** Context-menu items per right-click target. */
  function buildMenuItems(): MenuItem[] {
    if (!menu) return [];
    if (menu.kind === "workload") {
      const w = menu.workload;
      const scalable = w.kind !== "DaemonSet";
      const stopped = w.replicas === 0;
      return [
        scalable && !stopped
          ? { label: t("k8s.action_stop"), icon: Square, disabled: pending !== null, onClick: () => void stopWorkload(w) }
          : { type: "separator" as const },
        scalable && stopped
          ? { label: t("k8s.action_start"), icon: Play, disabled: pending !== null, onClick: () => void startWorkload(w) }
          : { type: "separator" as const },
        { label: t("k8s.action_restart"), icon: RotateCw, disabled: pending !== null, onClick: () => void restartWorkload(w) },
        scalable
          ? {
              label: t("k8s.setReplicas"),
              icon: SlidersHorizontal,
              disabled: pending !== null,
              onClick: () => void setReplicas(w),
            }
          : { type: "separator" as const },
        { type: "separator" },
        {
          label: t("k8s.viewYaml"),
          icon: FileCode,
          onClick: () => openYAML(w.kind.toLowerCase(), w.namespace, w.name),
        },
        {
          label: t("k8s.menuDescribe"),
          icon: TerminalSquare,
          onClick: () =>
            insertCommand(`kubectl describe ${w.kind.toLowerCase()} ${w.name} -n ${w.namespace}`),
        },
        {
          label: t("k8s.menuRolloutHistory"),
          icon: TerminalSquare,
          onClick: () =>
            insertCommand(`kubectl rollout history ${w.kind.toLowerCase()} ${w.name} -n ${w.namespace}`),
        },
        { type: "separator" },
        { label: t("k8s.copyName"), icon: Copy, onClick: () => void copyText(w.name) },
        { label: t("k8s.copyImage"), icon: Copy, onClick: () => void copyText((w.images ?? []).join(" ")) },
      ];
    }
    if (menu.kind === "pod") {
      const p = menu.pod;
      const cFlag = (p.containers ?? []).length > 1 ? ` -c ${(p.containers ?? [])[0]}` : "";
      return [
        { label: t("k8s.viewLogs"), icon: FileText, onClick: () => openLogs(p) },
        {
          label: t("k8s.menuFollowLogs"),
          icon: FileText,
          onClick: () => insertCommand(`kubectl logs --tail 100 -f ${p.name}${cFlag} -n ${p.namespace}`),
        },
        {
          label: t("k8s.menuExec"),
          icon: TerminalSquare,
          onClick: () => insertCommand(`kubectl exec -it ${p.name}${cFlag} -n ${p.namespace} -- sh`),
        },
        {
          label: t("k8s.menuDescribe"),
          icon: TerminalSquare,
          onClick: () => insertCommand(`kubectl describe pod ${p.name} -n ${p.namespace}`),
        },
        {
          label: t("k8s.viewYaml"),
          icon: FileCode,
          onClick: () => openYAML("pod", p.namespace, p.name),
        },
        { type: "separator" },
        {
          label: t("k8s.deletePod"),
          icon: Trash2,
          disabled: pending !== null,
          onClick: () => void deletePod(p),
        },
        { type: "separator" },
        { label: t("k8s.copyName"), icon: Copy, onClick: () => void copyText(p.name) },
        { label: t("k8s.copyIp"), icon: Copy, onClick: () => void copyText(p.ip || "") },
        { label: t("k8s.copyNode"), icon: Copy, onClick: () => void copyText(p.node || "") },
      ];
    }
    if (menu.kind === "service") {
      const s = menu.service;
      return [
        {
          label: t("k8s.viewYaml"),
          icon: FileCode,
          onClick: () => openYAML("service", s.namespace, s.name),
        },
        {
          label: t("k8s.menuDescribe"),
          icon: TerminalSquare,
          onClick: () => insertCommand(`kubectl describe service ${s.name} -n ${s.namespace}`),
        },
        { type: "separator" },
        { label: t("k8s.copyName"), icon: Copy, onClick: () => void copyText(s.name) },
        { label: t("k8s.copyIp"), icon: Copy, onClick: () => void copyText(s.clusterIp || "") },
      ];
    }
    if (menu.kind === "event") {
      return [
        { label: t("k8s.copyMessage"), icon: Copy, onClick: () => void copyText(menu.event.message || "") },
      ];
    }
    if (menu.kind === "node") {
      const n = menu.node;
      return [
        {
          label: t("k8s.menuDescribe"),
          icon: TerminalSquare,
          onClick: () => insertCommand(`kubectl describe node ${n.name}`),
        },
        { type: "separator" },
        { label: t("k8s.copyName"), icon: Copy, onClick: () => void copyText(n.name) },
        { label: t("k8s.copyIp"), icon: Copy, onClick: () => void copyText(n.internalIp || "") },
      ];
    }
    // Background
    return [
      {
        label: t("k8s.refresh"),
        icon: RefreshCw,
        disabled: activeFetching,
        onClick: () => refetchActive(),
      },
    ];
  }

  // Manual refresh: only the active tab's queries (mirrors the docker panel).
  // The refresh button spins and locks while they are in flight.
  const activeFetching =
    tab === "overview"
      ? infoQ.isFetching || nodesQ.isFetching
      : tab === "workloads"
        ? deployQ.isFetching || stsQ.isFetching || dsQ.isFetching
        : tab === "pods"
          ? podsQ.isFetching
          : tab === "logs"
            ? podsQ.isFetching || logsQ.isFetching
            : tab === "services"
              ? servicesQ.isFetching
              : eventsQ.isFetching;

  const refetchActive = () => {
    if (tab === "overview") {
      void infoQ.refetch();
      void nodesQ.refetch();
    } else if (tab === "workloads") {
      void deployQ.refetch();
      void stsQ.refetch();
      void dsQ.refetch();
    } else if (tab === "pods") {
      void podsQ.refetch();
    } else if (tab === "services") {
      void servicesQ.refetch();
    } else if (tab === "events") {
      void eventsQ.refetch();
    } else {
      void podsQ.refetch(); // also the logs picker's source
      void logsQ.refetch();
    }
  };

  const tabs: { id: SubTab; label: string }[] = [
    { id: "overview", label: t("k8s.overview") },
    { id: "workloads", label: t("k8s.workloads") },
    { id: "pods", label: t("k8s.pods") },
    { id: "services", label: t("k8s.services") },
    { id: "events", label: t("k8s.events") },
    { id: "logs", label: t("k8s.logs") },
  ];

  return (
    <div
      className="flex h-full flex-col overflow-hidden"
      onContextMenu={(e) => {
        // Panel-wide default-menu suppression: rows stopPropagation with
        // their own menus; everything else falls to the background menu.
        e.preventDefault();
        setMenu({ x: e.clientX, y: e.clientY, kind: "background" });
      }}
    >
      {/* Namespace scope + sub-tab strip + refresh */}
      <div className="flex h-9 shrink-0 items-center gap-1.5 border-b border-border px-2">
        <select
          value={namespace}
          onChange={(e) => setNamespace(e.target.value)}
          aria-label={t("k8s.namespace")}
          title={t("k8s.namespace")}
          className="h-6 max-w-28 shrink-0 rounded-[var(--radius)] border border-border bg-background px-1 text-[11px] text-foreground"
        >
          <option value="">{t("k8s.allNamespaces")}</option>
          {(nsQ.data ?? []).map((n) => (
            <option key={n.name} value={n.name}>
              {n.name}
            </option>
          ))}
        </select>
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
          {tabs.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => setTab(s.id)}
              className={cn(
                "flex h-7 shrink-0 items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
                tab === s.id
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
              )}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-1.5 pl-1">
          {tab !== "logs" && (
            <span
              className="text-[10px] text-muted-foreground"
              title={t("k8s.autoRefresh", { sec: intervalSec })}
            >
              {intervalSec}s
            </span>
          )}
          <button
            type="button"
            onClick={refetchActive}
            disabled={activeFetching}
            aria-label={t("k8s.refresh")}
            title={t("k8s.refresh")}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40"
          >
            <RefreshCw className={cn("h-3.5 w-3.5", activeFetching && "animate-spin")} />
          </button>
        </div>
      </div>

      {/* Content (background menu handled at the panel root) */}
      <div className="min-h-0 flex-1 overflow-y-auto p-2.5">
        {activeError ? (
          <ErrorHint error={activeError} />
        ) : tab === "overview" ? (
          <OverviewPane
            info={infoQ.data}
            nodes={nodesQ.data ?? []}
            loading={infoQ.isLoading || nodesQ.isLoading}
            onMenu={(x, y, node) => setMenu({ x, y, kind: "node", node })}
          />
        ) : tab === "workloads" ? (
          <WorkloadsPane
            showNamespace={namespace === ""}
            loading={deployQ.isLoading && stsQ.isLoading && dsQ.isLoading}
            workloads={workloads}
            pending={pending}
            actionError={workloadMut.isError ? workloadMut.error : null}
            onStop={stopWorkload}
            onStart={startWorkload}
            onRestart={restartWorkload}
            onYaml={(w) => openYAML(w.kind.toLowerCase(), w.namespace, w.name)}
            onMenu={(x, y, workload) => setMenu({ x, y, kind: "workload", workload })}
          />
        ) : tab === "pods" ? (
          <PodsPane
            showNamespace={namespace === ""}
            loading={podsQ.isLoading}
            pods={podsQ.data ?? []}
            pending={pending}
            actionError={deletePodMut.isError ? deletePodMut.error : null}
            onLogs={openLogs}
            onDelete={(p) => void deletePod(p)}
            onYaml={(p) => openYAML("pod", p.namespace, p.name)}
            onMenu={(x, y, pod) => setMenu({ x, y, kind: "pod", pod })}
          />
        ) : tab === "services" ? (
          <ServicesPane
            showNamespace={namespace === ""}
            loading={servicesQ.isLoading}
            services={servicesQ.data ?? []}
            onYaml={(s) => openYAML("service", s.namespace, s.name)}
            onMenu={(x, y, service) => setMenu({ x, y, kind: "service", service })}
          />
        ) : tab === "events" ? (
          <EventsPane
            loading={eventsQ.isLoading}
            events={eventsQ.data ?? []}
            filter={eventFilter}
            onFilter={setEventFilter}
            onMenu={(x, y, event) => setMenu({ x, y, kind: "event", event })}
          />
        ) : (
          <LogsPane
            showNamespace={namespace === ""}
            pods={podsQ.data ?? []}
            selectedPod={logPod}
            onSelectPod={(name) => {
              setLogPod(name);
              const p = (podsQ.data ?? []).find((x) => x.name === name);
              setLogContainer(p?.containers?.[0] ?? "");
            }}
            selectedContainer={logContainer}
            onSelectContainer={setLogContainer}
            tail={logTail}
            onTail={setLogTail}
            query={logsQ}
          />
        )}
      </div>

      {/* Context menu (row or background) */}
      {menu && (
        <ContextMenu x={menu.x} y={menu.y} items={buildMenuItems()} onClose={() => setMenu(null)} />
      )}

      {/* YAML viewer / editor */}
      <ResourceYAMLDialog
        open={yamlTarget !== null}
        target={yamlTarget}
        sessionID={embeddedSessionID}
        onClose={() => setYamlTarget(null)}
      />
    </div>
  );
}

// ── Error hint ───────────────────────────────────────────────────────

function ErrorHint({ error }: { error: unknown }) {
  const { t } = useTranslation();
  const kind = k8sErrorKind(error);
  const titleKey =
    kind === "notInstalled" ? "k8s.notInstalled" : kind === "clusterUnreachable" ? "k8s.clusterUnreachable" : "k8s.unavailable";
  const descKey =
    kind === "notInstalled"
      ? "k8s.notInstalledDesc"
      : kind === "clusterUnreachable"
        ? "k8s.clusterUnreachableDesc"
        : "k8s.unavailableDesc";
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center"
      title={String((error as Error)?.message ?? "")}
    >
      <Ship className="h-8 w-8 text-muted-foreground/40" />
      <div className="text-sm font-medium text-foreground">{t(titleKey)}</div>
      <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">{t(descKey)}</p>
    </div>
  );
}

// ── Overview ─────────────────────────────────────────────────────────

function OverviewPane({
  info,
  nodes,
  loading,
  onMenu,
}: {
  info: K8sClusterInfo | undefined;
  nodes: K8sNode[];
  loading: boolean;
  onMenu: (x: number, y: number, node: K8sNode) => void;
}) {
  const { t } = useTranslation();
  if (loading || !info) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }

  return (
    // @container scopes the node grid's breakpoints to THIS pane's width —
    // viewport-based sm:/md: variants would never reflow a draggable side
    // panel whose host window is always much wider.
    <div className="flex flex-col gap-2.5 @container">
      <div className="grid grid-cols-2 gap-2">
        <InfoCard label={t("k8s.serverVersion")} value={info.serverVersion || "—"} footer={info.clientVersion || ""} />
        <InfoCard label={t("k8s.context")} value={info.context || "—"} mono />
      </div>

      <div className="grid grid-cols-3 gap-2">
        <InfoCard label={t("k8s.nodesReady")} value={`${info.nodesReady}/${info.nodesTotal}`} accent />
        <InfoCard label={t("k8s.podsRunningCount")} value={String(info.podsRunning)} />
        <InfoCard label={t("k8s.podsAbnormal")} value={String(info.podsPending + info.podsFailed + info.podsUnknown)} />
      </div>

      <div className="grid grid-cols-3 gap-2">
        <InfoCard label={t("k8s.deploymentsCount")} value={String(info.deployments)} />
        <InfoCard label={t("k8s.statefulsetsCount")} value={String(info.statefulSets)} />
        <InfoCard label={t("k8s.daemonsetsCount")} value={String(info.daemonSets)} />
      </div>

      {/* Nodes (read-only): usage cards in a grid whose column count adapts
          to the pane's own width via container queries. */}
      <div className="flex flex-col gap-1">
        <span className="px-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          {t("k8s.nodes")} ({nodes.length})
        </span>
        <div className="grid grid-cols-1 gap-2 @[320px]:grid-cols-2 @[480px]:grid-cols-3">
          {nodes.map((n) => (
            <NodeCard key={n.name} node={n} onMenu={onMenu} />
          ))}
          {nodes.length === 0 && (
            <div className="col-span-full rounded-[var(--radius)] border border-border bg-card p-4 text-center text-xs text-muted-foreground">
              {t("k8s.noNodes")}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Node usage card ──────────────────────────────────────────────────

function NodeCard({
  node,
  onMenu,
}: {
  node: K8sNode;
  onMenu: (x: number, y: number, node: K8sNode) => void;
}) {
  const { t } = useTranslation();
  const hasUsage = node.cpuUsed !== "" || node.memUsed !== "";
  const notReady = node.status !== "Ready";
  // Full identity lives in the tooltip: OS image, pod capacity and the
  // raw capacity values (usually ≈ allocatable; shown where they differ).
  const tooltip = [
    node.name,
    node.os || "",
    `${t("k8s.pods")} ${t("k8s.capacity")} ${node.podCapacity || "—"}`,
    `${t("k8s.capacity")}: CPU ${node.capacityCpu || "—"} · ${t("k8s.memUsage")} ${node.capacityMem || "—"}`,
  ]
    .filter(Boolean)
    .join("\n");
  return (
    <div
      className={cn(
        "flex cursor-default flex-col gap-1.5 rounded-[var(--radius)] border bg-card p-2 transition-colors hover:bg-accent/30",
        notReady && "border-destructive/50",
      )}
      title={tooltip}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onMenu(e.clientX, e.clientY, node);
      }}
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", nodeDotClass(node.status))} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium" title={node.name}>
          {node.name}
        </span>
        {node.roles && (
          <span className="shrink-0 rounded-full border border-primary/40 px-1.5 py-px text-[9px] text-primary">
            {node.roles}
          </span>
        )}
      </div>

      {/* Resource rows always render — with metrics they show live usage
          bars, without them the allocatable/capacity facts from
          `kubectl get nodes` in the same chart-shaped layout. */}
      <ResourceRow
        label="CPU"
        filled={hasUsage}
        percent={node.cpuPercent}
        detail={
          hasUsage
            ? usageDetail(node.cpuUsed, node.allocatableCpu)
            : allocDetail(t, node.allocatableCpu, node.capacityCpu)
        }
      />
      <ResourceRow
        label={t("k8s.memUsage")}
        filled={hasUsage}
        percent={node.memPercent}
        detail={
          hasUsage
            ? usageDetail(node.memUsed, node.allocatableMem)
            : allocDetail(t, node.allocatableMem, node.capacityMem)
        }
      />

      {!hasUsage && (
        <p className="text-[10px] leading-relaxed text-muted-foreground/70">{t("k8s.metricsHint")}</p>
      )}

      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
        <span className="min-w-0 flex-1 truncate font-mono" title={node.internalIp || ""}>
          {node.internalIp || "—"}
        </span>
        <span className="shrink-0">{node.version}</span>
        <span className="shrink-0">{node.age}</span>
      </div>
    </div>
  );
}

/** usageDetail renders "1200m/4" for the bar row. */
function usageDetail(used: string, allocatable?: string) {
  return used ? (allocatable ? `${used}/${allocatable}` : used) : "";
}

/** allocDetail renders the no-metrics fallback: "可分配 4 · 容量 4". */
function allocDetail(
  t: (k: string) => string,
  allocatable?: string,
  capacity?: string,
): string {
  const parts: string[] = [];
  if (allocatable) parts.push(`${t("k8s.allocatable")} ${allocatable}`);
  if (capacity && capacity !== allocatable) parts.push(`${t("k8s.capacity")} ${capacity}`);
  return parts.join(" · ");
}

/** ResourceRow paints one resource line: label + value over a track. With
 *  metrics it carries a colored usage fill (amber ≥70, red ≥90 — same
 *  thresholds as the docker stats bars); without, an empty neutral track
 *  keeps the chart-like structure while the allocatable/capacity facts stay
 *  visible. */
function ResourceRow({
  label,
  filled,
  percent,
  detail,
}: {
  label: string;
  filled: boolean;
  percent: number;
  detail: string;
}) {
  const p = Math.max(0, Math.min(100, percent));
  const value = filled ? (detail ? `${percent}% · ${detail}` : `${percent}%`) : detail || "—";
  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex items-baseline justify-between gap-1 text-[10px]">
        <span className="shrink-0 text-muted-foreground">{label}</span>
        <span
          className={cn(
            "min-w-0 truncate font-mono tabular-nums",
            filled && p >= 90 && "text-destructive",
          )}
          title={detail}
        >
          {value}
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={cn(
            "h-full rounded-full transition-all",
            !filled && "hidden",
            p >= 90 ? "bg-destructive" : p >= 70 ? "bg-amber-400" : "bg-primary",
          )}
          style={{ width: filled ? `${p}%` : "0%" }}
        />
      </div>
    </div>
  );
}

function nodeDotClass(status: string) {
  if (status === "Ready") return "bg-success";
  if (status === "NotReady") return "bg-destructive";
  return "bg-muted-foreground/40";
}

function InfoCard({
  label,
  value,
  footer,
  mono,
  accent,
}: {
  label: string;
  value: string;
  footer?: string;
  mono?: boolean;
  accent?: boolean;
}) {
  return (
    <div className="flex flex-col gap-1 rounded-[var(--radius)] border border-border bg-card p-2">
      <span className="flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground">
        <span className="truncate">{label}</span>
      </span>
      <span
        className={cn(
          "truncate text-sm font-medium tabular-nums",
          mono && "font-mono text-[11px]",
          accent && "text-success",
        )}
        title={value}
      >
        {value}
      </span>
      {footer && <span className="truncate text-[10px] text-muted-foreground/70">{footer}</span>}
    </div>
  );
}

// ── Workloads (scale / rollout) ──────────────────────────────────────

function workloadDotClass(w: K8sWorkload) {
  if (w.replicas === 0) return "bg-muted-foreground/40";
  if (w.ready >= w.replicas) return "bg-success";
  return "bg-primary animate-pulse";
}

function WorkloadsPane({
  showNamespace,
  loading,
  workloads,
  pending,
  actionError,
  onStop,
  onStart,
  onRestart,
  onYaml,
  onMenu,
}: {
  showNamespace: boolean;
  loading: boolean;
  workloads: K8sWorkload[];
  pending: { key: string; action: string } | null;
  actionError: unknown;
  onStop: (w: K8sWorkload) => void;
  onStart: (w: K8sWorkload) => void;
  onRestart: (w: K8sWorkload) => void;
  onYaml: (w: K8sWorkload) => void;
  onMenu: (x: number, y: number, workload: K8sWorkload) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }

  const actionErrMsg = actionError ? String((actionError as Error)?.message ?? actionError) : "";

  const ActionBtn = ({
    w,
    action,
    title,
    children,
  }: {
    w: K8sWorkload;
    action: string;
    title: string;
    children: React.ReactNode;
  }) => {
    const busy = pending?.key === `${w.namespace}/${w.name}` && pending?.action === action;
    return (
      <button
        type="button"
        disabled={pending !== null}
        onClick={() => (action === "stop" ? onStop(w) : action === "start" ? onStart(w) : onRestart(w))}
        title={title}
        className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40"
      >
        {busy ? <RefreshCw className="h-3 w-3 animate-spin" /> : children}
      </button>
    );
  };

  return (
    <div className="flex flex-col gap-1.5 text-xs">
      {actionErrMsg && (
        <div
          className="truncate rounded border border-destructive/40 bg-destructive/10 px-2 py-1 text-[10px] text-destructive"
          title={actionErrMsg}
        >
          {t("k8s.actionFailed")}: {actionErrMsg}
        </div>
      )}

      <div className="divide-y divide-border/50">
        {workloads.map((w) => {
          const stopped = w.replicas === 0;
          const scalable = w.kind !== "DaemonSet";
          const images = (w.images ?? []).join(", ");
          return (
            <div
              key={`${w.kind}/${w.namespace}/${w.name}`}
              className="flex flex-col gap-0.5 px-1 py-1.5 hover:bg-accent/40"
              onContextMenu={(e) => {
                e.preventDefault();
                e.stopPropagation();
                onMenu(e.clientX, e.clientY, w);
              }}
            >
              <div className="flex items-center gap-1.5">
                <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", workloadDotClass(w))} />
                <span className="min-w-0 flex-1 truncate font-medium" title={`${w.namespace}/${w.name}`}>
                  {w.name}
                </span>
                <span className="shrink-0 rounded-full border border-border px-1.5 py-px text-[9px] text-muted-foreground">
                  {w.kind}
                </span>
                {scalable &&
                  (stopped ? (
                    <ActionBtn w={w} action="start" title={t("k8s.action_start")}>
                      <Play className="h-3 w-3" />
                    </ActionBtn>
                  ) : (
                    <ActionBtn w={w} action="stop" title={t("k8s.action_stop")}>
                      <Square className="h-3 w-3" />
                    </ActionBtn>
                  ))}
                <ActionBtn w={w} action="restart" title={t("k8s.action_restart")}>
                  <RotateCw className="h-3 w-3" />
                </ActionBtn>
                <button
                  type="button"
                  onClick={() => onYaml(w)}
                  title={t("k8s.viewYaml")}
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <FileCode className="h-3 w-3" />
                </button>
              </div>
              <div className="flex items-center gap-2 pl-3 text-[10px] text-muted-foreground">
                {showNamespace && <span className="shrink-0 rounded bg-muted/50 px-1 font-mono">{w.namespace}</span>}
                <span className="shrink-0 tabular-nums" title={t("k8s.replicasReady")}>
                  {w.ready}/{w.replicas}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono" title={images}>
                  {images || "—"}
                </span>
                <span className="shrink-0">{w.age}</span>
              </div>
            </div>
          );
        })}
        {workloads.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("k8s.noWorkloads")}</div>
        )}
      </div>
    </div>
  );
}

// ── Pods ─────────────────────────────────────────────────────────────

/** Pod status → filter bucket, mirroring what kubectl colour-codes. */
function podDotClass(status: string) {
  if (status === "Running") return "bg-success";
  if (status === "Pending" || status === "ContainerCreating" || status === "PodInitializing") {
    return "bg-primary animate-pulse";
  }
  if (status === "Succeeded" || status === "Completed") return "bg-muted-foreground/40";
  // Failed / Evicted / CrashLoopBackOff / ImagePullBackOff / …
  return "bg-destructive";
}

function PodsPane({
  showNamespace,
  loading,
  pods,
  pending,
  actionError,
  onLogs,
  onDelete,
  onYaml,
  onMenu,
}: {
  showNamespace: boolean;
  loading: boolean;
  pods: K8sPod[];
  pending: { key: string; action: string } | null;
  actionError: unknown;
  onLogs: (p: K8sPod) => void;
  onDelete: (p: K8sPod) => void;
  onYaml: (p: K8sPod) => void;
  onMenu: (x: number, y: number, pod: K8sPod) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }

  const actionErrMsg = actionError ? String((actionError as Error)?.message ?? actionError) : "";

  return (
    <div className="flex flex-col gap-1.5 text-xs">
      {actionErrMsg && (
        <div
          className="truncate rounded border border-destructive/40 bg-destructive/10 px-2 py-1 text-[10px] text-destructive"
          title={actionErrMsg}
        >
          {t("k8s.actionFailed")}: {actionErrMsg}
        </div>
      )}

      <div className="divide-y divide-border/50">
        {pods.map((p) => {
          const key = `${p.namespace}/${p.name}`;
          const busy = pending?.key === key;
          return (
            <div
              key={key}
              className="flex flex-col gap-0.5 px-1 py-1.5 hover:bg-accent/40"
              onContextMenu={(e) => {
                e.preventDefault();
                e.stopPropagation();
                onMenu(e.clientX, e.clientY, p);
              }}
            >
              <div className="flex items-center gap-1.5">
                <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", podDotClass(p.status))} />
                <span className="min-w-0 flex-1 truncate font-medium" title={`${p.namespace}/${p.name}`}>
                  {p.name}
                </span>
                {showNamespace && <span className="shrink-0 rounded bg-muted/50 px-1 font-mono text-[9px]">{p.namespace}</span>}
                {busy ? (
                  <RefreshCw className="h-3 w-3 shrink-0 animate-spin text-muted-foreground" />
                ) : (
                  <button
                    type="button"
                    onClick={() => onDelete(p)}
                    title={t("k8s.deletePod")}
                    className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-destructive"
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => onYaml(p)}
                  title={t("k8s.viewYaml")}
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <FileCode className="h-3 w-3" />
                </button>
                <button
                  type="button"
                  onClick={() => onLogs(p)}
                  title={t("k8s.viewLogs")}
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <FileText className="h-3 w-3" />
                </button>
              </div>
              <div className="flex items-center gap-2 pl-3 text-[10px] text-muted-foreground">
                <span className="shrink-0 font-medium text-foreground/80">{p.status}</span>
                <span className="shrink-0 tabular-nums">{p.ready}</span>
                {p.restarts > 0 && <span className="shrink-0 tabular-nums text-warning">↻{p.restarts}</span>}
                <span className="min-w-0 flex-1 truncate font-mono" title={`${p.node} · ${p.ip}`}>
                  {p.ip || p.node || "—"}
                </span>
                <span className="shrink-0">{p.age}</span>
              </div>
            </div>
          );
        })}
        {pods.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("k8s.noPods")}</div>
        )}
      </div>
    </div>
  );
}

// ── Services ─────────────────────────────────────────────────────────

function ServicesPane({
  showNamespace,
  loading,
  services,
  onYaml,
  onMenu,
}: {
  showNamespace: boolean;
  loading: boolean;
  services: K8sServiceInfo[];
  onYaml: (s: K8sServiceInfo) => void;
  onMenu: (x: number, y: number, service: K8sServiceInfo) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  return (
    <div className="flex flex-col divide-y divide-border/50 text-xs">
      {services.map((s) => (
        <div
          key={`${s.namespace}/${s.name}`}
          className="flex flex-col gap-0.5 px-1 py-1.5 hover:bg-accent/40"
          onContextMenu={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onMenu(e.clientX, e.clientY, s);
          }}
        >
          <div className="flex items-center gap-1.5">
            <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
            {showNamespace && <span className="shrink-0 rounded bg-muted/50 px-1 font-mono text-[9px]">{s.namespace}</span>}
            <span className="shrink-0 rounded-full border border-border px-1.5 py-px text-[9px] text-muted-foreground">
              {s.type}
            </span>
            <span className="shrink-0 text-[10px] text-muted-foreground">{s.age}</span>
            <button
              type="button"
              onClick={() => onYaml(s)}
              title={t("k8s.viewYaml")}
              className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              <FileCode className="h-3 w-3" />
            </button>
          </div>
          <div className="flex items-center gap-2 pl-3 font-mono text-[10px] text-muted-foreground">
            <span className="shrink-0">{s.clusterIp}</span>
            <span className="min-w-0 flex-1 truncate" title={s.ports}>
              {s.ports || "—"}
            </span>
            {s.externalIp && <span className="shrink-0 truncate" title={s.externalIp}>{s.externalIp}</span>}
          </div>
        </div>
      ))}
      {services.length === 0 && (
        <div className="p-6 text-center text-xs text-muted-foreground">{t("k8s.noServices")}</div>
      )}
    </div>
  );
}

// ── Events ───────────────────────────────────────────────────────────

function EventsPane({
  loading,
  events,
  filter,
  onFilter,
  onMenu,
}: {
  loading: boolean;
  events: K8sEvent[];
  filter: EventFilter;
  onFilter: (f: EventFilter) => void;
  onMenu: (x: number, y: number, event: K8sEvent) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  const visible = events.filter((e) => filter === "all" || e.type === "Warning");
  return (
    <div className="flex flex-col gap-1.5 text-xs">
      <div className="flex items-center gap-1">
        {(["warning", "all"] as EventFilter[]).map((f) => (
          <button
            key={f}
            type="button"
            onClick={() => onFilter(f)}
            className={cn(
              "rounded-full border px-2 py-0.5 text-[10px] transition-colors",
              filter === f
                ? "border-primary/50 bg-accent text-foreground"
                : "border-border text-muted-foreground hover:bg-accent/50",
            )}
          >
            {t(f === "warning" ? "k8s.filter_warning" : "k8s.filter_all")}
          </button>
        ))}
      </div>
      <div className="divide-y divide-border/50">
        {visible.map((e, i) => (
          <div
            key={`${e.namespace}/${e.object}/${e.reason}/${i}`}
            className="flex flex-col gap-0.5 px-1 py-1.5 hover:bg-accent/40"
            onContextMenu={(ev) => {
              ev.preventDefault();
              ev.stopPropagation();
              onMenu(ev.clientX, ev.clientY, e);
            }}
          >
            <div className="flex items-center gap-1.5">
              <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", e.type === "Warning" ? "bg-destructive" : "bg-muted-foreground/40")} />
              <span className={cn("shrink-0 font-medium", e.type === "Warning" ? "text-destructive" : "text-foreground/80")}>
                {e.reason}
              </span>
              <span className="min-w-0 flex-1 truncate font-mono text-[10px]" title={e.object}>
                {e.object}
              </span>
              {e.count > 1 && <span className="shrink-0 rounded bg-muted/50 px-1 text-[9px] tabular-nums">×{e.count}</span>}
              <span className="shrink-0 text-[10px] text-muted-foreground">{e.age}</span>
            </div>
            <div className="truncate pl-3 text-[10px] text-muted-foreground" title={e.message}>
              {e.message}
            </div>
          </div>
        ))}
        {visible.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("k8s.noEvents")}</div>
        )}
      </div>
    </div>
  );
}

// ── Logs ─────────────────────────────────────────────────────────────

function LogsPane({
  showNamespace,
  pods,
  selectedPod,
  onSelectPod,
  selectedContainer,
  onSelectContainer,
  tail,
  onTail,
  query,
}: {
  showNamespace: boolean;
  pods: K8sPod[];
  selectedPod: string;
  onSelectPod: (name: string) => void;
  selectedContainer: string;
  onSelectContainer: (name: string) => void;
  tail: number;
  onTail: (n: number) => void;
  query: ReturnType<typeof useQuery<string>>;
}) {
  const { t } = useTranslation();
  const bottomRef = useRef<HTMLSpanElement>(null);

  // Newest lines are last with --tail; keep the view pinned there.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [query.data]);

  const pod = pods.find((p) => p.name === selectedPod);
  const containers = pod?.containers ?? [];
  const multiContainer = containers.length > 1;

  return (
    <div className="flex h-full flex-col gap-1.5">
      <div className="flex shrink-0 items-center gap-1.5">
        <select
          value={selectedPod}
          onChange={(e) => onSelectPod(e.target.value)}
          aria-label={t("k8s.selectPod")}
          className="h-7 min-w-0 flex-1 rounded-[var(--radius)] border border-border bg-background px-1.5 text-xs text-foreground"
        >
          {pods.length === 0 && <option value="">{t("k8s.noPods")}</option>}
          {pods.map((p) => (
            <option key={`${p.namespace}/${p.name}`} value={p.name}>
              {p.name} ({p.status})
            </option>
          ))}
        </select>
        {multiContainer && (
          <select
            value={selectedContainer}
            onChange={(e) => onSelectContainer(e.target.value)}
            aria-label={t("k8s.selectContainer")}
            title={t("k8s.selectContainer")}
            className="h-7 max-w-32 shrink-0 rounded-[var(--radius)] border border-border bg-background px-1.5 text-xs text-foreground"
          >
            {containers.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        )}
        <select
          value={tail}
          onChange={(e) => onTail(Number(e.target.value))}
          aria-label={t("k8s.tail")}
          title={t("k8s.tail")}
          className="h-7 shrink-0 rounded-[var(--radius)] border border-border bg-background px-1.5 text-xs text-foreground"
        >
          {LOG_TAILS.map((n) => (
            <option key={n} value={n}>
              {n} {t("k8s.lines")}
            </option>
          ))}
        </select>
      </div>

      <div className="min-h-0 flex-1 overflow-auto rounded-[var(--radius)] border border-border bg-background p-1.5">
        {query.isError ? (
          <div
            className="p-3 text-[10px] text-destructive"
            title={String((query.error as Error)?.message ?? "")}
          >
            {t("k8s.logsFailed")}: {String((query.error as Error)?.message ?? "")}
          </div>
        ) : query.isLoading ? (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>
        ) : query.data ? (
          <pre className="whitespace-pre-wrap break-all font-mono text-[10px] leading-relaxed text-foreground/90">
            {query.data}
            <span ref={bottomRef} className="inline-block h-px w-0" />
          </pre>
        ) : (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("k8s.selectPod")}</div>
        )}
      </div>

      {pod && (
        <div className="shrink-0 truncate text-center text-[10px] text-muted-foreground/70" title={(pod.containers ?? []).join(", ")}>
          {showNamespace ? `${pod.namespace}/` : ""}
          {pod.name} · {pod.status}
        </div>
      )}
    </div>
  );
}

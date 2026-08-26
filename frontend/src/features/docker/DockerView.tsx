import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Container as ContainerIcon,
  FileText,
  Package,
  Pause,
  Play,
  RefreshCw,
  RotateCw,
  Square,
} from "lucide-react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type { DockerContainerStats } from "@/../bindings/github.com/ai-remote/workspace/internal/domain";
import { dockerApi, dockerErrorKind, type DockerAction } from "@/features/docker/api";
import { useConfirm } from "@/lib/useConfirm";
import { cn } from "@/lib/utils";

type SubTab = "overview" | "containers" | "images" | "logs";

interface DockerViewProps {
  /** The terminal session (tab) whose docker runtime is shown. */
  embeddedSessionID: string;
  embeddedSessionName: string;
}

const LOG_TAILS = [100, 200, 500, 1000];

/** Actions that visibly disrupt the workload — confirmed before running. */
const CONFIRMED_ACTIONS: ReadonlySet<DockerAction> = new Set(["stop", "restart", "kill", "pause"]);

/**
 * Docker panel (right side, next to Files/Agent/Monitor): engine overview,
 * container list with live stats and lifecycle actions, image list, and a
 * per-container log viewer. Everything runs on the backend through the
 * session's SSH exec channel (or the local docker CLI for local terminals);
 * a missing CLI or stopped daemon degrades to a calm hint.
 */
export function DockerView({ embeddedSessionID }: DockerViewProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<SubTab>("containers");
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

  const [showAll, setShowAll] = useState(true);
  const [logContainer, setLogContainer] = useState("");
  const [logTail, setLogTail] = useState(200);

  // The logs tab needs the container list for its picker, so containers are
  // fetched for both tabs.
  const containersEnabled = tab === "containers" || tab === "logs";
  const infoQ = useQuery({
    queryKey: ["docker-info", embeddedSessionID],
    queryFn: () => dockerApi.info(embeddedSessionID),
    enabled: tab === "overview",
    ...refetchOpts,
  });
  const containersQ = useQuery({
    queryKey: ["docker-containers", embeddedSessionID, showAll],
    queryFn: () => dockerApi.containers(embeddedSessionID, showAll),
    enabled: containersEnabled,
    ...refetchOpts,
  });
  const statsQ = useQuery({
    queryKey: ["docker-stats", embeddedSessionID],
    queryFn: () => dockerApi.stats(embeddedSessionID),
    enabled: tab === "containers",
    ...refetchOpts,
  });
  const imagesQ = useQuery({
    queryKey: ["docker-images", embeddedSessionID],
    queryFn: () => dockerApi.images(embeddedSessionID),
    enabled: tab === "images",
    ...refetchOpts,
  });
  const logsQ = useQuery({
    queryKey: ["docker-logs", embeddedSessionID, logContainer, logTail],
    queryFn: () => dockerApi.logs(embeddedSessionID, logContainer, logTail),
    enabled: tab === "logs" && logContainer !== "",
    // Logs are pull-only: no auto-refresh loop.
  });

  const activeQ =
    tab === "overview" ? infoQ : tab === "containers" ? containersQ : tab === "images" ? imagesQ : logsQ;

  // When the container list arrives and the picker selection vanished (state
  // change, removal), fall back to the first container.
  useEffect(() => {
    const list = containersQ.data ?? [];
    if (list.length > 0 && !list.some((c) => c.id === logContainer)) {
      setLogContainer(list[0].id);
    }
  }, [containersQ.data]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Lifecycle actions ─────────────────────────────────────────────
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<{ id: string; action: DockerAction } | null>(null);

  const actionMut = useMutation({
    mutationFn: (v: { id: string; name: string; action: DockerAction }) =>
      dockerApi.action(embeddedSessionID, v.id, v.action),
    onSettled: async () => {
      setPending(null);
      await queryClient.invalidateQueries({ queryKey: ["docker-containers", embeddedSessionID] });
      await queryClient.invalidateQueries({ queryKey: ["docker-stats", embeddedSessionID] });
    },
  });

  const runAction = async (id: string, name: string, action: DockerAction) => {
    if (CONFIRMED_ACTIONS.has(action)) {
      const ok = await askConfirm({
        title: t(`docker.action_${action}`),
        message: t("docker.actionConfirm", { action: t(`docker.action_${action}`), name }),
        confirmLabel: t(`docker.action_${action}`),
      });
      if (!ok) return;
    }
    setPending({ id, action });
    actionMut.mutate({ id, name, action });
  };

  const openLogs = (id: string) => {
    setLogContainer(id);
    setTab("logs");
  };

  const tabs: { id: SubTab; label: string }[] = [
    { id: "overview", label: t("docker.overview") },
    { id: "containers", label: t("docker.containers") },
    { id: "images", label: t("docker.images") },
    { id: "logs", label: t("docker.logs") },
  ];

  // Stats rows are matched by container ID prefix (`docker ps` truncates IDs
  // to 12 chars; `docker stats` prints the full ID).
  const statsById = useMemo(() => {
    const m = new Map<string, DockerContainerStats>();
    for (const s of statsQ.data ?? []) {
      if (s.containerId) m.set(s.containerId.slice(0, 12), s);
    }
    return m;
  }, [statsQ.data]);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Sub-tab strip + refresh */}
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
        {tabs.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setTab(s.id)}
            className={cn(
              "flex h-7 items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
              tab === s.id
                ? "bg-accent text-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
            )}
          >
            {s.label}
          </button>
        ))}
        {tab === "containers" && (
          <button
            type="button"
            onClick={() => setShowAll((v) => !v)}
            title={t("docker.showStopped")}
            className={cn(
              "ml-1 rounded px-1.5 py-0.5 text-[10px] transition-colors",
              showAll ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-accent/50",
            )}
          >
            {t("docker.showStopped")}
          </button>
        )}
        <div className="ml-auto flex items-center gap-1.5">
          {tab !== "logs" && (
            <span
              className="text-[10px] text-muted-foreground"
              title={t("docker.autoRefresh", { sec: intervalSec })}
            >
              {intervalSec}s
            </span>
          )}
          <button
            type="button"
            onClick={() => activeQ.refetch()}
            disabled={activeQ.isFetching}
            aria-label={t("docker.refresh")}
            title={t("docker.refresh")}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40"
          >
            <RefreshCw className={cn("h-3.5 w-3.5", activeQ.isFetching && "animate-spin")} />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="min-h-0 flex-1 overflow-y-auto p-2.5">
        {activeQ.isError ? (
          <ErrorHint error={activeQ.error} />
        ) : tab === "overview" ? (
          <OverviewPane q={infoQ} />
        ) : tab === "containers" ? (
          <ContainersPane
            loading={containersQ.isLoading}
            containers={containersQ.data ?? []}
            statsById={statsById}
            pending={pending}
            actionError={actionMut.isError ? actionMut.error : null}
            onAction={runAction}
            onLogs={openLogs}
          />
        ) : tab === "images" ? (
          <ImagesPane loading={imagesQ.isLoading} images={imagesQ.data ?? []} />
        ) : (
          <LogsPane
            containers={containersQ.data ?? []}
            selected={logContainer}
            onSelect={setLogContainer}
            tail={logTail}
            onTail={setLogTail}
            query={logsQ}
          />
        )}
      </div>
    </div>
  );
}

// ── Error hint ───────────────────────────────────────────────────────

function ErrorHint({ error }: { error: unknown }) {
  const { t } = useTranslation();
  const kind = dockerErrorKind(error);
  const titleKey =
    kind === "notInstalled" ? "docker.notInstalled" : kind === "daemonDown" ? "docker.daemonDown" : "docker.unavailable";
  const descKey =
    kind === "notInstalled"
      ? "docker.notInstalledDesc"
      : kind === "daemonDown"
        ? "docker.daemonDownDesc"
        : "docker.unavailableDesc";
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center"
      title={String((error as Error)?.message ?? "")}
    >
      <ContainerIcon className="h-8 w-8 text-muted-foreground/40" />
      <div className="text-sm font-medium text-foreground">{t(titleKey)}</div>
      <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">{t(descKey)}</p>
    </div>
  );
}

// ── Overview ─────────────────────────────────────────────────────────

function OverviewPane({
  q,
}: {
  q: ReturnType<typeof useQuery<import("@/../bindings/github.com/ai-remote/workspace/internal/domain").DockerInfo>>;
}) {
  const { t } = useTranslation();
  if (q.isLoading || !q.data) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  const d = q.data;

  return (
    <div className="flex flex-col gap-2.5">
      <div className="grid grid-cols-2 gap-2">
        <InfoCard label={t("docker.version")} value={d.version || "—"} footer={`API ${d.apiVersion || "—"}`} />
        <InfoCard
          label={t("docker.platform")}
          value={`${d.osType || "—"}/${d.arch || "—"}`}
          footer={d.kernelVersion || ""}
        />
      </div>

      <div className="grid grid-cols-3 gap-2">
        <InfoCard label={t("docker.state_running")} value={String(d.containersRunning)} accent />
        <InfoCard label={t("docker.state_paused")} value={String(d.containersPaused)} />
        <InfoCard label={t("docker.state_stopped")} value={String(d.containersStopped)} />
      </div>

      <div className="grid grid-cols-2 gap-2">
        <InfoCard label={t("docker.imagesCount")} value={String(d.images)} icon={<Package className="h-3 w-3 text-primary" />} />
        <InfoCard label={t("docker.rootDir")} value={d.dockerRootDir || "—"} mono />
      </div>
    </div>
  );
}

function InfoCard({
  label,
  value,
  footer,
  icon,
  mono,
  accent,
}: {
  label: string;
  value: string;
  footer?: string;
  icon?: React.ReactNode;
  mono?: boolean;
  accent?: boolean;
}) {
  return (
    <div className="flex flex-col gap-1 rounded-[var(--radius)] border border-border bg-card p-2">
      <span className="flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground">
        {icon}
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

// ── Containers ───────────────────────────────────────────────────────

function stateDotClass(state: string) {
  switch (state) {
    case "running":
      return "bg-success";
    case "paused":
      return "bg-amber-400";
    case "restarting":
      return "bg-primary animate-pulse";
    default:
      return "bg-muted-foreground/40";
  }
}

function ContainersPane({
  loading,
  containers,
  statsById,
  pending,
  actionError,
  onAction,
  onLogs,
}: {
  loading: boolean;
  containers: import("@/../bindings/github.com/ai-remote/workspace/internal/domain").DockerContainer[];
  statsById: Map<string, DockerContainerStats>;
  pending: { id: string; action: DockerAction } | null;
  actionError: unknown;
  onAction: (id: string, name: string, action: DockerAction) => void;
  onLogs: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }

  const ActionBtn = ({
    container,
    name,
    action,
    title,
    children,
  }: {
    container: string;
    name: string;
    action: DockerAction;
    title: string;
    children: React.ReactNode;
  }) => {
    const busy = pending?.id === container && pending?.action === action;
    return (
      <button
        type="button"
        disabled={pending !== null}
        onClick={() => onAction(container, name, action)}
        title={title}
        className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40"
      >
        {busy ? <RefreshCw className="h-3 w-3 animate-spin" /> : children}
      </button>
    );
  };

  const actionErrMsg = actionError ? String((actionError as Error)?.message ?? actionError) : "";

  return (
    <div className="flex flex-col text-xs">
      {actionErrMsg && (
        <div
          className="mb-1.5 truncate rounded border border-destructive/40 bg-destructive/10 px-2 py-1 text-[10px] text-destructive"
          title={actionErrMsg}
        >
          {t("docker.actionFailed")}: {actionErrMsg}
        </div>
      )}
      <div className="divide-y divide-border/50">
        {containers.map((c) => {
          const st = statsById.get(c.id);
          const running = c.state === "running";
          const paused = c.state === "paused";
          const name = c.names?.split(",")[0] || c.id;
          return (
            <div key={c.id} className="flex flex-col gap-0.5 px-1 py-1.5 hover:bg-accent/40">
              <div className="flex items-center gap-1.5">
                <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", stateDotClass(c.state))} />
                <span className="min-w-0 flex-1 truncate font-medium" title={c.names}>
                  {name}
                </span>
                <ActionBtn container={c.id} name={name} action="restart" title={t("docker.action_restart")}>
                  <RotateCw className="h-3 w-3" />
                </ActionBtn>
                {running || paused ? (
                  <ActionBtn container={c.id} name={name} action="stop" title={t("docker.action_stop")}>
                    <Square className="h-3 w-3" />
                  </ActionBtn>
                ) : (
                  <ActionBtn container={c.id} name={name} action="start" title={t("docker.action_start")}>
                    <Play className="h-3 w-3" />
                  </ActionBtn>
                )}
                {paused ? (
                  <ActionBtn container={c.id} name={name} action="unpause" title={t("docker.action_unpause")}>
                    <Play className="h-3 w-3" />
                  </ActionBtn>
                ) : running ? (
                  <ActionBtn container={c.id} name={name} action="pause" title={t("docker.action_pause")}>
                    <Pause className="h-3 w-3" />
                  </ActionBtn>
                ) : null}
                <button
                  type="button"
                  onClick={() => onLogs(c.id)}
                  title={t("docker.viewLogs")}
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <FileText className="h-3 w-3" />
                </button>
              </div>
              <div className="flex items-center gap-2 pl-3 text-[10px] text-muted-foreground">
                <span className="max-w-[45%] truncate font-mono" title={c.image}>
                  {c.image}
                </span>
                <span className="shrink-0">{c.status}</span>
                {st && (
                  <span className="ml-auto shrink-0 font-mono tabular-nums">
                    <span title={t("docker.cpuUsage")}>{st.cpuPercent}</span>
                    <span className="mx-1 text-muted-foreground/40">·</span>
                    <span title={t("docker.memUsage")}>{st.memUsage}</span>
                  </span>
                )}
              </div>
              {c.ports && (
                <div className="truncate pl-3 font-mono text-[10px] text-muted-foreground/60" title={c.ports}>
                  {c.ports}
                </div>
              )}
            </div>
          );
        })}
        {containers.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("docker.noContainers")}</div>
        )}
      </div>
    </div>
  );
}

// ── Images ───────────────────────────────────────────────────────────

function ImagesPane({
  loading,
  images,
}: {
  loading: boolean;
  images: import("@/../bindings/github.com/ai-remote/workspace/internal/domain").DockerImage[];
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  return (
    <div className="flex flex-col text-xs">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-background px-1 pb-1.5 font-medium text-muted-foreground">
        <span className="min-w-0 flex-1">{t("docker.repository")}</span>
        <span className="w-16 shrink-0">{t("docker.tag")}</span>
        <span className="w-20 shrink-0">{t("docker.created")}</span>
        <span className="w-16 shrink-0 text-right">{t("docker.size")}</span>
      </div>
      <div className="divide-y divide-border/50">
        {images.map((im) => (
          <div key={`${im.repository}-${im.tag}-${im.id}`} className="flex items-center gap-2 px-1 py-1 hover:bg-accent/40">
            <span className="min-w-0 flex-1 truncate font-mono text-[11px]" title={`${im.repository}:${im.tag} (${im.id})`}>
              {im.repository}
            </span>
            <span className="w-16 shrink-0 truncate text-[11px] text-muted-foreground">{im.tag}</span>
            <span className="w-20 shrink-0 truncate text-[10px] text-muted-foreground">{im.createdSince}</span>
            <span className="w-16 shrink-0 text-right font-mono text-[11px] tabular-nums">{im.size}</span>
          </div>
        ))}
        {images.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("docker.noData")}</div>
        )}
      </div>
    </div>
  );
}

// ── Logs ─────────────────────────────────────────────────────────────

function LogsPane({
  containers,
  selected,
  onSelect,
  tail,
  onTail,
  query,
}: {
  containers: import("@/../bindings/github.com/ai-remote/workspace/internal/domain").DockerContainer[];
  selected: string;
  onSelect: (id: string) => void;
  tail: number;
  onTail: (n: number) => void;
  query: ReturnType<typeof useQuery<string>>;
}) {
  const { t } = useTranslation();
  const bottomRef = useRef<HTMLDivElement>(null);

  // Newest lines are last with --tail; keep the view pinned there.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: "end" });
  }, [query.data]);

  const selectedContainer = containers.find((c) => c.id === selected);

  return (
    <div className="flex h-full flex-col gap-1.5">
      <div className="flex shrink-0 items-center gap-1.5">
        <select
          value={selected}
          onChange={(e) => onSelect(e.target.value)}
          aria-label={t("docker.selectContainer")}
          className="h-7 min-w-0 flex-1 rounded-[var(--radius)] border border-border bg-background px-1.5 text-xs text-foreground"
        >
          {containers.length === 0 && <option value="">{t("docker.noContainers")}</option>}
          {containers.map((c) => (
            <option key={c.id} value={c.id}>
              {c.names?.split(",")[0] || c.id} ({c.state})
            </option>
          ))}
        </select>
        <select
          value={tail}
          onChange={(e) => onTail(Number(e.target.value))}
          aria-label={t("docker.tail")}
          title={t("docker.tail")}
          className="h-7 shrink-0 rounded-[var(--radius)] border border-border bg-background px-1.5 text-xs text-foreground"
        >
          {LOG_TAILS.map((n) => (
            <option key={n} value={n}>
              {n} {t("docker.lines")}
            </option>
          ))}
        </select>
      </div>

      <div className="min-h-0 flex-1 overflow-auto rounded-[var(--radius)] border border-border bg-background p-1.5">
        {query.isError ? (
          <ErrorHint error={query.error} />
        ) : query.isLoading ? (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>
        ) : query.data ? (
          <pre className="whitespace-pre-wrap break-all font-mono text-[10px] leading-relaxed text-foreground/90">
            {query.data}
            <span ref={bottomRef} className="inline-block h-px w-0" />
          </pre>
        ) : (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("docker.selectContainer")}</div>
        )}
      </div>

      {selectedContainer && (
        <div className="shrink-0 truncate text-center text-[10px] text-muted-foreground/70" title={selectedContainer.image}>
          {selectedContainer.image}
        </div>
      )}
    </div>
  );
}

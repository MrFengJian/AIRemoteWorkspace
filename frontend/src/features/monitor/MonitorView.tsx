import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronUp,
  Copy,
  RefreshCw,
  Skull,
  Square,
  TerminalSquare,
} from "lucide-react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type { MonitorPort, MonitorProcess } from "@/../bindings/github.com/ai-remote/workspace/internal/domain";
import { monitorApi, fmtKB, fmtRate, fmtUptime } from "@/features/monitor/api";
import { ContextMenu, type MenuItem } from "@/components/ui/ContextMenu";
import { insertToTerminal } from "@/lib/insertTerminal";
import { toast } from "@/lib/toast";
import { useUIStore } from "@/stores/ui.store";
import { cn } from "@/lib/utils";

type SubTab = "overview" | "processes" | "ports";
type ProcSortKey = "cpu" | "rss" | "pid";

interface MonitorViewProps {
  /** The terminal session (tab) whose host is monitored. */
  embeddedSessionID: string;
  embeddedSessionName: string;
  /** Local terminal session: on Linux/macOS the app collects from its own
   *  machine; on Windows there is no lightweight native channel — the panel
   *  explains that instead of firing failing requests. */
  isLocal?: boolean;
}

/** Windows has no sh//proc/sysctl-style native channel for this collector. */
const isWindowsHost = () =>
  typeof navigator !== "undefined" && /win/i.test(navigator.userAgent);

/**
 * Host monitor panel (right side, next to Files/Agent): overview gauges,
 * a sortable process list, and listening ports — collected on the backend
 * over the session's SSH connection, or directly from the local machine for
 * local terminals on Linux/macOS. Auto-refreshes on the globally configured
 * interval while visible.
 */
export function MonitorView({ embeddedSessionID, embeddedSessionName, isLocal }: MonitorViewProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<SubTab>("overview");
  const [intervalSec, setIntervalSec] = useState(60);
  /** Panel context menu: proc row | port row | metric card (overview) | background. */
  const [menu, setMenu] = useState<
    | { x: number; y: number; entry: { kind: "proc"; proc: MonitorProcess } | { kind: "port"; port: MonitorPort } }
    | { x: number; y: number; metric: { label: string; value: string } }
    | { x: number; y: number }
    | null
  >(null);
  const setView = useUIStore((s) => s.setView);

  /** Copy text with a quiet confirmation. */
  const copyText = useCallback(
    async (text: string) => {
      try {
        await navigator.clipboard.writeText(text);
        toast.info(t("monitor.copied"));
      } catch {
        /* clipboard unavailable */
      }
    },
    [t],
  );

  /** Insert a diagnostic command into the active terminal (review + Enter). */
  const insertCommand = useCallback(
    (cmd: string) => {
      if (insertToTerminal(cmd)) {
        setView("terminal");
      } else {
        toast.info(t("monitor.noTerminal"));
      }
    },
    [setView, t],
  );

  /** Context-menu items per right-click target. Declared as a function so
   *  it can close over `activeQ` (defined below) at call time. */
  function buildMenuItems(m: NonNullable<typeof menu>): MenuItem[] {
    if ("metric" in m) {
      // Overview metric card: copy the displayed value.
      return [
        {
          label: t("monitor.copyMetric", { value: m.metric.value }),
          icon: Copy,
          onClick: () => void copyText(m.metric.value),
        },
      ];
    }
    if ("entry" in m && m.entry.kind === "proc") {
      const p = m.entry.proc;
      return [
        {
          label: t("monitor.killProc", { pid: p.pid }),
          icon: Square,
          onClick: () => insertCommand(`kill ${p.pid}`),
        },
        {
          label: t("monitor.kill9Proc", { pid: p.pid }),
          icon: Skull,
          onClick: () => insertCommand(`kill -9 ${p.pid}`),
        },
        { type: "separator" },
        {
          label: t("monitor.copyPid"),
          icon: Copy,
          onClick: () => void copyText(String(p.pid)),
        },
        {
          label: t("monitor.copyProcName"),
          icon: Copy,
          onClick: () => void copyText(p.name),
        },
        {
          label: t("monitor.copyProcCmd"),
          icon: Copy,
          onClick: () => void copyText(p.commandLine || p.name),
        },
      ];
    }
    if ("entry" in m && m.entry.kind === "port") {
      const port = m.entry.port;
      return [
        {
          label: t("monitor.portOwner", { port: port.port }),
          icon: TerminalSquare,
          onClick: () => insertCommand(`ss -tnlp | grep ':${port.port} '`),
        },
        { type: "separator" },
        {
          label: t("monitor.copyPort"),
          icon: Copy,
          onClick: () => void copyText(String(port.port)),
        },
        {
          label: t("monitor.copyAddr"),
          icon: Copy,
          onClick: () => void copyText(port.address),
        },
      ];
    }
    // Background: refresh + auto-refresh settings deep link.
    return [
      {
        label: t("monitor.refresh"),
        icon: RefreshCw,
        onClick: () => void activeQ.refetch(),
      },
      {
        label: t("monitor.openAutoRefreshSettings"),
        icon: RefreshCw,
        onClick: () => {
          useUIStore.getState().setView("settings");
          useUIStore.getState().setSettingsCategory("advanced");
        },
      },
    ];
  }

  // Windows local terminals have nothing to collect through — explain
  // instead of running requests that can only fail.
  if (isLocal && isWindowsHost()) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
        <Activity className="h-8 w-8 text-muted-foreground/40" />
        <div className="text-sm font-medium text-foreground">{t("monitor.localUnsupported")}</div>
        <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">
          {t("monitor.localUnsupportedDesc")}
        </p>
      </div>
    );
  }

  // The refresh interval lives in the global settings (Advanced). Read once
  // per panel mount; reopening the panel picks up changes.
  useEffect(() => {
    ConfigService.GetAppConfig()
      .then((cfg) => {
        if (cfg.monitorIntervalSeconds > 0) setIntervalSec(cfg.monitorIntervalSeconds);
      })
      .catch(() => {});
  }, []);

  const intervalMs = Math.max(5, intervalSec) * 1000;
  const refetchOpts = { refetchInterval: intervalMs } as const;

  const overviewQ = useQuery({
    queryKey: ["monitor-overview", embeddedSessionID],
    queryFn: () => monitorApi.overview(embeddedSessionID),
    enabled: tab === "overview",
    ...refetchOpts,
  });
  const processesQ = useQuery({
    queryKey: ["monitor-processes", embeddedSessionID],
    queryFn: () => monitorApi.processes(embeddedSessionID),
    enabled: tab === "processes",
    ...refetchOpts,
  });
  const portsQ = useQuery({
    queryKey: ["monitor-ports", embeddedSessionID],
    queryFn: () => monitorApi.ports(embeddedSessionID),
    enabled: tab === "ports",
    ...refetchOpts,
  });

  const activeQ = tab === "overview" ? overviewQ : tab === "processes" ? processesQ : portsQ;

  // ── Process sorting ────────────────────────────────────────────────
  const [sortKey, setSortKey] = useState<ProcSortKey>("cpu");
  const [sortAsc, setSortAsc] = useState(false);
  const sortedProcs = useMemo(() => {
    const list = [...(processesQ.data ?? [])];
    list.sort((a, b) => {
      const d =
        sortKey === "cpu"
          ? a.cpuPercent - b.cpuPercent
          : sortKey === "rss"
            ? a.rssKb - b.rssKb
            : a.pid - b.pid;
      return sortAsc ? d : -d;
    });
    return list;
  }, [processesQ.data, sortKey, sortAsc]);
  const toggleSort = (k: ProcSortKey) => {
    if (sortKey === k) setSortAsc((v) => !v);
    else {
      setSortKey(k);
      setSortAsc(false);
    }
  };

  const tabs: { id: SubTab; label: string }[] = [
    { id: "overview", label: t("monitor.overview") },
    { id: "processes", label: t("monitor.processes") },
    { id: "ports", label: t("monitor.ports") },
  ];

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
        <div className="ml-auto flex items-center gap-1.5">
          <span className="text-[10px] text-muted-foreground" title={t("monitor.autoRefresh", { sec: intervalSec })}>
            {intervalSec}s
          </span>
          <button
            type="button"
            onClick={() => activeQ.refetch()}
            disabled={activeQ.isFetching}
            aria-label={t("monitor.refresh")}
            title={t("monitor.refresh")}
            className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40"
          >
            <RefreshCw className={cn("h-3.5 w-3.5", activeQ.isFetching && "animate-spin")} />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="min-h-0 flex-1 overflow-y-auto p-2.5">
        {activeQ.isError ? (
          // Any collection failure (unsupported platform, missing channel,
          // empty output) degrades to a calm hint; the raw reason stays
          // available as a tooltip rather than an in-your-face error.
          <div
            className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center"
            title={String((activeQ.error as Error)?.message ?? "")}
          >
            <Activity className="h-8 w-8 text-muted-foreground/40" />
            <div className="text-sm font-medium text-foreground">{t("monitor.unavailable")}</div>
            <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">
              {t("monitor.unavailableDesc")}
            </p>
          </div>
        ) : tab === "overview" ? (
          <OverviewPane
            q={overviewQ}
            onMetricMenu={(x, y, metric) => setMenu({ x, y, metric })}
          />
        ) : tab === "processes" ? (
          <ProcessesPane
            loading={processesQ.isLoading}
            procs={sortedProcs}
            sortKey={sortKey}
            sortAsc={sortAsc}
            onSort={toggleSort}
            onMenu={(x, y, p) => setMenu({ x, y, entry: { kind: "proc", proc: p } })}
          />
        ) : (
          <PortsPane
            loading={portsQ.isLoading}
            ports={portsQ.data ?? []}
            onMenu={(x, y, port) => setMenu({ x, y, entry: { kind: "port", port } })}
          />
        )}
      </div>

      {/* Context menu (process row / port row / metric card / background) */}
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          items={buildMenuItems(menu)}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}

// ── Overview ─────────────────────────────────────────────────────────

function OverviewPane({
  q,
  onMetricMenu,
}: {
  q: ReturnType<typeof useQuery<import("@/../bindings/github.com/ai-remote/workspace/internal/domain").MonitorOverview>>;
  onMetricMenu: (x: number, y: number, metric: { label: string; value: string }) => void;
}) {
  const { t } = useTranslation();
  if (q.isLoading || !q.data) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  const o = q.data;

  return (
    <div className="flex flex-col gap-2.5">
      {/* CPU */}
      <MetricCard
        label={t("monitor.cpu")}
        value={`${o.cpuPercent.toFixed(1)}%`}
        onMenu={onMetricMenu}
        footer={
          <>
            <Bar percent={o.cpuPercent} />
            <div className="mt-1.5 truncate text-[10px] text-muted-foreground">
              {o.cpuModel || "—"} · {t("monitor.cores", { n: o.cpuCores || 1 })}
            </div>
          </>
        }
      />

      {/* Memory + swap */}
      <MetricCard
        label={t("monitor.memory")}
        value={`${fmtKB(o.memUsedKb)} / ${fmtKB(o.memTotalKb)}`}
        onMenu={onMetricMenu}
        footer={
          <>
            <Bar percent={o.memUsedPercent} />
            {o.swapTotalKb > 0 && (
              <div className="mt-1.5 text-[10px] text-muted-foreground">
                {t("monitor.swap")}: {fmtKB(o.swapUsedKb)} / {fmtKB(o.swapTotalKb)}
              </div>
            )}
          </>
        }
      />

      {/* Network rates */}
      <div className="grid grid-cols-2 gap-2">
        <MetricCard label={t("monitor.down")} value={fmtRate(o.netRxBytesPerSec)} icon={<ArrowDown className="h-3 w-3 text-primary" />} onMenu={onMetricMenu} />
        <MetricCard label={t("monitor.up")} value={fmtRate(o.netTxBytesPerSec)} icon={<ArrowUp className="h-3 w-3 text-success" />} onMenu={onMetricMenu} />
      </div>

      {/* Load + processes + uptime */}
      <div className="grid grid-cols-2 gap-2">
        <MetricCard
          label={t("monitor.load")}
          value={o.load1.toFixed(2)}
          onMenu={onMetricMenu}
          footer={<div className="text-[10px] text-muted-foreground">5s {o.load5.toFixed(2)} · 15s {o.load15.toFixed(2)}</div>}
        />
        <MetricCard
          label={t("monitor.procCount")}
          value={String(o.processTotal)}
          onMenu={onMetricMenu}
          footer={<div className="text-[10px] text-muted-foreground">{t("monitor.running")}: {o.processRunning}</div>}
        />
      </div>

      <div className="grid grid-cols-2 gap-2">
        <MetricCard label={t("monitor.uptime")} value={fmtUptime(o.uptimeSeconds)} onMenu={onMetricMenu} />
        <MetricCard
          label={t("monitor.tcpConnections")}
          value={String(o.tcpStates?.reduce((a, s) => a + s.count, 0) ?? 0)}
          onMenu={onMetricMenu}
          footer={
            <div className="space-y-0.5 text-[10px] text-muted-foreground">
              {(o.tcpStates ?? []).slice(0, 3).map((s) => (
                <div key={s.state} className="flex justify-between gap-2">
                  <span className="truncate">{s.state}</span>
                  <span>{s.count}</span>
                </div>
              ))}
            </div>
          }
        />
      </div>

      {/* Disks */}
      <div className="flex flex-col gap-1.5">
        <span className="text-[11px] font-medium text-muted-foreground">{t("monitor.disk")}</span>
        {(o.disks ?? []).length === 0 ? (
          <span className="text-[10px] text-muted-foreground">{t("monitor.noData")}</span>
        ) : (
          (o.disks ?? []).map((d) => (
            <div key={`${d.device}-${d.mount}`} className="flex flex-col gap-0.5">
              <div className="flex items-baseline justify-between gap-2 text-[10px]">
                <span className="truncate text-muted-foreground" title={`${d.device} → ${d.mount}`}>
                  {d.mount}
                </span>
                <span className="shrink-0 tabular-nums text-muted-foreground">
                  {fmtKB(d.usedKb)} / {fmtKB(d.totalKb)}
                </span>
              </div>
              <Bar percent={d.usedPercent} warn={d.usedPercent >= 90} />
            </div>
          ))
        )}
      </div>

      {o.kernel && (
        <div className="pt-1 text-center text-[10px] text-muted-foreground/70">{o.kernel}</div>
      )}
    </div>
  );
}

function MetricCard({
  label,
  value,
  icon,
  footer,
  accent,
  onMenu,
}: {
  label: string;
  value: string;
  icon?: ReactNode;
  footer?: ReactNode;
  accent?: boolean;
  /** Right-click: expose a copy-value menu for this metric. */
  onMenu?: (x: number, y: number, metric: { label: string; value: string }) => void;
}) {
  return (
    <div
      className="flex flex-col gap-1.5 rounded-[var(--radius)] border border-border bg-card p-2.5"
      onContextMenu={
        onMenu
          ? (e) => {
              e.preventDefault();
              e.stopPropagation();
              onMenu(e.clientX, e.clientY, { label, value });
            }
          : undefined
      }
    >
      <div className="flex items-center justify-between gap-2">
        <span className={cn("flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground", accent && "text-foreground")}>
          {icon}
          <span className="truncate">{label}</span>
        </span>
        <span className="shrink-0 font-mono text-sm font-medium tabular-nums">{value}</span>
      </div>
      {footer}
    </div>
  );
}

function Bar({ percent, warn }: { percent: number; warn?: boolean }) {
  const p = Math.max(0, Math.min(100, percent));
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
      <div
        className={cn("h-full rounded-full transition-all", warn ? "bg-destructive" : "bg-primary")}
        style={{ width: `${p}%` }}
      />
    </div>
  );
}

// ── Processes ────────────────────────────────────────────────────────

function ProcessesPane({
  loading,
  procs,
  sortKey,
  sortAsc,
  onSort,
  onMenu,
}: {
  loading: boolean;
  procs: import("@/../bindings/github.com/ai-remote/workspace/internal/domain").MonitorProcess[];
  sortKey: ProcSortKey;
  sortAsc: boolean;
  onSort: (k: ProcSortKey) => void;
  onMenu: (x: number, y: number, proc: MonitorProcess) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  const SortIcon = ({ k }: { k: ProcSortKey }) =>
    sortKey !== k ? null : sortAsc ? (
      <ChevronUp className="h-3 w-3" />
    ) : (
      <ChevronDown className="h-3 w-3" />
    );

  return (
    <div className="flex flex-col text-xs">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-background px-1 pb-1.5 font-medium text-muted-foreground">
        <button type="button" onClick={() => onSort("pid")} className="flex w-12 shrink-0 items-center gap-0.5 hover:text-foreground">
          {t("monitor.pid")} <SortIcon k="pid" />
        </button>
        <span className="w-28 shrink-0 truncate">{t("monitor.name")}</span>
        <button type="button" onClick={() => onSort("cpu")} className="flex w-16 shrink-0 items-center justify-end gap-0.5 hover:text-foreground">
          CPU% <SortIcon k="cpu" />
        </button>
        <button type="button" onClick={() => onSort("rss")} className="flex w-20 shrink-0 items-center justify-end gap-0.5 hover:text-foreground">
          {t("monitor.memUsage")} <SortIcon k="rss" />
        </button>
      </div>
      <div className="divide-y divide-border/50">
        {procs.map((p) => (
          <div
            key={p.pid}
            className="flex items-center gap-2 px-1 py-1 hover:bg-accent/40"
            onContextMenu={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onMenu(e.clientX, e.clientY, p);
            }}
          >
            <span className="w-12 shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">{p.pid}</span>
            <span className="w-28 shrink-0 truncate" title={p.commandLine || p.name}>
              {p.name || "??"}
            </span>
            <span className="w-16 shrink-0 text-right font-mono text-[11px] tabular-nums">
              {p.cpuPercent.toFixed(1)}
            </span>
            <span className="w-20 shrink-0 text-right font-mono text-[11px] tabular-nums text-muted-foreground">
              {fmtKB(p.rssKb)}
            </span>
            {p.commandLine && (
              <span className="min-w-0 flex-1 truncate text-[10px] text-muted-foreground/60" title={p.commandLine}>
                {p.commandLine}
              </span>
            )}
          </div>
        ))}
        {procs.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("monitor.noData")}</div>
        )}
      </div>
    </div>
  );
}

// ── Ports ────────────────────────────────────────────────────────────

function PortsPane({
  loading,
  ports,
  onMenu,
}: {
  loading: boolean;
  ports: import("@/../bindings/github.com/ai-remote/workspace/internal/domain").MonitorPort[];
  onMenu: (x: number, y: number, port: MonitorPort) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <div className="p-6 text-center text-xs text-muted-foreground">{t("common.loading")}</div>;
  }
  return (
    <div className="flex flex-col text-xs">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-background px-1 pb-1.5 font-medium text-muted-foreground">
        <span className="w-16 shrink-0">{t("monitor.port")}</span>
        <span className="w-14 shrink-0">{t("monitor.proto")}</span>
        <span className="min-w-0 flex-1">{t("monitor.address")}</span>
        <span className="w-12 shrink-0 text-right">{t("monitor.sockets")}</span>
      </div>
      <div className="divide-y divide-border/50">
        {ports.map((p) => (
          <div
            key={`${p.proto}-${p.address}-${p.port}`}
            className="flex items-center gap-2 px-1 py-1 hover:bg-accent/40"
            onContextMenu={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onMenu(e.clientX, e.clientY, p);
            }}
          >
            <span className="w-16 shrink-0 font-mono text-[11px] font-medium tabular-nums">{p.port}</span>
            <span className="w-14 shrink-0 text-[11px] text-muted-foreground">{p.proto}</span>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px]" title={p.address}>
              {p.address}
            </span>
            {p.count > 1 && (
              <span className="w-12 shrink-0 text-right font-mono text-[11px] tabular-nums text-muted-foreground">
                ×{p.count}
              </span>
            )}
          </div>
        ))}
        {ports.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">{t("monitor.noData")}</div>
        )}
      </div>
    </div>
  );
}

import { useEffect, useState, type MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  Copy as CopyIcon,
  Loader2,
  Network,
  Pencil,
  Play,
  RefreshCw,
  Square,
} from "lucide-react";
import { cn } from "@/lib/utils";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ContextMenu, type MenuItem } from "@/components/ui/ContextMenu";
import { useHosts } from "@/features/hosts/hooks";
import { useHostsUIStore } from "@/features/hosts/store";
import { errorMessage, toast } from "@/lib/toast";
import type {
  TunnelState,
  TunnelStatusDTO,
} from "./api";
import { tunnelApi, tunnelKey, tunnelRuleText, tunnelSshCommand } from "./api";
import { subscribeTunnelEvents, useTunnelStore } from "./store";

/**
 * TunnelPanel — the right-panel tab showing SSH tunnel status for every host
 * that has one (or has one configured). Status updates stream live via the
 * tunnel store; the active host's row is highlighted. Start/stop are manual
 * overrides — auto-start happens when a terminal tab opens on the host.
 */
export function TunnelPanel({ embeddedHostID }: { embeddedHostID?: string }) {
  const { t } = useTranslation();
  const statuses = useTunnelStore((s) => s.statuses);
  const apply = useTunnelStore((s) => s.apply);
  const openEditor = useHostsUIStore((s) => s.openEditor);

  // Context menu: tunnel rule row | panel background.
  const [menu, setMenu] = useState<
    | { kind: "row"; status: TunnelStatusDTO; x: number; y: number }
    | { kind: "bg"; x: number; y: number }
    | null
  >(null);

  // Live event feed + one-time initial snapshot seeding.
  useEffect(() => {
    subscribeTunnelEvents();
    tunnelApi
      .list()
      .then((list) => list.forEach(apply))
      .catch(() => {});
  }, [apply]);

  // A host can configure several tunnels; render one row per ENABLED rule,
  // merged with its live status (rules not started yet render as stopped).
  // Statuses of unknown hosts (deleted mid-session) stay visible until they
  // finish shutting down.
  const { data: hosts, isLoading: hostsLoading } = useHosts();

  const rows: TunnelStatusDTO[] = (() => {
    const consumed = new Set<string>();
    const out: TunnelStatusDTO[] = [];
    for (const h of hosts ?? []) {
      for (const rule of h.tunnels ?? []) {
        if (!rule.enabled) continue;
        const storeKey = `${h.id}|${tunnelKey(rule)}`;
        consumed.add(storeKey);
        const st = statuses[storeKey];
        out.push(
          st ?? {
            hostId: h.id,
            hostName: h.name,
            key: tunnelKey(rule),
            config: rule,
            state: "stopped",
            retries: 0,
          },
        );
      }
    }
    for (const [k, st] of Object.entries(statuses)) {
      if (!consumed.has(k) && !(hosts ?? []).some((h) => h.id === st.hostId)) {
        out.push(st);
      }
    }
    // Active host first, then by host name.
    out.sort((a, b) => {
      if (a.hostId === embeddedHostID) return -1;
      if (b.hostId === embeddedHostID) return 1;
      return a.hostName.localeCompare(b.hostName);
    });
    return out;
  })();

  const act = async (hostId: string, action: "start" | "stop") => {
    try {
      await tunnelApi[action](hostId);
    } catch (e) {
      toast.error(`${t("tunnel.actionFailed")}: ${errorMessage(e)}`);
    }
  };

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.info(t("tunnel.copied"));
    } catch {
      toast.error(t("common.clipboardFailed"));
    }
  };

  /** Re-seed the store from a full backend snapshot. */
  const refresh = () => {
    tunnelApi
      .list()
      .then((list) => {
        useTunnelStore.setState((prev) => {
          const next = { ...prev.statuses };
          for (const s of list) next[`${s.hostId}|${s.key}`] = s;
          return { statuses: next };
        });
      })
      .catch(() => {});
  };

  // ── Context menus (native menu suppressed) ─────────────────────────
  const rowMenuItems = (status: TunnelStatusDTO): MenuItem[] => {
    const running = status.state !== "stopped";
    const items: MenuItem[] = [
      running
        ? {
            label: t("tunnel.stop"),
            icon: Square,
            onClick: () => void act(status.hostId, "stop"),
          }
        : {
            label: t("tunnel.start"),
            icon: Play,
            onClick: () => void act(status.hostId, "start"),
          },
      { type: "separator" },
      {
        label: t("tunnel.copySsh"),
        icon: CopyIcon,
        onClick: () =>
          void copyText(
            tunnelSshCommand(
              status.config,
              (hosts ?? []).find((h) => h.id === status.hostId),
            ),
          ),
      },
    ];
    if (status.lastError && status.state !== "connected") {
      items.push({
        label: t("tunnel.copyError"),
        icon: CopyIcon,
        onClick: () => void copyText(status.lastError ?? ""),
      });
    }
    const host = (hosts ?? []).find((h) => h.id === status.hostId);
    items.push(
      { type: "separator" },
      {
        label: t("tunnel.editHost"),
        icon: Pencil,
        disabled: !host,
        onClick: () => {
          if (host) openEditor(host);
        },
      },
    );
    return items;
  };

  const bgMenuItems = (): MenuItem[] => [
    {
      label: t("tunnel.refresh"),
      icon: RefreshCw,
      onClick: refresh,
    },
  ];

  return (
    <div
      className="flex h-full flex-col overflow-auto p-3"
      onContextMenu={(e: MouseEvent) => {
        // Suppress the browser menu; plain background gets refresh actions.
        e.preventDefault();
        setMenu({ kind: "bg", x: e.clientX, y: e.clientY });
      }}
    >
      <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
        <Network className="h-3.5 w-3.5 shrink-0" />
        <span>{t("tunnel.hint")}</span>
      </div>

      {hostsLoading ? (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
        </div>
      ) : rows.length === 0 ? (
        <p className="py-8 text-center text-xs text-muted-foreground">
          {t("tunnel.empty")}
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          {rows.map((row) => (
            <TunnelRow
              key={`${row.hostId}|${row.key}`}
              status={row}
              active={row.hostId === embeddedHostID}
              onStart={() => void act(row.hostId, "start")}
              onStop={() => void act(row.hostId, "stop")}
              onContextMenu={(e) => {
                e.preventDefault();
                e.stopPropagation();
                setMenu({ kind: "row", status: row, x: e.clientX, y: e.clientY });
              }}
            />
          ))}
        </div>
      )}
      {/* Context menu (rule row / panel background) */}
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          items={menu.kind === "row" ? rowMenuItems(menu.status) : bgMenuItems()}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}

function TunnelRow({
  status,
  active,
  onStart,
  onStop,
  onContextMenu,
}: {
  status: TunnelStatusDTO;
  active: boolean;
  onStart: () => void;
  onStop: () => void;
  onContextMenu: (e: MouseEvent) => void;
}) {
  const { t } = useTranslation();
  const running = status.state !== "stopped";
  const connecting =
    status.state === "starting" || status.state === "reconnecting";

  return (
    <div
      className={cn(
        "rounded-[var(--radius)] border p-2.5",
        active ? "border-primary/50 bg-primary/5" : "border-border bg-background/40",
      )}
      onContextMenu={onContextMenu}
    >
      <div className="flex items-center gap-2">
        <StateDot state={status.state as TunnelState} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium text-foreground">
          {status.hostName}
        </span>
        <StateBadge state={status.state as TunnelState} />
        {connecting ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />
        ) : running ? (
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={onStop}
            aria-label={t("tunnel.stop")}
            title={t("tunnel.stop")}
          >
            <Square className="h-3 w-3" />
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={onStart}
            aria-label={t("tunnel.start")}
            title={t("tunnel.start")}
          >
            <Play className="h-3 w-3" />
          </Button>
        )}
      </div>

      <p className="mt-1.5 truncate font-mono text-[11px] text-muted-foreground">
        <span className="mr-1.5 inline-flex items-center gap-0.5 text-[10px] uppercase tracking-wide">
          {status.config.type === "dynamic" ? (
            t("tunnel.typeDynamic")
          ) : status.config.type === "remote" ? (
            t("tunnel.typeRemote")
          ) : (
            t("tunnel.typeLocal")
          )}
        </span>
        {tunnelRuleText(status.config)}
      </p>

      {status.state === "reconnecting" && status.retries > 0 && (
        <p className="mt-1 flex items-center gap-1 text-[11px] text-warning">
          <RefreshCw className="h-3 w-3 animate-spin" />
          {t("tunnel.reconnectCount", { count: status.retries })}
        </p>
      )}
      {status.lastError && status.state !== "connected" && (
        <p className="mt-1 break-all text-[11px] text-destructive/80">
          {status.lastError}
        </p>
      )}
    </div>
  );
}

const DOT_CLASSES: Partial<Record<TunnelState, string>> = {
  connected: "bg-success",
  starting: "bg-warning animate-pulse",
  reconnecting: "bg-warning animate-pulse",
  error: "bg-destructive",
  stopped: "bg-muted-foreground/50",
};

function StateDot({ state }: { state: TunnelState }) {
  return (
    <span
      className={cn("h-2 w-2 shrink-0 rounded-full", DOT_CLASSES[state] ?? "bg-muted-foreground")}
      aria-hidden
    />
  );
}

function StateBadge({ state }: { state: TunnelState }) {
  const { t } = useTranslation();
  const key = `tunnel.state.${state}`;
  return (
    <Badge
      variant={
        state === "connected"
          ? "success"
          : state === "error"
            ? "destructive"
            : state === "stopped"
              ? "outline"
              : "secondary"
      }
      className="shrink-0 text-[10px]"
    >
      {t(key)}
    </Badge>
  );
}

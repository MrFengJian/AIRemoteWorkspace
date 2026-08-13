import { useEffect, useState } from "react";
import {
  TerminalSquare,
  X,
  Plus,
  RefreshCw,
  Copy,
  Pencil,
  XCircle,
  ChevronsLeft,
  ChevronsRight,
  CircleX,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { TerminalPanel } from "@/features/terminal/TerminalPanel";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { useOpenTerminal, useHosts } from "@/features/hosts/hooks";
import { useHostsUIStore } from "@/features/hosts/store";
import { osInfo } from "@/features/hosts/osIcons";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { cn } from "@/lib/utils";

/**
 * Terminal view: a tab bar of live sessions above a stack of (hidden) panels.
 * All panels stay mounted so scrollback is preserved; only the active one is
 * visible. An empty state steers users to add a host first.
 *
 * Double-clicking a tab duplicates the session; right-clicking opens a menu
 * with reconnect / duplicate / edit host / close (this, left, right, all).
 */
export function TerminalView() {
  const { t } = useTranslation();
  const sessions = useTerminalStore((s) => s.sessions);
  const activeId = useTerminalStore((s) => s.activeId);
  const setActive = useTerminalStore((s) => s.setActive);
  const removeSession = useTerminalStore((s) => s.removeSession);
  const removeSessions = useTerminalStore((s) => s.removeSessions);
  const clearSessions = useTerminalStore((s) => s.clearSessions);
  const setView = useUIStore((s) => s.setView);
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const openTerminal = useOpenTerminal();

  // Context menu state: the tab it was opened on + cursor position.
  const [menu, setMenu] = useState<{
    sessId: string;
    x: number;
    y: number;
  } | null>(null);

  // Host list (for per-tab OS icon). OS is auto-detected at connect time on
  // the backend; polling the hosts query keeps the tab icon in sync until the
  // detection result lands (usually within a couple of seconds).
  const { data: hosts, refetch: refetchHosts } = useHosts();
  useEffect(() => {
    if (sessions.length === 0) return;
    // Only poll while at least one tab's host has no detected OS yet.
    const unknown = sessions.some((s) => {
      const h = (hosts ?? []).find((x) => x.id === s.hostID);
      return !h?.os;
    });
    if (!unknown) return;
    const timer = setInterval(() => refetchHosts(), 3000);
    return () => clearInterval(timer);
  }, [sessions, hosts, refetchHosts]);

  // hostOs resolves a session's distro id from the hosts query (keyed by hostID).
  const hostOs = (hostID: string): string | undefined =>
    (hosts ?? []).find((h) => h.id === hostID)?.os;

  // closeSession tears down a session end to end: close the backend PTY, then
  // drop it from the store. Called only when the user explicitly closes a tab.
  const closeSession = (id: string) => {
    TerminalService.CloseSession(id).catch(() => {
      /* session may already be gone on the backend */
    });
    removeSession(id);
  };

  // duplicateSession opens a new terminal on the same host as an existing tab.
  // The backend resolves remembered credentials from the OS vault, so no
  // password entry is needed here.
  const duplicateSession = (sess: { hostID: string; hostName: string; terminalTheme: string }) => {
    openTerminal
      .mutateAsync({
        host: { id: sess.hostID, name: sess.hostName, terminalTheme: sess.terminalTheme },
        creds: {},
      })
      .catch(() => {
        /* failed (e.g. no remembered credentials) — nothing to do */
      });
  };

  // reconnectSession closes the current tab and opens a fresh session on the
  // same host (effectively re-establishing the connection).
  const reconnectSession = (sess: (typeof sessions)[number]) => {
    const { hostID, hostName, terminalTheme } = sess;
    closeSession(sess.id);
    duplicateSession({ hostID, hostName, terminalTheme });
  };

  // editHostFor opens the host editor for the tab's host. The dialog renders
  // as a global overlay (HostsView stays mounted in AppShell), so we stay on
  // the terminal page — no navigation happens. Saving keeps the user here.
  const editHostFor = (hostID: string) => {
    const host = (hosts ?? []).find((h) => h.id === hostID);
    if (host) {
      openEditor(host);
    }
  };

  // closeSessions closes a set of sessions end to end (backend + store).
  const closeSessions = (ids: string[]) => {
    for (const id of ids) {
      TerminalService.CloseSession(id).catch(() => {});
    }
    removeSessions(ids);
  };

  const buildMenuItems = (sess: (typeof sessions)[number]): MenuItem[] => {
    const idx = sessions.findIndex((s) => s.id === sess.id);
    const leftIds = sessions.slice(0, idx).map((s) => s.id);
    const rightIds = sessions.slice(idx + 1).map((s) => s.id);
    const hasLeft = leftIds.length > 0;
    const hasRight = rightIds.length > 0;
    const isSingle = sessions.length <= 1;

    return [
      {
        label: t("terminal.reconnect"),
        icon: RefreshCw,
        onClick: () => reconnectSession(sess),
      },
      {
        label: t("terminal.duplicate"),
        icon: Copy,
        onClick: () => duplicateSession(sess),
      },
      {
        label: t("terminal.editHost"),
        icon: Pencil,
        onClick: () => editHostFor(sess.hostID),
      },
      { type: "separator" as const },
      {
        label: t("terminal.close"),
        icon: XCircle,
        danger: true,
        onClick: () => closeSession(sess.id),
      },
      {
        label: t("terminal.closeLeft"),
        icon: ChevronsLeft,
        disabled: !hasLeft,
        onClick: () => closeSessions(leftIds),
      },
      {
        label: t("terminal.closeRight"),
        icon: ChevronsRight,
        disabled: !hasRight,
        onClick: () => closeSessions(rightIds),
      },
      {
        label: t("terminal.closeAll"),
        icon: CircleX,
        danger: true,
        disabled: isSingle,
        onClick: () => {
          const allIds = sessions.map((s) => s.id);
          for (const id of allIds) {
            TerminalService.CloseSession(id).catch(() => {});
          }
          clearSessions();
        },
      },
    ];
  };

  // Auto-select hosts view when there are no sessions, so the empty state has
  // an obvious next action.
  useEffect(() => {
    /* no-op: kept as a seam for future auto-focus behaviour */
  }, [sessions.length]);

  if (sessions.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <TerminalSquare className="h-10 w-10 text-muted-foreground" />
        <div>
          <h2 className="text-lg font-medium text-foreground">{t("terminal.noTerminals")}</h2>
          <p className="mt-1 max-w-sm text-sm text-muted-foreground">
            {t("terminal.noTerminalsDesc")}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setView("hosts")}
          className="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" /> {t("terminal.goToHosts")}
        </button>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Tab bar */}
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border bg-card px-2">
        {sessions.map((sess) => (
          <div
            key={sess.id}
            className={cn(
              "group flex h-7 cursor-pointer items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs",
              sess.id === activeId
                ? "bg-accent text-foreground"
                : "text-muted-foreground hover:bg-accent/50",
            )}
            onClick={() => setActive(sess.id)}
            onDoubleClick={() => duplicateSession(sess)}
            onContextMenu={(e) => {
              e.preventDefault();
              setMenu({ sessId: sess.id, x: e.clientX, y: e.clientY });
            }}
            title={t("terminal.duplicateTab")}
          >
            {/* Distro icon (left of title, once detected). */}
            {osInfo(hostOs(sess.hostID)) && (
              <img
                src={osInfo(hostOs(sess.hostID))!.icon}
                alt=""
                title={osInfo(hostOs(sess.hostID))!.label}
                className="h-3 w-3 shrink-0 invert"
              />
            )}
            <span className="max-w-[12rem] truncate">{sess.hostName}</span>
            {/* Online status dot (right of the title). */}
            <span
              className={cn(
                "h-1.5 w-1.5 shrink-0 rounded-full",
                sess.status === "connected" && "bg-success",
                sess.status === "connecting" && "bg-warning",
                sess.status === "closed" && "bg-muted-foreground",
                sess.status === "error" && "bg-destructive",
              )}
            />
            <button
              type="button"
              aria-label="Close tab"
              onClick={(e) => {
                e.stopPropagation();
                closeSession(sess.id);
              }}
              className="rounded p-0.5 opacity-0 transition-opacity hover:bg-background/60 group-hover:opacity-100"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        ))}
      </div>

      {/* Panels: only active is visible, all stay mounted for scrollback. */}
      <div className="relative min-h-0 flex-1">
        {sessions.map((sess) => (
          <div
            key={sess.id}
            className={cn(
              "absolute inset-0",
              sess.id === activeId ? "visible" : "hidden",
            )}
          >
            <TerminalPanel session={sess} />
          </div>
        ))}
      </div>

      {/* Context menu */}
      {menu && (
        <TerminalTabMenu
          x={menu.x}
          y={menu.y}
          onClose={() => setMenu(null)}
          items={buildMenuItems(sessions.find((s) => s.id === menu.sessId)!)}
        />
      )}
    </div>
  );
}

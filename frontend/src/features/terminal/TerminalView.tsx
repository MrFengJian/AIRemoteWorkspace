import React, { useEffect, useState } from "react";
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
  FolderTree,
  Bot,
  Check,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { TerminalPanel } from "@/features/terminal/TerminalPanel";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { AgentView } from "@/features/agent/AgentView";
import { SftpView } from "@/features/sftp/SftpView";
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
  const addPane = useTerminalStore((s) => s.addPane);
  const removePane = useTerminalStore((s) => s.removePane);
  const setView = useUIStore((s) => s.setView);
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const openTerminal = useOpenTerminal();

  // Context menu state: the tab it was opened on + cursor position.
  const [menu, setMenu] = useState<{
    sessId: string;
    x: number;
    y: number;
  } | null>(null);

  // Right panel visibility + active tab (SFTP / Agent share one panel).
  const [rightOpen, setRightOpen] = useState(false);
  const [rightTab, setRightTab] = useState<"sftp" | "agent">("agent");
  const [copied, setCopied] = useState(false);

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

  // Active session info for the toolbar.
  const activeSession = sessions.find((s) => s.id === activeId);
  const activeHost = activeSession
    ? (hosts ?? []).find((h) => h.id === activeSession.hostID)
    : undefined;

  const handleCopyAddr = async () => {
    if (!activeHost) return;
    const addr = `${activeHost.username}@${activeHost.host}:${activeHost.port}`;
    try {
      await navigator.clipboard.writeText(addr);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* clipboard blocked */
    }
  };

  const handleReconnect = () => {
    if (activeSession) reconnectSession(activeSession);
  };

  // closeSession closes a single pane. If it's the last pane in the tab, the
  // tab is removed too. All panes are equal — no hierarchy.
  const closeSession = (tabId: string, paneId: string) => {
    TerminalService.CloseSession(paneId).catch(() => {});
    removePane(tabId, paneId);
  };

  // closeAllPanes closes every pane in a tab (used by tab right-click "close").
  const closeAllPanes = (tabId: string) => {
    const sess = sessions.find((s) => s.id === tabId);
    if (!sess) return;
    for (const pid of sess.paneIds) {
      TerminalService.CloseSession(pid).catch(() => {});
    }
    removeSession(tabId);
  };

  // handleSplit opens a new backend session on the same host and adds it as a
  // new pane to the tab.
  const handleSplit = async (tabId: string, direction: "horizontal" | "vertical") => {
    const sess = sessions.find((s) => s.id === tabId);
    if (!sess) return;
    try {
      const res = await TerminalService.OpenSession({
        hostId: sess.hostID,
        creds: {},
        size: { cols: 80, rows: 24 },
      });
      addPane(tabId, res.sessionId, direction);
    } catch {
      /* failed to open split — silent */
    }
  };

  // reconnectPane closes and reopens a single pane.
  const reconnectPane = async (tabId: string, paneId: string) => {
    const sess = sessions.find((s) => s.id === tabId);
    if (!sess) return;
    TerminalService.CloseSession(paneId).catch(() => {});
    try {
      const res = await TerminalService.OpenSession({
        hostId: sess.hostID,
        creds: {},
        size: { cols: 80, rows: 24 },
      });
      // Replace paneId with new session ID in the store.
      removePane(tabId, paneId);
      addPane(tabId, res.sessionId, sess.splitDirection ?? "horizontal");
    } catch {
      /* silent */
    }
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

  // reconnectSession closes all panes in the tab and opens a fresh tab.
  const reconnectSession = (sess: (typeof sessions)[number]) => {
    const { hostID, hostName, terminalTheme } = sess;
    closeAllPanes(sess.id);
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
        onClick: () => closeAllPanes(sess.id),
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
                className="os-icon h-3 w-3 shrink-0"
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
                closeAllPanes(sess.id);
              }}
              className="rounded p-0.5 opacity-0 transition-opacity hover:bg-background/60 group-hover:opacity-100"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        ))}
      </div>

      {/* Two-column workspace: [Terminal+Toolbar] [Right Panel] */}
      <div className="flex min-h-0 flex-1">
        {/* Center: toolbar + terminal panels */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* Quick-action toolbar */}
          <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border bg-card px-2">
            {/* SFTP toggle — opens right panel on the SFTP tab */}
            <button
              type="button"
              onClick={() => {
                if (rightOpen && rightTab === "sftp") {
                  setRightOpen(false);
                } else {
                  setRightTab("sftp");
                  setRightOpen(true);
                }
              }}
              title={t("terminal.toggleSftp")}
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-[var(--radius)] transition-colors",
                rightOpen && rightTab === "sftp"
                  ? "bg-accent text-primary"
                  : "text-muted-foreground hover:bg-accent/50",
              )}
            >
              <FolderTree className="h-4 w-4" />
            </button>
            <button
              type="button"
              onClick={handleCopyAddr}
              disabled={!activeHost}
              title={t("terminal.copyAddr")}
              className="flex h-7 w-7 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 disabled:opacity-40"
            >
              {copied ? (
                <Check className="h-4 w-4 text-success" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </button>
            <button
              type="button"
              onClick={handleReconnect}
              disabled={!activeSession}
              title={t("terminal.reconnect")}
              className="flex h-7 w-7 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 disabled:opacity-40"
            >
              <RefreshCw className="h-4 w-4" />
            </button>
            {/* Agent toggle — opens right panel on the Agent tab. Model
                selection lives inline in the agent panel's input area. */}
            <button
              type="button"
              onClick={() => {
                if (rightOpen && rightTab === "agent") {
                  setRightOpen(false);
                } else {
                  setRightTab("agent");
                  setRightOpen(true);
                }
              }}
              title={t("terminal.toggleAgent")}
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-[var(--radius)] transition-colors",
                rightOpen && rightTab === "agent"
                  ? "bg-accent text-primary"
                  : "text-muted-foreground hover:bg-accent/50",
              )}
            >
              <Bot className="h-4 w-4" />
            </button>
          </div>

          {/* Terminal panels: only active tab is visible, all stay mounted. */}
          <div className="relative min-h-0 flex-1">
            {sessions.map((sess) => (
              <div
                key={sess.id}
                className={cn(
                  "absolute inset-0",
                  sess.id === activeId ? "visible" : "hidden",
                )}
              >
                {sess.paneIds.length > 1 ? (
                  /* Split layout: panes side by side or stacked, all equal. */
                  <div
                    className={cn(
                      "flex h-full w-full",
                      sess.splitDirection === "horizontal"
                        ? "flex-row"
                        : "flex-col",
                    )}
                  >
                    {sess.paneIds.map((paneId, idx) => (
                      <React.Fragment key={paneId}>
                        {idx > 0 && (
                          <div
                            className={cn(
                              "shrink-0 bg-foreground/15",
                              sess.splitDirection === "horizontal"
                                ? "w-[3px] h-full cursor-col-resize"
                                : "h-[3px] w-full cursor-row-resize",
                            )}
                          />
                        )}
                        <div className="relative min-h-0 min-w-0 flex-1">
                          <TerminalPanel
                            session={{ ...sess, id: paneId }}
                            paneCount={sess.paneIds.length}
                            onDisconnect={() => closeSession(sess.id, paneId)}
                            onReconnect={() => reconnectPane(sess.id, paneId)}
                          />
                        </div>
                      </React.Fragment>
                    ))}
                  </div>
                ) : (
                  <TerminalPanel
                    session={sess}
                    onDisconnect={() => closeSession(sess.id, sess.paneIds[0])}
                    onReconnect={() => reconnectPane(sess.id, sess.paneIds[0])}
                    onSplitHorizontal={() => handleSplit(sess.id, "horizontal")}
                    onSplitVertical={() => handleSplit(sess.id, "vertical")}
                  />
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Right panel: tabbed (SFTP / Agent) — hidden when closed. */}
        {rightOpen && activeSession && (
          <div className="flex w-80 shrink-0 flex-col border-l border-border bg-card">
            {/* Tab strip */}
            <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
              <button
                type="button"
                onClick={() => setRightTab("sftp")}
                className={cn(
                  "flex h-7 items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
                  rightTab === "sftp"
                    ? "bg-accent text-foreground"
                    : "text-muted-foreground hover:bg-accent/50",
                )}
              >
                <FolderTree className="h-3.5 w-3.5" />
                {t("nav.files")}
              </button>
              <button
                type="button"
                onClick={() => setRightTab("agent")}
                className={cn(
                  "flex h-7 items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
                  rightTab === "agent"
                    ? "bg-accent text-foreground"
                    : "text-muted-foreground hover:bg-accent/50",
                )}
              >
                <Bot className="h-3.5 w-3.5" />
                {t("nav.agent")}
              </button>
              <button
                type="button"
                onClick={() => setRightOpen(false)}
                className="ml-auto rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                title={t("common.close")}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>

            {/* Tab content */}
            <div className="min-h-0 flex-1 overflow-hidden">
              {rightTab === "sftp" ? (
                <SftpView
                  embeddedHostID={activeSession.hostID}
                  embeddedHostName={activeSession.hostName}
                />
              ) : (
                <AgentView
                  embeddedSessionID={activeSession.id}
                  embeddedSessionName={activeSession.hostName}
                />
              )}
            </div>
          </div>
        )}
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

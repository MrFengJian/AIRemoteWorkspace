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
  Monitor,
  PanelLeftOpen,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { useTerminalStore, isLocalSession } from "@/features/terminal/terminal.store";

import { TerminalPanel } from "@/features/terminal/TerminalPanel";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { AgentView } from "@/features/agent/AgentView";
import { useAgentStore } from "@/features/agent/store";
import { agentApi } from "@/features/agent/api";
import { SftpView } from "@/features/sftp/SftpView";
import { useOpenTerminal, useOpenLocalTerminal, useHosts } from "@/features/hosts/hooks";
import { useHostsUIStore } from "@/features/hosts/store";
import { HostsSidebar } from "@/features/hosts/HostsSidebar";
import { osInfo } from "@/features/hosts/osIcons";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { cn } from "@/lib/utils";
import { toast, errorMessage } from "@/lib/toast";

/** Module-level guard for the default local terminal (StrictMode-safe). */
let defaultSessionEnsured = false;
function ensureDefaultLocalSession(open: () => void) {
  if (defaultSessionEnsured) return;
  defaultSessionEnsured = true;
  open();
}

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
  const openEditor = useHostsUIStore((s) => s.openEditor);
  const openTerminal = useOpenTerminal();
  const openLocal = useOpenLocalTerminal();
  const clearAgentHistory = useAgentStore((s) => s.clearHistory);

  // Dropping a tab's session also forgets its agent conversation (local UI +
  // backend memory) — the history is unreachable afterwards either way.
  const cleanupAgentHistory = (ids: string[]) => {
    for (const id of ids) {
      clearAgentHistory(id);
      agentApi.clearHistory(id).catch(() => {});
    }
  };

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
  // Hosts sidebar collapsed state — persisted so the preference survives
  // restarts; collapsing gives the terminal area more room.
  const [sidebarOpen, setSidebarOpen] = useState(
    () => localStorage.getItem("hosts-sidebar-open") !== "false",
  );
  const toggleSidebar = () =>
    setSidebarOpen((v) => {
      localStorage.setItem("hosts-sidebar-open", String(!v));
      return !v;
    });

  // Spinner on the "+" tab-button reflects THIS click only (not the mutation
  // object's state): a stale pending flag from a strict-mode remount would
  // otherwise spin forever with no meaningful work in flight.
  const [openingLocal, setOpeningLocal] = useState(false);
  const handleOpenLocal = async () => {
    if (openingLocal) return;
    setOpeningLocal(true);
    try {
      await openLocal.mutateAsync(t("terminal.localTab"));
    } catch {
      /* failure is toasted globally */
    } finally {
      setOpeningLocal(false);
    }
  };

  // On first launch with no sessions, open a local terminal by default so the
  // workspace always starts in its full form (sidebar + tabs). The guard is a
  // MODULE-level flag: React StrictMode's dev double-mount resets refs (and
  // would open two terminals), but not module state. Closing all tabs
  // afterwards intentionally leaves the empty state — no silent reopen.
  useEffect(() => {
    if (sessions.length === 0) {
      ensureDefaultLocalSession(() => openLocal.mutate(t("terminal.localTab")));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // Per-tab split ratio (first pane's share, 0–1) and the right panel width,
  // both draggable. Kept locally: TerminalView stays mounted for the app run.
  const [splitRatios, setSplitRatios] = useState<Record<string, number>>({});
  const [panelWidth, setPanelWidth] = useState(320);

  /** Drag handler for the split divider; adjusts the first pane's share. */
  const startSplitDrag = (
    e: React.PointerEvent,
    sessId: string,
    direction: "horizontal" | "vertical",
  ) => {
    e.preventDefault();
    const container = (e.currentTarget as HTMLElement).parentElement;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const onMove = (ev: PointerEvent) => {
      const ratio =
        direction === "horizontal"
          ? (ev.clientX - rect.left) / rect.width
          : (ev.clientY - rect.top) / rect.height;
      setSplitRatios((r) => ({ ...r, [sessId]: Math.min(0.8, Math.max(0.2, ratio)) }));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  /** Drag handler for the right panel's left edge (width 240–560px). */
  const startPanelDrag = (e: React.PointerEvent) => {
    e.preventDefault();
    const windowWidth = window.innerWidth;
    const onMove = (ev: PointerEvent) => {
      // The sidebar (56px) + terminal area sit left of the panel; clamp by
      // viewport so the terminal never collapses.
      const width = Math.min(560, Math.max(240, windowWidth - ev.clientX));
      setPanelWidth(width);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  // Host list (for per-tab OS icon). OS is auto-detected at connect time on
  // the backend; polling the hosts query keeps the tab icon in sync until the
  // detection result lands (usually within a couple of seconds). Local
  // sessions have no host and never resolve — exclude them or the poll
  // would run forever.
  const { data: hosts, refetch: refetchHosts } = useHosts();
  useEffect(() => {
    if (sessions.length === 0) return;
    // Only poll while at least one SSH tab's host has no detected OS yet.
    const unknown = sessions.some((s) => {
      if (isLocalSession(s)) return false;
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
      toast.error(t("common.clipboardFailed"));
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
    cleanupAgentHistory([tabId]);
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
    } catch (e) {
      toast.error(`${t("terminal.openSessionFailed")}: ${errorMessage(e)}`);
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
    } catch (e) {
      toast.error(`${t("terminal.openSessionFailed")}: ${errorMessage(e)}`);
    }
  };

  // duplicateSession opens a new terminal on the same host as an existing tab
  // (or a fresh local terminal for local tabs). The backend resolves
  // remembered credentials from the OS vault, so no password entry is needed.
  const duplicateSession = (sess: {
    hostID: string;
    hostName: string;
    terminalTheme: string;
    terminalFont: string;
    terminalFontSize: number;
  }) => {
    if (sess.hostID === "") {
      openLocal.mutate(sess.hostName);
      return;
    }
    openTerminal
      .mutateAsync({
        host: {
          id: sess.hostID,
          name: sess.hostName,
          terminalTheme: sess.terminalTheme,
          terminalFont: sess.terminalFont,
          terminalFontSize: sess.terminalFontSize,
        },
        creds: {},
      })
      .catch(() => {
        /* failure (e.g. no remembered credentials) is toasted globally */
      });
  };

  // reconnectSession closes all panes in the tab and opens a fresh tab.
  const reconnectSession = (sess: (typeof sessions)[number]) => {
    const { hostID, hostName, terminalTheme, terminalFont, terminalFontSize } = sess;
    closeAllPanes(sess.id);
    duplicateSession({ hostID, hostName, terminalTheme, terminalFont, terminalFontSize });
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
    cleanupAgentHistory(ids);
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
        disabled: isLocalSession(sess),
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
          cleanupAgentHistory(allIds);
        },
      },
    ];
  };

  if (sessions.length === 0) {
    return (
      <div className="flex h-full">
        {sidebarOpen && <HostsSidebar onClose={toggleSidebar} />}
        <div className="flex flex-1 flex-col items-center justify-center gap-4 p-8 text-center">
          <TerminalSquare className="h-10 w-10 text-muted-foreground" />
          <div>
            <h2 className="text-base font-medium text-foreground">{t("terminal.noTerminals")}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{t("terminal.emptyHint")}</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={openingLocal}
              onClick={handleOpenLocal}
              className="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              <Monitor className="h-4 w-4" /> {t("terminal.newLocalTab")}
            </button>
            {!sidebarOpen && (
              <button
                type="button"
                onClick={() => setSidebarOpen(true)}
                className="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-border px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent"
              >
                <PanelLeftOpen className="h-4 w-4" /> {t("hosts.expandSidebar")}
              </button>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      {/* Hosts sidebar: the session manager (Xshell-style workspace).
          Collapsible to give the terminal area more room. */}
      {sidebarOpen && <HostsSidebar onClose={toggleSidebar} />}
      <div className="flex min-w-0 flex-1 flex-col">
      {/* Tab bar */}
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border bg-card px-2">
        {/* Re-open the collapsed hosts sidebar. */}
        {!sidebarOpen && (
          <button
            type="button"
            onClick={() => setSidebarOpen(true)}
            aria-label={t("hosts.expandSidebar")}
            title={t("hosts.expandSidebar")}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <PanelLeftOpen className="h-4 w-4" />
          </button>
        )}
      <div
        role="tablist"
        aria-label={t("terminal.title")}
        className="flex min-w-0 flex-1 items-center gap-1"
        onKeyDown={(e) => {
          // Roving-tablist keyboard navigation: arrows move + activate.
          if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
          const idx = sessions.findIndex((s) => s.id === activeId);
          if (idx < 0) return;
          const next =
            e.key === "ArrowRight"
              ? (idx + 1) % sessions.length
              : (idx - 1 + sessions.length) % sessions.length;
          const target = sessions[next];
          setActive(target.id);
          document.getElementById(`term-tab-${target.id}`)?.focus();
          e.preventDefault();
        }}
      >
        {sessions.map((sess) => (
          <div
            key={sess.id}
            id={`term-tab-${sess.id}`}
            role="tab"
            aria-selected={sess.id === activeId}
            tabIndex={sess.id === activeId ? 0 : -1}
            className={cn(
              "group flex h-7 cursor-pointer items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
              sess.id === activeId
                ? "bg-accent text-foreground"
                : "text-muted-foreground hover:bg-accent/50",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            )}
            onClick={() => setActive(sess.id)}
            onDoubleClick={() => duplicateSession(sess)}
            onContextMenu={(e) => {
              e.preventDefault();
              setMenu({ sessId: sess.id, x: e.clientX, y: e.clientY });
            }}
            title={t("terminal.duplicateTab")}
          >
            {/* Tab icon: local sessions get a monitor; SSH tabs show the
                distro icon once detected. */}
            {isLocalSession(sess) ? (
              <Monitor className="h-3 w-3 shrink-0 text-primary" aria-hidden />
            ) : osInfo(hostOs(sess.hostID)) ? (
              <img
                src={osInfo(hostOs(sess.hostID))!.icon}
                alt=""
                title={osInfo(hostOs(sess.hostID))!.label}
                className="os-icon h-3 w-3 shrink-0"
              />
            ) : null}
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
              aria-label={t("terminal.closeTab")}
              onClick={(e) => {
                e.stopPropagation();
                closeAllPanes(sess.id);
              }}
              className="rounded p-0.5 opacity-0 transition-opacity hover:bg-background/60 focus-visible:opacity-100 group-hover:opacity-100"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        ))}

        {/* New local terminal tab (browser-style + button). */}
        <button
          type="button"
          onClick={handleOpenLocal}
          disabled={openingLocal}
          aria-label={t("terminal.newLocalTab")}
          title={t("terminal.newLocalTab")}
          className="ml-auto flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        >
          {openingLocal ? (
            <RefreshCw className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Plus className="h-3.5 w-3.5" />
          )}
        </button>
      </div>
      </div>

      {/* Two-column workspace: [Terminal+Toolbar] [Right Panel] */}
      <div className="flex min-h-0 flex-1">
        {/* Center: toolbar + terminal panels */}
        <div className="flex min-w-0 flex-1 flex-col">
          {/* Quick-action toolbar */}
          <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border bg-card px-2">
            {/* SFTP toggle — opens right panel on the SFTP tab. Local
                sessions have no host to browse. */}
            <button
              type="button"
              disabled={activeSession ? isLocalSession(activeSession) : false}
              onClick={() => {
                if (rightOpen && rightTab === "sftp") {
                  setRightOpen(false);
                } else {
                  setRightTab("sftp");
                  setRightOpen(true);
                }
              }}
              title={activeSession && isLocalSession(activeSession) ? t("terminal.sftpNeedsHost") : t("terminal.toggleSftp")}
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-[var(--radius)] transition-colors",
                "disabled:pointer-events-none disabled:opacity-40",
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
                            role="separator"
                            aria-orientation={sess.splitDirection === "horizontal" ? "vertical" : "horizontal"}
                            onPointerDown={(e) =>
                              startSplitDrag(e, sess.id, sess.splitDirection ?? "horizontal")
                            }
                            className={cn(
                              "shrink-0 bg-foreground/15 transition-colors hover:bg-primary/50",
                              sess.splitDirection === "horizontal"
                                ? "w-[3px] h-full cursor-col-resize"
                                : "h-[3px] w-full cursor-row-resize",
                            )}
                          />
                        )}
                        <div
                          className="relative min-h-0 min-w-0 flex-1"
                          style={
                            idx === 0 && sess.paneIds.length > 1
                              ? {
                                  flexGrow: 0,
                                  flexShrink: 0,
                                  flexBasis: `${(splitRatios[sess.id] ?? 0.5) * 100}%`,
                                }
                              : undefined
                          }
                        >
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

        {/* Right panel: tabbed (SFTP / Agent) — hidden when closed. Its left
            edge is a drag handle (width clamped 240–560px). */}
        {rightOpen && activeSession && (
          <div
            className="relative flex shrink-0 flex-col border-l border-border bg-card"
            style={{ width: panelWidth }}
          >
            {/* Drag handle on the panel's left edge */}
            <div
              role="separator"
              aria-orientation="vertical"
              onPointerDown={startPanelDrag}
              className="absolute inset-y-0 left-0 z-10 w-1 cursor-col-resize transition-colors hover:bg-primary/50"
            />
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
    </div>
  );
}

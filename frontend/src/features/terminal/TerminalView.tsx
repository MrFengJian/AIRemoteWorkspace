import { useEffect } from "react";
import { TerminalSquare, X, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";

import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { TerminalPanel } from "@/features/terminal/TerminalPanel";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { cn } from "@/lib/utils";

/**
 * Terminal view: a tab bar of live sessions above a stack of (hidden) panels.
 * All panels stay mounted so scrollback is preserved; only the active one is
 * visible. An empty state steers users to add a host first.
 */
export function TerminalView() {
  const { t } = useTranslation();
  const sessions = useTerminalStore((s) => s.sessions);
  const activeId = useTerminalStore((s) => s.activeId);
  const setActive = useTerminalStore((s) => s.setActive);
  const removeSession = useTerminalStore((s) => s.removeSession);
  const setView = useUIStore((s) => s.setView);

  // closeSession tears down a session end to end: close the backend PTY, then
  // drop it from the store. Called only when the user explicitly closes a tab.
  const closeSession = (id: string) => {
    TerminalService.CloseSession(id).catch(() => {
      /* session may already be gone on the backend */
    });
    removeSession(id);
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
          >
            <span
              className={cn(
                "h-1.5 w-1.5 rounded-full",
                sess.status === "connected" && "bg-success",
                sess.status === "connecting" && "bg-warning",
                sess.status === "closed" && "bg-muted-foreground",
                sess.status === "error" && "bg-destructive",
              )}
            />
            <span className="max-w-[12rem] truncate">{sess.hostName}</span>
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
    </div>
  );
}

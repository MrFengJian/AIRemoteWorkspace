import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { Events } from "@wailsio/runtime";
import { XCircle } from "lucide-react";

import { useTranslation } from "react-i18next";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useTerminalStore, type TerminalSession } from "@/features/terminal/terminal.store";
import { getTerminalTheme } from "@/features/terminal/themes";

import "@xterm/xterm/css/xterm.css";

interface TerminalPanelProps {
  session: TerminalSession;
}

/**
 * TerminalPanel mounts a single xterm.js instance bound to one live PTY
 * session. Lifecycle:
 *   - On mount: create Terminal + FitAddon, subscribe to term:<id>:out / :exit
 *   - Input:    xterm onData → WriteStdin (bound call)
 *   - Output:   term:<id>:out event → terminal.write
 *   - Resize:   container ResizeObserver → FitAddon → ResizeSession
 *   - On exit:  mark session closed; terminal becomes read-only
 *   - On unmount: CloseSession + dispose terminal
 *
 * Hidden when not the active session (kept mounted to preserve scrollback;
 * toggled via CSS by the parent so we don't pay dispose/recreate cost).
 */
export function TerminalPanel({ session }: TerminalPanelProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const setSessionStatus = useTerminalStore((s) => s.setSessionStatus);
  const removeSession = useTerminalStore((s) => s.removeSession);
  const themeId = useTerminalStore((s) => s.themeId);

  // The session's theme takes precedence; fall back to the global setting.
  const resolvedTheme = getTerminalTheme(session.terminalTheme || themeId);

  // Live-update the xterm theme when the scheme changes (per-host or global).
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme = getTerminalTheme(session.terminalTheme || themeId).theme;
    }
  }, [themeId, session.terminalTheme]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const themeDef = resolvedTheme;
    const term = new Terminal({
      fontFamily:
        'ui-monospace, "JetBrains Mono", "Cascadia Code", "Fira Code", Menlo, Consolas, monospace',
      fontSize: 13,
      cursorBlink: true,
      theme: themeDef.theme,
      allowProposedApi: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    try {
      fit.fit();
    } catch {
      /* container not laid out yet; resize observer will catch up */
    }
    termRef.current = term;
    setSessionStatus(session.id, "connected");

    // Report initial size once connected so the server PTY matches.
    const applySize = () => {
      try {
        fit.fit();
        TerminalService.ResizeSession(session.id, {
          cols: term.cols,
          rows: term.rows,
        });
      } catch {
        /* session may have just closed */
      }
    };
    applySize();

    // Keystrokes → backend stdin. Wails serialises []byte as a base64 string,
    // so encode before sending (handles control chars like Ctrl+C correctly).
    const onDataDisp = term.onData((data) => {
      const encoded = btoa(data);
      TerminalService.WriteStdin(session.id, encoded).catch(() => {
        /* swallow; a closed session surfaces via :exit */
      });
    });

    // Remote output → terminal. Backend sends base64-encoded bytes for
    // binary safety; decode back to a string for xterm.
    const outEventName = `term:${session.id}:out`;
    const outCancel = Events.On(outEventName, (event: unknown) => {
      const data = (event as { data?: unknown }).data;
      if (typeof data === "string" && data.length > 0) {
        try {
          const decoded = atob(data);
          term.write(decoded);
        } catch {
          // Not valid base64 (shouldn't happen) — write raw as fallback.
          console.warn("[TerminalPanel] base64 decode failed, writing raw");
          term.write(data);
        }
      }
    });

    // Session exit.
    const exitCancel = Events.On(`term:${session.id}:exit`, (event: unknown) => {
      const data = (event as { data?: unknown }).data;
      const msg = typeof data === "string" && data ? `\r\n\r\n[session exited: ${data}]\r\n` : "\r\n\r\n[session exited]\r\n";
      term.write(msg);
      setSessionStatus(session.id, "closed");
    });

    // Resize handling.
    const ro = new ResizeObserver(() => {
      if (container.offsetParent !== null) {
        applySize();
      }
    });
    ro.observe(container);

    return () => {
      // Clean up xterm + event subscriptions only. Do NOT close the backend
      // session here — React StrictMode mounts/cleans/mounts in dev, and any
      // real re-mount would prematurely kill a live PTY. Backend session
      // teardown happens when the user closes the tab (removeSession).
      onDataDisp.dispose();
      if (typeof outCancel === "function") outCancel();
      if (typeof exitCancel === "function") exitCancel();
      ro.disconnect();
      term.dispose();
      termRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id]);

  const themeDef = resolvedTheme;
  return (
    <div className="relative h-full w-full" style={{ background: themeDef.theme.background }}>
      <div ref={containerRef} className="h-full w-full p-2" />
      {session.status === "closed" && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/40">
          <button
            type="button"
            onClick={() => removeSession(session.id)}
            className="pointer-events-auto inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <XCircle className="h-3.5 w-3.5" /> {t("terminal.sessionClosed")}
          </button>
        </div>
      )}
    </div>
  );
}

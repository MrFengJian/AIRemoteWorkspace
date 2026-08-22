import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Events } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import {
  XCircle,
  PlugZap,
  Copy as CopyIcon,
  ClipboardPaste,
  FileText,
  ClipboardList,
  TextSelect,
  ScanLine,
  Search,
  Network,
  Globe,
  Eraser,
  X,
  ChevronUp,
  ChevronDown,
  Columns2,
  Rows2,
  Palette,
} from "lucide-react";

import { TerminalService, SystemService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useTerminalStore, type TerminalSession } from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { terminalFontFamily } from "@/features/terminal/fonts";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { TerminalThemeDialog } from "@/features/terminal/TerminalThemeDialog";
import { useHosts } from "@/features/hosts/hooks";
import { getTerminalTheme } from "@/features/terminal/themes";
import { base64ToBytes, encodeBase64 } from "@/lib/base64";
import { toast } from "@/lib/toast";

import "@xterm/xterm/css/xterm.css";

interface TerminalPanelProps {
  session: TerminalSession;
  onDisconnect?: () => void;
  onReconnect?: () => void;
  onSplitHorizontal?: () => void;
  onSplitVertical?: () => void;
  /** Total panes in this tab (1 = standalone, >1 = split). Controls menu labels. */
  paneCount?: number;
}

/**
 * TerminalPanel mounts a single xterm.js instance bound to one live PTY
 * session. Features a custom right-click context menu with copy/paste/search/
 * IP/clear actions, and a search bar overlay for finding text in the buffer.
 */
export function TerminalPanel({
  session,
  onDisconnect,
  onReconnect,
  onSplitHorizontal,
  onSplitVertical,
  paneCount = 1,
}: TerminalPanelProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const setSessionStatus = useTerminalStore((s) => s.setSessionStatus);
  const removeSession = useTerminalStore((s) => s.removeSession);
  const activeView = useUIStore((s) => s.activeView);
  const activeTabId = useTerminalStore((s) => s.activeId);
  const { data: hosts } = useHosts();

  // Context menu + search bar state.
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [hasSelection, setHasSelection] = useState(false);
  const [showSearch, setShowSearch] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [showThemeDialog, setShowThemeDialog] = useState(false);
  // Theme picked from the context menu for *this pane only* (session lifetime).
  const [liveThemeId, setLiveThemeId] = useState<string | null>(null);

  // The host's theme; "" falls back to the default scheme inside
  // getTerminalTheme. The context-menu pick overrides it for this pane.
  const resolvedTheme = getTerminalTheme(liveThemeId ?? session.terminalTheme);

  // Resolved font: per-host value (host edit → 外观 tab) or built-in default.
  // Empty font / 0 size fall back to the defaults inside terminalFontFamily.
  const resolvedFontFamily = terminalFontFamily(session.terminalFont || "");
  const resolvedFontSize = session.terminalFontSize || 13;

  // Resolve the remote host IP for the "paste remote IP" action.
  const remoteIP = (hosts ?? []).find((h) => h.id === session.hostID)?.host ?? "";

  // Live-update the xterm theme when the scheme changes.
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme = getTerminalTheme(liveThemeId ?? session.terminalTheme).theme;
    }
  }, [session.terminalTheme, liveThemeId]);

  // Auto-focus: when the terminal view is showing and this pane belongs to the
  // active tab, grab focus so typing works immediately (no manual click).
  // paneIds[0] is the tab's own id; in a split the first pane takes focus.
  const tabId = session.paneIds[0] ?? session.id;
  useEffect(() => {
    if (activeView === "terminal" && activeTabId === tabId) {
      termRef.current?.focus();
    }
  }, [activeView, activeTabId, tabId]);

  // Note: the resolved font/theme are snapshotted at session start (the
  // session's TerminalPanel mounts once). Editing the host's appearance
  // applies to newly opened sessions — reconnect to refresh an open one.

  // Mount xterm + wire events (runs once per session).
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const themeDef = resolvedTheme;
    const term = new Terminal({
      fontFamily: resolvedFontFamily,
      fontSize: resolvedFontSize,
      cursorBlink: true,
      theme: themeDef.theme,
      allowProposedApi: true,
      rightClickSelectsWord: false, // we handle right-click ourselves
    });
    const fit = new FitAddon();
    const search = new SearchAddon();
    term.loadAddon(fit);
    term.loadAddon(search);
    term.open(container);
    try {
      fit.fit();
    } catch {
      /* container not laid out yet */
    }
    termRef.current = term;
    searchAddonRef.current = search;
    setSessionStatus(session.id, "connected");

    // Track selection state for context-sensitive menu items.
    const selDisp = term.onSelectionChange(() => {
      setHasSelection(term.hasSelection());
    });

    // Right-click: show custom context menu (prevent the browser default).
    const onContext = (e: MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setHasSelection(term.hasSelection());
      setMenu({ x: e.clientX, y: e.clientY });
    };
    container.addEventListener("contextmenu", onContext);

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

    const onDataDisp = term.onData((data) => {
      TerminalService.WriteStdin(session.id, encodeBase64(data)).catch(() => {});
    });

    const outEventName = `term:${session.id}:out`;
    const outCancel = Events.On(outEventName, (event: unknown) => {
      const data = (event as { data?: unknown }).data;
      if (typeof data === "string" && data.length > 0) {
        try {
          // Write raw bytes: xterm's built-in UTF-8 parser handles multi-byte
          // characters even when a read splits them across events.
          term.write(base64ToBytes(data));
        } catch {
          term.write(data);
        }
      }
    });

    const exitCancel = Events.On(`term:${session.id}:exit`, (event: unknown) => {
      const data = (event as { data?: unknown }).data;
      const reason = typeof data === "string" ? data : "";
      // A non-empty reason means the PTY wait returned an error (connection
      // dropped, shell failed) — mark the tab red instead of plain closed.
      if (reason) {
        term.write(`\r\n\r\n[session exited: ${reason}]\r\n`);
        setSessionStatus(session.id, "error");
      } else {
        term.write(`\r\n\r\n${t("terminal.sessionExited")}\r\n`);
        setSessionStatus(session.id, "closed");
      }
    });

    const ro = new ResizeObserver(() => {
      if (container.offsetParent !== null) applySize();
    });
    ro.observe(container);

    return () => {
      onDataDisp.dispose();
      selDisp.dispose();
      container.removeEventListener("contextmenu", onContext);
      if (typeof outCancel === "function") outCancel();
      if (typeof exitCancel === "function") exitCancel();
      ro.disconnect();
      search.dispose();
      term.dispose();
      termRef.current = null;
      searchAddonRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id]);

  // ── Context menu actions ──────────────────────────────────────────

  const writeToStdin = (text: string) => {
    TerminalService.WriteStdin(session.id, encodeBase64(text)).catch(() => {});
  };

  const handleCopy = async () => {
    const sel = termRef.current?.getSelection();
    if (!sel) return;
    try {
      await navigator.clipboard.writeText(sel);
    } catch {
      toast.error(t("common.clipboardFailed"));
    }
  };

  const handleCopyRTF = async () => {
    const sel = termRef.current?.getSelection();
    if (!sel) return;
    // Minimal RTF with the terminal's monospace font + foreground colour.
    const rtf = `{\\rtf1\\ansi\\f0\\fs24 ${sel.replace(/\\/g, "\\\\").replace(/[{}]/g, (m) => `\\${m}`).replace(/\n/g, "\\par\n")}}`;
    try {
      await navigator.clipboard.writeText(rtf);
    } catch {
      toast.error(t("common.clipboardFailed"));
    }
  };

  const handlePasteSelected = () => {
    const sel = termRef.current?.getSelection();
    if (sel) writeToStdin(sel);
  };

  const handlePaste = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (text) writeToStdin(text);
    } catch {
      toast.error(t("common.clipboardFailed"));
    }
  };

  const handleSelectAll = () => termRef.current?.selectAll();

  const handleSelectScreen = () => {
    const term = termRef.current;
    if (!term) return;
    // Select from the top-left of the viewport to the bottom-right.
    term.select(0, term.buffer.active.viewportY, term.cols * (term.rows));
  };

  const handlePasteLocalIP = async () => {
    try {
      const res = await SystemService.GetLocalIP();
      if (res.ip) writeToStdin(res.ip);
    } catch {
      /* network error */
    }
  };

  const handlePasteRemoteIP = () => {
    if (remoteIP) writeToStdin(remoteIP);
  };

  const handleClear = () => termRef.current?.clear();

  const doSearch = (dir: "next" | "prev") => {
    if (!searchQuery || !searchAddonRef.current) return;
    if (dir === "next") searchAddonRef.current.findNext(searchQuery);
    else searchAddonRef.current.findPrevious(searchQuery);
  };

  // Build context menu items based on current state.
  const buildMenuItems = (): MenuItem[] => {
    const inSplit = paneCount > 1;
    const items: MenuItem[] = [
      {
        label: t("termMenu.reconnect"),
        icon: PlugZap,
        onClick: () => onReconnect?.(),
      },
      {
        label: inSplit ? t("termMenu.closePane") : t("termMenu.disconnect"),
        icon: XCircle,
        danger: true,
        onClick: () => onDisconnect?.(),
      },
    ];

    // Split actions only available when not already split (max 2 panes per tab).
    if (!inSplit) {
      items.push(
        { type: "separator" },
        {
          label: t("termMenu.splitH"),
          icon: Columns2,
          onClick: () => onSplitHorizontal?.(),
        },
        {
          label: t("termMenu.splitV"),
          icon: Rows2,
          onClick: () => onSplitVertical?.(),
        },
      );
    }

    if (hasSelection) {
      items.push({ type: "separator" });
      items.push(
        { label: t("termMenu.copy"), icon: CopyIcon, onClick: handleCopy },
        { label: t("termMenu.copyRtf"), icon: FileText, onClick: handleCopyRTF },
        { label: t("termMenu.pasteSelected"), icon: ClipboardList, onClick: handlePasteSelected },
        { label: t("termMenu.paste"), icon: ClipboardPaste, onClick: handlePaste },
      );
    }

    items.push(
      { type: "separator" },
      { label: t("termMenu.selectAll"), icon: TextSelect, onClick: handleSelectAll },
      { label: t("termMenu.selectScreen"), icon: ScanLine, onClick: handleSelectScreen },
      { type: "separator" },
      { label: t("termMenu.find"), icon: Search, onClick: () => setShowSearch(true) },
      {
        label: t("termMenu.pasteLocalIP"),
        icon: Network,
        onClick: handlePasteLocalIP,
      },
      {
        label: t("termMenu.pasteRemoteIP"),
        icon: Globe,
        disabled: !remoteIP,
        onClick: handlePasteRemoteIP,
      },
      { type: "separator" },
      { label: t("termMenu.clear"), icon: Eraser, onClick: handleClear },
      { type: "separator" },
      {
        label: t("termMenu.terminalTheme"),
        icon: Palette,
        onClick: () => setShowThemeDialog(true),
      },
    );

    return items;
  };

  return (
    <div className="relative h-full w-full" style={{ background: resolvedTheme.theme.background }}>
      {/* Search bar overlay */}
      {showSearch && (
        <div className="absolute right-2 top-2 z-20 flex items-center gap-1 rounded-[var(--radius)] border border-border bg-popover p-1.5 shadow-md">
          <input
            autoFocus
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") doSearch("next");
              if (e.key === "Escape") setShowSearch(false);
            }}
            placeholder={t("termMenu.findPlaceholder")}
            className="h-7 w-44 rounded-[calc(var(--radius)-2px)] border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <button
            type="button"
            onClick={() => doSearch("prev")}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            title={t("termMenu.findPrev")}
          >
            <ChevronUp className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => doSearch("next")}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            title={t("termMenu.findNext")}
          >
            <ChevronDown className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => setShowSearch(false)}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      <div ref={containerRef} className="h-full w-full p-2" />

      {/* Session-closed dismiss button */}
      {(session.status === "closed" || session.status === "error") && (
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

      {/* Custom context menu */}
      {menu && (
        <TerminalTabMenu
          x={menu.x}
          y={menu.y}
          onClose={() => setMenu(null)}
          items={buildMenuItems()}
        />
      )}

      {/* Colour scheme picker — preview inside the dialog, apply on confirm */}
      <TerminalThemeDialog
        open={showThemeDialog}
        currentThemeId={liveThemeId ?? session.terminalTheme}
        onConfirm={(id) => {
          setLiveThemeId(id);
          if (termRef.current) {
            termRef.current.options.theme = getTerminalTheme(id).theme;
          }
          setShowThemeDialog(false);
        }}
        onClose={() => setShowThemeDialog(false)}
      />
    </div>
  );
}

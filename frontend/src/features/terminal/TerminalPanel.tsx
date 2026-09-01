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
  ImageUp,
} from "lucide-react";

import { TerminalService, SystemService, SftpService, ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import {
  useTerminalStore,
  isLocalSession,
  type TerminalSession,
} from "@/features/terminal/terminal.store";
import { useUIStore } from "@/stores/ui.store";
import { terminalFontFamily } from "@/features/terminal/fonts";
import { TerminalTabMenu, type MenuItem } from "@/features/terminal/TerminalTabMenu";
import { TerminalAppearanceDialog } from "@/features/terminal/TerminalAppearanceDialog";
import { useHosts, useUpdateHost } from "@/features/hosts/hooks";
import { getTerminalTheme } from "@/features/terminal/themes";
import { useKeybindingStore } from "@/keybindings/store";
import { registerPaneActions } from "@/keybindings/registry";
import { base64ToBytes, encodeBase64, bytesToBase64 } from "@/lib/base64";
import { toast, errorMessage } from "@/lib/toast";

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
  const fitRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const setSessionStatus = useTerminalStore((s) => s.setSessionStatus);
  const setSessionAppearance = useTerminalStore((s) => s.setSessionAppearance);
  const removeSession = useTerminalStore((s) => s.removeSession);
  const setActivePane = useTerminalStore((s) => s.setActivePane);
  const activeView = useUIStore((s) => s.activeView);
  const activeTabId = useTerminalStore((s) => s.activeId);
  const bindings = useKeybindingStore((s) => s.resolved);
  const { data: hosts } = useHosts();
  const updateHost = useUpdateHost();

  // Context menu + search bar state.
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [hasSelection, setHasSelection] = useState(false);
  const [showSearch, setShowSearch] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [showAppearanceDialog, setShowAppearanceDialog] = useState(false);
  // Live font zoom for this pane (Ctrl+= / Ctrl+- shortcuts). Applied on top
  // of the session's base size; the appearance dialog writes absolute sizes
  // into the session snapshot and resets this to 0, so the two stay in sync.
  const [zoomDelta, setZoomDelta] = useState(0);

  // The host's theme ("" falls back to the default scheme inside
  // getTerminalTheme); resolved at open time into the session snapshot.
  const resolvedTheme = getTerminalTheme(session.terminalTheme);

  // Resolved font: the session snapshot's value (host override or global
  // default, resolved when the session opened). Empty font / 0 size fall
  // back to the defaults inside terminalFontFamily.
  const resolvedFontFamily = terminalFontFamily(session.terminalFont || "");
  const resolvedFontSize = session.terminalFontSize || 13;

  // Resolve the remote host IP for the "paste remote IP" action.
  const remoteIP = (hosts ?? []).find((h) => h.id === session.hostID)?.host ?? "";

  // Live-update the xterm theme when the scheme changes (appearance dialog
  // writes the session snapshot; the effect re-applies from it).
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme = getTerminalTheme(session.terminalTheme).theme;
    }
  }, [session.terminalTheme]);

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
    fitRef.current = fit;
    searchAddonRef.current = search;
    setSessionStatus(session.id, "connected");

    // Track selection state for context-sensitive menu items.
    const selDisp = term.onSelectionChange(() => {
      setHasSelection(term.hasSelection());
    });

    // Report focus for shortcut routing: copy/paste/zoom act on the pane
    // that last had keyboard focus (falls back to the tab's first pane).
    const onTermFocus = () => setActivePane(session.id);
    term.textarea?.addEventListener("focus", onTermFocus);

    // Right-click: show custom context menu (prevent the browser default).
    const onContext = (e: MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setHasSelection(term.hasSelection());
      setMenu({ x: e.clientX, y: e.clientY });
    };
    container.addEventListener("contextmenu", onContext);

    // Middle-click (Xshell-style). The action is read at event time from the
    // keybinding store, so settings changes apply without remounting panes.
    // preventDefault always runs — the WebView would otherwise start its
    // autoscroll mode on middle mousedown, whatever the configured action.
    const onMiddleDown = (e: MouseEvent) => {
      if (e.button !== 1) return;
      e.preventDefault();
      switch (useKeybindingStore.getState().mouseMiddleClick) {
        case "none":
          return;
        case "pasteClipboard":
          void handlePaste();
          return;
        case "pasteSelection":
          handlePasteSelected();
          return;
        case "sendEnter":
          writeToStdin("\r");
          return;
        case "contextMenu":
          setHasSelection(term.hasSelection());
          setMenu({ x: e.clientX, y: e.clientY });
          return;
      }
    };
    container.addEventListener("mousedown", onMiddleDown);

    // Debounced + change-gated resize. The scrollbar oscillation this used
    // to fight (scrollbar toggling changes usable width → fit → PTY resize →
    // repaint → scrollbar toggles again) is now structurally impossible: the
    // xterm viewport scrollbar is hidden via CSS and takes no layout space.
    // Debounce absorbs ResizeObserver jitter; the change gate skips no-op
    // PTY resizes (ConPTY repaints are expensive).
    let lastCols = 0;
    let lastRows = 0;
    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    const applySize = (immediate = false) => {
      const run = () => {
        try {
          fit.fit();
          if (term.cols !== lastCols || term.rows !== lastRows) {
            lastCols = term.cols;
            lastRows = term.rows;
            TerminalService.ResizeSession(session.id, {
              cols: term.cols,
              rows: term.rows,
            }).catch(() => {
              /* session may have just closed */
            });
          }
        } catch {
          /* container not laid out yet */
        }
      };
      if (immediate) {
        run();
        return;
      }
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(run, 50);
    };
    applySize(true);
    // Webfont metrics settle after load; re-fit once so the initial size is
    // measured with the real character cell.
    if (typeof document !== "undefined" && document.fonts?.ready) {
      document.fonts.ready.then(() => applySize(true)).catch(() => {});
    }

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
      clearTimeout(resizeTimer);
      onDataDisp.dispose();
      selDisp.dispose();
      term.textarea?.removeEventListener("focus", onTermFocus);
      container.removeEventListener("contextmenu", onContext);
      container.removeEventListener("mousedown", onMiddleDown);
      if (typeof outCancel === "function") outCancel();
      if (typeof exitCancel === "function") exitCancel();
      ro.disconnect();
      search.dispose();
      term.dispose();
      termRef.current = null;
      fitRef.current = null;
      searchAddonRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id]);

  // Live font application (appearance dialog + Ctrl+= / Ctrl+- shortcuts):
  // update the xterm options, re-fit the grid and tell the backend so the
  // PTY matches the new cols/rows. Skips no-ops so it never fights the
  // mount-time constructor values.
  const zoomedFontSize = Math.min(40, Math.max(6, resolvedFontSize + zoomDelta));
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    let changed = false;
    if (term.options.fontFamily !== resolvedFontFamily) {
      term.options.fontFamily = resolvedFontFamily;
      changed = true;
    }
    if (term.options.fontSize !== zoomedFontSize) {
      term.options.fontSize = zoomedFontSize;
      changed = true;
    }
    if (!changed) return;
    try {
      fitRef.current?.fit();
    } catch {
      /* container not laid out yet */
    }
    TerminalService.ResizeSession(session.id, {
      cols: term.cols,
      rows: term.rows,
    }).catch(() => {
      /* session may have just closed */
    });
  }, [resolvedFontFamily, zoomedFontSize, session.id]);

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

  /** Upload the clipboard image to /tmp (remote host via SFTP; local temp dir
   *  for local sessions) and insert the resulting path at the cursor. */
  const handleUploadClipboardImage = async () => {
    try {
      const items = await navigator.clipboard.read();
      const item = items.find((i) => i.types.some((ty) => ty.startsWith("image/")));
      if (!item) {
        toast.info(t("termMenu.noClipboardImage"));
        return;
      }
      const type = item.types.find((ty) => ty.startsWith("image/"))!;
      const blob = await item.getType(type);
      const buf = new Uint8Array(await blob.arrayBuffer());
      const ext = (type.split("/")[1] || "png").replace(/[^a-z0-9]/gi, "") || "png";
      const name = `clipboard-${Date.now()}.${ext}`;
      const path = await SftpService.UploadClipboardImage(
        session.hostID,
        name,
        bytesToBase64(buf),
      );
      // Insert the path at the cursor (no newline — nothing runs until the
      // user presses Enter) and surface where the file landed.
      writeToStdin(path);
      toast.success(t("termMenu.imageUploaded", { path }));
    } catch (e) {
      toast.error(`${t("termMenu.uploadImage")}: ${e instanceof Error ? e.message : String(e)}`);
    }
  };

  const handleClear = () => termRef.current?.clear();

  /**
   * Persist an appearance change made in this pane so the NEXT session on the
   * same target opens with it: host sessions write the host record (empty
   * fields elsewhere on the host are passed back unchanged); local tabs have
   * no host, so they update the global terminal defaults in AppConfig. The
   * live pane already switched — a persistence failure only toasts.
   */
  const persistAppearance = (themeId: string, font: string, fontSize: number) => {
    if (isLocalSession(session)) {
      ConfigService.GetAppConfig()
        .then((cfg) =>
          ConfigService.SetAppConfig({
            ...cfg,
            terminalTheme: themeId,
            terminalFont: font,
            terminalFontSize: fontSize,
          }),
        )
        .catch((e) => {
          toast.error(`${t("termAppearanceDialog.saveFailed")}: ${errorMessage(e)}`);
        });
      return;
    }
    const host = (hosts ?? []).find((h) => h.id === session.hostID);
    if (!host) return;
    updateHost.mutate(
      {
        id: host.id,
        input: {
          name: host.name,
          host: host.host,
          port: host.port,
          username: host.username,
          authType: host.authType,
          keyPath: host.keyPath ?? "",
          terminalTheme: themeId,
          terminalFont: font,
          terminalFontSize: fontSize,
          group: host.group ?? "",
          tags: host.tags ?? [],
          tunnels: host.tunnels ?? [],
        },
      },
      {
        onError: (e) => {
          toast.error(`${t("termAppearanceDialog.saveFailed")}: ${errorMessage(e)}`);
        },
      },
    );
  };

  const doSearch = (dir: "next" | "prev") => {
    if (!searchQuery || !searchAddonRef.current) return;
    if (dir === "next") searchAddonRef.current.findNext(searchQuery);
    else searchAddonRef.current.findPrevious(searchQuery);
  };

  // Expose this pane's actions to the shortcut system (TerminalView routes
  // terminal.* commands to the focused pane's registration). Re-registered
  // every render so the closures stay fresh.
  useEffect(() => {
    return registerPaneActions(session.id, {
      copy: () => void handleCopy(),
      paste: () => void handlePaste(),
      selectAll: handleSelectAll,
      find: () => setShowSearch(true),
      clear: handleClear,
      zoomIn: () => setZoomDelta((d) => Math.min(20, d + 1)),
      zoomOut: () => setZoomDelta((d) => Math.max(-10, d - 1)),
      zoomReset: () => setZoomDelta(0),
      focus: () => termRef.current?.focus(),
    });
  });

  // Build context menu items based on current state.
  const buildMenuItems = (): MenuItem[] => {
    const inSplit = paneCount > 1;
    // Right-aligned shortcut hints (Xshell-style) for bound commands.
    const hint = (id: string) => bindings[id] ?? undefined;
    const items: MenuItem[] = [
      {
        label: t("termMenu.reconnect"),
        icon: PlugZap,
        shortcut: hint("terminal.reconnect"),
        onClick: () => onReconnect?.(),
      },
      {
        label: inSplit ? t("termMenu.closePane") : t("termMenu.disconnect"),
        icon: XCircle,
        danger: true,
        shortcut: hint("pane.close"),
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
          shortcut: hint("pane.splitH"),
          onClick: () => onSplitHorizontal?.(),
        },
        {
          label: t("termMenu.splitV"),
          icon: Rows2,
          shortcut: hint("pane.splitV"),
          onClick: () => onSplitVertical?.(),
        },
      );
    }

    if (hasSelection) {
      items.push({ type: "separator" });
      items.push(
        {
          label: t("termMenu.copy"),
          icon: CopyIcon,
          shortcut: hint("terminal.copy"),
          onClick: () => void handleCopy(),
        },
        { label: t("termMenu.copyRtf"), icon: FileText, onClick: () => void handleCopyRTF() },
        { label: t("termMenu.pasteSelected"), icon: ClipboardList, onClick: handlePasteSelected },
      );
    }

    // Paste works with or without a selection; the clipboard image upload
    // lands in /tmp (remote) or the local temp dir and inserts the path.
    items.push({ type: "separator" });
    items.push(
      {
        label: t("termMenu.paste"),
        icon: ClipboardPaste,
        shortcut: hint("terminal.paste"),
        onClick: () => void handlePaste(),
      },
      { label: t("termMenu.uploadImage"), icon: ImageUp, onClick: () => void handleUploadClipboardImage() },
    );

    items.push(
      { type: "separator" },
      {
        label: t("termMenu.selectAll"),
        icon: TextSelect,
        shortcut: hint("terminal.selectAll"),
        onClick: handleSelectAll,
      },
      { label: t("termMenu.selectScreen"), icon: ScanLine, onClick: handleSelectScreen },
      { type: "separator" },
      {
        label: t("termMenu.find"),
        icon: Search,
        shortcut: hint("terminal.find"),
        onClick: () => setShowSearch(true),
      },
      {
        label: t("termMenu.pasteLocalIP"),
        icon: Network,
        onClick: () => void handlePasteLocalIP(),
      },
      {
        label: t("termMenu.pasteRemoteIP"),
        icon: Globe,
        disabled: !remoteIP,
        onClick: handlePasteRemoteIP,
      },
      { type: "separator" },
      {
        label: t("termMenu.clear"),
        icon: Eraser,
        shortcut: hint("terminal.clear"),
        onClick: handleClear,
      },
      { type: "separator" },
      {
        label: t("termMenu.terminalAppearance"),
        icon: Palette,
        onClick: () => setShowAppearanceDialog(true),
      },
    );

    return items;
  };

  return (
    <div
      className="relative h-full w-full overflow-hidden"
      style={{ background: resolvedTheme.theme.background }}
    >
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

      {/* The theme background covers sub-cell leftover pixels (canvas is
          sized in whole character cells) so no region ever shows through to
          the app shell behind the pane. */}
      <div
        ref={containerRef}
        className="h-full w-full p-2"
        style={{ background: resolvedTheme.theme.background }}
      />

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

      {/* Appearance picker (theme / font / size) — preview inside the dialog;
          on confirm: apply to this pane via the session snapshot (effects
          re-apply theme/font), and persist so the next session on the same
          host (or the next local terminal) opens with the new appearance. */}
      <TerminalAppearanceDialog
        open={showAppearanceDialog}
        currentThemeId={session.terminalTheme}
        currentFont={session.terminalFont || ""}
        currentFontSize={resolvedFontSize + zoomDelta}
        onConfirm={(themeId, font, size) => {
          setShowAppearanceDialog(false);
          // Write the new values into the tab's snapshot — the theme/font
          // effects re-apply from it — and reset the zoom delta so the
          // absolute size is kept without double application.
          setZoomDelta(0);
          setSessionAppearance(tabId, {
            terminalTheme: themeId,
            terminalFont: font,
            terminalFontSize: size,
          });
          persistAppearance(themeId, font, size);
        }}
        onClose={() => setShowAppearanceDialog(false)}
      />
    </div>
  );
}

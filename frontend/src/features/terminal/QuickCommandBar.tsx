import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ChevronDown,
  Play,
  Settings2,
  ListChecks,
} from "lucide-react";

import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import {
  useQuickCommands,
  quickCommandPayload,
  QUICKCMD_SKIP_CONFIRM_KEY,
  type QuickCommand,
} from "@/features/terminal/quickCommands";
import { QuickCommandManageDialog } from "@/features/terminal/QuickCommandManageDialog";
import { QuickSendConfirmDialog } from "@/features/terminal/QuickSendConfirmDialog";
import { Checkbox } from "@/components/ui/checkbox";
import { encodeBase64 } from "@/lib/base64";
import { cn } from "@/lib/utils";
import { toast } from "@/lib/toast";

/** First-line preview for tooltips / confirm dialogs. */
const previewOf = (command: string): string => {
  const first = command.replace(/\r?\n/g, " ⏎ ").trim();
  return first.length > 120 ? `${first.slice(0, 120)}…` : first;
};

/**
 * Quick command bar (Xshell 风格), rendered as a strip BELOW the terminal
 * workspace. Shows the user's saved quick commands as buttons; clicking one
 * types the script into every target session chosen in the bar's target
 * picker (defaults to the active tab, multi-select for batch sends).
 * Commands are managed in QuickCommandManageDialog; batch sends to more than
 * one tab confirm first unless the user opted out.
 */
export function QuickCommandBar() {
  const { t } = useTranslation();
  const commands = useQuickCommands();
  const sessions = useTerminalStore((s) => s.sessions);
  const activeId = useTerminalStore((s) => s.activeId);

  // Target selection = tab ids. Follows the active tab until the user picks
  // targets by hand (dirty), after which the explicit selection stands.
  const [targets, setTargets] = useState<Set<string>>(() => new Set());
  const [dirty, setDirty] = useState(false);

  const [pickerOpen, setPickerOpen] = useState(false);
  const [manageOpen, setManageOpen] = useState(false);
  // Snapshot of the command + targets under review — immune to concurrent
  // list edits while the confirm dialog is open.
  const [confirm, setConfirm] = useState<{
    cmd: QuickCommand;
    tabIds: string[];
  } | null>(null);

  const pickerRef = useRef<HTMLDivElement | null>(null);

  // Only live tabs can receive input; closed/error tabs would silently drop it.
  const sendable = useMemo(
    () => sessions.filter((s) => s.status === "connected"),
    [sessions],
  );

  // Follow the active tab while the user hasn't customized the selection.
  useEffect(() => {
    if (dirty) return;
    setTargets(activeId && sendable.some((s) => s.id === activeId) ? new Set([activeId]) : new Set());
  }, [activeId, dirty, sendable]);

  // Prune closed tabs out of an explicit selection so stale ids never linger.
  useEffect(() => {
    setTargets((prev) => {
      const live = new Set(sessions.map((s) => s.id));
      const next = new Set([...prev].filter((id) => live.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [sessions]);

  // Close the target picker on outside click / Escape (ContextMenu-style).
  useEffect(() => {
    if (!pickerOpen) return;
    const onDown = (e: MouseEvent) => {
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) {
        setPickerOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setPickerOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [pickerOpen]);

  const toggleTarget = (id: string) => {
    setDirty(true);
    setTargets((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const targetTabs = sendable.filter((s) => targets.has(s.id));

  const targetLabel = (() => {
    if (targetTabs.length === 0) return t("quickCmd.noTarget");
    if (targetTabs.length === 1) return targetTabs[0].hostName;
    return t("quickCmd.targetCount", { n: targetTabs.length });
  })();

  /**
   * Send a quick command to a set of tabs. Every pane of a tab receives the
   * payload (split panes are shells on the same target). Text is written
   * raw — line breaks already carriage returns per quickCommandPayload — so
   * multi-line scripts execute line by line, exactly as if typed.
   */
  const sendToTabs = (cmd: QuickCommand, tabIds: string[]) => {
    if (cmd.command.trim() === "" || tabIds.length === 0) return;
    const payload = quickCommandPayload(cmd);
    for (const tabId of tabIds) {
      const tab = sessions.find((s) => s.id === tabId);
      if (!tab) continue;
      for (const paneId of tab.paneIds) {
        TerminalService.WriteStdin(paneId, encodeBase64(payload)).catch(() => {
          /* session may have just closed */
        });
      }
    }
    toast.success(t("quickCmd.sent", { n: tabIds.length, name: cmd.name }));
  };

  const handleSend = (cmd: QuickCommand) => {
    const tabIds = targetTabs.map((s) => s.id);
    if (tabIds.length === 0 || cmd.command.trim() === "") return;
    // Batch sends (more than one tab) get one review dialog unless the user
    // checked "don't ask again" on an earlier send.
    if (tabIds.length <= 1 || localStorage.getItem(QUICKCMD_SKIP_CONFIRM_KEY) === "1") {
      sendToTabs(cmd, tabIds);
      return;
    }
    setConfirm({ cmd, tabIds });
  };

  return (
    <div className="relative flex h-9 shrink-0 items-center gap-1 border-t border-border bg-card px-2">
      {/* Target picker: multi-select over live tabs (Xshell compose-bar style
          "current session / all sessions / pick"), defaults to the active tab. */}
      <div ref={pickerRef} className="relative shrink-0">
        <button
          type="button"
          onClick={() => setPickerOpen((v) => !v)}
          title={t("quickCmd.targetTitle")}
          className={cn(
            "flex h-7 items-center gap-1.5 rounded-[var(--radius)] border border-border px-2 text-xs transition-colors",
            pickerOpen
              ? "bg-accent text-foreground"
              : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
          )}
        >
          <ListChecks className="h-3.5 w-3.5" />
          <span className="max-w-[10rem] truncate">{targetLabel}</span>
          <ChevronDown className="h-3 w-3 opacity-60" />
        </button>

        {pickerOpen && (
          <div className="absolute bottom-9 left-0 z-50 w-64 rounded-[var(--radius)] border border-border bg-popover p-1.5 shadow-lg">
            <div className="flex items-center justify-between px-1.5 pb-1">
              <span className="text-[11px] text-muted-foreground">
                {t("quickCmd.targetTitle")}
              </span>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  className="rounded px-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => {
                    setDirty(true);
                    setTargets(new Set(sendable.map((s) => s.id)));
                  }}
                >
                  {t("quickCmd.selectAll")}
                </button>
                <button
                  type="button"
                  className="rounded px-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={() => {
                    setDirty(true);
                    setTargets(new Set());
                  }}
                >
                  {t("quickCmd.selectNone")}
                </button>
              </div>
            </div>
            <div className="max-h-56 overflow-y-auto">
              {sendable.length === 0 && (
                <p className="px-1.5 py-2 text-xs text-muted-foreground">
                  {t("quickCmd.noSessions")}
                </p>
              )}
              {sendable.map((s) => (
                <label
                  key={s.id}
                  className="flex cursor-pointer items-center gap-2 rounded-[calc(var(--radius)-2px)] px-1.5 py-1 text-xs hover:bg-accent/60"
                >
                  <Checkbox
                    checked={targets.has(s.id)}
                    onCheckedChange={() => toggleTarget(s.id)}
                  />
                  <span className="min-w-0 flex-1 truncate">
                    {s.hostName}
                    {s.paneIds.length > 1 && (
                      <span className="ml-1 text-muted-foreground">
                        ({t("quickCmd.paneCount", { n: s.paneIds.length })})
                      </span>
                    )}
                  </span>
                </label>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="h-4 w-px shrink-0 bg-border" />

      {/* Quick command buttons; the row scrolls horizontally like Xshell's. */}
      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
        {commands.map((cmd) => (
          <button
            key={cmd.id}
            type="button"
            disabled={targetTabs.length === 0 || cmd.command.trim() === ""}
            onClick={() => handleSend(cmd)}
            title={`${previewOf(cmd.command)}${cmd.sendEnter ? " ↵" : ""}\n${t("quickCmd.sendTo", { target: targetLabel })}`}
            className="flex h-7 shrink-0 items-center gap-1 rounded-[var(--radius)] px-2.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
          >
            <Play className="h-3 w-3 text-primary/80" />
            <span className="max-w-[14rem] truncate">{cmd.name}</span>
            {cmd.sendEnter && <span className="text-[10px] opacity-60">↵</span>}
          </button>
        ))}
        {commands.length === 0 && (
          <span className="truncate px-1 text-xs text-muted-foreground">
            {t("quickCmd.emptyHint")}
          </span>
        )}
      </div>

      {/* Manage entries (add / edit / delete / reorder). */}
      <button
        type="button"
        onClick={() => setManageOpen(true)}
        title={t("quickCmd.manage")}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
      >
        <Settings2 className="h-4 w-4" />
      </button>

      {manageOpen && (
        <QuickCommandManageDialog onClose={() => setManageOpen(false)} />
      )}

      {confirm && (
        <QuickSendConfirmDialog
          command={confirm.cmd.command}
          sendEnter={confirm.cmd.sendEnter}
          targetTabs={targetTabs.filter((s) => confirm.tabIds.includes(s.id))}
          onConfirm={(dontAskAgain) => {
            if (dontAskAgain) {
              localStorage.setItem(QUICKCMD_SKIP_CONFIRM_KEY, "1");
            }
            const { cmd, tabIds } = confirm;
            setConfirm(null);
            sendToTabs(cmd, tabIds);
          }}
          onClose={() => setConfirm(null)}
        />
      )}
    </div>
  );
}

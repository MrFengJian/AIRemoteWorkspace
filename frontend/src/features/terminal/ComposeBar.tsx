import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ListChecks, Send } from "lucide-react";

import {
  useSendTargets,
  sendPayloadToTabs,
} from "@/features/terminal/sendTargets";
import { SendConfirmDialog } from "@/features/terminal/SendConfirmDialog";
import { QUICKCMD_SKIP_CONFIRM_KEY } from "@/features/terminal/quickCommands";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import { toast } from "@/lib/toast";

/**
 * Compose bar (Xshell 撰写栏) — an INDEPENDENT feature from the quick
 * command bar: a strip with a free-form input whose text is batch-sent to
 * the target sessions picked in the bar's own target selector. Enter
 * executes (trailing carriage return); Shift+Enter only types the text so
 * the user can review before pressing Enter themselves. Multi-target sends
 * go through the shared review dialog unless opted out.
 *
 * The IME composition guard keeps Enter-to-confirm-candidate from firing a
 * send mid-composition (Chinese input).
 */
export function ComposeBar() {
  const { t } = useTranslation();
  const { sessions, sendable, targets, targetTabs, toggleTarget, selectAllTargets, selectNoTargets } =
    useSendTargets();

  const [draft, setDraft] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  // Snapshot under review — immune to concurrent edits while the dialog is up.
  const [confirm, setConfirm] = useState<{
    name: string;
    command: string; // display text, without the trailing enter
    sendEnter: boolean;
    payload: string; // exact PTY bytes
    tabIds: string[];
  } | null>(null);

  const pickerRef = useRef<HTMLDivElement | null>(null);

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

  const targetLabel = (() => {
    if (targetTabs.length === 0) return t("batchSend.noTarget");
    if (targetTabs.length === 1) return targetTabs[0].hostName;
    return t("batchSend.targetCount", { n: targetTabs.length });
  })();

  const sendNow = (name: string, payload: string, tabIds: string[]) => {
    const sent = sendPayloadToTabs(sessions, payload, tabIds);
    if (sent > 0) {
      toast.success(t("batchSend.sent", { n: sent, name }));
      setDraft(""); // fresh input after a real send; cancel keeps the text
    }
  };

  const handleSend = (sendEnter: boolean) => {
    const text = draft;
    if (text.trim() === "") return;
    const tabIds = targetTabs.map((s) => s.id);
    if (tabIds.length === 0) return;
    const body = text.replace(/\r?\n/g, "\r");
    const payload = sendEnter ? `${body}\r` : body;
    const name = text.length > 16 ? `${text.slice(0, 16)}…` : text;
    // Batch sends (more than one tab) get one review dialog unless the user
    // checked "don't ask again" on an earlier send.
    if (tabIds.length <= 1 || localStorage.getItem(QUICKCMD_SKIP_CONFIRM_KEY) === "1") {
      sendNow(name, payload, tabIds);
      return;
    }
    setConfirm({ name, command: text, sendEnter, payload, tabIds });
  };

  return (
    <div className="relative flex h-9 shrink-0 items-center gap-1 border-t border-border bg-card px-2">
      {/* Target picker — this bar's OWN selection, independent of the quick
          command bar's (both follow the active tab until customized). */}
      <div ref={pickerRef} className="relative shrink-0">
        <button
          type="button"
          onClick={() => setPickerOpen((v) => !v)}
          title={t("batchSend.targetTitle")}
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
                {t("batchSend.targetTitle")}
              </span>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  className="rounded px-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={selectAllTargets}
                >
                  {t("batchSend.selectAll")}
                </button>
                <button
                  type="button"
                  className="rounded px-1 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
                  onClick={selectNoTargets}
                >
                  {t("batchSend.selectNone")}
                </button>
              </div>
            </div>
            <div className="max-h-56 overflow-y-auto">
              {sendable.length === 0 && (
                <p className="px-1.5 py-2 text-xs text-muted-foreground">
                  {t("batchSend.noSessions")}
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
                        ({t("batchSend.paneCount", { n: s.paneIds.length })})
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

      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault();
            handleSend(!e.shiftKey);
          }
        }}
        disabled={targetTabs.length === 0}
        title={t("compose.inputTitle")}
        placeholder={t("compose.placeholder")}
        className="h-7 min-w-0 flex-1 rounded-[var(--radius)] border border-input bg-background px-2.5 text-xs focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-40"
      />

      <button
        type="button"
        disabled={targetTabs.length === 0 || draft.trim() === ""}
        onClick={() => handleSend(true)}
        title={t("compose.sendTitle")}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--radius)] text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
      >
        <Send className="h-3.5 w-3.5" />
      </button>

      {confirm && (
        <SendConfirmDialog
          command={confirm.command}
          sendEnter={confirm.sendEnter}
          targetTabs={targetTabs.filter((s) => confirm.tabIds.includes(s.id))}
          onConfirm={(dontAskAgain) => {
            if (dontAskAgain) {
              localStorage.setItem(QUICKCMD_SKIP_CONFIRM_KEY, "1");
            }
            const { name, payload, tabIds } = confirm;
            setConfirm(null);
            sendNow(name, payload, tabIds);
          }}
          onClose={() => setConfirm(null)}
        />
      )}
    </div>
  );
}

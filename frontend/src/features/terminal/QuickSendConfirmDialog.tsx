import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Send, TriangleAlert } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import type { TerminalSession } from "@/features/terminal/terminal.store";

/**
 * Review gate before a quick command is batch-sent to more than one open
 * session: shows the exact payload and every target tab. "Don't ask again"
 * persists the opt-out so routine batch sends stay one click.
 */
export function QuickSendConfirmDialog({
  command,
  sendEnter,
  targetTabs,
  onConfirm,
  onClose,
}: {
  command: string;
  sendEnter: boolean;
  targetTabs: TerminalSession[];
  onConfirm: (dontAskAgain: boolean) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [dontAsk, setDontAsk] = useState(false);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <TriangleAlert className="h-4 w-4 shrink-0 text-warning" />
            {t("quickCmd.batchConfirmTitle")}
          </DialogTitle>
          <DialogDescription>
            {t("quickCmd.batchConfirmDesc", { n: targetTabs.length })}
          </DialogDescription>
        </DialogHeader>

        {/* The exact payload — monospace, line breaks visible. */}
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-[var(--radius)] border border-border bg-background/60 p-2.5 font-mono text-xs">
          {command}
          {sendEnter ? "↵" : ""}
        </pre>

        <div className="max-h-28 overflow-y-auto rounded-[var(--radius)] border border-border px-2.5 py-1.5">
          {targetTabs.map((s) => (
            <div key={s.id} className="truncate py-0.5 text-xs text-muted-foreground">
              {s.hostName}
              {s.paneIds.length > 1 && (
                <span className="ml-1 opacity-70">
                  ({t("quickCmd.paneCount", { n: s.paneIds.length })})
                </span>
              )}
            </div>
          ))}
        </div>

        <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
          <Checkbox checked={dontAsk} onCheckedChange={(v) => setDontAsk(v === true)} />
          {t("quickCmd.dontAskAgain")}
        </label>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            className="bg-primary text-primary-foreground"
            onClick={() => onConfirm(dontAsk)}
          >
            <Send className="h-3.5 w-3.5" />
            {t("quickCmd.batchConfirmSend")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

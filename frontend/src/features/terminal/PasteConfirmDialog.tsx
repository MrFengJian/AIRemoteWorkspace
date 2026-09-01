import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

/**
 * Multi-line paste guard dialog: shows the exact content about to be sent to
 * the shell (line breaks included) so the user can review it before anything
 * executes. Confirm writes the text verbatim; cancel discards it.
 */
export function PasteConfirmDialog({
  text,
  onConfirm,
  onClose,
}: {
  text: string;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const lines = text.replace(/[\r\n]+$/, "").split("\n").length;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <TriangleAlert className="h-4 w-4 shrink-0 text-warning" />
            {t("terminal.pasteMultiTitle")}
          </DialogTitle>
          <DialogDescription>{t("terminal.pasteMultiDesc", { n: lines })}</DialogDescription>
        </DialogHeader>

        {/* The exact paste payload — monospace, scrollable, line breaks shown */}
        <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded-[var(--radius)] border border-warning/40 bg-background/60 p-2.5 font-mono text-xs">
          {text}
        </pre>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" onClick={onConfirm}>
            {t("terminal.pasteMultiConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

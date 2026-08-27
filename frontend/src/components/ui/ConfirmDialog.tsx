import { useEffect, useState } from "react";
import { AlertTriangle, SquarePen } from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useConfirmStore } from "@/stores/confirm.store";
import { cn } from "@/lib/utils";

/**
 * Single confirmation/prompt dialog host. Mounted once in AppShell; driven by
 * useConfirmStore. Renders either a confirm (OK/Cancel) or prompt (text
 * input) dialog with the project's Dark Developer styling — replaces the
 * native window.confirm / window.prompt.
 */
export function ConfirmDialog() {
  const request = useConfirmStore((s) => s.request);
  const resolve = useConfirmStore((s) => s.resolve);
  const close = useConfirmStore((s) => s.close);
  const { t } = useTranslation();

  const [value, setValue] = useState("");

  // Reset the input whenever a new request opens.
  useEffect(() => {
    if (request?.mode === "prompt") {
      setValue(request.initialValue ?? "");
    }
  }, [request]);

  if (!request || !resolve) return null;

  const isPrompt = request.mode === "prompt";

  const finish = (result: boolean | string | null) => {
    const r = resolve;
    close();
    r(result);
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) finish(false); // treated as cancel
      }}
    >
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            {request.danger ? (
              <AlertTriangle className="h-4 w-4 text-destructive" />
            ) : isPrompt ? (
              <SquarePen className="h-4 w-4 text-primary" />
            ) : null}
            {request.title}
          </DialogTitle>
          {request.message && (
            <DialogDescription className="whitespace-pre-wrap break-words">
              {request.message}
            </DialogDescription>
          )}
        </DialogHeader>

        {isPrompt && (
          <div className="py-1">
            <Input
              autoFocus
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={request.placeholder}
              onKeyDown={(e) => {
                if (e.key === "Enter") finish(value.trim() || null);
                if (e.key === "Escape") finish(null);
              }}
            />
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => finish(false)}
          >
            {request.cancelLabel ?? t("common.cancel")}
          </Button>
          {request.altLabel && (
            <Button
              variant="outline"
              onClick={() => finish("alt")}
            >
              {request.altLabel}
            </Button>
          )}
          <Button
            variant={request.danger ? "destructive" : "default"}
            className={cn(!request.danger && "bg-primary text-primary-foreground")}
            onClick={() => finish(isPrompt ? value.trim() || null : true)}
            disabled={isPrompt && !value.trim()}
          >
            {request.confirmLabel ?? t(isPrompt ? "common.ok" : "common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

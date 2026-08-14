import { CheckCircle2, Info, XCircle, X } from "lucide-react";

import { useToastStore, type ToastVariant } from "@/stores/toast.store";
import { cn } from "@/lib/utils";

const ICONS: Record<ToastVariant, typeof Info> = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
};

const ICON_COLORS: Record<ToastVariant, string> = {
  success: "text-success",
  error: "text-destructive",
  info: "text-primary",
};

/**
 * Single toast host. Mounted once in AppShell; driven by useToastStore
 * (see lib/toast.ts). Stacks bottom-right above the status bar.
 */
export function Toaster() {
  const toasts = useToastStore((s) => s.toasts);
  const dismiss = useToastStore((s) => s.dismiss);

  if (toasts.length === 0) return null;

  return (
    <div
      aria-live="polite"
      aria-label="Notifications"
      className="pointer-events-none absolute bottom-10 right-4 z-50 flex w-80 flex-col gap-2"
    >
      {toasts.map((t) => {
        const Icon = ICONS[t.variant];
        return (
          <div
            key={t.id}
            role="status"
            className="pointer-events-auto flex items-start gap-2 rounded-[var(--radius)] border border-border bg-popover p-3 text-sm shadow-lg"
          >
            <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", ICON_COLORS[t.variant])} />
            <p className="min-w-0 flex-1 break-words whitespace-pre-wrap text-foreground">
              {t.message}
            </p>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss notification"
              className="rounded p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

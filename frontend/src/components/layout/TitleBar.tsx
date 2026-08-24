import { useState } from "react";
import { Minus, Square, Copy, X, TerminalSquare } from "lucide-react";
import { Window } from "@wailsio/runtime";

import { cn } from "@/lib/utils";

/** True on macOS — the system traffic lights are native there (HiddenInset
 *  title bar), so no custom window buttons are drawn and the app title makes
 *  room for them on the left. */
const IS_MAC =
  typeof navigator !== "undefined" && /Mac|iPod|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

/**
 * TitleBar — the frameless window's top chrome (Wails `Frameless: true` on
 * Windows/Linux). The empty strip is the drag region (--wails-draggable);
 * double-clicking it toggles maximise. On macOS the native traffic lights
 * overlay this bar and no custom buttons are rendered.
 */
export function TitleBar({ title }: { title: string }) {
  const [maximised, setMaximised] = useState(false);

  const minimize = () => {
    Window.Minimise().catch(() => {});
  };

  const toggleMaximise = async () => {
    try {
      await Window.ToggleMaximise();
      setMaximised(await Window.IsMaximised());
    } catch {
      /* window API unavailable (e.g. browser dev) */
    }
  };

  const close = () => {
    Window.Close().catch(() => {});
  };

  return (
    <div
      className="flex h-8 shrink-0 select-none items-center gap-2 border-b border-border bg-card px-3"
      style={{ "--wails-draggable": "drag" } as React.CSSProperties}
      onDoubleClick={(e) => {
        // Only the empty strip toggles maximise; buttons/children handle
        // their own double-clicks.
        if (e.target === e.currentTarget) {
          toggleMaximise();
        }
      }}
    >
      <span
        className={cn(
          "flex min-w-0 items-center gap-1.5 text-xs font-medium text-muted-foreground",
          // Leave room for the native traffic lights on macOS.
          IS_MAC && "pl-[70px]",
        )}
      >
        <TerminalSquare className="h-3.5 w-3.5 shrink-0 text-primary" />
        <span className="truncate">{title}</span>
      </span>

      <span className="flex-1" aria-hidden />

      {!IS_MAC && (
        <span className="flex items-center">
          <button
            type="button"
            onClick={minimize}
            aria-label="Minimize"
            title="Minimize"
            className="flex h-7 w-10 items-center justify-center rounded-[calc(var(--radius)-2px)] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Minus className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={toggleMaximise}
            aria-label="Maximize"
            title="Maximize"
            className="flex h-7 w-10 items-center justify-center rounded-[calc(var(--radius)-2px)] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {maximised ? <Copy className="h-3 w-3" /> : <Square className="h-3 w-3" />}
          </button>
          <button
            type="button"
            onClick={close}
            aria-label="Close"
            title="Close"
            className="flex h-7 w-10 items-center justify-center rounded-[calc(var(--radius)-2px)] text-muted-foreground transition-colors hover:bg-destructive hover:text-destructive-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </span>
      )}
    </div>
  );
}

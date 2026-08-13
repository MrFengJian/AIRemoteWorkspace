import { useEffect, useRef } from "react";
import type { LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export interface MenuItem {
  label?: string;
  icon?: LucideIcon;
  danger?: boolean;
  disabled?: boolean;
  /** Separator between menu groups. */
  type?: "separator";
  onClick?: () => void;
}

interface TerminalTabMenuProps {
  x: number;
  y: number;
  items: MenuItem[];
  onClose: () => void;
}

/**
 * Lightweight context menu for terminal tabs. Rendered at the cursor position
 * and closes on outside click / Escape / scroll.
 */
export function TerminalTabMenu({ x, y, items, onClose }: TerminalTabMenuProps) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const handle = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent) {
        if (e.key === "Escape") onClose();
        return;
      }
      // Close when clicking outside the menu.
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const onScroll = () => onClose();

    window.addEventListener("mousedown", handle);
    window.addEventListener("keydown", handle);
    window.addEventListener("scroll", onScroll, true);
    return () => {
      window.removeEventListener("mousedown", handle);
      window.removeEventListener("keydown", handle);
      window.removeEventListener("scroll", onScroll, true);
    };
  }, [onClose]);

  // Keep the menu within the viewport.
  const style: React.CSSProperties = {
    position: "fixed",
    left: Math.min(x, window.innerWidth - 200),
    top: Math.min(y, window.innerHeight - items.length * 32 - 8),
    zIndex: 1000,
  };

  return (
    <div
      ref={ref}
      style={style}
      className="min-w-[10rem] rounded-[var(--radius)] border border-border bg-popover p-1 shadow-lg"
      onContextMenu={(e) => e.preventDefault()}
    >
      {items.map((item, i) => {
        if (item.type === "separator") {
          return <div key={i} className="my-1 h-px bg-border" />;
        }
        const Icon = item.icon;
        return (
          <button
            key={i}
            type="button"
            disabled={item.disabled}
            onClick={() => {
              item.onClick?.();
              onClose();
            }}
            className={cn(
              "flex w-full items-center gap-2 rounded-[calc(var(--radius)-2px)] px-2.5 py-1.5 text-left text-sm transition-colors",
              item.danger
                ? "text-destructive hover:bg-destructive/10"
                : "text-popover-foreground hover:bg-accent",
              item.disabled && "cursor-not-allowed opacity-40 hover:bg-transparent",
            )}
          >
            {Icon && <Icon className="h-3.5 w-3.5 shrink-0" />}
            {item.label}
          </button>
        );
      })}
    </div>
  );
}

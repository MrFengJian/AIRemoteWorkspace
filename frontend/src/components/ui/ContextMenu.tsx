import { useEffect, useRef, useState } from "react";
import { Check, ChevronRight } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export interface MenuItem {
  label?: string;
  icon?: LucideIcon;
  danger?: boolean;
  disabled?: boolean;
  /** Show a check mark on the right when this item is the active choice. */
  checked?: boolean;
  /** Right-aligned keybinding hint (e.g. "Ctrl+Shift+C"), Xshell-style. */
  shortcut?: string;
  /** Separator between menu groups. */
  type?: "separator";
  onClick?: () => void;
  /** Nested submenu, revealed on hover. */
  children?: MenuItem[];
}

interface ContextMenuProps {
  x: number;
  y: number;
  items: MenuItem[];
  onClose: () => void;
}

/**
 * Lightweight shared context menu (terminal tabs, SFTP panel, …). Rendered
 * at the cursor position and closes on outside click / Escape / scroll.
 * Items with `children` reveal a nested submenu anchored to their right
 * edge on hover. The browser's default context menu is suppressed by the
 * caller via onContextMenu={e => { e.preventDefault(); … }}.
 */
export function ContextMenu({ x, y, items, onClose }: ContextMenuProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [sub, setSub] = useState<{ items: MenuItem[]; x: number; y: number } | null>(null);

  useEffect(() => {
    const handle = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent) {
        if (e.key === "Escape") onClose();
        return;
      }
      // Close when clicking outside the menu (and any open submenu).
      if (
        ref.current &&
        !ref.current.contains(e.target as Node) &&
        !(e.target as HTMLElement).closest?.("[data-submenu]")
      ) {
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

  // renderItems renders one menu level. inSubmenu marks items living inside
  // a nested submenu: hovering those must NOT clear the submenu state — only
  // hovering a PARENT-level plain item closes/replaces it (otherwise moving
  // the mouse onto a plain submenu item instantly unmounts the submenu it
  // belongs to).
  const renderItems = (list: MenuItem[], inSubmenu = false) =>
    list.map((item, i) => {
      if (item.type === "separator") {
        return <div key={i} className="my-1 h-px bg-border" />;
      }
      const Icon = item.icon;
      return (
        <button
          key={i}
          type="button"
          disabled={item.disabled}
          onMouseEnter={(e) => {
            if (item.children?.length) {
              const rect = e.currentTarget.getBoundingClientRect();
              // Open to the right when there is room; flip to the left when
              // the viewport edge would clamp the submenu back on top of the
              // parent menu (overlapping it makes the submenu unclosable-by-
              // navigation and instantly closed by parent-item hovers).
              let sx = rect.right + 4;
              if (sx + 180 > window.innerWidth) {
                sx = Math.max(4, rect.left - 184);
              }
              setSub({ items: item.children, x: sx, y: rect.top });
            } else if (!inSubmenu) {
              setSub(null);
            }
          }}
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
          <span className="flex-1 truncate">{item.label}</span>
          {item.shortcut && (
            <span className="ml-3 shrink-0 font-mono text-[10px] leading-none tracking-wide text-muted-foreground">
              {item.shortcut}
            </span>
          )}
          {item.checked && <Check className="h-3.5 w-3.5 shrink-0 text-foreground" />}
          {item.children?.length ? <ChevronRight className="h-3.5 w-3.5 shrink-0 opacity-60" /> : null}
        </button>
      );
    });

  // Keep the menu within the viewport.
  const style: React.CSSProperties = {
    position: "fixed",
    left: Math.min(x, window.innerWidth - 220),
    top: Math.min(y, window.innerHeight - items.length * 32 - 8),
    zIndex: 1000,
  };

  return (
    <div
      ref={ref}
      style={style}
      className="min-w-[11rem] rounded-[var(--radius)] border border-border bg-popover p-1 shadow-lg"
      onContextMenu={(e) => e.preventDefault()}
    >
      {renderItems(items)}
      {sub && (
        <div
          data-submenu
          style={{
            position: "fixed",
            left: Math.min(sub.x, window.innerWidth - 220),
            top: Math.min(sub.y, window.innerHeight - sub.items.length * 32 - 8),
            zIndex: 1001,
          }}
          className="min-w-[11rem] rounded-[var(--radius)] border border-border bg-popover p-1 shadow-lg"
          onMouseLeave={() => setSub(null)}
          onContextMenu={(e) => e.preventDefault()}
        >
          {renderItems(sub.items, true)}
        </div>
      )}
    </div>
  );
}

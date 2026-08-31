import {
  TerminalSquare,
  Settings,
  FolderTree,
  type LucideIcon,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { useUIStore, type AppView } from "@/stores/ui.store";

interface NavItem {
  view: AppView;
  labelKey: string;
  icon: LucideIcon;
}

// Main window: hosts, SFTP and the agent are intentionally NOT in the nav —
// they live inside the terminal workspace (hosts sidebar / right panel).
const NAV_ITEMS: NavItem[] = [
  { view: "terminal", labelKey: "nav.terminal", icon: TerminalSquare },
  { view: "settings", labelKey: "nav.settings", icon: Settings },
];

// Standalone SFTP window: the workbench itself is the workspace, plus the
// settings page (transfer limits etc. live in Settings → Advanced).
export const SFTP_NAV_ITEMS: NavItem[] = [
  { view: "sftp", labelKey: "nav.files", icon: FolderTree },
  { view: "settings", labelKey: "nav.settings", icon: Settings },
];

/**
 * Left navigation rail. Icon-driven, keyboard-targetable, dark-first —
 * matches the Cursor / Raycast / VS Code aesthetic (AGENT.md §4). The items
 * can be overridden for auxiliary windows (see SFTP_NAV_ITEMS).
 */
export function Sidebar({ items = NAV_ITEMS }: { items?: NavItem[] }) {
  const activeView = useUIStore((s) => s.activeView);
  const setView = useUIStore((s) => s.setView);
  const { t } = useTranslation();

  return (
    <nav
      aria-label="Primary"
      className="flex h-full w-14 flex-col items-center gap-1 border-r border-border bg-card py-3"
    >
      {items.map(({ view, labelKey, icon: Icon }) => {
        const active = activeView === view;
        const label = t(labelKey);
        return (
          <button
            key={view}
            type="button"
            onClick={() => setView(view)}
            title={label}
            aria-label={label}
            aria-current={active ? "page" : undefined}
            className={cn(
              "group relative flex h-10 w-10 items-center justify-center rounded-[var(--radius)] transition-colors",
              "text-muted-foreground hover:bg-accent hover:text-foreground",
              active && "bg-accent text-foreground",
            )}
          >
            <Icon className="h-5 w-5" />
          </button>
        );
      })}
    </nav>
  );
}

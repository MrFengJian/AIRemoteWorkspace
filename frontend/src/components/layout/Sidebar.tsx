import {
  LayoutDashboard,
  Server,
  TerminalSquare,
  FolderTree,
  Bot,
  Settings,
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

const NAV_ITEMS: NavItem[] = [
  { view: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
  { view: "hosts", labelKey: "nav.hosts", icon: Server },
  { view: "terminal", labelKey: "nav.terminal", icon: TerminalSquare },
  { view: "sftp", labelKey: "nav.files", icon: FolderTree },
  { view: "agent", labelKey: "nav.agent", icon: Bot },
  { view: "settings", labelKey: "nav.settings", icon: Settings },
];

/**
 * Left navigation rail. Icon-driven, keyboard-targetable, dark-first —
 * matches the Cursor / Raycast / VS Code aesthetic (AGENT.md §4).
 */
export function Sidebar() {
  const activeView = useUIStore((s) => s.activeView);
  const setView = useUIStore((s) => s.setView);
  const { t } = useTranslation();

  return (
    <nav
      aria-label="Primary"
      className="flex h-full w-14 flex-col items-center gap-1 border-r border-border bg-card py-3"
    >
      {NAV_ITEMS.map(({ view, labelKey, icon: Icon }) => {
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

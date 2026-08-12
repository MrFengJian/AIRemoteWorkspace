import {
  LayoutDashboard,
  Server,
  TerminalSquare,
  FolderTree,
  Bot,
  Settings,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { useUIStore, type AppView } from "@/stores/ui.store";

interface NavItem {
  view: AppView;
  label: string;
  icon: LucideIcon;
  /** Phase when this view becomes functional (used for the badge tooltip). */
  phase?: string;
}

const NAV_ITEMS: NavItem[] = [
  { view: "dashboard", label: "Dashboard", icon: LayoutDashboard },
  { view: "hosts", label: "Hosts", icon: Server, phase: "Phase 2" },
  { view: "terminal", label: "Terminal", icon: TerminalSquare, phase: "Phase 2" },
  { view: "sftp", label: "Files", icon: FolderTree, phase: "Phase 3" },
  { view: "agent", label: "Agent", icon: Bot, phase: "Phase 4" },
  { view: "settings", label: "Settings", icon: Settings },
];

/**
 * Left navigation rail. Icon-driven, keyboard-targetable, dark-first —
 * matches the Cursor / Raycast / VS Code aesthetic (AGENT.md §4).
 */
export function Sidebar() {
  const activeView = useUIStore((s) => s.activeView);
  const setView = useUIStore((s) => s.setView);

  return (
    <nav
      aria-label="Primary"
      className="flex h-full w-14 flex-col items-center gap-1 border-r border-border bg-card py-3"
    >
      {NAV_ITEMS.map(({ view, label, icon: Icon, phase }) => {
        const active = activeView === view;
        return (
          <button
            key={view}
            type="button"
            onClick={() => setView(view)}
            title={phase ? `${label} (${phase})` : label}
            aria-label={label}
            aria-current={active ? "page" : undefined}
            className={cn(
              "group relative flex h-10 w-10 items-center justify-center rounded-[var(--radius)] transition-colors",
              "text-muted-foreground hover:bg-accent hover:text-foreground",
              active && "bg-accent text-foreground",
            )}
          >
            <Icon className="h-5 w-5" />
            {phase && (
              <span className="pointer-events-none absolute left-12 hidden whitespace-nowrap rounded-[var(--radius)] border border-border bg-popover px-2 py-1 text-xs text-popover-foreground shadow-md group-hover:block" />
            )}
          </button>
        );
      })}
    </nav>
  );
}

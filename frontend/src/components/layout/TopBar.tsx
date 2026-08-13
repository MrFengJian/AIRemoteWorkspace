import { useTranslation } from "react-i18next";

import { useUIStore, type AppView } from "@/stores/ui.store";

const VIEW_TITLE_KEYS: Record<AppView, string> = {
  dashboard: "nav.dashboard",
  hosts: "nav.hosts",
  terminal: "nav.terminal",
  sftp: "nav.files",
  agent: "nav.agent",
  settings: "nav.settings",
};

/**
 * Slim top bar showing the active view. Intentionally minimal for Phase 1 —
 * keyboard shortcut hints and the Command Palette trigger land in a later
 * phase (AGENT.md §4 lists them as experience targets, not Phase 1 scope).
 */
export function TopBar() {
  const activeView = useUIStore((s) => s.activeView);
  const { t } = useTranslation();
  return (
    <header className="flex h-9 shrink-0 items-center border-b border-border bg-card px-4">
      <span className="text-sm font-medium text-foreground">
        {t(VIEW_TITLE_KEYS[activeView])}
      </span>
    </header>
  );
}

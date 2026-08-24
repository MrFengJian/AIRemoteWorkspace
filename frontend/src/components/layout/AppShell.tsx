import * as React from "react";
import { useTranslation } from "react-i18next";

import { useUIStore } from "@/stores/ui.store";
import { Sidebar } from "@/components/layout/Sidebar";
import { StatusBar } from "@/components/layout/StatusBar";
import { TitleBar } from "@/components/layout/TitleBar";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Toaster } from "@/components/ui/Toaster";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { ApprovalHost } from "@/features/agent/ApprovalHost";
import { HostFormDialog } from "@/features/hosts/HostFormDialog";
import { TerminalView } from "@/features/terminal/TerminalView";
import { SettingsView } from "@/features/settings/SettingsView";

/**
 * Application chrome. The terminal workspace is the app: a frameless-mode
 * title bar on top, the icon rail + the active feature view in the middle,
 * and the status bar at the bottom. All views stay mounted and are toggled
 * via CSS (hidden/visible) instead of conditional rendering — unmounting
 * TerminalView would dispose the xterm instances and lose scrollback.
 *
 *   ┌──────────────────────────────┐
 *   │ TitleBar (drag + win ctrl)   │
 *   ├───┬──────────────────────────┤
 *   │ S │ Active feature view      │
 *   │ i │ (terminal hosts sidebar  │
 *   │ d │  + tabs live inside it)  │
 *   ├───┴──────────────────────────┤
 *   │ StatusBar                    │
 *   └──────────────────────────────┘
 */
export function AppShell() {
  const activeView = useUIStore((s) => s.activeView);
  const { t } = useTranslation();

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
      <TitleBar title={t("app.title")} />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        {/* overflow-hidden, not auto: main is the last scroll ancestor of the
            terminal panes. During a fit() debounce the old (wider) xterm screen
            transiently overflows its container; with overflow-auto that spilled
            into a horizontal scrollbar here, stealing 10px of height → rows
            resize → PTY repaint → reflow → scrollbar toggles again — a slow
            self-sustaining flicker loop. Views scroll internally themselves. */}
        <main className="relative min-w-0 flex-1 overflow-hidden">
          <View active={activeView === "terminal"}>
            <ErrorBoundary label={t("nav.terminal")} resetLabel={t("common.retry")}>
              <TerminalView />
            </ErrorBoundary>
          </View>
          <View active={activeView === "settings"}>
            <ErrorBoundary label={t("nav.settings")} resetLabel={t("common.retry")}>
              <SettingsView />
            </ErrorBoundary>
          </View>
        </main>
      </div>
      <StatusBar />
      {/* Host add/edit dialog — store-driven, reachable from the hosts
          sidebar's context menu and action buttons. */}
      <HostFormDialog />
      {/* Single confirmation/prompt dialog host (replaces window.confirm/prompt). */}
      <ConfirmDialog />
      {/* Single toast notification host (driven by lib/toast.ts). */}
      <Toaster />
      {/* Global agent-approval dialog — works regardless of the active view. */}
      <ApprovalHost />
    </div>
  );
}

/** View keeps its children mounted at all times; inactive views are hidden. */
function View({ active, children }: { active: boolean; children: React.ReactNode }) {
  return (
    <div className={active ? "h-full w-full" : "hidden h-full w-full"}>{children}</div>
  );
}

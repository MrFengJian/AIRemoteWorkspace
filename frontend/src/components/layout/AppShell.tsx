import * as React from "react";
import { useUIStore } from "@/stores/ui.store";
import { Sidebar } from "@/components/layout/Sidebar";
import { StatusBar } from "@/components/layout/StatusBar";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { DashboardView } from "@/features/dashboard/DashboardView";
import { HostsView } from "@/features/hosts/HostsView";
import { TerminalView } from "@/features/terminal/TerminalView";
import { SftpView } from "@/features/sftp/SftpView";
import { AgentView } from "@/features/agent/AgentView";
import { SettingsView } from "@/features/settings/SettingsView";

/**
 * Application chrome. All views stay mounted and are toggled via CSS
 * (hidden/visible) instead of conditional rendering. This matters most for
 * the terminal: unmounting TerminalView disposes the xterm instance and loses
 * scrollback, so switching away and back would show a blank terminal. Keeping
 * it mounted preserves the session view instantly.
 *
 *   ┌───┬──────────────────────────────────┐
 *   │ S │                                   │
 *   │ i │   Active feature view             │
 *   │ d │                                   │
 *   ├───┴──────────────────────────────────┤
 *   │ StatusBar                             │
 *   └──────────────────────────────────────┘
 */
export function AppShell() {
  const activeView = useUIStore((s) => s.activeView);

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main className="relative min-w-0 flex-1 overflow-auto">
          <View active={activeView === "dashboard"}>
            <DashboardView />
          </View>
          <View active={activeView === "hosts"}>
            <HostsView />
          </View>
          <View active={activeView === "terminal"}>
            <TerminalView />
          </View>
          <View active={activeView === "sftp"}>
            <SftpView />
          </View>
          <View active={activeView === "agent"}>
            <AgentView />
          </View>
          <View active={activeView === "settings"}>
            <SettingsView />
          </View>
        </main>
      </div>
      <StatusBar />
      {/* Single confirmation/prompt dialog host (replaces window.confirm/prompt). */}
      <ConfirmDialog />
    </div>
  );
}

/** View keeps its children mounted at all times; inactive views are hidden. */
function View({ active, children }: { active: boolean; children: React.ReactNode }) {
  return (
    <div className={active ? "h-full w-full" : "hidden h-full w-full"}>{children}</div>
  );
}

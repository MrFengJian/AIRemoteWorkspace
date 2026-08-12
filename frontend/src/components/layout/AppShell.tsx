import { useUIStore } from "@/stores/ui.store";
import { Sidebar } from "@/components/layout/Sidebar";
import { TopBar } from "@/components/layout/TopBar";
import { StatusBar } from "@/components/layout/StatusBar";
import { DashboardView } from "@/features/dashboard/DashboardView";
import { HostsView } from "@/features/hosts/HostsView";
import { TerminalView } from "@/features/terminal/TerminalView";
import { SftpView } from "@/features/sftp/SftpView";
import { AgentView } from "@/features/agent/AgentView";
import { SettingsView } from "@/features/settings/SettingsView";

/**
 * Application chrome. A fixed three-pane desktop layout:
 *
 *   ┌──────────────────────────────────────┐
 *   │ TopBar                                │
 *   ├───┬──────────────────────────────────┤
 *   │ S │                                   │
 *   │ i │   Active feature view             │
 *   │ d │                                   │
 *   ├───┴──────────────────────────────────┤
 *   │ StatusBar                             │
 *   └──────────────────────────────────────┘
 *
 * The active view is global UI state (stores/ui.store.ts); each feature owns
 * its own view component, store, and api under features/<name>/.
 */
export function AppShell() {
  const activeView = useUIStore((s) => s.activeView);

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
      <TopBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main className="min-w-0 flex-1 overflow-auto">
          {activeView === "dashboard" && <DashboardView />}
          {activeView === "hosts" && <HostsView />}
          {activeView === "terminal" && <TerminalView />}
          {activeView === "sftp" && <SftpView />}
          {activeView === "agent" && <AgentView />}
          {activeView === "settings" && <SettingsView />}
        </main>
      </div>
      <StatusBar />
    </div>
  );
}

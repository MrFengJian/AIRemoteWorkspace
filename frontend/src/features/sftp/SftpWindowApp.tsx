import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { Sidebar, SFTP_NAV_ITEMS } from "@/components/layout/Sidebar";
import { StatusBar } from "@/components/layout/StatusBar";
import { TitleBar } from "@/components/layout/TitleBar";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Toaster } from "@/components/ui/Toaster";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { useUIStore } from "@/stores/ui.store";
import { useShortcutDispatcher } from "@/keybindings/useShortcutDispatcher";
import { hostsApi } from "@/features/hosts/api";
import { SettingsView } from "@/features/settings/SettingsView";
import { SftpWorkbench } from "@/features/sftp/SftpWorkbench";

/**
 * SftpWindowApp — the whole content of the standalone SFTP window (opened
 * per host from the hosts sidebar; the backend loads this app bundle with a
 * "#/sftp-window?host=<id>" URL).
 *
 * Chrome mirrors the main window: frameless title bar, the left navigation
 * rail (workbench ⇄ settings — transfer limits live in Settings →
 * Advanced), and the status bar. The zustand stores are per-webview, so
 * this window's view state never touches the main window's.
 */
export function SftpWindowApp({ hostID }: { hostID: string }) {
  const { t } = useTranslation();
  const activeView = useUIStore((s) => s.activeView);
  const [hostName, setHostName] = useState("");

  // The workbench is this window's home view.
  useEffect(() => {
    useUIStore.getState().setView("sftp");
  }, []);

  // Global keyboard shortcuts (per-window listener; handlers registered by
  // the mounted views apply here, e.g. appearance shortcuts from settings).
  useShortcutDispatcher();

  // Title shows the host; a miss (host deleted meanwhile) still leaves a
  // usable window with the generic title.
  useEffect(() => {
    let alive = true;
    hostsApi
      .get(hostID)
      .then((h) => {
        if (alive && h) setHostName(h.name);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [hostID]);

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-background text-foreground">
      <TitleBar title={hostName ? `SFTP · ${hostName}` : t("nav.files")} />
      <div className="flex min-h-0 flex-1">
        <Sidebar items={SFTP_NAV_ITEMS} />
        <main className="relative min-w-0 flex-1 overflow-hidden">
          <View active={activeView === "sftp"}>
            <ErrorBoundary label={t("nav.files")} resetLabel={t("common.retry")}>
              <SftpWorkbench hostID={hostID} />
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
      {/* Single confirmation/prompt + toast hosts (this window's DOM is
          separate from the main window's). */}
      <ConfirmDialog />
      <Toaster />
    </div>
  );
}

/** View keeps its children mounted at all times; inactive views are hidden. */
function View({ active, children }: { active: boolean; children: React.ReactNode }) {
  return (
    <div className={active ? "h-full w-full" : "hidden h-full w-full"}>{children}</div>
  );
}

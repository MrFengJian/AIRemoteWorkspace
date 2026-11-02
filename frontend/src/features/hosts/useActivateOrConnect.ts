import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useOpenTerminal } from "@/features/hosts/hooks";

/** Minimal host shape needed to (re)connect a terminal tab. */
export type ActivatableHost = {
  id: string;
  name: string;
  terminalTheme?: string;
  terminalFont?: string;
  terminalFontSize?: number;
};

/**
 * Shared HostsSidebar click behaviour: focus the host's live tab, or connect
 * a new one. Extracted so the apps page can jump from "install app → pick
 * host" straight into that host's session (the install itself runs in the
 * session's right-panel apps view).
 */
export function useActivateOrConnect() {
  const sessions = useTerminalStore((s) => s.sessions);
  const setActive = useTerminalStore((s) => s.setActive);
  const openTerminal = useOpenTerminal();

  const activateOrConnect = (host: ActivatableHost) => {
    const live = sessions.filter(
      (s) => s.hostID === host.id && (s.status === "connected" || s.status === "connecting"),
    );
    const latest = live.length > 0 ? live[live.length - 1] : null;
    if (latest) {
      setActive(latest.id);
      return;
    }
    openTerminal
      .mutateAsync({
        host: {
          id: host.id,
          name: host.name,
          terminalTheme: host.terminalTheme,
          terminalFont: host.terminalFont,
          terminalFontSize: host.terminalFontSize,
        },
        creds: {},
      })
      .catch(() => {
        // Failures surface in the ConnectOverlay dialog (reason + retry +
        // edit-host); nothing to do here.
      });
  };

  return { activateOrConnect };
}

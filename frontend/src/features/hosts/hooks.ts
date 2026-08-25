import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { hostsApi, type HostInputDTO, type CredentialsDTO } from "@/features/hosts/api";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import {
  ConfigService,
  TerminalService,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

/** Query key for the hosts list. */
export const HOSTS_KEY = ["hosts"] as const;

/**
 * Global terminal appearance defaults (AppConfig). Per-host values of "" / 0
 * fall back to these; a failure to load the config falls back to the
 * built-in defaults ("" / 0), same as before the fields existed.
 */
async function globalTerminalDefaults(): Promise<{
  terminalTheme: string;
  terminalFont: string;
  terminalFontSize: number;
}> {
  try {
    const cfg = await ConfigService.GetAppConfig();
    return {
      terminalTheme: cfg.terminalTheme ?? "",
      terminalFont: cfg.terminalFont ?? "",
      terminalFontSize: cfg.terminalFontSize ?? 0,
    };
  } catch {
    return { terminalTheme: "", terminalFont: "", terminalFontSize: 0 };
  }
}

/** useHosts subscribes to the host list via TanStack Query. */
export function useHosts() {
  return useQuery({
    queryKey: HOSTS_KEY,
    queryFn: () => hostsApi.list(),
  });
}

/** useCreateHost mutates and refreshes the list. */
export function useCreateHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: HostInputDTO) => hostsApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: HOSTS_KEY }),
  });
}

/** useUpdateHost mutates and refreshes the list. */
export function useUpdateHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: HostInputDTO }) =>
      hostsApi.update(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: HOSTS_KEY }),
  });
}

/** useDeleteHost mutates and refreshes the list. */
export function useDeleteHost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => hostsApi.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: HOSTS_KEY }),
  });
}

/** useTestConnection runs a one-shot connection check. */
export function useTestConnection() {
  return useMutation({
    mutationFn: ({ hostId, creds }: { hostId: string; creds: CredentialsDTO }) =>
      hostsApi.testConnection(hostId, creds),
  });
}

/**
 * useOpenTerminal connects to a host and registers a new terminal session.
 * The session id is stored in the terminal feature's store; TerminalView
 * mounts xterm.js against it.
 */
export function useOpenTerminal() {
  const addSession = useTerminalStore((s) => s.addSession);

  return useMutation({
    mutationFn: async ({
      host,
      creds,
    }: {
      host: {
        id: string;
        name: string;
        terminalTheme?: string;
        terminalFont?: string;
        terminalFontSize?: number;
      };
      creds: CredentialsDTO;
    }) => {
      const res = await TerminalService.OpenSession({
        hostId: host.id,
        creds,
        size: { cols: 80, rows: 24 },
      });
      // Snapshot the appearance at open time: per-host overrides, else the
      // global terminal defaults, else built-ins ("" / 0 downstream).
      const defaults = await globalTerminalDefaults();
      return {
        sessionId: res.sessionId,
        hostID: host.id,
        hostName: host.name,
        terminalTheme: host.terminalTheme || defaults.terminalTheme,
        terminalFont: host.terminalFont || defaults.terminalFont,
        terminalFontSize: host.terminalFontSize || defaults.terminalFontSize,
      };
    },
    onSuccess: ({ sessionId, hostID, hostName, terminalTheme, terminalFont, terminalFontSize }) => {
      addSession(sessionId, hostID, hostName, terminalTheme, terminalFont, terminalFontSize);
    },
  });
}

/**
 * useOpenLocalTerminal starts an interactive shell on the user's machine over
 * a local PTY (no SSH) and registers it as a terminal tab.
 */
export function useOpenLocalTerminal() {
  const addLocalSession = useTerminalStore((s) => s.addLocalSession);

  return useMutation({
    // The name is the tab label; the appearance comes from the global
    // terminal defaults (the appearance dialog in a local tab persists there).
    mutationFn: async (name: string) => {
      const [res, defaults] = await Promise.all([
        TerminalService.OpenLocalSession({ cols: 80, rows: 24 }),
        globalTerminalDefaults(),
      ]);
      return {
        sessionId: res.sessionId,
        name,
        appearance: defaults,
      };
    },
    onSuccess: ({ sessionId, name, appearance }) => {
      addLocalSession(
        sessionId,
        name,
        appearance.terminalTheme,
        appearance.terminalFont,
        appearance.terminalFontSize,
      );
    },
  });
}

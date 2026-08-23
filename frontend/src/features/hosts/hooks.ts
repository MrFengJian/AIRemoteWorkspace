import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { hostsApi, type HostInputDTO, type CredentialsDTO } from "@/features/hosts/api";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

/** Query key for the hosts list. */
export const HOSTS_KEY = ["hosts"] as const;

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
      return {
        sessionId: res.sessionId,
        hostID: host.id,
        hostName: host.name,
        terminalTheme: host.terminalTheme ?? "",
        terminalFont: host.terminalFont ?? "",
        terminalFontSize: host.terminalFontSize ?? 0,
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
    mutationFn: (name: string) =>
      TerminalService.OpenLocalSession({ cols: 80, rows: 24 }).then((res) => ({
        sessionId: res.sessionId,
        name,
      })),
    onSuccess: ({ sessionId, name }) => {
      addLocalSession(sessionId, name);
    },
  });
}

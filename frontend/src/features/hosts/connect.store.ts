import { create } from "zustand";

/** Connection-establishment stages reported by the backend (in order). */
export type ConnectStage = "credentials" | "connect" | "handshake" | "session";

export const CONNECT_STAGES: ConnectStage[] = ["credentials", "connect", "handshake", "session"];

/**
 * Global state of the SSH connection overlay: one tracked attempt at a time
 * (the latest click wins if the user fires several). Progress stages arrive
 * as "terminal:connect" Wails events; the failure path keeps the overlay
 * open as an error dialog with the reason and a retry hook.
 */
interface ConnectState {
  connId: string | null;
  hostId: string | null;
  hostName: string;
  stage: ConnectStage;
  error: string | null;
  startedAt: number;
  retry: (() => void) | null;
  /** A new connection attempt begins: resets and shows the overlay. */
  begin: (v: { connId: string; hostId: string; hostName: string }) => void;
  /** Progress event for the tracked connection. */
  setStage: (connId: string, stage: ConnectStage) => void;
  /** The attempt succeeded: close the overlay (if still tracking this id). */
  succeed: (connId: string) => void;
  /** The attempt failed: keep the overlay open as an error dialog. */
  fail: (connId: string, error: string, retry: () => void) => void;
  /** Close the overlay (user dismiss or after editing the host). */
  dismiss: () => void;
}

export const useConnectStore = create<ConnectState>((set, get) => ({
  connId: null,
  hostId: null,
  hostName: "",
  stage: "credentials",
  error: null,
  startedAt: 0,
  retry: null,
  begin: ({ connId, hostId, hostName }) =>
    set({ connId, hostId, hostName, stage: "credentials", error: null, startedAt: Date.now(), retry: null }),
  setStage: (connId, stage) => {
    if (get().connId === connId && !get().error) set({ stage });
  },
  succeed: (connId) => {
    if (get().connId === connId) set({ connId: null, retry: null });
  },
  fail: (connId, error, retry) => {
    if (get().connId === connId) set({ error, retry });
  },
  dismiss: () => set({ connId: null, retry: null }),
}));

/**
 * Map a raw connection error to a friendly cause. Matches the error strings
 * produced by internal/infrastructure/ssh (dial / handshake wrappers around
 * Go's net and crypto/ssh errors).
 */
export function classifyConnectError(msg: string): { kind: string; hint: string } {
  const m = msg.toLowerCase();
  if (m.includes("unable to authenticate") || m.includes("no supported methods remain")) {
    return { kind: "auth", hint: "authFailedHint" };
  }
  if (m.includes("connection refused")) {
    return { kind: "refused", hint: "refusedHint" };
  }
  if (m.includes("i/o timeout") || m.includes("timed out") || m.includes("timeout")) {
    return { kind: "timeout", hint: "timeoutHint" };
  }
  if (m.includes("no route") || m.includes("unreachable") || m.includes("network is down")) {
    return { kind: "network", hint: "networkHint" };
  }
  if (m.includes("no such host") || m.includes("lookup ") || m.includes("dns")) {
    return { kind: "dns", hint: "dnsHint" };
  }
  if (m.includes("host key")) {
    return { kind: "hostkey", hint: "hostKeyHint" };
  }
  return { kind: "generic", hint: "genericHint" };
}

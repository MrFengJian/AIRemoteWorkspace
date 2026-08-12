import { create } from "zustand";

/**
 * Terminal feature state. Phase 1: empty scaffold — Phase 2 wires this to
 * PTY sessions (one tab per SSH shell) and xterm.js.
 */
interface TerminalState {
  openSessions: number;
}

export const useTerminalStore = create<TerminalState>(() => ({
  openSessions: 0,
}));

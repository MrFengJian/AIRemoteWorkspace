import { create } from "zustand";

/**
 * Terminal feature state. Tracks live PTY sessions (one per open tab) and the
 * active tab. Session lifecycle (open/close) is driven by HostService hooks;
 * TerminalView subscribes to each session's events.
 */
export interface TerminalSession {
  id: string;
  hostName: string;
  status: "connecting" | "connected" | "closed" | "error";
  /** Per-host terminal colour scheme id (from the host config). "" = default. */
  terminalTheme: string;
}

interface TerminalState {
  sessions: TerminalSession[];
  activeId: string | null;
  /** Global fallback terminal colour scheme (used when a host has none set). */
  themeId: string;

  addSession: (id: string, hostName: string, terminalTheme: string) => void;
  setSessionStatus: (id: string, status: TerminalSession["status"]) => void;
  removeSession: (id: string) => void;
  setActive: (id: string) => void;
  setThemeId: (id: string) => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  activeId: null,
  themeId: "cobalt2",

  addSession: (id, hostName, terminalTheme) =>
    set((s) => ({
      sessions: [
        ...s.sessions,
        { id, hostName, status: "connecting" as const, terminalTheme },
      ],
      activeId: id,
    })),

  setSessionStatus: (id, status) =>
    set((s) => ({
      sessions: s.sessions.map((sess) =>
        sess.id === id ? { ...sess, status } : sess,
      ),
    })),

  removeSession: (id) =>
    set((s) => {
      const sessions = s.sessions.filter((sess) => sess.id !== id);
      const activeId =
        s.activeId === id ? (sessions[0]?.id ?? null) : s.activeId;
      return { sessions, activeId };
    }),

  setActive: (id) => set({ activeId: id }),

  setThemeId: (id) => set({ themeId: id }),
}));

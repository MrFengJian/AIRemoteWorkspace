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
}

interface TerminalState {
  sessions: TerminalSession[];
  activeId: string | null;

  addSession: (id: string, hostName: string) => void;
  setSessionStatus: (id: string, status: TerminalSession["status"]) => void;
  removeSession: (id: string) => void;
  setActive: (id: string) => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  activeId: null,

  addSession: (id, hostName) =>
    set((s) => ({
      sessions: [
        ...s.sessions,
        { id, hostName, status: "connecting" as const },
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
}));

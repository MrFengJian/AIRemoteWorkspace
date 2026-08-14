import { create } from "zustand";

/**
 * Terminal feature state. Tracks live PTY sessions (one per open tab) and the
 * active tab. Session lifecycle (open/close) is driven by HostService hooks;
 * TerminalView subscribes to each session's events.
 */
export interface TerminalSession {
  id: string; // tab identifier (also the first pane's backend session ID)
  hostID: string;
  hostName: string;
  status: "connecting" | "connected" | "closed" | "error";
  /** Per-host terminal colour scheme id (from the host config). "" = default. */
  terminalTheme: string;
  /** Per-host terminal font overrides. "" / 0 = follow the global settings. */
  terminalFont: string;
  terminalFontSize: number;
  /** All pane backend session IDs in this tab (flat list, no hierarchy).
   *  paneIds[0] === id. Length > 1 means split-screen is active. */
  paneIds: string[];
  /** Split direction (only meaningful when paneIds.length > 1). */
  splitDirection: "horizontal" | "vertical" | null;
  /** Agent model selection for this session (provider id + model). Set via the
   *  picker dialog when the agent is enabled; "" means not chosen yet. */
  agentProviderId?: string;
  agentModel?: string;
}

interface TerminalState {
  sessions: TerminalSession[];
  activeId: string | null;

  addSession: (id: string, hostID: string, hostName: string, terminalTheme: string, terminalFont: string, terminalFontSize: number) => void;
  setSessionStatus: (id: string, status: TerminalSession["status"]) => void;
  removeSession: (id: string) => void;
  removeSessions: (ids: string[]) => void;
  clearSessions: () => void;
  setActive: (id: string) => void;
  /** Add a new pane (backend session ID) to a tab's split. */
  addPane: (sessionId: string, paneId: string, direction: "horizontal" | "vertical") => void;
  /** Remove a pane from a tab. If no panes remain, the tab is removed too. */
  removePane: (sessionId: string, paneId: string) => void;
  /** Set the agent provider + model used by a session's chats. */
  setAgentModel: (sessionId: string, providerId: string, model: string) => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  activeId: null,

  addSession: (id, hostID, hostName, terminalTheme, terminalFont, terminalFontSize) =>
    set((s) => ({
      sessions: [
        ...s.sessions,
        { id, hostID, hostName, status: "connecting" as const, terminalTheme, terminalFont, terminalFontSize, paneIds: [id], splitDirection: null },
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

  removeSessions: (ids) =>
    set((s) => {
      const idSet = new Set(ids);
      const sessions = s.sessions.filter((sess) => !idSet.has(sess.id));
      const activeId = idSet.has(s.activeId ?? "")
        ? (sessions[0]?.id ?? null)
        : s.activeId;
      return { sessions, activeId };
    }),

  clearSessions: () => set({ sessions: [], activeId: null }),

  setActive: (id) => set({ activeId: id }),

  addPane: (sessionId, paneId, direction) =>
    set((s) => ({
      sessions: s.sessions.map((sess) =>
        sess.id === sessionId
          ? {
              ...sess,
              paneIds: [...sess.paneIds, paneId],
              splitDirection: direction,
            }
          : sess,
      ),
    })),

  removePane: (sessionId, paneId) =>
    set((s) => {
      const sessions = s.sessions.map((sess) => {
        if (sess.id !== sessionId) return sess;
        const paneIds = sess.paneIds.filter((id) => id !== paneId);
        if (paneIds.length === 0) return null; // signal removal
        return {
          ...sess,
          paneIds,
          splitDirection: paneIds.length <= 1 ? null : sess.splitDirection,
        };
      }).filter(Boolean) as TerminalSession[];
      const activeId = s.activeId === sessionId && !sessions.find((x) => x.id === sessionId)
        ? (sessions[0]?.id ?? null)
        : s.activeId;
      return { sessions, activeId };
    }),

  setAgentModel: (sessionId, providerId, model) =>
    set((s) => ({
      sessions: s.sessions.map((sess) =>
        sess.id === sessionId
          ? { ...sess, agentProviderId: providerId, agentModel: model }
          : sess,
      ),
    })),
}));

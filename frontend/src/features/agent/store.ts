import { create } from "zustand";

/**
 * Agent feature state. Per-session message history + streaming state + pending
 * approval requests. The activeSessionId ties the agent to a terminal tab.
 */

export interface ChatMessage {
  role: "user" | "assistant" | "tool";
  content: string;
  toolName?: string;
  toolArgs?: string;
}

export interface PendingApproval {
  reqId: string;
  sessionId: string;
  toolName: string;
  permission: string;
  args: string;
}

interface AgentState {
  /** Messages keyed by sessionID. */
  histories: Record<string, ChatMessage[]>;
  /** Sessions currently streaming a response. */
  streaming: Record<string, boolean>;
  /** Pending approval requests (only one at a time per session). */
  approvals: PendingApproval[];
  /** Whether the LLM is configured (has API key). */
  llmConfigured: boolean;

  addMessage: (sessionID: string, msg: ChatMessage) => void;
  appendToLast: (sessionID: string, text: string) => void;
  setStreaming: (sessionID: string, v: boolean) => void;
  addApproval: (a: PendingApproval) => void;
  removeApproval: (reqId: string) => void;
  setLLMConfigured: (v: boolean) => void;
  clearHistory: (sessionID: string) => void;
}

export const useAgentStore = create<AgentState>((set) => ({
  histories: {},
  streaming: {},
  approvals: [],
  llmConfigured: false,

  addMessage: (sessionID, msg) =>
    set((s) => ({
      histories: {
        ...s.histories,
        [sessionID]: [...(s.histories[sessionID] ?? []), msg],
      },
    })),

  appendToLast: (sessionID, text) =>
    set((s) => {
      const hist = s.histories[sessionID] ?? [];
      if (hist.length === 0) return s;
      const last = hist[hist.length - 1];
      // Append to the last assistant message (streaming) or create one.
      if (last.role === "assistant") {
        const updated = [...hist];
        updated[updated.length - 1] = { ...last, content: last.content + text };
        return { histories: { ...s.histories, [sessionID]: updated } };
      }
      return {
        histories: {
          ...s.histories,
          [sessionID]: [...hist, { role: "assistant", content: text }],
        },
      };
    }),

  setStreaming: (sessionID, v) =>
    set((s) => ({ streaming: { ...s.streaming, [sessionID]: v } })),

  addApproval: (a) => set((s) => ({ approvals: [...s.approvals, a] })),
  removeApproval: (reqId) =>
    set((s) => ({ approvals: s.approvals.filter((a) => a.reqId !== reqId) })),

  setLLMConfigured: (v) => set({ llmConfigured: v }),

  clearHistory: (sessionID) =>
    set((s) => {
      const h = { ...s.histories };
      delete h[sessionID];
      return { histories: h };
    }),
}));

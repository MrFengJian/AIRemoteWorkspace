import { create } from "zustand";

/**
 * Agent feature state. Per-session message history + streaming state.
 * The activeSessionId ties the agent to a terminal tab. Approval requests
 * live in their own global store (approval.store.ts) rendered by ApprovalHost.
 */
export interface ChatMessage {
  /** Stable identity for list keys — survives streaming appends/reorders. */
  id: number;
  role: "user" | "assistant" | "tool";
  /** Assistant-only presentation hint: errors (retryable) vs user cancels. */
  variant?: "error" | "cancelled";
  content: string;
  toolName?: string;
  toolArgs?: string;
  /** Tool-call correlation id — the start and end events share it. */
  callId?: string;
  /** True while the tool is executing; cleared when its result lands. */
  running?: boolean;
}

interface AgentState {
  /** Messages keyed by sessionID. */
  histories: Record<string, ChatMessage[]>;
  /** Sessions currently streaming a response. */
  streaming: Record<string, boolean>;

  addMessage: (sessionID: string, msg: Omit<ChatMessage, "id">) => void;
  appendToLast: (sessionID: string, text: string) => void;
  setStreaming: (sessionID: string, v: boolean) => void;
  /** Attach a tool execution result to its step (matched by call id; an empty
   *  callId targets the newest still-running step). */
  setToolResult: (sessionID: string, callId: string, result: string) => void;
  /** Mark every still-running tool step finished (chat ended). */
  finishToolSteps: (sessionID: string) => void;
  /** Remove the trailing assistant message if it is still empty (stream died
   *  before any text arrived — avoids a stray empty bubble above the error). */
  dropTrailingEmptyAssistant: (sessionID: string) => void;
  /** Remove the trailing error/cancelled notice (before a retry resends). */
  dropTrailingNotice: (sessionID: string) => void;
  clearHistory: (sessionID: string) => void;
}

/** Monotonic message id source — stable React keys across streaming updates. */
let nextMessageId = 1;

export const useAgentStore = create<AgentState>((set) => ({
  histories: {},
  streaming: {},

  addMessage: (sessionID, msg) =>
    set((s) => ({
      histories: {
        ...s.histories,
        [sessionID]: [...(s.histories[sessionID] ?? []), { ...msg, id: nextMessageId++ }],
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
          [sessionID]: [...hist, { id: nextMessageId++, role: "assistant", content: text }],
        },
      };
    }),

  setStreaming: (sessionID, v) => set((s) => ({ streaming: { ...s.streaming, [sessionID]: v } })),

  setToolResult: (sessionID, callId, result) =>
    set((s) => {
      const hist = s.histories[sessionID] ?? [];
      // Search from the end: the matching step is the most recent one with
      // this call id that hasn't received a result yet (or, for an empty
      // callId, the most recent running step).
      for (let i = hist.length - 1; i >= 0; i--) {
        const m = hist[i];
        if (m.role !== "tool") continue;
        if (m.content && !m.running) continue;
        if (callId && m.callId !== callId) continue;
        const updated = [...hist];
        updated[i] = { ...m, content: result, running: false };
        return { histories: { ...s.histories, [sessionID]: updated } };
      }
      return s;
    }),

  finishToolSteps: (sessionID) =>
    set((s) => {
      const hist = s.histories[sessionID];
      if (!hist?.some((m) => m.role === "tool" && m.running)) return s;
      return {
        histories: {
          ...s.histories,
          [sessionID]: hist.map((m) => (m.running ? { ...m, running: false } : m)),
        },
      };
    }),

  dropTrailingEmptyAssistant: (sessionID) =>
    set((s) => {
      const hist = s.histories[sessionID] ?? [];
      if (hist.length === 0) return s;
      const last = hist[hist.length - 1];
      if (last.role === "assistant" && last.content === "" && !last.variant) {
        return { histories: { ...s.histories, [sessionID]: hist.slice(0, -1) } };
      }
      return s;
    }),

  dropTrailingNotice: (sessionID) =>
    set((s) => {
      const hist = s.histories[sessionID] ?? [];
      if (hist.length === 0) return s;
      const last = hist[hist.length - 1];
      if (last.role === "assistant" && last.variant) {
        return { histories: { ...s.histories, [sessionID]: hist.slice(0, -1) } };
      }
      return s;
    }),

  clearHistory: (sessionID) =>
    set((s) => {
      const h = { ...s.histories };
      delete h[sessionID];
      return { histories: h };
    }),
}));

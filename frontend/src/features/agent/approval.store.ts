import { create } from "zustand";

/**
 * Global approval queue for agent tool calls.
 *
 * Lives outside AgentView so approvals keep working even when the agent panel
 * is closed or switched to the SFTP tab — the ApprovalHost in AppShell owns
 * the subscription and the dialog.
 */
export interface PendingApproval {
  reqId: string;
  sessionId: string;
  toolName: string;
  permission: string;
  args: string;
}

interface ApprovalState {
  queue: PendingApproval[];
  add: (a: PendingApproval) => void;
  remove: (reqId: string) => void;
}

export const useApprovalStore = create<ApprovalState>((set) => ({
  queue: [],

  add: (a) =>
    set((s) =>
      // Dedupe by reqId (event replays must not double-queue).
      s.queue.some((x) => x.reqId === a.reqId)
        ? s
        : { queue: [...s.queue, a] },
    ),

  remove: (reqId) =>
    set((s) => ({ queue: s.queue.filter((x) => x.reqId !== reqId) })),
}));

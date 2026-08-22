// Agent feature API — typed wrappers over the generated AgentService bindings.

import { AgentService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export const agentApi = {
  /** Start a streaming chat on a session using the selected provider + model. */
  startChat: (sessionID: string, providerID: string, model: string, message: string) =>
    AgentService.StartChat(sessionID, providerID, model, message),
  cancelChat: (sessionID: string) => AgentService.CancelChat(sessionID),
  /** Forget the backend's conversation memory for a session (multi-turn replay). */
  clearHistory: (sessionID: string) => AgentService.ClearHistory(sessionID),
  approveToolCall: (reqID: string, approved: boolean) =>
    AgentService.ApproveToolCall(reqID, approved),
};

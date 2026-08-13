// Agent feature API — typed wrappers over the generated AgentService bindings.

import {
  AgentService,
  type LLMConfigDTO,
  type SetLLMConfigInput,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type { LLMConfigDTO };

export const agentApi = {
  getLLMConfig: () => AgentService.GetLLMConfig(),
  setLLMConfig: (input: SetLLMConfigInput) => AgentService.SetLLMConfig(input),
  startChat: (sessionID: string, message: string) =>
    AgentService.StartChat(sessionID, message),
  cancelChat: (sessionID: string) => AgentService.CancelChat(sessionID),
  approveToolCall: (reqID: string, approved: boolean) =>
    AgentService.ApproveToolCall(reqID, approved),
};

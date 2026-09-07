// Agent feature API — typed wrappers over the generated AgentService bindings.

import {
  AgentService,
  type ConversationDTO,
  type ConversationMessageDTO,
  type SkillDTO,
  type ContextPathDTO,
  type ScenarioDraftDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";

export type {
  ConversationDTO,
  ConversationMessageDTO,
  SkillDTO,
  ContextPathDTO,
  ScenarioDraftDTO,
};

export const agentApi = {
  /** Start a streaming chat on a session using the selected provider + model. */
  startChat: (sessionID: string, providerID: string, model: string, message: string) =>
    AgentService.StartChat(sessionID, providerID, model, message),
  /** Diagnosis-mode chat: triage prompt + auto-collected health snapshot. */
  startDiagnosis: (sessionID: string, providerID: string, model: string, symptom: string) =>
    AgentService.StartDiagnosis(sessionID, providerID, model, symptom),
  cancelChat: (sessionID: string) => AgentService.CancelChat(sessionID),
  /** Forget the backend's conversation memory for a session (multi-turn replay). */
  clearHistory: (sessionID: string) => AgentService.ClearHistory(sessionID),
  approveToolCall: (reqID: string, approved: boolean) =>
    AgentService.ApproveToolCall(reqID, approved),
  /** Set a session's approval policy ("strict" | "auto_write"). */
  setSessionPolicy: (sessionID: string, policy: string) =>
    AgentService.SetSessionPolicy(sessionID, policy),
  /** Skill metadata for the input-box `/` picker and the scenario manager. */
  listSkills: () => AgentService.ListSkills().then((r) => r ?? []),
  /** One skill with its markdown body (scenario editor). */
  getSkill: (name: string) => AgentService.GetSkill(name),
  /** Create or overwrite a skill's SKILL.md. */
  saveSkill: (name: string, content: string) => AgentService.SaveSkill(name, content),
  /** Delete a skill (builtins stay dismissed across restarts). */
  deleteSkill: (name: string) => AgentService.DeleteSkill(name),
  /** Distill a conversation into a SKILL.md draft (preview before saving). */
  draftScenario: (conversationID: string, providerID: string, model: string) =>
    AgentService.DraftScenario(conversationID, providerID, model),
  /** Directory listing for the @-completion popup. */
  listContextPaths: (sessionID: string, dir: string) =>
    AgentService.ListContextPaths(sessionID, dir).then((r) => r ?? []),
  /** Persisted conversation history (newest first; filter by host client-side).
   *  The generated bindings mark slice returns nullable; normalize to []. */
  listConversations: () => AgentService.ListConversations().then((r) => r ?? []),
  getConversationMessages: (conversationID: string) =>
    AgentService.GetConversationMessages(conversationID).then((r) => r ?? []),
  /** Point a session at a persisted conversation and restore its memory. */
  resumeConversation: (sessionID: string, conversationID: string) =>
    AgentService.ResumeConversation(sessionID, conversationID),
  deleteConversation: (conversationID: string) =>
    AgentService.DeleteConversation(conversationID),
};

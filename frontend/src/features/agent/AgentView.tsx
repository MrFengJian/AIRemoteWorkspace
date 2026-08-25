import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  Send,
  Square,
  Cpu,
  Settings2,
  Loader2,
  Wrench,
  Check,
  ChevronRight,
  ArrowDown,
  Copy as CopyIcon,
  RotateCcw,
  Trash2,
  Scissors,
  History as HistoryIcon,
  MessageSquarePlus,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { agentApi, type ConversationDTO } from "@/features/agent/api";
import { useAgentStore, type ChatMessage } from "@/features/agent/store";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useModelProviders } from "@/features/settings/hooks";
import { useHosts } from "@/features/hosts/hooks";
import { AgentMarkdown } from "@/features/agent/AgentMarkdown";
import { HostService, TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useUIStore } from "@/stores/ui.store";
import { decodeBase64, encodeBase64 } from "@/lib/base64";
import { useConfirm } from "@/lib/useConfirm";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

/** Input-history cap (↑ recall of previously sent messages). */
const MAX_INPUT_HISTORY = 50

/** Query key for the persisted conversation list. */
const CONVERSATIONS_KEY = ["agent-conversations"] as const;

/**
 * AI Agent view. Bound to the active terminal session — the agent operates on
 * that session's connected host. Streams LLM responses, shows tool calls
 * inline, and gates WRITE/DANGEROUS tools behind the global ApprovalHost
 * (mounted in AppShell, works even when this panel is closed).
 */
interface AgentViewProps {
  embeddedSessionID?: string;
  embeddedSessionName?: string;
}

export function AgentView({ embeddedSessionID, embeddedSessionName }: AgentViewProps = {}) {
  const { t } = useTranslation();
  const storeActiveId = useTerminalStore((s) => s.activeId);
  const sessions = useTerminalStore((s) => s.sessions);
  const setAgentModel = useTerminalStore((s) => s.setAgentModel);
  const setView = useUIStore((s) => s.setView);
  const setSettingsCategory = useUIStore((s) => s.setSettingsCategory);
  const { askConfirm } = useConfirm();
  // Embedded mode overrides the global active session.
  const activeSessionId = embeddedSessionID ?? storeActiveId;

  const {
    histories,
    streaming,
    activeConvBySession,
    addMessage,
    setActiveConv,
    appendToLast,
    setStreaming,
    setToolResult,
    finishToolSteps,
    dropTrailingEmptyAssistant,
    dropTrailingNotice,
    clearHistory,
  } = useAgentStore();
  const queryClient = useQueryClient();

  const [input, setInput] = useState("");
  const [inputHistory, setInputHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState<number | null>(null);
  // Conversation-history panel: open state + host filter scope.
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyAllHosts, setHistoryAllHosts] = useState(false);
  const historyPanelRef = useRef<HTMLDivElement | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  // Follow-scroll flag: only auto-scroll while the user is at the bottom.
  const atBottomRef = useRef(true);
  const [showJumpDown, setShowJumpDown] = useState(false);

  // Shared provider query (same cache as Settings → Models), filtered to
  // enabled providers for the inline selector.
  const { data: allProviders, isLoading: providersLoading } = useModelProviders();
  const providers = useMemo(
    () => (allProviders ?? []).filter((p) => p.enabled),
    [allProviders],
  );
  const providersLoaded = !providersLoading;

  const sessionName = embeddedSessionName
    ?? (activeSessionId
      ? sessions.find((s) => s.id === activeSessionId)?.hostName ?? "session"
      : null);

  // The session's agent model selection (set via the inline selector).
  const session = activeSessionId
    ? sessions.find((s) => s.id === activeSessionId)
    : undefined;
  const agentProviderId = session?.agentProviderId ?? "";
  const agentModel = session?.agentModel ?? "";
  const modelChosen = !!agentProviderId && !!agentModel;
  const currentProvider = providers.find((p) => p.id === agentProviderId);
  // Hosts query — used to seed the session's selection from the host's saved
  // last-used preference (and to persist changes back to it).
  const { data: hosts } = useHosts();

  // Persist the selection as the host's hidden last-used preference so the
  // next agent session on this host starts where the last one left off.
  // Best-effort: a failed write never blocks chatting.
  const persistModel = (providerId: string, model: string) => {
    const hostID = session?.hostID;
    if (!hostID) return;
    HostService.SetAgentModel(hostID, providerId, model).catch(() => {});
  };

  // Seed the session's selection from the host's saved preference once, when
  // the session has no selection yet and the hosts query has loaded.
  // agentProviderId === undefined marks an unseeded session; "" means seeded
  // with no usable preference.
  useEffect(() => {
    if (!activeSessionId || !session || session.agentProviderId !== undefined || !hosts) return;
    const host = hosts.find((h) => h.id === session.hostID);
    setAgentModel(activeSessionId, host?.agentProviderId ?? "", host?.agentModel ?? "");
  }, [activeSessionId, session, hosts, setAgentModel]);

  // Default/self-heal the session's selection: pick the first available
  // provider when none is set, or when the stored one was deleted or disabled.
  // Manual model ids (provider without a fetched model list) are trusted and
  // never clobbered. Waits for the host-preference seeding above.
  useEffect(() => {
    if (!providersLoaded || !activeSessionId || providers.length === 0) return;
    if (session?.agentProviderId === undefined) return;
    const cur = providers.find((p) => p.id === agentProviderId);
    if (cur && agentModel) {
      // Trust a valid selection, and equally a hand-typed model id on a
      // provider that has no fetched model list.
      if ((cur.models?.length ?? 0) === 0) return;
      if (cur.models?.includes(agentModel)) return;
    }
    const target =
      (cur && (cur.models?.length ?? 0) > 0
        ? cur
        : providers.find((p) => (p.models?.length ?? 0) > 0)) ?? providers[0];
    const model = target.models?.[0] ?? "";
    setAgentModel(activeSessionId, target.id, model);
    persistModel(target.id, model);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providersLoaded, providers, activeSessionId, agentProviderId, agentModel]);

  // Point the user at the global model settings when nothing is usable.
  const goSettings = () => {
    setSettingsCategory("models");
    setView("settings");
  };

  const handleProviderChange = (id: string) => {
    if (!activeSessionId) return;
    const p = providers.find((x) => x.id === id);
    const model = p?.models?.[0] ?? "";
    setAgentModel(activeSessionId, id, model);
    persistModel(id, model);
  };

  const handleModelChange = (model: string) => {
    if (!activeSessionId) return;
    setAgentModel(activeSessionId, agentProviderId, model);
    persistModel(agentProviderId, model);
  };

  // Persisted conversation history (fetched when the panel opens).
  const { data: conversations, isLoading: convsLoading } = useQuery({
    queryKey: CONVERSATIONS_KEY,
    queryFn: () => agentApi.listConversations(),
    enabled: historyOpen,
  });

  // Close the history panel on outside clicks.
  useEffect(() => {
    if (!historyOpen) return;
    const onDown = (e: MouseEvent) => {
      if (historyPanelRef.current && !historyPanelRef.current.contains(e.target as Node)) {
        setHistoryOpen(false);
      }
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [historyOpen]);

  // ── Smart scroll: follow only while the user is at the bottom ─────────
  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    atBottomRef.current = near;
    setShowJumpDown(!near);
  };

  const jumpToBottom = () => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    atBottomRef.current = true;
    setShowJumpDown(false);
  };

  useEffect(() => {
    if (atBottomRef.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [histories, activeSessionId]);

  // Auto-grow the textarea with its content (up to ~8 lines).
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [input]);

  // Subscribe to agent events for the active session.
  useEffect(() => {
    if (!activeSessionId) return;
    const sid = activeSessionId;

    const chunkCancel = Events.On(`agent:${sid}:chunk`, (e: unknown) => {
      const data = (e as { data?: unknown }).data;
      if (typeof data === "string") {
        try {
          appendToLast(sid, decodeBase64(data));
        } catch {
          appendToLast(sid, data);
        }
      }
    });

    const toolCancel = Events.On(`agent:${sid}:toolcall`, (e: unknown) => {
      const data = (e as { data?: unknown }).data as
        | { id?: string; tool?: string; args?: string }
        | undefined;
      if (data?.tool) {
        addMessage(sid, {
          role: "tool",
          content: "",
          callId: data.id ?? "",
          toolName: data.tool,
          toolArgs: data.args,
          running: true,
        });
      }
    });

    const toolEndCancel = Events.On(`agent:${sid}:toolend`, (e: unknown) => {
      const data = (e as { data?: unknown }).data as
        | { id?: string; result?: string }
        | undefined;
      setToolResult(sid, data?.id ?? "", data?.result ?? "");
    });

    const doneCancel = Events.On(`agent:${sid}:done`, () => {
      setStreaming(sid, false);
      // Safety net: any step whose end event was missed stops spinning.
      finishToolSteps(sid);
    });

    const errorCancel = Events.On(`agent:${sid}:error`, (e: unknown) => {
      const data = (e as { data?: unknown }).data;
      setStreaming(sid, false);
      finishToolSteps(sid);
      // A user-initiated stop is not an error — render it as a quiet notice.
      const cancelled = typeof data === "string" && data === "cancelled";
      dropTrailingEmptyAssistant(sid);
      addMessage(sid, {
        role: "assistant",
        variant: cancelled ? "cancelled" : "error",
        content: cancelled ? t("agent.cancelled") : `${t("agent.errorPrefix")} ${typeof data === "string" ? data : "error"}`,
      });
    });

    return () => {
      if (typeof chunkCancel === "function") chunkCancel();
      if (typeof toolCancel === "function") toolCancel();
      if (typeof toolEndCancel === "function") toolEndCancel();
      if (typeof doneCancel === "function") doneCancel();
      if (typeof errorCancel === "function") errorCancel();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId]);

  // ── Sending ──────────────────────────────────────────────────────────

  const send = async (text: string) => {
    if (!activeSessionId || !text.trim() || streaming[activeSessionId]) return;
    if (!modelChosen) return;
    setInputHistory((h) => [...h.slice(-MAX_INPUT_HISTORY + 1), text.trim()]);
    setHistoryIdx(null);
    setInput("");
    addMessage(activeSessionId, { role: "user", content: text.trim() });
    setStreaming(activeSessionId, true);
    // Seed an empty assistant message for streaming append.
    addMessage(activeSessionId, { role: "assistant", content: "" });
    atBottomRef.current = true;
    try {
      await agentApi.startChat(activeSessionId, agentProviderId, agentModel, text.trim());
    } catch (e) {
      setStreaming(activeSessionId, false);
      dropTrailingEmptyAssistant(activeSessionId);
      addMessage(activeSessionId, {
        role: "assistant",
        variant: "error",
        content: `${t("agent.errorPrefix")} ${e instanceof Error ? e.message : String(e)}`,
      });
    }
  };

  const handleSend = () => send(input);

  /** Retry: drop the trailing notice and resend the last user message. */
  const handleRetry = () => {
    if (!activeSessionId) return;
    const hist = histories[activeSessionId] ?? [];
    for (let i = hist.length - 1; i >= 0; i--) {
      if (hist[i].role === "user") {
        dropTrailingNotice(activeSessionId);
        send(hist[i].content);
        return;
      }
    }
  };

  const handleCancel = () => {
    if (activeSessionId) agentApi.cancelChat(activeSessionId);
  };

  /** New conversation: detach from the current one. Non-destructive — every
   *  completed turn is already persisted, so the previous conversation stays
   *  in the history panel and remains resumable; the next turn simply starts
   *  a fresh conversation. No confirm needed (that belongs to deletion). */
  const handleNewChat = () => {
    if (!activeSessionId || streaming[activeSessionId]) return;
    clearHistory(activeSessionId);
    setActiveConv(activeSessionId, null);
    agentApi.clearHistory(activeSessionId).catch(() => {});
    queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY }).catch(() => {});
    setHistoryOpen(false);
  };

  /** Resume a persisted conversation into this session (display + memory). */
  const handleResume = async (conv: ConversationDTO) => {
    if (!activeSessionId || streaming[activeSessionId]) return;
    try {
      const [msgs] = await Promise.all([
        agentApi.getConversationMessages(conv.id),
        agentApi.resumeConversation(activeSessionId, conv.id),
      ]);
      clearHistory(activeSessionId);
      for (const m of msgs) {
        addMessage(activeSessionId, {
          role: m.role === "user" ? "user" : "assistant",
          content: m.content,
        });
      }
      setActiveConv(activeSessionId, conv.id);
      atBottomRef.current = true;
      setHistoryOpen(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  /** Delete a persisted conversation (backend clears active context if needed). */
  const handleDeleteConv = async (conv: ConversationDTO) => {
    const ok = await askConfirm({
      title: t("agent.deleteConvTitle"),
      message: t("agent.deleteConvConfirm", { title: conv.title || conv.hostName }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await agentApi.deleteConversation(conv.id);
      if (activeSessionId && activeConvBySession[activeSessionId] === conv.id) {
        clearHistory(activeSessionId);
        setActiveConv(activeSessionId, null);
      }
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_KEY }).catch(() => {});
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  // handleInsert sends a code/command block to the active terminal's stdin.
  // The text appears in the terminal as if the user typed it — NO trailing
  // newline is added, so nothing executes until the user presses Enter. This
  // lets the user review the inserted content and decide whether to run it.
  // Stable identity keeps memoized message bubbles from re-rendering.
  const handleInsert = useCallback(
    (code: string) => {
      if (!activeSessionId) return;
      TerminalService.WriteStdin(activeSessionId, encodeBase64(code)).catch(() => {
        /* session may have closed */
      });
    },
    [activeSessionId],
  );

  const messages = activeSessionId ? histories[activeSessionId] ?? [] : [];
  const isStreaming = activeSessionId ? streaming[activeSessionId] : false;

  // No active session.
  if (!activeSessionId || !sessionName) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <Bot className="h-10 w-10 text-muted-foreground" />
        <div>
          <h2 className="text-lg font-medium text-foreground">{t("agent.title")}</h2>
          <p className="mt-1 max-w-sm text-sm text-muted-foreground">
            {t("agent.noSession")}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border bg-card px-4 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Bot className="h-4 w-4 shrink-0 text-primary" />
          <span className="truncate text-sm font-medium">{t("agent.title")} · {sessionName}</span>
          {isStreaming && (
            <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-primary" />
          )}
        </div>
        <div className="relative flex shrink-0 items-center gap-0.5" ref={historyPanelRef}>
          <button
            type="button"
            onClick={() => setHistoryOpen((v) => !v)}
            aria-label={t("agent.history")}
            title={t("agent.history")}
            className={cn(
              "rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              historyOpen && "bg-accent text-foreground",
            )}
          >
            <HistoryIcon className="h-3.5 w-3.5" />
          </button>
          {messages.length > 0 && (
            <button
              type="button"
              onClick={handleNewChat}
              aria-label={t("agent.newChat")}
              title={t("agent.newChat")}
              className="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <MessageSquarePlus className="h-3.5 w-3.5" />
            </button>
          )}

          {/* Conversation history panel */}
          {historyOpen && (
            <div className="absolute right-0 top-full z-30 mt-1.5 w-80 rounded-[var(--radius)] border border-border bg-popover shadow-lg">
              <div className="flex items-center justify-between border-b border-border px-3 py-2">
                <span className="text-xs font-semibold text-foreground">{t("agent.history")}</span>
                <div className="flex items-center gap-1 text-[11px]">
                  <button
                    type="button"
                    onClick={() => setHistoryAllHosts(false)}
                    className={cn(
                      "rounded px-1.5 py-0.5 transition-colors",
                      !historyAllHosts ? "bg-accent text-foreground" : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {t("agent.historyCurrent")}
                  </button>
                  <button
                    type="button"
                    onClick={() => setHistoryAllHosts(true)}
                    className={cn(
                      "rounded px-1.5 py-0.5 transition-colors",
                      historyAllHosts ? "bg-accent text-foreground" : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {t("agent.historyAll")}
                  </button>
                </div>
              </div>
              <div className="max-h-72 overflow-auto p-1.5">
                {convsLoading ? (
                  <div className="flex items-center justify-center py-6 text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                  </div>
                ) : !conversations || conversations.length === 0 ? (
                  <p className="px-2 py-6 text-center text-xs text-muted-foreground">
                    {t("agent.historyEmpty")}
                  </p>
                ) : (
                  conversations
                    .filter((c) => historyAllHosts || c.hostId === (session?.hostID ?? ""))
                    .filter((c) => c.messageCount > 0)
                    .map((c) => {
                      const active = activeSessionId ? activeConvBySession[activeSessionId] === c.id : false;
                      return (
                        <div
                          key={c.id}
                          className={cn(
                            "group flex cursor-pointer items-start gap-2 rounded-[calc(var(--radius)-2px)] px-2 py-1.5 transition-colors",
                            active ? "bg-accent" : "hover:bg-accent/50",
                          )}
                          onClick={() => handleResume(c)}
                        >
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-xs font-medium text-foreground">
                              {c.title || c.hostName}
                            </p>
                            <p className="mt-0.5 flex items-center gap-1.5 text-[10px] text-muted-foreground">
                              <span className="truncate">{c.hostName}</span>·
                              <span className="shrink-0">{t("agent.msgCount", { count: c.messageCount })}</span>·
                              <span className="shrink-0">
                                {c.updatedAt ? new Date(c.updatedAt).toLocaleString() : ""}
                              </span>
                            </p>
                          </div>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDeleteConv(c);
                            }}
                            aria-label={t("common.delete")}
                            title={t("common.delete")}
                            className="rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100"
                          >
                            <Trash2 className="h-3 w-3" />
                          </button>
                        </div>
                      );
                    })
                )}
              </div>
              <div className="border-t border-border p-1.5">
                <button
                  type="button"
                  onClick={handleNewChat}
                  className="flex w-full items-center gap-1.5 rounded-[calc(var(--radius)-2px)] px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
                >
                  <MessageSquarePlus className="h-3.5 w-3.5" />
                  {t("agent.newChat")}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Messages */}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        className="relative min-h-0 flex-1 overflow-auto p-4"
      >
        {messages.length === 0 ? (
          <div className="flex h-full items-center justify-center text-center">
            <div>
              <Bot className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                {t("agent.emptyHint")}
              </p>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {messages.map((msg) => (
              <MessageBubble
                key={msg.id}
                msg={msg}
                canInsert={!!activeSessionId}
                onInsert={handleInsert}
                onRetry={handleRetry}
              />
            ))}
          </div>
        )}

        {/* Jump-to-bottom affordance when the user scrolled up mid-stream */}
        {showJumpDown && (
          <button
            type="button"
            onClick={jumpToBottom}
            aria-label={t("agent.scrollDown")}
            title={t("agent.scrollDown")}
            className="sticky bottom-0 left-full flex h-7 w-7 -translate-x-1 items-center justify-center rounded-full border border-border bg-popover text-muted-foreground shadow-md transition-colors hover:text-foreground"
          >
            <ArrowDown className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {/* Input area: inline model selector + message box */}
      <div className="border-t border-border bg-card px-3 py-2">
        {providers.length > 0 ? (
          <div className="flex items-center gap-1.5 pb-2">
            <Cpu className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <Select
              value={agentProviderId}
              onChange={(e) => handleProviderChange(e.target.value)}
              className="h-7 w-auto max-w-[45%] text-xs"
              title={t("agent.provider")}
              aria-label={t("agent.provider")}
            >
              {providers.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </Select>
            {(currentProvider?.models?.length ?? 0) > 0 ? (
              <Select
                value={agentModel}
                onChange={(e) => handleModelChange(e.target.value)}
                className="h-7 min-w-0 flex-1 font-mono text-xs"
                title={t("agent.model")}
                aria-label={t("agent.model")}
              >
                {currentProvider!.models!.map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </Select>
            ) : (
              // Provider has no recorded models — enter the model id by hand.
              <Input
                value={agentModel}
                onChange={(e) => handleModelChange(e.target.value)}
                placeholder={t("agent.modelManualPlaceholder")}
                className="h-7 flex-1 border-transparent bg-transparent px-1 font-mono text-xs focus-visible:border-input"
                aria-label={t("agent.model")}
              />
            )}
          </div>
        ) : providersLoaded ? (
          <div className="flex items-center gap-2 pb-2 text-xs text-muted-foreground">
            <span>{t("agent.noProviders")}</span>
            <Button variant="outline" size="sm" className="h-6 px-2 text-xs" onClick={goSettings}>
              <Settings2 className="h-3 w-3" />
              {t("agent.goSettings")}
            </Button>
          </div>
        ) : null}
        <div className="flex items-end gap-2">
          <textarea
            ref={textareaRef}
            rows={1}
            className="max-h-40 min-h-[40px] flex-1 resize-none rounded-[var(--radius)] border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            placeholder={
              modelChosen
                ? t("agent.placeholderConfigured")
                : t("agent.placeholderNoModel")
            }
            value={input}
            onChange={(e) => setInput(e.target.value)}
            aria-label={t("agent.placeholderConfigured")}
            onKeyDown={(e) => {
              // IME safety: Enter during composition confirms the candidate,
              // it must not send the message.
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                handleSend();
                return;
              }
              // ↑/↓ recall previously sent messages while at the start of
              // the line or already navigating history.
              if (e.key === "ArrowUp" && (input === "" || historyIdx !== null)) {
                if (inputHistory.length === 0) return;
                e.preventDefault();
                const idx = historyIdx === null ? inputHistory.length - 1 : Math.max(0, historyIdx - 1);
                setHistoryIdx(idx);
                setInput(inputHistory[idx]);
                return;
              }
              if (e.key === "ArrowDown" && historyIdx !== null) {
                e.preventDefault();
                const idx = historyIdx + 1;
                if (idx >= inputHistory.length) {
                  setHistoryIdx(null);
                  setInput("");
                } else {
                  setHistoryIdx(idx);
                  setInput(inputHistory[idx]);
                }
              }
            }}
          />
          {isStreaming ? (
            <Button
              type="button"
              variant="destructive"
              size="icon"
              onClick={handleCancel}
              aria-label={t("agent.stop")}
              title={t("agent.stop")}
              className="h-9 w-9 shrink-0"
            >
              <Square className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              type="button"
              size="icon"
              onClick={handleSend}
              disabled={!input.trim() || !modelChosen}
              aria-label={t("agent.send")}
              title={t("agent.send")}
              className="h-9 w-9 shrink-0"
            >
              <Send className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Memoized: during streaming only the changing message re-renders — completed
 * messages keep their identity in the store, so their markdown is not
 * re-parsed on every chunk.
 */
const MessageBubble = memo(function MessageBubble({
  msg,
  canInsert,
  onInsert,
  onRetry,
}: {
  msg: ChatMessage;
  canInsert: boolean;
  onInsert: (code: string) => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();

  if (msg.role === "tool") {
    return <ToolStep msg={msg} />;
  }

  const isUser = msg.role === "user";

  // Notices: errors (retryable) and user cancels (quiet).
  if (msg.variant === "error") {
    return (
      <div className="flex justify-start">
        <div className="flex max-w-[90%] flex-col gap-1.5 rounded-[var(--radius)] border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm">
          <p className="whitespace-pre-wrap break-words text-destructive">{msg.content}</p>
          <button
            type="button"
            onClick={onRetry}
            className="inline-flex w-fit items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <RotateCcw className="h-3 w-3" />
            {t("agent.retry")}
          </button>
        </div>
      </div>
    );
  }
  if (msg.variant === "cancelled") {
    return (
      <div className="flex justify-start">
        <p className="inline-flex max-w-[90%] items-center gap-1.5 rounded-full border border-border bg-secondary/50 px-2.5 py-1 text-xs text-muted-foreground">
          <Scissors className="h-3 w-3" />
          {msg.content}
        </p>
      </div>
    );
  }

  return (
    <div className={cn("flex", isUser ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[90%] rounded-[var(--radius)] px-3 py-2 text-sm",
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-secondary text-secondary-foreground",
        )}
      >
        {isUser ? (
          <pre className="whitespace-pre-wrap break-words font-sans">{msg.content}</pre>
        ) : (
          <AgentMarkdown content={msg.content} canInsert={canInsert} onInsert={onInsert} />
        )}
      </div>
    </div>
  );
});

/**
 * ToolStep shows one tool invocation as a collapsible step: the header always
 * carries the tool name plus a one-line argument summary; expanding reveals
 * the formatted arguments and the result. Running steps auto-expand and stay
 * open unless the user closes them.
 */
function ToolStep({ msg }: { msg: ChatMessage }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const running = msg.running === true;

  // Auto-expand while the tool runs (user toggles still win).
  useEffect(() => {
    if (running) setOpen(true);
  }, [running]);

  const summary = toolSummary(msg.toolArgs);
  const prettyArgs = prettyJSON(msg.toolArgs);
  const truncated = msg.content.includes("[truncated");

  const copyResult = async () => {
    try {
      await navigator.clipboard.writeText(msg.content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* clipboard may be blocked */
    }
  };

  return (
    <details
      open={open}
      onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}
      className="group rounded-[var(--radius)] border border-border bg-secondary/30 text-xs"
    >
      <summary className="flex cursor-pointer select-none items-center gap-2 p-2.5 hover:bg-accent/30 [&::-webkit-details-marker]:hidden">
        {running ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-primary" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
        )}
        <Wrench className="h-3.5 w-3.5 shrink-0 text-primary" />
        <span className="shrink-0 font-mono font-medium text-primary">{msg.toolName}</span>
        {summary && (
          <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground">{summary}</span>
        )}
        {truncated && (
          <span className="shrink-0 rounded-full border border-warning/40 bg-warning/10 px-1.5 py-0.5 text-[10px] text-warning">
            {t("agent.truncated")}
          </span>
        )}
        {running && (
          <span className="shrink-0 text-[10px] text-muted-foreground" role="status">
            {t("agent.toolRunning")}
          </span>
        )}
      </summary>
      <div className="flex flex-col gap-2 border-t border-border/60 p-2.5">
        {prettyArgs && (
          <div>
            <div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">
              {t("agent.approvalArgs")}
            </div>
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-background/80 p-2 font-mono text-muted-foreground">
              {prettyArgs}
            </pre>
          </div>
        )}
        {msg.content && (
          <div>
            <div className="mb-1 flex items-center justify-between">
              <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                {t("agent.toolResult")}
              </span>
              <button
                type="button"
                onClick={copyResult}
                aria-label={t("agent.copyResult")}
                title={t("agent.copyResult")}
                className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                {copied ? (
                  <Check className="h-3 w-3 text-success" />
                ) : (
                  <CopyIcon className="h-3 w-3" />
                )}
                {copied ? t("agent.copied") : t("agent.copyResult")}
              </button>
            </div>
            <pre className="max-h-60 overflow-auto whitespace-pre-wrap break-all rounded bg-background/80 p-2 font-mono text-foreground/80">
              {msg.content}
            </pre>
          </div>
        )}
      </div>
    </details>
  );
}

/** toolSummary extracts a one-line hint from the tool's JSON arguments — the
 *  command, path, or first value — for the collapsed step header. */
function toolSummary(argsJSON: string | undefined): string {
  if (!argsJSON) return "";
  try {
    const parsed = JSON.parse(argsJSON) as Record<string, unknown>;
    const pick =
      parsed.command ?? parsed.path ?? parsed.remotePath ?? parsed.localPath ?? parsed.session_id ?? parsed.host;
    if (typeof pick === "string" && pick) return pick;
  } catch {
    // not JSON
  }
  const line = argsJSON.replace(/\s+/g, " ").trim();
  return line.length > 120 ? `${line.slice(0, 120)}…` : line;
}

/** prettyJSON formats JSON arguments for display; non-JSON falls back raw. */
function prettyJSON(argsJSON: string | undefined): string {
  if (!argsJSON) return "";
  try {
    return JSON.stringify(JSON.parse(argsJSON), null, 2);
  } catch {
    return argsJSON;
  }
}

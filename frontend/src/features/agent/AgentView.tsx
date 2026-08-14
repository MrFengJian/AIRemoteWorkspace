import { useEffect, useMemo, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import {
  Bot,
  Send,
  Square,
  Cpu,
  Settings2,
  Loader2,
  Wrench,
  ShieldAlert,
  Check,
  ChevronRight,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { agentApi } from "@/features/agent/api";
import { useAgentStore } from "@/features/agent/store";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { useModelProviders } from "@/features/settings/hooks";
import { useHosts } from "@/features/hosts/hooks";
import { AgentMarkdown } from "@/features/agent/AgentMarkdown";
import { HostService, TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { useUIStore } from "@/stores/ui.store";
import { decodeBase64, encodeBase64 } from "@/lib/base64";
import { cn } from "@/lib/utils";

/**
 * AI Agent view. Bound to the active terminal session — the agent operates on
 * that session's connected host. Streams LLM responses, shows tool calls
 * inline, and gates WRITE/DANGEROUS tools behind an approval dialog.
 *
 * In embedded mode (embeddedSessionID set), it binds directly to that session
 * instead of reading the global activeId — for embedding inside the session
 * page's right panel. The provider + model used for chats are chosen per
 * session from the inline selector above the input box; with no usable
 * provider configured, the user is pointed at Settings → Models.
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
  // Embedded mode overrides the global active session.
  const activeSessionId = embeddedSessionID ?? storeActiveId;

  const {
    histories,
    streaming,
    approvals,
    addMessage,
    appendToLast,
    setStreaming,
    setToolResult,
    finishToolSteps,
    addApproval,
    removeApproval,
  } = useAgentStore();

  const [input, setInput] = useState("");
  // Shared provider query (same cache as Settings → Models), filtered to
  // enabled providers for the inline selector.
  const { data: allProviders, isLoading: providersLoading } = useModelProviders();
  const providers = useMemo(
    () => (allProviders ?? []).filter((p) => p.enabled),
    [allProviders],
  );
  const providersLoaded = !providersLoading;
  const scrollRef = useRef<HTMLDivElement | null>(null);

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

  // Enabled providers for the inline selector come from the shared
  // useModelProviders query — the panel re-reads on each open and stays in
  // sync with Settings → Models through the shared query cache.

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
  // Waits for the host-preference seeding above so it doesn't clobber it.
  useEffect(() => {
    if (!providersLoaded || !activeSessionId || providers.length === 0) return;
    if (session?.agentProviderId === undefined) return;
    const cur = providers.find((p) => p.id === agentProviderId);
    if (cur && agentModel && cur.models?.includes(agentModel)) return;
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

  // Auto-scroll to bottom on new messages.
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [histories, activeSessionId]);

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
      addMessage(sid, {
        role: "assistant",
        content: `${t("agent.errorPrefix")} ${typeof data === "string" ? data : "error"}`,
      });
    });

    return () => {
      if (typeof chunkCancel === "function") chunkCancel();
      if (typeof toolCancel === "function") toolCancel();
      if (typeof toolEndCancel === "function") toolEndCancel();
      if (typeof doneCancel === "function") doneCancel();
      if (typeof errorCancel === "function") errorCancel();
    };
  }, [activeSessionId, addMessage, appendToLast, setStreaming, setToolResult, finishToolSteps]);

  // Subscribe to approval requests (global, not per-session).
  useEffect(() => {
    const cancel = Events.On("agent:approval", (e: unknown) => {
      const data = (e as { data?: unknown }).data as
        | { reqId?: string; sessionId?: string; toolName?: string; permission?: string; args?: string }
        | undefined;
      if (data?.reqId) {
        addApproval({
          reqId: data.reqId,
          sessionId: data.sessionId ?? "",
          toolName: data.toolName ?? "",
          permission: data.permission ?? "",
          args: data.args ?? "",
        });
      }
    });
    return () => {
      if (typeof cancel === "function") cancel();
    };
  }, [addApproval]);

  const handleSend = async () => {
    if (!activeSessionId || !input.trim()) return;
    // Chats require a provider + model selection (the inline selector above
    // the input defaults to the first available one; send stays disabled
    // until something is selectable).
    if (!modelChosen) return;
    const msg = input.trim();
    setInput("");
    addMessage(activeSessionId, { role: "user", content: msg });
    setStreaming(activeSessionId, true);
    // Seed an empty assistant message for streaming append.
    addMessage(activeSessionId, { role: "assistant", content: "" });
    try {
      await agentApi.startChat(activeSessionId, agentProviderId, agentModel, msg);
    } catch (e) {
      setStreaming(activeSessionId, false);
      addMessage(activeSessionId, {
        role: "assistant",
        content: `${t("agent.errorPrefix")} ${e instanceof Error ? e.message : String(e)}`,
      });
    }
  };

  const handleCancel = () => {
    if (activeSessionId) agentApi.cancelChat(activeSessionId);
  };

  // handleInsert sends a code/command block to the active terminal's stdin.
  // The text appears in the terminal as if the user typed it — NO trailing
  // newline is added, so nothing executes until the user presses Enter. This
  // lets the user review the inserted content and decide whether to run it.
  const handleInsert = (code: string) => {
    if (!activeSessionId) return;
    TerminalService.WriteStdin(activeSessionId, encodeBase64(code)).catch(() => {
      /* session may have closed */
    });
  };

  const messages = activeSessionId ? histories[activeSessionId] ?? [] : [];
  const isStreaming = activeSessionId ? streaming[activeSessionId] : false;
  const currentApproval = approvals[0];

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
        <div className="flex items-center gap-2">
          <Bot className="h-4 w-4 text-primary" />
          <span className="text-sm font-medium">{t("agent.title")} · {sessionName}</span>
          {isStreaming && (
            <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
          )}
        </div>
      </div>

      {/* Messages */}
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto p-4">
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
              />
            ))}
          </div>
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
            className="min-h-[40px] max-h-32 flex-1 resize-none rounded-[var(--radius)] border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            placeholder={
              modelChosen
                ? t("agent.placeholderConfigured")
                : t("agent.placeholderNoModel")
            }
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            disabled={isStreaming}
            rows={1}
          />
          {isStreaming ? (
            <Button variant="destructive" size="icon" onClick={handleCancel} title={t("agent.stop")}>
              <Square className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              size="icon"
              onClick={handleSend}
              disabled={!input.trim() || !modelChosen}
              title={t("agent.send")}
            >
              <Send className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Approval dialog */}
      {currentApproval && (
        <ApprovalDialog
          approval={currentApproval}
          onResolve={(approved) => {
            agentApi.approveToolCall(currentApproval.reqId, approved);
            removeApproval(currentApproval.reqId);
          }}
        />
      )}
    </div>
  );
}

function MessageBubble({
  msg,
  canInsert,
  onInsert,
}: {
  msg: { role: string; content: string; toolName?: string; toolArgs?: string; callId?: string; running?: boolean };
  canInsert: boolean;
  onInsert: (code: string) => void;
}) {
  if (msg.role === "tool") {
    return <ToolStep msg={msg} />;
  }

  const isUser = msg.role === "user";

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
}

/**
 * ToolStep shows one tool invocation as a collapsible step: the header always
 * carries the tool name plus a one-line argument summary; expanding reveals
 * the full arguments and (when the runtime reports one) the result.
 */
function ToolStep({
  msg,
}: {
  msg: { role: string; content: string; toolName?: string; toolArgs?: string; callId?: string; running?: boolean };
}) {
  const { t } = useTranslation();
  const summary = toolSummary(msg.toolArgs);
  const running = msg.running === true;

  return (
    <details className="group rounded-[var(--radius)] border border-border bg-secondary/30 text-xs">
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
        {running && (
          <span className="shrink-0 text-[10px] text-muted-foreground">{t("agent.toolRunning")}</span>
        )}
      </summary>
      <div className="flex flex-col gap-2 border-t border-border/60 p-2.5">
        {msg.toolArgs && (
          <div>
            <div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">
              {t("agent.approvalArgs")}
            </div>
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-background/80 p-2 font-mono text-muted-foreground">
              {msg.toolArgs}
            </pre>
          </div>
        )}
        {msg.content && (
          <div>
            <div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">
              {t("agent.toolResult")}
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

function ApprovalDialog({
  approval,
  onResolve,
}: {
  approval: { toolName: string; permission: string; args: string };
  onResolve: (approved: boolean) => void;
}) {
  const { t } = useTranslation();
  const isDangerous = approval.permission === "dangerous";

  // Parse the args JSON to extract the command/path for prominent display.
  let commandText = approval.args;
  let argLabel = t("agent.approvalArgs");
  try {
    const parsed = JSON.parse(approval.args);
    if (parsed.command) {
      commandText = parsed.command;
      argLabel = t("agent.approvalCommand");
    } else if (parsed.path) {
      commandText = parsed.path;
      argLabel = t("agent.approvalPath");
    } else if (parsed.remotePath) {
      commandText = `${parsed.remotePath}${parsed.localPath ? " ← " + parsed.localPath : ""}`;
      argLabel = t("agent.approvalFile");
    }
  } catch {
    // not JSON; show raw
  }

  return (
    <Dialog open onOpenChange={() => onResolve(false)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldAlert className={cn("h-5 w-5", isDangerous ? "text-destructive" : "text-warning")} />
            {isDangerous ? t("agent.approvalDangerous") : t("agent.approvalTitle")}
          </DialogTitle>
          <DialogDescription>
            {t("agent.approvalDesc", { permission: approval.permission })}
          </DialogDescription>
        </DialogHeader>

        {/* The command/operation — displayed prominently */}
        <div
          className={cn(
            "rounded-[var(--radius)] border p-3",
            isDangerous
              ? "border-destructive/40 bg-destructive/10"
              : "border-warning/40 bg-warning/10",
          )}
        >
          <div className="mb-1.5 flex items-center gap-2">
            <Wrench className={cn("h-4 w-4", isDangerous ? "text-destructive" : "text-warning")} />
            <span className="font-mono text-sm font-medium">{approval.toolName}</span>
            <Badge variant={isDangerous ? "destructive" : "warning"}>
              {approval.permission}
            </Badge>
          </div>
          <div className="text-xs text-muted-foreground">{argLabel}</div>
          <pre
            className={cn(
              "mt-1 overflow-auto whitespace-pre-wrap break-all rounded bg-background/80 p-2.5 font-mono text-sm",
              isDangerous ? "text-destructive" : "text-foreground",
            )}
          >
            {commandText}
          </pre>
        </div>

        {isDangerous && (
          <p className="text-xs text-destructive">
            {t("agent.approvalDanger")}
          </p>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onResolve(false)}>
            <X className="h-4 w-4" /> {t("agent.deny")}
          </Button>
          <Button
            variant={isDangerous ? "destructive" : "default"}
            onClick={() => onResolve(true)}
          >
            <Check className="h-4 w-4" /> {t("agent.approve")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import {
  Bot,
  Send,
  Square,
  Settings2,
  Loader2,
  Wrench,
  ShieldAlert,
  Check,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { agentApi, type LLMConfigDTO } from "@/features/agent/api";
import { useAgentStore } from "@/features/agent/store";
import { useTerminalStore } from "@/features/terminal/terminal.store";
import { CodeBlock } from "@/features/agent/CodeBlock";
import { TerminalService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { cn } from "@/lib/utils";

/**
 * AI Agent view. Bound to the active terminal session — the agent operates on
 * that session's connected host. Streams LLM responses, shows tool calls
 * inline, and gates WRITE/DANGEROUS tools behind an approval dialog.
 */
export function AgentView() {
  const { t } = useTranslation();
  const activeSessionId = useTerminalStore((s) => s.activeId);
  const sessions = useTerminalStore((s) => s.sessions);

  const {
    histories,
    streaming,
    approvals,
    llmConfigured,
    addMessage,
    appendToLast,
    setStreaming,
    addApproval,
    removeApproval,
    setLLMConfigured,
  } = useAgentStore();

  const [input, setInput] = useState("");
  const [showConfig, setShowConfig] = useState(false);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const sessionName = activeSessionId
    ? sessions.find((s) => s.id === activeSessionId)?.hostName ?? "session"
    : null;

  // Check LLM config on mount.
  useEffect(() => {
    agentApi
      .getLLMConfig()
      .then((cfg: LLMConfigDTO) => setLLMConfigured(cfg.hasApiKey))
      .catch(() => {});
  }, [setLLMConfigured]);

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
          appendToLast(sid, atob(data));
        } catch {
          appendToLast(sid, data);
        }
      }
    });

    const toolCancel = Events.On(`agent:${sid}:toolcall`, (e: unknown) => {
      const data = (e as { data?: unknown }).data as
        | { tool?: string; args?: string; result?: string }
        | undefined;
      if (data?.tool) {
        addMessage(sid, {
          role: "tool",
          content: data.result || "",
          toolName: data.tool,
          toolArgs: data.args,
        });
      }
    });

    const doneCancel = Events.On(`agent:${sid}:done`, () => {
      setStreaming(sid, false);
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
      if (typeof doneCancel === "function") doneCancel();
      if (typeof errorCancel === "function") errorCancel();
    };
  }, [activeSessionId, addMessage, appendToLast, setStreaming]);

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
    const msg = input.trim();
    setInput("");
    addMessage(activeSessionId, { role: "user", content: msg });
    setStreaming(activeSessionId, true);
    // Seed an empty assistant message for streaming append.
    addMessage(activeSessionId, { role: "assistant", content: "" });
    try {
      await agentApi.startChat(activeSessionId, msg);
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
    TerminalService.WriteStdin(activeSessionId, btoa(code)).catch(() => {
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
        <Button
          variant="ghost"
          size="sm"
          className="h-7"
          onClick={() => setShowConfig(true)}
        >
          <Settings2 className="h-3.5 w-3.5" />
          {t("agent.llmConfig")}
          {!llmConfigured && (
            <Badge variant="destructive" className="ml-1 text-[10px]">
              {t("agent.noKey")}
            </Badge>
          )}
        </Button>
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
            {messages.map((msg, i) => (
              <MessageBubble
                key={i}
                msg={msg}
                canInsert={!!activeSessionId}
                onInsert={handleInsert}
              />
            ))}
          </div>
        )}
      </div>

      {/* Input */}
      <div className="border-t border-border bg-card p-3">
        <div className="flex items-end gap-2">
          <textarea
            className="min-h-[40px] max-h-32 flex-1 resize-none rounded-[var(--radius)] border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            placeholder={
              llmConfigured
                ? t("agent.placeholderConfigured")
                : t("agent.placeholderNoKey")
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
            <Button size="icon" onClick={handleSend} disabled={!input.trim()} title={t("agent.send")}>
              <Send className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* LLM Config dialog */}
      {showConfig && (
        <LLMConfigDialog onClose={() => setShowConfig(false)} onSaved={() => setLLMConfigured(true)} />
      )}

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
  msg: { role: string; content: string; toolName?: string; toolArgs?: string };
  canInsert: boolean;
  onInsert: (code: string) => void;
}) {
  if (msg.role === "tool") {
    return (
      <div className="flex items-start gap-2 rounded-[var(--radius)] border border-border bg-secondary/30 p-2.5 text-xs">
        <Wrench className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <span className="font-mono font-medium text-primary">{msg.toolName}</span>
          {msg.toolArgs && (
            <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-muted-foreground">
              {msg.toolArgs}
            </pre>
          )}
          {msg.content && (
            <pre className="mt-1 max-h-60 overflow-auto whitespace-pre-wrap break-all border-t border-border pt-1 font-mono text-foreground/80">
              {msg.content}
            </pre>
          )}
        </div>
      </div>
    );
  }

  const isUser = msg.role === "user";

  // For assistant messages, parse out fenced code blocks (```...```) and
  // render them with Copy + Insert actions. Inline text stays as prose.
  const segments = isUser ? null : parseCodeBlocks(msg.content);

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
          <div className="flex flex-col gap-1">
            {segments?.map((seg, i) =>
              seg.type === "code" ? (
                <CodeBlock
                  key={i}
                  code={seg.text}
                  canInsert={canInsert}
                  onInsert={onInsert}
                />
              ) : (
                <pre
                  key={i}
                  className="whitespace-pre-wrap break-words font-sans"
                >
                  {seg.text}
                </pre>
              ),
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/** A parsed segment of an assistant message: either prose or a code block. */
type Segment = { type: "text" | "code"; text: string };

/**
 * parseCodeBlocks splits content into alternating text/code segments.
 * Recognises ``` fenced blocks. A fenced block without a language tag that
 * spans a single short line is treated as a command (still rendered as code
 * so it gets Copy/Insert buttons).
 */
function parseCodeBlocks(content: string): Segment[] {
  const segments: Segment[] = [];
  // Match ```lang\n ... ``` (fenced), or ``` ... ```
  const fenceRe = /```[a-zA-Z]*\n?([\s\S]*?)```/g;
  let lastIdx = 0;
  let match: RegExpExecArray | null;
  while ((match = fenceRe.exec(content)) !== null) {
    if (match.index > lastIdx) {
      const text = content.slice(lastIdx, match.index).trim();
      if (text) segments.push({ type: "text", text });
    }
    segments.push({ type: "code", text: match[1].replace(/\n$/, "") });
    lastIdx = fenceRe.lastIndex;
  }
  if (lastIdx < content.length) {
    const rest = content.slice(lastIdx).trim();
    if (rest) segments.push({ type: "text", text: rest });
  }
  // If no code blocks were found but the entire message looks like a single
  // command line, wrap it as a code block so it gets buttons.
  if (segments.length === 0 && content.trim()) {
    const line = content.trim();
    if (looksLikeCommand(line)) {
      return [{ type: "code", text: line }];
    }
    return [{ type: "text", text: content }];
  }
  return segments.length > 0 ? segments : [{ type: "text", text: content }];
}

/** looksLikeCommand heuristically detects a bare command line. */
function looksLikeCommand(line: string): boolean {
  if (line.includes("\n")) return false;
  // starts with a known command word, or contains typical shell syntax
  return /^\s*(sudo\s+)?(apt|apt-get|yum|dnf|systemctl|service|docker|kubectl|git|ssh|curl|wget|cat|ls|cd|cp|mv|rm|mkdir|chmod|chown|grep|sed|awk|find|tar|gzip|gunzip|echo|export|source|ps|df|du|free|top|htop|uptime|who|last|journalctl|dmesg|lsof|netstat|ss|ping|traceroute|dig|nslookup)\b/.test(
    line,
  );
}

function LLMConfigDialog({
  onClose,
  onSaved,
}: {
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [config, setConfig] = useState<LLMConfigDTO>({
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o",
    hasApiKey: false,
  });
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    agentApi.getLLMConfig().then(setConfig).catch(() => {});
  }, []);

  const handleSave = async () => {
    setSaving(true);
    try {
      await agentApi.setLLMConfig({
        baseUrl: config.baseUrl,
        model: config.model,
        apiKey, // empty = keep existing
      });
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("agent.configTitle")}</DialogTitle>
          <DialogDescription>{t("agent.configDesc")}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-1.5">
            <Label htmlFor="baseUrl">{t("agent.baseUrl")}</Label>
            <Input
              id="baseUrl"
              value={config.baseUrl}
              onChange={(e) => setConfig({ ...config, baseUrl: e.target.value })}
              placeholder="https://api.openai.com/v1"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="model">{t("agent.model")}</Label>
            <Input
              id="model"
              value={config.model}
              onChange={(e) => setConfig({ ...config, model: e.target.value })}
              placeholder="gpt-4o"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="apiKey">
              {t("agent.apiKey")}
              {config.hasApiKey && (
                <Badge variant="success" className="ml-2 text-[10px]">{t("hostForm.saved")}</Badge>
              )}
            </Label>
            <Input
              id="apiKey"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={config.hasApiKey ? "•••••••• (saved in OS vault)" : "sk-..."}
            />
            <p className="text-xs text-muted-foreground">
              {t("agent.apiKeyHint")}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t("common.cancel")}</Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="h-4 w-4 animate-spin" />} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
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

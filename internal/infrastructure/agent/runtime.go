// Package agent implements the AI Agent runtime: it constructs eino ReAct
// agents per SSH session, streams their output, and routes tool calls through
// the permission gate.
//
// It lives in infrastructure (not application) because it depends on eino,
// ssh, sftp, and the tools package — all infrastructure concerns. The
// application layer defines the port interfaces (AgentRuntime, AgentEvents).
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent/tools"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// AgentEvents delivers streaming agent output upward (to the Wails service).
// A tool invocation is reported twice with the same stepID: once when it
// starts (OnToolCallStart) and once when the result is available
// (OnToolCallEnd) — the UI folds both into one step. Both come from the
// tool-layer observer (tools.RunObserver), which works for every model.
type AgentEvents interface {
	OnChunk(sessionID, text string)
	OnToolCallStart(sessionID, callID, toolName, args string)
	OnToolCallEnd(sessionID, callID, result string)
	OnDone(sessionID string)
	OnError(sessionID, errMsg string)
}

// eventsObserver adapts AgentEvents to tools.RunObserver.
type eventsObserver struct{ events AgentEvents }

func (o eventsObserver) OnToolStart(sessionID, stepID, toolName, args string) {
	if o.events != nil {
		o.events.OnToolCallStart(sessionID, stepID, toolName, args)
	}
}

func (o eventsObserver) OnToolEnd(sessionID, stepID, result string) {
	if o.events != nil {
		o.events.OnToolCallEnd(sessionID, stepID, result)
	}
}

// LLMResolver resolves a (providerID, model) selection into endpoint
// credentials for building the chat model. Implemented by the application
// layer's ModelProviderService.
type LLMResolver interface {
	ResolveLLM(providerID, model string) (domain.LLMEndpoint, error)
}

// CredsResolver returns host + credentials for a session (for SFTP tools).
type CredsResolver = tools.CredsResolver

// PermissionGate is the approval gate tools call before WRITE/DANGEROUS ops.
type PermissionGate = tools.PermissionGate

// SftpFileOps is the subset of the SFTP manager the file tools need.
type SftpFileOps = tools.SftpFileOps

// Conversation-memory budget: replayed history is capped by characters
// (≈6k tokens) and turn count, whichever binds first. Only complete turns
// (user + final assistant answer) are recorded; tool steps stay internal to
// their turn — the final answer summarizes them.
const (
	historyCharBudget = 24 * 1024
	maxHistoryTurns   = 40
)

// Runtime manages per-session ReAct agents and streams their output.
// Conversation history is kept in memory per session and replayed on each
// turn (multi-turn memory); ClearHistory drops it.
type Runtime struct {
	llm     LLMResolver
	sshMgr  *ssh.Manager
	sftp    SftpFileOps
	gate    PermissionGate
	secrets SecretsForResolver

	mu        sync.Mutex
	cancelFns map[string]context.CancelFunc
	histories map[string][]*schema.Message
}

// SecretsForResolver provides remembered host secrets for credential resolution.
type SecretsForResolver interface {
	GetHostSecret(hostID string, kind string) ([]byte, error)
}

// NewRuntime wires the agent runtime.
func NewRuntime(llm LLMResolver, sshMgr *ssh.Manager, sftp SftpFileOps, gate PermissionGate, secrets SecretsForResolver) *Runtime {
	return &Runtime{
		llm:       llm,
		sshMgr:    sshMgr,
		sftp:      sftp,
		gate:      gate,
		secrets:   secrets,
		cancelFns: make(map[string]context.CancelFunc),
		histories: make(map[string][]*schema.Message),
	}
}

// Chat starts a streaming agent chat for a session using the selected
// provider + model. The session's conversation history is replayed so the
// model keeps context across turns.
func (r *Runtime) Chat(ctx context.Context, sessionID, providerID, model, userMessage string, events AgentEvents) error {
	ep, err := r.llm.ResolveLLM(providerID, model)
	if err != nil {
		return err
	}
	// Local OpenAI-compatible endpoints (Ollama, LM Studio) need no real key,
	// but the client requires a non-empty one.
	apiKey := ep.APIKey
	if apiKey == "" {
		apiKey = "local-no-key"
	}

	chatModel, err := openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
		BaseURL: ep.BaseURL,
		APIKey:  apiKey,
		Model:   ep.Model,
	})
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}

	credsResolver := r.buildResolver()
	ts, err := tools.NewToolSet(tools.Deps{SSH: r.sshMgr, SFTP: r.sftp}, credsResolver, r.gate, eventsObserver{events})
	if err != nil {
		return fmt.Errorf("build toolset: %w", err)
	}
	// Local terminal sessions (id prefix "local-") have no SSH host behind
	// them: expose only the local tools.
	var toolList []tool.BaseTool
	if strings.HasPrefix(sessionID, "local-") {
		toolList, err = ts.BuildLocalForSession(sessionID)
	} else {
		toolList, err = ts.BuildForSession(sessionID)
	}
	if err != nil {
		return fmt.Errorf("build session tools: %w", err)
	}

	ag, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: toolList,
		},
		MaxStep: 20,
		// Route the model's stream: tool calls anywhere → tools node; pure
		// text past a short preamble → END. The default first-chunk checker
		// breaks models that emit text before tool calls (Claude, DeepSeek-R1).
		StreamToolCallChecker: hybridToolCallChecker,
	})
	if err != nil {
		return fmt.Errorf("create react agent: %w", err)
	}

	chatCtx, cancel := context.WithCancel(ctx)
	r.registerCancel(sessionID, cancel)
	defer r.unregisterCancel(sessionID)

	// [system] + replayed history + this turn's user message.
	msgs := make([]*schema.Message, 0, 8)
	msgs = append(msgs, schema.SystemMessage(r.systemPrompt(sessionID)))
	r.mu.Lock()
	msgs = append(msgs, r.histories[sessionID]...)
	r.mu.Unlock()
	msgs = append(msgs, schema.UserMessage(userMessage))

	reader, err := ag.Stream(chatCtx, msgs)
	if err != nil {
		return fmt.Errorf("agent stream: %w", err)
	}
	defer reader.Close()

	// Tool steps are reported by the tool-layer observer (see eventsObserver);
	// this loop only carries the final assistant text.
	var finalText strings.Builder
	for {
		msg, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, context.Canceled) && events != nil {
				events.OnError(sessionID, "cancelled")
				return err
			}
			if events != nil {
				events.OnError(sessionID, err.Error())
			}
			return err
		}
		if msg.Content != "" {
			if events != nil {
				events.OnChunk(sessionID, msg.Content)
			}
			finalText.WriteString(msg.Content)
		}
	}

	if events != nil {
		events.OnDone(sessionID)
	}
	// Record the completed turn for multi-turn memory. Failed/cancelled turns
	// are not recorded (the user saw no complete answer).
	if finalText.Len() > 0 {
		r.recordTurn(sessionID, userMessage, finalText.String())
	}
	return nil
}

// Cancel cancels an ongoing chat.
func (r *Runtime) Cancel(sessionID string) {
	r.mu.Lock()
	cancel, ok := r.cancelFns[sessionID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
}

// ClearHistory forgets a session's conversation (frontend "clear chat").
func (r *Runtime) ClearHistory(sessionID string) {
	r.mu.Lock()
	delete(r.histories, sessionID)
	r.mu.Unlock()
}

// recordTurn appends a completed (user, assistant) turn and trims the oldest
// turns to stay within the char/turn budget.
func (r *Runtime) recordTurn(sessionID, user, assistant string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := append(r.histories[sessionID],
		schema.UserMessage(user),
		schema.AssistantMessage(assistant, nil))

	total := len(user) + len(assistant)
	i := 0
	// History is (user, assistant) pairs; drop two at a time. Always keep the
	// newest turn (the last two messages).
	for i+2 <= len(h)-2 && (total > historyCharBudget || (len(h)-i)/2 > maxHistoryTurns) {
		total -= len(h[i].Content) + len(h[i+1].Content)
		i += 2
	}
	r.histories[sessionID] = h[i:]
}

func (r *Runtime) registerCancel(sessionID string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancelFns[sessionID] = cancel
	r.mu.Unlock()
}

func (r *Runtime) unregisterCancel(sessionID string) {
	r.mu.Lock()
	delete(r.cancelFns, sessionID)
	r.mu.Unlock()
}

// hybridToolCallChecker routes the model's streamed output:
//   - any chunk carrying tool calls → tools node (OpenAI-style, instant);
//   - pure text past ~200 chars with no tool calls → END (it's the answer,
//     and streaming latency stays bounded);
//   - end of stream → END.
//
// The checker receives its own fork of the stream (eino copies the branch
// input), so consuming it never loses data downstream.
func hybridToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	textLen := 0
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
		textLen += len(msg.Content)
		if textLen > 200 {
			return false, nil
		}
	}
}

// systemPrompt is the LLM-facing contract: tools, workflow, and approval
// semantics (a DENIED result must change the plan, never be retried).
// Local terminal sessions get a local-only variant.
func (r *Runtime) systemPrompt(sessionID string) string {
	if strings.HasPrefix(sessionID, "local-") {
		return "You are an AI operations assistant working on the user's LOCAL machine (a local terminal session, no remote host).\n\n" +
			"Tools:\n" +
			"- local_exec(command): run a shell command locally. Returns combined stdout+stderr; a non-zero exit is reported as " +
			"[exit status N] — diagnostic output, not a failure.\n" +
			"- local_read_file(path): read a local file as text.\n\n" +
			"Workflow: start with read-only diagnostics, analyze, then summarize findings in concise markdown and propose fixes.\n\n" +
			"Permissions: state-changing operations (file writes, package/service mutations, destructive commands) require the " +
			"user's approval. If a tool result says the user DENIED the operation, do NOT retry it — explain and propose an alternative."
	}
	host, ok := r.sshMgr.HostOfSession(sessionID)
	name := "unknown"
	if ok {
		name = fmt.Sprintf("%s@%s", host.Username, host.Host)
	}
	return fmt.Sprintf(
		"You are an AI operations assistant embedded in an SSH workspace, connected to host %s.\n\n"+
			"Tools:\n"+
			"- ssh_exec(command): run a shell command on the remote host. Returns combined stdout+stderr; "+
			"a non-zero exit is reported as [exit status N] — that is diagnostic output, not a failure.\n"+
			"- ssh_read_file(path) / ssh_write_file(path, content): read or overwrite a remote file over SFTP.\n"+
			"- upload(localPath, remotePath) / download(remotePath, localPath): move files between the user's machine and the host.\n"+
			"- local_exec(command) / local_read_file(path): run/read on the user's LOCAL machine. Prefer the remote tools unless local context is required.\n\n"+
			"Workflow: start with read-only diagnostics (uptime, df -h, free -m, ps aux, journalctl …), analyze the output, "+
			"then summarize findings in concise markdown and propose fixes.\n\n"+
			"Permissions: state-changing operations (file writes, uploads, package/service mutations, destructive commands) "+
			"require the user's approval. If a tool result says the user DENIED the operation, do NOT retry it — "+
			"explain what you were about to do and propose an alternative.",
		name,
	)
}

func (r *Runtime) buildResolver() CredsResolver {
	return func(sessionID string) (domain.Host, domain.Credentials, error) {
		host, ok := r.sshMgr.HostOfSession(sessionID)
		if !ok {
			return domain.Host{}, domain.Credentials{}, errors.New("session not found")
		}
		creds := domain.Credentials{}
		if r.secrets != nil {
			if v, err := r.secrets.GetHostSecret(host.ID, "password"); err == nil {
				creds.Password = string(v)
			}
			if v, err := r.secrets.GetHostSecret(host.ID, "passphrase"); err == nil {
				creds.KeyPassphrase = string(v)
			}
		}
		return host, creds, nil
	}
}

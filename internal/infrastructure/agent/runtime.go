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
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent/tools"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
)

// AgentEvents delivers streaming agent output upward (to the Wails service).
// A tool invocation is reported twice with the same callID: once when the
// model requests it (OnToolCallStart) and once when the result is available
// (OnToolCallEnd) — the UI folds both into one step.
type AgentEvents interface {
	OnChunk(sessionID, text string)
	OnToolCallStart(sessionID, callID, toolName, args string)
	OnToolCallEnd(sessionID, callID, result string)
	OnDone(sessionID string)
	OnError(sessionID, errMsg string)
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

// Runtime manages per-session ReAct agents and streams their output.
type Runtime struct {
	llm     LLMResolver
	sshMgr  *ssh.Manager
	sftp    SftpFileOps
	gate    PermissionGate
	secrets SecretsForResolver

	mu        sync.Mutex
	cancelFns map[string]context.CancelFunc
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
	}
}

// Chat starts a streaming agent chat for a session using the selected
// provider + model.
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
	ts, err := tools.NewToolSet(tools.Deps{SSH: r.sshMgr, SFTP: r.sftp}, credsResolver, r.gate)
	if err != nil {
		return fmt.Errorf("build toolset: %w", err)
	}
	toolList, err := ts.BuildForSession(sessionID)
	if err != nil {
		return fmt.Errorf("build session tools: %w", err)
	}

	ag, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: toolList,
		},
		MaxStep: 20,
	})
	if err != nil {
		return fmt.Errorf("create react agent: %w", err)
	}

	chatCtx, cancel := context.WithCancel(ctx)
	r.registerCancel(sessionID, cancel)
	defer r.unregisterCancel(sessionID)

	reader, err := ag.Stream(chatCtx, []*schema.Message{
		schema.SystemMessage(r.systemPrompt(sessionID)),
		schema.UserMessage(userMessage),
	})
	if err != nil {
		return fmt.Errorf("agent stream: %w", err)
	}
	defer reader.Close()

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
		if msg.Role == schema.Tool {
			// Tool result — pair it with the matching call, never into the
			// assistant text stream.
			if events != nil {
				events.OnToolCallEnd(sessionID, msg.ToolCallID, msg.Content)
			}
			continue
		}
		if msg.Content != "" && events != nil {
			events.OnChunk(sessionID, msg.Content)
		}
		if len(msg.ToolCalls) > 0 && events != nil {
			for _, tc := range msg.ToolCalls {
				events.OnToolCallStart(sessionID, tc.ID, tc.Function.Name, tc.Function.Arguments)
			}
		}
	}

	if events != nil {
		events.OnDone(sessionID)
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

func (r *Runtime) systemPrompt(sessionID string) string {
	host, ok := r.sshMgr.HostOfSession(sessionID)
	name := "unknown"
	if ok {
		name = fmt.Sprintf("%s@%s", host.Username, host.Host)
	}
	return fmt.Sprintf(
		"You are an AI operations assistant integrated into an SSH workspace. "+
			"You are connected to host %s. Use ssh_exec to run diagnostic commands on this host. "+
			"Be concise. When diagnosing, run read-only commands first (uptime, df -h, free -m, ps aux), "+
			"analyze the output, then summarize findings and suggest fixes. "+
			"Dangerous operations require user approval.",
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

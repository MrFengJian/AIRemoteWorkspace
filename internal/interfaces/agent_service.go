package interfaces

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent"
)

// LLMConfigDTO carries the non-sensitive LLM settings to the frontend.
type LLMConfigDTO struct {
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"hasApiKey"` // never the key itself
}

// SetLLMConfigInput is what the frontend sends to configure the provider.
type SetLLMConfigInput struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	APIKey  string `json:"apiKey"` // empty = keep existing; " " = clear
}

// ApprovalRequestDTO mirrors application.ApprovalRequest for the frontend.
type ApprovalRequestDTO struct {
	ReqID      string `json:"reqId"`
	SessionID  string `json:"sessionId"`
	ToolName   string `json:"toolName"`
	Permission string `json:"permission"`
	Args       string `json:"args"`
}

// AgentService exposes the AI agent + LLM config to the frontend.
type AgentService struct {
	app       *wailsapp.App
	llm       *appsvc.LLMService
	runtime   *agent.Runtime
	gate      *appsvc.PermissionGate
}

// NewAgentService wires the AgentService. The *Application is injected via
// ServiceStartup.
func NewAgentService(llm *appsvc.LLMService, runtime *agent.Runtime, gate *appsvc.PermissionGate) *AgentService {
	return &AgentService{llm: llm, runtime: runtime, gate: gate}
}

func (a *AgentService) ServiceName() string { return "AgentService" }

func (a *AgentService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	a.app = wailsapp.Get()
	return nil
}

// EmitApproval sends an approval request to the frontend (implements
// application.ApprovalEmitter). The frontend shows a dialog and calls
// ApproveToolCall(reqID, approved).
func (a *AgentService) EmitApproval(req appsvc.ApprovalRequest) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("agent:approval", ApprovalRequestDTO{
		ReqID:      req.ReqID,
		SessionID:  req.SessionID,
		ToolName:   req.ToolName,
		Permission: string(req.Permission),
		Args:       req.Args,
	})
}

// GetLLMConfig returns the provider settings + whether an API key is stored.
func (a *AgentService) GetLLMConfig() (LLMConfigDTO, error) {
	cfg, err := a.llm.GetConfig()
	if err != nil {
		return LLMConfigDTO{}, err
	}
	key, _ := a.llm.GetAPIKey()
	return LLMConfigDTO{
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		HasAPIKey: key != "",
	}, nil
}

// SetLLMConfig persists BaseURL/Model and stores/clears the API key.
func (a *AgentService) SetLLMConfig(in SetLLMConfigInput) error {
	if err := a.llm.SetConfig(domain.LLMConfig{BaseURL: in.BaseURL, Model: in.Model}); err != nil {
		return err
	}
	if in.APIKey != "" {
		return a.llm.SetAPIKey(in.APIKey)
	}
	return nil
}

// StartChat kicks off a streaming agent chat. Output flows via events:
//   agent:<sessionID>:chunk   — incremental LLM text
//   agent:<sessionID>:toolcall — tool invocation notice
//   agent:<sessionID>:done    — chat completed
//   agent:<sessionID>:error   — chat failed
func (a *AgentService) StartChat(sessionID, message string) error {
	if a.runtime == nil {
		return fmt.Errorf("agent runtime not available")
	}
	sid := sessionID
	events := &agentEventsEmitter{app: a.app, sessionID: sid}
	// Run in the background — the stream is long-lived and event-driven.
	go func() {
		ctx := context.Background()
		if err := a.runtime.Chat(ctx, sid, message, events); err != nil {
			log.Printf("[AgentService] chat for %s ended: %v", sid, err)
		}
	}()
	return nil
}

// CancelChat aborts an ongoing agent chat.
func (a *AgentService) CancelChat(sessionID string) error {
	if a.runtime != nil {
		a.runtime.Cancel(sessionID)
	}
	return nil
}

// ApproveToolCall resolves a pending approval request.
func (a *AgentService) ApproveToolCall(reqID string, approved bool) error {
	return a.gate.Resolve(reqID, approved)
}

// --- agentEventsEmitter bridges agent.AgentEvents to Wails events ---

type agentEventsEmitter struct {
	app       *wailsapp.App
	sessionID string
}

func (e *agentEventsEmitter) emit(name string, data any) {
	if e.app == nil {
		return
	}
	e.app.Event.Emit(fmt.Sprintf("agent:%s:%s", e.sessionID, name), data)
}

func (e *agentEventsEmitter) OnChunk(_ string, text string) {
	// base64-encode for binary safety (LLM text should be UTF-8 but be safe)
	e.emit("chunk", base64.StdEncoding.EncodeToString([]byte(text)))
}

func (e *agentEventsEmitter) OnToolCall(_ string, toolName, args, result string) {
	e.emit("toolcall", map[string]string{
		"tool":   toolName,
		"args":   args,
		"result": result,
	})
}

func (e *agentEventsEmitter) OnDone(_ string) {
	e.emit("done", "")
}

func (e *agentEventsEmitter) OnError(_ string, msg string) {
	e.emit("error", msg)
}

// Compile-time check: agentEventsEmitter satisfies agent.AgentEvents.
var _ agent.AgentEvents = (*agentEventsEmitter)(nil)

package interfaces

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync/atomic"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/infrastructure/agent"
)

// ApprovalRequestDTO mirrors application.ApprovalRequest for the frontend.
type ApprovalRequestDTO struct {
	ReqID      string `json:"reqId"`
	SessionID  string `json:"sessionId"`
	ToolName   string `json:"toolName"`
	Permission string `json:"permission"`
	Args       string `json:"args"`
}

// AgentService exposes the AI agent to the frontend. Provider/model selection
// is per chat call; provider management lives in ModelProviderService.
type AgentService struct {
	app     *wailsapp.App
	runtime *agent.Runtime
	gate    *appsvc.PermissionGate
}

// NewAgentService wires the AgentService. The *Application is injected via
// ServiceStartup.
func NewAgentService(runtime *agent.Runtime, gate *appsvc.PermissionGate) *AgentService {
	return &AgentService{runtime: runtime, gate: gate}
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

// StartChat kicks off a streaming agent chat against the selected provider +
// model. Output flows via events:
//   agent:<sessionID>:chunk    — incremental LLM text
//   agent:<sessionID>:toolcall — tool invocation start (id/tool/args)
//   agent:<sessionID>:toolend  — tool invocation result (id/result)
//   agent:<sessionID>:done     — chat completed
//   agent:<sessionID>:error    — chat failed
func (a *AgentService) StartChat(sessionID, providerID, model, message string) error {
	if a.runtime == nil {
		return fmt.Errorf("agent runtime not available")
	}
	sid := sessionID
	events := &agentEventsEmitter{app: a.app, sessionID: sid}
	// Run in the background — the stream is long-lived and event-driven.
	go func() {
		ctx := context.Background()
		if err := a.runtime.Chat(ctx, sid, providerID, model, message, events); err != nil {
			log.Printf("[AgentService] chat for %s ended: %v", sid, err)
			// Surface pre-stream failures (bad provider, disabled, …) to the
			// frontend so it doesn't wait for events that will never come.
			// The emitter dedupes if the stream already reported an error.
			events.OnError(sid, err.Error())
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
	errored   atomic.Bool
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

func (e *agentEventsEmitter) OnToolCallStart(_ string, callID, toolName, args string) {
	e.emit("toolcall", map[string]string{
		"id":   callID,
		"tool": toolName,
		"args": args,
	})
}

func (e *agentEventsEmitter) OnToolCallEnd(_ string, callID, result string) {
	e.emit("toolend", map[string]string{
		"id":     callID,
		"result": result,
	})
}

func (e *agentEventsEmitter) OnDone(_ string) {
	e.emit("done", "")
}

func (e *agentEventsEmitter) OnError(_ string, msg string) {
	// First error wins — later calls (e.g. StartChat re-reporting the error
	// Chat already surfaced mid-stream) are dropped so the user sees one.
	if !e.errored.CompareAndSwap(false, true) {
		return
	}
	e.emit("error", msg)
}

// Compile-time check: agentEventsEmitter satisfies agent.AgentEvents.
var _ agent.AgentEvents = (*agentEventsEmitter)(nil)

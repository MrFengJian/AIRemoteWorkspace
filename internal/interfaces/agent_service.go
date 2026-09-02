package interfaces

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/schema"
	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
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

// ConversationDTO is a persisted agent conversation for the history list.
type ConversationDTO struct {
	ID           string `json:"id"`
	HostID       string `json:"hostId"`
	HostName     string `json:"hostName"`
	Title        string `json:"title"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int64  `json:"messageCount"`
}

// ConversationMessageDTO is one persisted user/assistant message.
type ConversationMessageDTO struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// SkillDTO is one agent skill's metadata for the input-box `/` picker.
type SkillDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ContextPathDTO is one entry of an @-mention directory listing.
type ContextPathDTO struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// AgentService exposes the AI agent to the frontend. Provider/model selection
// is per chat call; provider management lives in ModelProviderService.
type AgentService struct {
	app     *wailsapp.App
	runtime *agent.Runtime
	gate    *appsvc.PermissionGate
	convs   *appsvc.ConversationService
	skills  *appsvc.SkillService
}

// NewAgentService wires the AgentService. The *Application is injected via
// ServiceStartup. skills (may be nil) backs the input-box `/` skill picker.
func NewAgentService(runtime *agent.Runtime, gate *appsvc.PermissionGate, convs *appsvc.ConversationService, skills *appsvc.SkillService) *AgentService {
	return &AgentService{runtime: runtime, gate: gate, convs: convs, skills: skills}
}

// ListSkills returns the metadata of every available skill (the `/` picker).
func (a *AgentService) ListSkills() ([]SkillDTO, error) {
	if a.skills == nil {
		return []SkillDTO{}, nil
	}
	skills, err := a.skills.ListSkills()
	if err != nil {
		return nil, err
	}
	out := make([]SkillDTO, 0, len(skills))
	for _, s := range skills {
		out = append(out, SkillDTO{Name: s.Name, Description: s.Description})
	}
	return out, nil
}

// ListContextPaths lists a directory for the @-completion popup (remote
// session → SFTP listing; local session → disk listing).
func (a *AgentService) ListContextPaths(sessionID, dir string) ([]ContextPathDTO, error) {
	entries, err := a.runtime.ListContextPaths(sessionID, dir)
	if err != nil {
		return nil, err
	}
	out := make([]ContextPathDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, ContextPathDTO{Name: e.Name, IsDir: e.IsDir, Size: e.Size})
	}
	return out, nil
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
	// Bind the session to a persisted conversation (created on the first
	// turn). Failure only means history isn't stored — chat continues.
	if a.convs != nil {
		if _, err := a.convs.EnsureMapping(sessionID, message); err != nil {
			log.Printf("[AgentService] ensure conversation: %v", err)
		}
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

// ClearHistory forgets a session's conversation memory (the frontend also
// clears its local message list when the user clears the chat) and detaches
// the session from its persisted conversation — the next turn starts a new
// one.
func (a *AgentService) ClearHistory(sessionID string) error {
	if a.runtime != nil {
		a.runtime.ClearHistory(sessionID)
	}
	if a.convs != nil {
		a.convs.ClearMapping(sessionID)
	}
	return nil
}

// ListConversations returns all persisted agent conversations (newest
// first); the frontend filters by host.
func (a *AgentService) ListConversations() ([]ConversationDTO, error) {
	list, err := a.convs.List()
	if err != nil {
		return nil, err
	}
	out := make([]ConversationDTO, 0, len(list))
	for _, c := range list {
		count, _ := a.convs.Messages(c.ID)
		out = append(out, ConversationDTO{
			ID:           c.ID,
			HostID:       c.HostID,
			HostName:     c.HostName,
			Title:        c.Title,
			UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
			MessageCount: int64(len(count)),
		})
	}
	return out, nil
}

// GetConversationMessages returns a conversation's user/assistant messages
// in order.
func (a *AgentService) GetConversationMessages(conversationID string) ([]ConversationMessageDTO, error) {
	msgs, err := a.convs.Messages(conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]ConversationMessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ConversationMessageDTO{Role: m.Role, Content: m.Content})
	}
	return out, nil
}

// ResumeConversation points a terminal session at a persisted conversation
// and replays it into the agent's multi-turn memory, so follow-up questions
// keep context.
func (a *AgentService) ResumeConversation(sessionID, conversationID string) error {
	msgs, err := a.convs.Messages(conversationID)
	if err != nil {
		return err
	}
	hist := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "user" {
			hist = append(hist, schema.UserMessage(m.Content))
		} else {
			hist = append(hist, schema.AssistantMessage(m.Content, nil))
		}
	}
	a.runtime.RestoreHistory(sessionID, hist)
	a.convs.SetActive(sessionID, conversationID)
	return nil
}

// DeleteConversation removes a persisted conversation. If it was the active
// conversation of a session, that session's in-memory context is cleared too.
func (a *AgentService) DeleteConversation(conversationID string) error {
	affected, err := a.convs.Delete(conversationID)
	if err != nil {
		return err
	}
	if affected != "" && a.runtime != nil {
		a.runtime.ClearHistory(affected)
	}
	return nil
}

// ApproveToolCall resolves a pending approval request.
func (a *AgentService) ApproveToolCall(reqID string, approved bool) error {
	return a.gate.Resolve(reqID, approved)
}

// SetSessionPolicy sets the approval policy a session's agent runs (the
// dropdown in the agent input bar). "strict" asks for every WRITE/DANGEROUS
// call; "auto_write" silently approves WRITE and keeps asking for DANGEROUS.
// Unknown values normalize to strict.
func (a *AgentService) SetSessionPolicy(sessionID, policy string) error {
	if a.gate == nil {
		return fmt.Errorf("permission gate not available")
	}
	a.gate.SetSessionPolicy(sessionID, domain.SessionPolicy(policy))
	return nil
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

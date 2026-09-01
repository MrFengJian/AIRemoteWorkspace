package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ai-remote/workspace/internal/domain"
)

// ApprovalRequest is sent to the frontend when a WRITE/DANGEROUS tool needs
// the user's approval. The frontend shows a dialog and calls ApproveToolCall.
type ApprovalRequest struct {
	ReqID     string            `json:"reqId"`
	SessionID string            `json:"sessionId"`
	ToolName  string            `json:"toolName"`
	Permission domain.Permission `json:"permission"`
	Args      string            `json:"args"`
}

// ApprovalEmitter sends an approval request to the frontend (over Wails events).
// Implemented by the Wails AgentService.
type ApprovalEmitter interface {
	EmitApproval(req ApprovalRequest)
}

// PermissionGate implements tools.PermissionGate. READ tools pass through;
// WRITE/DANGEROUS tools block on a channel until the user responds — unless
// the session's policy auto-approves the tier (see domain.SessionPolicy:
// auto_write passes WRITE silently; DANGEROUS always reaches the user).
type PermissionGate struct {
	emitter ApprovalEmitter
	timeout time.Duration

	mu       sync.Mutex
	pending  map[string]chan bool            // reqID → approval channel
	policies map[string]domain.SessionPolicy // sessionID → approval policy
}

// NewPermissionGate builds a gate. timeout caps how long we wait for the user.
func NewPermissionGate(emitter ApprovalEmitter) *PermissionGate {
	return &PermissionGate{
		emitter:  emitter,
		timeout:  5 * time.Minute,
		pending:  make(map[string]chan bool),
		policies: make(map[string]domain.SessionPolicy),
	}
}

// SetEmitter wires the approval emitter after construction (used when the
// emitter is the Wails AgentService, which itself needs the gate at creation).
func (g *PermissionGate) SetEmitter(emitter ApprovalEmitter) {
	g.mu.Lock()
	g.emitter = emitter
	g.mu.Unlock()
}

// ErrDenied is returned when the user rejects a tool call.
var ErrDenied = errors.New("tool call denied by user")

// ErrApprovalTimeout is returned when the user doesn't respond in time.
var ErrApprovalTimeout = errors.New("tool call approval timed out")

// Check is called before every tool execution. READ auto-passes; WRITE is
// auto-passed when the session runs the auto_write policy; DANGEROUS always
// asks. Unknown sessions default to strict.
func (g *PermissionGate) Check(ctx context.Context, sessionID, toolName string, perm domain.Permission, argsJSON string) error {
	if perm == domain.PermissionRead {
		return nil
	}

	g.mu.Lock()
	policy := domain.NormalizeSessionPolicy(g.policies[sessionID])
	g.mu.Unlock()
	if policy == domain.PolicyAutoWrite && perm == domain.PermissionWrite {
		return nil
	}

	reqID := uuid.NewString()
	ch := make(chan bool, 1)

	g.mu.Lock()
	g.pending[reqID] = ch
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.pending, reqID)
		g.mu.Unlock()
	}()

	// Ask the frontend.
	g.mu.Lock()
	emitter := g.emitter
	g.mu.Unlock()
	if emitter != nil {
		emitter.EmitApproval(ApprovalRequest{
			ReqID:      reqID,
			SessionID:  sessionID,
			ToolName:   toolName,
			Permission: perm,
			Args:       argsJSON,
		})
	}

	// Block until the user responds, the context is cancelled, or we time out.
	timer := time.NewTimer(g.timeout)
	defer timer.Stop()
	select {
	case approved := <-ch:
		if approved {
			return nil
		}
		return ErrDenied
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrApprovalTimeout
	}
}

// Resolve is called by the Wails layer when the user approves/denies.
func (g *PermissionGate) Resolve(reqID string, approved bool) error {
	g.mu.Lock()
	ch, ok := g.pending[reqID]
	g.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending approval %s", reqID)
	}
	ch <- approved
	return nil
}

// SetSessionPolicy records the approval policy a session runs. Unknown values
// normalize to strict, so a malformed payload can only tighten, never widen.
func (g *PermissionGate) SetSessionPolicy(sessionID string, policy domain.SessionPolicy) {
	g.mu.Lock()
	g.policies[sessionID] = domain.NormalizeSessionPolicy(policy)
	g.mu.Unlock()
}

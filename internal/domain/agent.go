package domain

import "time"

// Agent represents an LLM-driven conversation that drives Tools to act on a
// target (local or remote). Phase 4 will flesh out the runtime.
type Agent struct {
	ID      string
	Model   string
	Target  AgentTarget
	History []Turn
}

// AgentTarget is what the Agent operates against.
type AgentTarget string

const (
	TargetLocal  AgentTarget = "local"
	TargetRemote AgentTarget = "remote" // a Host
)

// Turn is a single message in the Agent conversation (user / assistant /
// tool-call / tool-result). Detailed in Phase 4.
type Turn struct {
	Role    string
	Content string
}

// Conversation is a persisted agent chat, scoped to the host (HostID "" =
// local machine) it happened on. Resumable across app restarts.
type Conversation struct {
	ID        string
	HostID    string
	HostName  string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConversationMessage is one user/assistant message inside a Conversation.
// Tool steps are deliberately not persisted — the runtime's multi-turn memory
// replays only user/assistant turns, and the resumed view shows content.
type ConversationMessage struct {
	ID        int64
	Role      string // "user" | "assistant"
	Content   string
	CreatedAt time.Time
}

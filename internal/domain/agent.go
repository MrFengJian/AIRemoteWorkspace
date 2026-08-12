package domain

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

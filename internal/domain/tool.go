package domain

// Tool is the unified abstraction every AI action funnels through
// (AGENT.md §13). LLMs never touch SSH directly — they invoke Tools, which
// pass through Permission before hitting an Executor.
type Tool interface {
	Name() string
	Description() string
	// Permission classifies the action for the approval gate (READ/WRITE/DANGEROUS).
	Permission() Permission
	// Execute runs the tool. input is tool-specific; Result carries output or error.
	Execute(input any) Result
}

// Permission tier (AGENT.md §14). READ runs silently; WRITE prompts;
// DANGEROUS requires explicit user approval.
type Permission string

const (
	PermissionRead      Permission = "read"
	PermissionWrite     Permission = "write"
	PermissionDangerous Permission = "dangerous"
)

// SessionPolicy is the per-session approval aggressiveness the user picks in
// the agent input bar (AGENT.md §14). It decides who approves a WRITE — the
// gate or the user. DANGEROUS always reaches the user regardless of policy:
// on production hosts that decision is never delegated.
type SessionPolicy string

const (
	// PolicyStrict asks for every WRITE and DANGEROUS call (the default).
	PolicyStrict SessionPolicy = "strict"
	// PolicyAutoWrite silently approves WRITE calls; DANGEROUS still prompts.
	PolicyAutoWrite SessionPolicy = "auto_write"
)

// NormalizeSessionPolicy maps unknown/empty values to the safe default so a
// malformed frontend payload can never widen permissions.
func NormalizeSessionPolicy(p SessionPolicy) SessionPolicy {
	if p == PolicyAutoWrite {
		return PolicyAutoWrite
	}
	return PolicyStrict
}

// Result is what every Tool returns.
type Result struct {
	Output string
	Error  error
}

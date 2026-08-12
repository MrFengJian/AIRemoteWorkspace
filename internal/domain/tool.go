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

// Result is what every Tool returns.
type Result struct {
	Output string
	Error  error
}

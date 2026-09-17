package domain

import "time"

// Expert is one "digital employee" (数字员工) — a role-specialized ops expert
// persona the agent runtime can embody. Following the industry persona-card
// convention (Coze/Dify/GPTs-style bots), an expert bundles a structured
// identity (name/role/icon), the persona instructions, capability scoping
// (tool allowlist + bound skills), default model binding, interaction design
// (opening message + suggested prompts) and lifecycle flags.
//
// Security invariant: a persona can never elevate permissions — the approval
// contract is composed by the runtime (agent package), never by the expert.
type Expert struct {
	ID string `json:"id"`
	// Name is the display name (e.g. "K8s 运维专家"); Role the job title
	// (e.g. "Kubernetes 运维工程师").
	Name string `json:"name"`
	Role string `json:"role"`
	// Icon is a frontend lucide icon name (e.g. "Container"); Color a named
	// avatar gradient scheme. Both degrade gracefully to defaults.
	Icon  string `json:"icon"`
	Color string `json:"color"`
	// Description is the one-line responsibility summary shown in pickers.
	Description string `json:"description"`
	// SystemPrompt is the persona core (SOUL.md): identity, expertise,
	// working method, output contract and boundaries. The runtime layers the
	// fixed tool and permission contracts around it.
	SystemPrompt string `json:"systemPrompt"`
	// Heartbeat holds operational guidelines (HEARTBEAT.md): periodic
	// patrol/inspection routines the expert should propose on a cadence —
	// guidance only, never self-executed loops (the approval contract wins).
	Heartbeat string `json:"heartbeat,omitempty"`
	// AllowedTools scopes the toolset; empty = every default tool. Unknown
	// names are ignored at build time.
	AllowedTools []string `json:"allowedTools,omitempty"`
	// SkillRefs binds SKILL.md scenario packs: listed in the system prompt
	// (the model loads their content on demand via the skill tool).
	SkillRefs []string `json:"skillRefs,omitempty"`
	// Default model binding — empty = follow the session's current selection.
	ProviderID string `json:"providerId,omitempty"`
	Model      string `json:"model,omitempty"`
	// Policy is the default approval policy applied on expert switch
	// ("" / "strict" / "auto_write"); the user can still override it
	// per session afterwards.
	Policy string `json:"policy,omitempty"`
	// Temperature > 0 overrides the model's sampling temperature for this
	// expert; MaxSteps > 0 overrides the global step budget.
	Temperature float64 `json:"temperature,omitempty"`
	MaxSteps    int     `json:"maxSteps,omitempty"`
	// OpeningMessage greets the user on an empty chat; SuggestedPrompts are
	// the one-click starter questions rendered under it.
	OpeningMessage   string   `json:"openingMessage,omitempty"`
	SuggestedPrompts []string `json:"suggestedPrompts,omitempty"`
	// AutoSnapshot injects the deterministic health snapshot into the first
	// turn after this expert is activated (the SRE diagnostician's triage
	// context). Collection failure degrades to a note, never a failure.
	AutoSnapshot bool `json:"autoSnapshot,omitempty"`
	// Builtin marks experts seeded from the binary; they are editable but
	// never re-seeded over user edits, and deleting one only dismisses it
	// (Dismissed) so it can be restored by re-creating it.
	Builtin bool `json:"builtin,omitempty"`
	// Enabled hides an expert from the pickers without deleting it.
	Enabled bool `json:"enabled"`
	// Dismissed marks a deleted builtin: the seeder does not resurrect it,
	// and listing filters it out. Custom experts are hard-deleted instead.
	Dismissed bool `json:"dismissed,omitempty"`
	SortOrder int  `json:"sortOrder"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Builtin expert ids (stable across versions; rows are seeded on startup).
const (
	// ExpertIDGeneralAssistant is the default expert: the refactored
	// "general assistant" persona, always first in the roster.
	ExpertIDGeneralAssistant = "builtin-general-assistant"
	ExpertIDDiagnosticsSRE   = "builtin-diagnosis-sre"
	ExpertIDK8sOps           = "builtin-k8s-ops"
	ExpertIDK8sDeveloper     = "builtin-k8s-dev"
	ExpertIDDocker           = "builtin-docker"
	ExpertIDLinuxSys         = "builtin-linux-sys"
	ExpertIDDatabase         = "builtin-dba"
)

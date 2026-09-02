package domain

// AppConfig is the persisted application configuration.
// Stored in the `settings` table (Phase 1); sensitive fields would route
// through SecretStore per AGENT.md §9.
type AppConfig struct {
	SecurityMode SecurityMode `json:"securityMode"`
	DefaultShell string       `json:"defaultShell"`
	Theme        string       `json:"theme"`    // "light" | "dark" | "auto"
	UIFont       string       `json:"uiFont"`   // interface font family name
	FontSize     int          `json:"fontSize"` // interface font size in px
	CJKFont      string       `json:"cjkFont"`  // CJK (Chinese) font family name
	// Global terminal appearance defaults. Per-host values ("" / 0) fall back
	// to these; they are also where the appearance dialog persists changes
	// made in a LOCAL terminal tab (no host record to write to).
	// "" / 0 = built-in defaults (cobalt2 / Cascadia stack / 13px).
	TerminalTheme     string `json:"terminalTheme"`
	TerminalFont      string `json:"terminalFont"`
	TerminalFontSize  int    `json:"terminalFontSize"`
	// Terminal content highlighting (http/https links, ERROR/WARN keywords).
	// Inverted flags: an older settings row without the fields keeps both
	// highlights enabled — the out-of-box default.
	DisableLinkHighlight    bool `json:"disableLinkHighlight"`
	DisableKeywordHighlight bool `json:"disableKeywordHighlight"`
	// User-defined highlight rules: a regex and the color scheme used to
	// paint its matches. Invalid patterns are skipped by the renderer.
	HighlightRules []HighlightRule `json:"highlightRules,omitempty"`
	LLM               LLMConfig    `json:"llm"` // AI agent provider config (API key in SecretStore)
	// Keyboard shortcut overrides (Xshell-style). Key = command id
	// ("terminal.copy"), value = binding string ("Ctrl+Shift+C"). Only entries
	// the user changed from the default are stored; nil/empty = all defaults.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
	// Middle-click behavior on terminal panes (Xshell-style): "none" |
	// "pasteSelection" (default) | "pasteClipboard" | "sendEnter" |
	// "contextMenu".
	MiddleClickAction string `json:"middleClickAction"`
	// Host monitor panel auto-refresh interval in seconds (0 = default 60).
	MonitorIntervalSeconds int `json:"monitorIntervalSeconds"`
	// AI agent runtime tunables (提示词 / 最大步数等, user-adjustable).
	Agent AgentConfig `json:"agent"`
	// SFTP file-transfer tunables (streaming chunk size + size ceilings).
	Transfer TransferConfig `json:"transfer"`
}

// HighlightRule is a user-defined terminal content highlight: a regular
// expression and the color scheme used to paint its matches. The regex is
// JavaScript-flavored (compiled in the renderer); invalid patterns are
// ignored, never breaking the terminal.
type HighlightRule struct {
	Pattern string `json:"pattern"`
	// Color scheme id from the highlight palette (red/orange/yellow/green/
	// cyan/blue/purple/pink).
	Color string `json:"color"`
}

// TransferConfig holds the SFTP streaming-transfer tunables exposed in
// global settings. Zero values mean "use the built-in default" (filled in
// by the config service). Size ceilings exist to protect the user from
// accidental giant transfers, not from memory pressure — transfers stream
// with a fixed buffer regardless of file size.
type TransferConfig struct {
	// ChunkKB is the streaming chunk size in KB (also the progress-event
	// granularity). Default 256.
	ChunkKB int `json:"chunkKb"`
	// MaxUploadMB caps a single upload. Default 4096; raise it in settings
	// for effectively-unlimited transfers (0 falls back to the default).
	MaxUploadMB int `json:"maxUploadMb"`
	// MaxDownloadMB caps a single download. Default 4096.
	MaxDownloadMB int `json:"maxDownloadMb"`
}

// AgentConfig holds the AI agent runtime tunables exposed in global
// settings. Zero values mean "use the built-in default" (filled in by the
// config service), so an absent block in an older settings row upgrades
// cleanly.
type AgentConfig struct {
	// MaxSteps bounds the ReAct loop per turn (model + tool node executions;
	// one tool round ≈ 2 steps). Default 100.
	MaxSteps int `json:"maxSteps"`
	// HistoryTurns caps how many past (user, assistant) turns are replayed
	// as conversation memory. Default 40.
	HistoryTurns int `json:"historyTurns"`
	// ToolOutputLimitKB caps a single tool result fed back to the model
	// (head+tail kept, middle elided). Default 64.
	ToolOutputLimitKB int `json:"toolOutputLimitKB"`
	// CustomInstructions is appended to the built-in system prompt on every
	// turn — standing preferences, host context, tone. Empty = built-in
	// prompt only.
	CustomInstructions string `json:"customInstructions"`
}

// LLMConfig holds the non-sensitive LLM provider settings. The API Key is
// stored in the OS vault (SecretStore), never here.
//
// Deprecated: superseded by ModelProvider (multi-provider). Kept only so the
// legacy single-provider config can be migrated on first run.
type LLMConfig struct {
	BaseURL string `json:"baseUrl"` // e.g. "https://api.openai.com/v1" or a compatible endpoint
	Model   string `json:"model"`   // e.g. "gpt-4o", "deepseek-chat"
}

// ModelProvider is a user-configured OpenAI-compatible LLM provider. The API
// key is stored in the OS vault under a per-provider ref, never here.
type ModelProvider struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	BaseURL string   `json:"baseUrl"`
	Models  []string `json:"models"`  // recorded model list offered in session pickers
	Enabled bool     `json:"enabled"` // disabled providers are hidden from pickers and rejected at chat time
}

// LLMEndpoint is a resolved (provider, model) selection ready for building a
// chat model. APIKey may be empty for local endpoints (Ollama, LM Studio, …).
type LLMEndpoint struct {
	BaseURL string
	Model   string
	APIKey  string
}

// SecurityMode governs how credentials are handled (AGENT.md §10).
type SecurityMode string

const (
	SecurityConvenience SecurityMode = "convenience"
	SecurityBalanced    SecurityMode = "balanced" // default
	SecuritySecure      SecurityMode = "secure"
)

// DefaultConfig returns the out-of-the-box config applied on first launch.
func DefaultConfig() AppConfig {
	return AppConfig{
		SecurityMode: SecurityBalanced,
		DefaultShell: "/bin/bash",
		Theme:        "dark",
		UIFont:       "",
		FontSize:     13,
		CJKFont:      "",
		LLM: LLMConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
		MiddleClickAction: "pasteSelection",
		MonitorIntervalSeconds: 60,
		Agent: AgentConfig{
			MaxSteps:          100,
			HistoryTurns:      40,
			ToolOutputLimitKB: 64,
		},
		Transfer: TransferConfig{
			ChunkKB:       256,
			MaxUploadMB:   4096,
			MaxDownloadMB: 4096,
		},
	}
}

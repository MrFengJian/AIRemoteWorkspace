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
	LLM               LLMConfig    `json:"llm"` // AI agent provider config (API key in SecretStore)
	// Keyboard shortcut overrides (Xshell-style). Key = command id
	// ("terminal.copy"), value = binding string ("Ctrl+Shift+C"). Only entries
	// the user changed from the default are stored; nil/empty = all defaults.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
	// Middle-click behavior on terminal panes (Xshell-style): "none" |
	// "pasteSelection" (default) | "pasteClipboard" | "sendEnter" |
	// "contextMenu".
	MiddleClickAction string `json:"middleClickAction"`
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
	}
}

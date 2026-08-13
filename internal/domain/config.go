package domain

// AppConfig is the persisted application configuration.
// Stored in the `settings` table (Phase 1); sensitive fields would route
// through SecretStore per AGENT.md §9.
type AppConfig struct {
	SecurityMode  SecurityMode `json:"securityMode"`
	DefaultShell  string       `json:"defaultShell"`
	Theme         string       `json:"theme"`
	TerminalTheme string       `json:"terminalTheme"` // terminal colour scheme id
	LLM           LLMConfig    `json:"llm"`           // AI agent provider config (API key in SecretStore)
}

// LLMConfig holds the non-sensitive LLM provider settings. The API Key is
// stored in the OS vault (SecretStore), never here.
type LLMConfig struct {
	BaseURL string `json:"baseUrl"` // e.g. "https://api.openai.com/v1" or a compatible endpoint
	Model   string `json:"model"`   // e.g. "gpt-4o", "deepseek-chat"
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
		SecurityMode:  SecurityBalanced,
		DefaultShell:  "/bin/bash",
		Theme:         "dark",
		TerminalTheme: "cobalt2",
		LLM: LLMConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
	}
}

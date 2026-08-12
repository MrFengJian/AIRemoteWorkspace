package domain

// AppConfig is the persisted application configuration.
// Stored in the `settings` table (Phase 1); sensitive fields would route
// through SecretStore per AGENT.md §9.
type AppConfig struct {
	SecurityMode   SecurityMode `json:"securityMode"`
	DefaultShell   string       `json:"defaultShell"`
	Theme          string       `json:"theme"`
	TerminalTheme  string       `json:"terminalTheme"` // terminal colour scheme id
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
	}
}

// Package domain holds pure business models — no I/O, no framework code.
// AGENT.md §7: domain captures the nouns of the product (Host, Session,
// Tool, Agent) so that application/infrastructure layers depend on these
// types rather than the other way around.
package domain

import "time"

// Host is a single remote machine a user can connect to.
//
// Security note (AGENT.md §9): secrets (password, key material) are NEVER
// stored on this struct or in SQLite. Only SecretRef — an opaque handle into
// the OS keychain — is persisted. Phase 5's SecretStore resolves the ref at
// connect time.
type Host struct {
	ID        string
	Name      string
	Host      string
	Port      int
	Username  string
	AuthType  AuthType
	SecretRef string // keychain handle; empty in Secure mode (ask each time)

	TerminalTheme string   // per-host terminal colour scheme id; "" = use default
	// Per-host terminal font overrides; "" / 0 mean "follow the global
	// settings" (AppConfig.TerminalFont / TerminalFontSize).
	TerminalFont     string // font family name; "" = global setting
	TerminalFontSize int    // px; 0 = global setting
	Group         string   // host group: "test" | "stage" | "production" | custom
	Tags          []string // free-form labels, e.g. ["nginx", "us-east-1"]
	OS            string   // detected distro id, e.g. "ubuntu"; read-only, set at connect time

	// Last model provider + model the agent used on this host. Hidden
	// preference persisted by the agent panel when the user changes the inline
	// selector; deliberately NOT part of the host edit form.
	AgentProviderID string
	AgentModel      string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AuthType describes how a Host authenticates.
type AuthType string

const (
	AuthPassword AuthType = "password"
	AuthKey      AuthType = "key"
	AuthAgent    AuthType = "agent" // ssh-agent
)

// SessionID uniquely identifies an active connection / terminal session.
type SessionID string

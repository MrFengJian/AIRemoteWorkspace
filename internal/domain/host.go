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
	// Path to the private key for AuthKey hosts. Not a secret (no keychain
	// needed) but must be persisted — background dials (tunnel auto-start,
	// reconnect) have no form in front of the user to supply it.
	KeyPath string

	TerminalTheme string // per-host terminal colour scheme id; "" = use default
	// Per-host terminal font overrides; "" / 0 mean "follow the global
	// settings" (AppConfig.TerminalFont / TerminalFontSize).
	TerminalFont     string   // font family name; "" = global setting
	TerminalFontSize int      // px; 0 = global setting
	Group            string   // host group: "test" | "stage" | "production" | custom
	Tags             []string // free-form labels, e.g. ["nginx", "us-east-1"]
	OS               string   // detected distro id, e.g. "ubuntu"; read-only, set at connect time

	// Last model provider + model the agent used on this host. Hidden
	// preference persisted by the agent panel when the user changes the inline
	// selector; deliberately NOT part of the host edit form.
	AgentProviderID string
	AgentModel      string

	// SSH tunnel definitions (host settings form; a host may have several).
	// Enabled tunnels auto-start when a session opens on this host; the
	// tunnel manager dedupes per host + rule.
	Tunnels []TunnelConfig

	// How to reach this host before SSH: directly (nil), through a jump
	// host (堡垒机, itself possibly behind another jump / proxy — chains
	// form by recursion), or through an HTTP CONNECT / SOCKS5 proxy.
	// Resolved into a concrete SshRoute at connect time by ProxyService.
	Proxy *ProxyConfig

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProxyKind enumerates the ways a host can be reached before SSH.
type ProxyKind string

const (
	ProxyNone   ProxyKind = ""       // direct connection (default)
	ProxyJump   ProxyKind = "jump"   // via another managed host (SSH 跳板)
	ProxyHTTP   ProxyKind = "http"   // via an HTTP CONNECT proxy
	ProxySocks5 ProxyKind = "socks5" // via a SOCKS5 proxy
)

// ProxyConfig is the host's pre-SSH reachability setting (host edit form,
// 连接 tab). Exactly one branch is meaningful per Kind:
//
//	jump   → HostID (the managed host to hop through)
//	http   → Addr (+ optional Username / vault password)
//	socks5 → Addr (+ optional Username / vault password)
type ProxyConfig struct {
	Kind ProxyKind `json:"kind"`
	// HostID names the managed host used as the jump box (jump kind only).
	// The jump box's own Proxy config, if any, is applied recursively.
	HostID string `json:"hostId,omitempty"`
	// Addr is the proxy address "host:port" (http / socks5 kinds).
	Addr string `json:"addr,omitempty"`
	// Username for authenticated proxies (http / socks5 kinds). The matching
	// password lives in the OS vault under SecretProxyPassword, never here.
	Username string `json:"username,omitempty"`
}

// HasTransport reports whether the config actually routes the connection
// somewhere (a zero/none config means direct).
func (p *ProxyConfig) HasTransport() bool {
	return p != nil && p.Kind != ProxyNone
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

package domain

// SshRoute is the RESOLVED path to reach a host before its SSH handshake —
// the plan the connection layer executes: dial the innermost transport (a
// direct TCP connection or a proxy), then walk the jump chain, opening an
// SSH session on every hop and tunneling the next connection through it.
//
// Built by application.ProxyService (RouteFor); consumed by
// infrastructure/ssh (Dial). Lives in domain so both sides can depend on it
// without an import cycle.
type SshRoute struct {
	// Jumps is the jump chain from OUTERMOST to INNERMOST: jumps[0] is
	// reached through Proxy (or directly), jumps[i] is reached through the
	// SSH connection to jumps[i-1], and the target is reached through the
	// last jump. Empty = no jump hosts.
	Jumps []SshJump `json:"jumps,omitempty"`
	// Proxy is a non-SSH transport used to reach jumps[0] — or the target
	// itself when there are no jumps. Nil = direct TCP.
	Proxy *ProxyEndpoint `json:"proxy,omitempty"`
}

// SshJump is one SSH jump box on the route: a full SSH connection is
// established to it (host-key verified under HostID, authenticated with
// Creds) and the next connection is tunneled through a direct-tcpip channel.
type SshJump struct {
	HostID   string // known_hosts identity of the jump box
	Addr     string // "host:port" of the jump box
	Username string
	Creds    Credentials // resolved (vault included) before the dial
}

// ProxyEndpoint is a plain (non-SSH) proxy: HTTP CONNECT or SOCKS5.
type ProxyEndpoint struct {
	Kind     string // "http" | "socks5"
	Addr     string // "host:port"
	Username string
	Password string
}

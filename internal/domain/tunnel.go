package domain

import "fmt"

// TunnelType selects how an SSH tunnel exposes a network on the far side —
// the three forms Xshell offers (AGENT.md §16.5).
type TunnelType string

const (
	// TunnelLocal forwards a local listen port to (TargetHost, TargetPort)
	// as seen from the SSH server — `ssh -L`. Use-case: reach a database or
	// web panel bound to the server's internal/loopback interface.
	TunnelLocal TunnelType = "local"
	// TunnelRemote is the reverse: the SSH SERVER listens on
	// (BindHost, ListenPort) and every connection there is carried back to
	// (TargetHost, TargetPort) as seen from the local machine — `ssh -R`.
	// Use-case: expose a service running on your machine to the server (or
	// its network).
	TunnelRemote TunnelType = "remote"
	// TunnelDynamic exposes a SOCKS5 proxy on (BindHost, ListenPort) whose
	// connects originate from the SSH server — `ssh -D`.
	TunnelDynamic TunnelType = "dynamic"
)

// TunnelConfig is a per-host SSH tunnel definition, edited in the host
// settings and persisted with the host (no secrets inside — the tunnel
// authenticates with the host's credentials).
type TunnelConfig struct {
	Enabled    bool       `json:"enabled"`
	Type       TunnelType `json:"type"`
	ListenPort int        `json:"listenPort"` // local/dynamic: local machine; remote: the SSH server
	// Listen bind address: the local machine for local/dynamic, the server
	// for remote. "" = 127.0.0.1 (only the tunnel's own machine); "0.0.0.0"
	// exposes the listener to the network.
	BindHost string `json:"bindHost,omitempty"`
	// Forward target. local: resolved from the SERVER's side (e.g. its
	// loopback MySQL); remote: resolved from the LOCAL machine's side.
	TargetHost string `json:"targetHost,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
}

// ListenHost returns the effective bind address (empty = loopback only).
func (c TunnelConfig) ListenHost() string {
	if c.BindHost == "" {
		return "127.0.0.1"
	}
	return c.BindHost
}

// Key returns a stable identity for the rule — used by the tunnel manager to
// dedupe supervisors and by the UI to match rules with live statuses.
func (c TunnelConfig) Key() string {
	return fmt.Sprintf("%s|%s|%d|%s|%d", c.Type, c.BindHost, c.ListenPort, c.TargetHost, c.TargetPort)
}

// ValidPort reports whether p is a usable TCP port.
func ValidPort(p int) bool { return p >= 1 && p <= 65535 }

// Normalized reports whether the config carries enough information to run.
func (c TunnelConfig) Valid() bool {
	if !c.Enabled || !ValidPort(c.ListenPort) {
		return false
	}
	switch c.Type {
	case TunnelDynamic:
		return true
	case TunnelLocal, TunnelRemote:
		return c.TargetHost != "" && ValidPort(c.TargetPort)
	default:
		return false
	}
}

// TunnelState is the lifecycle state of a running (or attempted) tunnel.
type TunnelState string

const (
	TunnelStopped      TunnelState = "stopped"      // not running (never started or stopped by user)
	TunnelStarting     TunnelState = "starting"     // dialing the first connection
	TunnelConnected    TunnelState = "connected"    // SSH up, listener serving
	TunnelReconnecting TunnelState = "reconnecting" // connection lost; backing off before redial
	TunnelError        TunnelState = "error"        // fatal setup problem (e.g. port already in use)
)

// TunnelStatus is the manager's view of one host's tunnel — pushed to the
// frontend on every change and returned by ListTunnels. One host can have
// several; Key identifies the rule within the host.
type TunnelStatus struct {
	HostID    string       `json:"hostId"`
	HostName  string       `json:"hostName"`
	Key       string       `json:"key"` // rule identity (TunnelConfig.Key)
	Config    TunnelConfig `json:"config"`
	State     TunnelState  `json:"state"`
	LastError string       `json:"lastError,omitempty"` // human-readable, shown in the panel
	Retries   int          `json:"retries"`             // consecutive failed dials in the current cycle
}

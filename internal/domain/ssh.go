package domain

// SSH connection / host-key types.
//
// These describe the *runtime* data needed to dial a host. Credentials live
// only in memory for Phase 2 — persistence of secrets is Phase 5 (SecretStore).

// HostKey is a recorded fingerprint for a host, used for known_hosts-style
// verification (AGENT.md §8).
type HostKey struct {
	HostID      string
	Algorithm   string // e.g. "ssh-ed25519"
	Fingerprint string // SHA256:... hex form
}

// Credentials carries the secret material for a single connection attempt.
// It is never persisted: callers build it from UI input + an optional
// SecretStore lookup (Phase 5).
type Credentials struct {
	// Password, when AuthType == AuthPassword. Empty otherwise.
	Password string
	// KeyPath points at a private key file on disk (AuthKey). The file's
	// *contents* are read at dial time and never stored.
	KeyPath string
	// KeyPassphrase unlocks KeyPath if it is encrypted. Memory-only.
	KeyPassphrase string
	// UseAgent is true when AuthType == AuthAgent (ssh-agent).
	UseAgent bool
}

// PtySize is the initial / resized dimensions of a terminal PTY.
type PtySize struct {
	Cols int
	Rows int
}

package domain

import "time"

// Session represents an active connection to a Host (SSH, terminal, SFTP).
// Phase 2/3 will populate this from the SSH Runtime.
type Session struct {
	ID        SessionID
	HostID    string
	StartedAt time.Time
	// Status will be one of: connecting, connected, closed, error.
	Status SessionStatus
}

type SessionStatus string

const (
	SessionConnecting SessionStatus = "connecting"
	SessionConnected  SessionStatus = "connected"
	SessionClosed     SessionStatus = "closed"
	SessionError      SessionStatus = "error"
)

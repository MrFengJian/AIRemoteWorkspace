// GORM model definitions for the SQLite schema. These drive AutoMigrate:
// the tables are created/altered automatically on startup from these structs,
// replacing the old hand-maintained schema.sql.
package sqlite

import "time"

// GORM model definitions for the SQLite schema. These drive AutoMigrate:
// the tables are created/altered automatically on startup from these structs,
// replacing the old hand-maintained schema.sql.
//
// Each model pins its table name via TableName() to match the pre-existing
// schema (hosts, host_keys, settings, sessions, secret_refs) — GORM would
// otherwise pluralise the struct names (hostModel → host_models), silently
// creating a second, empty set of tables.

// hostModel mirrors the hosts table (domain.Host + persistence concerns).
type hostModel struct {
	ID            string `gorm:"primaryKey;size:64"`
	Name          string `gorm:"not null;size:200"`
	Host          string `gorm:"not null;size:255"`
	Port          int    `gorm:"not null;default:22"`
	Username      string `gorm:"not null;size:100"`
	AuthType      string `gorm:"not null;default:password;size:20"`
	SecretRef     string `gorm:"not null;default:'';size:255"`
	// Private key path for AuthKey hosts (not a secret; persisted so
	// background dials — tunnels, reconnect — can authenticate).
	KeyPath       string `gorm:"not null;default:'';size:255"`
	TerminalTheme string `gorm:"not null;default:'';size:50"`
	// Per-host terminal font overrides; "" / 0 = follow the global settings.
	TerminalFont     string `gorm:"not null;default:'';size:100"`
	TerminalFontSize int    `gorm:"not null;default:0"`
	Group            string `gorm:"not null;default:'';size:50;index"`
	Tags          string `gorm:"not null;default:'[]';type:text"` // JSON array
	OS            string `gorm:"not null;default:'';size:50"`     // detected distro id (read-only)
	// Last-used agent model preference (hidden; not in the host edit form).
	AgentProviderID string `gorm:"not null;default:'';size:64"`
	AgentModel      string `gorm:"not null;default:'';size:200"`
	// SSH tunnel definition (domain.TunnelConfig as JSON; '' = no tunnel).
	Tunnel string `gorm:"not null;default:'';type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName pins the table to "hosts" (GORM would otherwise pluralise the
// struct name to "host_models", which would create a second empty table).
func (hostModel) TableName() string { return "hosts" }

// hostKeyModel mirrors host_keys (known_hosts-style fingerprints).
type hostKeyModel struct {
	HostID      string `gorm:"primaryKey;size:64"`
	Algorithm   string `gorm:"primaryKey;size:64"`
	Fingerprint string `gorm:"not null;size:128"`
	FirstSeen   time.Time
	LastSeen    time.Time
}

func (hostKeyModel) TableName() string { return "host_keys" }

// settingModel is a key/value settings row.
type settingModel struct {
	Key   string `gorm:"primaryKey;size:64"`
	Value string `gorm:"not null"`
}

func (settingModel) TableName() string { return "settings" }

// sessionModel records connection lifecycle (hostory).
type sessionModel struct {
	ID        string `gorm:"primaryKey;size:64"`
	HostID    string `gorm:"not null;size:64;index"`
	StartedAt time.Time
	EndedAt   *time.Time
	Status    string `gorm:"not null;default:connecting;size:20"`
}

func (sessionModel) TableName() string { return "sessions" }

// secretRefModel is the Phase 5 secret reference placeholder.
type secretRefModel struct {
	Ref       string `gorm:"primaryKey;size:128"`
	Kind      string `gorm:"not null;size:20"`
	HostID    string `gorm:"size:64"`
	CreatedAt time.Time
}

func (secretRefModel) TableName() string { return "secret_refs" }

// conversationModel is one persisted agent conversation (chat history),
// scoped to the host (or local machine) it happened on. No DB-level foreign
// key to conversationMessageModel — children are cleaned up in the repo.
type conversationModel struct {
	ID        string `gorm:"primaryKey;size:64"`
	HostID    string `gorm:"not null;default:'';size:64;index"` // "" = local machine
	HostName  string `gorm:"not null;default:'';size:200"`
	Title     string `gorm:"not null;default:'';size:200"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (conversationModel) TableName() string { return "conversations" }

// conversationMessageModel is one user/assistant message in a conversation.
type conversationMessageModel struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	ConversationID string `gorm:"not null;size:64;index"`
	Role           string `gorm:"not null;size:20"` // "user" | "assistant"
	Content        string `gorm:"not null;type:text"`
	CreatedAt      time.Time
}

func (conversationMessageModel) TableName() string { return "conversation_messages" }

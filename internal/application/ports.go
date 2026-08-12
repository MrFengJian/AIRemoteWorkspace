// Package application orchestrates business flows on top of domain types.
// It depends only on domain and on port interfaces defined here — never on
// concrete infrastructure (AGENT.md §7 layering).
package application

import "github.com/ai-remote/workspace/internal/domain"

// ConfigService is the port the Wails ConfigService implements.
// Keeping it as an interface here means the UI layer can be tested against a
// stub without a database.
type ConfigService interface {
	GetAppConfig() (domain.AppConfig, error)
	SetAppConfig(cfg domain.AppConfig) error
}

// HostRepository is the persistence port for hosts (Phase 2 fills the impl).
type HostRepository interface {
	List() ([]domain.Host, error)
	Get(id string) (domain.Host, error)
	Save(h domain.Host) error
	Delete(id string) error
}

// HostConnection is the SSH Runtime port (Phase 2).
type HostConnection interface {
	Connect(host domain.Host) (domain.Session, error)
	Close(sessionID domain.SessionID) error
}

// ToolRegistry holds the available Tools and dispatches calls through the
// permission gate (Phase 4).
type ToolRegistry interface {
	Registered() []domain.Tool
}

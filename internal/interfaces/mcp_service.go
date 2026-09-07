package interfaces

import (
	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/mcpserver"
)

// MCPService exposes the local MCP server's runtime state to the settings
// UI. Enabling/port changes go through the normal config flow
// (ConfigService.SetAppConfig → Server.ApplyConfig); this service adds
// read-only status plus token rotation.
type MCPService struct {
	server *mcpserver.Server
}

// NewMCPService wires the Wails MCPService to the running server.
func NewMCPService(server *mcpserver.Server) *MCPService {
	return &MCPService{server: server}
}

// ServiceName lets Wails register the service under a stable name.
func (s *MCPService) ServiceName() string { return "MCPService" }

// Status returns the MCP server's runtime state (running/port/token/URL/
// last error) for the settings page.
func (s *MCPService) Status() domain.MCPStatus {
	return s.server.Status()
}

// RegenerateToken replaces the bearer token — existing client configs must
// be updated — persists it, and returns the refreshed status.
func (s *MCPService) RegenerateToken() domain.MCPStatus {
	return s.server.RegenerateToken()
}

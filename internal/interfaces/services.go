// Package interfaces adapts application services to the Wails binding surface.
//
// A Wails Service is a plain Go struct whose exported methods become callable
// from the frontend (Wails generates TS bindings for them). This package keeps
// that adapter thin: each Service delegates to an application port so business
// logic stays testable without the framework.
package interfaces

import (
	"context"
	"fmt"
	"net"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// --- SystemService -------------------------------------------------------

// SystemInfoResult mirrors what the frontend StatusBar displays.
// Field names become the TS property names in generated bindings.
type SystemInfoResult struct {
	AppName   string `json:"appName"`
	Version   string `json:"version"`
	Platform  string `json:"platform"`
	GoVersion string `json:"goVersion"`
}

// SystemService exposes ambient runtime info to the UI. Phase 6's MCP
// `system_info` tool will reuse this.
type SystemService struct {
	appName string
	version string
}

// NewSystemService constructs a SystemService. appName/version are injected
// from main so this package stays free of build constants.
func NewSystemService(appName, version string) *SystemService {
	return &SystemService{appName: appName, version: version}
}

// ServiceName lets Wails register the service under a stable name.
func (s *SystemService) ServiceName() string { return "SystemService" }

// ServiceStartup runs when the service is registered with the app.
func (s *SystemService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error { return nil }

// SystemInfo returns the runtime info shown in the StatusBar.
func (s *SystemService) SystemInfo() SystemInfoResult {
	return SystemInfoResult{
		AppName:   s.appName,
		Version:   s.version,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		GoVersion: runtime.Version(),
	}
}

// LocalIPResult carries the machine's primary local IP address.
type LocalIPResult struct {
	IP string `json:"ip"`
}

// GetLocalIP returns the machine's primary outbound IPv4 address by dialing
// a public address (no actual connection is established — UDP dial just
// resolves the route). Used by the terminal right-click "paste local IP".
func (s *SystemService) GetLocalIP() (LocalIPResult, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return LocalIPResult{}, err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return LocalIPResult{IP: localAddr.IP.String()}, nil
}

// --- ConfigService -------------------------------------------------------

// ConfigService exposes the application ConfigService to the frontend.
// (Same struct name on both sides is intentional; this lives in a different
// package so it doesn't shadow the application interface.)
type ConfigService struct {
	svc appsvc.ConfigService
}

// NewConfigService wires the Wails ConfigService to its application port.
func NewConfigService(svc appsvc.ConfigService) *ConfigService {
	return &ConfigService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (c *ConfigService) ServiceName() string { return "ConfigService" }

// ServiceStartup runs when the service is registered with the app.
func (c *ConfigService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error { return nil }

// GetAppConfig returns the persisted application configuration.
// Returning the concrete domain.AppConfig lets Wails emit typed TS bindings.
func (c *ConfigService) GetAppConfig() (domain.AppConfig, error) {
	return c.svc.GetAppConfig()
}

// SetAppConfig persists a new configuration.
func (c *ConfigService) SetAppConfig(cfg domain.AppConfig) error {
	return c.svc.SetAppConfig(cfg)
}

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
	"os"
	"os/exec"
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
	appName  string
	version  string
	dataDirs *appsvc.DataDirService
	app      *application.App
}

// NewSystemService constructs a SystemService. appName/version are injected
// from main so this package stays free of build constants. dataDirs (may be
// nil) backs the data-directory display/migration in the settings page.
func NewSystemService(appName, version string, dataDirs *appsvc.DataDirService) *SystemService {
	return &SystemService{appName: appName, version: version, dataDirs: dataDirs}
}

// ServiceName lets Wails register the service under a stable name.
func (s *SystemService) ServiceName() string { return "SystemService" }

// ServiceStartup runs when the service is registered with the app.
func (s *SystemService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.app = application.Get()
	return nil
}

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

// ── Data directory (settings → advanced) ---------------------------------

// OpenDataDir opens the current data directory in the OS file browser
// (Explorer / Finder / xdg-open).
func (s *SystemService) OpenDataDir() error {
	if s.dataDirs == nil {
		return fmt.Errorf("data directory service not available")
	}
	dir := s.dataDirs.Current()
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("data directory %q: %w", dir, err)
	}
	switch runtime.GOOS {
	case "windows":
		// explorer returns a non-zero exit code even on success — ignore it.
		return exec.Command("explorer", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// DataDirInfoDTO describes the current data directory.
type DataDirInfoDTO struct {
	Path       string `json:"path"`
	IsDefault  bool   `json:"isDefault"`
	TotalBytes int64  `json:"totalBytes"`
}

// GetDataDirInfo returns the current data directory (path + size + whether
// it is still the default location).
func (s *SystemService) GetDataDirInfo() (DataDirInfoDTO, error) {
	if s.dataDirs == nil {
		return DataDirInfoDTO{}, fmt.Errorf("data directory service not available")
	}
	info := s.dataDirs.Info()
	return DataDirInfoDTO{
		Path:       info.Path,
		IsDefault:  info.IsDefault,
		TotalBytes: info.TotalBytes,
	}, nil
}

// PickDataDir opens a native directory chooser and returns the picked path
// ("" when the user cancels).
func (s *SystemService) PickDataDir() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("application not ready")
	}
	path, err := s.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("选择数据目录").
		PromptForSingleSelection()
	if err != nil {
		return "", nil // dialog failure/cancel → treat as cancel
	}
	return path, nil
}

// MigrateDataDir moves the whole data directory (database + skills) to the
// target and repoints the app in place — the UI keeps working from the new
// location without a restart.
func (s *SystemService) MigrateDataDir(target string) (DataDirInfoDTO, error) {
	if s.dataDirs == nil {
		return DataDirInfoDTO{}, fmt.Errorf("data directory service not available")
	}
	if err := s.dataDirs.Migrate(target); err != nil {
		return DataDirInfoDTO{}, err
	}
	info := s.dataDirs.Info()
	return DataDirInfoDTO{
		Path:       info.Path,
		IsDefault:  info.IsDefault,
		TotalBytes: info.TotalBytes,
	}, nil
}

// --- ConfigService -------------------------------------------------------

// ConfigService exposes the application ConfigService to the frontend.
// (Same struct name on both sides is intentional; this lives in a different
// package so it doesn't shadow the application interface.)
type ConfigService struct {
	svc appsvc.ConfigService
	app *application.App
}

// NewConfigService wires the Wails ConfigService to its application port.
func NewConfigService(svc appsvc.ConfigService) *ConfigService {
	return &ConfigService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (c *ConfigService) ServiceName() string { return "ConfigService" }

// ServiceStartup runs when the service is registered with the app.
func (c *ConfigService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	c.app = application.Get()
	return nil
}

// GetAppConfig returns the persisted application configuration.
// Returning the concrete domain.AppConfig lets Wails emit typed TS bindings.
func (c *ConfigService) GetAppConfig() (domain.AppConfig, error) {
	return c.svc.GetAppConfig()
}

// SetAppConfig persists a new configuration and broadcasts "config:changed"
// to every window — settings are global but each window applies its own
// chrome (theme/fonts/shortcuts), so aux windows like the standalone SFTP
// window follow changes made anywhere.
func (c *ConfigService) SetAppConfig(cfg domain.AppConfig) error {
	if err := c.svc.SetAppConfig(cfg); err != nil {
		return err
	}
	if c.app != nil {
		c.app.Event.Emit("config:changed")
	}
	return nil
}

package interfaces

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"sync"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
)

// WindowService opens auxiliary app windows from the frontend. Currently:
// the standalone SFTP workbench window (dual-pane local ⋅ remote file
// manager), one per host — re-opening focuses the existing window.
type WindowService struct {
	hosts *appsvc.HostService
	app   *wailsapp.App

	mu          sync.Mutex
	sftpWindows map[string]*wailsapp.WebviewWindow
}

// NewWindowService wires the Wails WindowService to the host service (the
// window is titled with, and validated against, the requested host).
func NewWindowService(hosts *appsvc.HostService) *WindowService {
	return &WindowService{hosts: hosts, sftpWindows: make(map[string]*wailsapp.WebviewWindow)}
}

// ServiceName lets Wails register the service under a stable name.
func (s *WindowService) ServiceName() string { return "WindowService" }

// ServiceStartup captures the Application handle so new windows can be
// created on demand (same pattern as SftpService).
func (s *WindowService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	s.app = wailsapp.Get()
	return nil
}

// OpenSftpWindow opens the dual-pane SFTP workbench for the host in a new
// native window. The window loads "#/sftp-window?host=<id>" — the frontend
// router mounts the workbench (instead of the main app shell) when it sees
// that hash. A second call for the same host focuses the existing window.
func (s *WindowService) OpenSftpWindow(hostID string) error {
	host, err := s.hosts.Get(hostID)
	if err != nil {
		return fmt.Errorf("open sftp window: %w", err)
	}
	s.mu.Lock()
	existing := s.sftpWindows[hostID]
	s.mu.Unlock()
	if existing != nil && existing.IsVisible() {
		existing.Focus()
		return nil
	}
	if s.app == nil {
		return fmt.Errorf("app not started")
	}
	win := s.app.Window.NewWithOptions(wailsapp.WebviewWindowOptions{
		Title:     "SFTP · " + host.Name,
		Width:     1200,
		Height:    760,
		MinWidth:  880,
		MinHeight: 520,
		// Frameless like the main window: the frontend TitleBar draws the
		// drag region and window controls (macOS keeps the native bar).
		Frameless: runtime.GOOS != "darwin",
		Mac: wailsapp.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                wailsapp.MacBackdropTranslucent,
			TitleBar:                wailsapp.MacTitleBarHiddenInset,
		},
		BackgroundColour: wailsapp.NewRGB(10, 10, 14),
		URL:              "/#/sftp-window?host=" + url.QueryEscape(hostID),
	})
	s.mu.Lock()
	s.sftpWindows[hostID] = win
	s.mu.Unlock()
	return nil
}

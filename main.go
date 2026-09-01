package main

import (
	"embed"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/adrg/xdg"
	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
	"github.com/ai-remote/workspace/internal/infrastructure/agent"
	"github.com/ai-remote/workspace/internal/infrastructure/localpty"
	"github.com/ai-remote/workspace/internal/infrastructure/secret"
	"github.com/ai-remote/workspace/internal/infrastructure/sftp"
	"github.com/ai-remote/workspace/internal/infrastructure/sqlite"
	"github.com/ai-remote/workspace/internal/infrastructure/ssh"
	"github.com/ai-remote/workspace/internal/interfaces"
)

// appMetadata is the single source of truth for identity surfaced through
// the UI and, later, the MCP system_info tool.
const (
	appName    = "AI Remote Workspace"
	appVersion = "0.1.0"
)

// Wails embeds the built frontend (frontend/dist) into the binary so the app
// ships as a single file (AGENT.md §1 single-binary, §19 MVP success).
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register the `time` event with a string payload so Wails generates a
	// typed TS API for it. The StatusBar subscribes via Events.On("time").
	wailsapp.RegisterEvent[string]("time")
}

func main() {
	store, err := initStorage()
	if err != nil {
		log.Fatalf("storage init failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Repositories (infrastructure).
	configRepo := sqlite.NewConfigRepo(store)
	hostRepo := sqlite.NewHostRepo(store)
	hostKeyRepo := sqlite.NewHostKeyRepo(store)

	// OS-backed secret store (Windows Credential Manager / macOS Keychain /
	// Linux Secret Service). CGO-free on all platforms.
	secretSvc := application.NewSecretService(secret.Default())

	// Connection manager needs a HostKeyStore; adapt the app-layer repo.
	connManager := ssh.NewManager(ssh.FromHostKeyRepo(hostKeyRepo))

	// Per-host SSH tunnel manager (host settings → auto-start on session
	// open, one tunnel per host, auto-reconnect). Its status emitter is wired
	// to the TunnelService once that exists.
	tunnelMgr := ssh.NewTunnelManager(ssh.FromHostKeyRepo(hostKeyRepo))

	// SFTP manager dials its own (cached) connections per host, reusing the
	// same dial logic + host-key verification as the terminal connection.
	keyStore := ssh.FromHostKeyRepo(hostKeyRepo)
	sftpMgr := sftp.NewManager(func(host domain.Host, creds domain.Credentials) (*ssh.Client, error) {
		return ssh.Dial(ssh.ConnectOptions{
			HostID:   host.ID,
			Host:     host.Host,
			Port:     host.Port,
			Username: host.Username,
		}, ssh.Auth{
			Password:      creds.Password,
			KeyPath:       creds.KeyPath,
			KeyPassphrase: creds.KeyPassphrase,
			UseAgent:      creds.UseAgent,
		}, keyStore)
	})

	// Application services.
	configSvc := application.NewConfigService(configRepo)
	hostSvc := application.NewHostService(hostRepo, connManager, hostKeyRepo, secretSvc)
	monitorSvc := application.NewMonitorService(connManager)
	dockerSvc := application.NewDockerService(connManager)
	sftpSvc := application.NewSftpService(sftp.NewAppClient(sftpMgr), hostRepo, secretSvc, func() domain.TransferConfig {
		cfg, err := configSvc.GetAppConfig()
		if err != nil {
			return domain.TransferConfig{}
		}
		return cfg.Transfer
	})
	// ModelProviderService takes the provider repo + legacy config repo (both
	// implemented by ConfigRepo) and the vault for per-provider API keys.
	providerSvc := application.NewModelProviderService(configRepo, configRepo, secretSvc)
	// Agent conversation persistence (chat history, resumable).
	convRepo := sqlite.NewConversationRepo(store)
	convSvc := application.NewConversationService(convRepo, connManager)

	// Agent: permission gate (emitter wired after AgentService is created),
	// tool runtime (eino ReAct agent per session; turns persisted via convSvc).
	// The config source feeds the user-adjustable agent settings (prompt,
	// max steps, history budget, tool output cap); read per chat so changes
	// apply without a restart.
	permGate := application.NewPermissionGate(nil)
	agentRuntime := agent.NewRuntime(providerSvc, connManager, sftpMgr, permGate, &secretResolver{secretSvc}, convSvc, func() domain.AgentConfig {
		cfg, err := configSvc.GetAppConfig()
		if err != nil {
			return domain.AgentConfig{}
		}
		return cfg.Agent
	})

	// Wails-facing services (interface adapter layer).
	systemService := interfaces.NewSystemService(appName, appVersion)
	configService := interfaces.NewConfigService(configSvc)
	hostService := interfaces.NewHostService(hostSvc, tunnelMgr)
	localPtyMgr := localpty.NewManager()
	terminalService := interfaces.NewTerminalService(hostSvc, connManager, localPtyMgr, tunnelMgr)
	tunnelService := interfaces.NewTunnelService(tunnelMgr, hostSvc)
	monitorService := interfaces.NewMonitorService(monitorSvc)
	dockerService := interfaces.NewDockerService(dockerSvc)
	sftpService := interfaces.NewSftpService(sftpSvc)
	windowService := interfaces.NewWindowService(hostSvc)
	providerService := interfaces.NewModelProviderService(providerSvc)
	agentService := interfaces.NewAgentService(agentRuntime, permGate, convSvc)

	// Wire the approval emitter now that AgentService exists.
	permGate.SetEmitter(agentService)

	app := wailsapp.New(wailsapp.Options{
		Name:        appName,
		Description: "A lightweight, local-first, AI-native desktop remote workspace.",
		Services: []wailsapp.Service{
			wailsapp.NewService(systemService),
			wailsapp.NewService(configService),
			wailsapp.NewService(hostService),
			wailsapp.NewService(terminalService),
			wailsapp.NewService(tunnelService),
			wailsapp.NewService(monitorService),
			wailsapp.NewService(dockerService),
			wailsapp.NewService(sftpService),
			wailsapp.NewService(windowService),
			wailsapp.NewService(providerService),
			wailsapp.NewService(agentService),
		},
		Assets: wailsapp.AssetOptions{
			Handler: wailsapp.AssetFileServerFS(assets),
		},
		Mac: wailsapp.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(wailsapp.WebviewWindowOptions{
		Title:     appName,
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		// Frameless on Windows/Linux: the frontend TitleBar draws the drag
		// region and window controls (unified cross-platform look). macOS
		// keeps the native hidden-inset title bar with system traffic lights.
		Frameless: runtime.GOOS != "darwin",
		Mac: wailsapp.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                wailsapp.MacBackdropTranslucent,
			TitleBar:                wailsapp.MacTitleBarHiddenInset,
		},
		BackgroundColour: wailsapp.NewRGB(10, 10, 14),
		URL:              "/",
	})

	// Heartbeat that proves the Wails event bus is live. The StatusBar clock
	// renders each tick; later phases reuse this channel for session events.
	go func() {
		for {
			app.Event.Emit("time", time.Now().Format(time.RFC1123))
			time.Sleep(time.Second)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// initStorage opens (and migrates) the SQLite database in the per-user data
// directory. The path follows the OS convention (AGENT.md §2.1 local-first).
func initStorage() (*sqlite.Store, error) {
	dbPath := filepath.Join(xdg.DataHome, "ai-remote-workspace", "workspace.db")
	return sqlite.Open(dbPath)
}

// secretResolver adapts application.SecretService to the agent.SecretsForResolver
// interface (translating the string kind to SecretKind).
type secretResolver struct{ inner *application.SecretService }

func (r *secretResolver) GetHostSecret(hostID string, kind string) ([]byte, error) {
	return r.inner.GetHostSecret(hostID, application.SecretKind(kind))
}

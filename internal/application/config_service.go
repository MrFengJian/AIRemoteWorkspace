package application

import "github.com/ai-remote/workspace/internal/domain"

// configService is the application-layer implementation of ConfigService.
// It owns the repository and applies defaults/validation.
type configService struct {
	repo ConfigRepository
}

// NewConfigService wires a ConfigService to its repository.
func NewConfigService(repo ConfigRepository) ConfigService {
	return &configService{repo: repo}
}

func (s *configService) GetAppConfig() (domain.AppConfig, error) {
	cfg, err := s.repo.Get()
	if err != nil {
		return domain.AppConfig{}, err
	}
	// Guard against an empty store returning a zero-value theme/shell.
	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}
	if cfg.DefaultShell == "" {
		cfg.DefaultShell = "/bin/bash"
	}
	if cfg.SecurityMode == "" {
		cfg.SecurityMode = domain.SecurityBalanced
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM = domain.DefaultConfig().LLM
	}
	if cfg.FontSize == 0 {
		cfg.FontSize = 13
	}
	if cfg.MiddleClickAction == "" {
		cfg.MiddleClickAction = "pasteSelection"
	}
	if cfg.MonitorIntervalSeconds == 0 {
		cfg.MonitorIntervalSeconds = 60
	}
	// Agent tunables: zero fields in an older settings row fall back to the
	// defaults (same values as DefaultConfig, kept in sync here because the
	// stored row may predate the field).
	if cfg.Agent.MaxSteps == 0 {
		cfg.Agent.MaxSteps = 100
	}
	if cfg.Agent.HistoryTurns == 0 {
		cfg.Agent.HistoryTurns = 40
	}
	if cfg.Agent.ToolOutputLimitKB == 0 {
		cfg.Agent.ToolOutputLimitKB = 64
	}
	// Transfer tunables: same-value fallbacks for older stored rows.
	if cfg.Transfer.ChunkKB == 0 {
		cfg.Transfer.ChunkKB = 256
	}
	if cfg.Transfer.MaxUploadMB == 0 {
		cfg.Transfer.MaxUploadMB = 4096
	}
	if cfg.Transfer.MaxDownloadMB == 0 {
		cfg.Transfer.MaxDownloadMB = 4096
	}
	return cfg, nil
}

func (s *configService) SetAppConfig(cfg domain.AppConfig) error {
	return s.repo.Set(cfg)
}

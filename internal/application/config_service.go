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
	return cfg, nil
}

func (s *configService) SetAppConfig(cfg domain.AppConfig) error {
	return s.repo.Set(cfg)
}

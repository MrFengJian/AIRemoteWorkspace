package application

import (
	"github.com/ai-remote/workspace/internal/domain"
)

// LLMService manages LLM provider configuration (BaseURL/Model in SQLite) and
// the API key (in the OS vault via SecretService).
type LLMService struct {
	configRepo ConfigRepository
	secrets    *SecretService
}

// NewLLMService wires an LLMService.
func NewLLMService(configRepo ConfigRepository, secrets *SecretService) *LLMService {
	return &LLMService{configRepo: configRepo, secrets: secrets}
}

// GetConfig returns the LLM provider settings (BaseURL + Model). The API key
// is fetched separately via GetAPIKey.
func (s *LLMService) GetConfig() (domain.LLMConfig, error) {
	cfg, err := s.configRepo.Get()
	if err != nil {
		return domain.LLMConfig{}, err
	}
	// Fall back to defaults on an older config without LLM fields.
	if cfg.LLM.BaseURL == "" {
		return domain.DefaultConfig().LLM, nil
	}
	return cfg.LLM, nil
}

// SetConfig persists BaseURL + Model to SQLite. The API key is NOT stored here
// — it goes to the OS vault via SetAPIKey.
func (s *LLMService) SetConfig(llm domain.LLMConfig) error {
	cfg, err := s.configRepo.Get()
	if err != nil {
		return err
	}
	cfg.LLM = llm
	return s.configRepo.Set(cfg)
}

// GetAPIKey returns the stored API key from the OS vault, or "" if none.
func (s *LLMService) GetAPIKey() (string, error) {
	if s.secrets == nil {
		return "", nil
	}
	key, err := s.secrets.GetLLMKey()
	if err != nil {
		if IsErrSecretNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return key, nil
}

// SetAPIKey stores the API key in the OS vault. An empty key clears it.
func (s *LLMService) SetAPIKey(key string) error {
	if s.secrets == nil {
		return nil
	}
	return s.secrets.SaveLLMKey(key)
}

// IsErrSecretNotFound is a convenience wrapper so callers don't import the
// sentinel directly.
func IsErrSecretNotFound(err error) bool {
	return err == ErrSecretNotFound
}

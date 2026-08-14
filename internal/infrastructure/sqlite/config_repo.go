package sqlite

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/domain"
)

// ConfigRepo persists AppConfig as a single JSON row under the 'app' key.
// This is deliberately simple for Phase 1; if settings grow large we can
// normalise into columns later without changing the domain type.
type ConfigRepo struct {
	store *Store
}

// NewConfigRepo binds a ConfigRepo to a Store.
func NewConfigRepo(store *Store) *ConfigRepo {
	return &ConfigRepo{store: store}
}

// Get returns the stored AppConfig, or domain.DefaultConfig() if none exists.
func (r *ConfigRepo) Get() (domain.AppConfig, error) {
	cfg := domain.DefaultConfig()

	var m settingModel
	err := r.store.db.First(&m, "key = ?", "app").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// No row yet → return defaults, not an error.
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if jsonErr := json.Unmarshal([]byte(m.Value), &cfg); jsonErr != nil {
		return domain.DefaultConfig(), jsonErr
	}
	return cfg, nil
}

// Set persists cfg as JSON under the 'app' key (upsert).
func (r *ConfigRepo) Set(cfg domain.AppConfig) error {
	return r.setSetting("app", cfg)
}

// GetProviders returns the stored LLM model providers, or an empty list.
func (r *ConfigRepo) GetProviders() ([]domain.ModelProvider, error) {
	var providers []domain.ModelProvider
	var m settingModel
	err := r.store.db.First(&m, "key = ?", "providers").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return providers, nil
	}
	if err != nil {
		return nil, err
	}
	if jsonErr := json.Unmarshal([]byte(m.Value), &providers); jsonErr != nil {
		return nil, jsonErr
	}
	return providers, nil
}

// SetProviders persists the LLM model providers as JSON under the 'providers'
// key (upsert). Kept separate from 'app' so whole-config saves never clobber
// provider edits made through ModelProviderService.
func (r *ConfigRepo) SetProviders(providers []domain.ModelProvider) error {
	return r.setSetting("providers", providers)
}

// setSetting upserts an arbitrary JSON-serialisable value under a settings key.
func (r *ConfigRepo) setSetting(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var m settingModel
	err = r.store.db.First(&m, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.store.db.Create(&settingModel{Key: key, Value: string(raw)}).Error
	}
	if err != nil {
		return err
	}
	m.Value = string(raw)
	return r.store.db.Save(&m).Error
}

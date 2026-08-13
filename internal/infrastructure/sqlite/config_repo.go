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
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var m settingModel
	err = r.store.db.First(&m, "key = ?", "app").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.store.db.Create(&settingModel{Key: "app", Value: string(raw)}).Error
	}
	if err != nil {
		return err
	}
	m.Value = string(raw)
	return r.store.db.Save(&m).Error
}

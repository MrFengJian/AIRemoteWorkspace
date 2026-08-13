package sqlite

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// gormErrRecordNotFound aliases gorm's sentinel for repo-internal checks.
var gormErrRecordNotFound = gorm.ErrRecordNotFound

// HostKeyRepo implements application.HostKeyRepository over GORM.
type HostKeyRepo struct {
	store *Store
}

// NewHostKeyRepo binds a HostKeyRepo to a Store.
func NewHostKeyRepo(store *Store) *HostKeyRepo {
	return &HostKeyRepo{store: store}
}

// Get returns the recorded key for hostID.
func (r *HostKeyRepo) Get(hostID string) (domain.HostKey, error) {
	var m hostKeyModel
	err := r.store.db.First(&m, "host_id = ?", hostID).Error
	if errors.Is(err, gormErrRecordNotFound) {
		return domain.HostKey{}, application.ErrHostKeyNotFound
	}
	if err != nil {
		return domain.HostKey{}, err
	}
	return domain.HostKey{
		HostID:      m.HostID,
		Algorithm:   m.Algorithm,
		Fingerprint: m.Fingerprint,
	}, nil
}

// Upsert records or replaces the fingerprint and refreshes last_seen.
func (r *HostKeyRepo) Upsert(hostID, algorithm, fingerprint string) error {
	now := time.Now().UTC()
	var m hostKeyModel
	err := r.store.db.First(&m, "host_id = ? AND algorithm = ?", hostID, algorithm).Error
	if errors.Is(err, gormErrRecordNotFound) {
		// Insert a new row.
		return r.store.db.Create(&hostKeyModel{
			HostID:      hostID,
			Algorithm:   algorithm,
			Fingerprint: fingerprint,
			FirstSeen:   now,
			LastSeen:    now,
		}).Error
	}
	if err != nil {
		return err
	}
	// Update existing.
	m.Fingerprint = fingerprint
	m.LastSeen = now
	return r.store.db.Save(&m).Error
}

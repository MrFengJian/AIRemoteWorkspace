package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// HostKeyRepo implements application.HostKeyRepository over SQLite.
type HostKeyRepo struct {
	store *Store
}

// NewHostKeyRepo binds a HostKeyRepo to a Store.
func NewHostKeyRepo(store *Store) *HostKeyRepo {
	return &HostKeyRepo{store: store}
}

// Get returns the recorded key for hostID.
func (r *HostKeyRepo) Get(hostID string) (domain.HostKey, error) {
	var (
		alg string
		fp  string
	)
	err := r.store.db.QueryRow(
		`SELECT algorithm, fingerprint FROM host_keys WHERE host_id = ?1`,
		hostID,
	).Scan(&alg, &fp)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HostKey{}, application.ErrHostKeyNotFound
	}
	if err != nil {
		return domain.HostKey{}, fmt.Errorf("get host key: %w", err)
	}
	return domain.HostKey{HostID: hostID, Algorithm: alg, Fingerprint: fp}, nil
}

// Upsert records or replaces the fingerprint and refreshes last_seen.
func (r *HostKeyRepo) Upsert(hostID, algorithm, fingerprint string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.store.db.Exec(`
		INSERT INTO host_keys (host_id, algorithm, fingerprint, first_seen, last_seen)
		VALUES (?1, ?2, ?3, ?4, ?4)
		ON CONFLICT(host_id, algorithm) DO UPDATE SET
			fingerprint = excluded.fingerprint,
			last_seen   = excluded.last_seen`,
		hostID, algorithm, fingerprint, now)
	if err != nil {
		return fmt.Errorf("upsert host key: %w", err)
	}
	return nil
}

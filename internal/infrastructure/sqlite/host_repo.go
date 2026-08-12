package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// HostRepo implements application.HostRepository over SQLite.
type HostRepo struct {
	store *Store
}

// NewHostRepo binds a HostRepo to a Store.
func NewHostRepo(store *Store) *HostRepo {
	return &HostRepo{store: store}
}

// ErrHostNotFound is returned by Get for a missing id.
var ErrHostNotFound = errors.New("host not found")

// List returns all hosts ordered by name.
func (r *HostRepo) List() ([]domain.Host, error) {
	rows, err := r.store.db.Query(`
		SELECT id, name, host, port, username, auth_type, secret_ref, created_at, updated_at
		FROM hosts ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	defer rows.Close()

	var hosts []domain.Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// Get returns a single host by id.
func (r *HostRepo) Get(id string) (domain.Host, error) {
	row := r.store.db.QueryRow(`
		SELECT id, name, host, port, username, auth_type, secret_ref, created_at, updated_at
		FROM hosts WHERE id = ?1`, id)
	h, err := scanHost(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Host{}, ErrHostNotFound
	}
	return h, err
}

// Save inserts or updates a host (upsert on id).
func (r *HostRepo) Save(h domain.Host) error {
	now := time.Now().UTC()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}
	h.UpdatedAt = now

	_, err := r.store.db.Exec(`
		INSERT INTO hosts (id, name, host, port, username, auth_type, secret_ref, created_at, updated_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			host=excluded.host,
			port=excluded.port,
			username=excluded.username,
			auth_type=excluded.auth_type,
			secret_ref=excluded.secret_ref,
			updated_at=excluded.updated_at`,
		h.ID, h.Name, h.Host, h.Port, h.Username, string(h.AuthType), h.SecretRef, h.CreatedAt.Format(time.RFC3339), h.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save host: %w", err)
	}
	return nil
}

// Delete removes a host by id.
func (r *HostRepo) Delete(id string) error {
	_, err := r.store.db.Exec(`DELETE FROM hosts WHERE id = ?1`, id)
	if err != nil {
		return fmt.Errorf("delete host: %w", err)
	}
	return nil
}

// scanner abstracts *sql.Row and *sql.Rows for shared scan logic.
type scanner interface {
	Scan(dest ...any) error
}

func scanHost(s scanner) (domain.Host, error) {
	var (
		h           domain.Host
		authType    string
		createdRaw  sql.NullString
		updatedRaw  sql.NullString
		secretRef   string
	)
	if err := s.Scan(&h.ID, &h.Name, &h.Host, &h.Port, &h.Username, &authType, &secretRef, &createdRaw, &updatedRaw); err != nil {
		return domain.Host{}, err
	}
	h.AuthType = domain.AuthType(authType)
	h.SecretRef = secretRef
	if createdRaw.Valid {
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdRaw.String)
	}
	if updatedRaw.Valid {
		h.UpdatedAt, _ = time.Parse(time.RFC3339, updatedRaw.String)
	}
	return h, nil
}

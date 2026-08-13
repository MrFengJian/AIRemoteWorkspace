package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// HostRepo implements application.HostRepository over GORM.
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
	var models []hostModel
	if err := r.store.db.Order("name COLLATE NOCASE").Find(&models).Error; err != nil {
		return nil, err
	}
	hosts := make([]domain.Host, 0, len(models))
	for _, m := range models {
		hosts = append(hosts, hostFromModel(m))
	}
	return hosts, nil
}

// Get returns a single host by id.
func (r *HostRepo) Get(id string) (domain.Host, error) {
	var m hostModel
	err := r.store.db.First(&m, "id = ?", id).Error
	if errors.Is(err, gormErrRecordNotFound) {
		return domain.Host{}, ErrHostNotFound
	}
	if err != nil {
		return domain.Host{}, err
	}
	return hostFromModel(m), nil
}

// Save inserts or updates a host (GORM Save = upsert on primary key).
func (r *HostRepo) Save(h domain.Host) error {
	now := time.Now().UTC()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}
	h.UpdatedAt = now

	tagsJSON, err := json.Marshal(h.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	return r.store.db.Save(&hostModel{
		ID:            h.ID,
		Name:          h.Name,
		Host:          h.Host,
		Port:          h.Port,
		Username:      h.Username,
		AuthType:      string(h.AuthType),
		SecretRef:     h.SecretRef,
		TerminalTheme: h.TerminalTheme,
		Group:         h.Group,
		Tags:          string(tagsJSON),
		OS:            h.OS,
		CreatedAt:     h.CreatedAt,
		UpdatedAt:     h.UpdatedAt,
	}).Error
}

// Delete removes a host by id.
func (r *HostRepo) Delete(id string) error {
	return r.store.db.Delete(&hostModel{}, "id = ?", id).Error
}

// hostFromModel converts a GORM row to the domain type.
func hostFromModel(m hostModel) domain.Host {
	var tags []string
	_ = json.Unmarshal([]byte(m.Tags), &tags) // tolerate empty/legacy values
	return domain.Host{
		ID:            m.ID,
		Name:          m.Name,
		Host:          m.Host,
		Port:          m.Port,
		Username:      m.Username,
		AuthType:      domain.AuthType(m.AuthType),
		SecretRef:     m.SecretRef,
		TerminalTheme: m.TerminalTheme,
		Group:         m.Group,
		Tags:          tags,
		OS:            m.OS,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

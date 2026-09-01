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
	tunnelsJSON, err := json.Marshal(h.Tunnels)
	if err != nil {
		return fmt.Errorf("marshal tunnels: %w", err)
	}

	return r.store.db.Save(&hostModel{
		ID:               h.ID,
		Name:             h.Name,
		Host:             h.Host,
		Port:             h.Port,
		Username:         h.Username,
		AuthType:         string(h.AuthType),
		SecretRef:        h.SecretRef,
		KeyPath:          h.KeyPath,
		TerminalTheme:    h.TerminalTheme,
		TerminalFont:     h.TerminalFont,
		TerminalFontSize: h.TerminalFontSize,
		Group:            h.Group,
		Tags:             string(tagsJSON),
		OS:               h.OS,
		AgentProviderID:  h.AgentProviderID,
		AgentModel:       h.AgentModel,
		Tunnel:           string(tunnelsJSON),
		CreatedAt:        h.CreatedAt,
		UpdatedAt:        h.UpdatedAt,
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
	tunnels := tunnelsFromJSON(m.Tunnel)
	return domain.Host{
		ID:               m.ID,
		Name:             m.Name,
		Host:             m.Host,
		Port:             m.Port,
		Username:         m.Username,
		AuthType:         domain.AuthType(m.AuthType),
		SecretRef:        m.SecretRef,
		KeyPath:          m.KeyPath,
		TerminalTheme:    m.TerminalTheme,
		TerminalFont:     m.TerminalFont,
		TerminalFontSize: m.TerminalFontSize,
		Group:            m.Group,
		Tags:             tags,
		OS:               m.OS,
		AgentProviderID:  m.AgentProviderID,
		AgentModel:       m.AgentModel,
		Tunnels:          tunnels,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

// tunnelsFromJSON decodes the tunnel column: an array of rules today, with a
// fallback for the interim single-object format ('' / legacy → no tunnels).
func tunnelsFromJSON(s string) []domain.TunnelConfig {
	var out []domain.TunnelConfig
	if s == "" {
		return out
	}
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	// Interim single-rule format from before multi-tunnel support.
	var one domain.TunnelConfig
	if err := json.Unmarshal([]byte(s), &one); err == nil && one.Type != "" {
		return []domain.TunnelConfig{one}
	}
	return nil
}

package sqlite

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/domain"
)

// ErrExpertNotFound is returned by Get for a missing id.
var ErrExpertNotFound = errors.New("expert not found")

// ExpertRepo implements application.ExpertRepository over GORM.
type ExpertRepo struct {
	store *Store
}

// NewExpertRepo binds an ExpertRepo to a Store.
func NewExpertRepo(store *Store) *ExpertRepo {
	return &ExpertRepo{store: store}
}

// List returns every expert row (including dismissed builtins — the service
// layer decides visibility), ordered by SortOrder then name.
func (r *ExpertRepo) List() ([]domain.Expert, error) {
	var models []expertModel
	if err := r.store.db.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Expert, 0, len(models))
	for _, m := range models {
		out = append(out, expertFromModel(m))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Get returns a single expert by id.
func (r *ExpertRepo) Get(id string) (domain.Expert, error) {
	var m expertModel
	err := r.store.db.First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Expert{}, ErrExpertNotFound
	}
	if err != nil {
		return domain.Expert{}, err
	}
	return expertFromModel(m), nil
}

// Save inserts or updates an expert (upsert on primary key).
func (r *ExpertRepo) Save(e domain.Expert) error {
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	toolsJSON, err := json.Marshal(e.AllowedTools)
	if err != nil {
		return err
	}
	skillsJSON, err := json.Marshal(e.SkillRefs)
	if err != nil {
		return err
	}
	suggJSON, err := json.Marshal(e.SuggestedPrompts)
	if err != nil {
		return err
	}

	return r.store.db.Save(&expertModel{
		ID:             e.ID,
		Name:           e.Name,
		Role:           e.Role,
		Icon:           e.Icon,
		Color:          e.Color,
		SortOrder:      e.SortOrder,
		Description:    e.Description,
		SystemPrompt:   e.SystemPrompt,
		Heartbeat:      e.Heartbeat,
		AllowedTools:   string(toolsJSON),
		SkillRefs:      string(skillsJSON),
		Suggested:      string(suggJSON),
		ProviderID:     e.ProviderID,
		Model:          e.Model,
		Policy:         e.Policy,
		Temperature:    e.Temperature,
		MaxSteps:       e.MaxSteps,
		OpeningMessage: e.OpeningMessage,
		AutoSnapshot:   e.AutoSnapshot,
		Builtin:        e.Builtin,
		Enabled:        e.Enabled,
		Dismissed:      e.Dismissed,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}).Error
}

// Delete removes an expert row by id.
func (r *ExpertRepo) Delete(id string) error {
	return r.store.db.Delete(&expertModel{}, "id = ?", id).Error
}

func expertFromModel(m expertModel) domain.Expert {
	var tools, skills, suggested []string
	_ = json.Unmarshal([]byte(m.AllowedTools), &tools) // tolerate ''/legacy
	_ = json.Unmarshal([]byte(m.SkillRefs), &skills)
	_ = json.Unmarshal([]byte(m.Suggested), &suggested)
	return domain.Expert{
		ID:               m.ID,
		Name:             m.Name,
		Role:             m.Role,
		Icon:             m.Icon,
		Color:            m.Color,
		SortOrder:        m.SortOrder,
		Description:      m.Description,
		SystemPrompt:     m.SystemPrompt,
		Heartbeat:        m.Heartbeat,
		AllowedTools:     tools,
		SkillRefs:        skills,
		SuggestedPrompts: suggested,
		ProviderID:       m.ProviderID,
		Model:            m.Model,
		Policy:           m.Policy,
		Temperature:      m.Temperature,
		MaxSteps:         m.MaxSteps,
		OpeningMessage:   m.OpeningMessage,
		AutoSnapshot:     m.AutoSnapshot,
		Builtin:          m.Builtin,
		Enabled:          m.Enabled,
		Dismissed:        m.Dismissed,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

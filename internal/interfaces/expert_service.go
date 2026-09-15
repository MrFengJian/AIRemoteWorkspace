package interfaces

import (
	"context"
	"fmt"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// ExpertDTO mirrors domain.Expert for the frontend (digital-employee roster
// management and the chat-side persona pickers).
type ExpertDTO struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Role             string   `json:"role"`
	Icon             string   `json:"icon"`
	Color            string   `json:"color"`
	Description      string   `json:"description"`
	SystemPrompt     string   `json:"systemPrompt"`
	AllowedTools     []string `json:"allowedTools"`
	SkillRefs        []string `json:"skillRefs"`
	ProviderID       string   `json:"providerId"`
	Model            string   `json:"model"`
	Policy           string   `json:"policy"`
	Temperature      float64  `json:"temperature"`
	MaxSteps         int      `json:"maxSteps"`
	OpeningMessage   string   `json:"openingMessage"`
	SuggestedPrompts []string `json:"suggestedPrompts"`
	AutoSnapshot     bool     `json:"autoSnapshot"`
	Builtin          bool     `json:"builtin"`
	Enabled          bool     `json:"enabled"`
	SortOrder        int      `json:"sortOrder"`
}

func expertToDTO(e domain.Expert) ExpertDTO {
	return ExpertDTO{
		ID:               e.ID,
		Name:             e.Name,
		Role:             e.Role,
		Icon:             e.Icon,
		Color:            e.Color,
		Description:      e.Description,
		SystemPrompt:     e.SystemPrompt,
		AllowedTools:     orEmpty(e.AllowedTools),
		SkillRefs:        orEmpty(e.SkillRefs),
		ProviderID:       e.ProviderID,
		Model:            e.Model,
		Policy:           e.Policy,
		Temperature:      e.Temperature,
		MaxSteps:         e.MaxSteps,
		OpeningMessage:   e.OpeningMessage,
		SuggestedPrompts: orEmpty(e.SuggestedPrompts),
		AutoSnapshot:     e.AutoSnapshot,
		Builtin:          e.Builtin,
		Enabled:          e.Enabled,
		SortOrder:        e.SortOrder,
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ExpertService exposes the digital-employee roster (运维专家/数字员工) to
// the frontend: the chat-side pickers read it, the settings page manages it.
type ExpertService struct {
	experts *appsvc.ExpertService
}

// NewExpertService wires the Wails ExpertService.
func NewExpertService(experts *appsvc.ExpertService) *ExpertService {
	return &ExpertService{experts: experts}
}

func (s *ExpertService) ServiceName() string { return "ExpertService" }

func (s *ExpertService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	return nil
}

// ListExperts returns the visible roster (dismissed builtins hidden).
func (s *ExpertService) ListExperts() ([]ExpertDTO, error) {
	if s.experts == nil {
		return []ExpertDTO{}, nil
	}
	list, err := s.experts.ListExperts()
	if err != nil {
		return nil, err
	}
	out := make([]ExpertDTO, 0, len(list))
	for _, e := range list {
		out = append(out, expertToDTO(e))
	}
	return out, nil
}

// GetExpert returns one expert including its full persona prompt (editor use).
func (s *ExpertService) GetExpert(id string) (ExpertDTO, error) {
	if s.experts == nil {
		return ExpertDTO{}, fmt.Errorf("experts not available")
	}
	e, err := s.experts.GetExpert(id)
	if err != nil {
		return ExpertDTO{}, err
	}
	return expertToDTO(e), nil
}

// SaveExpert creates or updates an expert and returns the stored row
// (ids/flags are assigned server-side).
func (s *ExpertService) SaveExpert(e ExpertDTO) (ExpertDTO, error) {
	if s.experts == nil {
		return ExpertDTO{}, fmt.Errorf("experts not available")
	}
	saved, err := s.experts.SaveExpert(domain.Expert{
		ID:               e.ID,
		Name:             e.Name,
		Role:             e.Role,
		Icon:             e.Icon,
		Color:            e.Color,
		Description:      e.Description,
		SystemPrompt:     e.SystemPrompt,
		AllowedTools:     e.AllowedTools,
		SkillRefs:        e.SkillRefs,
		ProviderID:       e.ProviderID,
		Model:            e.Model,
		Policy:           e.Policy,
		Temperature:      e.Temperature,
		MaxSteps:         e.MaxSteps,
		OpeningMessage:   e.OpeningMessage,
		SuggestedPrompts: e.SuggestedPrompts,
		AutoSnapshot:     e.AutoSnapshot,
		Enabled:          e.Enabled,
		SortOrder:        e.SortOrder,
	})
	if err != nil {
		return ExpertDTO{}, err
	}
	return expertToDTO(saved), nil
}

// DeleteExpert removes an expert; builtins are only dismissed and can be
// restored by saving an expert with the same builtin id.
func (s *ExpertService) DeleteExpert(id string) error {
	if s.experts == nil {
		return fmt.Errorf("experts not available")
	}
	return s.experts.DeleteExpert(id)
}

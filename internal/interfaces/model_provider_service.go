package interfaces

import (
	"context"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// ModelProviderDTO carries a provider to the frontend — never the API key.
type ModelProviderDTO struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	BaseURL   string   `json:"baseUrl"`
	Models    []string `json:"models"`
	Enabled   bool     `json:"enabled"`
	HasAPIKey bool     `json:"hasApiKey"`
}

// SaveProviderInput is what the frontend sends to create/update a provider.
type SaveProviderInput struct {
	ID      string   `json:"id"` // empty = create
	Name    string   `json:"name"`
	BaseURL string   `json:"baseUrl"`
	Models  []string `json:"models"`
	Enabled bool     `json:"enabled"`
	APIKey  string   `json:"apiKey"` // empty = keep existing; " " = clear
}

// TestProviderInput addresses a provider for testing / model fetching. Empty
// BaseURL or APIKey with an ID present falls back to the stored values.
type TestProviderInput struct {
	ID      string `json:"id"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"` // optional; defaults to the first recorded model
}

// ModelProviderService exposes LLM provider management to the frontend.
type ModelProviderService struct {
	svc *appsvc.ModelProviderService
}

// NewModelProviderService wires the ModelProviderService.
func NewModelProviderService(svc *appsvc.ModelProviderService) *ModelProviderService {
	return &ModelProviderService{svc: svc}
}

func (m *ModelProviderService) ServiceName() string { return "ModelProviderService" }

// ServiceStartup runs when the service is registered with the app.
func (m *ModelProviderService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	return nil
}

// ListProviders returns all providers with their enabled state and whether an
// API key is stored.
func (m *ModelProviderService) ListProviders() ([]ModelProviderDTO, error) {
	list, err := m.svc.List()
	if err != nil {
		return nil, err
	}
	out := make([]ModelProviderDTO, 0, len(list))
	for _, p := range list {
		key, _ := m.svc.APIKey(p.ID)
		models := p.Models
		if models == nil {
			models = []string{}
		}
		out = append(out, ModelProviderDTO{
			ID:        p.ID,
			Name:      p.Name,
			BaseURL:   p.BaseURL,
			Models:    models,
			Enabled:   p.Enabled,
			HasAPIKey: key != "",
		})
	}
	return out, nil
}

// SaveProvider creates or updates a provider.
func (m *ModelProviderService) SaveProvider(in SaveProviderInput) error {
	return m.svc.Save(domain.ModelProvider{
		ID:      in.ID,
		Name:    in.Name,
		BaseURL: in.BaseURL,
		Models:  in.Models,
		Enabled: in.Enabled,
	}, in.APIKey)
}

// DeleteProvider removes a provider and its stored API key.
func (m *ModelProviderService) DeleteProvider(id string) error {
	return m.svc.Delete(id)
}

// TestProvider verifies the endpoint with a minimal chat completion.
func (m *ModelProviderService) TestProvider(in TestProviderInput) (TestConnectionResult, error) {
	ok, msg := m.svc.TestConnection(context.Background(), in.ID, in.BaseURL, in.APIKey, in.Model)
	return TestConnectionResult{OK: ok, Msg: msg}, nil
}

// FetchModels lists the models a provider exposes via its /models endpoint.
func (m *ModelProviderService) FetchModels(in TestProviderInput) ([]string, error) {
	return m.svc.FetchModels(context.Background(), in.ID, in.BaseURL, in.APIKey)
}

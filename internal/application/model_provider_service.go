package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// ModelProviderService manages the LLM provider list (SQLite via
// ProviderRepository) and their API keys (OS vault via SecretService). It also
// implements the agent runtime's LLMResolver port.
type ModelProviderService struct {
	providers ProviderRepository
	config    ConfigRepository // legacy single-provider config, for migration
	secrets   *SecretService
}

// NewModelProviderService wires a ModelProviderService.
func NewModelProviderService(providers ProviderRepository, config ConfigRepository, secrets *SecretService) *ModelProviderService {
	return &ModelProviderService{providers: providers, config: config, secrets: secrets}
}

// List returns all providers, migrating the legacy single-provider config on
// first use (old vault ref airemote:llm:apikey → per-provider ref).
func (s *ModelProviderService) List() ([]domain.ModelProvider, error) {
	list, err := s.providers.GetProviders()
	if err != nil {
		return nil, err
	}
	if len(list) > 0 {
		return list, nil
	}
	return s.migrateLegacy()
}

// Get returns a single provider by id.
func (s *ModelProviderService) Get(id string) (domain.ModelProvider, error) {
	list, err := s.List()
	if err != nil {
		return domain.ModelProvider{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.ModelProvider{}, fmt.Errorf("provider %q not found", id)
}

// Save creates or updates a provider. apiKey semantics follow the existing
// convention: "" keeps the stored key, " " clears it.
func (s *ModelProviderService) Save(p domain.ModelProvider, apiKey string) error {
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	p.Models = normalizeModels(p.Models)
	if p.Name == "" || p.BaseURL == "" {
		return fmt.Errorf("provider name and base URL are required")
	}

	list, err := s.providers.GetProviders()
	if err != nil {
		return err
	}

	if p.ID == "" {
		p.ID = newID()
		list = append(list, p)
	} else {
		found := false
		for i := range list {
			if list[i].ID == p.ID {
				list[i] = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("provider %q not found", p.ID)
		}
	}

	if err := s.providers.SetProviders(list); err != nil {
		return err
	}
	if apiKey != "" {
		return s.secrets.SaveProviderKey(p.ID, strings.TrimSpace(apiKey))
	}
	return nil
}

// Delete removes a provider and its vault entry.
func (s *ModelProviderService) Delete(id string) error {
	list, err := s.providers.GetProviders()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			list = append(list[:i], list[i+1:]...)
			if err := s.providers.SetProviders(list); err != nil {
				return err
			}
			return s.secrets.DeleteProviderKey(id)
		}
	}
	return fmt.Errorf("provider %q not found", id)
}

// APIKey returns a provider's stored API key, or "" if none.
func (s *ModelProviderService) APIKey(providerID string) (string, error) {
	return s.secrets.GetProviderKey(providerID)
}

// ResolveLLM implements the agent runtime's LLMResolver port: it turns a
// (providerID, model) selection into endpoint credentials.
func (s *ModelProviderService) ResolveLLM(providerID, model string) (domain.LLMEndpoint, error) {
	if providerID == "" {
		return domain.LLMEndpoint{}, fmt.Errorf("no model provider selected — pick one in the agent panel")
	}
	p, err := s.Get(providerID)
	if err != nil {
		return domain.LLMEndpoint{}, err
	}
	if !p.Enabled {
		return domain.LLMEndpoint{}, fmt.Errorf("provider %q is disabled — enable it in Settings", p.Name)
	}
	if model == "" {
		return domain.LLMEndpoint{}, fmt.Errorf("no model selected — pick one in the agent panel")
	}
	apiKey, err := s.APIKey(providerID)
	if err != nil {
		return domain.LLMEndpoint{}, fmt.Errorf("load api key: %w", err)
	}
	return domain.LLMEndpoint{BaseURL: p.BaseURL, Model: model, APIKey: apiKey}, nil
}

// TestConnection sends a minimal chat completion against the provider and
// reports success/failure with a human-readable message. An empty apiKey with
// a non-empty providerID falls back to the stored key.
func (s *ModelProviderService) TestConnection(ctx context.Context, providerID, baseURL, apiKey, model string) (bool, string) {
	if apiKey == "" && providerID != "" {
		if k, err := s.APIKey(providerID); err == nil {
			apiKey = k
		}
	}
	if model == "" && providerID != "" {
		if p, err := s.Get(providerID); err == nil && len(p.Models) > 0 {
			model = p.Models[0]
		}
	}
	if strings.TrimSpace(baseURL) == "" || model == "" {
		return false, "base URL and model are required to test"
	}

	payload := map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	}
	status, body := s.postChat(ctx, baseURL, apiKey, payload)
	if status/100 == 2 {
		return true, fmt.Sprintf("HTTP %d — connection OK", status)
	}
	// OpenAI's newer reasoning models reject max_tokens; retry once with the
	// renamed field before reporting failure.
	if status == http.StatusBadRequest && strings.Contains(body, "max_completion_tokens") {
		payload["max_completion_tokens"] = payload["max_tokens"]
		delete(payload, "max_tokens")
		status, body = s.postChat(ctx, baseURL, apiKey, payload)
		if status/100 == 2 {
			return true, fmt.Sprintf("HTTP %d — connection OK", status)
		}
	}
	msg := strings.TrimSpace(body)
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return false, fmt.Sprintf("HTTP %d: %s", status, msg)
}

// FetchModels lists models from the provider's /models endpoint so the user
// can record them without typing. Empty apiKey + providerID uses stored key.
func (s *ModelProviderService) FetchModels(ctx context.Context, providerID, baseURL, apiKey string) ([]string, error) {
	if apiKey == "" && providerID != "" {
		if k, err := s.APIKey(providerID); err == nil {
			apiKey = k
		}
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	// Standard shape: {"data":[{"id":"gpt-4o"},…]}; some servers return a
	// bare array. Both carry only the fields we need.
	var wrapped struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Data) > 0 {
		models := make([]string, 0, len(wrapped.Data))
		for _, m := range wrapped.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		sort.Strings(models)
		return models, nil
	}
	var bare []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &bare); err == nil {
		models := make([]string, 0, len(bare))
		for _, m := range bare {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		sort.Strings(models)
		return models, nil
	}
	return nil, fmt.Errorf("unrecognized /models response")
}

// postChat issues a chat-completions request and returns status + body.
func (s *ModelProviderService) postChat(ctx context.Context, baseURL, apiKey string, payload map[string]any) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err.Error()
	}
	url := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(body)
}

// migrateLegacy converts the pre-multi-provider config (single BaseURL/Model +
// vault key under airemote:llm:apikey) into one provider entry. Runs at most
// once: after it stores the provider, List() takes the normal path.
func (s *ModelProviderService) migrateLegacy() ([]domain.ModelProvider, error) {
	key, err := s.secrets.GetLLMKey()
	if err != nil && !IsErrSecretNotFound(err) {
		return nil, err
	}
	if key == "" {
		// Nothing to migrate — leave the provider list empty.
		return []domain.ModelProvider{}, nil
	}

	cfg, err := s.config.Get()
	if err != nil {
		return nil, err
	}
	legacy := cfg.LLM
	if legacy.BaseURL == "" {
		legacy = domain.DefaultConfig().LLM
	}

	p := domain.ModelProvider{
		ID:      newID(),
		Name:    legacyName(legacy.BaseURL),
		BaseURL: strings.TrimRight(legacy.BaseURL, "/"),
		Models:  normalizeModels([]string{legacy.Model}),
		Enabled: true,
	}
	if err := s.providers.SetProviders([]domain.ModelProvider{p}); err != nil {
		return nil, err
	}
	if err := s.secrets.SaveProviderKey(p.ID, key); err != nil {
		return nil, err
	}
	// Drop the legacy ref so deleting all providers later doesn't re-trigger
	// this migration on every List().
	if err := s.secrets.DeleteLLMKey(); err != nil {
		return nil, err
	}
	return []domain.ModelProvider{p}, nil
}

// legacyName derives a readable provider name from the legacy base URL host.
func legacyName(baseURL string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	if i := strings.IndexByte(host, '/'); i > 0 {
		host = host[:i]
	}
	switch {
	case host == "":
		return "Provider"
	case strings.Contains(host, "openai"):
		return "OpenAI"
	case strings.Contains(host, "deepseek"):
		return "DeepSeek"
	default:
		return host
	}
}

// normalizeModels trims, dedupes (order-preserving) and drops empties.
func normalizeModels(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

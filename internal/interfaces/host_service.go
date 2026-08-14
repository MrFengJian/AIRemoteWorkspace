package interfaces

import (
	"context"
	"time"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
	"github.com/ai-remote/workspace/internal/domain"
)

// HostDTO is the frontend-facing host representation. It mirrors domain.Host
// but exposes authType as a plain string and omits internal timestamps that
// the UI doesn't need.
type HostDTO struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Host               string   `json:"host"`
	Port               int      `json:"port"`
	Username           string   `json:"username"`
	AuthType           string   `json:"authType"`
	KeyPath            string   `json:"keyPath,omitempty"`
	HasRememberedSecret bool    `json:"hasRememberedSecret"`
	TerminalTheme      string   `json:"terminalTheme"` // per-host terminal colour scheme id
	TerminalFont      string   `json:"terminalFont"`   // per-host override; "" = follow settings
	TerminalFontSize  int      `json:"terminalFontSize"` // per-host override; 0 = follow settings
	Group              string   `json:"group"`
	Tags               []string `json:"tags"`
	OS                 string   `json:"os"` // detected distro id; read-only, never editable
	// Last-used agent model preference; read-only for the host form, written
	// via SetAgentModel by the agent panel.
	AgentProviderID    string   `json:"agentProviderId"`
	AgentModel         string   `json:"agentModel"`
}

// HostInputDTO is what the frontend sends to create/update a host.
type HostInputDTO struct {
	Name             string   `json:"name"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Username         string   `json:"username"`
	AuthType         string   `json:"authType"`
	KeyPath          string   `json:"keyPath,omitempty"`
	TerminalTheme    string   `json:"terminalTheme"`
	TerminalFont     string   `json:"terminalFont"`
	TerminalFontSize int      `json:"terminalFontSize"`
	Group            string   `json:"group"`
	Tags             []string `json:"tags"`
}

// CredentialsDTO carries connect-time secret material supplied by the UI.
// Never persisted.
type CredentialsDTO struct {
	Password      string `json:"password,omitempty"`
	KeyPath       string `json:"keyPath,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
	UseAgent      bool   `json:"useAgent,omitempty"`
}

// HostService exposes host CRUD + connection testing to the frontend.
type HostService struct {
	svc *appsvc.HostService
}

// NewHostService wires the Wails HostService to its application port.
func NewHostService(svc *appsvc.HostService) *HostService {
	return &HostService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (h *HostService) ServiceName() string { return "HostService" }

// ServiceStartup runs when the service is registered with the app.
func (h *HostService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error { return nil }

// ListHosts returns all stored hosts.
func (h *HostService) ListHosts() ([]HostDTO, error) {
	hosts, err := h.svc.List()
	if err != nil {
		return nil, err
	}
	out := make([]HostDTO, 0, len(hosts))
	for _, host := range hosts {
		dto := toHostDTO(host)
		dto.HasRememberedSecret = h.hasRememberedSecret(host.ID)
		out = append(out, dto)
	}
	return out, nil
}

// GetHost returns a single host by id.
func (h *HostService) GetHost(id string) (HostDTO, error) {
	host, err := h.svc.Get(id)
	if err != nil {
		return HostDTO{}, err
	}
	dto := toHostDTO(host)
	dto.HasRememberedSecret = h.hasRememberedSecret(id)
	return dto, nil
}

// CreateHost validates and stores a new host.
func (h *HostService) CreateHost(in HostInputDTO) (HostDTO, error) {
	host, err := h.svc.Create(toHostInput(in))
	if err != nil {
		return HostDTO{}, err
	}
	return toHostDTO(host), nil
}

// UpdateHost modifies an existing host.
func (h *HostService) UpdateHost(id string, in HostInputDTO) (HostDTO, error) {
	host, err := h.svc.Update(id, toHostInput(in))
	if err != nil {
		return HostDTO{}, err
	}
	return toHostDTO(host), nil
}

// DeleteHost removes a host by id.
func (h *HostService) DeleteHost(id string) error {
	return h.svc.Delete(id)
}

// SetAgentModel persists the host's last-used agent provider + model (hidden
// preference; written by the agent panel, not the host edit form).
func (h *HostService) SetAgentModel(hostID, providerID, model string) error {
	return h.svc.SetAgentModel(hostID, providerID, model)
}

// TestConnectionResult reports a connection attempt outcome to the UI.
type TestConnectionResult struct {
	OK bool   `json:"ok"`
	Msg string `json:"msg"`
}

// TestConnection dials the host with the given credentials and reports
// success/failure without keeping the session open. If the caller sends empty
// credentials, remembered secrets from the OS vault are filled in first
// (so "remember password" works for testing too, not just connecting).
func (h *HostService) TestConnection(hostID string, creds CredentialsDTO) (TestConnectionResult, error) {
	host, err := h.svc.Get(hostID)
	if err != nil {
		return TestConnectionResult{}, err
	}
	resolved, err := h.svc.ResolveCredentials(host, toDomainCreds(creds))
	if err != nil {
		return TestConnectionResult{OK: false, Msg: err.Error()}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err = h.svc.TestConnection(ctx, host, resolved)
	if err != nil {
		// Return a structured result rather than a raw error so the UI can
		// show the reason inline.
		return TestConnectionResult{OK: false, Msg: err.Error()}, nil
	}
	return TestConnectionResult{OK: true, Msg: "connected"}, nil
}

// --- mappers ---

func toHostInput(in HostInputDTO) appsvc.CreateHostInput {
	return appsvc.CreateHostInput{
		Name:             in.Name,
		Host:             in.Host,
		Port:             in.Port,
		Username:         in.Username,
		AuthType:         domain.AuthType(in.AuthType),
		KeyPath:          in.KeyPath,
		TerminalTheme:    in.TerminalTheme,
		TerminalFont:     in.TerminalFont,
		TerminalFontSize: in.TerminalFontSize,
		Group:            in.Group,
		Tags:             in.Tags,
	}
}

func toHostDTO(h domain.Host) HostDTO {
	return HostDTO{
		ID:                 h.ID,
		Name:               h.Name,
		Host:               h.Host,
		Port:               h.Port,
		Username:           h.Username,
		AuthType:           string(h.AuthType),
		TerminalTheme:      h.TerminalTheme,
		TerminalFont:       h.TerminalFont,
		TerminalFontSize:   h.TerminalFontSize,
		Group:              h.Group,
		Tags:               h.Tags,
		OS:                 h.OS,
		AgentProviderID:    h.AgentProviderID,
		AgentModel:         h.AgentModel,
	}
}

func toDomainCreds(c CredentialsDTO) domain.Credentials {
	return domain.Credentials{
		Password:      c.Password,
		KeyPath:       c.KeyPath,
		KeyPassphrase: c.KeyPassphrase,
		UseAgent:      c.UseAgent,
	}
}

// hasRememberedSecret reports whether any secret is stored in the OS vault for
// this host. Returns false when no keychain backend is available.
func (h *HostService) hasRememberedSecret(hostID string) bool {
	secrets := h.svc.Secrets()
	if secrets == nil {
		return false
	}
	return secrets.HasHostSecret(hostID)
}

// RememberedCredentialsDTO tells the UI whether remembered secrets exist.
// The secret values are deliberately NOT returned — the vault is read at
// connect time on the backend, never shipped to the frontend.
type RememberedCredentialsDTO struct {
	HasPassword   bool `json:"hasPassword"`
	HasPassphrase bool `json:"hasPassphrase"`
}

// GetRememberedCredentials reports which secrets are stored for a host. Used by
// the edit dialog to show a "remembered" indicator without revealing values.
func (h *HostService) GetRememberedCredentials(hostID string) (RememberedCredentialsDTO, error) {
	out := RememberedCredentialsDTO{}
	secrets := h.svc.Secrets()
	if secrets == nil {
		return out, nil
	}
	if _, err := secrets.GetHostSecret(hostID, appsvc.SecretPassword); err == nil {
		out.HasPassword = true
	}
	if _, err := secrets.GetHostSecret(hostID, appsvc.SecretPassphrase); err == nil {
		out.HasPassphrase = true
	}
	return out, nil
}

// SaveCredentials stores or clears remembered secrets for a host.
// When remember is false, any existing secrets for this host are removed.
// Password is stored for password auth; passphrase for key auth.
func (h *HostService) SaveCredentials(hostID string, creds CredentialsDTO, remember bool) error {
	secrets := h.svc.Secrets()
	if secrets == nil {
		return nil // no keychain; silently no-op (memory-only)
	}

	if !remember {
		return secrets.DeleteHostSecrets(hostID)
	}

	host, err := h.svc.Get(hostID)
	if err != nil {
		return err
	}
	switch host.AuthType {
	case domain.AuthPassword:
		if creds.Password != "" {
			if err := secrets.SaveHostSecret(hostID, appsvc.SecretPassword, []byte(creds.Password)); err != nil {
				return err
			}
		}
	case domain.AuthKey:
		if creds.KeyPassphrase != "" {
			if err := secrets.SaveHostSecret(hostID, appsvc.SecretPassphrase, []byte(creds.KeyPassphrase)); err != nil {
				return err
			}
		}
	}
	return nil
}

// hostNotFoundMsg is reserved for future stable error messaging to the UI.

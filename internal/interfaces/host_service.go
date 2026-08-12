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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	AuthType string `json:"authType"`
	KeyPath  string `json:"keyPath,omitempty"`
}

// HostInputDTO is what the frontend sends to create/update a host.
type HostInputDTO struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	AuthType string `json:"authType"`
	KeyPath  string `json:"keyPath,omitempty"`
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
func (h *HostService) ServiceStartup(_ *wailsapp.App) error { return nil }

// ListHosts returns all stored hosts.
func (h *HostService) ListHosts() ([]HostDTO, error) {
	hosts, err := h.svc.List()
	if err != nil {
		return nil, err
	}
	out := make([]HostDTO, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, toHostDTO(host))
	}
	return out, nil
}

// GetHost returns a single host by id.
func (h *HostService) GetHost(id string) (HostDTO, error) {
	host, err := h.svc.Get(id)
	if err != nil {
		return HostDTO{}, err
	}
	return toHostDTO(host), nil
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

// TestConnectionResult reports a connection attempt outcome to the UI.
type TestConnectionResult struct {
	OK bool   `json:"ok"`
	Msg string `json:"msg"`
}

// TestConnection dials the host with the given credentials and reports
// success/failure without keeping the session open.
func (h *HostService) TestConnection(hostID string, creds CredentialsDTO) (TestConnectionResult, error) {
	host, err := h.svc.Get(hostID)
	if err != nil {
		return TestConnectionResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err = h.svc.TestConnection(ctx, host, toDomainCreds(creds))
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
		Name:     in.Name,
		Host:     in.Host,
		Port:     in.Port,
		Username: in.Username,
		AuthType: domain.AuthType(in.AuthType),
		KeyPath:  in.KeyPath,
	}
}

func toHostDTO(h domain.Host) HostDTO {
	return HostDTO{
		ID:       h.ID,
		Name:     h.Name,
		Host:     h.Host,
		Port:     h.Port,
		Username: h.Username,
		AuthType: string(h.AuthType),
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

// hostNotFoundMsg is reserved for future stable error messaging to the UI.
// Removed: fmt/errors not otherwise needed in this file.

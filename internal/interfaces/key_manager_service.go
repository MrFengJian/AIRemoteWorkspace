package interfaces

import (
	"context"
	"fmt"
	"strings"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appsvc "github.com/ai-remote/workspace/internal/application"
)

// ManagedKeyDTO mirrors application.ManagedKey for the frontend.
type ManagedKeyDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
	Encrypted   bool   `json:"encrypted"`
	CreatedAt   string `json:"createdAt"`
}

// GenerateKeyRequestDTO is the generate/import payload from the frontend.
type GenerateKeyRequestDTO struct {
	Name       string `json:"name"`
	Algorithm  string `json:"algorithm"` // "ed25519" | "rsa" | "ecdsa"
	Bits       int    `json:"bits"`
	Passphrase string `json:"passphrase"`
	Comment    string `json:"comment"`
}

// ImportKeyRequestDTO is the import payload from the frontend.
type ImportKeyRequestDTO struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Passphrase string `json:"passphrase"`
}

// KeyManagerService exposes the 密钥管理器 to the frontend: managed SSH keys
// stored under <数据目录>/keys (generate / import / export / delete).
type KeyManagerService struct {
	svc *appsvc.KeyManagerService
	app *wailsapp.App
}

// NewKeyManagerService wires the Wails KeyManagerService.
func NewKeyManagerService(svc *appsvc.KeyManagerService) *KeyManagerService {
	return &KeyManagerService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (s *KeyManagerService) ServiceName() string { return "KeyManagerService" }

// ServiceStartup captures the Application handle.
func (s *KeyManagerService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	s.app = wailsapp.Get()
	return nil
}

func toKeyDTO(k appsvc.ManagedKey) ManagedKeyDTO {
	return ManagedKeyDTO{
		ID:          k.ID,
		Name:        k.Name,
		Algorithm:   k.Algorithm,
		Fingerprint: k.Fingerprint,
		Comment:     k.Comment,
		Encrypted:   k.Encrypted,
		CreatedAt:   k.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// ListKeys returns every managed key.
func (s *KeyManagerService) ListKeys() ([]ManagedKeyDTO, error) {
	keys, err := s.svc.List()
	if err != nil {
		return nil, err
	}
	out := make([]ManagedKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, toKeyDTO(k))
	}
	return out, nil
}

// GenerateKey creates a key pair and registers it. Returns the entry and
// the authorized_keys-format public line (for copying to servers).
func (s *KeyManagerService) GenerateKey(req GenerateKeyRequestDTO) (ManagedKeyDTO, string, error) {
	if strings.TrimSpace(req.Name) == "" {
		return ManagedKeyDTO{}, "", fmt.Errorf("密钥名称不能为空")
	}
	key, pubkey, err := s.svc.Generate(appsvc.GenerateKeyRequest{
		Name:       req.Name,
		Algorithm:  req.Algorithm,
		Bits:       req.Bits,
		Passphrase: req.Passphrase,
		Comment:    req.Comment,
	})
	if err != nil {
		return ManagedKeyDTO{}, "", err
	}
	return toKeyDTO(key), pubkey, nil
}

// ImportKey registers an existing private key file.
func (s *KeyManagerService) ImportKey(req ImportKeyRequestDTO) (ManagedKeyDTO, error) {
	key, err := s.svc.Import(appsvc.ImportKeyRequest{
		Name:       req.Name,
		Path:       req.Path,
		Passphrase: req.Passphrase,
	})
	if err != nil {
		return ManagedKeyDTO{}, err
	}
	return toKeyDTO(key), nil
}

// DeleteKey removes a managed key (registry + private file).
func (s *KeyManagerService) DeleteKey(id string) error {
	return s.svc.Delete(id)
}

// ExportKey copies the private key to targetPath (0600) and writes the
// matching .pub next to it (when derivable). Returns the .pub path (empty
// for encrypted keys).
func (s *KeyManagerService) ExportKey(id, targetPath string) (string, error) {
	if strings.TrimSpace(targetPath) == "" {
		return "", fmt.Errorf("导出路径为空")
	}
	return s.svc.Export(id, targetPath)
}

package application

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// KeyManagerService is the 密钥管理器 (Phase 8 P2): managed SSH keys stored
// under <数据目录>/keys/ — an index.json registry plus one 0600 private-key
// file per key, kept VERBATIM (generated keys are OpenSSH format; imported
// keys keep their original OpenSSH/PEM encoding, encrypted included).
//
// Generate supports optional passphrase protection (OpenSSH aes256-ctr).
// Import accepts unencrypted and passphrase-protected OpenSSH/PEM files.
// Export copies the stored private key to a caller-chosen path and writes
// the matching authorized_keys-style public key next to it.
type KeyManagerService struct {
	dataDirs *DataDirService

	mu sync.Mutex
}

// ManagedKey is one entry of the key registry.
type ManagedKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Algorithm   string    `json:"algorithm"`   // "ssh-ed25519" | "ssh-rsa" | "ecdsa-sha2-nistp256"…
	Fingerprint string    `json:"fingerprint"` // SHA256:… (x/crypto format)
	Comment     string    `json:"comment"`
	Encrypted   bool      `json:"encrypted"`
	CreatedAt   time.Time `json:"createdAt"`
	FileName    string    `json:"fileName"`
}

// keyIndex is the persisted registry (index.json).
type keyIndex struct {
	Keys []ManagedKey `json:"keys"`
}

// GenerateRequest carries the generate/import parameters from the UI.
type GenerateKeyRequest struct {
	Name       string `json:"name"`
	Algorithm  string `json:"algorithm"` // "ed25519" | "rsa" | "ecdsa"
	Bits       int    `json:"bits"`      // rsa only (2048/3072/4096); 0 = 4096
	Passphrase string `json:"passphrase"`
	Comment    string `json:"comment"`
}

// ImportKeyRequest carries the import parameters from the UI.
type ImportKeyRequest struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Passphrase string `json:"passphrase"`
}

// NewKeyManagerService wires the service to the data directory.
func NewKeyManagerService(dataDirs *DataDirService) *KeyManagerService {
	return &KeyManagerService{dataDirs: dataDirs}
}

// Dir returns the managed-keys folder.
func (s *KeyManagerService) Dir() string {
	return filepath.Join(s.dataDirs.Current(), "keys")
}

// ── registry (index.json) ───────────────────────────────────────────────

func (s *KeyManagerService) readIndex() (keyIndex, error) {
	var idx keyIndex
	raw, err := os.ReadFile(filepath.Join(s.Dir(), "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return idx, err
	}
	err = json.Unmarshal(raw, &idx)
	return idx, err
}

func (s *KeyManagerService) writeIndex(idx keyIndex) error {
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir(), "index.json"), raw, 0o600)
}

// List returns every registered key.
func (s *KeyManagerService) List() ([]ManagedKey, error) {
	idx, err := s.readIndex()
	if err != nil {
		return nil, err
	}
	return idx.Keys, nil
}

// privateKeyPath resolves a registry entry's private-key file.
func (s *KeyManagerService) privateKeyPath(k ManagedKey) string {
	return filepath.Join(s.Dir(), k.FileName)
}

// ── generate ─────────────────────────────────────────────────────────────

// Generate creates a new key pair and registers it. An empty passphrase
// stores the key unencrypted; a passphrase encrypts it (OpenSSH
// aes256-ctr) and marks the registry entry as encrypted.
func (s *KeyManagerService) Generate(req GenerateKeyRequest) (ManagedKey, string, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ManagedKey{}, "", fmt.Errorf("密钥名称不能为空")
	}

	var privPub interface{} // public half (for ssh.NewPublicKey)
	var privForMarshal interface{}
	switch strings.ToLower(req.Algorithm) {
	case "ed25519":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return ManagedKey{}, "", err
		}
		privPub, privForMarshal = pub, priv
	case "rsa":
		bits := req.Bits
		if bits < 2048 {
			bits = 4096
		}
		priv, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return ManagedKey{}, "", err
		}
		privPub, privForMarshal = &priv.PublicKey, priv
	case "ecdsa":
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return ManagedKey{}, "", err
		}
		privPub, privForMarshal = &priv.PublicKey, priv
	default:
		return ManagedKey{}, "", fmt.Errorf("不支持的算法 %q", req.Algorithm)
	}

	sshPub, err := ssh.NewPublicKey(privPub)
	if err != nil {
		return ManagedKey{}, "", err
	}
	fingerprint := ssh.FingerprintSHA256(sshPub)
	authorizedKey := strings.TrimRight(string(ssh.MarshalAuthorizedKey(sshPub)), "\n")
	if req.Comment != "" {
		authorizedKey += " " + req.Comment
	}
	authorizedKey += "\n"

	var block *pem.Block
	if req.Passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privForMarshal, req.Comment, []byte(req.Passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(privForMarshal, req.Comment)
	}
	if err != nil {
		return ManagedKey{}, "", err
	}
	pemBytes := pem.EncodeToMemory(block)

	entry, _, err := s.store(ManagedKey{
		Name:        name,
		Algorithm:   sshPub.Type(),
		Fingerprint: fingerprint,
		Comment:     req.Comment,
		Encrypted:   req.Passphrase != "",
	}, pemBytes, authorizedKey)
	if err != nil {
		return ManagedKey{}, "", err
	}
	return entry, authorizedKey, nil
}

// ── import ───────────────────────────────────────────────────────────────

// Import registers an existing private key file (OpenSSH or PEM; unencrypted
// or passphrase-protected — the passphrase is only used to read the file,
// the bytes are stored verbatim). Returns the registry entry.
func (s *KeyManagerService) Import(req ImportKeyRequest) (ManagedKey, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(req.Path), filepath.Ext(req.Path))
	}
	raw, err := os.ReadFile(req.Path)
	if err != nil {
		return ManagedKey{}, fmt.Errorf("读取私钥文件: %w", err)
	}

	var parsed interface{}
	if req.Passphrase != "" {
		parsed, err = ssh.ParseRawPrivateKeyWithPassphrase(raw, []byte(req.Passphrase))
	} else {
		parsed, err = ssh.ParseRawPrivateKey(raw)
	}
	if err != nil {
		msg := err.Error()
		if req.Passphrase == "" &&
			(strings.Contains(msg, "encrypted") || strings.Contains(msg, "passphrase protected")) {
			return ManagedKey{}, fmt.Errorf("私钥已加密：请填写口令后重新导入")
		}
		return ManagedKey{}, fmt.Errorf("解析私钥: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(publicOf(parsed))
	if err != nil {
		return ManagedKey{}, fmt.Errorf("非支持的私钥类型: %w", err)
	}

	entry, _, err := s.store(ManagedKey{
		Name:        name,
		Algorithm:   sshPub.Type(),
		Fingerprint: ssh.FingerprintSHA256(sshPub),
		Encrypted:   req.Passphrase != "" && isEncryptedPEM(raw),
	}, raw, "")
	return entry, err
}

// publicOf extracts the public half of a parsed private key.
func publicOf(key interface{}) interface{} {
	switch k := key.(type) {
	case ed25519.PrivateKey:
		return k.Public()
	case *ed25519.PrivateKey:
		return k.Public()
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return key
	}
}

// isEncryptedPEM detects a passphrase-protected OpenSSH/PEM private key.
// isEncryptedPEM detects a passphrase-protected private key. Legacy PEM
// keys carry a "Proc-Type: 4,ENCRYPTED" header; OpenSSH-format keys keep
// the KDF name INSIDE the binary blob (magic + cipher/kdf length-prefixed
// strings) — encrypted means the KDF is anything but "none".
func isEncryptedPEM(raw []byte) bool {
	block, _ := pem.Decode(raw)
	if block == nil {
		return false
	}
	if block.Type != "OPENSSH PRIVATE KEY" {
		return strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED")
	}
	const magic = "openssh-key-v1\x00"
	data := block.Bytes
	if len(data) < len(magic)+4 || string(data[:len(magic)]) != magic {
		return false
	}
	rest := data[len(magic):]
	readStr := func() (string, []byte) {
		if len(rest) < 4 {
			return "", rest[len(rest):]
		}
		n := int(binary.BigEndian.Uint32(rest[:4]))
		if n > len(rest)-4 {
			return "", rest[len(rest):]
		}
		s := string(rest[4 : 4+n])
		return s, rest[4+n:]
	}
	_, rest = readStr() // cipher name
	kdf, rest := readStr()
	_ = rest
	return kdf != "" && kdf != "none"
}

// ── delete / export ──────────────────────────────────────────────────────

// Delete removes a key from the registry and deletes its private-key file.
func (s *KeyManagerService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.readIndex()
	if err != nil {
		return err
	}
	remaining := idx.Keys[:0]
	var removed *ManagedKey
	for _, k := range idx.Keys {
		if k.ID == id {
			kk := k
			removed = &kk
			continue
		}
		remaining = append(remaining, k)
	}
	if removed == nil {
		return fmt.Errorf("密钥不存在")
	}
	if err := s.writeIndex(keyIndex{Keys: remaining}); err != nil {
		return err
	}
	if err := os.Remove(s.privateKeyPath(*removed)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Export copies the stored private key verbatim to targetPath (0600) and
// writes the matching public key to targetPath + ".pub". Returns the public
// key path.
func (s *KeyManagerService) Export(id, targetPath string) (string, error) {
	k, err := s.byID(id)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(s.privateKeyPath(k))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(targetPath, raw, 0o600); err != nil {
		return "", err
	}

	authorized, authErr := s.authorizedKeyOf(k)
	pubPath := targetPath + ".pub"
	if authErr == nil {
		if werr := os.WriteFile(pubPath, []byte(authorized), 0o644); werr != nil {
			pubPath = ""
		}
	} else {
		pubPath = ""
	}
	return pubPath, nil
}

// authorizedKeyOf reconstructs the "algorithm key comment" public line from
// the stored private key. Encrypted keys cannot be re-parsed without the
// passphrase — they export the private file only.
func (s *KeyManagerService) authorizedKeyOf(k ManagedKey) (string, error) {
	raw, err := os.ReadFile(s.privateKeyPath(k))
	if err != nil {
		return "", err
	}
	if k.Encrypted {
		return "", fmt.Errorf("encrypted key: public key not derivable")
	}
	priv, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		return "", err
	}
	sshPub, err := ssh.NewPublicKey(publicOf(priv))
	if err != nil {
		return "", err
	}
	line := strings.TrimRight(string(ssh.MarshalAuthorizedKey(sshPub)), "\n")
	if k.Comment != "" {
		line += " " + k.Comment
	}
	return line + "\n", nil
}

// ── internal ─────────────────────────────────────────────────────────────

func (s *KeyManagerService) byID(id string) (ManagedKey, error) {
	idx, err := s.readIndex()
	if err != nil {
		return ManagedKey{}, err
	}
	for _, k := range idx.Keys {
		if k.ID == id {
			return k, nil
		}
	}
	return ManagedKey{}, fmt.Errorf("密钥不存在")
}

// store persists the private bytes under a fresh file name and appends the
// registry entry. authorizedKey may be empty (import keeps the original
// bytes; the public half is re-derivable for unencrypted keys).
func (s *KeyManagerService) store(k ManagedKey, pemBytes []byte, authorizedKey string) (ManagedKey, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return ManagedKey{}, "", err
	}
	idx, err := s.readIndex()
	if err != nil {
		return ManagedKey{}, "", err
	}
	k.ID = fmt.Sprintf("%x", sha1Sum(k.Name+k.Fingerprint+time.Now().String()))
	k.CreatedAt = time.Now()
	k.FileName = fmt.Sprintf("%s_%s", k.ID[:12], sanitizeLogSegment(k.Name)) + ".pem"

	// Name uniqueness: two keys may share a display name — ids differ.
	if err := os.WriteFile(s.privateKeyPath(k), pemBytes, 0o600); err != nil {
		return ManagedKey{}, "", err
	}
	if authorizedKey != "" {
		_ = os.WriteFile(s.privateKeyPath(k)+".pub", []byte(authorizedKey), 0o644)
	}
	idx.Keys = append(idx.Keys, k)
	if err := s.writeIndex(idx); err != nil {
		_ = os.Remove(s.privateKeyPath(k))
		return ManagedKey{}, "", err
	}
	return k, s.privateKeyPath(k) + ".pub", nil
}

// sha1Sum is a short internal digest helper for id generation (not
// security-relevant — uniqueness only).
func sha1Sum(s string) []byte {
	sum := sha1.Sum([]byte(s))
	return sum[:]
}

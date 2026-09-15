package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newTestKeyManager(t *testing.T) *KeyManagerService {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "keys-test-"+t.Name())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return NewKeyManagerService(NewDataDirService(dir, filepath.Join(dir, "ptr"), nil))
}

func TestKeyGenerateListExport(t *testing.T) {
	s := newTestKeyManager(t)

	key, pub, err := s.Generate(GenerateKeyRequest{
		Name: "deploy", Algorithm: "ed25519", Comment: "me@host",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if key.Algorithm != "ssh-ed25519" || key.Encrypted {
		t.Fatalf("unexpected entry: %+v", key)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") || !strings.Contains(pub, "me@host") {
		t.Fatalf("public key line wrong: %q", pub)
	}

	keys, err := s.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("list: %d keys, err %v", len(keys), err)
	}
	// Fingerprints are stable across restarts (index.json round trip).
	again, err := s.List()
	if err != nil || again[0].Fingerprint != keys[0].Fingerprint {
		t.Fatalf("fingerprint not stable")
	}

	// Export: private (verbatim OpenSSH PEM) + public sidecar path.
	target := filepath.Join(os.TempDir(), "keys-export-test", "id_ed25519")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(target)) })
	pubPath, err := s.Export(key.ID, target)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if pubPath != target+".pub" {
		t.Fatalf("pub path %q", pubPath)
	}
	priv, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(priv), "OPENSSH PRIVATE KEY") {
		t.Fatalf("exported private key malformed")
	}
	if _, err := ssh.ParseRawPrivateKey(priv); err != nil {
		t.Fatalf("exported key does not parse: %v", err)
	}
}

func TestKeyGenerateRSAPassphrase(t *testing.T) {
	s := newTestKeyManager(t)
	key, _, err := s.Generate(GenerateKeyRequest{
		Name: "rsa-1", Algorithm: "rsa", Bits: 2048, Passphrase: "hunter2",
	})
	if err != nil {
		t.Fatalf("generate rsa: %v", err)
	}
	if !key.Encrypted {
		t.Fatal("passphrase-protected key not marked encrypted")
	}
	// Without the passphrase the stored key cannot be parsed (that is the
	// point) — Delete must still work.
	if err := s.Delete(key.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	keys, _ := s.List()
	if len(keys) != 0 {
		t.Fatalf("delete left entries: %+v", keys)
	}
}

func TestKeyImportRoundTrip(t *testing.T) {
	s := newTestKeyManager(t)
	gen, _, err := s.Generate(GenerateKeyRequest{
		Name: "src", Algorithm: "ed25519", Comment: "orig",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Import the generated private file back under a new name.
	imp, err := s.Import(ImportKeyRequest{Name: "reimport", Path: s.privateKeyPath(gen)})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imp.Fingerprint != gen.Fingerprint {
		t.Fatalf("fingerprint mismatch: %s vs %s", imp.Fingerprint, gen.Fingerprint)
	}
}

func TestKeyImportEncryptedWithoutPassFails(t *testing.T) {
	s := newTestKeyManager(t)
	key, _, err := s.Generate(GenerateKeyRequest{
		Name: "enc", Algorithm: "ed25519", Passphrase: "pw",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	_, err = s.Import(ImportKeyRequest{Name: "copy", Path: s.privateKeyPath(key)})
	if err == nil || !strings.Contains(err.Error(), "加密") {
		t.Fatalf("expected encrypted-key hint, got %v", err)
	}
	// With the passphrase the import succeeds.
	if _, err := s.Import(ImportKeyRequest{Name: "copy", Path: s.privateKeyPath(key), Passphrase: "pw"}); err != nil {
		t.Fatalf("import with passphrase: %v", err)
	}
}

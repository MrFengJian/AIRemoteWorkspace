package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// TestAutoMigrateExisting verifies AutoMigrate creates the schema from scratch
// and that host/config/hostkey CRUD works end-to-end via GORM.
func TestAutoMigrateExisting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// 1. Migrate should create all tables.
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 2. Host CRUD via the repo.
	repo := NewHostRepo(s)
	h := domain.Host{ID: "test1", Name: "A", Host: "1.2.3.4", Port: 22, Username: "root", AuthType: "password"}
	if err := repo.Save(h); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := repo.Get("test1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "A" || got.TerminalTheme != "" {
		t.Fatalf("unexpected host: %+v", got)
	}

	// 3. Update TerminalTheme and re-read.
	h.TerminalTheme = "dracula"
	if err := repo.Save(h); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got2, _ := repo.Get("test1")
	if got2.TerminalTheme != "dracula" {
		t.Fatalf("theme not persisted: %+v", got2)
	}

	// 4. Config repo.
	crepo := NewConfigRepo(s)
	cfg := domain.DefaultConfig()
	cfg.LLM = domain.LLMConfig{BaseURL: "http://x", Model: "m"}
	if err := crepo.Set(cfg); err != nil {
		t.Fatalf("config set: %v", err)
	}
	gotCfg, err := crepo.Get()
	if err != nil || gotCfg.LLM.BaseURL != "http://x" {
		t.Fatalf("config get: %v %+v", err, gotCfg)
	}

	// 5. Host key repo.
	krepo := NewHostKeyRepo(s)
	if err := krepo.Upsert("test1", "ssh-ed25519", "SHA256:abc"); err != nil {
		t.Fatalf("hostkey upsert: %v", err)
	}
	// Overwrite.
	if err := krepo.Upsert("test1", "ssh-ed25519", "SHA256:def"); err != nil {
		t.Fatalf("hostkey upsert2: %v", err)
	}
	k, err := krepo.Get("test1")
	if err != nil || k.Fingerprint != "SHA256:def" {
		t.Fatalf("hostkey get: %v %+v", err, k)
	}

	// 6. Delete.
	if err := repo.Delete("test1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get("test1"); err == nil {
		t.Fatal("expected not found after delete")
	}

	t.Log("all gorm repo operations passed")
}

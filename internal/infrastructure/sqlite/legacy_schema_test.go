package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ai-remote/workspace/internal/domain"
)

// TestLegacyTEXTtimestamps simulates the OLD schema (created by the previous
// schema.sql): created_at/updated_at stored as RFC3339 TEXT strings. It proves
// GORM can still read those rows after migration.
func TestLegacyTEXTtimestamps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	// Create the legacy table shape with TEXT timestamps (as schema.sql did).
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE hosts (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		host       TEXT NOT NULL,
		port       INTEGER NOT NULL DEFAULT 22,
		username   TEXT NOT NULL,
		auth_type  TEXT NOT NULL DEFAULT 'password',
		secret_ref TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`).Error; err != nil {
		t.Fatal(err)
	}
	// Insert a row with RFC3339 TEXT timestamps (what the app wrote before).
	if err := db.Exec(`INSERT INTO hosts (id, name, host, port, username, auth_type, secret_ref, created_at, updated_at)
		VALUES ('legacy1', 'OldHost', '10.0.0.1', 22, 'root', 'password', '', '2026-08-12T10:00:00+00:00', '2026-08-12T10:00:00+00:00')`).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := db.DB()
	_ = raw.Close()

	// Now open with our real Store (which runs AutoMigrate) and read it back.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	repo := NewHostRepo(s)
	h, err := repo.Get("legacy1")
	if err != nil {
		t.Fatalf("get legacy host: %v", err)
	}
	if h.Name != "OldHost" {
		t.Fatalf("name mismatch: %q", h.Name)
	}
	t.Logf("legacy row read OK: name=%q created=%v theme=%q", h.Name, h.CreatedAt, h.TerminalTheme)

	// Saving it again must keep it readable (theme gets added).
	h.TerminalTheme = "nord"
	if err := repo.Save(h); err != nil {
		t.Fatalf("save legacy host: %v", err)
	}
	got, _ := repo.Get("legacy1")
	if got.TerminalTheme != "nord" || got.CreatedAt.IsZero() {
		t.Fatalf("after save: %+v", got)
	}
	t.Logf("post-save read OK (theme=%q, created=%v)", got.TerminalTheme, got.CreatedAt)

	_ = domain.Host{}
}

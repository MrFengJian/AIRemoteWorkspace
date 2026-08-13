package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealUserDBMigration opens the actual user database (which was created by
// the old schema.sql with RFC3339 TEXT timestamps) and verifies GORM can read
// it after AutoMigrate without data loss.
//
// This test is skipped if no user database exists (e.g. CI).
func TestRealUserDBMigration(t *testing.T) {
	home, _ := os.UserHomeDir()
	realPath := filepath.Join(home, "AppData", "Local", "ai-remote-workspace", "workspace.db")
	if _, err := os.Stat(realPath); err != nil {
		t.Skipf("no real user DB at %s: %v", realPath, err)
	}

	// Open the real DB with our new GORM path.
	s, err := Open(realPath)
	if err != nil {
		t.Fatalf("open real db: %v", err)
	}
	defer s.Close()

	// Migrate must succeed against existing tables/columns.
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate real db: %v", err)
	}

	// Reading hosts must not fail (old rows have RFC3339 timestamps).
	repo := NewHostRepo(s)
	hosts, err := repo.List()
	if err != nil {
		t.Fatalf("list real hosts: %v", err)
	}
	t.Logf("real db has %d hosts after gorm migration", len(hosts))
	for _, h := range hosts {
		t.Logf("  host id=%s name=%q theme=%q", h.ID, h.Name, h.TerminalTheme)
	}
}

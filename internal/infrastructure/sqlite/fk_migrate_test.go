package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func mustOpenRaw(path string, t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	return db
}

// TestFKLegacyMigration creates the exact legacy schema (with FK constraints
// on host_keys/sessions, TEXT timestamps), then opens via our Store and
// verifies AutoMigrate succeeds and data survives.
func TestFKLegacyMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	legacyDDL := `
CREATE TABLE hosts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    host       TEXT NOT NULL,
    port       INTEGER NOT NULL DEFAULT 22,
    username   TEXT NOT NULL,
    auth_type  TEXT NOT NULL DEFAULT 'password',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE host_keys (
    host_id     TEXT NOT NULL,
    algorithm   TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    first_seen  TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (host_id, algorithm),
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`
	rawDB := mustOpenRaw(path, t)
	if _, err := rawDB.Exec(legacyDDL); err != nil {
		t.Fatalf("exec legacy ddl: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO hosts (id, name, host, port, username) VALUES ('h1', 'Prod', '10.0.0.1', 22, 'root')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(`INSERT INTO host_keys (host_id, algorithm, fingerprint) VALUES ('h1', 'ssh-ed25519', 'SHA256:abc')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(`INSERT INTO settings (key, value) VALUES ('app', '{"theme":"dark"}')`); err != nil {
		t.Fatal(err)
	}
	_ = rawDB.Close()

	// Open through the real Store — triggers stripLegacyForeignKeys + AutoMigrate.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer s.Close()

	// Data must survive.
	repo := NewHostRepo(s)
	hosts, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hosts) != 1 || hosts[0].ID != "h1" {
		t.Fatalf("host data lost: %+v", hosts)
	}

	krepo := NewHostKeyRepo(s)
	k, err := krepo.Get("h1")
	if err != nil || k.Fingerprint != "SHA256:abc" {
		t.Fatalf("host key lost: %v %+v", err, k)
	}

	crepo := NewConfigRepo(s)
	cfg, err := crepo.Get()
	if err != nil || cfg.Theme != "dark" {
		t.Fatalf("settings lost: %v %+v", err, cfg)
	}

	t.Log("legacy FK schema migrated with data intact")
}

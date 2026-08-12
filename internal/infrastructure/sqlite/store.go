// Package sqlite is the local persistence implementation for the workspace.
//
// It embeds schema.sql and applies it idempotently on Open. The store stays
// generic (key/value settings + host rows) so the domain/application layers
// above it remain storage-agnostic.
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver; no CGO (AGENT.md §8.1)
)

//go:embed schema.sql
var schemaFS embed.FS

// Store wraps a SQLite connection. Methods are safe for concurrent use from
// the UI/services: modernc.org/sqlite serialises writes under the hood and we
// enable WAL for read concurrency.
type Store struct {
	db *sql.DB
}

// Open creates or opens the database at path and runs migrations.
// path should live in the user data dir (see xdg). Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	// Ensure the parent directory exists — SQLite returns a misleading
	// "unable to open database file (SQLITE_CANTOPEN)" when the parent dir is
	// missing rather than when the file itself is absent.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir %q: %w", dir, err)
		}
	}

	// _pragma keys tune modernc.org/sqlite for a desktop single-user workload.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// Single writer is the SQLite model; one connection pool is plenty.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB exposes the underlying *sql.DB for repositories that need it.
// Kept unexported-baggage-free: callers go through typed methods instead.
func (s *Store) DB() *sql.DB { return s.db }

// migrate runs schema.sql. It is idempotent (CREATE TABLE IF NOT EXISTS) and
// records the applied version for future incremental migrations.
func (s *Store) migrate() error {
	schema, err := fs.ReadFile(schemaFS, "schema.sql")
	if err != nil {
		return fmt.Errorf("read embedded schema: %w", err)
	}
	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

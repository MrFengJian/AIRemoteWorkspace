// Package sqlite is the local persistence implementation for the workspace.
//
// It uses GORM (gorm.io/gorm) with the pure-Go sqlite driver
// github.com/glebarez/sqlite (built on modernc.org/sqlite), so the whole
// stack stays CGO-free — preserving the single-binary, no-dependency promise
// (AGENT.md §8.1).
//
// Schema is managed by GORM AutoMigrate from the structs in models.go: tables
// are created and altered automatically on startup, replacing the old
// hand-maintained schema.sql.
package sqlite

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store wraps the GORM database handle. Methods are safe for concurrent use
// from the UI/services (GORM handles pooling; WAL keeps reads concurrent).
type Store struct {
	db *gorm.DB
}

// Open creates or opens the database at path and runs AutoMigrate.
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

	// glebarez/sqlite accepts a DSN that includes pragmas, same as modernc.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// Keep logs quiet for a desktop app; errors are still surfaced.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	raw, err := s.db.DB()
	if err != nil {
		return err
	}
	return raw.Close()
}

// DB exposes the underlying *gorm.DB for repositories.
func (s *Store) DB() *gorm.DB { return s.db }

// migrate prepares the schema for GORM and then AutoMigrates. It first strips
// legacy FOREIGN KEY constraints (which break glebarez's migrator), then lets
// AutoMigrate create/alter the tables from the model structs. AutoMigrate is
// idempotent and additive: it adds missing tables/columns/indexes and never
// drops data.
func (s *Store) migrate() error {
	if err := stripLegacyForeignKeys(s.db); err != nil {
		return fmt.Errorf("prepare legacy schema: %w", err)
	}
	return s.db.AutoMigrate(
		&hostModel{},
		&hostKeyModel{},
		&settingModel{},
		&sessionModel{},
		&secretRefModel{},
	)
}

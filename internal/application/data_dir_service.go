package application

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DataDirService owns the application data directory: it reports the current
// location and migrates the whole directory (SQLite database, skills,
// anything else inside) to a new one.
//
// The chosen location is persisted in a small pointer file under the config
// directory — the default location would move away with the data itself, so
// it cannot remember its own replacement.
type DataDirService struct {
	defaultDir  string
	pointerPath string
	store       MigratableStore

	// onMigrated fires after a successful migration with the new dir
	// (main.go uses it to repoint the skills directory).
	onMigrated func(newDir string)
}

// MigratableStore is the persistence handle the migration needs: closing
// releases the SQLite file locks required before moving the files (Windows),
// reopening at the new location keeps every repo holding the same store
// working in place — no restart needed.
type MigratableStore interface {
	Close() error
	ReopenAt(path string) error
}

// DataDirInfo describes the current data directory for the settings UI.
type DataDirInfo struct {
	Path       string `json:"path"`
	IsDefault  bool   `json:"isDefault"`
	TotalBytes int64  `json:"totalBytes"`
}

// NewDataDirService wires the service. defaultDir is the xdg data location;
// pointerPath is the file remembering a migrated location.
func NewDataDirService(defaultDir, pointerPath string, store MigratableStore) *DataDirService {
	return &DataDirService{defaultDir: defaultDir, pointerPath: pointerPath, store: store}
}

// SetOnMigrated registers the post-migration hook.
func (s *DataDirService) SetOnMigrated(fn func(newDir string)) {
	s.onMigrated = fn
}

// ResolveDataDir returns the effective data directory: the one recorded in
// the pointer file when present and valid, otherwise the default. Called at
// startup BEFORE anything opens the store.
func ResolveDataDir(pointerPath, defaultDir string) string {
	raw, err := os.ReadFile(pointerPath)
	if err == nil {
		if p := strings.TrimSpace(string(raw)); p != "" {
			return p
		}
	}
	return defaultDir
}

// Current returns the effective data directory.
func (s *DataDirService) Current() string {
	return ResolveDataDir(s.pointerPath, s.defaultDir)
}

// Info reports the current path plus whether it is the default location and
// the total size of everything inside.
func (s *DataDirService) Info() DataDirInfo {
	current := s.Current()
	return DataDirInfo{
		Path:       current,
		IsDefault:  current == s.defaultDir,
		TotalBytes: dirSize(current),
	}
}

// Migrate moves the whole data directory to target: closes the database
// (releasing the WAL file locks), copies everything, updates the pointer,
// reopens the database at the new location, then deletes the originals.
// Any failure rolls back cleanly — the app keeps running from the old
// location.
func (s *DataDirService) Migrate(target string) error {
	target = filepath.Clean(target)
	current := filepath.Clean(s.Current())
	abs := filepath.Abs
	if t, err := abs(target); err == nil {
		target = t
	}
	if c, err := abs(current); err == nil {
		current = c
	}
	if target == current {
		return errors.New("目标目录与当前数据目录相同")
	}
	if strings.HasPrefix(current, target+string(filepath.Separator)) ||
		strings.HasPrefix(target, current+string(filepath.Separator)) {
		return errors.New("目标目录不能位于当前数据目录内部")
	}

	// The target must exist (created here) and be empty — migrating onto
	// foreign files would silently mix data.
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}
	targetEntries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(targetEntries) > 0 {
		return errors.New("目标目录不为空")
	}
	probe := filepath.Join(target, ".write-probe")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		return fmt.Errorf("target not writable: %w", err)
	}
	_ = os.Remove(probe)

	// Release the SQLite WAL file locks (Windows keeps the files locked
	// while the pool is open). Reopen at the old location on any failure.
	if err := s.store.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	migrated := false
	defer func() {
		if !migrated {
			_ = s.store.ReopenAt(filepath.Join(current, dbFileName))
		}
	}()

	if err := copyTree(current, target); err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("copy data: %w", err)
	}

	if err := writePointer(s.pointerPath, target); err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("record data dir: %w", err)
	}
	if err := s.store.ReopenAt(filepath.Join(target, dbFileName)); err != nil {
		_ = os.RemoveAll(target)
		_ = os.Remove(s.pointerPath)
		return fmt.Errorf("reopen database: %w", err)
	}

	// Success — the originals were copied; remove them (true move).
	for _, e := range entries(current) {
		_ = os.RemoveAll(filepath.Join(current, e))
	}
	migrated = true
	if s.onMigrated != nil {
		s.onMigrated(target)
	}
	return nil
}

// dbFileName is the SQLite file every data directory contains.
const dbFileName = "workspace.db"

// writePointer records the migrated location (best-effort dir creation).
func writePointer(pointerPath, dir string) error {
	if err := os.MkdirAll(filepath.Dir(pointerPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pointerPath, []byte(dir+"\n"), 0o644)
}

// entries lists the names of a directory's direct children.
func entries(dir string) []string {
	out, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(out))
	for _, e := range out {
		names = append(names, e.Name())
	}
	return names
}

// copyTree copies every child of src into dst (files + directories,
// recursively). src/dst must exist.
func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
			if err := copyTree(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// dirSize sums the size of every file under dir (best effort).
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

package application

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"github.com/ai-remote/workspace/internal/domain"
)

// builtinExpertFS holds the embedded expert directories — the binary's
// canonical expert set, laid out per industry practice (manifest.json +
// SOUL.md + optional HEARTBEAT.md, plus an optional private skills/ tree).
// They are seeded into the experts root on startup; user-edited files are
// never overwritten, and a dismissed builtin is never resurrected.
//
//go:embed all:experts
var builtinExpertFS embed.FS

// builtinExperts parses the embedded expert directories into domain.Expert
// definitions (sorted by SortOrder).
func builtinExperts() []domain.Expert {
	entries, err := fs.Glob(builtinExpertFS, "experts/*/manifest.json")
	if err != nil {
		return nil
	}
	out := make([]domain.Expert, 0, len(entries))
	for _, path := range entries {
		e, err := expertFromEmbeddedDir(path)
		if err != nil {
			continue // malformed builtin — skip, never break the roster
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}

// expertFromEmbeddedDir loads one embedded expert directory: manifest.json
// for the identity card, SOUL.md (or IDENTITY.md) for the persona core and
// HEARTBEAT.md for the operational guidelines.
func expertFromEmbeddedDir(manifestPath string) (domain.Expert, error) {
	dir := manifestPath[:len(manifestPath)-len("/manifest.json")]
	raw, err := builtinExpertFS.ReadFile(manifestPath)
	if err != nil {
		return domain.Expert{}, err
	}
	var m ExpertManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return domain.Expert{}, fmt.Errorf("%s: %w", manifestPath, err)
	}
	dirName := dir[len("experts/"):]
	if m.ID == "" {
		m.ID = dirName
	}
	if m.ID != dirName {
		return domain.Expert{}, fmt.Errorf("manifest id %q does not match directory %q", m.ID, dirName)
	}
	e := m.expert()
	// Embedded experts are builtins by definition, seeded enabled.
	e.Builtin = true
	e.Enabled = true
	soul, err := readEmbeddedFirst(builtinExpertFS, dir, expertSoulFile, expertSoulAltFile)
	if err != nil {
		return domain.Expert{}, err
	}
	e.SystemPrompt = soul
	if hb, err := readEmbeddedFirst(builtinExpertFS, dir, expertHeartbeatFile); err == nil {
		e.Heartbeat = hb
	}
	return e, nil
}

// readEmbeddedFirst reads the first existing file (relative to the embedded
// expert directory); missing files yield "" with a nil error.
func readEmbeddedFirst(fsys embed.FS, dir string, names ...string) (string, error) {
	for _, n := range names {
		raw, err := fsys.ReadFile(dir + "/" + n)
		if err == nil {
			return string(raw), nil
		}
	}
	return "", nil
}

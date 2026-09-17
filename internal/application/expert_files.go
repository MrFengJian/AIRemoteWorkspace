package application

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// Expert directory layout (industry practice): one directory per expert
// under the experts root, mirroring the DB row so experts are portable,
// hand-editable and diffable.
//
//	<experts>/<id>/manifest.json   identity card (id/label/description/icon/color + bindings)
//	<experts>/<id>/SOUL.md         persona core & behavioral instructions (IDENTITY.md accepted on import)
//	<experts>/<id>/HEARTBEAT.md    operational / periodic-task guidelines (optional)
//	<experts>/<id>/skills/         expert-private skill packs (shadow same-name public packs)
const (
	expertManifestFile   = "manifest.json"
	expertSoulFile       = "SOUL.md"
	expertSoulAltFile    = "IDENTITY.md"
	expertHeartbeatFile  = "HEARTBEAT.md"
	expertPrivateSkills  = "skills"
	expertManifestFormat = 1
)

// ExpertManifest is the manifest.json identity card of an expert directory.
// It mirrors the roster-visible fields of domain.Expert; the persona texts
// live in SOUL.md / HEARTBEAT.md next to it.
type ExpertManifest struct {
	Format           int      `json:"format"`
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Role             string   `json:"role,omitempty"`
	Description      string   `json:"description,omitempty"`
	Icon             string   `json:"icon,omitempty"`
	Color            string   `json:"color,omitempty"`
	SortOrder        int      `json:"sortOrder,omitempty"`
	AutoSnapshot     bool     `json:"autoSnapshot,omitempty"`
	Temperature      float64  `json:"temperature,omitempty"`
	MaxSteps         int      `json:"maxSteps,omitempty"`
	Policy           string   `json:"policy,omitempty"`
	ProviderID       string   `json:"providerId,omitempty"`
	Model            string   `json:"model,omitempty"`
	OpeningMessage   string   `json:"openingMessage,omitempty"`
	SuggestedPrompts []string `json:"suggestedPrompts,omitempty"`
	AllowedTools     []string `json:"allowedTools,omitempty"`
	SkillRefs        []string `json:"skillRefs,omitempty"`
}

// expert projects the manifest onto a domain.Expert (persona fields empty —
// they come from SOUL.md / HEARTBEAT.md).
func (m ExpertManifest) expert() domain.Expert {
	return domain.Expert{
		ID:               m.ID,
		Name:             m.Label,
		Role:             m.Role,
		Description:      m.Description,
		Icon:             m.Icon,
		Color:            m.Color,
		SortOrder:        m.SortOrder,
		AutoSnapshot:     m.AutoSnapshot,
		Temperature:      m.Temperature,
		MaxSteps:         m.MaxSteps,
		Policy:           m.Policy,
		ProviderID:       m.ProviderID,
		Model:            m.Model,
		OpeningMessage:   m.OpeningMessage,
		SuggestedPrompts: m.SuggestedPrompts,
		AllowedTools:     m.AllowedTools,
		SkillRefs:        m.SkillRefs,
	}
}

// manifest projects an expert row onto the identity card.
func expertToManifest(e domain.Expert) ExpertManifest {
	return ExpertManifest{
		Format:           expertManifestFormat,
		ID:               e.ID,
		Label:            e.Name,
		Role:             e.Role,
		Description:      e.Description,
		Icon:             e.Icon,
		Color:            e.Color,
		SortOrder:        e.SortOrder,
		AutoSnapshot:     e.AutoSnapshot,
		Temperature:      e.Temperature,
		MaxSteps:         e.MaxSteps,
		Policy:           e.Policy,
		ProviderID:       e.ProviderID,
		Model:            e.Model,
		OpeningMessage:   e.OpeningMessage,
		SuggestedPrompts: e.SuggestedPrompts,
		AllowedTools:     e.AllowedTools,
		SkillRefs:        e.SkillRefs,
	}
}

// ExpertDir returns the on-disk directory of one expert.
func (s *ExpertService) ExpertDir(id string) string {
	return filepath.Join(s.dir, id)
}

// seedEmbeddedExpertFiles writes the embedded top-level files (manifest.json,
// SOUL.md, HEARTBEAT.md) of one builtin expert into the experts root,
// skipping files that already exist (user edits win).
func (s *ExpertService) seedEmbeddedExpertFiles(id string) {
	entries, err := fs.ReadDir(builtinExpertFS, "experts/"+id)
	if err != nil {
		return
	}
	dir := s.ExpertDir(id)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") &&
			!strings.HasSuffix(entry.Name(), ".md") {
			continue // skill subtrees and odd files are not part of seeding
		}
		target := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(target); err == nil {
			continue // exists (possibly user-edited) — never overwrite
		}
		raw, err := builtinExpertFS.ReadFile("experts/" + id + "/" + entry.Name())
		if err != nil {
			continue
		}
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(target, raw, 0o644)
	}
}

// fillExpertFilesFromRow creates only the MISSING persona files of an
// existing expert row (startup path for pre-layout installs): a user-edited
// SOUL.md/manifest.json on disk is never regressed by the row, and the row's
// persona fills the gaps so the directory layout materializes losslessly.
func (s *ExpertService) fillExpertFilesFromRow(e domain.Expert) {
	dir := s.ExpertDir(e.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	ensure := func(name, content string) {
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			return // exists — never overwrite
		}
		_ = os.WriteFile(target, []byte(content), 0o644)
	}
	manifest, err := json.MarshalIndent(expertToManifest(e), "", "  ")
	if err == nil {
		ensure(expertManifestFile, string(manifest)+"\n")
	}
	ensure(expertSoulFile, e.SystemPrompt)
	if strings.TrimSpace(e.Heartbeat) != "" {
		ensure(expertHeartbeatFile, e.Heartbeat)
	}
}

// mirrorExpertToFiles writes an expert row's current state to its directory
// (manifest.json + SOUL.md + HEARTBEAT.md). Used on save (write-through) and
// when an already-existing builtin row gains its directory for the first time
// — there the ROW wins, so a user-edited persona is never regressed by the
// embedded defaults. HEARTBEAT.md is removed when the field is empty.
func (s *ExpertService) mirrorExpertToFiles(e domain.Expert) error {
	dir := s.ExpertDir(e.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(expertToManifest(e), "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, expertManifestFile),
			append(manifest, '\n'), 0o644)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, expertSoulFile), []byte(e.SystemPrompt), 0o644)
	}
	if err != nil {
		return err
	}
	hbPath := filepath.Join(dir, expertHeartbeatFile)
	if strings.TrimSpace(e.Heartbeat) == "" {
		_ = os.Remove(hbPath)
		return nil
	}
	return os.WriteFile(hbPath, []byte(e.Heartbeat), 0o644)
}

// loadExpertFiles overlays the directory persona files onto an expert row:
// SOUL.md (or IDENTITY.md) fills SystemPrompt, HEARTBEAT.md fills Heartbeat.
// Missing files leave the row fields untouched (custom experts created
// before the directory layout keep working until their next save).
func (s *ExpertService) loadExpertFiles(e *domain.Expert) {
	dir := s.ExpertDir(e.ID)
	if raw, err := os.ReadFile(filepath.Join(dir, expertSoulFile)); err == nil {
		e.SystemPrompt = string(raw)
	} else if raw, err := os.ReadFile(filepath.Join(dir, expertSoulAltFile)); err == nil {
		e.SystemPrompt = string(raw)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, expertHeartbeatFile)); err == nil {
		e.Heartbeat = string(raw)
	}
}

// readManifestFile parses an expert directory's manifest.json.
func readManifestFile(path string) (ExpertManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ExpertManifest{}, err
	}
	var m ExpertManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return ExpertManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	if m.Format != 0 && m.Format != expertManifestFormat {
		return ExpertManifest{}, fmt.Errorf("unsupported manifest format %d", m.Format)
	}
	return m, nil
}

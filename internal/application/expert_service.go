package application

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// ExpertService manages the ops-expert roster (运维专家):
// builtin experts seeded from the binary plus user-defined ones, with CRUD,
// enable/disable and the builtin-dismissal lifecycle. It also feeds the
// agent runtime as its ExpertSource (GetExpert).
//
// Since the directory layout refactor each expert also materializes as a
// directory under dir (manifest.json + SOUL.md + HEARTBEAT.md + optional
// private skills/) — see expert_files.go. The DB row stays the roster
// source of truth; the files are its portable mirror and are overlaid on
// GetExpert so hand edits take effect without a UI round-trip.
type ExpertService struct {
	repo ExpertRepository
	dir  string
}

// NewExpertService wires the service over the expert repository and the
// experts root directory, seeding every builtin expert that is missing.
// Seeding failures on individual experts are ignored — a partially seeded
// roster beats a broken startup, and the next start retries.
func NewExpertService(repo ExpertRepository, dir string) *ExpertService {
	svc := &ExpertService{repo: repo, dir: dir}
	_ = os.MkdirAll(dir, 0o755)
	svc.seedBuiltins()
	return svc
}

// SetDir repoints the experts root (data-dir migration) and reseeds.
func (s *ExpertService) SetDir(dir string) {
	s.dir = dir
	_ = os.MkdirAll(dir, 0o755)
	s.seedBuiltins()
}

// seedBuiltins inserts the builtin experts that have no row yet and
// materializes their directories. Existing rows win as soon as they exist —
// user-edited builtins are never overwritten, and dismissed ones stay
// dismissed — but their directory files are filled in from the row when
// missing, so the layout materializes losslessly. The one exception is the
// SkillRefs upgrade below: it tops up bindings on rows that still carry the
// pre-skillhub default signature.
func (s *ExpertService) seedBuiltins() {
	for _, def := range builtinExperts() {
		existing, err := s.repo.Get(def.ID)
		if err != nil {
			s.seedEmbeddedExpertFiles(def.ID)
			if err := s.repo.Save(def); err != nil {
				continue // best-effort; retried on next startup
			}
			continue
		}
		s.upgradeBuiltinSkillRefs(def, existing)
		s.fillExpertFilesFromRow(existing)
	}
}

// legacyBuiltinSkillRefs records the default SkillRefs as shipped before the
// skillhub-sourced packs. A stored builtin row whose SkillRefs still equals
// this signature was never customized by the user, so it is safe to refresh
// to the current defaults. Any other value (including a deliberate removal)
// is left untouched.
var legacyBuiltinSkillRefs = map[string][]string{
	domain.ExpertIDDiagnosticsSRE: {},
	domain.ExpertIDK8sOps:         {},
	domain.ExpertIDK8sDeveloper:   {},
	domain.ExpertIDDocker:         {"container-restart-loop"},
	domain.ExpertIDLinuxSys:       {},
	domain.ExpertIDDatabase:       {},
}

// upgradeBuiltinSkillRefs refreshes an existing builtin row's SkillRefs to
// the current defaults when the row still carries the legacy signature.
func (s *ExpertService) upgradeBuiltinSkillRefs(def, existing domain.Expert) {
	legacy, ok := legacyBuiltinSkillRefs[def.ID]
	if !ok || len(def.SkillRefs) == 0 || !equalStringSlices(existing.SkillRefs, legacy) {
		return
	}
	existing.SkillRefs = def.SkillRefs
	_ = s.repo.Save(existing)
}

// equalStringSlices compares two string sets order-insensitively.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	count := make(map[string]int, len(a))
	for _, v := range a {
		count[v]++
	}
	for _, v := range b {
		if count[v] == 0 {
			return false
		}
		count[v]--
	}
	return true
}

// List returns the visible roster: every expert except dismissed builtins.
func (s *ExpertService) ListExperts() ([]domain.Expert, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]domain.Expert, 0, len(all))
	for _, e := range all {
		if e.Dismissed {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// GetExpert returns one expert by id, including disabled ones (a session may
// still reference an expert that was disabled after being selected). The
// directory persona files (SOUL.md / HEARTBEAT.md) overlay the row, so
// hand edits take effect without a UI round-trip.
// Implements the agent runtime's ExpertSource.
func (s *ExpertService) GetExpert(id string) (domain.Expert, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Expert{}, errors.New("empty expert id")
	}
	e, err := s.repo.Get(id)
	if err != nil {
		return domain.Expert{}, err
	}
	s.loadExpertFiles(&e)
	return e, nil
}

// SaveExpert creates or updates an expert. Custom experts get a generated id
// on create; builtin experts are updated in place (the stored Builtin flag is
// authoritative — a client cannot forge it). Name is required; unknown
// policy values normalize to "" (follow the session's policy). The saved
// row is mirrored back to its directory files (write-through).
func (s *ExpertService) SaveExpert(e domain.Expert) (domain.Expert, error) {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return domain.Expert{}, errors.New("专家名称不能为空 / expert name is required")
	}
	e.Role = strings.TrimSpace(e.Role)
	e.Description = strings.TrimSpace(e.Description)
	e.SystemPrompt = strings.TrimSpace(e.SystemPrompt)
	e.Heartbeat = strings.TrimSpace(e.Heartbeat)
	e.Policy = normalizeExpertPolicy(e.Policy)
	e.SkillRefs = cleanList(e.SkillRefs)
	e.AllowedTools = cleanList(e.AllowedTools)
	e.SuggestedPrompts = cleanList(e.SuggestedPrompts)

	if existing, err := s.repo.Get(e.ID); err == nil {
		// Update: the identity flags stay authoritative from storage, and
		// re-saving a dismissed builtin restores it to the roster.
		e.Builtin = existing.Builtin
		e.Dismissed = false
		e.CreatedAt = existing.CreatedAt
	} else {
		// Create: builtin shapes come from the binary only; the repo stamps
		// CreatedAt.
		e.Builtin = false
		e.Dismissed = false
		if e.ID == "" || isBuiltinExpertID(e.ID) {
			e.ID = newID()
		}
		if e.SortOrder == 0 {
			e.SortOrder = 100
		}
	}
	if e.ID == "" {
		return domain.Expert{}, errors.New("empty expert id")
	}
	if err := s.repo.Save(e); err != nil {
		return domain.Expert{}, fmt.Errorf("save expert: %w", err)
	}
	// Write-through the persona files; the DB row stays authoritative, so a
	// mirror failure only costs portability, not correctness.
	_ = s.mirrorExpertToFiles(e)
	return e, nil
}

// DeleteExpert removes an expert. Builtins are only dismissed (their row
// stays, hidden and never re-seeded) so the persona can be restored by
// saving it again; custom experts are deleted outright along with their
// directory.
func (s *ExpertService) DeleteExpert(id string) error {
	e, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if !e.Builtin {
		if err := s.repo.Delete(id); err != nil {
			return err
		}
		_ = os.RemoveAll(s.ExpertDir(id))
		return nil
	}
	e.Dismissed = true
	e.Enabled = false
	return s.repo.Save(e)
}

// isBuiltinExpertID reports whether id belongs to the builtin namespace.
func isBuiltinExpertID(id string) bool {
	return strings.HasPrefix(id, "builtin-")
}

// normalizeExpertPolicy maps unknown values to "" (follow the session).
func normalizeExpertPolicy(p string) string {
	switch domain.SessionPolicy(p) {
	case domain.PolicyStrict:
		return string(domain.PolicyStrict)
	case domain.PolicyAutoWrite:
		return string(domain.PolicyAutoWrite)
	default:
		return ""
	}
}

// cleanList trims and drops empty entries from a string list.
func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

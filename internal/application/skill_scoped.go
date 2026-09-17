package application

import (
	"os"
	"path/filepath"

	"github.com/ai-remote/workspace/internal/domain"
)

// scopedSkillStore is an expert-scoped view over the skill system: the
// expert's private packs (under <experts>/<id>/skills/) shadow same-name
// public packs, and private packs never appear in the public listing —
// the store is only handed to the agent runtime for that expert's sessions.
// Implements domain.SkillStore.
type scopedSkillStore struct {
	base        *SkillService
	privateRoot string
}

// SkillSourceFor returns an expert-scoped skill store (private packs first,
// shadowing same-name public packs). ok is false when the expert has no
// private skills directory yet — callers then keep the global store. The
// experts root is wired via SetExpertsRoot; without it scoping is disabled.
func (s *SkillService) SkillSourceFor(expertID string) (domain.SkillStore, bool) {
	if s.expertsRoot == "" || !skillNameRe.MatchString(expertID) {
		return nil, false
	}
	privateRoot := filepath.Join(s.expertsRoot, expertID, "skills")
	if privateRoot == s.dir {
		return nil, false
	}
	if _, err := os.Stat(privateRoot); err != nil {
		return nil, false // no private skills — global view is exact
	}
	return &scopedSkillStore{base: s, privateRoot: privateRoot}, true
}

// ListSkills returns the expert's private packs followed by the public packs
// that are not shadowed (same name) — the tool-description listing for that
// expert's sessions.
func (v *scopedSkillStore) ListSkills() ([]domain.Skill, error) {
	priv, err := listFromRoot(v.privateRoot)
	if err != nil {
		return nil, err
	}
	pub, err := listFromRoot(v.base.dir)
	if err != nil {
		return nil, err
	}
	shadow := make(map[string]bool, len(priv))
	for _, p := range priv {
		shadow[p.Name] = true
	}
	out := make([]domain.Skill, 0, len(priv)+len(pub))
	out = append(out, priv...)
	for _, p := range pub {
		if !shadow[p.Name] {
			out = append(out, p)
		}
	}
	return out, nil
}

// GetSkill resolves a skill by name: the expert's private pack wins over a
// same-name public pack.
func (v *scopedSkillStore) GetSkill(name string) (domain.Skill, error) {
	if sk, err := getFromRoot(v.privateRoot, name); err == nil {
		return sk, nil
	}
	return getFromRoot(v.base.dir, name)
}

// ReadSkillFile reads a bundled file with the same private-first resolution.
func (v *scopedSkillStore) ReadSkillFile(name, path string) (string, error) {
	if raw, err := fileFromRoot(v.privateRoot, name, path); err == nil {
		return string(raw), nil
	}
	raw, err := fileFromRoot(v.base.dir, name, path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

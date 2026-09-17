package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// memExpertRepo is an in-memory ExpertRepository for tests.
type memExpertRepo struct {
	items map[string]domain.Expert
}

func newMemExpertRepo() *memExpertRepo {
	return &memExpertRepo{items: make(map[string]domain.Expert)}
}

func (r *memExpertRepo) List() ([]domain.Expert, error) {
	out := make([]domain.Expert, 0, len(r.items))
	for _, e := range r.items {
		out = append(out, e)
	}
	return out, nil
}

func (r *memExpertRepo) Get(id string) (domain.Expert, error) {
	e, ok := r.items[id]
	if !ok {
		return domain.Expert{}, errors.New("not found")
	}
	return e, nil
}

func (r *memExpertRepo) Save(e domain.Expert) error {
	r.items[e.ID] = e
	return nil
}

func (r *memExpertRepo) Delete(id string) error {
	delete(r.items, id)
	return nil
}

func TestExpertServiceSeedsBuiltins(t *testing.T) {
	repo := newMemExpertRepo()
	svc := NewExpertService(repo, t.TempDir())

	all, err := svc.ListExperts()
	if err != nil {
		t.Fatalf("ListExperts: %v", err)
	}
	if len(all) != len(builtinExperts()) {
		t.Fatalf("expected %d seeded experts, got %d", len(builtinExperts()), len(all))
	}
	// The unified diagnosis expert must be present with AutoSnapshot on.
	dx, err := svc.GetExpert(domain.ExpertIDDiagnosticsSRE)
	if err != nil {
		t.Fatalf("diagnosis expert missing: %v", err)
	}
	if !dx.AutoSnapshot || !dx.Builtin {
		t.Fatalf("diagnosis expert flags wrong: autoSnapshot=%v builtin=%v", dx.AutoSnapshot, dx.Builtin)
	}
	if dx.SystemPrompt == "" {
		t.Fatal("diagnosis expert has empty persona prompt")
	}
	if dx.Heartbeat == "" {
		t.Fatal("diagnosis expert has no heartbeat guidance")
	}
}

// The general assistant is the default expert: builtin, first in the roster,
// and carrying the standard welcome card (opening message + prompts).
func TestBuiltinGeneralAssistant(t *testing.T) {
	defs := builtinExperts()
	if len(defs) == 0 || defs[0].ID != domain.ExpertIDGeneralAssistant {
		t.Fatalf("general assistant must sort first, got %v", defs[0].ID)
	}
	if !defs[0].Builtin || !defs[0].Enabled {
		t.Fatal("general assistant must be an enabled builtin")
	}
	if defs[0].Name == "" || defs[0].OpeningMessage == "" || len(defs[0].SuggestedPrompts) == 0 {
		t.Fatal("general assistant lacks welcome card fields")
	}
}

// Expert directories materialize next to the DB rows, overlay hand edits on
// read, write through on save, and disappear with a custom expert.
func TestExpertDirectoryLifecycle(t *testing.T) {
	repo := newMemExpertRepo()
	dir := t.TempDir()
	svc := NewExpertService(repo, dir)

	// Builtin dirs seeded from the embedded tree.
	soulPath := filepath.Join(dir, domain.ExpertIDK8sOps, "SOUL.md")
	manifestPath := filepath.Join(dir, domain.ExpertIDK8sOps, "manifest.json")
	for _, p := range []string{soulPath, manifestPath,
		filepath.Join(dir, domain.ExpertIDK8sOps, "HEARTBEAT.md")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected seeded file %s: %v", p, err)
		}
	}

	// Hand edit wins on read: overlay SOUL.md onto the row.
	if err := os.WriteFile(soulPath, []byte("HAND EDITED SOUL"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := svc.GetExpert(domain.ExpertIDK8sOps)
	if err != nil {
		t.Fatal(err)
	}
	if e.SystemPrompt != "HAND EDITED SOUL" {
		t.Fatalf("SOUL.md overlay not applied: %q", e.SystemPrompt)
	}

	// UI save writes through: the file reflects the saved row again.
	e.SystemPrompt = "UI SAVED SOUL"
	e.Heartbeat = "UI SAVED HEARTBEAT"
	if _, err := svc.SaveExpert(e); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(soulPath)
	if string(raw) != "UI SAVED SOUL" {
		t.Fatalf("SOUL.md not written through: %q", raw)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, domain.ExpertIDK8sOps, "HEARTBEAT.md"))
	if string(raw) != "UI SAVED HEARTBEAT" {
		t.Fatalf("HEARTBEAT.md not written through: %q", raw)
	}
	// And the next read comes back from the file.
	e2, _ := svc.GetExpert(domain.ExpertIDK8sOps)
	if e2.Heartbeat != "UI SAVED HEARTBEAT" {
		t.Fatalf("heartbeat overlay broken: %q", e2.Heartbeat)
	}

	// Custom expert: dir created on save, removed on delete.
	custom, err := svc.SaveExpert(domain.Expert{Name: "临时专家", SystemPrompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, custom.ID, "SOUL.md")); err != nil {
		t.Fatalf("custom expert dir missing: %v", err)
	}
	if err := svc.DeleteExpert(custom.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, custom.ID)); !os.IsNotExist(err) {
		t.Fatal("custom expert dir should be removed on delete")
	}
}

func TestExpertServiceSeedNeverOverwritesEdits(t *testing.T) {
	repo := newMemExpertRepo()
	// Pre-create an edited builtin.
	repo.items[domain.ExpertIDDocker] = domain.Expert{
		ID: domain.ExpertIDDocker, Name: "我的 Docker", Builtin: true, Enabled: true,
	}
	NewExpertService(repo, t.TempDir())

	e, err := repo.Get(domain.ExpertIDDocker)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if e.Name != "我的 Docker" {
		t.Fatalf("user edit was overwritten: %q", e.Name)
	}
}

func TestExpertServiceDismissedBuiltinStaysDismissed(t *testing.T) {
	repo := newMemExpertRepo()
	svc := NewExpertService(repo, t.TempDir())

	if err := svc.DeleteExpert(domain.ExpertIDK8sOps); err != nil {
		t.Fatalf("delete builtin: %v", err)
	}
	all, _ := svc.ListExperts()
	for _, e := range all {
		if e.ID == domain.ExpertIDK8sOps {
			t.Fatal("dismissed builtin still listed")
		}
	}

	// Restart (new service, same repo) must not resurrect it.
	NewExpertService(repo, t.TempDir())
	all, _ = svc.ListExperts()
	for _, e := range all {
		if e.ID == domain.ExpertIDK8sOps {
			t.Fatal("dismissed builtin resurrected on reseed")
		}
	}
}

func TestExpertServiceSaveCustom(t *testing.T) {
	repo := newMemExpertRepo()
	svc := NewExpertService(repo, t.TempDir())

	created, err := svc.SaveExpert(domain.Expert{
		Name: "Nginx 专家", Role: "反向代理工程师", Policy: "bogus",
		SuggestedPrompts: []string{" ", "如何排查 502"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.ID == domain.ExpertIDDocker {
		t.Fatalf("custom expert got bad id %q", created.ID)
	}
	if created.Policy != "" {
		t.Fatalf("unknown policy should normalize to empty, got %q", created.Policy)
	}
	if len(created.SuggestedPrompts) != 1 {
		t.Fatalf("blank prompts should be dropped, got %v", created.SuggestedPrompts)
	}
	if created.Builtin {
		t.Fatal("custom expert must not be builtin")
	}

	if _, err := svc.SaveExpert(domain.Expert{Name: "  "}); err == nil {
		t.Fatal("empty name should be rejected")
	}

	// Forged builtin flag is stripped on update of a custom expert.
	created.Builtin = true
	updated, err := svc.SaveExpert(created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Builtin {
		t.Fatal("builtin flag must stay authoritative from storage")
	}
}

func TestExpertServiceDeleteCustomHardDeletes(t *testing.T) {
	repo := newMemExpertRepo()
	svc := NewExpertService(repo, t.TempDir())

	created, err := svc.SaveExpert(domain.Expert{Name: "临时专家"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteExpert(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetExpert(created.ID); err == nil {
		t.Fatal("custom expert should be hard-deleted")
	}
}

// Every builtin expert's SkillRefs must resolve to an embedded builtin pack —
// the runtime silently skips missing names, so a typo here would disable the
// binding without any visible error.
func TestBuiltinExpertSkillRefsExist(t *testing.T) {
	for _, e := range builtinExperts() {
		for _, ref := range e.SkillRefs {
			if !builtinNames[ref] {
				t.Errorf("expert %s references missing builtin skill %q", e.ID, ref)
			}
		}
	}
}

// Upgraded installs: a builtin row still carrying the pre-skillhub default
// SkillRefs gets topped up to the new defaults on seed.
func TestExpertServiceUpgradesLegacySkillRefs(t *testing.T) {
	repo := newMemExpertRepo()
	repo.items[domain.ExpertIDDocker] = domain.Expert{
		ID: domain.ExpertIDDocker, Name: "Docker 专家", Builtin: true, Enabled: true,
		SkillRefs: []string{"container-restart-loop"}, // legacy default signature
	}
	repo.items[domain.ExpertIDK8sOps] = domain.Expert{
		ID: domain.ExpertIDK8sOps, Name: "K8s 运维专家", Builtin: true, Enabled: true,
		// legacy empty default
	}
	NewExpertService(repo, t.TempDir())

	docker, err := repo.Get(domain.ExpertIDDocker)
	if err != nil {
		t.Fatalf("get docker: %v", err)
	}
	if !equalStringSlices(docker.SkillRefs, []string{"docker-essentials", "container-restart-loop"}) {
		t.Fatalf("docker SkillRefs not upgraded: %v", docker.SkillRefs)
	}
	k8s, err := repo.Get(domain.ExpertIDK8sOps)
	if err != nil {
		t.Fatalf("get k8s: %v", err)
	}
	if len(k8s.SkillRefs) == 0 {
		t.Fatal("k8s ops SkillRefs not upgraded")
	}
}

// A user-customized SkillRefs list never matches the legacy signature and
// must survive seeding untouched.
func TestExpertServiceKeepsCustomSkillRefs(t *testing.T) {
	repo := newMemExpertRepo()
	repo.items[domain.ExpertIDDocker] = domain.Expert{
		ID: domain.ExpertIDDocker, Name: "我的 Docker", Builtin: true, Enabled: true,
		SkillRefs: []string{"my-own-skill"},
	}
	NewExpertService(repo, t.TempDir())

	e, err := repo.Get(domain.ExpertIDDocker)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !equalStringSlices(e.SkillRefs, []string{"my-own-skill"}) {
		t.Fatalf("custom SkillRefs were overwritten: %v", e.SkillRefs)
	}
	if e.Name != "我的 Docker" {
		t.Fatalf("user edit was overwritten: %q", e.Name)
	}
}

package application

import (
	"errors"
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
	svc := NewExpertService(repo)

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
}

func TestExpertServiceSeedNeverOverwritesEdits(t *testing.T) {
	repo := newMemExpertRepo()
	// Pre-create an edited builtin.
	repo.items[domain.ExpertIDDocker] = domain.Expert{
		ID: domain.ExpertIDDocker, Name: "我的 Docker", Builtin: true, Enabled: true,
	}
	NewExpertService(repo)

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
	svc := NewExpertService(repo)

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
	NewExpertService(repo)
	all, _ = svc.ListExperts()
	for _, e := range all {
		if e.ID == domain.ExpertIDK8sOps {
			t.Fatal("dismissed builtin resurrected on reseed")
		}
	}
}

func TestExpertServiceSaveCustom(t *testing.T) {
	repo := newMemExpertRepo()
	svc := NewExpertService(repo)

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
	svc := NewExpertService(repo)

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

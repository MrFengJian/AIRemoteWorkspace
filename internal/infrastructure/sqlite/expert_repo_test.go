package sqlite

import (
	"errors"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

func TestExpertRepoRoundTrip(t *testing.T) {
	s := newTestConversationStore(t)
	repo := NewExpertRepo(s)

	in := domain.Expert{
		ID:               "builtin-k8s-ops",
		Name:             "K8s 运维专家",
		Role:             "Kubernetes 运维工程师",
		Icon:             "Network",
		Color:            "blue",
		SortOrder:        1,
		Description:      "集群巡检与故障排查",
		SystemPrompt:     "You are a senior Kubernetes operator…",
		AllowedTools:     []string{"ssh_exec", "skill"},
		SkillRefs:        []string{"container-restart-loop"},
		SuggestedPrompts: []string{"帮我巡检集群", "Pod Pending 排查"},
		ProviderID:       "prov1",
		Model:            "gpt-test",
		Policy:           "strict",
		Temperature:      0.3,
		MaxSteps:         42,
		OpeningMessage:   "你好",
		AutoSnapshot:     false,
		Builtin:          true,
		Enabled:          true,
	}
	if err := repo.Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := repo.Get(in.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Name != in.Name || out.Role != in.Role || out.Icon != in.Icon || out.Color != in.Color {
		t.Fatalf("identity fields lost: %+v", out)
	}
	if out.SystemPrompt != in.SystemPrompt || out.Description != in.Description {
		t.Fatalf("persona fields lost")
	}
	if len(out.AllowedTools) != 2 || len(out.SkillRefs) != 1 || len(out.SuggestedPrompts) != 2 {
		t.Fatalf("json list fields lost: %+v", out)
	}
	if out.Policy != in.Policy || out.Temperature != in.Temperature || out.MaxSteps != in.MaxSteps {
		t.Fatalf("tunables lost: %+v", out)
	}
	if !out.Builtin || !out.Enabled || out.Dismissed {
		t.Fatalf("flags lost: %+v", out)
	}

	// List includes the row; ordering by SortOrder puts it before a
	// sort-order-100 custom expert.
	custom := domain.Expert{ID: "abc123", Name: "Nginx 专家", SortOrder: 100, Enabled: true}
	if err := repo.Save(custom); err != nil {
		t.Fatalf("save custom: %v", err)
	}
	all, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 || all[0].ID != in.ID || all[1].ID != custom.ID {
		t.Fatalf("list/order wrong: %+v", all)
	}

	// Get of a missing id is the sentinel error.
	if _, err := repo.Get("nope"); !errors.Is(err, ErrExpertNotFound) {
		t.Fatalf("expected ErrExpertNotFound, got %v", err)
	}

	// Delete removes the row.
	if err := repo.Delete(custom.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(custom.ID); !errors.Is(err, ErrExpertNotFound) {
		t.Fatalf("custom expert still present after delete")
	}
}

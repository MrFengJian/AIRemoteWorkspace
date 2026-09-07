package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillService(t *testing.T) {
	dir := t.TempDir()
	svc := NewSkillService(dir) // seeds builtin scenario packs + the example skill

	skills, err := svc.ListSkills()
	if err != nil || len(skills) == 0 {
		t.Fatalf("seeded skills missing: %v %v", skills, err)
	}
	found := false
	for _, s := range skills {
		if s.Name == "daily-check" {
			found = true
		}
	}
	if !found {
		t.Fatalf("daily-check not seeded: %v", skills)
	}

	// Frontmatter supplies name/description; body kept as content.
	write := func(name, content string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("deploy", "---\nname: deploy\ndescription: 部署应用\n---\n# Deploy\nstep 1\nstep 2")
	sk, err := svc.GetSkill("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != "deploy" || sk.Description != "部署应用" || !strings.Contains(sk.Content, "step 1") {
		t.Fatalf("frontmatter parse failed: %+v", sk)
	}

	// No frontmatter: name falls back to the directory, description to the
	// first non-heading body line.
	write("plain", "# Plain skill\nDo things.")
	sk2, err := svc.GetSkill("plain")
	if err != nil {
		t.Fatal(err)
	}
	if sk2.Name != "plain" || !strings.HasPrefix(sk2.Description, "Do things") {
		t.Fatalf("fallback parse failed: %+v", sk2)
	}

	// Path traversal in the skill name is rejected.
	if _, err := svc.GetSkill("../escape"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	if _, err := svc.GetSkill("."); err == nil {
		t.Fatal("expected dot name to be rejected")
	}
}

// Builtin scenario packs are seeded on first launch, never overwrite a user
// copy, and stay deleted once removed (dismissed) — until re-created.
func TestBuiltinScenarioSeeding(t *testing.T) {
	dir := t.TempDir()

	// A pre-existing user-edited copy of a builtin must survive seeding.
	userCopy := filepath.Join(dir, "cpu-high")
	if err := os.MkdirAll(userCopy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userCopy, "SKILL.md"), []byte("---\nname: cpu-high\ndescription: 用户自改版\n---\n# custom"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSkillService(dir)
	skills, err := svc.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	builtin := map[string]bool{}
	edited := ""
	for _, s := range skills {
		if s.Builtin {
			builtin[s.Name] = true
		}
		if s.Name == "cpu-high" {
			edited = s.Description
		}
	}
	if len(builtin) < 6 {
		t.Fatalf("expected >=6 builtin scenario packs, got %v", builtin)
	}
	for _, want := range []string{"cpu-high", "disk-full", "memory-oom", "service-down", "port-unreachable", "container-restart-loop"} {
		if !builtin[want] {
			t.Fatalf("builtin %q missing (note: a user-edited copy must still be flagged builtin)", want)
		}
	}
	if edited != "用户自改版" {
		t.Fatalf("seeding overwrote the user-edited pack: %q", edited)
	}

	// Delete a builtin → dismissed list keeps it dead across restarts.
	if err := svc.DeleteSkill("disk-full"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetSkill("disk-full"); err == nil {
		t.Fatal("deleted builtin still readable")
	}
	svc2 := NewSkillService(dir)
	if _, err := svc2.GetSkill("disk-full"); err == nil {
		t.Fatal("dismissed builtin was resurrected on restart")
	}
	// The others survive.
	if _, err := svc2.GetSkill("cpu-high"); err != nil {
		t.Fatalf("unrelated builtin lost: %v", err)
	}

	// Re-creating a dismissed builtin clears the dismissal.
	if err := svc2.SaveSkill("disk-full", "---\nname: disk-full\ndescription: 重建\n---\nbody"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.GetSkill("disk-full"); err != nil {
		t.Fatalf("re-created skill unreadable: %v", err)
	}
}

func TestSaveDeleteSkill(t *testing.T) {
	svc := NewSkillService(t.TempDir())

	if err := svc.SaveSkill("my-scenario", "---\nname: my-scenario\ndescription: x\n---\nbody"); err != nil {
		t.Fatal(err)
	}
	sk, err := svc.GetSkill("my-scenario")
	if err != nil || sk.Description != "x" || sk.Builtin {
		t.Fatalf("save/get failed: %+v %v", sk, err)
	}

	// Overwrite.
	if err := svc.SaveSkill("my-scenario", "no frontmatter now"); err != nil {
		t.Fatal(err)
	}
	if sk, _ = svc.GetSkill("my-scenario"); !strings.HasPrefix(sk.Description, "no frontmatter") {
		t.Fatalf("overwrite failed: %+v", sk)
	}

	// Invalid names rejected on write and delete.
	for _, bad := range []string{"../evil", ".hidden", "", "with space"} {
		if err := svc.SaveSkill(bad, "x"); err == nil {
			t.Fatalf("SaveSkill accepted %q", bad)
		}
		if err := svc.DeleteSkill(bad); err == nil {
			t.Fatalf("DeleteSkill accepted %q", bad)
		}
	}

	// Deleting a user skill works and does not touch the dismissed list.
	if err := svc.DeleteSkill("my-scenario"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetSkill("my-scenario"); err == nil {
		t.Fatal("deleted skill still readable")
	}
}

func TestExtractSkillFrontmatter(t *testing.T) {
	name, desc := ExtractSkillFrontmatter("---\nname: redis-down\ndescription: Redis 连不上\n---\n# body")
	if name != "redis-down" || desc != "Redis 连不上" {
		t.Fatalf("frontmatter extraction failed: %q %q", name, desc)
	}
	name, desc = ExtractSkillFrontmatter("no frontmatter at all")
	if name != "" || desc != "" {
		t.Fatalf("expected empty extraction, got %q %q", name, desc)
	}
}

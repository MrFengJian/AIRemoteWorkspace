package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillService(t *testing.T) {
	dir := t.TempDir()
	svc := NewSkillService(dir) // seeds the example skill on first launch

	skills, err := svc.ListSkills()
	if err != nil || len(skills) == 0 {
		t.Fatalf("seeded skill missing: %v %v", skills, err)
	}
	if skills[0].Name != "daily-check" {
		t.Fatalf("unexpected seeded skill %q", skills[0].Name)
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

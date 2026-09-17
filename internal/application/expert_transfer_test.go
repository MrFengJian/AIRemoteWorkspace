package application

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// buildTransferSvc wires ExpertTransferService over temp-dir-backed expert
// and skill services (the real seeding populates the embedded builtin packs).
func buildTransferSvc(t *testing.T) (*ExpertTransferService, string) {
	t.Helper()
	skills := NewSkillService(t.TempDir())
	repo := newMemExpertRepo()
	experts := NewExpertService(repo)
	return NewExpertTransferService(experts, skills), skills.dir
}

// TestExportImportRoundTrip exports a custom expert with a directory-form
// skill pack and re-imports it elsewhere: the expert comes back as a new
// custom row and the pack (SKILL.md + bundled file) is fully restored.
func TestExportImportRoundTrip(t *testing.T) {
	src, _ := buildTransferSvc(t)

	// A source app state: a custom expert bound to a directory pack.
	srcSkills := src.skills
	if err := srcSkills.ImportSkillFiles("deploy-pack", map[string][]byte{
		"SKILL.md":            []byte("---\nname: deploy-pack\ndescription: deploy steps\n---\n# Deploy\nrun scripts/roll.sh"),
		"scripts/roll.sh":     []byte("#!/bin/sh\necho rolling"),
		"references/notes.md": []byte("# notes\ncheck health endpoint"),
	}); err != nil {
		t.Fatalf("seed pack: %v", err)
	}
	if _, err := src.experts.SaveExpert(domain.Expert{
		Name: "发布专家", Role: "Deploy", SkillRefs: []string{"deploy-pack"},
		Temperature: 0.3, Enabled: true,
	}); err != nil {
		t.Fatalf("save expert: %v", err)
	}
	created, err := src.experts.ListExperts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var expID string
	for _, e := range created {
		if e.Name == "发布专家" {
			expID = e.ID
		}
	}
	if expID == "" {
		t.Fatal("custom expert not found")
	}

	zipPath := filepath.Join(t.TempDir(), "发布专家.expert.zip")
	zipPath, included, err := src.ExportPackage(expID, zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.HasSuffix(zipPath, ".zip") {
		t.Fatalf("export path should keep .zip: %q", zipPath)
	}
	if len(included) != 1 || included[0] != "deploy-pack" {
		t.Fatalf("unexpected included packs: %v", included)
	}

	// A different install imports the package.
	dst, _ := buildTransferSvc(t)
	imported, err := dst.ImportPackage(zipPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.ID == "" || imported.ID == expID {
		t.Fatalf("imported expert must get a fresh id, got %q", imported.ID)
	}
	if imported.Builtin {
		t.Fatal("imported expert must be custom, never builtin")
	}
	if !imported.Enabled {
		t.Fatal("imported expert must be enabled")
	}
	if len(imported.SkillRefs) != 1 || imported.SkillRefs[0] != "deploy-pack" {
		t.Fatalf("skill refs lost: %v", imported.SkillRefs)
	}
	sk, err := dst.skills.GetSkill("deploy-pack")
	if err != nil {
		t.Fatalf("pack not installed: %v", err)
	}
	if !strings.Contains(sk.Content, "run scripts/roll.sh") {
		t.Fatalf("SKILL.md body wrong: %q", sk.Content)
	}
	if len(sk.Files) != 2 || sk.Files[0] != "references/notes.md" || sk.Files[1] != "scripts/roll.sh" {
		t.Fatalf("bundled files not restored: %v", sk.Files)
	}
	if raw, err := dst.skills.ReadSkillFile("deploy-pack", "scripts/roll.sh"); err != nil || raw != "#!/bin/sh\necho rolling" {
		t.Fatalf("bundled script wrong: %q err=%v", raw, err)
	}
}

// TestImportRejectsZipSlip crafts a package whose entry escapes the skills
// root; import must fail without writing anything outside.
func TestImportRejectsZipSlip(t *testing.T) {
	svc, skillsDir := buildTransferSvc(t)
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	entry, err := w.Create("../evil.md")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("evil"))
	w.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportPackage(zipPath); err == nil {
		t.Fatal("zip-slip entry must be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(skillsDir), "evil.md")); err == nil {
		t.Fatal("evil.md escaped the skills root")
	}
}

// TestImportRejectsMissingEnvelope: a zip without expert.json is not a package.
func TestImportRejectsMissingEnvelope(t *testing.T) {
	svc, _ := buildTransferSvc(t)
	zipPath := filepath.Join(t.TempDir(), "noexp.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	entry, _ := w.Create("skills/x/SKILL.md")
	entry.Write([]byte("---\nname: x\ndescription: x\n---\nbody"))
	w.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)
	if _, err := svc.ImportPackage(zipPath); err == nil || !strings.Contains(err.Error(), "expert.json") {
		t.Fatalf("expected missing-expert.json error, got %v", err)
	}
}

// TestImportReplacesSameNamePack verifies the explicit-replace semantics.
func TestImportReplacesSameNamePack(t *testing.T) {
	svc, _ := buildTransferSvc(t)
	if err := svc.skills.ImportSkillFiles("pack-a", map[string][]byte{
		"SKILL.md": []byte("---\nname: pack-a\ndescription: old\n---\nold body"),
		"old.txt":  []byte("stale"),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	zipPath := filepath.Join(t.TempDir(), "p.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for _, e := range []struct{ name, body string }{
		{"expert.json", ""}, // filled below
		{"skills/pack-a/SKILL.md", "---\nname: pack-a\ndescription: new\n---\nnew body"},
		{"skills/pack-a/new.txt", "fresh"},
	} {
		var raw []byte
		if e.name == "expert.json" {
			raw, _ = json.Marshal(expertEnvelope{Version: expertPackageVersion, Expert: domain.Expert{
				Name: "导入专家", Enabled: true, SkillRefs: []string{"pack-a"},
			}})
		} else {
			raw = []byte(e.body)
		}
		entry, err := w.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	imported, err := svc.ImportPackage(zipPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Name != "导入专家" {
		t.Fatalf("expert name wrong: %q", imported.Name)
	}
	sk, err := svc.skills.GetSkill("pack-a")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !strings.Contains(sk.Content, "new body") || len(sk.Files) != 1 || sk.Files[0] != "new.txt" {
		t.Fatalf("pack not replaced: %q %v", sk.Content, sk.Files)
	}
}

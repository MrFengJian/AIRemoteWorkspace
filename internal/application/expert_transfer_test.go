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
// and skill services (the real seeding populates the embedded builtin packs
// and expert directories), linking the experts root so skill scoping works.
func buildTransferSvc(t *testing.T) (*ExpertTransferService, *SkillService) {
	t.Helper()
	skills := NewSkillService(t.TempDir())
	experts := NewExpertService(newMemExpertRepo(), t.TempDir())
	skills.SetExpertsRoot(experts.dir)
	return NewExpertTransferService(experts, skills), skills
}

// TestExportImportRoundTrip exports a custom expert with a private skill
// pack and re-imports it elsewhere: the expert comes back as a new custom
// row, the pack lands in the expert's PRIVATE skills/ directory (not the
// public list), and SOUL.md/manifest.json are restored.
func TestExportImportRoundTrip(t *testing.T) {
	src, _ := buildTransferSvc(t)

	// A source app state: a custom expert bound to a private directory pack.
	privateRoot := filepath.Join(src.experts.ExpertDir("custom-1"), expertPrivateSkills)
	if err := importSkillFilesAt(privateRoot, "deploy-pack", map[string][]byte{
		"SKILL.md":            []byte("---\nname: deploy-pack\ndescription: deploy steps\n---\n# Deploy\nrun scripts/roll.sh"),
		"scripts/roll.sh":     []byte("#!/bin/sh\necho rolling"),
		"references/notes.md": []byte("# notes\ncheck health endpoint"),
	}); err != nil {
		t.Fatalf("seed pack: %v", err)
	}
	expert, err := src.experts.SaveExpert(domain.Expert{
		ID: "custom-1", Name: "发布专家", Role: "Deploy", SkillRefs: []string{"deploy-pack"},
		Temperature: 0.3, Enabled: true,
		SystemPrompt: "soul text", Heartbeat: "beat text",
	})
	if err != nil {
		t.Fatalf("save expert: %v", err)
	}

	zipPath := filepath.Join(t.TempDir(), "发布专家.expert.zip")
	zipPath, included, err := src.ExportPackage(expert.ID, zipPath)
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
	dst, dstSkills := buildTransferSvc(t)
	imported, err := dst.ImportPackage(zipPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.ID == "" || imported.ID == expert.ID {
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
	if imported.SystemPrompt != "soul text" || imported.Heartbeat != "beat text" {
		t.Fatalf("persona files not restored: %q / %q", imported.SystemPrompt, imported.Heartbeat)
	}

	// The pack is installed as the expert's PRIVATE skill: visible (and
	// shadowing) through the expert-scoped store, absent from the public list.
	store, ok := dstSkills.SkillSourceFor(imported.ID)
	if !ok {
		t.Fatal("expert-scoped skill store not available after import")
	}
	sk, err := store.GetSkill("deploy-pack")
	if err != nil {
		t.Fatalf("private pack not installed: %v", err)
	}
	if !strings.Contains(sk.Content, "run scripts/roll.sh") {
		t.Fatalf("SKILL.md body wrong: %q", sk.Content)
	}
	raw, err := store.ReadSkillFile("deploy-pack", "scripts/roll.sh")
	if err != nil || raw != "#!/bin/sh\necho rolling" {
		t.Fatalf("bundled script wrong: %q err=%v", raw, err)
	}
	if _, err := dstSkills.GetSkill("deploy-pack"); err == nil {
		t.Fatal("private pack must not leak into the public skills list")
	}
	if files, err := dstSkills.SkillFileList("deploy-pack"); err == nil && len(files) > 0 {
		t.Fatalf("public skills root polluted: %v", files)
	}
}

// TestScopedSkillsShadow: an expert-private pack with the same name as a
// public pack wins for that expert's sessions; other experts and the public
// list keep seeing the public pack.
func TestScopedSkillsShadow(t *testing.T) {
	svc, skills := buildTransferSvc(t)
	if err := skills.ImportSkillFiles("shared-pack", map[string][]byte{
		"SKILL.md": []byte("---\nname: shared-pack\ndescription: public version\n---\nPUBLIC BODY"),
	}); err != nil {
		t.Fatalf("seed public pack: %v", err)
	}
	privateRoot := filepath.Join(svc.experts.ExpertDir("expert-x"), expertPrivateSkills)
	if err := importSkillFilesAt(privateRoot, "shared-pack", map[string][]byte{
		"SKILL.md": []byte("---\nname: shared-pack\ndescription: private version\n---\nPRIVATE BODY"),
	}); err != nil {
		t.Fatalf("seed private pack: %v", err)
	}

	store, ok := skills.SkillSourceFor("expert-x")
	if !ok {
		t.Fatal("scoped store missing")
	}
	sk, err := store.GetSkill("shared-pack")
	if err != nil || !strings.Contains(sk.Content, "PRIVATE BODY") {
		t.Fatalf("private pack does not shadow public one: %v err=%v", sk.Content, err)
	}
	// Global view unchanged.
	pub, err := skills.GetSkill("shared-pack")
	if err != nil || !strings.Contains(pub.Content, "PUBLIC BODY") {
		t.Fatalf("public pack affected by shadowing: %v err=%v", pub.Content, err)
	}
	// Scoped listing: private pack present; public pack listed once (the
	// private one) and the shadowed global entry suppressed.
	list, err := store.ListSkills()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, s := range list {
		if s.Name == "shared-pack" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("shadowed pack listed %d times in scoped view", count)
	}
	// An expert without private packs keeps the global store.
	if _, ok := skills.SkillSourceFor("expert-none"); ok {
		t.Fatal("scoped store must not appear without a private skills dir")
	}
}

// TestImportRejectsZipSlip crafts a package whose entry escapes the experts
// root; import must fail without writing anything outside.
func TestImportRejectsZipSlip(t *testing.T) {
	svc, _ := buildTransferSvc(t)
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
	if _, err := os.Stat(filepath.Join(filepath.Dir(svc.experts.dir), "evil.md")); err == nil {
		t.Fatal("evil.md escaped the experts root")
	}
}

// TestImportRejectsMissingManifest: a zip without manifest.json is not a
// package (there is no legacy format).
func TestImportRejectsMissingManifest(t *testing.T) {
	svc, _ := buildTransferSvc(t)
	zipPath := filepath.Join(t.TempDir(), "noexp.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	entry, _ := w.Create("skills/x/SKILL.md")
	entry.Write([]byte("---\nname: x\ndescription: x\n---\nbody"))
	w.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)
	if _, err := svc.ImportPackage(zipPath); err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("expected missing-manifest error, got %v", err)
	}
}

// TestImportReplacesSameNamePack verifies that a packaged pack replaces an
// existing private pack of the same name on re-import.
func TestImportReplacesSameNamePack(t *testing.T) {
	svc, _ := buildTransferSvc(t)
	zipPath := filepath.Join(t.TempDir(), "p.zip")
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	manifest, _ := json.MarshalIndent(ExpertManifest{
		Format: expertPackageVersion, ID: "whatever", Label: "导入专家",
		SkillRefs: []string{"pack-a"},
	}, "", "  ")
	for _, e := range []zipEntry{
		{expertManifestFile, append(manifest, '\n')},
		{expertSoulFile, []byte("soul")},
		{expertPrivateSkills + "/pack-a/SKILL.md", []byte("---\nname: pack-a\ndescription: new\n---\nnew body")},
		{expertPrivateSkills + "/pack-a/new.txt", []byte("fresh")},
	} {
		entry, err := w.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(e.raw); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	// First import.
	first, err := svc.ImportPackage(zipPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	privateRoot := filepath.Join(svc.experts.ExpertDir(first.ID), expertPrivateSkills)
	if err := importSkillFilesAt(privateRoot, "pack-a", map[string][]byte{
		"SKILL.md": []byte("---\nname: pack-a\ndescription: stale\n---\nstale body"),
		"old.txt":  []byte("stale"),
	}); err != nil {
		t.Fatalf("simulate stale pack: %v", err)
	}
	// Second import of the same package replaces the stale pack.
	second, err := svc.ImportPackage(zipPath)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("re-import must create a new expert id")
	}
	store, ok := svc.skills.SkillSourceFor(second.ID)
	if !ok {
		t.Fatal("scoped store missing after re-import")
	}
	sk, err := store.GetSkill("pack-a")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !strings.Contains(sk.Content, "new body") || len(sk.Files) != 1 || sk.Files[0] != "new.txt" {
		t.Fatalf("pack not replaced: %q %v", sk.Content, sk.Files)
	}
}

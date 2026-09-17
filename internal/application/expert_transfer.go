package application

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// ExpertTransferService moves expert configurations in and out of the app as
// self-contained zip packages following the expert directory layout:
//
//	manifest.json   identity card (+ bindings)
//	SOUL.md         persona core (IDENTITY.md accepted on import)
//	HEARTBEAT.md    operational guidelines (optional)
//	skills/<pack>/  bound skill packs, bundled files included
//
// Import restores the expert as a NEW custom row (builtin flags never
// travel) and installs the packaged skill packs as the expert's PRIVATE
// skills — they shadow same-name public packs and never appear in the
// public list.
type ExpertTransferService struct {
	experts *ExpertService
	skills  *SkillService
}

// NewExpertTransferService wires the transfer service over the expert and
// skill services.
func NewExpertTransferService(experts *ExpertService, skills *SkillService) *ExpertTransferService {
	return &ExpertTransferService{experts: experts, skills: skills}
}

// expertPackageVersion is the package format version (the manifest's format
// field). Bump when the layout changes and reject other versions.
const expertPackageVersion = expertManifestFormat

// Zip guards: a package is a persona plus a few text/markdown packs —
// anything beyond these caps is abuse, not content.
const (
	zipMaxBytes     = 64 << 20 // archive size on disk
	zipEntryMax     = 500      // entry count
	zipFileMaxBytes = 8 << 20  // per entry (uncompressed)
	zipTotalMax     = 64 << 20 // total uncompressed
)

// zipEntry is one archive file pending write.
type zipEntry struct {
	name string
	raw  []byte
}

// ExportPackage writes the expert directory (manifest + SOUL + HEARTBEAT +
// bound skill packs) to zipPath (".zip" appended when missing). Returns the
// actual archive path and the names of the skill packs included — refs whose
// packs exist neither privately nor publicly are skipped silently.
func (s *ExpertTransferService) ExportPackage(id, zipPath string) (string, []string, error) {
	if s.skills == nil {
		return "", nil, fmt.Errorf("skills not available")
	}
	e, err := s.experts.GetExpert(id)
	if err != nil {
		return "", nil, err
	}
	zipPath = strings.TrimSpace(zipPath)
	if zipPath == "" {
		return "", nil, fmt.Errorf("empty export path")
	}
	if !strings.HasSuffix(strings.ToLower(zipPath), ".zip") {
		zipPath += ".zip"
	}

	included := make([]string, 0, len(e.SkillRefs))
	packs := map[string]map[string][]byte{} // name -> rel path (slash) -> bytes
	privateRoot := filepath.Join(s.experts.ExpertDir(e.ID), expertPrivateSkills)
	for _, ref := range e.SkillRefs {
		if files, ok := exportPackFromRoot(privateRoot, ref); ok {
			packs[ref] = files
			included = append(included, ref)
			continue
		}
		if files, ok := exportPublicPack(s.skills, ref); ok {
			packs[ref] = files
			included = append(included, ref)
		}
	}
	sort.Strings(included)

	manifest, err := json.MarshalIndent(expertToManifest(e), "", "  ")
	if err != nil {
		return "", nil, err
	}
	entries := []zipEntry{
		{expertManifestFile, append(manifest, '\n')},
		{expertSoulFile, []byte(e.SystemPrompt)},
	}
	if strings.TrimSpace(e.Heartbeat) != "" {
		entries = append(entries, zipEntry{expertHeartbeatFile, []byte(e.Heartbeat)})
	}
	for name, files := range packs {
		for rel, raw := range files {
			entries = append(entries, zipEntry{expertPrivateSkills + "/" + name + "/" + rel, raw})
		}
	}

	f, err := os.Create(zipPath)
	if err != nil {
		return "", nil, err
	}
	w := zip.NewWriter(f)
	for _, entry := range entries {
		if err := writeZipEntry(w, entry.name, entry.raw); err != nil {
			_ = w.Close()
			_ = f.Close()
			return "", nil, err
		}
	}
	if err := w.Close(); err != nil {
		_ = f.Close()
		return "", nil, err
	}
	return zipPath, included, f.Close()
}

// exportPackFromRoot collects one pack (raw SKILL.md + bundled files) from an
// arbitrary skills root; ok is false when the pack is absent or unreadable.
func exportPackFromRoot(root, ref string) (map[string][]byte, bool) {
	files := map[string][]byte{}
	skillMD, err := fileFromRoot(root, ref, "SKILL.md")
	if err != nil {
		return nil, false
	}
	files["SKILL.md"] = skillMD
	for _, f := range listSkillFiles(filepath.Join(root, ref)) {
		raw, err := fileFromRoot(root, ref, f)
		if err != nil {
			return nil, false // half-readable pack — skip rather than ship broken
		}
		files[f] = raw
	}
	return files, true
}

// exportPublicPack collects one pack from the global skills root.
func exportPublicPack(skills *SkillService, ref string) (map[string][]byte, bool) {
	files := map[string][]byte{}
	skillMD, err := skills.SkillFileBytes(ref, "SKILL.md")
	if err != nil {
		return nil, false
	}
	files["SKILL.md"] = skillMD
	if bundled, err := skills.SkillFileList(ref); err == nil {
		for _, f := range bundled {
			raw, err := skills.SkillFileBytes(ref, f)
			if err != nil {
				return nil, false
			}
			files[f] = raw
		}
	}
	return files, true
}

// writeZipEntry adds one regular file to the archive (explicit 0644 mode, so
// extractors never see odd permission bits).
func writeZipEntry(w *zip.Writer, name string, raw []byte) error {
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()}
	hdr.SetMode(0o644)
	entry, err := w.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = entry.Write(raw)
	return err
}

// importedPackage is the normalized view of a v1 or v2 package.
type importedPackage struct {
	manifest  ExpertManifest
	soul      string
	heartbeat string
	packs     map[string]map[string][]byte
}

// ImportPackage restores an expert package: the expert becomes a NEW custom
// row (fresh id, builtin flag stripped) and every packaged skill pack is
// installed into the expert's PRIVATE skills/ directory (shadowing same-name
// public packs, never polluting the public list). Path traversal, oversized
// and non-regular entries are rejected before anything is written.
func (s *ExpertTransferService) ImportPackage(zipPath string) (domain.Expert, error) {
	info, err := os.Stat(zipPath)
	if err != nil {
		return domain.Expert{}, err
	}
	if info.Size() > zipMaxBytes {
		return domain.Expert{}, fmt.Errorf("package exceeds %d bytes", zipMaxBytes)
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return domain.Expert{}, fmt.Errorf("open package: %w", err)
	}
	defer r.Close()
	if len(r.File) > zipEntryMax {
		return domain.Expert{}, fmt.Errorf("package has too many entries (%d)", len(r.File))
	}

	pkg, err := readPackage(r)
	if err != nil {
		return domain.Expert{}, err
	}

	// Fresh custom identity: imports never overwrite an existing expert and
	// the builtin flag never travels.
	e := pkg.manifest.expert()
	e.ID = newID()
	e.Builtin = false
	e.Dismissed = false
	e.Enabled = true
	e.SystemPrompt = pkg.soul
	e.Heartbeat = pkg.heartbeat

	// Install private packs before the row exists so its SkillRefs resolve
	// immediately; SaveExpert then mirrors manifest/SOUL/HEARTBEAT into the
	// same directory.
	privateRoot := filepath.Join(s.experts.ExpertDir(e.ID), expertPrivateSkills)
	for name, files := range pkg.packs {
		if err := importSkillFilesAt(privateRoot, name, files); err != nil {
			return domain.Expert{}, err
		}
	}
	if _, err := s.experts.SaveExpert(e); err != nil {
		return domain.Expert{}, fmt.Errorf("save imported expert: %w", err)
	}
	return e, nil
}

// readPackage scans the archive once, normalizing the manifest.json layout
// (plus SOUL.md / HEARTBEAT.md / skills/) into an importedPackage. Packages
// without a manifest.json are rejected — there is no legacy format.
func readPackage(r *zip.ReadCloser) (*importedPackage, error) {
	var (
		manifestRaw []byte
		pkg         = &importedPackage{packs: map[string]map[string][]byte{}}
		total       int
	)
	for _, zf := range r.File {
		if zf.Mode().IsDir() || strings.HasSuffix(zf.Name, "/") {
			continue // real directory entries and trailing-slash markers
		}
		if !zf.Mode().IsRegular() {
			return nil, fmt.Errorf("entry %q: not a regular file", zf.Name)
		}
		name, err := sanitizeZipName(zf.Name)
		if err != nil {
			return nil, err
		}
		raw, err := readBoundedEntry(zf)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > zipTotalMax {
			return nil, fmt.Errorf("package exceeds %d bytes uncompressed", zipTotalMax)
		}
		switch {
		case name == expertManifestFile:
			manifestRaw = raw
		case name == expertSoulFile || name == expertSoulAltFile:
			pkg.soul = string(raw)
		case name == expertHeartbeatFile:
			pkg.heartbeat = string(raw)
		case strings.HasPrefix(name, expertPrivateSkills+"/"):
			rest := strings.TrimPrefix(name, expertPrivateSkills+"/")
			skill, file, ok := strings.Cut(rest, "/")
			if !ok || file == "" {
				return nil, fmt.Errorf("entry %q: not a skill file", zf.Name)
			}
			if !skillNameRe.MatchString(skill) {
				return nil, fmt.Errorf("entry %q: invalid skill name", zf.Name)
			}
			if pkg.packs[skill] == nil {
				pkg.packs[skill] = map[string][]byte{}
			}
			pkg.packs[skill][file] = raw
		}
	}
	if manifestRaw == nil {
		return nil, fmt.Errorf("package has no %s", expertManifestFile)
	}
	if err := json.Unmarshal(manifestRaw, &pkg.manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", expertManifestFile, err)
	}
	if pkg.manifest.Format != 0 && pkg.manifest.Format != expertPackageVersion {
		return nil, fmt.Errorf("unsupported package format %d", pkg.manifest.Format)
	}
	if pkg.manifest.Label == "" {
		pkg.manifest.Label = pkg.manifest.ID
	}
	if pkg.soul == "" {
		pkg.soul = pkg.manifest.expert().SystemPrompt
	}
	return pkg, nil
}

// readBoundedEntry reads one archive entry with the per-file size cap.
func readBoundedEntry(zf *zip.File) ([]byte, error) {
	if zf.UncompressedSize64 > zipFileMaxBytes {
		return nil, fmt.Errorf("entry %q exceeds %d bytes", zf.Name, zipFileMaxBytes)
	}
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, zipFileMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > zipFileMaxBytes {
		return nil, fmt.Errorf("entry %q exceeds %d bytes", zf.Name, zipFileMaxBytes)
	}
	return raw, nil
}

// sanitizeZipName validates a zip entry name against zip-slip: no absolute
// paths, no dot-dot segments, no backslashes or drive letters. Returns the
// cleaned forward-slash name.
func sanitizeZipName(name string) (string, error) {
	if strings.ContainsRune(name, '\\') || strings.ContainsRune(name, ':') {
		return "", fmt.Errorf("entry %q: invalid path", name)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("entry %q: absolute path", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("entry %q: invalid path", name)
		}
	}
	return name, nil
}

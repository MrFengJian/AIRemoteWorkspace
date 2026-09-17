package application

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// ExpertTransferService moves expert configurations in and out of the app as
// self-contained zip packages: the expert profile plus every bound skill pack
// (directory-form, bundled files included). Export writes
// `<name>.expert.zip`; import restores the expert as a NEW custom row
// (builtin flags never travel) and installs the packaged skill packs.
type ExpertTransferService struct {
	experts *ExpertService
	skills  *SkillService
}

// NewExpertTransferService wires the transfer service over the expert and
// skill services (the skills root is where packaged packs are installed).
func NewExpertTransferService(experts *ExpertService, skills *SkillService) *ExpertTransferService {
	return &ExpertTransferService{experts: experts, skills: skills}
}

// expertPackageVersion is the envelope format version of expert.json inside
// a package. Bump when the layout changes and reject older/newer on import.
const expertPackageVersion = 1

// Zip guards: a package is a persona plus a few text/markdown packs —
// anything beyond these caps is abuse, not content.
const (
	zipMaxBytes     = 64 << 20 // archive size on disk
	zipEntryMax     = 500      // entry count
	zipFileMaxBytes = 8 << 20  // per entry (uncompressed)
	zipTotalMax     = 64 << 20 // total uncompressed
)

// expertEnvelope is the expert.json payload.
type expertEnvelope struct {
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exportedAt"`
	Expert     domain.Expert `json:"expert"`
}

// ExportPackage writes the expert + its bound skill packs to zipPath (".zip"
// appended when missing). Returns the actual archive path and the names of
// the skill packs included — refs whose packs don't exist on disk are
// skipped silently (the runtime already degrades to "skill not found").
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
	for _, ref := range e.SkillRefs {
		// Raw SKILL.md bytes (frontmatter included) — sk.Content alone would
		// lose the name/description header.
		skillMD, err := s.skills.SkillFileBytes(ref, "SKILL.md")
		if err != nil {
			continue // missing pack — the import side tolerates absent skills
		}
		files := map[string][]byte{"SKILL.md": skillMD}
		full := true
		if bundled, listErr := s.skills.SkillFileList(ref); listErr == nil {
			for _, f := range bundled {
				raw, err := s.skills.SkillFileBytes(ref, f)
				if err != nil {
					full = false
					break
				}
				files[f] = raw
			}
		}
		if !full {
			continue // unreadable bundled file — skip the pack rather than ship it broken
		}
		packs[ref] = files
		included = append(included, ref)
	}
	sort.Strings(included)

	env, err := json.Marshal(expertEnvelope{
		Version:    expertPackageVersion,
		ExportedAt: time.Now(),
		Expert:     e,
	})
	if err != nil {
		return "", nil, err
	}

	f, err := os.Create(zipPath)
	if err != nil {
		return "", nil, err
	}
	w := zip.NewWriter(f)
	if err := writeZipEntry(w, "expert.json", env); err != nil {
		_ = w.Close()
		_ = f.Close()
		return "", nil, err
	}
	for name, files := range packs {
		for rel, raw := range files {
			if err := writeZipEntry(w, "skills/"+name+"/"+rel, raw); err != nil {
				_ = w.Close()
				_ = f.Close()
				return "", nil, err
			}
		}
	}
	if err := w.Close(); err != nil {
		_ = f.Close()
		return "", nil, err
	}
	return zipPath, included, f.Close()
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

// ImportPackage restores an expert package: the expert becomes a NEW custom
// row (fresh id, builtin flag stripped) and every `skills/...` entry is
// installed into the skills root, replacing same-name packs — import is an
// explicit user action. Path traversal, oversized and non-regular entries are
// rejected before anything is written.
func (s *ExpertTransferService) ImportPackage(zipPath string) (domain.Expert, error) {
	if s.skills == nil {
		return domain.Expert{}, fmt.Errorf("skills not available")
	}
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

	var envRaw []byte
	packs := map[string]map[string][]byte{}
	total := 0
	for _, zf := range r.File {
		if zf.Mode().IsDir() || strings.HasSuffix(zf.Name, "/") {
			continue // real directory entries and trailing-slash markers
		}
		if !zf.Mode().IsRegular() {
			return domain.Expert{}, fmt.Errorf("entry %q: not a regular file", zf.Name)
		}
		name, err := sanitizeZipName(zf.Name)
		if err != nil {
			return domain.Expert{}, err
		}
		if sizeErr := func() error {
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			if int(zf.UncompressedSize64) > zipFileMaxBytes {
				return fmt.Errorf("entry %q exceeds %d bytes", zf.Name, zipFileMaxBytes)
			}
			raw, err := io.ReadAll(io.LimitReader(rc, zipFileMaxBytes+1))
			if err != nil {
				return err
			}
			if len(raw) > zipFileMaxBytes {
				return fmt.Errorf("entry %q exceeds %d bytes", zf.Name, zipFileMaxBytes)
			}
			total += len(raw)
			if total > zipTotalMax {
				return fmt.Errorf("package exceeds %d bytes uncompressed", zipTotalMax)
			}
			switch {
			case name == "expert.json":
				envRaw = raw
			case strings.HasPrefix(name, "skills/"):
				rest := strings.TrimPrefix(name, "skills/")
				skill, file, ok := strings.Cut(rest, "/")
				if !ok || file == "" {
					return fmt.Errorf("entry %q: not a skill file", zf.Name)
				}
				if !skillNameRe.MatchString(skill) {
					return fmt.Errorf("entry %q: invalid skill name", zf.Name)
				}
				if packs[skill] == nil {
					packs[skill] = map[string][]byte{}
				}
				packs[skill][file] = raw
			}
			return nil
		}(); sizeErr != nil {
			return domain.Expert{}, sizeErr
		}
	}
	if envRaw == nil {
		return domain.Expert{}, fmt.Errorf("package has no expert.json")
	}
	var env expertEnvelope
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return domain.Expert{}, fmt.Errorf("expert.json: %w", err)
	}
	if env.Version != expertPackageVersion {
		return domain.Expert{}, fmt.Errorf("unsupported package version %d", env.Version)
	}

	// Install packs first, so the expert's SkillRefs resolve immediately
	// after the row is created.
	for name, files := range packs {
		if err := s.skills.ImportSkillFiles(name, files); err != nil {
			return domain.Expert{}, err
		}
	}

	e := env.Expert
	e.ID = ""         // fresh custom row — never overwrite an existing expert
	e.Builtin = false // the flag never travels; imports are always custom
	e.Dismissed = false
	if !e.Enabled { // exported disabled experts import re-enabled
		e.Enabled = true
	}
	created, err := s.experts.SaveExpert(e)
	if err != nil {
		return domain.Expert{}, fmt.Errorf("save imported expert: %w", err)
	}
	return created, nil
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

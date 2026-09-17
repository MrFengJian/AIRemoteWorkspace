package application

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// builtinSkills holds the embedded scenario packs (a directory per skill:
// SKILL.md plus optional bundled scripts/references). They ship inside the
// binary and are seeded into the skills root on startup — user-edited files
// are never overwritten, and skills the user deliberately deleted stay
// deleted (dismissed list, see seedBuiltins).
//
//go:embed all:skills
var builtinSkills embed.FS

// dismissedFile lists builtin skill names the user removed via the scenario
// manager, so a restart does not resurrect them. Lives inside the skills root
// and starts with a dot, so it never matches the skill-name pattern.
const dismissedFile = ".dismissed-builtins"

// skillFileMaxBytes bounds a single bundled skill file read (the agent-facing
// ReadSkillFile): scripts and reference docs are small; anything larger is
// data, not instructions.
const skillFileMaxBytes = 2 << 20 // 2 MiB

// SkillService loads agent skills from the skills directory, following the
// eino adk/middlewares/skill convention: one subdirectory per skill, each
// containing a SKILL.md with optional YAML frontmatter (name/description)
// and a markdown body with the instructions. It backs both the `/skill`
// invocation in the agent input and the model-facing `skill` tool, plus the
// scenario-manager UI (list / read / write / delete).
type SkillService struct {
	dir string
	// expertsRoot (optional) enables expert-scoped skill views: private
	// packs under <expertsRoot>/<expertID>/skills/ shadow same-name public
	// packs for that expert's sessions. Wired from main.go.
	expertsRoot string
}

// NewSkillService builds a SkillService rooted at dir, creating the directory,
// seeding the embedded builtin scenario packs (missing ones only) and — on a
// fresh install — the daily-check example so the feature is discoverable.
func NewSkillService(dir string) *SkillService {
	if err := os.MkdirAll(dir, 0o755); err == nil {
		// Fresh-install detection must run before seeding: the builtin packs
		// themselves would otherwise mask an empty directory.
		fresh := false
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
			fresh = true
		}
		seedBuiltins(dir)
		if fresh {
			seedExampleSkill(dir)
		}
	}
	return &SkillService{dir: dir}
}

// SetDir repoints the skills root (data-dir migration). The directory is
// created and builtin packs are seeded if missing; an existing install's
// files are never overwritten. The migration copies the old skills root, so
// seeding is a no-op there in practice.
// SetExpertsRoot wires the experts root (data-dir aware) so expert-scoped
// skill views can resolve private packs.
func (s *SkillService) SetExpertsRoot(dir string) { s.expertsRoot = dir }

func (s *SkillService) SetDir(dir string) {
	s.dir = dir
	if err := os.MkdirAll(dir, 0o755); err == nil {
		seedBuiltins(dir)
	}
}

// seedBuiltins writes every embedded pack file that is missing on disk,
// per file — an edited SKILL.md on the user's side keeps winning, while new
// bundled files shipped in later versions still get seeded next to it.
// Failures on individual files are ignored — a partially seeded set is
// better than an error.
func seedBuiltins(dir string) {
	dismissed := readDismissed(dir)
	// The embedded FS walk itself cannot fail meaningfully; per-file errors
	// below are ignored by design (partial seed beats a broken startup).
	_ = fs.WalkDir(builtinSkills, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, ok := strings.CutPrefix(path, "skills/")
		if !ok || !strings.Contains(rel, "/") {
			return nil // not inside a skill directory
		}
		name, file, _ := strings.Cut(rel, "/")
		if file == "" || dismissed[name] {
			return nil
		}
		target := filepath.Join(dir, name, filepath.FromSlash(file))
		if _, err := os.Stat(target); err == nil {
			return nil // exists (possibly user-edited) — never overwrite
		}
		raw, err := builtinSkills.ReadFile(path)
		if err != nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil
		}
		_ = os.WriteFile(target, raw, 0o644)
		return nil
	})
}

// readDismissed loads the deleted-builtin names (missing file → empty set).
func readDismissed(dir string) map[string]bool {
	out := make(map[string]bool)
	raw, err := os.ReadFile(filepath.Join(dir, dismissedFile))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			out[name] = true
		}
	}
	return out
}

// writeDismissed persists the deleted-builtin names.
func writeDismissed(dir string, set map[string]bool) {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	_ = os.WriteFile(filepath.Join(dir, dismissedFile), []byte(strings.Join(names, "\n")+"\n"), 0o644)
}

var skillNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ErrReservedName is returned when a skill name would collide with internal
// files in the skills root.
var ErrReservedName = errors.New("reserved skill name")

// builtinNames is computed once from the embedded FS so ListSkills can flag
// scenario packs that came with the binary (the badge in the manager UI).
var builtinNames = func() map[string]bool {
	out := make(map[string]bool)
	entries, err := fs.Glob(builtinSkills, "skills/*/SKILL.md")
	if err != nil {
		return out
	}
	for _, path := range entries {
		out[path[len("skills/"):len(path)-len("/SKILL.md")]] = true
	}
	return out
}()

// ListSkills returns every skill's metadata (frontmatter; body not loaded).
func (s *SkillService) ListSkills() ([]domain.Skill, error) {
	return listFromRoot(s.dir)
}

// listFromRoot lists the skill packs under one root directory (the global
// skills root, or an expert's private skills/ directory).
func listFromRoot(root string) ([]domain.Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Skill{}, nil
		}
		return nil, err
	}
	out := make([]domain.Skill, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !skillNameRe.MatchString(e.Name()) {
			continue
		}
		sk, err := parseSkillMD(filepath.Join(root, e.Name(), "SKILL.md"), e.Name())
		if err != nil {
			continue // unreadable/broken skill — skip, never break listing
		}
		sk.Builtin = builtinNames[e.Name()]
		out = append(out, withFiles(sk))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetSkill returns one skill with its full markdown body.
func (s *SkillService) GetSkill(name string) (domain.Skill, error) {
	return getFromRoot(s.dir, name)
}

// getFromRoot loads one skill (frontmatter + body + bundled files) from a
// root directory.
func getFromRoot(root, name string) (domain.Skill, error) {
	if !skillNameRe.MatchString(name) {
		return domain.Skill{}, fmt.Errorf("invalid skill name %q", name)
	}
	sk, err := parseSkillMD(filepath.Join(root, name, "SKILL.md"), name)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("skill %q: %w", name, err)
	}
	sk.Builtin = builtinNames[name]
	return withFiles(sk), nil
}

// SaveSkill writes (creating or overwriting) the skill's SKILL.md. Content is
// the full file — frontmatter optional, name/description parsed on next read.
// Re-saving a dismissed builtin clears its dismissal, so it stays visible.
func (s *SkillService) SaveSkill(name, content string) error {
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("invalid skill name %q (use letters, digits, '-' or '_', up to 64 chars)", name)
	}
	if strings.HasPrefix(name, ".") {
		return ErrReservedName
	}
	skillDir := filepath.Join(s.dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	if set := readDismissed(s.dir); set[name] {
		delete(set, name)
		writeDismissed(s.dir, set)
	}
	return nil
}

// DeleteSkill removes a skill directory. Builtins are recorded in the
// dismissed list so the seeder does not bring them back on next start.
func (s *SkillService) DeleteSkill(name string) error {
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	if strings.HasPrefix(name, ".") {
		return ErrReservedName
	}
	skillDir := filepath.Join(s.dir, name)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		return fmt.Errorf("skill %q: %w", name, err)
	}
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	if builtinNames[name] {
		set := readDismissed(s.dir)
		set[name] = true
		writeDismissed(s.dir, set)
	}
	return nil
}

// parseSkillMD reads a SKILL.md, splitting the optional `---` frontmatter
// (name/description keys) from the markdown body. A missing frontmatter or a
// missing name falls back to the directory name; a missing description falls
// back to the first non-empty body line, trimmed to one sentence.
func parseSkillMD(path, fallbackName string) (domain.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, err
	}
	sk := domain.Skill{Name: fallbackName, Path: path}
	body, name, desc := splitFrontmatter(string(raw))
	if name != "" {
		sk.Name = name
	}
	sk.Description = desc
	sk.Content = strings.TrimSpace(body)
	if sk.Description == "" {
		sk.Description = firstSentence(sk.Content)
	}
	return sk, nil
}

// listSkillFiles walks a skill's directory and returns its bundled files —
// every regular file except SKILL.md itself — as sorted relative paths with
// forward slashes (the portable convention zip and web budgets share).
func listSkillFiles(skillDir string) []string {
	var files []string
	_ = filepath.WalkDir(skillDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil // unreadable/odd entries are skipped, never fatal
		}
		rel, err := filepath.Rel(skillDir, p)
		if err != nil || rel == "SKILL.md" {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files
}

// withFiles attaches the bundled-file listing to a parsed skill.
func withFiles(sk domain.Skill) domain.Skill {
	sk.Files = listSkillFiles(filepath.Dir(sk.Path))
	return sk
}

// safeSkillRelPath validates a bundled-file path from an untrusted source
// (the LLM's skill-tool call or a zip entry): forward slashes, no absolute
// paths, no dot-dot segments, no backslashes or drive letters. Returns the
// cleaned slash path.
func safeSkillRelPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty skill file path")
	}
	if strings.ContainsRune(p, '\\') || strings.ContainsRune(p, ':') {
		return "", fmt.Errorf("invalid skill file path %q", p)
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("absolute skill file path %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".", "..":
			return "", fmt.Errorf("invalid skill file path %q", p)
		}
	}
	return p, nil
}

// skillFileMax total guard is per file (skillFileMaxBytes); PackMaxBytes caps
// one skill pack's bundled payload (export/import sanity, not a hard law).
const skillPackMaxBytes = 32 << 20 // 32 MiB

// SkillFileBytes reads one bundled file of a skill pack (path relative to the
// skill directory, slash-separated) from the global skills root. Used by the
// agent's skill tool (via ReadSkillFile) and the expert export path.
func (s *SkillService) SkillFileBytes(name, path string) ([]byte, error) {
	return fileFromRoot(s.dir, name, path)
}

// fileFromRoot is SkillFileBytes against an arbitrary root (package-level so
// the expert-scoped store can reuse it for private packs).
func fileFromRoot(root, name, path string) ([]byte, error) {
	if !skillNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid skill name %q", name)
	}
	rel, err := safeSkillRelPath(path)
	if err != nil {
		return nil, err
	}
	skillDir := filepath.Join(root, name)
	target := filepath.Join(skillDir, filepath.FromSlash(rel))
	// Containment double-check after cleaning (defence in depth — the
	// validator above already rejects dot-dot segments).
	if rel2, err := filepath.Rel(skillDir, target); err != nil ||
		strings.HasPrefix(rel2, "..") || filepath.IsAbs(rel2) {
		return nil, fmt.Errorf("invalid skill file path %q", path)
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("skill %q file %q: %w", name, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill %q file %q is not a regular file", name, path)
	}
	if info.Size() > skillFileMaxBytes {
		return nil, fmt.Errorf("skill %q file %q exceeds %d bytes", name, path, skillFileMaxBytes)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// SkillFileList returns a skill's bundled files (relative slash paths,
// SKILL.md excluded) — the export/packaging view of withFiles.
func (s *SkillService) SkillFileList(name string) ([]string, error) {
	if !skillNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid skill name %q", name)
	}
	skillDir := filepath.Join(s.dir, name)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		return nil, fmt.Errorf("skill %q: %w", name, err)
	}
	return listSkillFiles(skillDir), nil
}

// ReadSkillFile returns one bundled file's content as text (agent-facing).
func (s *SkillService) ReadSkillFile(name, path string) (string, error) {
	raw, err := s.SkillFileBytes(name, path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ImportSkillFiles installs a whole skill pack (expert zip import): files map
// relative slash paths to contents, SKILL.md required. An existing pack of
// the same name is replaced — import is an explicit user action, and a
// half-overwritten pack would be worse than a replaced one. Re-saving a
// dismissed builtin clears its dismissal (same as SaveSkill).
func (s *SkillService) ImportSkillFiles(name string, files map[string][]byte) error {
	if err := importSkillFilesAt(s.dir, name, files); err != nil {
		return err
	}
	if set := readDismissed(s.dir); set[name] {
		delete(set, name)
		writeDismissed(s.dir, set)
	}
	return nil
}

// importSkillFilesAt is ImportSkillFiles against an arbitrary root (the
// expert-private skills/ directory for scoped imports). Same replace
// semantics; the dismissed bookkeeping only applies to the global root.
func importSkillFilesAt(root, name string, files map[string][]byte) error {
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	if files["SKILL.md"] == nil {
		return fmt.Errorf("skill %q: SKILL.md missing", name)
	}
	cleaned := make(map[string][]byte, len(files))
	var total int
	for p, raw := range files {
		rel, err := safeSkillRelPath(p)
		if err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
		if len(raw) > skillFileMaxBytes {
			return fmt.Errorf("skill %q file %q exceeds %d bytes", name, p, skillFileMaxBytes)
		}
		total += len(raw)
		if total > skillPackMaxBytes {
			return fmt.Errorf("skill %q exceeds %d bytes", name, skillPackMaxBytes)
		}
		cleaned[rel] = raw
	}
	skillDir := filepath.Join(root, name)
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("replace skill %q: %w", name, err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	for rel, raw := range cleaned {
		target := filepath.Join(skillDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return fmt.Errorf("skill %q: write %q: %w", name, rel, err)
		}
	}
	return nil
}

// ExtractSkillFrontmatter parses the name/description keys out of a
// SKILL.md-shaped document's frontmatter (without a file on disk) — used on
// LLM-drafted scenario drafts before they are saved.
func ExtractSkillFrontmatter(raw string) (name, description string) {
	_, name, description = splitFrontmatter(raw)
	return name, description
}

// splitFrontmatter splits the optional `---`-delimited YAML frontmatter from
// a markdown document, reading only the name/description keys.
func splitFrontmatter(doc string) (body, name, description string) {
	trimmed := strings.TrimSpace(doc)
	if strings.HasPrefix(trimmed, "---") {
		if idx := strings.Index(trimmed[3:], "\n---"); idx >= 0 {
			fm := trimmed[3:][:idx]
			rest := trimmed[3:][idx+4:]
			for _, line := range strings.Split(fm, "\n") {
				key, val, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				val = strings.TrimSpace(val)
				switch strings.TrimSpace(strings.ToLower(key)) {
				case "name":
					if val != "" {
						name = val
					}
				case "description":
					description = val
				}
			}
			return rest, name, description
		}
	}
	return doc, "", ""
}

// firstSentence extracts the first non-heading, non-empty line as a fallback
// description (capped at 120 chars).
func firstSentence(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 120 {
			line = line[:120] + "…"
		}
		return line
	}
	return ""
}

// seedExampleSkill writes a starter skill so a fresh install has something
// under the `/` picker. Failures are silent — the directory simply stays empty.
func seedExampleSkill(dir string) {
	skillDir := filepath.Join(dir, "daily-check")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return
	}
	const body = `---
name: daily-check
description: 对当前主机做一轮例行巡检并输出结论
---
# 日常巡检

对当前连接的主机执行一轮例行巡检，步骤：

1. 采集基础指标：uptime、df -h、free -m、top -bn1 前 10 行；
2. 检查失败的 systemd 单元：systemctl --failed；
3. 检查最近的错误日志：journalctl -p err --since "24 hours ago"（限制行数）；
4. 汇总为一份简短结论：健康状态、需要关注的问题、建议的处理动作。

注意：以上命令均为只读诊断（READ），可直接执行；发现需要修复的问题时
先给出方案，等待用户确认，不要自行执行变更操作。
`
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644)
}

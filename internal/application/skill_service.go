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

// builtinSkills holds the embedded diagnosis scenario packs (SKILL.md per
// directory). They ship inside the binary and are seeded into the skills
// root on startup — user-edited copies are never overwritten, and skills the
// user deliberately deleted stay deleted (dismissed list, see seedBuiltins).
//
//go:embed all:skills
var builtinSkills embed.FS

// dismissedFile lists builtin skill names the user removed via the scenario
// manager, so a restart does not resurrect them. Lives inside the skills root
// and starts with a dot, so it never matches the skill-name pattern.
const dismissedFile = ".dismissed-builtins"

// SkillService loads agent skills from the skills directory, following the
// eino adk/middlewares/skill convention: one subdirectory per skill, each
// containing a SKILL.md with optional YAML frontmatter (name/description)
// and a markdown body with the instructions. It backs both the `/skill`
// invocation in the agent input and the model-facing `skill` tool, plus the
// scenario-manager UI (list / read / write / delete).
type SkillService struct {
	dir string
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
func (s *SkillService) SetDir(dir string) {
	s.dir = dir
	if err := os.MkdirAll(dir, 0o755); err == nil {
		seedBuiltins(dir)
	}
}

// seedBuiltins writes every embedded scenario pack that is missing on disk.
// Failures on individual packs are ignored — the directory copy wins as soon
// as the user edits it, and a partially seeded set is better than an error.
func seedBuiltins(dir string) {
	dismissed := readDismissed(dir)
	entries, err := fs.Glob(builtinSkills, "skills/*/SKILL.md")
	if err != nil {
		return
	}
	for _, path := range entries {
		name := path[len("skills/") : len(path)-len("/SKILL.md")]
		if dismissed[name] {
			continue
		}
		skillDir := filepath.Join(dir, name)
		target := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(target); err == nil {
			continue // exists (possibly user-edited) — never overwrite
		}
		raw, err := builtinSkills.ReadFile(path)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(target, raw, 0o644)
	}
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
	entries, err := os.ReadDir(s.dir)
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
		sk, err := parseSkillMD(filepath.Join(s.dir, e.Name(), "SKILL.md"), e.Name())
		if err != nil {
			continue // unreadable/broken skill — skip, never break listing
		}
		sk.Builtin = builtinNames[e.Name()]
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetSkill returns one skill with its full markdown body.
func (s *SkillService) GetSkill(name string) (domain.Skill, error) {
	if !skillNameRe.MatchString(name) {
		return domain.Skill{}, fmt.Errorf("invalid skill name %q", name)
	}
	sk, err := parseSkillMD(filepath.Join(s.dir, name, "SKILL.md"), name)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("skill %q: %w", name, err)
	}
	sk.Builtin = builtinNames[name]
	return sk, nil
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

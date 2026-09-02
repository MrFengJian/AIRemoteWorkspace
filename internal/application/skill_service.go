package application

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// SkillService loads agent skills from the skills directory, following the
// eino adk/middlewares/skill convention: one subdirectory per skill, each
// containing a SKILL.md with optional YAML frontmatter (name/description)
// and a markdown body with the instructions. It backs both the `/skill`
// invocation in the agent input and the model-facing `skill` tool.
type SkillService struct {
	dir string
}

// NewSkillService builds a SkillService rooted at dir, creating the directory
// and seeding an example skill on first launch so the feature is discoverable.
func NewSkillService(dir string) *SkillService {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &SkillService{dir: dir}
	}
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		seedExampleSkill(dir)
	}
	return &SkillService{dir: dir}
}

// SetDir repoints the skills root (data-dir migration). The directory is
// created if missing; no example is re-seeded for an existing install.
func (s *SkillService) SetDir(dir string) {
	s.dir = dir
	_ = os.MkdirAll(dir, 0o755)
}

var skillNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

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
	return sk, nil
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
	body := string(raw)

	if strings.HasPrefix(strings.TrimSpace(body), "---") {
		if idx := strings.Index(strings.TrimSpace(body)[3:], "\n---"); idx >= 0 {
			fm := strings.TrimSpace(body)[3:][:idx]
			rest := strings.TrimSpace(body)[3:][idx+4:]
			for _, line := range strings.Split(fm, "\n") {
				key, val, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				val = strings.TrimSpace(val)
				switch strings.TrimSpace(strings.ToLower(key)) {
				case "name":
					if val != "" {
						sk.Name = val
					}
				case "description":
					sk.Description = val
				}
			}
			body = rest
		}
	}

	body = strings.TrimSpace(body)
	sk.Content = body
	if sk.Description == "" {
		sk.Description = firstSentence(body)
	}
	return sk, nil
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

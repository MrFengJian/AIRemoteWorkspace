package appdeploy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ai-remote/workspace/internal/domain"
)

// specIDRe 限制应用 ID 为 slug（目录名安全）。
var specIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-_.]{0,63}$`)

// templateRe 匹配命令模板占位符 {{.key}}。
var templateRe = regexp.MustCompile(`\{\{\.([A-Za-z0-9_]+)\}\}`)

// ValidateSpec 校验 app.json 的结构完整性（目录加载与更新安装前的关卡）。
func ValidateSpec(s *domain.AppSpec) error {
	if s.SchemaVersion != domain.AppSpecSchemaVersion {
		return fmt.Errorf("app %q: 不支持的 schemaVersion %d（当前 %d）", s.ID, s.SchemaVersion, domain.AppSpecSchemaVersion)
	}
	if !specIDRe.MatchString(s.ID) {
		return fmt.Errorf("app id %q 不合法（小写字母/数字/-/_. 开头字母或数字，≤64 字符）", s.ID)
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("app %q: 缺少 name", s.ID)
	}
	if s.CatalogVersion <= 0 {
		return fmt.Errorf("app %q: catalogVersion 必须为正整数", s.ID)
	}
	if len(s.Install.Steps) == 0 {
		return fmt.Errorf("app %q: install.steps 不能为空", s.ID)
	}
	if strings.TrimSpace(s.Install.Detect) == "" {
		return fmt.Errorf("app %q: install.detect 不能为空", s.ID)
	}
	if strings.TrimSpace(s.Ops.Status) == "" {
		return fmt.Errorf("app %q: ops.status 不能为空", s.ID)
	}
	if strings.TrimSpace(s.Ops.Logs) == "" {
		return fmt.Errorf("app %q: ops.logs 不能为空", s.ID)
	}
	for _, in := range s.Install.Inputs {
		if !templateRe.MatchString(safeInputKey(in.Key)) {
			return fmt.Errorf("app %q: 输入 key %q 不合法", s.ID, in.Key)
		}
	}
	if s.Panel != nil && s.Panel.URL != "" && s.Panel.Port <= 0 {
		return fmt.Errorf("app %q: panel.port 必须为正整数", s.ID)
	}
	return nil
}

// safeInputKey 包一层以满足上面的正则检查（仅判断 key 本身）。
func safeInputKey(k string) string { return "{{." + k + "}}" }

// ValidateInputs 校验用户输入：必填、min=N 最短长度。返回面向用户的错误。
func ValidateInputs(spec *domain.AppSpec, inputs map[string]string) error {
	for _, in := range spec.Install.Inputs {
		v := strings.TrimSpace(inputs[in.Key])
		if in.Required && v == "" {
			return fmt.Errorf("缺少必填输入：%s", in.Label)
		}
		if v == "" {
			continue
		}
		if strings.HasPrefix(in.Validate, "min=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(in.Validate, "min=")); err == nil && len([]rune(v)) < n {
				return fmt.Errorf("%s 长度不能少于 %d 位", in.Label, n)
			}
		}
	}
	return nil
}

// RenderCommand 渲染命令模板：{{.key}} 替换为对应值（一律经单引号 shell
// 转义，值中含引号也安全）。未知占位符返回错误，避免把字面大括号送上远端。
func RenderCommand(tmpl string, vars map[string]string) (string, error) {
	return render(tmpl, vars, true)
}

// RenderTemplate 渲染非 shell 模板（URL 等）：直接替换，不做引号转义。
func RenderTemplate(tmpl string, vars map[string]string) (string, error) {
	return render(tmpl, vars, false)
}

func render(tmpl string, vars map[string]string, quote bool) (string, error) {
	var missing []string
	out := templateRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := templateRe.FindStringSubmatch(m)[1]
		v, ok := vars[key]
		if !ok {
			missing = append(missing, key)
			return m
		}
		if quote {
			return shellQuote(v)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("命令模板缺少变量: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// shellQuote 用单引号包裹 s 供 POSIX shell 使用，转义内嵌单引号。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

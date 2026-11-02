package appdeploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// lookupBuiltin 读取内嵌目录文件（测试断言用）。
func lookupBuiltin(name string) ([]byte, error) { return builtinCatalog.ReadFile("catalog/" + name) }

func mustReadBuiltin(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := lookupBuiltin(name)
	if err != nil {
		t.Fatalf("read builtin %s: %v", name, err)
	}
	return raw
}

// lookupDisk 读取数据目录中已种子解压的文件。
func lookupDisk(t *testing.T, dir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "catalog", filepath.FromSlash(name)))
	return string(b), err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// quotedScript 把脚本包成 runSession 实际发送的 bash -l -c 形式（与
// service.go 的 shellQuote 转义一致），供 ranContains 断言使用。
func quotedScript(script string) string {
	return "bash -l -c '" + stringsReplaceAll(script) + "'"
}

func stringsReplaceAll(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }

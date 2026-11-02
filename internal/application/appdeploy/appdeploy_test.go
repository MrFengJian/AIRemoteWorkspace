package appdeploy

import (
	"strings"
	"testing"

	"github.com/ai-remote/workspace/internal/domain"
)

// TestBuiltinCatalogValid 内置目录自检：catalog.json 与每个 app.json 都能解析
// 并通过结构校验；每个应用实现几乎相同的操作集（统一数据集）。
func TestBuiltinCatalogValid(t *testing.T) {
	version, err := readBuiltinVersion()
	if err != nil {
		t.Fatalf("read builtin catalog.json: %v", err)
	}
	if version <= 0 {
		t.Fatalf("builtin catalog version = %d, want > 0", version)
	}

	var cat domain.Catalog
	raw, err := builtinCatalog.ReadFile("catalog/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonUnmarshal(raw, &cat); err != nil {
		t.Fatalf("catalog.json: %v", err)
	}
	if len(cat.Apps) == 0 {
		t.Fatal("builtin catalog has no apps")
	}

	for _, ref := range cat.Apps {
		t.Run(ref.ID, func(t *testing.T) {
			raw, err := builtinCatalog.ReadFile("catalog/" + ref.ID + "/app.json")
			if err != nil {
				t.Fatalf("read app.json: %v", err)
			}
			spec, err := loadSpecFromBytes(raw)
			if err != nil {
				t.Fatalf("invalid app.json: %v", err)
			}
			if spec.ID != ref.ID {
				t.Fatalf("spec.ID = %q, dir = %q", spec.ID, ref.ID)
			}
			// 统一操作集：检测/安装步骤/状态/日志必备；控制三件套必备。
			if spec.Install.Detect == "" || len(spec.Install.Steps) == 0 {
				t.Fatal("install.detect / install.steps required")
			}
			if spec.Ops.Start == "" || spec.Ops.Stop == "" || spec.Ops.Restart == "" {
				t.Fatal("ops.start/stop/restart required")
			}
			// status 契约：脚本必须产出全部统一 KEY（Hermes 无端口也输出
			// LISTENING=no——键集一致才能解析成同一数据集）。
			for _, key := range []string{"STATUS=", "PID=", "UPTIME_SEC=", "MEM_KB=", "LISTENING=", "HEALTH="} {
				if !strings.Contains(spec.Ops.Status, key) {
					t.Errorf("ops.status missing contract key %s", key)
				}
			}
			if !strings.Contains(spec.Ops.Logs, "{{.lines}}") {
				t.Error("ops.logs missing {{.lines}}")
			}
			// 官方图标必须随目录分发（保持原图格式，按 app.json 登记的文件名）。
			if spec.Icon == "" {
				t.Error("icon field empty — 官方图标未登记")
			} else if _, err := lookupBuiltin(ref.ID + "/" + spec.Icon); err != nil {
				t.Errorf("icon file %s missing from catalog dir: %v", spec.Icon, err)
			}
		})
	}
}

// TestSeedBuiltinCatalog 种子解压：首启写入全部文件；GitHub 更新过的磁盘
// 不被覆盖；内置来源的同版本内容漂移随二进制更新。
func TestSeedBuiltinCatalog(t *testing.T) {
	dir := t.TempDir()
	if err := SeedBuiltinCatalog(dir, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, name := range []string{"catalog.json", "octop/app.json", "openclaw/app.json", "hermes/app.json"} {
		if _, err := lookupBuiltin(name); err != nil {
			t.Fatalf("seeded file missing: %s", name)
		}
	}
	disk := readDiskCatalogVersion(dir + "/catalog")
	builtin, _ := readBuiltinVersion()
	if disk != builtin {
		t.Fatalf("disk version = %d, want %d", disk, builtin)
	}

	// 磁盘版本更高（GitHub 更新过）→ 内置不覆盖。动态写入 builtin+1，避免
	// 每次目录升版都改这里的硬编码。
	var cat domain.Catalog
	if err := jsonUnmarshal(mustReadBuiltin(t, "catalog.json"), &cat); err != nil {
		t.Fatal(err)
	}
	newer, err := jsonMarshal(domain.Catalog{CatalogVersion: builtin + 1, Apps: cat.Apps})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir+"/catalog/catalog.json", string(newer))
	if err := SeedBuiltinCatalog(dir, domain.AppCatalogSourceRelease); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if got := readDiskCatalogVersion(dir + "/catalog"); got != builtin+1 {
		t.Fatalf("disk version after reseed = %d, want %d (newer wins)", got, builtin+1)
	}

	// 内置来源 + 同版本 + 内容漂移（改一条命令）→ 随二进制更新覆盖。
	writeFile(t, dir+"/catalog/octop/app.json",
		strings.Replace(string(mustReadBuiltin(t, "octop/app.json")), "octop service start", "octop service START", 1))
	if err := SeedBuiltinCatalog(dir, domain.AppCatalogSourceBuiltin); err != nil {
		t.Fatalf("reseed (builtin drift): %v", err)
	}
	if got, _ := lookupDisk(t, dir, "octop/app.json"); !strings.Contains(got, "octop service start") {
		t.Fatal("builtin-sourced drift not restored from embedded catalog")
	}

	// GitHub 来源 + 同版本 + 内容漂移 → 不覆盖（防降级）。
	writeFile(t, dir+"/catalog/octop/app.json",
		strings.Replace(string(mustReadBuiltin(t, "octop/app.json")), "octop service start", "octop service START", 1))
	if err := SeedBuiltinCatalog(dir, domain.AppCatalogSourceRelease); err != nil {
		t.Fatalf("reseed (release drift): %v", err)
	}
	if got, _ := lookupDisk(t, dir, "octop/app.json"); !strings.Contains(got, "octop service START") {
		t.Fatal("release-sourced catalog was downgraded by builtin")
	}
}

// TestRenderCommand 模板渲染：值一律单引号转义；未知变量报错。
func TestRenderCommand(t *testing.T) {
	out, err := RenderCommand("octop init --admin-password {{.adminPassword}} --yes",
		map[string]string{"adminPassword": "sec'123"})
	if err != nil {
		t.Fatal(err)
	}
	want := `octop init --admin-password 'sec'\''123' --yes`
	if out != want {
		t.Fatalf("render = %q, want %q", out, want)
	}
	if _, err := RenderCommand("cmd {{.missing}}", map[string]string{}); err == nil {
		t.Fatal("expected error for missing template var")
	}

	// URL 等展示模板不做 shell 转义（否则地址会带引号）。
	url, err := RenderTemplate("http://{{.host}}:{{.port}}", map[string]string{"host": "172.18.93.135", "port": "8088"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://172.18.93.135:8088" {
		t.Fatalf("RenderTemplate = %q, want unquoted URL", url)
	}
}

// TestValidateSpec 结构校验的关键拒绝路径。
func TestValidateSpec(t *testing.T) {
	base := domain.AppSpec{
		SchemaVersion: 1, ID: "demo", Name: "Demo", CatalogVersion: 1,
		Install: domain.AppInstall{Detect: "echo yes", Steps: []domain.DeployStep{{Name: "s", Command: "echo hi"}}},
		Ops:     domain.AppOps{Status: "echo STATUS=running", Logs: "tail {{.lines}}"},
	}
	if err := ValidateSpec(&base); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	badID := base
	badID.ID = "Bad ID!"
	if err := ValidateSpec(&badID); err == nil {
		t.Fatal("expected id error")
	}
	noStatus := base
	noStatus.Ops.Status = ""
	if err := ValidateSpec(&noStatus); err == nil {
		t.Fatal("expected ops.status error")
	}
	wrongSchema := base
	wrongSchema.SchemaVersion = 99
	if err := ValidateSpec(&wrongSchema); err == nil {
		t.Fatal("expected schemaVersion error")
	}
}

// TestValidateInputs 必填与最短长度校验。
func TestValidateInputs(t *testing.T) {
	spec := domain.AppSpec{Install: domain.AppInstall{Inputs: []domain.AppInput{
		{Key: "pw", Label: "密码", Required: true, Validate: "min=8"},
	}}}
	if err := ValidateInputs(&spec, map[string]string{}); err == nil {
		t.Fatal("expected required error")
	}
	if err := ValidateInputs(&spec, map[string]string{"pw": "short"}); err == nil {
		t.Fatal("expected min length error")
	}
	if err := ValidateInputs(&spec, map[string]string{"pw": "longenough1"}); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
}

// TestParseHostAppStatus 契约解析（含缺省字段）。
func TestParseHostAppStatus(t *testing.T) {
	raw := "STATUS=running\nPID=98236\nUPTIME_SEC=5967\nMEM_KB=757786\nLISTENING=yes\nHEALTH=ok\nVERSION=1.2.3\n"
	st := ParseHostAppStatus(raw)
	if st.State != domain.AppStateRunning || st.PID != 98236 || !st.Listening ||
		st.UptimeSec != 5967 || st.MemKB != 757786 || st.Health != "ok" || st.Version != "1.2.3" {
		t.Fatalf("parsed = %+v", st)
	}
	empty := ParseHostAppStatus("")
	if empty.State != domain.AppStateUnknown || empty.Health != "unknown" || empty.PID != 0 {
		t.Fatalf("empty parse = %+v", empty)
	}
}

// TestFormatUptime 时长格式（天-时:分:秒）。
func TestFormatUptime(t *testing.T) {
	cases := map[int64]string{
		0:      "-",
		59:     "0:00:59",
		3661:   "1:01:01",
		147279: "1-16:54:39",
	}
	for in, want := range cases {
		if got := FormatUptime(in); got != want {
			t.Errorf("FormatUptime(%d) = %q, want %q", in, got, want)
		}
	}
}

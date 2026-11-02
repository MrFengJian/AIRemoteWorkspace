package appdeploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// ── 测试替身 ─────────────────────────────────────────────────────────────

// memRepo 是 Repository 的内存实现。
type memRepo struct {
	mu      sync.Mutex
	insts   map[string]domain.Installation
	tasks   map[string]domain.DeployTask
	order   []string
	meta    domain.CatalogMeta
	hasMeta bool
}

func newMemRepo() *memRepo {
	return &memRepo{insts: map[string]domain.Installation{}, tasks: map[string]domain.DeployTask{}}
}

func installationKey(hostID, appID string) string { return hostID + "/" + appID }

func (m *memRepo) SaveInstallation(inst domain.Installation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.insts[installationKey(inst.HostID, inst.AppID)] = inst
	return nil
}
func (m *memRepo) GetInstallation(hostID, appID string) (domain.Installation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.insts[installationKey(hostID, appID)]
	if !ok {
		return domain.Installation{}, fmt.Errorf("not found")
	}
	return in, nil
}
func (m *memRepo) ListInstallations(hostID string) ([]domain.Installation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Installation
	for k, in := range m.insts {
		if hostID == "" || strings.HasPrefix(k, hostID+"/") {
			out = append(out, in)
		}
	}
	return out, nil
}
func (m *memRepo) DeleteInstallation(hostID, appID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.insts, installationKey(hostID, appID))
	return nil
}
func (m *memRepo) SaveTask(t domain.DeployTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[t.ID]; !ok {
		m.order = append(m.order, t.ID)
	}
	m.tasks[t.ID] = t
	return nil
}
func (m *memRepo) GetTask(id string) (domain.DeployTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return domain.DeployTask{}, fmt.Errorf("not found")
	}
	return t, nil
}
func (m *memRepo) ListTasks(hostID string, limit int) ([]domain.DeployTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.DeployTask
	for i := len(m.order) - 1; i >= 0 && len(out) < limit; i-- {
		t := m.tasks[m.order[i]]
		if hostID == "" || t.HostID == hostID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memRepo) SaveCatalogMeta(m2 domain.CatalogMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meta = m2
	m.hasMeta = true
	return nil
}
func (m *memRepo) GetCatalogMeta() (domain.CatalogMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasMeta {
		return domain.CatalogMeta{}, nil
	}
	return m.meta, nil
}

// memCreds 是 CredentialStore 的内存实现（验证安装密码自动落为面板凭证）。
type memCreds struct {
	mu   sync.Mutex
	data map[string]map[string]string
}

func (m *memCreds) SavePanelCredential(hostID, appID string, values map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = map[string]map[string]string{}
	}
	m.data[hostID+"/"+appID] = values
	return nil
}
func (m *memCreds) GetPanelCredential(hostID, appID string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[hostID+"/"+appID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return v, nil
}
func (m *memCreds) DeletePanelCredential(hostID, appID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, hostID+"/"+appID)
	return nil
}

// fakeSessions 记录执行的脚本（应用命令经 stdin 送入 bash -l -s），按脚本
// 片段返回预置结果。
type fakeSessions struct {
	mu    sync.Mutex
	ran   []string
	reply func(script string) (string, error)
}

func (f *fakeSessions) ExecInSessionCtx(_ context.Context, _ string, cmd string) (string, error) {
	f.mu.Lock()
	f.ran = append(f.ran, cmd)
	f.mu.Unlock()
	if f.reply != nil {
		return f.reply(cmd)
	}
	return "", nil
}

// ExecInSessionStdin 是应用命令的实际执行路径：记录 stdin 脚本内容。
func (f *fakeSessions) ExecInSessionStdin(_ context.Context, _ string, cmd string, stdin []byte) (string, error) {
	f.mu.Lock()
	f.ran = append(f.ran, string(stdin))
	f.mu.Unlock()
	if f.reply != nil {
		return f.reply(string(stdin))
	}
	return "", nil
}

func (f *fakeSessions) ranContains(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.ran {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// waitFor 轮询条件成立（部署是异步 goroutine）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ── 用例 ─────────────────────────────────────────────────────────────────

// newTestService 用临时目录（含一个最小 demo 应用）装配服务。
func newTestService(t *testing.T, reply func(string) (string, error)) (*Service, *memRepo, *fakeSessions) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "custom", "demo", "app.json"), `{
		"schemaVersion": 1, "id": "demo", "name": "Demo", "website": "https://example.com",
		"catalogVersion": 1,
		"install": {
			"detect": "command -v demo",
			"inputs": [{"key": "password", "label": "密码", "type": "secret", "required": true, "credentialKey": "password"}],
			"steps": [{"name": "安装", "command": "install-demo --password {{.password}}"}],
			"verify": "demo --check"
		},
		"ops": {
			"start": "start-demo", "stop": "stop-demo", "restart": "restart-demo",
			"status": "echo STATUS=running",
			"logs": "demo-logs {{.lines}}",
			"openPort": "open-port {{.port}}",
			"update": "update-demo",
			"uninstall": [{"name": "卸载", "command": "uninstall-demo"}]
		},
		"panel": {"url": "http://{{.host}}:8080", "port": 8080,
			"credentials": [{"key": "password", "label": "密码", "secret": true}]}
	}`)
	sessions := &fakeSessions{reply: reply}
	svc := NewService(dir, newMemRepo(), sessions, nil, nil, nil, nil, nil, nil)
	return svc, svc.repo.(*memRepo), sessions
}

// TestInstallFlowSuccess 安装成功路径：步骤渲染（值带引号）、安装记录 installed。
func TestInstallFlowSuccess(t *testing.T) {
	svc, repo, sessions := newTestService(t, func(cmd string) (string, error) {
		return "", nil
	})
	task, err := svc.StartInstall(context.Background(), "sess-1", "host-1", "demo",
		map[string]string{"password": "sec123"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "task success", func() bool {
		got, _ := repo.GetTask(task.ID)
		return got.Status == domain.AppTaskSucceeded
	})
	if !sessions.ranContains(quotedScript("install-demo --password 'sec123'")) {
		t.Fatal("rendered install command missing (value must be shell-quoted)")
	}
	// 步骤需携带渲染后的最终命令（不含模板变量），供面板展示。
	got, err := repo.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) == 0 || got.Steps[0].Command != `install-demo --password 'sec123'` {
		t.Fatalf("step command not persisted/rendered: %+v", got.Steps)
	}
	inst, err := repo.GetInstallation("host-1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if inst.Status != domain.AppInstallStatusInstalled {
		t.Fatalf("installation status = %s, want installed", inst.Status)
	}
}

// TestInstallFlowFailure 中途失败：任务 failed、最后错误指向失败步骤、记录 failed。
func TestInstallFlowFailure(t *testing.T) {
	step := 0
	svc, repo, _ := newTestService(t, func(cmd string) (string, error) {
		step++
		if step == 2 { // verify 失败
			return "demo: not found", fmt.Errorf("exit status 127")
		}
		return "", nil
	})
	task, err := svc.StartInstall(context.Background(), "sess-1", "host-1", "demo",
		map[string]string{"password": "pw123456"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "task failure", func() bool {
		got, _ := repo.GetTask(task.ID)
		return got.Status == domain.AppTaskFailed
	})
	got, _ := repo.GetTask(task.ID)
	if got.LastError == "" {
		t.Fatal("lastError empty")
	}
	// 失败步骤的错误里必须保留远端实际输出（错误流），而非只有 exit status。
	var failedStep *domain.DeployStepRun
	for i := range got.Steps {
		if got.Steps[i].Status == domain.AppStepFailed {
			failedStep = &got.Steps[i]
		}
	}
	if failedStep == nil || !strings.Contains(failedStep.Error, "demo: not found") {
		t.Fatalf("failed step error missing remote output: %+v", failedStep)
	}
	inst, _ := repo.GetInstallation("host-1", "demo")
	if inst.Status != domain.AppInstallStatusFailed {
		t.Fatalf("installation status = %s, want failed", inst.Status)
	}
}

// TestStartInstallValidation 必填输入缺失直接拒绝（不启动任务）。
func TestStartInstallValidation(t *testing.T) {
	svc, repo, _ := newTestService(t, nil)
	if _, err := svc.StartInstall(context.Background(), "s", "h", "demo", nil); err == nil {
		t.Fatal("expected missing-input error")
	}
	if tasks, _ := repo.ListTasks("", 10); len(tasks) != 0 {
		t.Fatalf("no task should be created, got %d", len(tasks))
	}
}

// TestUninstallRemovesInstallation 卸载成功后删除安装记录。
func TestUninstallRemovesInstallation(t *testing.T) {
	svc, repo, _ := newTestService(t, nil)
	if _, err := svc.StartInstall(context.Background(), "s", "h", "demo",
		map[string]string{"password": "pw123456"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "install", func() bool {
		in, _ := repo.GetInstallation("h", "demo")
		return in.Status == domain.AppInstallStatusInstalled
	})
	if _, err := svc.StartUninstall(context.Background(), "s", "h", "demo"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "uninstall", func() bool {
		_, err := repo.GetInstallation("h", "demo")
		return err != nil
	})
}

// TestStatusAndLogs 会话通道的状态/日志走 ops.status / ops.logs。
func TestStatusAndLogs(t *testing.T) {
	svc, _, sessions := newTestService(t, func(cmd string) (string, error) {
		if strings.Contains(cmd, "STATUS=") {
			return "STATUS=running\nPID=7\nLISTENING=yes\nHEALTH=ok\n", nil
		}
		if strings.Contains(cmd, "demo-logs") {
			return "log line 1\nlog line 2", nil
		}
		return "", nil
	})
	st, err := svc.Status(context.Background(), "sess", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != domain.AppStateRunning || st.PID != 7 {
		t.Fatalf("status = %+v", st)
	}
	logs, err := svc.Logs(context.Background(), "sess", "demo", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs, "log line 1") || !sessions.ranContains(quotedScript("demo-logs '100'")) {
		t.Fatalf("logs render/execute wrong: ran=%v logs=%q", sessions.ran, logs)
	}
}

// TestCatalogMerge 目录加载：custom + 内置合并且按 ID 排序。
func TestCatalogMerge(t *testing.T) {
	svc, _, _ := newTestService(t, nil)
	entries, err := svc.ListCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Spec.ID)
	}
	// 临时目录里没有种子（NewService 不种子），custom/demo 在列即可；
	// 内置目录在 embed 里但未解压到该 appsDir，不应出现。
	if len(ids) != 1 || ids[0] != "demo" {
		t.Fatalf("catalog ids = %v, want [demo]", ids)
	}
	entry, err := svc.GetApp("demo")
	if err != nil || entry.Source != "custom" {
		t.Fatalf("GetApp = %+v, %v", entry, err)
	}
	if _, err := svc.GetApp("missing"); err == nil {
		t.Fatal("expected missing app error")
	}
}

// TestCapOutput 脱敏 + 截断。
func TestCapOutput(t *testing.T) {
	out := capOutput("password=hunter2\ntoken=abc123\nok")
	if strings.Contains(out, "hunter2") || strings.Contains(out, "abc123") {
		t.Fatalf("secrets leaked: %q", out)
	}
	big := strings.Repeat("x", 40<<10)
	capped := capBytes(big, stepOutputCap)
	if len(capped) >= len(big) || !strings.Contains(capped, "truncated") {
		t.Fatal("capBytes failed to truncate")
	}
	_ = os.Getenv // 保持 os import（跨平台文件测试未来用）
}

// TestRecoverInterrupted 启动恢复：installing 安装记录与 running 任务转为
// failed（可直接重装）；installed 保持原状。
func TestRecoverInterrupted(t *testing.T) {
	svc, repo, _ := newTestService(t, nil)
	_ = repo.SaveInstallation(domain.Installation{
		HostID: "h", AppID: "demo", Status: domain.AppInstallStatusInstalling,
	})
	_ = repo.SaveTask(domain.DeployTask{
		ID: "task-1", HostID: "h", AppID: "demo", Kind: domain.AppTaskKindInstall,
		Status: domain.AppTaskRunning,
		Steps: []domain.DeployStepRun{
			{StepID: "s1", Name: "安装", Status: domain.AppStepRunning},
			{StepID: "s2", Name: "验证", Status: domain.AppStepPending},
		},
	})
	_ = repo.SaveInstallation(domain.Installation{HostID: "h2", AppID: "demo", Status: domain.AppInstallStatusInstalled})
	svc.RecoverInterrupted()

	in, err := repo.GetInstallation("h", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != domain.AppInstallStatusFailed || in.LastError == "" {
		t.Fatalf("recovered installation = %+v, want failed with reason", in)
	}
	tk, err := repo.GetTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != domain.AppTaskFailed || tk.FinishedAt == nil {
		t.Fatalf("recovered task = %+v, want failed with finish time", tk)
	}
	if tk.Steps[0].Status != domain.AppStepFailed || tk.Steps[1].Status != domain.AppStepPending {
		t.Fatalf("step statuses = %s/%s, want failed/pending", tk.Steps[0].Status, tk.Steps[1].Status)
	}
	// 已成功的安装保持原状
	in2, _ := repo.GetInstallation("h2", "demo")
	if in2.Status != domain.AppInstallStatusInstalled {
		t.Fatalf("installed record was touched: %+v", in2)
	}
}

// TestInstallSavesCredential 安装成功后自动把初始化密码保存为面板登录凭证，
// 且保留已有用户名等非安装字段。
func TestInstallSavesCredential(t *testing.T) {
	svc, repo, _ := newTestService(t, nil)
	creds := &memCreds{data: map[string]map[string]string{
		"h/demo": {"username": "admin"},
	}}
	svc.creds = creds

	// demo 应用的密码输入声明了 credentialKey=password。
	entry, err := svc.GetApp("demo")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Spec.Install.Inputs[0].CredentialKey != "password" {
		t.Fatalf("test fixture credentialKey = %q", entry.Spec.Install.Inputs[0].CredentialKey)
	}
	task, err := svc.StartInstall(context.Background(), "s", "h", "demo",
		map[string]string{"password": "pw123456"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "install", func() bool {
		got, _ := repo.GetTask(task.ID)
		return got.Status == domain.AppTaskSucceeded
	})
	waitFor(t, "credential saved", func() bool {
		v, err := creds.GetPanelCredential("h", "demo")
		return err == nil && v["password"] == "pw123456"
	})
	v, _ := creds.GetPanelCredential("h", "demo")
	if v["username"] != "admin" {
		t.Fatalf("existing credential lost: %+v", v)
	}
}

// fakeStream 是 StreamExecutor 的测试替身：分块吐出预置输出（模拟流式）。
type fakeStream struct {
	sess *fakeSessions
}

func (f *fakeStream) ExecInSessionStream(_ context.Context, sessionID, cmd string, onChunk func(string)) (string, int, error) {
	f.sess.ExecInSessionCtx(context.Background(), sessionID, cmd) // 保持 ran 记录一致
	for _, chunk := range []string{"downloading...\n", "installing...\n", "done\n"} {
		onChunk(chunk)
	}
	return "downloading...\ninstalling...\ndone\n", 0, nil
}

// TestInstallStreamsOutput 流式执行：步骤输出在运行中即逐步可见（chunks 合并）。
func TestInstallStreamsOutput(t *testing.T) {
	svc, repo, sessions := newTestService(t, nil)
	svc.stream = &fakeStream{sess: sessions}
	task, err := svc.StartInstall(context.Background(), "sess", "h", "demo",
		map[string]string{"password": "pw123456"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "task success", func() bool {
		got, _ := repo.GetTask(task.ID)
		return got.Status == domain.AppTaskSucceeded
	})
	got, _ := repo.GetTask(task.ID)
	first := got.Steps[0]
	if first.Status != domain.AppStepSucceeded {
		t.Fatalf("step status = %s", first.Status)
	}
	for _, want := range []string{"downloading...", "installing...", "done"} {
		if !strings.Contains(first.Output, want) {
			t.Fatalf("streamed output missing %q: %q", want, first.Output)
		}
	}
}

// slowStream 先吐一块输出，停顿超过刷新周期后再吐第二块——模拟长安装器。
type slowStream struct{ sess *fakeSessions }

func (f *slowStream) ExecInSessionStream(_ context.Context, sessionID, cmd string, onChunk func(string)) (string, int, error) {
	f.sess.ExecInSessionCtx(context.Background(), sessionID, cmd)
	onChunk("early chunk\n")
	time.Sleep(1200 * time.Millisecond) // > 500ms 刷新周期
	onChunk("late chunk\n")
	return "early chunk\nlate chunk\n", 0, nil
}

// TestInstallOutputVisibleDuringRun 流式路径：运行中（步骤未结束时）输出已经
// 落库可见，而不是攒到结束。
func TestInstallOutputVisibleDuringRun(t *testing.T) {
	svc, repo, sessions := newTestService(t, nil)
	svc.stream = &slowStream{sess: sessions}
	task, err := svc.StartInstall(context.Background(), "sess", "h", "demo",
		map[string]string{"password": "pw123456"})
	if err != nil {
		t.Fatal(err)
	}
	// 700ms 时（第一块已吐出、刷新周期 500ms 已过、步骤仍在运行）输出应可见。
	waitFor(t, "mid-run output", func() bool {
		got, _ := repo.GetTask(task.ID)
		if got.Status != domain.AppTaskRunning || len(got.Steps) == 0 {
			return false
		}
		return strings.Contains(got.Steps[0].Output, "early chunk")
	})
	waitFor(t, "task success", func() bool {
		got, _ := repo.GetTask(task.ID)
		return got.Status == domain.AppTaskSucceeded
	})
	got, _ := repo.GetTask(task.ID)
	if !strings.Contains(got.Steps[0].Output, "late chunk") {
		t.Fatalf("final output missing late chunk: %q", got.Steps[0].Output)
	}
}

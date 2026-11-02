package appdeploy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ai-remote/workspace/internal/domain"
)

// 默认步骤超时与输出上限。
const (
	defaultStepTimeout = 600 * time.Second
	stepOutputCap      = 16 << 10 // 16KB（截断 + 脱敏后落库/推送）
	logByteCap         = 256 << 10
)

// SessionExecutor 复用已打开的终端会话执行命令（写操作的主通道：安装/控制
// 都从会话内应用面板发起，终端可见）。实现：ssh.Manager。
type SessionExecutor interface {
	ExecInSessionCtx(ctx context.Context, sessionID, cmd string) (string, error)
}

// HostExecutor 按主机一次性拨号执行命令（独立菜单页只读总览的通道）。
// 实现：ssh.OneShotRunner。非零退出码不是 error，由 exitCode 承载。
type HostExecutor interface {
	Exec(ctx context.Context, hostID, cmd string, stdin []byte, timeout time.Duration) (output string, exitCode int, err error)
}

// StreamExecutor 在会话通道上执行命令并实时流式返回输出（安装步骤的主通
// 道：长时间安装器能看到实时进度）。实现：ssh.Manager。err 语义与
// SessionExecutor 一致（非零退出 = *ssh.ExitError）。
type StreamExecutor interface {
	ExecInSessionStream(ctx context.Context, sessionID, cmd string, onChunk func(string)) (string, int, error)
}

// HostResolver 解析主机记录（面板地址渲染需要主机地址）。实现：HostService。
type HostResolver interface {
	Get(hostID string) (domain.Host, error)
}

// CredentialStore 面板登录凭证的 OS 钥匙串存储（按 主机+应用 一条 JSON）。
type CredentialStore interface {
	SavePanelCredential(hostID, appID string, values map[string]string) error
	GetPanelCredential(hostID, appID string) (map[string]string, error)
	DeletePanelCredential(hostID, appID string) error
}

// EventEmitter 把任务事件推送到前端（interfaces 层桥接 Wails "apps:task"）。
type EventEmitter interface {
	EmitTaskEvent(ev domain.DeployTaskEvent)
}

// Repository 持久化安装记录、部署任务与目录元数据。实现：sqlite.AppDeployRepo。
type Repository interface {
	SaveInstallation(inst domain.Installation) error
	GetInstallation(hostID, appID string) (domain.Installation, error)
	ListInstallations(hostID string) ([]domain.Installation, error) // hostID 为空 = 全部
	DeleteInstallation(hostID, appID string) error
	SaveTask(task domain.DeployTask) error
	GetTask(id string) (domain.DeployTask, error)
	ListTasks(hostID string, limit int) ([]domain.DeployTask, error)
	SaveCatalogMeta(m domain.CatalogMeta) error
	GetCatalogMeta() (domain.CatalogMeta, error)
}

// RepoFunc 返回应用目录更新源仓库（"owner/repo"），从用户设置实时读取。
type RepoFunc func() string

// Service 是应用部署与管理的应用服务。
type Service struct {
	repo    Repository
	sess    SessionExecutor
	host    HostExecutor
	hosts   HostResolver
	creds   CredentialStore
	emit    EventEmitter
	repoFn  RepoFunc
	stream  StreamExecutor
	appsDir string

	mu      sync.Mutex
	running map[string]bool // hostID/appID → 防止同主机同应用并发任务
}

// NewService 装配应用部署服务。emit/creds/repoFn/hosts/tunnels 可为 nil（对应
// 能力降级：无事件推送/无凭证存储/用默认仓库/无面板地址渲染/无隧道按钮）。
func NewService(appsDir string, repo Repository, sess SessionExecutor, host HostExecutor, hosts HostResolver, creds CredentialStore, emit EventEmitter, repoFn RepoFunc, stream StreamExecutor) *Service {
	return &Service{
		appsDir: appsDir,
		repo:    repo,
		sess:    sess,
		host:    host,
		hosts:   hosts,
		creds:   creds,
		emit:    emit,
		repoFn:  repoFn,
		running: map[string]bool{},
	}
}

// SetEmitter 在 Wails 服务启动后注入事件发射器（构造时无法拿到 app 句柄）。
func (s *Service) SetEmitter(emit EventEmitter) { s.emit = emit }

// SetAppsDir 在数据目录迁移后重定向应用目录根。
func (s *Service) SetAppsDir(dir string) { s.appsDir = dir }

// ── 目录 ─────────────────────────────────────────────────────────────────

// RecoverInterrupted 在应用启动时收尾上次运行被打断的工作：仍处于
// installing 的安装记录与 running 的部署任务一律标记为失败（应用本体重启
// 即视为失败，可直接重新安装）；已成功的安装保持原状。
func (s *Service) RecoverInterrupted() {
	now := time.Now()
	if insts, err := s.repo.ListInstallations(""); err == nil {
		for _, in := range insts {
			if in.Status != domain.AppInstallStatusInstalling {
				continue // installed/failed 保持原状
			}
			in.Status = domain.AppInstallStatusFailed
			in.LastError = "应用重启导致安装中断，请重新安装"
			in.UpdatedAt = now
			_ = s.repo.SaveInstallation(in)
		}
	}
	tasks, err := s.repo.ListTasks("", 200)
	if err != nil {
		return
	}
	for i := range tasks {
		tk := &tasks[i]
		if tk.Status != domain.AppTaskRunning {
			continue
		}
		tk.Status = domain.AppTaskFailed
		tk.LastError = "应用重启导致任务中断，请重新安装"
		tk.FinishedAt = &now
		for j := range tk.Steps {
			if tk.Steps[j].Status == domain.AppStepRunning {
				tk.Steps[j].Status = domain.AppStepFailed
				tk.Steps[j].Error = "应用重启导致中断"
				tk.Steps[j].FinishedAt = &now
			}
		}
		_ = s.repo.SaveTask(*tk)
	}
}

// ListCatalog 返回内置（磁盘 catalog/）+ 用户自建（custom/）的应用目录，
// 按 ID 排序。目录在启动时已由 SeedBuiltinCatalog 种子解压。
func (s *Service) ListCatalog() ([]domain.CatalogEntry, error) {
	out := make([]domain.CatalogEntry, 0, 8)
	for _, source := range []string{catalogDirName, customDirName} {
		base := filepath.Join(s.appsDir, source)
		dirs, err := listSpecDirs(base)
		if err != nil {
			return nil, fmt.Errorf("枚举 %s: %w", source, err)
		}
		for _, name := range dirs {
			spec, err := loadSpecFromDir(filepath.Join(base, name))
			if err != nil {
				continue // 畸形条目跳过，不破坏整个目录
			}
			out = append(out, domain.CatalogEntry{Spec: spec, Source: map[string]string{catalogDirName: domain.AppCatalogSourceBuiltin, customDirName: "custom"}[source]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.ID < out[j].Spec.ID })
	return out, nil
}

// GetApp 返回单个应用（先 custom 后 catalog，custom 可覆盖内置同名条目）。
func (s *Service) GetApp(id string) (domain.CatalogEntry, error) {
	for _, source := range []string{customDirName, catalogDirName} {
		dir := filepath.Join(s.appsDir, source, id)
		if _, err := os.Stat(filepath.Join(dir, "app.json")); err != nil {
			continue
		}
		spec, err := loadSpecFromDir(dir)
		if err != nil {
			return domain.CatalogEntry{}, fmt.Errorf("加载应用 %s: %w", id, err)
		}
		src := domain.AppCatalogSourceBuiltin
		if source == customDirName {
			src = "custom"
		}
		return domain.CatalogEntry{Spec: spec, Source: src}, nil
	}
	return domain.CatalogEntry{}, fmt.Errorf("应用 %q 不存在", id)
}

// Guide 返回应用指南内容（人读/agent 读）。
func (s *Service) Guide(id string) (string, error) {
	for _, source := range []string{customDirName, catalogDirName} {
		raw, err := os.ReadFile(filepath.Join(s.appsDir, source, id, "guide.md"))
		if err == nil {
			return string(raw), nil
		}
	}
	return "", fmt.Errorf("应用 %q 无指南文档", id)
}

// Icon 返回应用图标的原始字节（保持官方原图格式：SVG/PNG/ICO 等均可）。
func (s *Service) Icon(id, file string) ([]byte, error) {
	if file == "" || strings.Contains(file, "/") || strings.Contains(file, "\\") || strings.Contains(file, "..") {
		return nil, fmt.Errorf("非法图标文件名 %q", file)
	}
	for _, source := range []string{customDirName, catalogDirName} {
		raw, err := os.ReadFile(filepath.Join(s.appsDir, source, id, file))
		if err == nil {
			return raw, nil
		}
	}
	return nil, os.ErrNotExist
}

// IconDataURL 把图标字节包装成 data URL（按扩展名推断 MIME，base64 编码），
// 前端直接用作 <img src>；二进制格式（PNG/ICO 等）不经过文本通道。
func IconDataURL(file string, raw []byte) string {
	var mime string
	switch strings.ToLower(filepath.Ext(file)) {
	case ".svg":
		mime = "image/svg+xml"
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".ico":
		mime = "image/x-icon"
	case ".webp":
		mime = "image/webp"
	case ".gif":
		mime = "image/gif"
	default:
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// CatalogVersion 返回数据目录中目录的当前版本与来源。
func (s *Service) CatalogVersion() (int, domain.CatalogMeta, error) {
	version := readDiskCatalogVersion(filepath.Join(s.appsDir, catalogDirName))
	meta, _ := s.repo.GetCatalogMeta()
	if meta.Version == 0 {
		meta.Version = version
	}
	return version, meta, nil
}

// ── 部署任务 ─────────────────────────────────────────────────────────────

// DetectInstalled 在会话内执行 install.detect，返回是否已安装。
func (s *Service) DetectInstalled(ctx context.Context, sessionID, appID string) (bool, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return false, err
	}
	out, err := s.runSession(ctx, sessionID, entry.Spec.Install.Detect, 30*time.Second)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "yes", nil
}

// StartInstall 启动一次安装任务：校验输入 → 渲染步骤 → 落库 → 异步执行。
// 返回创建时的任务快照；进度经事件 "apps:task" 推送，也可轮询 GetTask。
func (s *Service) StartInstall(ctx context.Context, sessionID, hostID, appID string, inputs map[string]string) (domain.DeployTask, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return domain.DeployTask{}, err
	}
	if err := ValidateInputs(&entry.Spec, inputs); err != nil {
		return domain.DeployTask{}, err
	}
	steps := make([]domain.DeployStep, len(entry.Spec.Install.Steps))
	copy(steps, entry.Spec.Install.Steps)
	if entry.Spec.Install.Verify != "" {
		steps = append(steps, domain.DeployStep{Name: "安装后验证", Command: entry.Spec.Install.Verify, TimeoutSec: 60})
	}
	return s.startTask(ctx, sessionID, hostID, entry.Spec, domain.AppTaskKindInstall, steps, inputs)
}

// StartUninstall 启动卸载任务（ops.uninstall 步骤）。
func (s *Service) StartUninstall(ctx context.Context, sessionID, hostID, appID string) (domain.DeployTask, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return domain.DeployTask{}, err
	}
	if len(entry.Spec.Ops.Uninstall) == 0 {
		return domain.DeployTask{}, fmt.Errorf("应用 %s 未定义卸载步骤", appID)
	}
	return s.startTask(ctx, sessionID, hostID, entry.Spec, domain.AppTaskKindUninstall, entry.Spec.Ops.Uninstall, nil)
}

// StartUpdate 启动升级任务（单步 ops.update）。
func (s *Service) StartUpdate(ctx context.Context, sessionID, hostID, appID string) (domain.DeployTask, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return domain.DeployTask{}, err
	}
	if entry.Spec.Ops.Update == "" {
		return domain.DeployTask{}, fmt.Errorf("应用 %s 未定义升级命令", appID)
	}
	steps := []domain.DeployStep{{Name: "升级 " + entry.Spec.Name, Command: entry.Spec.Ops.Update, TimeoutSec: 900}}
	return s.startTask(ctx, sessionID, hostID, entry.Spec, domain.AppTaskKindUpdate, steps, nil)
}

// startTask 创建任务并异步执行。
func (s *Service) startTask(ctx context.Context, sessionID, hostID string, spec domain.AppSpec, kind string, steps []domain.DeployStep, inputs map[string]string) (domain.DeployTask, error) {
	if s.sess == nil {
		return domain.DeployTask{}, errors.New("应用操作需要已打开的会话")
	}
	key := hostID + "/" + spec.ID
	s.mu.Lock()
	if s.running[key] {
		s.mu.Unlock()
		return domain.DeployTask{}, fmt.Errorf("应用 %s 在该主机上已有任务执行中", spec.ID)
	}
	s.running[key] = true
	s.mu.Unlock()

	vars := map[string]string{"port": strconvItoa(panelPort(&spec))}
	for k, v := range inputs {
		vars[k] = v
	}

	now := time.Now()
	task := domain.DeployTask{
		ID:        newID("task"),
		HostID:    hostID,
		SessionID: sessionID,
		AppID:     spec.ID,
		Kind:      kind,
		Status:    domain.AppTaskRunning,
		StartedAt: now,
	}
	for i, st := range steps {
		task.Steps = append(task.Steps, domain.DeployStepRun{
			StepID:    fmt.Sprintf("s%d", i+1),
			Name:      st.Name,
			Status:    domain.AppStepPending,
			StartedAt: now,
		})
	}
	if kind == domain.AppTaskKindInstall {
		_ = s.repo.SaveInstallation(domain.Installation{
			HostID: hostID, AppID: spec.ID,
			Status: domain.AppInstallStatusInstalling,
			Port:   panelPort(&spec),
		})
	}
	if err := s.repo.SaveTask(task); err != nil {
		s.mu.Lock()
		delete(s.running, key)
		s.mu.Unlock()
		return domain.DeployTask{}, err
	}

	go s.runTask(task, spec, kind, steps, vars, inputs, key)
	return task, nil
}

// runTask 顺序执行任务步骤（写操作一律走会话通道），逐步落库+推送事件。
func (s *Service) runTask(task domain.DeployTask, spec domain.AppSpec, kind string, steps []domain.DeployStep, vars map[string]string, inputs map[string]string, runningKey string) {
	defer func() {
		s.mu.Lock()
		delete(s.running, runningKey)
		s.mu.Unlock()
	}()
	ctx := context.Background()
	ok := true
	for i := range steps {
		step := &task.Steps[i]
		cmd, err := RenderCommand(steps[i].Command, vars)
		if err != nil {
			s.setStep(&task, step, domain.AppStepFailed, -1, err.Error(), &steps[i])
			ok = false
			break
		}
		// 渲染后的最终命令（变量已替换为实际值）随步骤落库并推送到面板展示。
		step.Command = cmd
		s.setStep(&task, step, domain.AppStepRunning, 0, "", nil)
		timeout := defaultStepTimeout
		if steps[i].TimeoutSec > 0 {
			timeout = time.Duration(steps[i].TimeoutSec) * time.Second
		}
		// 流式执行（可用时）：输出按块实时写入步骤并推送面板——长时间安装器
		// 不再表现为"卡住无输出"。onChunk 只累积，500ms 节流刷新落库/推送。
		var (
			out      string
			execErr  error
			exitCode int
			mu       sync.Mutex
			pending  strings.Builder
		)
		if s.stream != nil && task.SessionID != "" {
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			type streamRes struct {
				out  string
				code int
				err  error
			}
			resCh := make(chan streamRes, 1)
			go func() {
				o, c, e := s.stream.ExecInSessionStream(cctx, task.SessionID, scriptExec(cmd), func(chunk string) {
					mu.Lock()
					pending.WriteString(chunk)
					mu.Unlock()
				})
				resCh <- streamRes{o, c, e}
			}()
			ticker := time.NewTicker(500 * time.Millisecond)
		streamLoop:
			for {
				select {
				case r := <-resCh:
					out, exitCode, execErr = r.out, r.code, r.err
					ticker.Stop()
					break streamLoop
				case <-ticker.C:
					mu.Lock()
					if pending.Len() == 0 {
						mu.Unlock()
						continue
					}
					step.Output = capOutput(pending.String())
					pending.Reset()
					_ = s.repo.SaveTask(task)
					s.emitTask(task, step)
					mu.Unlock()
				case <-cctx.Done():
					// ExecInSessionStream 的看门狗会终止远端命令并从 resCh
					// 返回；这里仅兜底。
				}
			}
			if execErr != nil {
				exitCode = exitCodeOf(execErr)
			}
		} else {
			out, execErr = s.runSession(ctx, task.SessionID, cmd, timeout)
			if execErr != nil {
				exitCode = exitCodeOf(execErr)
			}
		}
		if execErr != nil {
			// 失败步骤保存完整的远端输出（stderr 已合并）+ 退出错误，面板能
			// 直接看到错误流而不是只有 "exit status N"。
			msg := strings.TrimSpace(out)
			if msg != "" {
				msg += "\n"
			}
			msg += execErr.Error()
			s.setStep(&task, step, domain.AppStepFailed, exitCode, msg, &steps[i])
			_ = s.repo.SaveTask(task)
			s.emitTask(task, step)
			ok = false
			break
		}
		s.setStep(&task, step, domain.AppStepSucceeded, 0, out, &steps[i])
		s.emitTask(task, step)
	}

	now := time.Now()
	task.FinishedAt = &now
	if ok {
		task.Status = domain.AppTaskSucceeded
	} else {
		task.Status = domain.AppTaskFailed
		task.LastError = lastStepError(task)
	}
	_ = s.repo.SaveTask(task)
	s.emitTask(task, nil)
	s.syncInstallation(task, spec, kind, ok)
	if ok && kind == domain.AppTaskKindInstall {
		s.saveInstallCredentials(task, spec, inputs)
	}
}

// saveInstallCredentials 把安装时输入的凭据类参数（AppInput.CredentialKey）
// 自动写为面板登录凭证：用户在向导里设置的初始化密码，打开"登录凭证"时
// 即是该值，无需二次输入。非密码字段（用户名等）由面板凭证默认值提供。
func (s *Service) saveInstallCredentials(task domain.DeployTask, spec domain.AppSpec, inputs map[string]string) {
	if s.creds == nil {
		return
	}
	values := map[string]string{}
	for _, in := range spec.Install.Inputs {
		if in.CredentialKey == "" {
			continue
		}
		if v := inputs[in.Key]; v != "" {
			values[in.CredentialKey] = v
		}
	}
	if len(values) == 0 {
		return
	}
	// 合并已有凭证（用户名等非安装输入的字段保持原值）。
	if existing, err := s.creds.GetPanelCredential(task.HostID, spec.ID); err == nil {
		for k, v := range existing {
			if _, ok := values[k]; !ok && v != "" {
				values[k] = v
			}
		}
	}
	_ = s.creds.SavePanelCredential(task.HostID, spec.ID, values)
}

// setStep 更新步骤状态并落库（输出统一截断+脱敏）。
func (s *Service) setStep(task *domain.DeployTask, step *domain.DeployStepRun, status string, exitCode int, outputOrErr string, src *domain.DeployStep) {
	now := time.Now()
	step.Status = status
	step.ExitCode = exitCode
	if status == domain.AppStepRunning {
		step.StartedAt = now
	} else {
		step.FinishedAt = &now
	}
	if status == domain.AppStepSucceeded {
		step.Output = capOutput(outputOrErr)
	} else if status == domain.AppStepFailed {
		step.Error = capOutput(outputOrErr)
	}
	_ = s.repo.SaveTask(*task)
	if status != domain.AppStepRunning {
		s.emitTask(*task, step)
	}
}

// syncInstallation 在任务结束后同步安装记录。
func (s *Service) syncInstallation(task domain.DeployTask, spec domain.AppSpec, kind string, ok bool) {
	existing, err := s.repo.GetInstallation(task.HostID, task.AppID)
	if err != nil {
		existing = domain.Installation{HostID: task.HostID, AppID: task.AppID}
	}
	switch kind {
	case domain.AppTaskKindInstall:
		now := time.Now()
		if ok {
			existing.Status = domain.AppInstallStatusInstalled
			existing.InstalledAt = now
			existing.Port = panelPort(&spec)
			existing.LastError = ""
		} else {
			existing.Status = domain.AppInstallStatusFailed
			existing.LastError = task.LastError
		}
		_ = s.repo.SaveInstallation(existing)
	case domain.AppTaskKindUninstall:
		if ok {
			_ = s.repo.DeleteInstallation(task.HostID, task.AppID)
			if s.creds != nil {
				_ = s.creds.DeletePanelCredential(task.HostID, task.AppID)
			}
		} else {
			existing.Status = domain.AppInstallStatusFailed
			existing.LastError = task.LastError
			_ = s.repo.SaveInstallation(existing)
		}
	case domain.AppTaskKindUpdate:
		if ok {
			existing.Status = domain.AppInstallStatusInstalled
			existing.LastError = ""
			_ = s.repo.SaveInstallation(existing)
		}
	}
}

// GetTask / ListTasks 暴露任务查询。
func (s *Service) GetTask(id string) (domain.DeployTask, error) { return s.repo.GetTask(id) }
func (s *Service) ListTasks(hostID string, limit int) ([]domain.DeployTask, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListTasks(hostID, limit)
}

// ListInstallations 返回安装记录（hostID 为空 = 全部主机）。
func (s *Service) ListInstallations(hostID string) ([]domain.Installation, error) {
	return s.repo.ListInstallations(hostID)
}

// ── 监控与控制 ───────────────────────────────────────────────────────────

// Status 在会话内执行 ops.status 并解析契约。
func (s *Service) Status(ctx context.Context, sessionID, appID string) (domain.HostAppStatus, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return domain.HostAppStatus{}, err
	}
	out, err := s.runSession(ctx, sessionID, entry.Spec.Ops.Status, 30*time.Second)
	if err != nil && out == "" {
		return domain.HostAppStatus{}, err
	}
	return ParseHostAppStatus(out), nil
}

// StatusByHost 用一次性连接执行 ops.status（独立菜单页的总览轮询，只读）。
func (s *Service) StatusByHost(ctx context.Context, hostID, appID string) (domain.HostAppStatus, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return domain.HostAppStatus{}, err
	}
	out, _, err := s.runHost(ctx, hostID, entry.Spec.Ops.Status, 30*time.Second)
	if err != nil && out == "" {
		return domain.HostAppStatus{}, err
	}
	return ParseHostAppStatus(out), nil
}

// Logs 返回最近 lines 行日志（会话通道）。
func (s *Service) Logs(ctx context.Context, sessionID, appID string, lines int) (string, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return "", err
	}
	lines = clampLines(lines)
	cmd, err := RenderCommand(entry.Spec.Ops.Logs, map[string]string{"lines": strconvItoa(lines)})
	if err != nil {
		return "", err
	}
	out, err := s.runSession(ctx, sessionID, cmd, 30*time.Second)
	return capBytes(out, logByteCap), err
}

// LogsByHost 返回最近 lines 行日志（一次性连接，只读）。
func (s *Service) LogsByHost(ctx context.Context, hostID, appID string, lines int) (string, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return "", err
	}
	lines = clampLines(lines)
	cmd, err := RenderCommand(entry.Spec.Ops.Logs, map[string]string{"lines": strconvItoa(lines)})
	if err != nil {
		return "", err
	}
	out, _, err := s.runHost(ctx, hostID, cmd, 30*time.Second)
	return capBytes(out, logByteCap), err
}

// controlActions 是允许的控制动作（映射到 ops 同名字段）。
var controlActions = map[string]func(domain.AppOps) string{
	"start":   func(o domain.AppOps) string { return o.Start },
	"stop":    func(o domain.AppOps) string { return o.Stop },
	"restart": func(o domain.AppOps) string { return o.Restart },
}

// Control 执行启动/停止/重启（会话通道）。
func (s *Service) Control(ctx context.Context, sessionID, appID, action string) (string, error) {
	get, ok := controlActions[action]
	if !ok {
		return "", fmt.Errorf("不支持的控制动作 %q", action)
	}
	entry, err := s.GetApp(appID)
	if err != nil {
		return "", err
	}
	cmd := get(entry.Spec.Ops)
	if cmd == "" {
		return "", fmt.Errorf("应用 %s 未定义 %s 命令", appID, action)
	}
	return s.runSession(ctx, sessionID, cmd, 120*time.Second)
}

// OpenPort 执行 ops.openPort（默认面板端口）。
func (s *Service) OpenPort(ctx context.Context, sessionID, appID string) (string, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return "", err
	}
	if entry.Spec.Ops.OpenPort == "" {
		return "", fmt.Errorf("应用 %s 未定义开放端口命令", appID)
	}
	cmd, err := RenderCommand(entry.Spec.Ops.OpenPort, map[string]string{"port": strconvItoa(panelPort(&entry.Spec))})
	if err != nil {
		return "", err
	}
	return s.runSession(ctx, sessionID, cmd, 120*time.Second)
}

// ── 面板与凭证 ───────────────────────────────────────────────────────────

// PanelURL 渲染面板地址（{{.host}} 用主机记录中的地址）。
func (s *Service) PanelURL(hostID, appID string) (string, error) {
	entry, err := s.GetApp(appID)
	if err != nil {
		return "", err
	}
	if entry.Spec.Panel == nil || entry.Spec.Panel.URL == "" {
		return "", fmt.Errorf("应用 %s 无 Web 面板", appID)
	}
	hostAddr := ""
	if s.hosts != nil {
		if h, err := s.hosts.Get(hostID); err == nil {
			hostAddr = h.Host
		}
	}
	// URL 是展示用文本，绝不能走 shell 引号转义（否则出现 '172.18.x.x'）。
	return RenderTemplate(entry.Spec.Panel.URL, map[string]string{"host": hostAddr, "port": strconvItoa(entry.Spec.Panel.Port)})
}

// SavePanelCredential 保存面板登录凭证（OS 钥匙串，按 主机+应用 一条）。
func (s *Service) SavePanelCredential(hostID, appID string, values map[string]string) error {
	if s.creds == nil {
		return errors.New("凭证存储不可用")
	}
	return s.creds.SavePanelCredential(hostID, appID, values)
}

// GetPanelCredential 读取面板登录凭证（自动补全面板地址）。
func (s *Service) GetPanelCredential(hostID, appID string) (map[string]string, error) {
	if s.creds == nil {
		return nil, errors.New("凭证存储不可用")
	}
	return s.creds.GetPanelCredential(hostID, appID)
}

// ── 执行通道 ─────────────────────────────────────────────────────────────

// scriptExec 把脚本包装成远端执行命令：
//   - `bash -l`：登录 shell 加载 profile，保证 ~/.local/bin 等安装脚本追加的
//     PATH 生效；脚本整体单引号转义经 argv 传入（heredoc/引号无需二次转义）；
//   - `stdbuf -oL -eL`：非 TTY 下安装器的 stdout 默认 4KB 块缓冲，输出会攒到
//     结束才到达（面板表现为"卡住"），行缓冲让输出逐行实时流出；stdbuf 缺失
//     （macOS）时原样执行；
//   - `PYTHONUNBUFFERED=1`：Python 系安装器（octop/hermes）自带缓冲；
//   - `</dev/null`：stdin 立即 EOF，交互提示（如 openclaw 的 nvm 询问）按默认
//     值继续或干净失败，既不阻塞也不吞掉脚本剩余部分。
func scriptExec(cmd string) string {
	quoted := shellQuote(cmd)
	return "export PYTHONUNBUFFERED=1; if command -v stdbuf >/dev/null 2>&1; then exec stdbuf -oL -eL bash -l -c " +
		quoted + " </dev/null; else exec bash -l -c " + quoted + " </dev/null; fi"
}

// runSession 在会话通道上执行命令脚本（带超时；非零退出 = error）。
func (s *Service) runSession(ctx context.Context, sessionID, cmd string, timeout time.Duration) (string, error) {
	if s.sess == nil {
		return "", errors.New("无会话执行通道")
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.sess.ExecInSessionCtx(cctx, sessionID, scriptExec(cmd))
}

// runHost 在一次性连接上执行命令脚本（只读总览），执行方式同 runSession。
func (s *Service) runHost(ctx context.Context, hostID, cmd string, timeout time.Duration) (string, int, error) {
	if s.host == nil {
		return "", -1, errors.New("无主机执行通道")
	}
	return s.host.Exec(ctx, hostID, scriptExec(cmd), nil, timeout)
}

func (s *Service) emitTask(task domain.DeployTask, step *domain.DeployStepRun) {
	if s.emit == nil {
		return
	}
	s.emit.EmitTaskEvent(domain.DeployTaskEvent{
		TaskID: task.ID, HostID: task.HostID, AppID: task.AppID,
		Kind: task.Kind, Status: task.Status, Step: step, LastError: task.LastError,
	})
}

// ── 小工具 ───────────────────────────────────────────────────────────────

func panelPort(spec *domain.AppSpec) int {
	if spec.Panel != nil {
		return spec.Panel.Port
	}
	return 0
}

func clampLines(n int) int {
	if n <= 0 {
		return 200
	}
	if n > 1000 {
		return 1000
	}
	return n
}

func strconvItoa(n int) string { return strconv.Itoa(n) }

// exitCodeOf 从 ExecInSessionCtx 的 error 中提取远端退出码。
func exitCodeOf(err error) int {
	var exitErr interface{ ExitStatus() int }
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus()
	}
	return -1
}

// lastStepError 返回最后一个失败步骤的摘要。
func lastStepError(task domain.DeployTask) string {
	for i := len(task.Steps) - 1; i >= 0; i-- {
		if task.Steps[i].Status == domain.AppStepFailed {
			name := task.Steps[i].Name
			msg := task.Steps[i].Error
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return fmt.Sprintf("步骤「%s」失败: %s", name, msg)
		}
	}
	return ""
}

// sensitiveRe 匹配常见敏感赋值行，输出落库前打码。
var sensitiveRe = regexp.MustCompile(`(?i)((password|passwd|secret|token|api[_-]?key)\s*[=:]\s*)\S+`)

// capOutput 截断 + 脱敏一步输出。
func capOutput(s string) string {
	s = sensitiveRe.ReplaceAllString(s, "$1********")
	return capBytes(s, stepOutputCap)
}

// capBytes 截断到 max 字节（保留头部，中置省略标记）。
func capBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - 64
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut-- // 不切断 UTF-8 序列
	}
	return s[:cut] + fmt.Sprintf("\n…[%d bytes truncated]…\n", len(s)-cut)
}

// newID 生成带前缀的随机 ID。
func newID(prefix string) string {
	return prefix + "-" + randomHex(8)
}

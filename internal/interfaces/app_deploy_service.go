package interfaces

import (
	"context"
	"time"

	wailsapp "github.com/wailsapp/wails/v3/pkg/application"

	appdeploy "github.com/ai-remote/workspace/internal/application/appdeploy"
	"github.com/ai-remote/workspace/internal/domain"
)

// ── AppDeployService -------------------------------------------------------

// AppDeployService 暴露应用部署与管理（应用目录/安装/监控/控制/目录更新）
// 给前端。写操作（安装/卸载/升级/启停/开放端口）通过已打开的终端会话执行；
// 只读总览（状态/日志）支持按主机一次性连接执行。任务进度经 "apps:task"
// 事件推送。
type AppDeployService struct {
	svc *appdeploy.Service
	app *wailsapp.App
}

// NewAppDeployService wires the Wails service to the application service.
func NewAppDeployService(svc *appdeploy.Service) *AppDeployService {
	return &AppDeployService{svc: svc}
}

// ServiceName lets Wails register the service under a stable name.
func (d *AppDeployService) ServiceName() string { return "AppDeployService" }

// ServiceStartup captures the Application handle and wires the task emitter.
func (d *AppDeployService) ServiceStartup(_ context.Context, _ wailsapp.ServiceOptions) error {
	d.app = wailsapp.Get()
	d.svc.SetEmitter(d)
	// 上次运行中被打断的安装/任务（应用重启）标记为失败，允许重新安装。
	d.svc.RecoverInterrupted()
	return nil
}

// EmitTaskEvent 把任务事件桥接到 Wails 事件总线 "apps:task"（实现
// appdeploy.EventEmitter；ServiceStartup 时注入应用服务）。
func (d *AppDeployService) EmitTaskEvent(ev domain.DeployTaskEvent) {
	if d.app != nil {
		d.app.Event.Emit("apps:task", ev)
	}
}

// deadline 是一次性远程命令的通用超时（状态/日志类）。
func deadline(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

// ── 目录 ─────────────────────────────────────────────────────────────────

// ListCatalog 返回应用目录（内置 + 用户自建）。
func (d *AppDeployService) ListCatalog() ([]domain.CatalogEntry, error) {
	return d.svc.ListCatalog()
}

// GetApp 返回单个应用定义。
func (d *AppDeployService) GetApp(id string) (domain.CatalogEntry, error) {
	return d.svc.GetApp(id)
}

// GetGuide 返回应用指南（Markdown 文本）。
func (d *AppDeployService) GetGuide(id string) (string, error) {
	return d.svc.Guide(id)
}

// GetIcon 返回应用图标的 data URL（保持官方原图格式，按扩展名推断 MIME，
// base64 编码；前端直接用作 <img src>）。
func (d *AppDeployService) GetIcon(id, file string) (string, error) {
	raw, err := d.svc.Icon(id, file)
	if err != nil {
		return "", err
	}
	return appdeploy.IconDataURL(file, raw), nil
}

// GetCatalogVersion 返回目录当前版本与元数据（来源/同步时间）。
func (d *AppDeployService) GetCatalogVersion() (int, domain.CatalogMeta, error) {
	return d.svc.CatalogVersion()
}

// CheckCatalogUpdate 检查目录更新（source: "release" | "main"）。
func (d *AppDeployService) CheckCatalogUpdate(source string) (domain.CatalogUpdateCheck, error) {
	ctx, cancel := deadline(60)
	defer cancel()
	return d.svc.CheckCatalogUpdate(ctx, source)
}

// ApplyCatalogUpdate 应用目录更新（下载→校验→落盘）。
func (d *AppDeployService) ApplyCatalogUpdate(source string) (domain.CatalogMeta, error) {
	ctx, cancel := deadline(180)
	defer cancel()
	return d.svc.ApplyCatalogUpdate(ctx, source)
}

// ── 安装与任务 ───────────────────────────────────────────────────────────

// DetectInstalled 检测应用是否已安装在会话主机上。
func (d *AppDeployService) DetectInstalled(sessionID, appID string) (bool, error) {
	ctx, cancel := deadline(45)
	defer cancel()
	return d.svc.DetectInstalled(ctx, sessionID, appID)
}

// StartInstall 启动安装任务（异步执行，进度走 "apps:task" 事件）。
func (d *AppDeployService) StartInstall(sessionID, hostID, appID string, inputs map[string]string) (domain.DeployTask, error) {
	return d.svc.StartInstall(context.Background(), sessionID, hostID, appID, inputs)
}

// StartUninstall 启动卸载任务。
func (d *AppDeployService) StartUninstall(sessionID, hostID, appID string) (domain.DeployTask, error) {
	return d.svc.StartUninstall(context.Background(), sessionID, hostID, appID)
}

// StartUpdate 启动升级任务（执行 ops.update）。
func (d *AppDeployService) StartUpdate(sessionID, hostID, appID string) (domain.DeployTask, error) {
	return d.svc.StartUpdate(context.Background(), sessionID, hostID, appID)
}

// GetTask 返回一个部署任务（含步骤输出）。
func (d *AppDeployService) GetTask(id string) (domain.DeployTask, error) {
	return d.svc.GetTask(id)
}

// ListTasks 返回部署任务列表（hostID 为空 = 全部）。
func (d *AppDeployService) ListTasks(hostID string, limit int) ([]domain.DeployTask, error) {
	return d.svc.ListTasks(hostID, limit)
}

// ListInstallations 返回安装记录（hostID 为空 = 全部主机，供总览页）。
func (d *AppDeployService) ListInstallations(hostID string) ([]domain.Installation, error) {
	return d.svc.ListInstallations(hostID)
}

// ── 监控与控制 ───────────────────────────────────────────────────────────

// GetStatus 返回会话主机的应用状态（ops.status 契约解析）。
func (d *AppDeployService) GetStatus(sessionID, appID string) (domain.HostAppStatus, error) {
	ctx, cancel := deadline(45)
	defer cancel()
	return d.svc.Status(ctx, sessionID, appID)
}

// GetStatusByHost 按主机（一次性连接）返回应用状态（总览页轮询）。
func (d *AppDeployService) GetStatusByHost(hostID, appID string) (domain.HostAppStatus, error) {
	ctx, cancel := deadline(45)
	defer cancel()
	return d.svc.StatusByHost(ctx, hostID, appID)
}

// GetLogs 返回会话主机上应用的最近日志。
func (d *AppDeployService) GetLogs(sessionID, appID string, lines int) (string, error) {
	ctx, cancel := deadline(45)
	defer cancel()
	return d.svc.Logs(ctx, sessionID, appID, lines)
}

// GetLogsByHost 按主机（一次性连接）返回应用日志。
func (d *AppDeployService) GetLogsByHost(hostID, appID string, lines int) (string, error) {
	ctx, cancel := deadline(45)
	defer cancel()
	return d.svc.LogsByHost(ctx, hostID, appID, lines)
}

// Control 执行启动/停止/重启（action: start|stop|restart）。
func (d *AppDeployService) Control(sessionID, appID, action string) (string, error) {
	ctx, cancel := deadline(150)
	defer cancel()
	return d.svc.Control(ctx, sessionID, appID, action)
}

// OpenPort 在会话主机上放行应用面板端口。
func (d *AppDeployService) OpenPort(sessionID, appID string) (string, error) {
	ctx, cancel := deadline(150)
	defer cancel()
	return d.svc.OpenPort(ctx, sessionID, appID)
}

// ── 面板与凭证 ───────────────────────────────────────────────────────────

// GetPanelURL 渲染应用面板地址。
func (d *AppDeployService) GetPanelURL(hostID, appID string) (string, error) {
	return d.svc.PanelURL(hostID, appID)
}

// SavePanelCredential 保存面板登录凭证（OS 钥匙串）。
func (d *AppDeployService) SavePanelCredential(hostID, appID string, values map[string]string) error {
	return d.svc.SavePanelCredential(hostID, appID, values)
}

// GetPanelCredential 读取面板登录凭证。
func (d *AppDeployService) GetPanelCredential(hostID, appID string) (map[string]string, error) {
	return d.svc.GetPanelCredential(hostID, appID)
}

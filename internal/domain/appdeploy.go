package domain

import "time"

// ── 应用部署（应用目录与操作手册）领域模型 ────────────────────────────────
//
// app.json 是权威执行源：UI 支持的每个操作（安装/启动/停止/重启/状态/日志/
// 开放端口/升级/卸载）都映射到具体 shell 命令，经 SSH 执行；guide.md 是同一
// 批命令的人读/agent 读指南，不参与解析。规范见 docs/APP_CATALOG.md。

// AppSpecSchemaVersion 是 app.json 当前遵循的模式版本。
const AppSpecSchemaVersion = 1

// 安装记录 / 部署任务状态常量。
const (
	// Installation status.
	AppInstallStatusInstalling = "installing"
	AppInstallStatusInstalled  = "installed"
	AppInstallStatusFailed     = "failed"

	// Deploy task status.
	AppTaskRunning   = "running"
	AppTaskSucceeded = "succeeded"
	AppTaskFailed    = "failed"

	// Deploy task kinds.
	AppTaskKindInstall   = "install"
	AppTaskKindUninstall = "uninstall"
	AppTaskKindUpdate    = "update"

	// Step run status.
	AppStepPending   = "pending"
	AppStepRunning   = "running"
	AppStepSucceeded = "succeeded"
	AppStepFailed    = "failed"

	// Step risk levels (display only; execution is always explicit).
	AppRiskWrite = "write"
	AppRiskHigh  = "high"

	// HostAppStatus.State values (parsed from the ops.status contract).
	AppStateRunning = "running"
	AppStateStopped = "stopped"
	AppStateFailed  = "failed"
	AppStateUnknown = "unknown"

	// Catalog sources.
	AppCatalogSourceBuiltin = "builtin"
	AppCatalogSourceRelease = "release"
	AppCatalogSourceMain    = "main"
)

// AppSpec 是单个应用的操作手册（app.json 反序列化目标）。所有应用实现几乎
// 相同的操作集；模板占位符 {{.key}} 在执行前渲染，值一律经 shell 引用转义。
type AppSpec struct {
	SchemaVersion  int      `json:"schemaVersion"`
	ID             string   `json:"id"` // slug，目录唯一键
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Website        string   `json:"website"`        // 官网（GitHub 等）
	Icon           string   `json:"icon,omitempty"` // 应用目录内的图标文件名
	Tags           []string `json:"tags,omitempty"`
	CatalogVersion int      `json:"catalogVersion"` // 条目版本（种子覆盖判断）

	Install AppInstall `json:"install"`
	Ops     AppOps     `json:"ops"`
	Panel   *AppPanel  `json:"panel,omitempty"`
}

// AppInstall 固化安装流程：检测 → 步骤 → 验证。
type AppInstall struct {
	// Detect 已安装检测命令：输出 yes = 已安装，其余 = 未安装。
	Detect string `json:"detect"`
	// Notice 安装前提示（展示在向导参数页，如"将覆盖已有安装"）。
	Notice string       `json:"notice,omitempty"`
	Inputs []AppInput   `json:"inputs,omitempty"`
	Steps  []DeployStep `json:"steps"`
	// Verify 安装后验证命令（退出码 0 = 成功）。
	Verify string `json:"verify,omitempty"`
}

// AppInput 是部署前需要用户提供的参数。Type: text | secret | number。
type AppInput struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	// Validate 目前仅支持 "min=N"（最短长度，用于密码类输入）。
	Validate string `json:"validate,omitempty"`
	// CredentialKey 非空时，该输入值在安装成功后自动保存为应用面板登录凭证
	// 的这一字段（如 adminPassword → password），用户无需再次手填。
	CredentialKey string `json:"credentialKey,omitempty"`
}

// DeployStep 是一步固化命令。Risk 仅用于 UI 展示与确认页提示。
type DeployStep struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	TimeoutSec int    `json:"timeoutSec,omitempty"` // 0 = 默认 600s
	Risk       string `json:"risk,omitempty"`       // write | high
}

// AppOps 统一操作集：UI 每个按钮/指标一一映射到这里的具体命令。
type AppOps struct {
	Start   string `json:"start,omitempty"`
	Stop    string `json:"stop,omitempty"`
	Restart string `json:"restart,omitempty"`
	// Status 状态聚合脚本：输出标准化 KEY=VALUE 契约（见 ParseHostAppStatus）。
	Status string `json:"status"`
	// Logs 日志命令，模板变量 {{.lines}}（默认 200）。
	Logs string `json:"logs"`
	// OpenPort 开放端口命令，模板变量 {{.port}}。
	OpenPort  string       `json:"openPort,omitempty"`
	Version   string       `json:"version,omitempty"`
	Update    string       `json:"update,omitempty"`
	Uninstall []DeployStep `json:"uninstall,omitempty"`
}

// AppPanel 描述应用的 Web 面板与登录凭证。无 Web 面板的应用整个字段为空，
// UI 隐藏"打开面板/登录凭证"。
type AppPanel struct {
	// URL 面板地址模板，可用变量 {{.host}}（主机地址）与 {{.port}}。
	URL         string          `json:"url,omitempty"`
	Port        int             `json:"port,omitempty"`
	HealthPath  string          `json:"healthPath,omitempty"`
	Credentials []AppCredential `json:"credentials,omitempty"`
}

// AppCredential 是面板登录凭证的一个字段。Secret 字段存 OS 钥匙串，
// 非密码字段（如用户名）随安装记录存 SQLite。
type AppCredential struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Secret bool   `json:"secret,omitempty"`
	// Default 是未保存凭证时的默认值（如固定用户名 admin）。
	Default string `json:"default,omitempty"`
}

// CatalogEntry = AppSpec + 目录来源（列表接口返回）。
type CatalogEntry struct {
	Spec   AppSpec `json:"spec"`
	Source string  `json:"source"` // builtin | custom
}

// Catalog 是 catalog.json 的结构（目录索引 + 版本）。
type Catalog struct {
	CatalogVersion int          `json:"catalogVersion"`
	Apps           []CatalogRef `json:"apps"`
}

// CatalogRef 是 catalog.json 里的应用索引条目。
type CatalogRef struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Website     string   `json:"website"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// HostAppStatus 是 ops.status 契约解析后的统一数据集（面板三卡+时长/内存）。
type HostAppStatus struct {
	State     string `json:"state"` // running | stopped | failed | unknown
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Listening bool   `json:"listening"`
	UptimeSec int64  `json:"uptimeSec"`
	MemKB     int64  `json:"memKb"`
	Health    string `json:"health"` // ok | fail | unknown
	Version   string `json:"version"`
	// Raw 保留原始脚本输出，排障时展示。
	Raw string `json:"raw,omitempty"`
}

// Installation 记录某应用在某台主机上的安装情况（本地数据库）。
type Installation struct {
	ID          string    `json:"id"`
	HostID      string    `json:"hostId"`
	AppID       string    `json:"appId"`
	Status      string    `json:"status"` // installing | installed | failed
	AppVersion  string    `json:"appVersion,omitempty"`
	ServiceUnit string    `json:"serviceUnit,omitempty"`
	Port        int       `json:"port,omitempty"`
	InstallDir  string    `json:"installDir,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
	InstalledAt time.Time `json:"installedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// DeployTask 是一次部署任务的执行记录（含每步输出）。
type DeployTask struct {
	ID         string          `json:"id"`
	HostID     string          `json:"hostId"`
	SessionID  string          `json:"sessionId,omitempty"`
	AppID      string          `json:"appId"`
	Kind       string          `json:"kind"` // install | uninstall | update
	Status     string          `json:"status"`
	Steps      []DeployStepRun `json:"steps"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	LastError  string          `json:"lastError,omitempty"`
}

// DeployStepRun 是一步的执行结果。Command 是渲染后（变量已替换为实际值）
// 的最终命令，随步骤落库并在安装面板展示；Output 截断至 16KB 并脱敏。
type DeployStepRun struct {
	StepID     string     `json:"stepId"`
	Name       string     `json:"name"`
	Command    string     `json:"command,omitempty"`
	Status     string     `json:"status"`
	ExitCode   int        `json:"exitCode"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// DeployTaskEvent 是推送到前端的任务事件（Wails 事件 "apps:task"）。
type DeployTaskEvent struct {
	TaskID    string         `json:"taskId"`
	HostID    string         `json:"hostId"`
	AppID     string         `json:"appId"`
	Kind      string         `json:"kind"`
	Status    string         `json:"status"`
	Step      *DeployStepRun `json:"step,omitempty"`
	LastError string         `json:"lastError,omitempty"`
}

// CatalogMeta 记录数据目录中应用目录的来源与版本。
type CatalogMeta struct {
	Source   string    `json:"source"` // builtin | release | main
	Repo     string    `json:"repo"`
	Version  int       `json:"version"`
	Ref      string    `json:"ref,omitempty"` // release tag 或 commit sha
	SyncedAt time.Time `json:"syncedAt"`
}

// CatalogUpdateCheck 是一次更新检查的结果。
type CatalogUpdateCheck struct {
	Source          string `json:"source"` // release | main
	Repo            string `json:"repo"`
	CurrentVersion  int    `json:"currentVersion"`
	LatestVersion   int    `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Ref             string `json:"ref,omitempty"` // tag 名或 commit sha
	Notes           string `json:"notes,omitempty"`
}

# TODO — AI Remote Workspace

> 开发任务清单，按阶段组织。阶段顺序与依赖关系见 [ROADMAP.md](./ROADMAP.md)。
> 勾选规则：`[ ]` 待办 / `[~]` 进行中 / `[x]` 已完成。

---

## Phase 1 — 基础框架

- [x] Wails v3 项目初始化
- [x] React 19 + TypeScript 前端脚手架（Vite + Tailwind v4 + shadcn/ui + Radix + Zustand + TanStack Query + Lucide）
- [x] Wails 前后端事件通信打通（SystemService / ConfigService 绑定 + `time` 事件）
- [x] SQLite 存储层（schema + 基础 CRUD + 迁移，纯 Go 驱动 modernc.org/sqlite）
- [x] 基础 UI 框架（AppShell 布局、Sidebar 导航、StatusBar、Dark Developer Theme）
- [x] 配置管理（ConfigService + 应用配置读写，持久化到 SQLite）

## Phase 2 — SSH Workspace（MVP 核心）

- [x] Host 数据模型与 CRUD（domain.Host + sqlite.HostRepo + HostService）
- [x] Host 添加 / 编辑 / 删除 UI（HostsView + HostFormDialog）
- [x] 测试连接功能（TestConnection，凭据仅本次会话，不持久化）
- [x] SSH Client 封装（infrastructure/ssh：dial / 三种认证 / keepalive / 已知主机校验）
- [x] Connection Manager（多会话状态管理，infrastructure/ssh.Manager）
- [x] xterm.js 终端组件集成（@xterm/xterm + addon-fit，多 Tab）
- [x] PTY 支持（RequestPty + Shell + stdin/stdout/resize 事件流）
- [x] Ctrl+C 与长连接支持（Ctrl+C 经 xterm onData 透传；keepalive 30s 保活）

## Phase 3 — 文件管理（MVP 核心）

- [x] SFTP 客户端封装（infrastructure/sftp.Manager，按 host 缓存连接 + 空闲 10 分钟关闭）
- [x] 远程文件浏览器 UI（SftpView：host 选择 / 面包屑 / 目录列表 / 上下导航）
- [x] 文件上传（file input → ArrayBuffer → SFTP 写入）
- [x] 文件下载（SFTP 读取 → Blob 下载，50MB 上限）
- [x] 文件删除 / 重命名（DeleteFile / RenameFile，含目录删除）

## Phase 4 — AI Agent（MVP 核心）

- [x] LLM Provider 抽象与接入（CloudWeGo eino + eino-ext openai，OpenAI 兼容 API）
- [x] Agent Runtime（eino ReAct Agent，自动工具循环，流式输出）
- [x] Tool Registry（eino utils.InferTool，7 个工具注册）
- [x] `local_exec` Tool
- [x] `local_read_file` Tool
- [x] `ssh_exec` Tool（复用 SSH 连接，ExecInSession）
- [x] `ssh_read_file` / `ssh_write_file` Tool（复用 SFTP）
- [x] `upload` / `download` Tool
- [x] Permission 系统（READ 自动 / WRITE+DANGEROUS 同步等待用户批准）
- [x] AI 基础诊断场景（ssh_exec 检查 CPU/内存/磁盘，关联终端会话）

## Phase 5 — 安全增强

- [x] SecretStore 抽象层（application.SecretStore 接口 + ErrSecretNotFound sentinel）
- [x] Windows Credential Manager 实现（danieljoos/wincred，纯 syscall，无 CGO）
- [x] macOS Keychain 实现（zalando/go-keyring，exec /usr/bin/security，无 CGO）
- [x] Linux Secret Service 实现（zalando/go-keyring + godbus，无 CGO）
- [x] 数据库 `secret_ref` 引用机制（HasRememberedSecret + 记住/清除流程）
- [x] Security Mode 显示（Convenience / Balanced / Secure，当前只读展示）
- [ ] Security Mode 强制策略切换（Convenience 自动保存 / Secure 每次输入）
- [x] Tool Permission 分类（READ / WRITE / DANGEROUS，classifyCommand 命令分级）
- [x] 危险操作 Approval UI（approval.store + ApprovalHost，WRITE/DANGEROUS 同步审批）

## Phase 6 — MCP Server

- [ ] MCP Server 协议实现
- [ ] `list_hosts` / `connect_host`
- [ ] `exec_command`
- [ ] `read_file` / `write_file`
- [ ] `upload` / `download`
- [ ] `system_info`
- [ ] Tool 到 Permission 的映射
- [ ] 外部 Agent 联调（Claude / Codex / Cursor）

## Phase 7 — Docker / Kubernetes

- [x] Docker 面板：概览 / 容器（含实时 stats）/ 镜像 / 日志 四个子页（docker CLI 原生采集，SSH + 本地双通道）
- [x] 容器生命周期控制（start / stop / restart / pause / unpause，allowlist + 确认对话框）
- [x] 友好降级（CLI 未安装 / 守护进程未运行分类提示，不报错）
- [x] Agent 容器运维：直接经 ssh_exec / local_exec 使用 docker / kubectl CLI（提示词引导 + 危险动词 WRITE 分级 + 64KB 输出截断）
- [ ] Kubernetes 面板（pod / deploy / logs UI）— 延后，待 Docker 面板使用反馈
- [ ] Diagnosis Agent（故障定位知识库）
- [ ] 诊断场景沉淀（CPU 高、磁盘满、服务异常等）

---

## 计划外已交付

- [x] 本地终端（跨平台本地 PTY，ConPTY / Unix pty）
- [x] 主机监控面板（概览 / 进程 / 端口，远程零依赖采集，间隔可配置）
- [x] 终端外观设置（配色 / 字体 / 字号，随主机持久化）
- [x] 快捷键系统（全局可改键、冲突检测、鼠标中键行为）
- [x] Agent 会话历史（持久化、可恢复）
- [x] 中英双语（i18next）
- [x] 发布工程（tag 触发三平台打包 + 自动 Release，v0.1.0 已发布）

---

## MVP 发布检查

- [x] 单 Binary 打包（v0.1.0 Release，三平台安装包）
- [ ] 启动快速（冷启动 < 目标值）
- [ ] SSH 长时间稳定
- [ ] Terminal 长时间稳定
- [x] 多 Host 管理可用
- [x] AI 基础诊断可用
- [ ] MCP 调用可用

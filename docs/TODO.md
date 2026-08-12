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

- [ ] SFTP 客户端封装
- [ ] 远程文件浏览器 UI
- [ ] 文件上传
- [ ] 文件下载
- [ ] 文件删除 / 重命名

## Phase 4 — AI Agent（MVP 核心）

- [ ] LLM Provider 抽象与接入（OpenAI / 兼容 API）
- [ ] Agent Runtime（对话、上下文管理）
- [ ] Tool Registry（注册、发现、调用）
- [ ] `local_exec` Tool
- [ ] `local_read_file` Tool
- [ ] `ssh_exec` Tool
- [ ] `ssh_read_file` / `ssh_write_file` Tool
- [ ] `upload` / `download` Tool
- [ ] AI 基础诊断场景验证

## Phase 5 — 安全增强

- [ ] SecretStore 抽象层
- [ ] Windows Credential Manager 实现
- [ ] macOS Keychain 实现
- [ ] Linux Secret Service 实现
- [ ] 数据库 `secret_ref` 引用机制
- [ ] Security Mode 切换（Convenience / Balanced / Secure）
- [ ] Tool Permission 分类（READ / WRITE / DANGEROUS）
- [ ] 危险操作 Approval UI

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

- [ ] Docker Tool 集（容器列表、日志、exec）
- [ ] Kubernetes Tool 集（pod / deploy / logs）
- [ ] Diagnosis Agent（故障定位知识库）
- [ ] 诊断场景沉淀（CPU 高、磁盘满、服务异常等）

---

## MVP 发布检查

- [ ] 单 Binary 打包
- [ ] 启动快速（冷启动 < 目标值）
- [ ] SSH 长时间稳定
- [ ] Terminal 长时间稳定
- [ ] 多 Host 管理可用
- [ ] AI 基础诊断可用
- [ ] MCP 调用可用

# ROADMAP — AI Remote Workspace

> 项目开发路线图。各阶段的详细任务清单见 [TODO.md](./TODO.md)。

---

## 路线图总览

```
Phase 1  基础框架               ✅ 已完成
   ↓
Phase 2  SSH Workspace          ★ MVP 核心 ✅ 已完成
   ↓
Phase 3  文件管理 (SFTP)        ★ MVP 核心 ✅ 已完成
   ↓
Phase 4  AI Agent               ★ MVP 核心 ✅ 已完成
   ↓
Phase 5  安全增强               ✅ 已完成（SecretStore）
   ↓
Phase 6  MCP Server
   ↓
Phase 7  Docker / Kubernetes
```

---

## Phase 1 — 基础框架 ✅

搭建应用骨架。

- Wails v3 + React 19 + TypeScript 前后端集成
- SQLite 存储层（modernc.org/sqlite，无 CGO）
- 基础 UI 框架（AppShell / Sidebar / StatusBar / Dark Developer Theme）
- 配置管理（ConfigService，持久化到 SQLite）
- 后端分层骨架（domain / application / infrastructure / interfaces）

**状态**：已完成。`wails3 task build` 产出单 Binary `bin/ai-remote-workspace.exe`。

## Phase 2 — SSH Workspace（MVP 核心） ✅

让用户能连上服务器、打开终端、执行命令。

- Host CRUD（SQLite 持久化非敏感字段，凭据仅会话内存）
- SSH Client（golang.org/x/crypto/ssh，三种认证 + keepalive + 已知主机校验）
- Connection Manager（多会话生命周期管理）
- xterm.js 终端（@xterm/xterm + addon-fit，多 Tab，scrollback 保留）
- PTY 支持（RequestPty + Shell，事件总线双向流，resize/Ctrl+C 透传）

**状态**：已完成。`wails3 task build` 产出 15MB 单 Binary，含完整 Host 管理 + 多 Tab SSH 终端。

**目标流程**

```
添加服务器
 ↓
打开 Terminal
 ↓
执行命令
```

## Phase 3 — 文件管理（MVP 核心）✅

- SFTP 集成（github.com/pkg/sftp，复用 SSH 连接层，按 host 缓存）
- 文件浏览（目录列表 / 面包屑导航 / 上下级）
- 上传 / 下载（Blob 下载，file input 上传）
- 删除 / 重命名 / 新建文件夹

**状态**：已完成。基础文件操作；凭据复用 Phase 5 的 OS vault 自动解析。

## Phase 4 — AI Agent（MVP 核心）✅

引入 LLM 驱动的智能运维。

- LLM Provider（CloudWeGo eino + eino-ext openai，OpenAI 兼容 API）
- Agent Runtime（eino ReAct Agent，自动工具循环，流式输出）
- Tool Registry（7 个工具：local_exec / local_read_file / ssh_exec / ssh_read_file / ssh_write_file / upload / download）
- Permission 系统（READ 自动 / WRITE+DANGEROUS 同步等待用户批准）
- Agent 关联终端会话（每个会话独立 Agent，操作该会话连接的 Host）
- API Key 存 OS 密码库（复用 Phase 5 SecretStore）

**状态**：已完成。27MB 单 Binary。需要配置 LLM API Key 后使用。

## Phase 5 — 安全增强（SecretStore）✅

凭据不再明文落库，存入操作系统密码库。

- SecretStore 抽象层（`application.SecretStore` 接口）
- Windows Credential Manager（danieljoos/wincred，纯 syscall，无 CGO）
- macOS Keychain（zalando/go-keyring，无 CGO）
- Linux Secret Service（zalando/go-keyring + godbus，无 CGO）
- HostFormDialog "记住密码/记住 passphrase"（存 OS vault，不回显，不存 SQLite）
- 连接时自动从 vault 回填空密码（ResolveCredentials）
- 删除 Host 级联清理 vault 条目
- Security Mode 显示（只读）

**状态**：已完成。三平台均无 CGO，单 Binary 不变。Windows 实测通过（写入/读取/删除 Windows Credential Manager）。

> 未完成：Tool Permission 分类与 Approval UI（依赖 Phase 4 AI Agent / Tool Runtime）。

## Phase 6 — MCP Server

让外部 AI Agent 使用本地能力。

- MCP Server 实现
- Tool 暴露
- Permission 映射

### MCP Tools

- `list_hosts`
- `connect_host`
- `exec_command`
- `read_file`
- `write_file`
- `upload`
- `download`
- `system_info`

### 支持的外部 Agent

- Claude
- Codex
- Cursor

## Phase 7 — Docker / Kubernetes

扩展到容器与集群运维。

- Docker Tools
- Kubernetes Tools
- Diagnosis Agent

---

## MVP 里程碑

满足以下条件即可发布 MVP：

- 单 Binary
- 启动快速
- SSH 稳定
- Terminal 稳定
- 多 Host 管理
- AI 基础诊断
- MCP 调用

对应阶段：**Phase 1 – Phase 6** 完成。

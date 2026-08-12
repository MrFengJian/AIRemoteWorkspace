# ROADMAP — AI Remote Workspace

> 项目开发路线图。各阶段的详细任务清单见 [TODO.md](./TODO.md)。

---

## 路线图总览

```
Phase 1  基础框架               ✅ 已完成
   ↓
Phase 2  SSH Workspace          ★ MVP 核心
   ↓
Phase 3  文件管理 (SFTP)        ★ MVP 核心
   ↓
Phase 4  AI Agent               ★ MVP 核心
   ↓
Phase 5  安全增强
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

## Phase 2 — SSH Workspace（MVP 核心）

让用户能连上服务器、打开终端、执行命令。

- Host CRUD
- SSH Client
- Connection Manager
- xterm.js 终端
- PTY 支持

**目标流程**

```
添加服务器
 ↓
打开 Terminal
 ↓
执行命令
```

## Phase 3 — 文件管理（MVP 核心）

- SFTP 集成
- 文件浏览
- 上传 / 下载

## Phase 4 — AI Agent（MVP 核心）

引入 LLM 驱动的智能运维。

- LLM Provider 接入
- Agent Runtime
- Tool Registry
- `ssh_exec` / `local_exec` Tool

## Phase 5 — 安全增强

- SecretStore（Windows Credential Manager / macOS Keychain / Linux Secret Service）
- Security Mode（Convenience / Balanced / Secure）
- 危险操作 Approval UI

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

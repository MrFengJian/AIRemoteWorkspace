# ARCHITECTURE — AI Remote Workspace

> 技术架构与核心模块设计

---

## 1. 总体技术架构

```
React 19 + TypeScript
        |
       Wails v3
        |
      Go Core

        |
 --------------------
 |        |          |
SSH    Agent      Storage (SQLite)

        |
 Local / Remote Machine
```

### 技术栈

**Backend**（Go 1.24+，分层架构 domain / application / infrastructure / interfaces）：

- Wails v3
- modernc.org/sqlite（纯 Go，无 CGO，保证单 Binary）
- adrg/xdg（用户数据目录定位）

**Frontend**（Feature-Based Architecture）：

- React 19 + TypeScript + Vite
- Tailwind CSS v4 + shadcn/ui + Radix UI
- Zustand（业务状态）+ TanStack Query（服务端状态）
- Lucide Icons
- xterm.js（Phase 2 终端）

### 后端分层

```
internal/
  domain/          业务模型（Host/Session/Tool/Agent/Config），无 I/O 依赖
  application/     业务流程 + port 接口（ConfigService / HostRepository / ToolRegistry）
  infrastructure/  外部实现（sqlite/ 存储层，Phase 5 起 secret/、llm/）
  interfaces/      Wails Service 适配层，调用 application
```

### 前后端通信

- **Service 绑定**：Go struct 经 `application.NewService` 注册 → Wails 自动生成 `frontend/bindings/` 下的 TS 调用函数。
- **Events**：后端 `app.Event.Emit` ↔ 前端 `Events.On`（如 `time` 事件驱动 StatusBar 时钟）。

---

## 2. 核心模块

### 2.1 Host Manager

负责：

- 添加服务器
- 编辑服务器
- 删除服务器
- 测试连接
- 管理连接状态

### 2.2 SSH Runtime

负责：

- SSH Connection
- Authentication
- Keepalive
- Reconnect
- Command Execute
- PTY
- SFTP

### 2.3 Terminal

架构：

```
xterm.js
 ↓
Wails Event
 ↓
PTY
 ↓
SSH Shell
```

支持：

- stdin
- stdout
- resize
- Ctrl+C
- 长连接

### 2.4 SFTP

支持：

- 文件浏览
- 上传
- 下载
- 删除
- 重命名

---

## 3. AI Agent 架构

### 不要

```
LLM
 |
SSH
```

直接让 LLM 调 SSH，缺少权限与执行隔离。

### 采用

```
LLM
 ↓
Tool Runtime
 ↓
Permission
 ↓
Executor
 ↓
Target
```

LLM 只能通过 Tool Runtime 触发动作，统一经过 Permission 校验和 Executor 执行，确保所有 AI 行为可控、可审计。

---

## 4. Tool 系统

### 统一 Tool 抽象

所有 AI 动作都抽象为 Tool，便于权限管理、审计与跨目标（Local / Remote）复用。

### MVP Tool 列表

- `local_exec`
- `local_read_file`
- `ssh_exec`
- `ssh_read_file`
- `ssh_write_file`
- `upload`
- `download`

### 未来扩展

- `docker`
- `kubectl`

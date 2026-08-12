# AI Remote Workspace

> 一个轻量、本地优先、AI 增强的个人开发者 Remote Workspace。

Go 原生（Wails v3 + React）的跨平台桌面应用，把 **SSH / Terminal / SFTP / Local Shell / AI Agent / MCP / Docker / Kubernetes** 统一到一个面向个人开发者的 AI 工作环境。

不替代传统 SSH Client，而是让 AI 在「Remote Context + Tools + Permission」之上真正完成运维与诊断工作。

```
AI
 ↓
Remote Context
 ↓
Tools
 ↓
Execution
 ↓
Diagnosis
```

## 核心特性（MVP 规划）

- 🔌 **SSH Workspace** — 多 Host 管理、稳定终端（xterm.js + PTY）
- 📁 **SFTP 文件管理** — 浏览、上传、下载
- 🤖 **AI Agent** — LLM + Tool Calling，本地与远程统一执行
- 🔐 **分层安全** — 系统密码库托管敏感数据，危险操作需用户授权
- 🔗 **MCP Server** — 让 Claude / Codex / Cursor 等外部 Agent 复用本地能力
- 📦 **单 Binary** — 下载即用，无需复杂部署

## 技术栈

桌面应用基于 **Wails v3**（Go 原生，单 Binary，跨平台）：

| 层 | 技术 |
| --- | --- |
| Desktop Framework | Wails v3 |
| Backend | Go 1.24+ |
| Frontend | React 19 · TypeScript · Vite |
| Styling | Tailwind CSS v4 · shadcn/ui · Radix UI |
| State / Data | Zustand · TanStack Query |
| Icons | Lucide |
| Terminal | xterm.js（Phase 2） |
| Storage | SQLite（纯 Go 驱动 modernc.org/sqlite，无 CGO） |
| SSH | golang.org/x/crypto/ssh（认证 / keepalive / PTY / 已知主机校验） |
| SFTP | github.com/pkg/sftp（远程文件操作，连接缓存） |

> 技术栈以 [`AGENT.md`](./AGENT.md)（Coding Agent Guide）为准。

## 快速开始

### 前置依赖

- **Go** 1.25+
- **Node.js** 20.19+（推荐 22.12+）
- **Wails v3 CLI**：`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`

### 开发

```bash
# 热重载开发模式（启动桌面窗口 + 前端 HMR）
wails3 task dev
```

### 构建

```bash
# 生产构建，产出 bin/ai-remote-workspace.exe（Windows）
wails3 task build
```

构建产出为**单 Binary**，前端已通过 `//go:embed` 嵌入。

## 项目结构

```
.
├── main.go                 # 应用入口：组装各层 + Wails 窗口 + time 事件
├── internal/
│   ├── domain/             # 业务模型（Host/Session/Tool/Agent/Config/SSH）
│   ├── application/        # 业务流程 + port 接口（HostService/ConnectionManager）
│   ├── infrastructure/
│   │   ├── secret/         # OS 密码库（Windows Credential Manager / macOS Keychain / Linux Secret Service）
│   │   ├── sftp/           # SFTP Manager（连接缓存）+ 文件操作（ls/upload/download/delete/rename/mkdir）
│   │   ├── sqlite/         # SQLite 存储实现 + schema 迁移（hosts/host_keys/settings）
│   │   └── ssh/            # SSH Client / PTY Session / ConnectionManager / 已知主机校验
│   └── interfaces/         # Wails Service（HostService/TerminalService/SystemService/ConfigService）
├── frontend/
│   ├── src/
│   │   ├── app/            # providers, router
│   │   ├── features/       # Feature-Based：hosts/ terminal/ agent/ sftp/ settings/
│   │   ├── components/     # ui/ (shadcn), layout/ (AppShell/Sidebar/StatusBar)
│   │   ├── stores/         # 全局状态（Zustand）
│   │   ├── lib/            # utils, queryClient, wails helpers
│   │   ├── styles/         # Design Token (globals.css)
│   │   └── themes/         # Dark theme token overrides
│   └── bindings/           # Wails 自动生成的 TS 绑定（勿手改）
├── build/                  # 各平台打包资源（Windows/macOS/Linux/iOS/Android）
└── docs/                   # PRD / 架构 / 安全 / 路线图 / TODO
```

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [AGENT.md](./AGENT.md) | Coding Agent 指南：技术栈、架构、编码规范（source of truth） |
| [PRD.md](./docs/PRD.md) | 产品需求：定位、原则、MVP 范围、发布标准、护城河 |
| [ARCHITECTURE.md](./docs/ARCHITECTURE.md) | 技术架构：总体架构、核心模块、Agent 架构、Tool 系统 |
| [SECURITY.md](./docs/SECURITY.md) | 安全设计：数据分类、Security Mode、Tool Permission |
| [ROADMAP.md](./docs/ROADMAP.md) | 开发路线图：7 个 Phase 与 MVP 里程碑 |
| [TODO.md](./docs/TODO.md) | 开发任务清单：按阶段拆分的可勾选条目 |

## 状态

- ✅ **Phase 1 — 基础框架**：Wails v3 + React 19 + SQLite + Dark Developer Theme
- ✅ **Phase 2 — SSH Workspace**：Host CRUD + 多 Tab xterm.js 终端 + SSH 连接/认证/keepalive/已知主机校验
- ✅ **Phase 3 — 文件管理（SFTP）**：远程文件浏览器，上传/下载/删除/重命名/新建文件夹
- ✅ **Phase 5 — 安全增强（SecretStore）**：记住密码存入 OS 密码库（Windows Credential Manager / macOS Keychain / Linux Secret Service），三平台无 CGO
- 🚧 文件管理（SFTP）/ AI Agent / MCP 等后续阶段开发中 — 详见 [ROADMAP.md](./docs/ROADMAP.md)

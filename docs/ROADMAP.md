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
Phase 6  MCP Server             ✅ 已完成
   ↓
Phase 7  Docker / Kubernetes    Docker 面板 ✅ · K8s 面板延后
   ↓
Phase 8  Xshell 能力对齐        连接链路 / 审计 / 传输 / 运维效率
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

## Phase 6 — MCP Server ✅

让外部 AI Agent 使用本地能力。

- MCP Server 实现（官方 modelcontextprotocol/go-sdk，Streamable HTTP 绑定 127.0.0.1，Bearer Token 鉴权）
- 设置页管理：启用开关 / 端口 / 令牌重新生成 / 客户端配置片段复制
- Tool 暴露（8 个，见下）
- Permission 映射（复用 PermissionGate：READ 自动；exec 按命令分级；WRITE/DANGEROUS 在应用内弹审批框，标注 `mcp:<主机名>` 目标）
- 首次启用自动生成令牌并持久化；MCP 会话生命周期随服务器启停（停用时关闭其打开的 SSH 连接）

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

**状态**：已完成。stdio-only 客户端（如 Codex CLI）经 `mcp-remote` 桥接；单测覆盖鉴权、协议回环与权限映射。

## Phase 7 — Docker / Kubernetes

扩展到容器与集群运维。

### Docker 面板 ✅

与主机监控同构的右侧面板，通过 docker CLI 原生采集（SSH 会话走 exec 通道，本地终端直连本机 Docker），不依赖 API socket 暴露或额外安装：

- 概览：Docker 版本 / API 版本 / 平台 / 数据目录 / 容器与镜像计数
- 容器：状态过滤（全部 / 运行中 / 已暂停 / 已停止，含计数）、实时 CPU / 内存、端口映射
- 监控：每容器资源卡片（CPU / 内存条形图、网络与磁盘 I/O、PIDs），按 CPU 排序
- 网络：列表（驱动 / 范围），展开查看子网 / 网关 / 接入容器
- 生命周期控制：start / stop / restart / pause / unpause（allowlist 校验 + 确认对话框）
- 镜像：仓库 / 标签 / 大小 / 创建时间
- 日志：按容器查看，行数可选（100–1000），自动滚到最新行
- 友好降级：CLI 未安装 / 守护进程未运行分别提示，不报错
- 自动刷新间隔复用全局监控设置（默认 60s）

### Agent 容器运维 ✅

刻意**不做** docker 命令的专用工具封装——现有原子工具（ssh_exec / local_exec）配合 LLM 自行组合更灵活。配套加固：

- 系统提示词明确引导使用 docker / kubectl CLI（含有界输出建议）
- 危险命令分级补全：docker stop/start/restart/pause/kill 及 compose/swarm 变更类动词 → WRITE（需审批）
- 工具输出 64KB 截断（保头保尾），防止大日志 / 大文件撑爆对话上下文

### Kubernetes 面板（延后）

- Pod / Deployment / 日志 / 事件面板（kubectl CLI 同构方案）
- 待 Docker 面板实际使用反馈后再排期

### Diagnosis Agent ✅（Phase C 远期可选）

按 [DIAGNOSIS_AGENT.md](./DIAGNOSIS_AGENT.md) 交付，Phase A + B 已上线：

- 故障定位知识库 —— 内置 9 个 SKILL.md 诊断场景包（CPU 高 / 磁盘满 / 内存 OOM / 服务异常 / 端口不通 / 容器重启循环 / 网络延迟 / 磁盘 IO 高 / SSH 登录慢），随二进制 `go:embed` 分发，启动时落入技能目录（不覆盖用户改动；删除内置包会被记住）
- 确定性体检快照 —— `MonitorService.Snapshot` 聚合 CPU / 内存 / 磁盘 / Top 进程 / 监听端口 / journalctl·dmesg 近期错误，诊断会话首轮自动注入，不烧 LLM 的分诊上下文
- 诊断模式 —— 独立系统提示词模板（快照优先、场景包决策树、证据优先，结论按 现象 / 根因 / 证据 / 建议 / 风险 输出）；Agent 面板一键诊断入口，症状输入自动带快照发起会话，权限策略零新机制
- 沉淀闭环 —— 会话历史右键「保存为场景」，LLM 一次性提炼为 SKILL.md 草稿，预览编辑后写入技能目录，下次同类症状即被命中；场景库轻 UI（列表 / 新建 / 编辑 / 删除）
- Phase C（远期可选）：历史诊断检索 / 结构化结论面板

---

## Phase 8 — Xshell 能力对齐

对照 XShell 的差距分析（2026-09）：主干功能（多标签 / 分屏 / SFTP / 隧道 / 快速命令批量发送）已齐，缺的是**连接链路、审计合规、传输兼容**三条线。按优先级分三批交付，宏录制、打印、Rlogin 等过时能力刻意不做（AI Agent + 快速命令已是更强形态）。

### P0 — 进入生产运维场景的前置能力

**跳板机 / 堡垒机 + 连接代理** ✅
`Host` 增加代理配置：经其他主机跳转（支持多级，形成链）或经 HTTP CONNECT / SOCKS5 代理连接。ProxyService 把配置递归解析成 SshRoute（含环路检测、逐跳凭据经 OS 密码库解析），SSH 层把跳板连接作为隧道逐级转发，两级 known_hosts 校验；HostFormDialog 提供配置 UI。终端会话、断线重连、隧道、SFTP 复用同一路由解析，自动受益。

**会话日志（Session Logging）**
PTY 输出 tee 到 `<数据目录>/logs/<主机>/<日期>.log`，全局开关 + 按主机覆盖，可选时间戳前缀；设置页管理并提供打开日志目录入口。审计合规与事后排障的硬需求。

**会话断线自动重连** ✅
复用隧道的指数退避策略（2s→30s 封顶，10 次上限）：连接死亡时在后端原位重连——同一会话 ID 换新连接和新 PTY，前端标签、PTY 实例与滚动缓冲全部保持；keepalive 探针区分「链路死亡」（自动重连）与「shell 正常退出」（不重连），用户主动关闭绝不触发。

### P1 — 高频效率

**同步键入（broadcast input）**
在快速命令栏的目标多选基础上增加「持续同步」模式：焦点窗格的键入实时镜像到所有目标窗格，批量执行交互式操作（同时 vim / watch）时不可替代；与一次性批量发送互补。

**Zmodem（rz/sz）**
识别 ZRQINIT / ZRINIT 序列自动拉起传输。堡垒机与嵌入式设备常无 SFTP 子系统，rz/sz 是事实标准，也是老运维最强的习惯依赖。

**登录脚本（expect 式序列）**
主机级配置「等待 xxx → 发送 yyy」序列，连接建立后自动执行：堡垒机二次认证、进设备自动 enable 等场景。

**会话恢复 + 回滚行数配置**
重启后恢复上次打开的标签与分屏布局；xterm scrollback 行数进设置（含大值 / 无限）。

### P2 — 覆盖面

- **Telnet / 串口协议源**：网络设备与 Console 线场景；PTY 源已抽象（localpty / ssh），新增 telnet（TCP+IAC）与 serial（go.bug.st/serial）两个源即可复用全部前端
- **密钥管理器 + SSH Agent 转发**：密钥生成 / 导入 / 转换 / 导出（纯库实现）；会话开启 agent forwarding（跳板链与远程 Git 依赖）
- **会话组批量打开**：一键打开分组内全部主机
- **终端编码切换**：按主机 UTF-8 / GBK（simplifiedchinese 在 PTY 流上转码），老设备中文不乱码
- **系统通知**：会话断开、长任务完成的系统级提醒
- **远程文件右键编辑**：SFTP 面板下载 → 本地编辑器 → 保存自动回传

### 交付批次与版本目标

- 第一批（P0）：v0.6.x — 跳板/代理、会话日志、自动重连
- 第二批（P1）：v0.7 — 同步键入、Zmodem、登录脚本、会话恢复
- 第三批（P2）：按反馈排期

---

## 计划外已交付

路线图之外按实际需求交付的功能（已全部上线）：

- 本地终端（跨平台本地 PTY：Windows ConPTY / Unix pty，不走 SSH）
- 主机监控面板（概览 / 进程 / 端口，/proc 与系统原生工具采集，远程零依赖）
- 终端外观设置（13 套 xterm 配色 / 字体 / 字号，实时预览，随主机持久化）
- Xshell 风格快捷键系统（全局设置可改键、冲突检测、鼠标中键行为可配置）
- Agent 会话历史（多轮对话持久化、可恢复）
- 中英双语（i18next，400+ 键完全对齐）
- 发布工程（GitHub Actions：v*.*.* 标签触发三平台打包，自动发布 Release，v0.1.0 已发布；v0.5.1 起说明由提交记录自动分组生成）
- SSH 隧道（本地 / 远程 / 动态转发，多规则自动重连，独立连接，右侧面板管理）
- 快速命令栏（Xshell 风格：自定义脚本、多会话批量发送、批量发送确认）

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

> **现状**：v0.1.0 已发布（Phase 1–5 + 计划外功能全部达成，发布流水线已验证）。
> Phase 6 MCP Server 已实现（目标 v0.2），MVP 功能清单全部达成；剩余为稳定性验收
> （冷启动 / SSH / Terminal 长时间稳定）与 Phase 7 延后项（K8s 面板、Diagnosis Agent）。

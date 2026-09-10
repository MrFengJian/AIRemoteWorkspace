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
- [x] 双栏 SFTP 工作台（独立窗口，本地 + 远程左右双栏，主机列表一键打开）
- [x] 右键复制 / 粘贴（应用内剪贴板：同侧粘贴为复制，本地→远程上传，远程→本地下载；支持文件夹递归与同名冲突处理）
- [x] 目录级流式传输（上传 / 下载 / 远程↔远程复制自动展开目录树，聚合进度 + 可取消）

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
- [x] ~~Security Mode 强制策略切换~~ — **不做**：默认 Balanced 足够，逐主机"记住密码"勾选已提供更细粒度的控制
- [x] Tool Permission 分类（READ / WRITE / DANGEROUS，classifyCommand 命令分级）
- [x] 危险操作 Approval UI（approval.store + ApprovalHost，WRITE/DANGEROUS 同步审批）

## Phase 6 — MCP Server

- [x] MCP Server 协议实现（官方 go-sdk，Streamable HTTP @ 127.0.0.1，Bearer Token 鉴权，设置页可启停 / 改端口 / 换令牌）
- [x] `list_hosts` / `connect_host`
- [x] `exec_command`
- [x] `read_file` / `write_file`
- [x] `upload` / `download`
- [x] `system_info`
- [x] Tool 到 Permission 的映射（复用 PermissionGate：READ 自动；exec 按命令分级；WRITE 需应用内审批，弹窗标注目标主机 `mcp:<主机名>`）
- [x] 外部 Agent 联调准备（设置页提供 Claude / Cursor 配置片段；stdio-only 客户端走 `mcp-remote`）

## Phase 7 — Docker / Kubernetes

- [x] Docker 面板：概览 / 容器（状态过滤 + 实时 stats）/ 监控 / 网络 / 镜像 / 日志 六个子页（docker CLI 原生采集，SSH + 本地双通道）
- [x] 容器监控子页：每容器 CPU / 内存条形图、网络与磁盘 I/O、PIDs（按 CPU 排序）
- [x] 网络子页：列表 + 展开查看子网 / 网关 / 接入容器（network inspect）
- [x] 容器生命周期控制（start / stop / restart / pause / unpause，allowlist + 确认对话框）
- [x] 友好降级（CLI 未安装 / 守护进程未运行分类提示，不报错）
- [x] Agent 容器运维：直接经 ssh_exec / local_exec 使用 docker / kubectl CLI（提示词引导 + 危险动词 WRITE 分级 + 64KB 输出截断）
- [ ] Kubernetes 面板（pod / deploy / logs UI）— 延后，待 Docker 面板使用反馈
- [~] Diagnosis Agent — 方案见 [DIAGNOSIS_AGENT.md](./DIAGNOSIS_AGENT.md)
  - [x] Phase A：内置场景包（9 个 SKILL.md）+ 诊断模式提示词 + 体检快照注入 + Agent 面板入口
  - [x] Phase B：沉淀闭环（「保存为场景」LLM 提炼 SKILL.md）+ 场景库管理轻 UI
  - [ ] Phase C（远期可选）：历史诊断检索 / 结构化结论面板（结论面板设计已定，见 [DIAGNOSIS_AGENT.md](./DIAGNOSIS_AGENT.md)，待排期）
- [x] 诊断场景沉淀（CPU 高、磁盘满、内存/OOM、服务异常、端口不通、容器重启循环、网络延迟、磁盘 IO、SSH 登录慢）

## Phase 8 — Xshell 能力对齐

> 背景与方案见 [ROADMAP.md](./ROADMAP.md) Phase 8。按 P0 → P1 → P2 三批交付。

### P0 — 生产运维前置能力（目标 v0.6.x）

- [x] 跳板机 / 堡垒机（多级 SSH 跳转）+ HTTP / SOCKS5 代理 — 全拨号路径统一走路由解析
  - [x] domain.Host 增加代理配置（直连 / 经主机跳转 / HTTP CONNECT / SOCKS5；跳板引用其他 Host 递归成链，环路检测）
  - [x] ssh.Dial 支持 Route：SOCKS5（RFC1928/1929，含用户名密码）与 HTTP CONNECT 拨号器；逐跳 SSH 连接 + direct-tcpip 隧道；每跳按自身 HostID 校验已知主机
  - [x] HostFormDialog 代理小节（连接标签页）：连接方式 / 跳板主机选择（排除自身）/ 代理地址与认证；代理密码存 OS 密码库（写only + 清除勾选）
  - [x] 终端会话（含断线自动重连）、隧道、SFTP 全部复用同一路由；跳板/代理配置变更后重连即时生效
- [~] 会话日志（Session Logging）— 会话右键菜单版已上线：菜单「日志」启停（勾选态）+ tee 落盘 + 打开日志目录
  - [x] TerminalService PTY 输出 tee 落盘 `<数据目录>/logs/<主机>/<时间>-<会话>.log`（会话级文件，分屏互不混写；开始/结束标记；关闭标签与 PTY 退出双路径收尾）
  - [x] 会话右键菜单「日志」子菜单：开始记录 / 停止记录（toast 回显文件路径）、打开日志目录
  - [ ] 全局开关 + 按主机覆盖 + 可选时间戳前缀
  - [ ] 设置页管理入口
- [x] 会话断线自动重连 — 后端原位重连：同一会话 ID 换新连接 + 新 PTY，前端标签与滚动缓冲保持不动
  - [x] 指数退避（2s→30s 封顶）+ 次数上限（10 次，耗尽走正常 exit 路径）
  - [x] 连接死亡判定：PTY 退出后对链路发 keepalive 探针（3s 超时）——链路健康时的 shell 正常退出不重连；用户主动关闭先标记 closing，绝不触发
  - [x] 重连窗口内的 resize 记忆并在新 PTY 上生效；新事件 `term:<id>:reconnecting`（attempt≥1 重连中 / 0 成功），终端内打印提示行，标签状态点黄色脉冲

### P1 — 高频效率（目标 v0.7）

- [ ] 同步键入（broadcast input）：基于快速命令栏目标多选，焦点窗格键入实时镜像到目标窗格，开关退出
- [ ] Zmodem（rz/sz）：识别 ZRQINIT/ZRINIT 自动拉起，传输进度 UI
- [ ] 登录脚本：主机级「等待→发送」序列，连接建立后自动执行
- [ ] 会话恢复：重启后恢复上次打开的标签与分屏布局
- [ ] scrollback 行数进设置（含大值档位）

### P2 — 覆盖面（按反馈排期）

- [ ] Telnet 协议源（TCP + IAC 协商）
- [ ] 串口协议源（go.bug.st/serial）
- [ ] 密钥管理器（生成 / 导入 / 转换 / 导出）
- [ ] SSH Agent 转发（会话级开关）
- [ ] 会话组批量打开（一键打开分组内全部主机）
- [ ] 终端编码按主机切换（UTF-8 / GBK）
- [ ] 系统通知（会话断开 / 长任务完成）
- [ ] 远程文件右键本地编辑、保存自动回传

---

## 计划外已交付

- [x] 本地终端（跨平台本地 PTY，ConPTY / Unix pty）
- [x] 主机监控面板（概览 / 进程 / 端口，远程零依赖采集，间隔可配置）
- [x] 终端外观设置（配色 / 字体 / 字号，随主机持久化）
- [x] 快捷键系统（全局可改键、冲突检测、鼠标中键行为）
- [x] Agent 会话历史（持久化、可恢复）
- [x] 中英双语（i18next）
- [x] 发布工程（tag 触发三平台打包 + 自动 Release；v0.5.1 起说明由提交记录自动分组生成）
- [x] 隧道面板（多规则状态、启停、复用主机编辑直达隧道配置）
- [x] 快速命令栏（自定义脚本、多会话批量发送、批量确认）
- [x] 撰写栏（独立于快速命令栏：即时输入批量发送到选中会话，Enter 执行 / Shift+Enter 仅输入）

---

## MVP 发布检查

- [x] 单 Binary 打包（v0.1.0 Release，三平台安装包）
- [ ] 启动快速（冷启动 < 目标值）
- [ ] SSH 长时间稳定
- [ ] Terminal 长时间稳定
- [x] 多 Host 管理可用
- [x] AI 基础诊断可用
- [x] MCP 调用可用

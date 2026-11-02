# 应用目录与管理文档规范（APP_CATALOG）

应用部署功能的目录契约：每个应用以一份 **app.json 操作手册**（权威执行源）+
**guide.md 部署指南**（人读/AI 智能体读）+ **icon.svg 图标** 描述。UI 支持的
每个操作（安装/启动/停止/重启/状态/日志/开放端口/升级/卸载）一一映射到
app.json 中的具体 shell 命令，经 SSH 在远端执行；`internal/application/appdeploy`
不硬编码任何 systemctl/防火墙逻辑。内置目录随二进制分发
（`internal/application/appdeploy/catalog/`，`go:embed`），启动时种子解压到
`<数据目录>/apps/catalog/`；用户自建应用放 `<数据目录>/apps/custom/<id>/`，
种子与更新绝不触碰。

## 目录布局

```
catalog.json               # {catalogVersion, apps: [{id,name,description,website,icon,tags}]}
<app-id>/
  app.json                 # 操作手册（schema v1，见下）
  guide.md                 # 部署指南（与 app.json 命令一一对应的分步说明）
  icon.svg                 # 图标（文件名登记在 app.json 的 icon 字段）
```

## app.json 字段（统一数据集）

所有应用实现几乎相同的操作集，字段如下（schemaVersion 固定 `1`）：

| 字段 | 说明 |
|---|---|
| `id` | slug（`[a-z0-9][a-z0-9-_.]{0,63}`），目录名一致 |
| `name` / `description` / `website` / `tags` | 应用元数据 |
| `icon` | 同目录图标文件名；**使用应用官方图标原图**（保持其原始格式：svg/png/ico/webp 均可，按扩展名推断 MIME，后端经 base64 data URL 下发，二进制格式不经过文本通道） |
| `catalogVersion` | 条目版本（正整数；内置种子按整体 catalogVersion 比对覆盖） |
| `install.detect` | 已安装检测命令：输出 `yes` = 已安装 |
| `install.inputs[]` | 部署输入：`{key,label,type(text\|secret),required,placeholder,validate}`，validate 目前支持 `min=N` |
| `install.steps[]` | 固化安装步骤 `{name,command,timeoutSec,risk(write\|high)}` |
| `install.verify` | 安装后验证命令（退出码 0 = 成功） |
| `ops.start/stop/restart` | 服务控制命令 |
| `ops.status` | 状态聚合脚本，输出标准化 `KEY=VALUE` 契约（见下） |
| `ops.logs` | 日志命令，模板变量 `{{.lines}}`（默认 200） |
| `ops.openPort` | 放行端口命令，模板变量 `{{.port}}`（命令内自适配 firewalld/ufw/iptables） |
| `ops.version` / `ops.update` | 版本查询 / 应用自升级命令 |
| `ops.uninstall[]` | 卸载步骤（同 install.steps 结构） |
| `panel.url` / `panel.port` | 面板地址模板（`{{.host}}`/`{{.port}}`）与端口；无 Web 面板则整个 `panel` 为 `null` |
| `panel.credentials[]` | 面板登录凭证字段 `{key,label,secret}`；secret 存 OS 钥匙串 |

**模板渲染规则**：`{{.key}}` 在执行前替换，值一律经 POSIX 单引号转义
（含引号的密码也安全）；未知占位符直接报错，不会把字面大括号送上远端。

执行环境**：所有操作命令统一经 `stdbuf -oL -eL bash -l -c <脚本> </dev/null>`
在远端执行——登录 shell（-l）保证 `~/.local/bin` 等安装脚本追加的 PATH 生效；
行缓冲（stdbuf，缺失时原样执行）+ `PYTHONUNBUFFERED=1` 让安装器输出逐行实时
到达（非 TTY 默认 4KB 块缓冲，否则面板只能看到攒到最后的一次性输出）；stdin
重定向 /dev/null 让交互提示立即 EOF，不会阻塞或吞掉脚本；脚本整体单引号转义
经 argv 传入，heredoc/引号无需二次转义。`ops.status` 解析只认
`KEY=VALUE` 行，登录 shell 的 profile 输出不影响契约。注意：命令无 TTY，
步骤中需要 `sudo` 的应用要求目标用户具备 NOPASSWD 免密 sudo。

**命令展示**：每步执行前渲染模板（变量替换为实际值），渲染后的最终命令随
步骤（`DeployStepRun.Command`）落库并推送前端，安装面板按 `命令 → 输出`
展示执行过程。

## ops.status 输出契约（统一监控数据集）

`ops.status` 是一段 shell 脚本，逐行输出以下 KEY（全部键都必须出现，取不到
就给安全缺省，Go 端 `ParseHostAppStatus` 统一解析为 `HostAppStatus`）：

```
STATUS=running|stopped|failed       # systemd is-active / pgrep 推断
PID=<int>                           # 无进程时 0
PORT=<int>                          # 无端口应用输出 0
LISTENING=yes|no                    # ss -tln 探测
UPTIME_SEC=<int>                    # ps -o etimes=
MEM_KB=<int>                        # ps -o rss=
HEALTH=ok|fail                      # curl 本机健康检查（无则按运行状态推导）
VERSION=<string>                    # 可为空
```

## 目录更新（GitHub release / main 分支）

数据目录中的目录可从 GitHub 主动更新，两种来源（设置 `apps.catalogRepo`，
默认 `MrFengJian/AIRemoteWorkspace`）：

- **最新 Release**：取 `<repo>/releases/latest` 名为 **`catalog.zip`** 的资产。
  zip 布局与上述目录布局一致（根下 `catalog.json` + 各应用目录），整体
  sha 由 release 流程保证。
- **main 分支**：依次抓取 `raw.githubusercontent.com/<repo>/main/apps/catalog/`
  下的 `catalog.json`、`<id>/app.json`、`<id>/guide.md`、`<id>/icon.svg`
  （icon 缺失容忍 404）。仓库内路径约定：`apps/catalog/`。

两种来源都是**先全量下载到内存、逐个 app.json 过结构校验后才落盘**（覆盖
写入 + 裁剪 catalog.json 已移除的应用目录），校验失败不会污染本地目录。
内置种子只在 `catalogVersion` 大于磁盘版本时覆盖，因此更新后的目录不会被
旧二进制降级。

## 新增应用步骤

1. 新建 `internal/application/appdeploy/catalog/<id>/{app.json,guide.md,icon.svg}`；
2. app.json 覆盖完整操作集（测试 `TestBuiltinCatalogValid` 强制 status 契约
   键集与 start/stop/restart 齐备）；
3. `catalog.json` 登记 `apps[]` 并把 `catalogVersion` 加一；
4. `go test ./internal/application/appdeploy/` 通过后，随版本发布即可；
5. 如需免发版更新，把同样的目录推到更新仓库 `apps/catalog/` 并打含
   `catalog.zip` 资产的 release。

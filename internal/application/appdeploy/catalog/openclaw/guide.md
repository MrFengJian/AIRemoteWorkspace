# OpenClaw 部署与运维指南

> 本指南与 `app.json` 操作手册一一对应：安装/启动/停止/重启/状态/日志/开放端口/升级/卸载的每一步都有可直接执行的命令，应用面板按 `app.json` 自动执行；本文供人工排障与 AI 智能体参考。

## 概述

[OpenClaw](https://github.com/openclaw/openclaw) 是开源个人 AI 助理运行时，跑在自己的服务器上，通过 WhatsApp/Telegram/Discord/Slack 等消息渠道交互，内置 Web 控制台（Gateway，默认端口 **18789**）。

- 数据目录：`~/.openclaw/`
- 服务托管：`openclaw onboard --install-daemon` 安装守护进程（Linux 为 systemd 单元 `openclaw-gateway.service`，系统级安装位于 `/etc/systemd/system/`）
- 运行时：安装脚本按需自动安装受支持的 Node.js

## 前置要求

- Linux / macOS；建议 2 核 2G 以上
- 可访问外网（脚本从 openclaw.ai 下载，npm 安装依赖）
- 使用消息渠道（Telegram/WhatsApp 等）需在安装后按官方文档配置渠道凭据

## 安装（应用面板一键执行）

| 步骤 | 命令 | 说明 |
|---|---|---|
| 1. 安装 | `curl -fsSL --proto '=https' --tlsv1.2 https://openclaw.ai/install.sh \| bash -s -- --no-prompt --no-onboard --verify` | 官方 CI/自动化参数组合：`--no-prompt` 禁用一切交互提示、`--no-onboard` 跳过向导（onboard 由第 2 步非交互完成）、`--verify` 安装后冒烟自检。若默认 Node < 24.22，面板会先经 nvm 非交互安装 Node 26 |
| 2. 初始化 + 守护进程 | `openclaw onboard --non-interactive --accept-risk --gateway-auth password --gateway-password <密码> --install-daemon` | 官方非交互模式：密码认证经 `--gateway-password` 传入（密码模式必须显式提供），创建默认配置并安装守护进程，`--accept-risk` 跳过风险提示 |
| 3. 网关认证 + LAN 绑定 | `openclaw config set gateway.auth.mode password` → `gateway.auth.password <密码>` → `gateway.bind lan` → `openclaw gateway install --force` → systemd drop-in 注入 `OPENCLAW_GATEWAY_PASSWORD`（600 权限）→ `daemon-reload` + `openclaw gateway restart` | 密码即安装向导中输入的值（面板自动保存）；配置热加载 + 环境变量双通道，杜绝"password 模式但无密码"的启动失败 |
| 4. 验证 | `ss -tln \| grep 18789` | 端口监听即成功 |

> **nvm 主机说明**：OpenClaw 安装在 nvm node 26 的 bin 目录下（仅切到 26 时可用）。第 1 步在默认 Node < 24.22 时会自动经 nvm 非交互安装 Node 26；第 2/3 步执行前自动 `nvm use 26` 并补全 `~/.local/bin` PATH（`--no-prompt` 保证全程无任何交互提示）。手动执行时请先 `nvm use 26`。
> **systemd 目录权限**：OpenClaw 安装守护进程前会校验 `~/.config/systemd/user` 及其最近祖先目录不可组/他人写，否则报 `SERVICE_DEFINITION_UNKNOWN: unsafe-permissions` 拒绝安装；安装步骤已自动 `chmod go-w` 修正。手动遇到时：`chmod go-w ~/.config ~/.config/systemd ~/.config/systemd/user` 后重试 `openclaw gateway install`（不要用 sudo 或递归 chmod）。
> **访问说明**：网关以 `bind: lan` 监听 `0.0.0.0:18789`（OpenClaw 强制要求非回环绑定必须配置认证），浏览器打开 `http://<服务器IP>:18789`，用安装时设置的密码登录。密码强度自行保证。
> **渠道接入（交互式，在终端执行一次）**：消息渠道（Telegram/WhatsApp 等）需要交互选择，`--non-interactive` 会跳过——装完后在 Web 控制台或终端补配即可。

## 服务管理

```bash
openclaw gateway start     # 启动
openclaw gateway stop      # 停止
openclaw gateway restart   # 重启（官方推荐用 restart，勿用 stop+start 替代）
openclaw gateway status    # 运行状态（Runtime: running + 连通性检查）
systemctl status openclaw-gateway   # systemd 视角（用户级加 --user）
```

## 状态与监控指标

面板"服务状态"卡按 `app.json` 的 `ops.status` 脚本采集：进程（`pgrep -f openclaw`）、监听端口 18789、运行时长、内存占用、健康检查（HTTP `http://127.0.0.1:18789/` 有响应即在线，401 也算——密码模式下未带凭据的探测本就会被拒）、版本号。

## 日志

```bash
openclaw logs                       # 官方日志命令
journalctl --user -u openclaw-gateway -n 200 --no-pager   # 用户级服务
journalctl -u openclaw-gateway -n 200 --no-pager          # 系统级服务
```

## 开放端口

```bash
# firewalld（CentOS/OpenCloudOS 等）
firewall-cmd --permanent --add-port=18789/tcp && firewall-cmd --reload
# ufw（Ubuntu/Debian）
ufw allow 18789/tcp
# 云服务器还需在控制台安全组放行 TCP 18789
```

> 网关已配置密码认证（password 模式）+ LAN 绑定，防火墙/安全组放行后即可直接访问；公网暴露仍建议限制来源 IP。更保守的方案是保持 loopback 绑定 + SSH 隧道（`ssh -N -L 18789:127.0.0.1:18789 user@<主机>`）。

## 外部访问

Gateway 默认仅回环监听（`loopback`）。OpenClaw 强制要求：非回环绑定必须配置认证，否则启动报 `refusing to bind gateway ... without auth`。需要外部访问 Control UI 时二选一：

```bash
# 方式一（官方推荐）：SSH 隧道，本机访问 http://127.0.0.1:18789
ssh -N -L 18789:127.0.0.1:18789 user@<主机>

# 方式二：开放 LAN 绑定 + 认证（token/password 缺一不可）
openclaw gateway install --force --bind lan   # 并按官方文档配置 gateway.auth 或 OPENCLAW_GATEWAY_TOKEN
```

## 升级与卸载

```bash
openclaw update                      # 官方升级命令（或 npm install -g openclaw@latest）
openclaw gateway restart             # 升级后重启
openclaw gateway stop                # 停止守护进程
systemctl --user disable --now openclaw-gateway.service   # 移除用户级服务（系统级去掉 --user）
npm uninstall -g openclaw            # 卸载程序（保留 ~/.openclaw 数据）
```

## 常见问题

- **控制台打不开**：`openclaw gateway status` 看运行状态；`ss -tlnp | grep 18789` 确认监听；检查防火墙/安全组。
- **修改端口**：优先级 `--port` → `OPENCLAW_GATEWAY_PORT` → `gateway.port` 配置 → 默认 18789；改完执行 `openclaw gateway install --force` 或 `openclaw doctor --fix`。
- **服务异常**：`openclaw doctor` 诊断；`openclaw logs` 查看日志。

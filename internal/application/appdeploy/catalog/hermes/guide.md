# Hermes Agent 部署与运维指南

> 本指南与 `app.json` 操作手册一一对应：安装/启动/停止/重启/状态/日志/升级/卸载的每一步都有可直接执行的命令，应用面板按 `app.json` 自动执行；本文供人工排障与 AI 智能体参考。

## 概述

[Hermes Agent](https://github.com/NousResearch/hermes-agent) 是 Nous Research 开源的自进化 AI 智能体，7×24 运行在自己的服务器上，可接入 Telegram/Discord/Slack/WhatsApp/Signal/Email 等消息网关，并自带 Web 控制台（dashboard，默认端口 **9119**）。

- 数据目录：`~/.hermes/`（认证配置在 `~/.hermes/.env`，权限 600）
- 服务托管：**两个用户态 systemd 服务**，无需 sudo
  - `hermes-gateway.service`——消息网关（出站连接）
  - `hermes-dashboard.service`——Web 控制台，监听 `0.0.0.0:9119`，Basic Auth 认证
- 运行时：安装脚本自动配置 Python、Node.js、ripgrep、FFmpeg

## 前置要求

- Linux（systemd）；官方也支持 macOS/Windows/WSL2/Termux，但本指南面向 Linux 服务器
- 可访问外网（脚本从 hermes-agent.nousresearch.com 下载）
- 无需 root 权限

## 安装（应用面板一键执行）

| 步骤 | 命令 | 说明 |
|---|---|---|
| 1. 安装 | `curl -fsSL https://hermes-agent.nousresearch.com/install.sh \| bash -s -- --non-interactive` | 官方一键脚本的非交互模式：跳过所有需要输入的阶段（setup/gateway），适合自动化 |
| 2. 网关服务 | 写入 `~/.config/systemd/user/hermes-gateway.service`（`ExecStart=<hermes 绝对路径> gateway run`） | 面板自动执行 |
| 3. dashboard 认证 | 向 `~/.hermes/.env` 写入 `HERMES_DASHBOARD_BASIC_AUTH_USERNAME=admin`、`HERMES_DASHBOARD_BASIC_AUTH_PASSWORD=<密码>`、`HERMES_DASHBOARD_BASIC_AUTH_SECRET=<openssl 随机签名>`，权限 600 | 密码即安装向导中输入的值 |
| 4. dashboard 服务 | 写入 `~/.config/systemd/user/hermes-dashboard.service`（`EnvironmentFile=%h/.hermes/.env`，`ExecStart=<hermes> dashboard --no-open --host 0.0.0.0 --port 9119`） | 非 loopback 绑定强制要求认证，`.env` 即为其提供 |
| 5. 启动 | `systemctl --user enable --now hermes-gateway hermes-dashboard` | 失败自动打印最近日志 |
| 6. 验证 | `curl http://127.0.0.1:9119/api/status` | 有响应即成功 |

> **密码说明**：dashboard 登录为用户名 `admin` + 安装时输入的密码（强度自行保证）。安装成功后密码自动保存到面板"登录凭证"，打开凭证对话框即可查看。固定会话签名（`HERMES_DASHBOARD_BASIC_AUTH_SECRET`）让服务重启后浏览器登录态仍然有效。
> **非交互模式说明**：`--non-interactive` 只跳过 9 个安装阶段中的最后两个交互阶段——`setup`（模型/API key 配置，必须人工输入）与 `gateway`（Hermes 自带的网关服务自动安装，我们用自己的 systemd unit 替代，避免同名冲突）；CLI、Python 环境与运行时依赖照常完整安装。
> **模型配置**：安装后打开 dashboard（http://<服务器IP>:9119，admin + 安装时设置的密码），在图形界面的模型设置里选择供应商/模型/填 API key——与终端的 `hermes setup` 写同一份 `~/.hermes/config.yaml`，功能等价（dashboard 的模型选择器就是 setup 交互选择器的 REST 版）。注意变更只对新会话生效；也可在终端执行 `hermes setup` 代替。

### 让服务在 SSH 退出后继续运行

用户态服务默认随用户会话回收。执行一次（需要 sudo，安装第 5 步会尽量自动完成）：

```bash
sudo loginctl enable-linger $USER
```

## 服务管理（用户态）

```bash
systemctl --user start hermes-gateway hermes-dashboard       # 启动
systemctl --user stop hermes-gateway hermes-dashboard        # 停止
systemctl --user restart hermes-gateway hermes-dashboard     # 重启
systemctl --user status hermes-gateway hermes-dashboard      # 运行状态
hermes doctor                                                # 官方诊断命令
```

## 状态与监控指标

面板"服务状态"卡按 `app.json` 的 `ops.status` 脚本采集：网关服务状态、网关进程 PID、dashboard 端口 9119、网关运行时长与内存、dashboard 是否监听（`ss -tln`）、健康检查（`curl http://127.0.0.1:9119/api/status` 有响应即 ok）、版本号。

## 日志

```bash
journalctl --user -u hermes-gateway -u hermes-dashboard -n 200 --no-pager
hermes doctor                                                # 诊断信息
```

## 端口与安全

dashboard 监听 `0.0.0.0:9119` 供浏览器直接访问，**认证由 Basic Auth 强制**（`HERMES_DASHBOARD_BASIC_AUTH_*`，非 loopback 绑定无认证时 hermes 会拒绝启动）。防火墙放行：

```bash
sudo firewall-cmd --permanent --add-port=9119/tcp && sudo firewall-cmd --reload   # firewalld
sudo ufw allow 9119/tcp                                                          # ufw
# 云服务器还需在控制台安全组放行 TCP 9119
```

公网暴露仍建议限制来源 IP。本地 Hermes Desktop 连接：设置 → 网关 → 远程网关 → 输入 `http://<服务器 IP>:9119`，用 `admin` + 安装时设置的密码登录。

## 升级与卸载

```bash
hermes update                                   # 官方升级命令
systemctl --user reset-failed hermes-gateway hermes-dashboard
systemctl --user restart hermes-gateway hermes-dashboard

systemctl --user disable --now hermes-gateway hermes-dashboard   # 停止并注销
rm -f ~/.config/systemd/user/hermes-gateway.service ~/.config/systemd/user/hermes-dashboard.service
systemctl --user daemon-reload
rm -f $(command -v hermes)                      # 卸载程序（保留 ~/.hermes 数据与 .env 认证配置）
```

## 常见问题

- **`sudo: A terminal is required to authenticate`**：应用面板的执行通道是非交互 SSH（无 TTY），无法为 sudo 输入密码。服务托管全程使用用户态 systemd，不需要 sudo；仅 `enable-linger` 需要，可手动执行一次。
- **dashboard 打不开 / 401**：确认 `.env` 三个 `HERMES_DASHBOARD_BASIC_AUTH_*` 变量在（`cat ~/.hermes/.env`），用户名 `admin`、密码为安装时输入的值；`systemctl --user restart hermes-dashboard` 后重试。
- **改密码**：编辑 `~/.hermes/.env` 中的 `HERMES_DASHBOARD_BASIC_AUTH_PASSWORD`，然后 `systemctl --user restart hermes-dashboard`，并同步更新面板"登录凭证"。
- **服务反复重启**：`journalctl --user -u hermes-gateway -n 100`（或 dashboard）看退出原因；常见为未配置模型（在 dashboard 图形界面完成模型配置）或 Python 环境损坏（重跑安装脚本）。
- **PATH 找不到 hermes**：unit 里写的是安装时的绝对路径；若移动了安装位置，修改两个 unit 的 `ExecStart` 后 `systemctl --user daemon-reload`。
- **SSH 断开后服务停止**：执行 `sudo loginctl enable-linger $USER`。

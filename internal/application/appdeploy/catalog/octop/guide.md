# Octop 部署与运维指南

> 本指南与 `app.json` 操作手册一一对应：安装/启动/停止/重启/状态/日志/开放端口/升级/卸载的每一步都有可直接执行的命令，应用面板按 `app.json` 自动执行；本文供人工排障与 AI 智能体参考。

## 概述

[Octop](https://github.com/TencentCloud/Octop) 是腾讯开源的自托管多用户多智能体 AI 助手，单进程部署（Python/uv），内置 Web 面板（默认端口 **8088**）、智能体管理与 IM 渠道接入。

- 数据目录：`~/.octop/`（SQLite 数据库、JWT、日志 `~/.octop/logs/`）
- 服务托管：**用户态 systemd**（`octop service start` 自动检测，unit 位于 `~/.config/systemd/user/octop.service`），无需 sudo
- 官方安装脚本：`https://finnie-1258344699.cos.ap-guangzhou.myqcloud.com/octop/install.sh`

## 前置要求

- Linux（systemd）或 macOS；建议 2 核 4G 以上
- 可访问外网（安装脚本从腾讯云 COS 下载）
- 无需预装 Python——安装器使用 uv 在 `~/.octop/` 隔离 venv 中自动配置

## 安装（应用面板一键执行）

| 步骤 | 命令 | 说明 |
|---|---|---|
| 1. 安装 | `curl -fsSL https://finnie-1258344699.cos.ap-guangzhou.myqcloud.com/octop/install.sh \| bash` | 下载 octop 可执行文件并加入 PATH |
| 2. 初始化 | `octop init --admin-username admin --admin-password <密码> --yes` | 非交互创建 SQLite 数据库、JWT secret 与管理员账号（密码 ≥8 位，字母+数字）。若 `~/.octop` 已存在会提示已初始化；需要全新安装请先备份并手动清理该目录 |
| 3. 启动服务 | `octop service start` + 用户态 drop-in 设置 `OCTOP_BIND_HOST=0.0.0.0` | 安装并启动 `octop.service`（用户态），监听 `0.0.0.0:8088`（支持外部访问面板） |
| 4. 验证 | `curl -fsS http://127.0.0.1:8088/api/health` | 健康检查接口返回即成功 |

> 安装面板中的"管理员初始密码"输入会自动填入第 2 步；安装完成后可在面板"登录凭证"中保存 Web 登录信息。

## 服务管理（用户态）

```bash
octop service start                     # 启动（自动按用户态注册 systemd 服务）
octop service stop                      # 停止
octop service restart                   # 重启
octop service status                    # 查看安装/运行/健康状态（scope: user）
systemctl --user status octop           # 用户态 systemd 视角
```

### 启动失败："start of the service was attempted too often"

systemd 的启动频率限制：服务连续崩溃被重启过多（默认 5 次/10 秒）后进入
`failed` 并拒绝再启动。按以下顺序恢复：

```bash
systemctl --user reset-failed octop.service    # 清零启动计数（关键）
octop service stop 2>/dev/null || true          # 停掉残留
fuser -k 8088/tcp 2>/dev/null || true           # 清理占用端口的残留进程
octop service start
journalctl --user -u octop -n 50 --no-pager    # 若仍失败，看崩溃原因
```

应用面板的"启动/重启"按钮已内置 `reset-failed`；安装第 3 步也会自动清理
启动限流并在失败时打印最近日志。

## 状态与监控指标

面板"服务状态"卡按 `app.json` 的 `ops.status` 脚本采集：服务状态（unit 文件在 `~/.config/systemd/user/` 则走 `systemctl --user`，否则系统态）、进程 PID、监听端口 8088、运行时长、内存占用、健康检查（HTTP `/api/health`）、版本号。

## 日志（用户态）

```bash
journalctl --user -u octop -n 200 --no-pager   # 用户态服务日志
tail -n 200 ~/.octop/logs/*.log                # 文件日志（前台方式运行时）
```

## 外部访问

服务通过**用户态** drop-in（`~/.config/systemd/user/octop.service.d/10-bind.conf`）设置 `OCTOP_BIND_HOST=0.0.0.0`，监听所有网卡（若 unit 被安装到系统态且有免密 sudo，则写系统态 drop-in）。需要放行防火墙/安全组 TCP 8088（见下"开放端口"）。面板自带管理员登录，但公网暴露仍建议限制来源 IP 或走反向代理 + HTTPS。

> 用户态服务随用户会话存在：服务器场景下 SSH 全部退出后服务可能被回收，建议执行一次 `sudo loginctl enable-linger $USER` 让用户态服务常驻。

## 开放端口

```bash
# firewalld（CentOS/OpenCloudOS 等，需 root，免密 sudo 时面板会自动加 sudo）
firewall-cmd --permanent --add-port=8088/tcp && firewall-cmd --reload
# ufw（Ubuntu/Debian）
sudo ufw allow 8088/tcp
# 云服务器还需在控制台安全组放行 TCP 8088
```

## 升级与卸载

```bash
octop update              # 升级程序（保留 ~/.octop 数据），升级后重启服务
octop backup              # 跨版本升级前建议先备份
octop service stop        # 停止服务
systemctl --user disable --now octop.service         # 注销用户态服务
rm -rf ~/.config/systemd/user/octop.service.d        # 移除 drop-in（含 0.0.0.0 绑定）
rm -f ~/.config/systemd/user/octop.service && systemctl --user daemon-reload
pip uninstall -y octop    # 或 pipx uninstall octop（保留 ~/.octop 数据）
```

## 常见问题

- **面板打不开**：先 `octop service status` 看健康状态；再确认 8088 已监听（`ss -tlnp | grep 8088`）与防火墙/安全组放行。
- **启动报 "attempted too often"**：systemd 启动限流。执行 `systemctl --user reset-failed octop.service` 后重试（面板"启动"按钮已内置），仍失败则 `journalctl --user -u octop -n 50` 查看崩溃原因。
- **忘记管理员密码**：删除 `~/.octop/` 后重新 `octop init`（会清空数据），或参考官方文档重置。
- **修改端口**：编辑 drop-in 增加 `Environment=OCTOP_PORT=<端口>` 后 `systemctl --user daemon-reload && octop service restart`，并同步更新面板监控端口。
- **服务随 SSH 退出而停止**：执行 `sudo loginctl enable-linger $USER` 开启用户级常驻。

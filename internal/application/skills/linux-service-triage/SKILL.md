---
name: linux-service-triage
description: Linux 服务故障排查：日志与 systemd/PM2 状态、文件权限、Nginx 反向代理与 DNS 逐段验证，输出最小修复方案
---
# Linux 服务排障（logs / systemd / PM2 / 权限 / Nginx / DNS）

## PURPOSE
Diagnoses common Linux service issues using logs, systemd/PM2, file permissions, Nginx reverse proxy checks, and DNS sanity checks.

## WHEN TO USE
- 服务起不来 / 不可达 / 行为异常，需要从日志定位并给出修复命令
- 重启应用并确认它监听在正确的端口上
- 修正目录权限让服务能安全读写
- 为某个端口配置 Nginx 反向代理，并验证 DNS / TLS
- 把脚本做成 systemd 服务并设置开机自启

不适用：内核级调试、深度性能剖析（另用性能分析类手册）。

## WORKFLOW
1. **确认范围与安全**：服务名（systemd unit 或 PM2 进程）、是否允许变更。
2. **采集证据**：状态输出 + 近期日志（命令速查见下节）。
3. **故障分类**：配置错误 / 依赖缺失 / 权限拒绝 / 端口冲突 / 上游不可达 / DNS 不匹配。
4. **给出最小修复方案 + 验证步骤**。
5. **Web 服务验证链路**：应用监听 → Nginx 代理 → DNS 解析 →（TLS 抽查）。
6. **重启/重载计划**，并确认健康检查通过。
7. **停下询问用户**，当：日志/状态缺失、需要未确认的特权操作、TLS 证书体系未知。

## OUTPUT FORMAT
```text
TRIAGE REPORT
- Symptom:
- Evidence (what you provided):
- Most likely cause:
- Fix plan (minimal steps):
- Exact commands (ONLY if user approved changes):
- Verification:
- Rollback:
```

## Command cheat-sheet (safe first)

### Logs
- systemd: `journalctl -u <service> -n 200 --no-pager`
- live: `journalctl -u <service> -f`
- PM2: `pm2 logs <name> --lines 200`

### Status
- systemd: `systemctl status <service> --no-pager`
- ports: `ss -ltnp | grep <port>`
- processes: `ps aux | grep <name>`

### Permissions
- `ls -la <path>`
- `namei -l <path>` (checks each directory in path)

### Nginx
- config test: `nginx -t`
- reload: `systemctl reload nginx`
- logs: `/var/log/nginx/access.log`, `/var/log/nginx/error.log`

### DNS sanity
- `dig +short <host>`
- `dig +trace <host>`

## SAFETY & EDGE CASES
- Read-only by default: diagnose from provided outputs; do not assume you can run changes.
- Avoid destructive changes; require explicit confirmation for anything risky.
- Prefer `nginx -t` before reload and verify ports with `ss`.

## EXAMPLES
- Input: "journal shows permission denied on /var/app/uploads."
  Output: path permission analysis + safe chown/chmod plan + verification.
- Input: "App works locally but domain returns 502."
  Output: upstream port checks + nginx error log interpretation + proxy_pass fix plan.

---

> 来源：改编自 skillhub.cn 收录的 linux-service-triage（clawhub @kowl64），许可证 MIT-0。
> 其 references/triage-commands.md 已内联为上方速查节；变更操作在本产品中需用户确认后执行。

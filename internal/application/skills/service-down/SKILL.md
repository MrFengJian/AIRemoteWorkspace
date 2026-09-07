---
name: service-down
description: systemd 服务启动失败、反复重启或状态异常，从状态与日志定位根因
---
# 服务异常（systemd）

## 1. 确认服务状态

```sh
systemctl status <unit> --no-pager -l
systemctl list-units --state=failed --no-legend --plain
```

状态含义：

- `active (running)` 正常；`active (auto-restart)`/`status=203/EXEC` 反复拉起失败；
- `failed` 看最近的退出码：203=可执行文件/权限问题，1=应用报错，137/SIGKILL 常为 OOM 或人为 kill，
  转memory-oom 场景核对 dmesg。

## 2. 看日志（最重要的证据）

```sh
journalctl -u <unit> -n 100 --no-pager
journalctl -u <unit> --since "1 hour ago" -p err --no-pager
```

典型错误：

- 绑定端口被占：`Address already in use` → `ss -ltnp | grep <port>` 找占用者；
- 配置错误：`Failed to parse`/配置项报错 → 定位到具体行再查配置文件；
- 权限/路径：`Permission denied`、`No such file or directory` → 核对
  `systemctl cat <unit>` 里的 User/WorkingDirectory/ExecStart 路径；
- 依赖未就绪：`Depends:` 链上的其他 unit，`systemctl list-dependencies <unit>`。

## 3. 交叉验证

- 资源限制：`systemctl show <unit> -p LimitNOFILE,TasksMax,MemoryMax`；
- 单元文件最近是否改过：`systemctl cat <unit>`；改过未生效需
  `systemctl daemon-reload`（WRITE）。

## 4. 处置（全部等用户确认）

- `systemctl restart <unit>`（WRITE）、`systemctl daemon-reload`（WRITE）；
- 开机自启修正：`systemctl enable <unit>`（WRITE）；
- 禁止在未查明根因前盲目 restart 循环里的服务。

输出按 现象/根因/证据/建议/风险 组织，引用 journalctl 的关键行作为证据。

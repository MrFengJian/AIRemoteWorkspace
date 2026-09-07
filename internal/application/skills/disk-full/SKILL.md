---
name: disk-full
description: 磁盘空间不足或写满，找出大文件/大目录并给出安全的清理建议
---
# 磁盘空间满 / 不足

## 1. 确认哪个分区满

```sh
df -h
df -i
```

- 关注使用率 > 90% 的挂载点；`df -i` inode 用满（大量小文件）时删文件才有效，
  空间看不出来的满就是 inode 满。
- 已删除但未释放：`lsof +L1 2>/dev/null | head -15` 或 `lsof | grep deleted | head`，
  常见于日志被删而进程仍持有句柄——需重启/重载对应服务才真正释放。

## 2. 逐层定位大目录

```sh
du -xh --max-depth=1 / 2>/dev/null | sort -rh | head -15
du -xh --max-depth=2 /var 2>/dev/null | sort -rh | head -15
```

对可疑目录继续下钻（日志、dump、镜像缓存、上传目录）。

## 3. 常见可安全清理项（先报告，再等确认）

- 旧内核日志/压缩日志：`ls -lh /var/log | head`；
- journal 日志：`journalctl --disk-usage` → 建议 `journalctl --vacuum-size=200M`（WRITE）；
- 包管理缓存：`apt clean` / `yum clean all` / `dnf clean all`（WRITE）；
- Docker：`docker system df` → `docker image prune`（DANGEROUS，必须确认）；
- core dump：`ls -lh /var/crash /var/lib/systemd/coredump 2>/dev/null`。

## 4. 风险

- 删除任何业务文件、truncate 在写的日志前必须说明后果并等用户确认；
- `rm -rf` 一律 DANGEROUS；建议优先用 truncate/重定向清空而非删除句柄占用的文件。

输出按 现象/根因/证据/建议/风险 组织，清理动作全部等用户批准。

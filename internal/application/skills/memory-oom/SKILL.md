---
name: memory-oom
description: 内存耗尽、swap 打满或进程被 OOM Killer 杀死，定位吃内存的进程与 OOM 记录
---
# 内存不足 / OOM

## 1. 确认内存水位

```sh
free -m
cat /proc/pressure/memory 2>/dev/null
swapon --show
```

- available 很低且 swap used 高 → 内存确实吃紧；
- kswapd CPU 高、PSI some/full 值大 → 正在颠簸（thrashing）。

## 2. 查 OOM 记录（确认是否发生过）

```sh
dmesg -T 2>/dev/null | grep -iE "oom|out of memory" | tail -20
journalctl -k --since "24 hours ago" 2>/dev/null | grep -i oom | tail -20
```

记录被杀进程名与时间点，和用户描述的症状对时间。

## 3. 找到吃内存的进程

```sh
ps aux --sort=-%mem | head -15
smem -tk 2>/dev/null | head -15
```

注意：`ps` 的 RSS 会重复统计共享库，多进程程序（java、chrome、golang 多进程）
用 smem 或把同组进程 RSS 相加评估。

## 4. 常见根因

- 业务内存泄漏：进程 RSS 缓慢爬升、从不回落 → 建议重启（WRITE）+ 排查泄漏；
- 缓存配置过大（mysql/jvm/redis）：查对应配置 `Xmx`、`innodb_buffer_pool_size`、`maxmemory`；
- 突发并发/大查询：结合业务日志时间点；
- 无 swap 或 swap 过小：`swapon --show` 为空时瞬时尖峰直接 OOM。

## 5. 输出

按 现象/根因/证据/建议/风险 组织。调参、加 swap、重启进程均为写操作，
给出具体命令与影响，等待用户确认后执行。

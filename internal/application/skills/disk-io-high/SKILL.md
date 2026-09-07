---
name: disk-io-high
description: iowait 高或磁盘 IO 打满，找出高 IO 进程与慢盘，区分顺序/随机与读写来源
---
# 磁盘 IO 高

## 1. 确认 IO 压力

```sh
iostat -x 1 3 2>/dev/null || cat /proc/diskstats
cat /proc/pressure/io 2>/dev/null
vmstat 1 3
```

- `%util` 接近 100、`await` 远大于通常值（HDD>20ms / SSD>5ms 视为慢）→ IO 瓶颈；
- PSI io some/full 高 → 任务在等 IO；
- `%wa` 高而 CPU 其余空闲，与体检快照的 CPU iowait 对照。

## 2. 找出高 IO 进程

```sh
pidstat -d 1 3 2>/dev/null | head -25 || iotop -boqqqn -d 2 2>/dev/null | head -25
```

没有 pidstat/iotop 时，用 /proc 近似：

```sh
for f in /proc/[0-9]*/io; do rb=$(awk '/read_bytes/{print $2}' $f 2>/dev/null); wb=$(awk '/write_bytes/{print $2}' $f 2>/dev/null); [ -n "$rb" ] && echo "$((rb+wb)) ${f}"; done | sort -rn | head -10
```

（两次执行对比增量更准；只读采集。）

## 3. 判断 IO 类型

```sh
cat /proc/meminfo | grep -iE "dirty|writeback"
free -m
```

- Dirty 很大 → 大量写回，多为日志/落盘风暴；
- 内存充足仍在换页 → 转 memory-oom 场景（kswapd 引起的假 IO 瓶颈）。

## 4. 常见根因

- 日志刷写过大/过于频繁（结合 disk-full 场景的目录体积）；
- 数据库落盘、备份任务/cron 在跑（核对任务时间点）；
- 虚拟机/云盘性能不足（iops 限额），`iostat -x` 的 `w/s` 不高但 `await` 很高时要怀疑配额；
- 磁盘硬件劣化：

```sh
dmesg -T 2>/dev/null | grep -iE "i/o error|ata[0-9]|blk_update" | tail -10
smartctl -H /dev/<盘> 2>/dev/null || true
```

## 5. 输出

按 现象/根因/证据/建议/风险 组织。调整调度器/限流/迁移数据均为写操作，
给方案等确认。

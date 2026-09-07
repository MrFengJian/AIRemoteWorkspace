---
name: cpu-high
description: CPU 使用率持续偏高，定位占用进程并判断是业务负载还是异常进程
---
# CPU 使用率高

按决策树排查，先只读取证，再下结论。

## 1. 确认整体负载

```sh
uptime
top -bn1 | head -20
nproc
```

- load1 持续 > 核数 × 0.7 视为高负载；对比体检快照里的 CPU% 判断是否仍在发生。
- `%wa`（iowait）高 → 转磁盘 IO 场景（disk-io-high）；`%si` 高 → 检查网络/软中断。

## 2. 找到占用进程

```sh
ps aux --sort=-%cpu | head -15
```

对头号进程判断类型：

- 业务进程（java/nginx/自家服务）→ 看它是否该忙：并发量、定时任务、慢查询；
  `pidstat -p <PID> 1 5`（如有）区分用户态/内核态。
- 系统进程异常（kworker 占满、kswapd 活跃）→ kswapd 活跃说明内存吃紧，转 memory-oom 场景。
- 陌生/可疑进程（名字随机、路径在 /tmp、挖矿特征）→ 不要直接 kill，
  先 `ls -l /proc/<PID>/exe`、`cat /proc/<PID>/cmdline` 取证并报告用户。

## 3. 常见根因

- 定时任务风暴：`grep -r . /etc/cron.d/ 2>/dev/null | head`、`crontab -l`；
- 进程失控/死循环：确认线程数 `ls /proc/<PID>/task | wc -l`；
- 挖矿木马：结合 CPU 高 + 陌生二进制 + 计划任务持久化。

## 4. 输出

按 现象/根因/证据/建议/风险 给出结论。处置（kill、调参、重启服务）属 WRITE/DANGEROUS，
只提建议并等用户确认，不要自行执行。

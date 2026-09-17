# 值守指引（Heartbeat）

系统层的例行节奏（作为建议提出，不要自行循环执行）：

1. **接入主机时的系统快检**（可主动提议一次）：uptime/load、`free -m`、`df -h`（关注 >80% 分区）、`iostat -x 1 3`（如可用）、`systemctl --failed`、最近 dmesg OOM 记录。
2. **周期性关注点**（发现迹象时建议落地）：
   - journald 日志体积：建议 `SystemMaxUse` 上限与 vacuum 策略；
   - 定时任务健康：cron/systemd timer 失败项（`systemctl list-timers --all`、cron 日志）建议补输出重定向与告警；
   - 内核参数变更：给出 sysctl.d 片段 + 持久化位置 + 回滚值。
3. **基线沉淀**：为主机记录一份「正常水位」快照（负载/内存/IO 区间），后续对比异常更敏感。

升级条件：卸载/格式化文件系统、修改 SSH/PAM 认证配置、内核参数大改、包管理器大版本升级——先给风险评估与回滚路径，等用户确认；SSH 配置变更必须提醒保持当前会话不断开再验证。

# 值守指引（Heartbeat）

例行巡检节奏（向用户提出巡检计划，不要自行循环执行）：

1. **每日巡检清单**（用户接入集群会话时可主动提议一次）：
   - `kubectl get nodes`：NotReady 节点与 pressure 条件；
   - `kubectl get pods -A --field-selector=status.phase!=Running`：Pending/Failed 汇总；
   - `kubectl get events -A --sort-by=.lastTimestamp | tail`：近期 Warning；
   - 重启次数异常增长的 Pod（RESTARTS 增速）；
   - 证书与租约到期（kubeadm 集群可查 `kubeadm certs check-expiration`，如可用）。
2. **发布窗口纪律**：滚动更新期间盯 rollout status 与 events，确认旧副本回收、无 502 迹象再收尾。
3. **容量节奏**：节点 Allocated resources 接近 requests 上限时，提醒扩容或修正 requests；DiskPressure 出现过即建议镜像/日志清理策略。

升级条件：控制面组件异常、etcd 报警、多节点同时 NotReady、需要 drain/cordon 或删除有状态负载时——一律先给影响评估与回滚方案，等用户确认。

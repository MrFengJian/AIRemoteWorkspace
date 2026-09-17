# 值守指引（Heartbeat）

发布与迭代的例行节奏（作为建议提出，不要自行执行变更）：

1. **发布前检查清单**（用户提到发版时主动给一遍）：
   - 镜像 tag 固定（不用 latest）、imagePullSecrets 就位；
   - requests/limits 齐全、readinessProbe 反映真实可服务、livenessProbe 不依赖下游；
   - rollingUpdate maxUnavailable/maxSurge 与 PodDisruptionBudget 合理。
2. **发布后复查节奏**：rollout status 完成 → 新副本 Ready → 无 CrashLoopBackOff/OOMKilled 事件 → 错误日志无新增模式；建议用户观察一个完整业务高峰再收尾。
3. **迭代复盘建议**：发布出过一次事故的配置点（探针阈值、资源配额、优雅停机）建议固化为模板或准入检查。

升级条件：需要 apply/upgrade/rollout undo 等变更操作时，给出 YAML diff 与回滚命令，等用户确认；涉及删除数据卷（PVC）或 Secret 轮换时必须额外强调不可逆风险。

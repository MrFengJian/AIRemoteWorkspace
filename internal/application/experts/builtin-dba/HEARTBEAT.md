# 值守指引（Heartbeat）

数据库的例行节奏（作为建议提出，不要自行执行任何变更）：

1. **接入主机时的只读体检**（可主动提议一次）：processlist 连接水位与运行时长 Top、`SHOW ENGINE INNODB STATUS`（或 pg_locks）等待情况、慢查询日志最近条目、磁盘剩余空间（数据目录所在分区）。
2. **周期性关注点**（发现迹象时建议落地）：
   - 慢查询集中出现：建议 EXPLAIN 复核与索引方案（附写入放大与表体量评估）；
   - 复制延迟：从库 `Seconds_Behind_Master`/`pg_stat_replication` 趋势，追因（大事务/单线程回放/网络）；
   - 备份有效性：提醒最近一次备份的可恢复性验证（恢复演练建议，不由 Agent 代替执行）；
   - Redis：maxmemory 水位与淘汰策略命中情况、RDB/AOF 持久化状态。
3. **变更纪律**：任何 DDL/数据订正先给「备份 → 变更 → 验证 → 回滚」四步方案，大表 DDL 附 online 方案与锁影响评估。

升级条件：DROP/TRUNCATE/大批量 UPDATE·DELETE、主从切换、参数重启级变更（innodb_buffer_pool 等）、删除备份——一律停在工作方案，等用户确认；凭据永不回显。

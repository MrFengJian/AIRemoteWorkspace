# 值守指引（Heartbeat）

容器环境的例行节奏（作为建议提出，不要自行循环执行）：

1. **接入主机时的容器体检**（可主动提议一次）：`docker ps -a` 状态汇总、Exited/重启次数异常的容器、`docker stats --no-stream` 资源水位、`docker system df` 磁盘占用。
2. **周期性关注点**（发现迹象时建议落地）：
   - 日志膨胀：提醒配置 logging driver 的 max-size/max-file 或轮转；
   - 镜像/卷堆积：建议 `docker image prune`/`volume prune` 的白名单化清理方案（强调确认再删）；
   - 重启策略：关键业务容器建议 `restart=unless-stopped` + healthcheck。
3. **compose 栈巡检**：depends_on + healthcheck 是否成对出现；端口冲突与网络隔离建议。

升级条件：删除容器/卷/镜像、compose down（可能中断业务）、修改生产容器启动参数——先给影响面与数据保留说明，等用户确认。

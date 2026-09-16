---
name: k8s-pod-troubleshoot
description: K8s Pod 异常排查决策树：CrashLoopBackOff / OOMKilled / Pending / ImagePullBackOff / Terminating 不退 / Service 不通，按"调度→容器→网络→应用"四层分流，给出 kubectl 命令与判读标准
---
# Pod 异常排查（症状驱动）

总原则：先确定"哪一层坏了"再深入——调度层 → 容器运行层 → 网络层 → 应用层。
只读命令优先；Events 里的 Reason 往往就是答案，不要靠猜。

## 1. 60 秒开局（任何症状先跑这三条）

```bash
kubectl get pod -n <ns> -o wide                 # 状态 / 重启次数 / 所在节点
kubectl describe pod <pod> -n <ns> | tail -40    # Events 是第一答案来源
kubectl logs <pod> -n <ns> --previous --tail=100 # --previous 看崩溃前那一次
```

判读要点：`RESTARTS` 增长速度、Events 的 `Reason`、`Last State: Terminated` 的 Exit Code。

## 2. 按状态分流

### CrashLoopBackOff → 看 Exit Code

- `0`：主进程正常退出——一次性任务误写成 Deployment？考虑改 Job/CronJob
- `1` / `2`：应用自身异常——看 `--previous` 日志，检查配置（环境变量、ConfigMap/Secret 未挂载）
- `137`：被 SIGKILL → 转 OOM 分支
- `143`：SIGTERM——优雅关闭超时，检查 preStop 与 terminationGracePeriodSeconds
- `126` / `127`：命令不可执行 / 不存在——entrypoint 路径错，或镜像架构不匹配（arm64/amd64）

### OOMKilled（137）→ 先分层，两种处置完全不同

```bash
kubectl describe pod <pod> -n <ns> | grep -A3 "Last State"   # 容器级：Reason: OOMKilled
kubectl describe node <node> | grep -i -A5 pressure           # 节点级：MemoryPressure=True
```

- 容器级：调大 limit 或修内存泄漏。JVM 默认不感知 cgroup limit，需设
  `-XX:MaxRAMPercentage=75`（否则按宿主机内存算堆，必被 OOMKill）
- 节点级：是 requests 过低导致超卖——修 requests 而不是 limits

### Pending → 三查

```bash
kubectl describe pod <pod> -n <ns> | grep -A10 Events   # FailedScheduling 原因
kubectl describe node <node> | grep -A5 "Allocated resources"
kubectl get pvc -n <ns>                                  # PVC 未绑定也会卡 Pending
```

常见根因：requests 过大（资源不足）、亲和性/污点不匹配、无可用 PV、nodeSelector 标签写错。

### ImagePullBackOff → 依次排除

镜像 tag 是否存在 → 私仓 `imagePullSecrets` 是否挂在同一命名空间（Secret 不跨 ns）→
节点能否出网/走代理 → 镜像架构是否匹配节点。

### Terminating 不退（超过宽限期）

进程忽略 SIGTERM（PID 1 是 shell 而非应用、信号未转发，入口改用 `exec` 形式），
或 finalizer 卡住：

```bash
kubectl get pod <pod> -n <ns> -o jsonpath='{.metadata.finalizers}'
```

## 3. Service 不通：五跳逐层，别跳步

1. Pod 自身能否响应：`kubectl exec -it <pod> -n <ns> -- curl -sv localhost:<port>/healthz`
2. Endpoints 是否有 IP：`kubectl get endpoints <svc> -n <ns>`——**为空是最高频根因**
   （selector 与 label 不匹配，或 readiness 一直没通过）
3. 集群内经 ClusterIP 访问（可在临时 Pod 里 curl `<svc>.<ns>:<port>`）
4. DNS 解析：`nslookup <svc>.<ns>.svc.cluster.local`（失败查 CoreDNS Pod）
5. Ingress 规则与后端：`kubectl describe ingress <ing> -n <ns>`

## 4. 探针配置自查（配错会自己把服务打挂）

- `startupProbe` 保护慢启动应用；没有它而 liveness 的 initialDelay 太短
  = 启动期被反复杀，表现恰好是 CrashLoopBackOff
- `readinessProbe` 只决定是否进 Endpoints：失败摘流量、不重启；应反映"可服务"
- `livenessProbe` 只查进程自身是否僵死，**绝不能依赖下游**（下游抖动会引发全量重启雪崩）

## 5. 节点级：NotReady / DiskPressure / 驱逐

```bash
kubectl get node
kubectl describe node <node> | grep -i -A8 conditions
kubectl top node; kubectl top pod -n <ns> --sort-by=memory   # 依赖 metrics-server
```

- `DiskPressure`：先清镜像与日志；`NotReady`：查 kubelet 与容器运行时
- 驱逐顺序 BestEffort → Burstable → Guaranteed；核心服务建议 requests == limits
- metrics-server 缺失时，改用 `kubectl describe node` 的 Allocated resources 估算水位

## 边界

- 只读命令（get/describe/logs/top/exec 只读探测）可直接执行；
  delete / scale / rollout / apply 等变更操作先给方案，等用户确认
- 强删 finalizer、`--grace-period=0`、删节点等都可能造成数据不一致，必须明确标注风险

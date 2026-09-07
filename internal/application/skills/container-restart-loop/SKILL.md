---
name: container-restart-loop
description: Docker 容器反复重启或一直退出，从退出码、日志和 OOM 记录定位原因
---
# 容器反复重启 / 异常退出

## 1. 确认容器状态与重启次数

```sh
docker ps -a --format "table {{.Names}}\t{{.Status}}\t{{.Image}}" | head -20
docker inspect <c> --format "RestartCount={{.RestartCount}} RestartPolicy={{.HostConfig.RestartPolicy.Name}} OOMKilled={{.State.OOMKilled}} ExitCode={{.State.ExitCode}} Error={{.State.Error}}"
```

退出码速查：137=SIGKILL（OOMKilled=true 即内存不足，转 memory-oom；
人为 docker kill 同码）；143=SIGTERM（正常停止信号）；126/127=命令不可执行/不存在；
1=应用自身报错。`Restarting (1) x seconds ago` 表示应用启动即崩。

## 2. 看容器日志（本次崩溃的直接证据）

```sh
docker logs --tail 100 <c> 2>&1 | tail -60
docker logs --previous <c> 2>&1 | tail -40 2>/dev/null
```

典型根因：配置/环境变量缺失、连不上依赖（DB/注册中心）、证书过期、
启动命令写错、端口冲突、挂载路径不存在。

## 3. 资源与宿主机层面

```sh
docker stats --no-stream | head -15
docker inspect <c> --format "MemLimit={{.HostConfig.Memory}} CPU={{.HostConfig.NanoCpus}}"
dmesg -T 2>/dev/null | grep -iE "oom|killed process" | tail -10
```

- 内存限制过小 → OOMKill 循环；查 `docker system df` 之外还要看宿主内存余量；
- 宿主机磁盘满会导致容器写日志/层失败退出，转 disk-full 场景。

## 4. 编排场景（compose/k8s）

```sh
docker compose -f <file> ps 2>/dev/null
kubectl get pod <p> -o wide 2>/dev/null; kubectl describe pod <p> 2>/dev/null | tail -30; kubectl logs --previous <p> --tail=50 2>/dev/null
```

CrashLoopBackOff 优先看 `describe` 的 Events 与 `--previous` 日志。

## 5. 处置（全部等用户确认）

- `docker restart <c>` / `docker compose up -d`（WRITE）；
- 改资源限制、镜像、配置需重建容器（WRITE/DANGEROUS 视操作）；
- 禁止用 `RestartPolicy=always` 掩盖启动失败。

输出按 现象/根因/证据/建议/风险 组织。

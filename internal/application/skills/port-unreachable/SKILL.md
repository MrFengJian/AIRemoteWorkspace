---
name: port-unreachable
description: 服务端口从外部访问不通，沿 监听→本机连通→防火墙→链路 四层排查
---
# 端口不通

先明确症状：连接被拒（RST）、超时（无响应），还是通但报错。沿四层排查：

## 1. 服务在监听吗

```sh
ss -ltnp | grep <port>
ss -ltnp
```

- 没监听 → 服务没起来/崩了，转 service-down 场景；
- 只监听 `127.0.0.1`/`::1` → 绑定地址问题，外部永远连不上，
  需改绑定到 `0.0.0.0` 或具体内网 IP（改配置属 WRITE）。

## 2. 本机自连通吗（隔离服务与网络）

```sh
curl -sv --max-time 3 http://127.0.0.1:<port>/ 2>&1 | head -15
timeout 3 bash -c "</dev/tcp/127.0.0.1/<port>" && echo OPEN || echo CLOSED
```

- 本机不通 → 服务/应用层问题（转服务日志）；
- 本机通、外部不通 → 继续 3、4。

## 3. 防火墙

```sh
sudo iptables -L -n | head -40 2>/dev/null
sudo nft list ruleset 2>/dev/null | head -40
systemctl is-active firewalld ufw 2>/dev/null
sudo iptables -vnL INPUT 2>/dev/null | head -20
```

注意：读规则 iptables/nft 一般免 sudo 也可；DROP/REJECT 命中即根因。
云主机还要提示用户检查安全组（本机看不到）。

## 4. 链路与网络

```sh
ip addr | grep -E "^[0-9]+:|inet "
ip route
ping -c 3 <网关或客户端IP>
```

- 容器场景：端口映射是否配置（`docker port <c>`）、宿主机转发是否开启；
- 监听在容器内而未映射端口是常见根因。

## 5. 输出

按 现象/根因/证据/建议/风险 组织。改防火墙规则（iptables -A/-D、ufw allow）
属 DANGEROUS，只给建议并说明影响面，等用户确认；改服务绑定地址属 WRITE。

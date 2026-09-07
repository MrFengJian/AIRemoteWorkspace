---
name: network-latency
description: 网络延迟高、丢包或访问慢，区分 本机/链路/对端/DNS 各段责任
---
# 网络延迟 / 丢包

## 1. 先分段定位

```sh
ping -c 5 <网关>
ping -c 5 <目标IP或域名>
ping -c 5 223.5.5.5 2>/dev/null || ping -c 5 8.8.8.8
```

- 到网关就丢包 → 本机/内网问题（网卡、交换机）；
- 到外网 IP 丢包 → 链路/运营商；
- ping IP 正常但域名慢 → DNS 问题，走第 4 步。

## 2. 链路路径

```sh
traceroute -n <目标> 2>/dev/null || tracepath -n <目标> 2>/dev/null | head -20
mtr -r -c 10 <目标> 2>/dev/null || true
```

看哪一跳开始出现 `Loss%` 高或延迟陡增——之后的跳都高，问题在那一跳之后。

## 3. 本机网卡与连接压力

```sh
ip -s link
ss -s
cat /proc/net/sockstat 2>/dev/null
ethtool <iface> 2>/dev/null | grep -E "Speed|Duplex"
```

- errors/dropped 持续增长 → 网卡/驱动/带宽打满；
- TIME_WAIT 数万、tcp tw 复用未开 → 连接未复用，应用短连接风暴；
- 半双工/百兆协商是老机器经典问题。

## 4. DNS

```sh
cat /etc/resolv.conf
time nslookup <业务域名> 2>/dev/null || time getent hosts <业务域名>
```

- 解析 >1s 或偶发失败 → 换 DNS/加缓存是解法（改 resolv.conf 属 WRITE）。

## 5. 带宽占用

```sh
cat /proc/net/dev
iftop -t -n -s 5 2>/dev/null | head -20 || true
```

结合体检快照的网卡收发速率判断是否打满带宽。

输出按 现象/根因/证据/建议/风险 组织，指明问题在 本机/链路/对端/DNS 哪一段。

---
name: login-slow
description: SSH 登录或 shell 打开很慢，从 DNS 反解、认证方式、PAM 模块逐段计时定位
---
# SSH 登录慢 / shell 卡顿

## 1. 分段计时（服务器侧本机验证）

```sh
time ssh -o BatchMode=yes -o StrictHostKeyChecking=no localhost true 2>&1 | tail -5
systemctl status sshd --no-pager | head -8
```

- 本机自连也慢 → 问题在 sshd/PAM/系统负载；
- 本机快、外部慢 → 网络/客户端侧，转 network-latency 场景思路。

## 2. DNS 反向解析（最常见根因）

```sh
grep -iE "^UseDNS|^GSSAPIAuthentication" /etc/ssh/sshd_config
cat /etc/resolv.conf
```

- `UseDNS yes` 且 DNS 不通/慢 → 每次登录卡 30s 左右，建议改 `UseDNS no`（WRITE，改后需 reload sshd）；
- `GSSAPIAuthentication yes` 且无 KDC → 客户端可 `-o GSSAPIAuthentication=no`，服务端亦可关闭。

## 3. 认证阶段慢

```sh
journalctl -u sshd -n 50 --no-pager 2>/dev/null || journalctl -u ssh -n 50 --no-pager
```

- 多个认证方式依次尝试失败（公钥→密码）属于客户端侧慢，检查客户端 `PreferredAuthentications`；
- PAM 模块卡顿（如 `pam_systemd`、`pam_loginuid`，或 LDAP/NIS 远程认证超时）：

```sh
grep -v "^#" /etc/pam.d/sshd | grep -v "^$"
```

## 4. 登录后 shell 卡顿

```sh
ls -l /etc/profile.d/ | head
tail -20 /etc/profile
ls -la ~/.bashrc ~/.bash_profile ~/.profile 2>/dev/null
```

- 登录脚本里的远程调用（curl 拉取、nvidia-smi、conda init）会拖慢首提示符；
- `time bash -lc true` 检查 login shell 初始化耗时。

## 5. 资源因素

负载过高导致登录慢（fork 慢、内存紧张）→ 结合体检快照转 cpu-high / memory-oom。

输出按 现象/根因/证据/建议/风险 组织。改 sshd_config 属 WRITE，
注明 `sshd -t` 校验后再 reload，避免锁死远程访问。

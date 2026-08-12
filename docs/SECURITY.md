# SECURITY — AI Remote Workspace

> 安全设计策略、Security Mode 与 Agent 安全模型

---

## 1. 安全设计策略

个人桌面版不强制企业级安全。

采用：

```
SQLite
+
Secret Storage
+
可选字段加密
```

而不是默认 SQLCipher。

### 1.1 数据分类

#### 普通数据

- Host 名称
- IP
- 端口
- 用户名
- 会话记录

保存：

```
SQLite
```

#### 敏感数据

- SSH Password
- API Key
- Token
- 私钥内容

保存：

- Windows Credential Manager
- macOS Keychain
- Linux Secret Service

数据库只保存：

```
secret_ref
```

---

## 2. Security Mode

提供三个模式，按使用场景灵活切换。

### 2.1 Convenience

适合个人服务器：

- 自动登录
- 保存密码

### 2.2 Balanced（默认）

推荐模式：

- SSH Key 使用文件路径
- 密码保存到系统密码库
- API Key 保存到系统密码库

### 2.3 Secure

高安全：

- 不保存密码
- 每次输入
- 不缓存敏感数据

---

## 3. Agent 安全

Agent 的安全重点**不是数据库加密**，而是 **Tool Permission**：
控制 Agent 能做什么、需要用户授权什么。

### 3.1 Tool Permission 分类

#### READ

- `ls`
- `ps`
- `df`
- `journalctl`

#### WRITE

- 修改文件
- 上传文件
- `restart service`

#### DANGEROUS

- `rm`
- `shutdown`
- `iptables`
- `docker prune`

### 3.2 危险操作流程

危险操作必须经过用户授权：

```
Agent
 ↓
Permission Check
 ↓
User Approval
 ↓
Execute
```

任何 WRITE / DANGEROUS 级别的 Tool 调用，都不得在未经用户批准的情况下执行。

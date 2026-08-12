# PRD — AI Remote Workspace

> 产品需求文档（Product Requirements Document）
> 个人桌面版 AI 运维 / 开发工作台

---

## 1. 项目定位

AI Remote Workspace 是一个 Go 原生、轻量级、跨平台的 AI 驱动远程机器工作台。

目标不是替代传统 SSH Client，而是将：

- SSH
- Terminal
- SFTP
- Local Shell
- AI Agent
- MCP
- Docker
- Kubernetes

统一到一个面向个人开发者的 AI 工作环境。

### 核心理念

```
AI
 |
Remote Context
 |
Tools
 |
Execution
 |
Diagnosis
```

---

## 2. 产品原则

### 轻量优先

目标用户：

- 个人开发者
- DevOps 工程师
- AI Agent 用户

### 优先体验

```
下载
 ↓
启动
 ↓
连接服务器
 ↓
开始工作
```

### 不优先引入

- 企业 RBAC
- SSO
- 团队管理
- 云端同步
- 复杂安全体系

---

## 3. MVP 功能范围

### 必须实现

- SSH
- Terminal
- Host Management
- Local Terminal
- SFTP
- AI Assistant
- Tool Calling

### 暂不实现

- 企业权限
- 云同步
- 完整 IDE
- 完整 K8s Dashboard

---

## 4. MVP 发布标准

满足：

- 单 Binary
- 启动快速
- SSH 稳定
- Terminal 稳定
- 多 Host 管理
- AI 基础诊断
- MCP 调用

---

## 5. 长期方向

### 最终产品

不是：

```
SSH Client
```

而是：

```
AI Remote Operator
```

### 典型用例

用户：

> 帮我检查 production 服务器 CPU 为什么高

Agent：

```
连接服务器
 ↓
检查 CPU
 ↓
分析进程
 ↓
查看日志
 ↓
给出原因
 ↓
执行修复
```

---

## 6. 核心护城河

不是：

- SSH
- Terminal
- SFTP

而是：

- Remote Context
- Tool Ecosystem
- Agent Runtime
- Permission Model
- Diagnosis Knowledge
- MCP Integration

### 最终定位

> 一个轻量、本地优先、AI 增强的个人开发者 Remote Workspace。

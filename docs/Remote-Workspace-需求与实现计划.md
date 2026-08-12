# AI Remote Workspace

## 个人桌面版 AI 运维 / 开发工作台需求与实现计划

## 1. 项目定位

AI Remote Workspace 是一个 Go 原生、轻量级、跨平台的 AI
驱动远程机器工作台。

目标不是替代传统 SSH Client，而是将：

-   SSH
-   Terminal
-   SFTP
-   Local Shell
-   AI Agent
-   MCP
-   Docker
-   Kubernetes

统一到一个面向个人开发者的 AI 工作环境。

核心理念：

    AI
     |
    Remote Context
     |
    Tools
     |
    Execution
     |
    Diagnosis

------------------------------------------------------------------------

# 2. 产品原则

## 轻量优先

目标用户：

-   个人开发者
-   DevOps 工程师
-   AI Agent 用户

优先体验：

    下载
     ↓
    启动
     ↓
    连接服务器
     ↓
    开始工作

不优先引入：

-   企业 RBAC
-   SSO
-   团队管理
-   云端同步
-   复杂安全体系

------------------------------------------------------------------------

# 3. 安全设计策略

个人桌面版不强制企业级安全。

采用：

    SQLite
    +
    Secret Storage
    +
    可选字段加密

而不是默认 SQLCipher。

## 数据分类

普通数据：

-   Host 名称
-   IP
-   端口
-   用户名
-   会话记录

保存：

    SQLite

敏感数据：

-   SSH Password
-   API Key
-   Token
-   私钥内容

保存：

    Windows Credential Manager
    macOS Keychain
    Linux Secret Service

数据库只保存：

    secret_ref

------------------------------------------------------------------------

# 4. Security Mode

提供三个模式：

## Convenience

适合个人服务器：

-   自动登录
-   保存密码

## Balanced（默认）

推荐：

-   SSH Key 使用文件路径
-   密码保存到系统密码库
-   API Key 保存到系统密码库

## Secure

高安全：

-   不保存密码
-   每次输入
-   不缓存敏感数据

------------------------------------------------------------------------

# 5. MVP 功能范围

必须实现：

    SSH
    Terminal
    Host Management
    Local Terminal
    SFTP
    AI Assistant
    Tool Calling

暂不实现：

    企业权限
    云同步
    完整 IDE
    完整 K8s Dashboard

------------------------------------------------------------------------

# 6. 技术架构

    React 19 + TypeScript
            |
           Wails v3
            |
          Go Core

            |
     --------------------
     |        |          |
    SSH    Agent      Storage (SQLite)

            |
     Local / Remote Machine

技术：

-   Go
-   Wails v3
-   React 19 + TypeScript
-   Tailwind CSS + shadcn/ui
-   xterm.js
-   SQLite

> 注：本文件为早期需求草稿，技术栈细节以 `AGENT.md` 与 `ARCHITECTURE.md` 为准。

------------------------------------------------------------------------

# 7. 核心模块

## Host Manager

负责：

-   添加服务器
-   编辑服务器
-   删除服务器
-   测试连接
-   管理连接状态

------------------------------------------------------------------------

## SSH Runtime

负责：

-   SSH Connection
-   Authentication
-   Keepalive
-   Reconnect
-   Command Execute
-   PTY
-   SFTP

------------------------------------------------------------------------

## Terminal

架构：

    xterm.js

    ↓

    Wails Event

    ↓

    PTY

    ↓

    SSH Shell

支持：

-   stdin
-   stdout
-   resize
-   Ctrl+C
-   长连接

------------------------------------------------------------------------

## SFTP

支持：

-   文件浏览
-   上传
-   下载
-   删除
-   重命名

------------------------------------------------------------------------

# 8. AI Agent 架构

不要：

    LLM
     |
    SSH

采用：

    LLM

    ↓

    Tool Runtime

    ↓

    Permission

    ↓

    Executor

    ↓

    Target

------------------------------------------------------------------------

# 9. Tool 系统

统一 Tool：

-   local_exec
-   local_read_file
-   ssh_exec
-   ssh_read_file
-   ssh_write_file
-   upload
-   download

未来：

-   docker
-   kubectl

------------------------------------------------------------------------

# 10. Agent 安全

重点不是数据库加密，而是：

## Tool Permission

分类：

READ

    ls
    ps
    df
    journalctl

WRITE

    修改文件
    上传文件
    restart service

DANGEROUS

    rm
    shutdown
    iptables
    docker prune

危险操作：

必须：

    Agent
     ↓
    Permission Check
     ↓
    User Approval
     ↓
    Execute

------------------------------------------------------------------------

# 11. MCP Server

目标：

让外部 AI Agent 使用本地能力。

支持：

-   Claude
-   Codex
-   Cursor

MCP Tools：

-   list_hosts
-   connect_host
-   exec_command
-   read_file
-   write_file
-   upload
-   download
-   system_info

------------------------------------------------------------------------

# 12. 开发阶段规划

## Phase 1 基础框架

TODO：

-   Wails v3
-   React 19
-   SQLite
-   基础 UI
-   配置管理

## Phase 2 SSH Workspace

TODO：

-   Host CRUD
-   SSH Client
-   Connection Manager
-   xterm.js
-   PTY

目标：

    添加服务器

    ↓

    打开 Terminal

    ↓

    执行命令

## Phase 3 文件管理

TODO：

-   SFTP
-   文件浏览
-   上传下载

## Phase 4 AI Agent

TODO：

-   LLM Provider
-   Agent Runtime
-   Tool Registry
-   ssh_exec
-   local_exec

## Phase 5 安全增强

TODO：

-   SecretStore
-   Keychain
-   Credential Manager
-   Security Mode
-   Approval UI

## Phase 6 MCP

TODO：

-   MCP Server
-   Tool Exposure
-   Permission Mapping

## Phase 7 Docker/Kubernetes

TODO：

-   Docker Tools
-   Kubernetes Tools
-   Diagnosis Agent

------------------------------------------------------------------------

# 13. MVP 发布标准

满足：

-   单 Binary
-   启动快速
-   SSH 稳定
-   Terminal 稳定
-   多 Host 管理
-   AI 基础诊断
-   MCP 调用

------------------------------------------------------------------------

# 14. 长期方向

最终产品：

不是：

    SSH Client

而是：

    AI Remote Operator

用户：

    帮我检查 production 服务器 CPU 为什么高

Agent：

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

------------------------------------------------------------------------

# 15. 核心护城河

不是：

-   SSH
-   Terminal
-   SFTP

而是：

-   Remote Context
-   Tool Ecosystem
-   Agent Runtime
-   Permission Model
-   Diagnosis Knowledge
-   MCP Integration

最终定位：

> 一个轻量、本地优先、AI 增强的个人开发者 Remote Workspace。

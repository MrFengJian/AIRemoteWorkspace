# Agent.md

# AI Remote Workspace - Coding Agent Guide

## 1. Project Overview

实现一个个人桌面版 AI Remote Workspace。

目标：

打造类似：

- Cursor
- Raycast
- Claude Desktop
- VS Code Remote

的 AI Native Desktop Agent。

主要面向：

- SSH 运维
- 远程开发
- 服务器诊断
- AI Agent 操作远程环境
- MCP Tool 调用

核心理念：

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

本项目不是传统 SSH Client。

目标是：

> 让 AI 理解用户本地和远程机器环境，并通过安全 Tool 系统完成任务。

---

# 2. Product Principles

## 2.1 Local First

应用优先运行在用户本地。

原则：

- 数据本地保存
- 不依赖云端账号
- 不强制用户上传服务器信息

---

## 2.2 Lightweight First

目标：

个人开发者工具。

避免过早引入：

- 企业 RBAC
- SSO
- 多租户
- 云端管理
- 复杂权限中心

优先：

```
下载

↓

启动

↓

连接服务器

↓

开始工作
```

---

## 2.3 AI Native

AI 不是聊天窗口。

AI 应该：

- 理解 Host
- 理解 Terminal
- 理解文件
- 调用 Tool
- 分析环境
- 执行任务

---

# 3. Technology Stack

## Desktop Framework

```
Wails v3
```

原因：

- Go 原生
- 单 Binary
- 跨平台
- 适合系统工具

---

## Backend

```
Go 1.24+
```

负责：

- SSH
- PTY
- SFTP
- Agent Runtime
- MCP
- Storage
- System Integration

---

## Frontend

采用：

```
React 19

TypeScript

Vite

Tailwind CSS

shadcn/ui

Radix UI

Zustand

TanStack Query

Lucide Icons

xterm.js
```

原因：

更适合：

- AI 产品
- Desktop App
- 高度定制 UI
- Design System

---

# 4. Frontend Design Philosophy

## Product Style

目标视觉：

类似：

- Cursor
- Raycast
- Linear
- Claude Desktop

特点：

- Dark First
- 高信息密度
- 键盘驱动
- Command Palette
- Terminal First

---

# 5. Design System

不要依赖单一组件库主题。

建立：

```
Design System

        |

Experience Pack

        |

Theme
```

未来支持：

```
Cyberpunk

Industrial

Business

Minimal

Pixel
```

---

## MVP Theme

第一版本：

只实现：

```
Dark Developer Theme
```

类似：

- VS Code Dark
- Cursor Dark

---

## Theme Architecture

推荐：

```
themes/

    dark/

    cyber/

    business/

    pixel/

components/

    Button

    Card

    Dialog

    Panel
```

使用 Design Token：

例如：

```css
--background

--foreground

--primary

--border

--radius

--shadow
```

---

# 6. Frontend Architecture

采用 Feature Based Architecture。

不要：

```
components/

pages/

utils/
```

推荐：

```
src/


app/

    providers

    router


features/


    hosts/

        components/

        store.ts

        api.ts



    terminal/


        TerminalView.tsx

        terminal.store.ts



    agent/


        AgentPanel.tsx

        ToolCall.tsx



    sftp/


    settings/



components/


    ui/

    layout/


stores/


lib/


themes/
```

---

# 7. Backend Architecture

采用分层设计。

目录：

```
internal/


domain/


application/


infrastructure/


interfaces/
```

职责：

## domain

业务模型。

例如：

- Host
- Session
- Tool
- Agent

---

## application

业务流程：

例如：

- Connect Host
- Execute Tool
- Run Agent

---

## infrastructure

外部实现：

例如：

- SSH
- SQLite
- Secret Store
- LLM Provider

---

# 8. Security Design

## 8.1 不默认使用 SQLCipher

原因：

个人桌面应用：

- 增加 CGO
- 增加部署复杂度
- 收益有限

采用：

```
SQLite

+

Secret Storage

+

Field Encryption
```

---

# 9. Data Security Model

## 普通数据

SQLite 保存：

```
Host

IP

Port

Username

Session

History

Settings
```

---

## 敏感数据

禁止直接保存：

```
SSH Password

Private Key

API Key

Token
```

使用：

```
SecretStore
```

抽象：

```go
type SecretStore interface {

    Set(
        key string,
        value []byte,
    ) error


    Get(
        key string,
    ) ([]byte,error)


    Delete(
        key string,
    ) error

}
```

实现：

```
Windows Credential Manager

macOS Keychain

Linux Secret Service
```

SQLite 保存：

```
secret_ref
```

---

# 10. Security Mode

支持：

## Convenience

适合个人机器。

允许：

- 保存密码
- 自动连接

---

## Balanced (Default)

推荐：

- SSH Key 保存路径
- Password 使用系统密码库
- API Key 使用 SecretStore

---

## Secure

高安全：

- 不保存密码
- 每次输入
- 不缓存敏感信息

---

# 11. Core Modules

# 11.1 Host Manager

功能：

- 添加 Host
- 编辑 Host
- 删除 Host
- 测试连接
- 保存配置

模型：

```go
type Host struct {

    ID string

    Name string

    Host string

    Port int

    Username string

    AuthType string

    SecretRef string

}
```

---

# 11.2 SSH Runtime

负责：

- SSH Connection
- Authentication
- Keepalive
- Reconnect
- Command Execute
- PTY
- SFTP

接口：

```go
type SSHConnection interface {

    Exec(
        command string,
    ) error


    Shell()


    SFTP()


    Close()

}
```

---

# 11.3 Terminal

架构：

```
React xterm.js

        |

Wails Event

        |

Go PTY

        |

SSH Shell
```

支持：

- stdin
- stdout
- resize
- Ctrl+C
- session restore

---

# 11.4 SFTP

支持：

- 文件浏览
- 上传
- 下载
- 删除
- 重命名

---

# 12. AI Agent Architecture

禁止：

```
LLM

↓

SSH
```

必须：

```
LLM

↓

Tool Runtime

↓

Permission Check

↓

Executor

↓

Target
```

---

# 13. Tool System

统一接口：

```go
type Tool interface {

    Name() string


    Description() string


    Execute(
        input any,
    ) Result

}
```

---

## First Tools

Local:

```
local_exec

local_read_file

local_write_file
```

Remote:

```
ssh_exec

ssh_read_file

ssh_write_file
```

File:

```
upload

download
```

---

# 14. Agent Permission System

核心安全点。

Tool 分类：

## READ

允许：

```
ls

ps

df

journalctl

docker ps
```

---

## WRITE

需要确认：

```
modify file

upload file

restart service
```

---

## DANGEROUS

必须人工确认：

```
rm

shutdown

iptables

docker prune
```

会话权限策略（Agent 输入区下拉，按会话生效）：

```
strict     写入与危险操作均需人工确认（默认）

auto_write 写入自动放行；危险操作仍必须人工确认
```

DANGEROUS 永不自动放行——无论策略如何，高危命令一律弹窗；未知策略值
归一化为 strict。分级由命令内容动态判定（classifyCommand），对绕过形态
（bash -c / eval 载荷、管道执行、脚本文件执行、变量间接、命令替换、
fork 炸弹）宁可升级、绝不放行。

技能与上下文引用（Agent 输入框，参考 eino adk/middlewares/skill）：

```
/name   调用技能：SKILL.md（frontmatter name/description + markdown 正文）
        位于 <数据目录>/skills/<name>/SKILL.md；正文 inline 注入本回合
@path   引用文件：@/var/log/app.log → <file> 块注入内容（限额截断）
@终端   引用终端缓冲区：@终端（100-120行），前端发送时展开为 <terminal> 块
```

模型侧另有 `skill` 工具可按名自助加载技能（READ 级）。

---

流程：

```
Agent

↓

Tool Call

↓

Permission Check

↓

User Approval

↓

Execute
```

---

# 15. MCP Support

目标：

让外部 AI 调用本地能力。

支持：

- Claude
- Codex
- Cursor

MCP Tools：

```
list_hosts

connect_host

exec_command

read_file

write_file

upload

download

system_info
```

---

# 16. AI Diagnosis

核心差异化。

目标：

用户：

```
检查 production CPU 为什么高
```

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

定位原因

↓

提出修复方案

↓

等待确认执行
```

---

# 16.5 SSH 隧道

按主机管理，每主机可配置多条规则（管理器按 主机+规则 去重/回收）：

```
本地转发 -L   本机 127.0.0.1:<port> → 目标（从服务器侧解析）
远程转发 -R   服务器侧监听 <bind>:<port> → 目标（从本机侧解析）
动态代理 -D   本机 SOCKS5 :<port>，连接从服务器侧发出
```

监听绑定地址可选：仅本机（127.0.0.1，默认）/ 所有网卡（0.0.0.0；
服务器侧还需 sshd GatewayPorts 允许）。

保存时端口冲突预检：表单对启用规则的本机监听端口做占用检测
（排除本主机自己的隧道），命中时提示确认而不是静默失败；
同一表单内两条本机侧规则端口重复则直接报校验错误。

生命周期：

```
打开终端标签页（有启用的隧道规则）

↓

TunnelManager.Ensure（按规则 reconcile：同配置去重、变更替换、删除回收）

↓

专用 SSH 连接 + 指数退避自动重连（2s → 30s 封顶）

↓

状态事件 tunnel:status（按规则键） → 右侧面板「隧道」标签页
```

- 每主机可配置多条规则，隧道按 主机+规则 全局唯一：分屏、复制会话、
  开新标签页都复用同一条隧道，绝不重复创建。
- 隧道开关独立于会话：面板手动停止会被记住（manualStop），此后开会话/
  分屏/复制均不会悄悄重启该规则；只有面板手动启动、主机删除或该规则
  配置变更（规则键变化）才会解除。
- 本地/动态的监听器随重连存活；远程转发（-R）的监听器随连接消亡，
  每次重连成功后重新申请 tcpip-forward。
- 隧道使用独立连接：关闭终端标签页不影响隧道；隧道启停也不影响主机
  的会话连接。
- 本地端口被占用属于致命错误（不重试）；凭据沿用主机的记住密码。

---

# 16.8 数字员工（运维专家角色系统）

按业界数字员工最佳实践建模（结构化人设 + 能力配置 + 安全边界 + 交互设计 + 生命周期）。详设见 `docs/ROADMAP.md` Phase 9。

专家档案（domain.Expert）：

```
身份档案   Name / Role / Icon / Color / Description
人设指令   SystemPrompt（身份-专长-工作方法-输出规范-边界）
能力配置   AllowedTools 工具白名单 + SkillRefs 绑定 SKILL.md 技能包
模型绑定   ProviderID / Model / Temperature / MaxSteps
安全边界   Policy 默认审批策略（权限契约由运行时固定注入，人设不可覆盖）
交互设计   OpeningMessage 开场白 + SuggestedPrompts 推荐问题
生命周期   内置种子 / CRUD / 启用停用 / 副本 / 内置删除仅 Dismissed
```

关键规则：

```
SystemPrompt 分层组装：专家层 + 环境层 + 工具契约 + 权限契约 + 技能层 + 全局指令
权限契约逐字固定（runtime.go），任何人设不能提权 —— 安全不变量
诊断模式已统一为内置 SRE 诊断专家（AutoSnapshot：选中/切换/新对话后的首回合自动注入体检快照）
新对话保留专家选择；退出人设是显式操作（徽章 X / 选择器）
内置专家语义与内置 SKILL.md 一致：编辑不覆盖、删除仅 Dismissed、重新保存恢复
```

落点：

```
domain/expert.go                     Expert 模型 + 内置 ID 常量
application/experts_builtin.go       6 个内置专家人设（SRE/K8s运维/K8s开发/Docker/Linux/DBA）
application/expert_service.go        种子 + CRUD（GetExpert 兼作 runtime ExpertSource）
infrastructure/sqlite/expert_repo.go experts 表
infrastructure/agent/runtime.go      activeExperts 映射、resolveExpert、expertPrompt、工具过滤
infrastructure/agent/tools           BuildForSession(sessionID, allowed)
interfaces/expert_service.go         Wails ExpertService
frontend features/experts/           api / hooks / 头像组件
frontend settings ExpertsSection     设置 → 数字员工 管理界面
```

---

# 17. Development Roadmap

# Phase 1 - Desktop Foundation

TODO:

```
[ ] Wails v3

[ ] React + Vite

[ ] Tailwind

[ ] shadcn/ui

[ ] Dark Theme

[ ] Basic Layout
```

目标：

桌面应用启动。

---

# Phase 2 - Host Management

TODO:

```
[ ] Host CRUD

[ ] SQLite

[ ] Connection Manager

[ ] SSH Authentication
```

---

# Phase 3 - Terminal

TODO:

```
[ ] xterm.js

[ ] PTY

[ ] SSH Shell

[ ] Terminal Tabs
```

---

# Phase 4 - File Workspace

TODO:

```
[ ] SFTP

[ ] File Explorer

[ ] Upload

[ ] Download
```

---

# Phase 5 - AI Agent

TODO:

```
[ ] LLM Provider

[ ] Agent Runtime

[ ] Tool Registry

[ ] Tool Calling

[ ] Permission UI
```

---

# Phase 6 - MCP

TODO:

```
[x] MCP Server

[x] Tool Exposure

[x] Permission Mapping
```

---

# Phase 7 - Advanced

TODO:

```
[x] Docker Tools

[ ] Kubernetes Tools

[x] Diagnosis Agent

[x] Experience Packs
```

---

# Phase 8 - Xshell 能力对齐

详见 docs/ROADMAP.md Phase 8 / docs/TODO.md。

---

# Phase 9 - Digital Employees（数字员工）

详见 docs/ROADMAP.md Phase 9 / §16.8。

```
[x] Expert Persona Card（domain.Expert）

[x] Expert Roster + Builtin Seeds（6 角色）

[x] Runtime Persona Layer + Tool Allowlist

[x] Diagnosis Mode → Builtin SRE Expert（统一）

[x] Settings 管理界面 + 会话选择器
```

---

# 18. Coding Rules

## Go

要求：

- interface 优先
- 清晰分层
- 不写巨型 Service
- domain 与 infrastructure 分离
- 错误明确

---

## React

要求：

- TypeScript strict
- Feature Based
- Zustand 管理业务状态
- shadcn 组件二次封装

避免：

- 巨型 Component
- 全局 Context 滥用
- UI 与业务逻辑混合

---

# 19. MVP Success Criteria

必须满足：

```
[ ] 单 Binary

[ ] 快速启动

[ ] 多 Host

[ ] SSH 稳定

[ ] Terminal 稳定

[x] AI Tool Calling

[x] MCP 基础支持
```

---

# 20. Long Term Vision

最终产品：

```
AI Remote Operator
```

核心资产：

```
Remote Context

Tool Ecosystem

Agent Runtime

Permission Model

Diagnosis Knowledge

MCP Integration
```

开发过程中始终遵循：

```
简单优先

本地优先

用户控制安全

AI 增强效率

保持可扩展
```

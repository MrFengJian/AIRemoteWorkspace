# Diagnosis Agent — 实现思路与计划

> Phase 7 延后项的方案设计（已评审保留）。落地节奏：Phase A → B → C（C 视实际使用反馈排期，与 K8s 面板同策略）。

## 核心结论

Diagnosis Agent **不需要一个新引擎**，而是四件现有机制的组合：

```
诊断场景知识包（SKILL.md）  ←── 路线图「故障定位知识库」
        +
确定性体检快照（Monitor）   ←── 不烧 LLM 的分诊上下文
        +
诊断模式（提示词 + 入口）   ←── 现有 ReAct Agent 换模板
        +
沉淀闭环（会话 → 场景）     ←── 路线图「诊断场景沉淀」
```

**明确不做**：向量库 / RAG、规则引擎式推理。本地优先 + 单 Binary 的定位下，markdown 场景包 + ReAct 灵活组合的复杂度收益比最高；经验检索需求在 Phase C 之前用会话历史搜索顶住。MCP Server 已就绪，外部 Agent 也能自主排障——内置诊断 Agent 的增量价值在于**内置经验 + 零配置快照 + 应用内一键闭环**。

## 设计要点

### 1. 知识库 = SKILL.md 场景包

项目已采用 eino 技能约定（`domain.Skill`：YAML frontmatter 的 name/description + markdown 正文），Agent 已有 `skill` 工具按名加载全文。诊断场景天然就是一篇篇排障 playbook：

- **description** 写症状特征（供 LLM / 关键字匹配选场景）；
- **正文** 是决策树式排查步骤：跑哪些只读命令、看哪些指标、什么结果意味着什么、安全注意事项。

内置场景包随二进制 `go:embed` 分发，首次启动落入 `<data>/skills/`（沿袭 `seedExampleSkill` 先例）；用户可直接编辑 markdown——**「沉淀」就是编辑和新增文件**。

### 2. 分诊快照用确定性采集

`MonitorService` 已有 GetOverview / GetProcesses / GetPorts（/proc 与原生工具解析，远程零依赖），DockerService 已有容器状态解析。诊断会话开始时先跑一次体检快照（CPU / 内存 / 磁盘 / Top 进程 / 端口监听 + journalctl / dmesg 近期错误摘要），作为结构化上下文注入第一轮对话——比让 LLM 自己摸索跑十遍 `top` 又快又省 token。

### 3. 诊断模式 = 提示词 + 入口

Runtime 已按会话组装系统提示词并支持自定义指令。新增「诊断模式」：

- 切换诊断系统提示词模板：分诊流程、证据优先、结论输出规范（现象 / 根因 / 证据 / 建议 / 风险）；
- 权限侧零新机制：READ 自动、WRITE/DANGEROUS 走既有审批。

### 4. 沉淀闭环 = 会话转技能

诊断会话结束后提供「保存为场景」：用 LLM 把本次对话（症状、关键证据、根因、处置）提炼成新 SKILL.md 草稿，预览后写入技能目录，下次同类症状即被 `skill` 工具命中。会话历史（convRepo）已持久化，素材现成。

## 分期计划

### Phase A — 场景包 + 诊断模式（核心）✅

- [x] 内置高价值场景 SKILL.md ×9：CPU 高（cpu-high）、磁盘满（disk-full）、内存 / OOM（memory-oom）、服务异常（service-down）、端口不通（port-unreachable）、容器反复重启（container-restart-loop）、网络延迟（network-latency）、磁盘 IO 高（disk-io-high）、SSH 登录慢（login-slow）
- [x] `skill_service` 支持内置种子落盘（`go:embed all:skills`；已有技能不覆盖，删除的内置包记入 dismissed 列表不复活）
- [x] Runtime 诊断提示词模板 + 快照注入（`StartDiagnosis`）；`monitor_service` 补 `Snapshot(ctx, sessionID)` 聚合输出（概览 + Top 进程 + 端口 + journalctl / dmesg 精简摘要）
- [x] 前端：Agent 面板诊断入口（输入区听诊器按钮），症状输入 → 自动带快照发起会话；场景包以 chip 形式一键填充 `/场景名`
- 触点：`skill_service.go` · `agent/runtime.go` · `monitor_snapshot.go` · `AgentView.tsx` · locales

### Phase B — 沉淀闭环 ✅

- [x] 「保存为场景」：会话右键 → LLM 一次性提炼（`DistillScenario`）→ SKILL.md 草稿预览（名称 / 内容可改）→ 写入技能目录
- [x] 场景管理轻 UI（列表 / 新建 / 编辑 / 删除，复用技能目录读写；内置包带徽标，删除可重建恢复）
- 触点：`skill_service.go`（SaveSkill / DeleteSkill）· `agent_service.go`（GetSkill / SaveSkill / DeleteSkill / DraftScenario）· `Scenarios.tsx` · Agent 面板菜单

> 实现落点与开放决策的取舍：命令全英文、叙述中文（沿袭 daily-check 先例）；入口放 Agent 面板输入区（贴近助手、改动最小）；快照默认精简（journalctl 仅 err 级 25 行 + dmesg 过滤 15 行 + failed 单元 10 行），深挖交给场景包。

### Phase C — 远期可选

- [ ] 历史诊断会话检索（按主机 / 症状关键字）、跨会话「同类故障」提示
- [ ] 结构化结论面板（结论卡片落库）——设计已定（见下），等 Phase B 使用反馈排期

#### 结构化结论面板设计（已评审，待排期）

展示形式分两层：

**1. 会话内联结论卡片（基础形态，先做）**

诊断回复结尾在 markdown 之上渲染一张结构化卡片：

```
┌─ 🩺 诊断结论 ────────────────────────── 2026-09-07 14:32 ─┐
│ 严重度  ● 高                                     主机 web-1 │
│ 现象  nginx 反复重启，外部访问 502                          │
│ 根因  磁盘写满导致日志写入失败，worker 退出                  │
│ ▸ 证据（3 条）   df -h → / 100%；journalctl → No space…     │
│ 建议  [WRITE] journalctl --vacuum-size=200M   ☐ 未执行      │
│       [WRITE] systemctl restart nginx         ☐ 未执行      │
│ 风险  清理日志前确认无审计留存要求                           │
└────────────────────────────────────────────────────────────┘
```

- 建议行带 READ / WRITE / DANGEROUS 标签，配色复用审批框的分级语言；每条建议带
  未执行 / 已执行 / 已忽略 状态且状态可更新——卡片是活的，不是聊天记录里的死文本；
- 卡片右上角放「保存为场景」快捷入口（复用 Phase B 的提炼链路）；
- 数据来源：诊断提示词在五段式结论之外约定输出一段 ```json 块，后端解析后落库，
  不新增工具调用；解析失败降级为纯 markdown，不出卡片；
- 存储：一张 conclusion 表（host / 时间 / 严重度 / 现象 / 根因 / 证据 / 建议+状态 / 风险 /
  关联会话 ID）。

**2. 历史结论面板（跨会话，看使用反馈再定挂载点）**

- 首选：主机监控面板加「诊断」标签页（概览 / 进程 / 端口旁），该主机结论卡片按时间
  倒序成时间线 + 严重度筛选 chip——与故障现场同屏，排查时能翻到上次结论；
- 备选：会话历史面板给含结论的会话加标记；
- 远期：聚合视图（侧栏主机列表标「近 7 天 N 条高危结论」），依赖前两层先用起来。

落地顺序：先内联卡片（提示词 JSON 约定 → 解析入库 → `ConclusionCard` 组件，改动集中、
无新面板）；历史面板等内联卡片证明结论数据真的会被回看（Phase B 沉淀的使用反馈可直接
回答这一点）再决定挂载点。

## 开放决策点

1. **内置场景包正文语言**：建议命令全英文、叙述中英双语或随界面语言选包；
2. **入口形态**：终端面板按钮 vs 独立「诊断」视图——倾向前者，改动小且贴近故障现场；
3. **快照采集边界**：是否默认包含 journalctl / dmesg——建议默认精简（控输出体积），场景包内按需深挖。

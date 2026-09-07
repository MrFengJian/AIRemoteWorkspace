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

### Phase A — 场景包 + 诊断模式（核心）

- [ ] 内置 6–8 个高价值场景 SKILL.md：CPU 高、磁盘满、内存 / OOM、服务异常（systemd）、端口不通、容器反复重启
- [ ] `skill_service` 支持内置种子落盘（已有技能不覆盖）
- [ ] Runtime 增加诊断提示词模板 + 快照注入；`monitor_service` 补 `Snapshot(ctx, sessionID) string` 聚合输出
- [ ] 前端：Agent 面板诊断入口（按钮 / 快捷命令），症状输入 → 自动带快照发起会话
- 触点：`skill_service.go` · `agent/runtime.go` · `monitor_service.go` · `AgentView.tsx` · locales

### Phase B — 沉淀闭环

- [ ] 「保存为场景」：会话 → LLM 提炼 → SKILL.md 草稿预览 → 写入技能目录
- [ ] 场景管理轻 UI（列表 / 编辑 / 删除，复用技能目录读写）
- 触点：`skill_service.go`（加写入路径）· Agent 面板菜单

### Phase C — 远期可选

- [ ] 历史诊断会话检索（按主机 / 症状关键字）、跨会话「同类故障」提示
- [ ] 结构化结论面板（结论卡片落库）——等 Phase B 使用反馈再定

## 开放决策点

1. **内置场景包正文语言**：建议命令全英文、叙述中英双语或随界面语言选包；
2. **入口形态**：终端面板按钮 vs 独立「诊断」视图——倾向前者，改动小且贴近故障现场；
3. **快照采集边界**：是否默认包含 journalctl / dmesg——建议默认精简（控输出体积），场景包内按需深挖。

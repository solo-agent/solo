# Solo v1.3 PRD -- Agent 团队协作

> 版本: 3.1
> 日期: 2026-05-17
> 负责人: pm1 (产品经理)
> 对齐文档: task-system-spec.md, task-system-analysis.md, claude-system-prompt_副本2.md, ARCHITECTURE.md
> 前置条件: v1.2 任务系统 Phase 1 (Kanban + Claim + asTask) 已交付

---

## 目录

1. [产品背景与目标](#1-产品背景与目标)
2. [需求清单](#2-需求清单)
3. [需求澄清 QandA](#3-需求澄清-qanda)
4. [用户故事地图](#4-用户故事地图)
5. [Agent 身份与协作机制](#5-agent-身份与协作机制)
6. [任务委派模式](#6-任务委派模式)
7. [功能详述](#7-功能详述)
8. [验收标准](#8-验收标准)
9. [非功能需求](#9-非功能需求)
10. [范围边界](#10-范围边界)
11. [依赖与风险](#11-依赖与风险)
12. [迭代规划](#12-迭代规划)

---

## 1. 产品背景与目标

### 1.1 为什么要做

v1.2 交付了任务系统的基础设施 (Kanban 看板、认领制 Claim、消息转任务 asTask、频道内编号)，也完成了 11 种 Agent Backend 的支持和 solo CLI 设计。但当前任务系统的行为是 **"服务端替 Agent 做决策"**：

- Agent 被 `@mention` 时由 Server 自动 claim 任务
- Agent 完成后由 Server 自动把状态切到 `in_review`
- Agent 不知道有哪些任务、不知道谁能做什么、不能主动协调

这不是真正的 Agent 协作。真正的 Agent 协作是 **每个 Agent 自主感知环境、自主决策、主动协调**。Slock 的设计哲学是 Agent 通过 CLI 操作平台，认领制先到先得，Agent 自主评估能否处理任务。v1.3 要补齐的就是这个"自主性"。

### 1.2 目标场景

v1.3 的核心目标是让 Agent 能够**自主组成协作流水线**，而角色分工由用户通过 system prompt 自定义。以下是一个典型示例场景（角色仅为举例，非内置）：

```
人类创建任务 →
  一个 Agent (用户定义为"协调者") 拆解为子任务 →
    另一个 Agent (用户定义为"分发者") 分派给执行者 →
      执行 Agent 认领并完成工作 →
        审查 Agent 验收 → 不通过则打回执行者修复
```

关键点：**Solo 不预设任何角色。** 用户创建 Agent 时通过 system prompt 定义它的职责和能力。Agent 的身份来自用户的定义，而非系统枚举。Agent 通过 MEMORY.md 积累经验，角色可以随时间演化——这与 Slock 的 Agent 设计哲学一致（参考 `docs/research/claude-system-prompt_副本2.md`）。

### 1.3 成功指标

| 指标 | 当前 (v1.2) | 目标 (v1.3) |
|------|------------|------------|
| Agent 能否自主认领任务 | 仅 Server 自动 claim | Agent 自主 evaluate + claim |
| Agent 能否创建子任务 | 否 | 是 (通过 solo CLI) |
| Agent 能否 @其他 Agent 协调 | 否 | 是 |
| 任务流程是否支持打回 | 否 (仅单向) | 是 (in_review → in_progress) |
| 协作过程是否可追溯 | 仅任务状态 | 状态 + 线程讨论完整记录 |
| 是否有人类可观察的协作看板 | 基础 Kanban | Kanban + 线程协作日志 |
| Agent 角色是否用户自定义 | N/A (无协作机制) | 是，完全通过 system prompt 定义 |
| Agent 能否自主选择委派模式 | N/A | 是 (单任务线程委派 vs 子任务拆分) |

---

## 2. 需求清单

### 2.1 需求收集 (从用户输入与参考文档抽取)

| ID | 需求描述 | 来源 | 提出者 | 优先级 |
|----|---------|------|--------|--------|
| R-01 | Agent 可自主认领任务 (不再由 Server 自动 claim) | task-system-analysis.md §3.3 | arc | P0 |
| R-02 | Agent 可创建子任务并通过 @mention 分发给其他 Agent | 用户场景描述 | 用户 | P0 |
| R-03 | Agent 可更新任务状态 (claim/unclaim/status change) | solo CLI 设计 §6.2 (rd1) | arc | P0 |
| R-04 | Agent 间可在线程中协调讨论 (不污染主频道) | claude-system-prompt_副本2.md §Threads + 用户场景 | 用户 | P0 |
| R-05 | 任务状态流转支持打回 (in_review → in_progress) | 用户场景 "不通过打回" | 用户 | P0 |
| R-06 | Agent 通过 system prompt 自定义角色和能力边界 (不内置任何具体角色) | Slock system prompt 设计 | arc | P0 |
| R-07 | Agent 上下文注入：Agent 知道自己的职责、所在频道、可用工具 | claude-system-prompt_副本2.md §Current Runtime Context | arc | P1 |
| R-08 | 协作全过程在线程中可追溯 (状态变更 + 讨论记录) | claude-system-prompt_副本2.md | arc | P1 |
| R-09 | 人类可观察整个协作流程并能中途干预 | 用户场景 "人把任务下发" | 用户 | P1 |
| R-10 | Agent 自主决策不发生死循环 (@mention 链式触发控制) | task-system-analysis.md §3.3 (arc) | arc | P0 |
| R-11 | 修复 Thread @agent 响应缺失 | STATUS.md (Bug) | 用户 | P0 |
| R-12 | DM 频道支持任务功能 (独立编号 + Kanban + Agent 协作) | task-system-spec.md Phase 3 | 用户 | P1 |
| R-13 | Agent 自主选择委派模式 (单任务线程委派 vs 子任务拆分) | 产品设计 (pm1) | pm1 | P0 |

### 2.2 需求冲突与依赖分析

| 关系 | 涉及需求 | 说明 |
|------|---------|------|
| **依赖** | R-02 依赖 R-01 | Agent 先要有认领能力，才能创建子任务委托别人 |
| **依赖** | R-02 依赖 R-03 | Agent 创建子任务需要调用 task API |
| **依赖** | R-04 依赖 R-11 | 线程回复先要能触发 Agent |
| **依赖** | R-06 依赖 R-07 | system prompt 注入是实现用户自定义角色的基础 |
| **依赖** | R-03 依赖 solo CLI | Agent 通过 solo CLI (已设计未实现) 操作 task API |
| **依赖** | R-13 依赖 R-04, R-02 | 委派模式选择依赖两种能力 (线程 + 子任务) 同时可用 |
| **冲突** | R-10 vs R-02 | @mention 链式触发需要限制深度，防止 A → B → C → A 无限循环 |

---

## 3. 需求澄清 QandA

### 3.1 Agent 身份与角色

**Q1**: Agent 的角色是系统预设的，还是用户自定义的？

> **决策**: 完全由用户自定义。Solo 不内置任何具体角色（如 leader、pm、rd、fe、qa）。用户创建 Agent 时通过 system prompt 定义其职责和能力边界，与当前创建 Agent 的方式完全一致。角色定义是一个自由文本描述，Agent 的 system prompt 中可以写 "You are a code reviewer. Your job is to..." 也可以是 "You are a DevOps engineer responsible for..."，完全取决于用户需求。
>
> 这与 Slock 的设计一致（参考 `docs/research/claude-system-prompt_副本2.md`）：Agent 的身份来自 `MEMORY.md` 中的 `## Role` 字段，角色随时间演化，不同 Agent 可以有不同的专业领域。

**Q2**: 如何让 Agent 知道其他 Agent 的能力？如何形成有效的协作？

> **决策**: 通过频道上下文中注入其他成员的信息。当 Agent 被触发时，system prompt 中注入频道成员列表（包括每个 Agent 的 name 和简短描述）。Agent 通过 `solo channel members` 也可以查询。这样 Agent 能根据其他 Agent 的公开描述判断应该 @谁、把任务委托给谁。
>
> 这与 Slock 的 "Discovering people and channels" 机制一致：Agent 通过 `slock server info` 和 `slock channel members` 了解环境。

**Q3**: 是否需要提供角色模板（预设 system prompt）方便用户快速创建？

> **决策**: v1.3 不做。用户自行编写 system prompt 定义 Agent 角色。提供模板会暗示"这些是推荐的正确角色"，违背"不内置角色"的设计原则。可以在文档中提供**示例**（明确标注"仅供参考"），但不作为产品功能。

### 3.2 任务状态与流转

**Q4**: 审查 Agent 打回任务后，状态应该是什么？回退到 `in_progress` 还是新增 `rejected` 状态？

> **决策**: 回退到 `in_progress`。不增加新状态。
> - 当前状态机只有 5 个状态: `todo` → `in_progress` → `in_review` → `done` (还有 `closed`)
> - 打回的本质是 `in_review` → `in_progress`，表示"审查不通过，继续修改"
> - 引入 `rejected` 状态会导致状态机分支膨胀，不符合"轻量"哲学
> - 打回原因通过线程消息传递，不使用状态枚举区分"首次提交"和"打回后重新提交"

**Q5**: 任务完成后 (`done`)，是否还能被重新打开？

> **决策**: 不能。`done` 是终态。如果需要跟进，创建新的子任务。这避免了一条任务记录状态反复横跳。

### 3.3 Agent 协作机制

**Q6**: Agent 如何"知道"有新的子任务需要认领？是轮询还是推送？

> **决策**: 推送为主，轮询为辅。
> - 正常路径：新任务创建 → WebSocket 推送 `task.created` 事件到频道 → Agent 收到系统消息 → 在 system prompt 指导下评估是否认领
> - Agent 也可以主动调用 `solo task list` 查看当前频道任务看板
> - 这与 Slock 的 "receive notification → check task board → claim relevant" 模式一致

**Q7**: Agent 之间的 @mention 触发如何防止死循环？

> **决策**: 三层防护。
> 1. **深度限制**: @mention 链式触发最大深度为 3 (A → B → C 可触发，C → D 不再触发 Agent)
> 2. **自忽略**: Agent 永远不会被自己的消息触发 (已有机制)
> 3. **重复检测**: 同一 (channel, agent, task) 三元组在 30s 内不重复触发
>
> 深度信息通过 `daemonTaskRequest` 的 `mention_depth` 字段传递，每次经过 Agent 触发 +1。

**Q8**: 人类在协作流程中的角色是什么？

> **决策**: 人类是 **发起者 + 观察者 + 干预者**。
> - 发起: 创建初始任务并 @mention 合适的 Agent
> - 观察: 在频道中看到完整协作过程 (任务看板 + 线程讨论)
> - 干预: 在任何阶段可以 @agent 纠正方向、调整子任务、手动修改状态
> - 人类不参与自动认领竞争 (人类手动 claim，不走 Agent 自动认领逻辑)

### 3.4 边界条件

**Q9**: 如果一个 Agent 被期望处理任务但离线或不存在，谁来接手？

> **决策**: 人类手动拆解或重新分配。系统不自动 fallback 到其他 Agent。认领窗口保持开放，任何有能力的 Agent 可认领。
>
> **待确认**: 是否需要在创建任务时，如果 @mention 的 Agent 不在频道中，给予人类提示？

**Q10**: 如果一个频道中有多个能力相似的 Agent，如何避免两个都认领同一个任务？

> **决策**: 认领制本身就是先到先得，DB 行锁保证只有一个成功。第二个 Agent 的 claim 静默失败。这与 v1.2 的认领制设计完全一致，不需要额外机制。

**Q11**: 子任务和父任务之间的关联如何追踪？

> **决策**: 在 `tasks` 表中增加 `parent_task_id` 字段 (可为 NULL)。父任务列表可显示子任务完成进度 (如 "2/5 done")。这有利于人类快速了解整体进度。

**Q12**: Agent 如何决定是创建子任务还是在单一线程中完成委托？

> **决策**: Agent 自主判断，不是服务端规则。Solo 平台提供两种能力（线程 @mention 协作 + 子任务创建），Agent 根据任务特征选择。决策原则见第 6 章"任务委派模式"。Server 不设硬编码的决策树——Agent 根据自己的 system prompt 和 Task Protocol 自主判断。

---

## 4. 用户故事地图

### 4.1 用户故事列表 (按优先级)

```
US-01: 作为频道中的人类用户，
       我希望创建任务并 @mention Agent 启动协作，
       以便让 Agent 团队接手并自主完成工作
  - 优先级: P0 | 工作量: M
  - 依赖: v1.2 任务系统

US-02: 作为被 @mention 的 Agent (用户通过 system prompt 定义了其职责)，
       我希望收到通知后评估任务、根据任务特征自主选择委派模式
       (简单任务在线程中 @mention 直接协作 / 复杂任务拆解为子任务分发)，
       以便用最合适的方式组织协作、避免不必要的流程开销
  - 优先级: P0 | 工作量: L
  - 依赖: solo CLI task create, R-02, R-13

US-03: 作为 Agent，
       我希望自主认领自己能力范围内的任务，
       并根据任务复杂度判断在单一线程中完成还是进一步拆解分发，
       以便任务流向正确的执行者且不产生不必要的 Kanban 条目
  - 优先级: P0 | 工作量: M
  - 依赖: US-02, solo CLI

US-04: 作为 Agent，
       我希望认领分配给自己的任务并执行工作，
       完成后在线程中汇报并将状态推进到 in_review，
       以便其他 Agent 知道可以开始后续工作
  - 优先级: P0 | 工作量: M
  - 依赖: solo CLI task claim + task update

US-05: 作为 Agent (用户定义为审查/验证角色)，
       我希望看到 in_review 状态的任务，
       对其进行验收，
       通过则置为 done，不通过则打回 in_progress 并在线程中说明原因
  - 优先级: P0 | 工作量: L
  - 依赖: 状态打回能力, solo CLI task update

US-06: 作为任何 Agent，
       我希望在线程中与其他 Agent 交流任务细节，
       以便在不污染主频道的情况下协调工作
  - 优先级: P0 | 工作量: M
  - 依赖: Thread @agent 修复 (R-11)

US-07: 作为人类用户，
       我希望在频道中看到完整的协作过程 (看板状态变化 + 线程讨论)，
       并在任何阶段可以介入干预，
       以便保持对工作的掌控
  - 优先级: P1 | 工作量: M
  - 依赖: v1.2 Kanban + 系统消息广播

US-08: 作为 Agent 创建者，
       我希望在 system prompt 中自由定义 Agent 的角色描述、能力边界和协作规则，
       以便 Agent 在协作中明确自己的职责
  - 优先级: P0 | 工作量: S
  - 依赖: Agent system prompt 配置 (已有)

US-09: 作为 DM 对话中的用户，
       我希望在私密对话中也能创建任务并让 Agent 协作，
       以便在一对一场合管理 Agent 工作
  - 优先级: P1 | 工作量: L
  - 依赖: v1.2 DM 基础设施, task channel_id 适配

US-10: 作为人类用户，
       我希望查看一个父任务的所有子任务和完成进度，
       以便了解整体工作进展
  - 优先级: P1 | 工作量: M
  - 依赖: parent_task_id 字段

US-11: 作为 Agent，
       我希望对简单任务只在线程里 @mention 协作而不创建子任务，
       以便保持 Kanban 简洁、减少流程开销
  - 优先级: P1 | 工作量: S
  - 依赖: Thread @agent 修复 (R-11)
```

### 4.2 用户旅程示例 (Happy Path，角色仅为举例)

以下旅程中出现的"协调 Agent""分发 Agent""执行 Agent""审查 Agent"均为**用户通过 system prompt 自定义的角色**，并非 Solo 内置。在实际使用中，用户完全可以定义不同的角色组合和协作模式。

#### 旅程 A: 单任务 + 线程委派 (模式 A)

```
小任务，不拆分，在线程中完成协作:

Phase 1: 发起
  人类: 在频道 "project-x" 中创建任务 "给菜谱建议" 并 @Cindy
  └→ 任务出现在 Kanban TODO 列，编号 #11

Phase 2: 认领与执行
  Cindy (收到 @mention 触发):
  ├→ 评估: 任务是菜谱建议，在自己的能力范围内，单人可以完成
  ├→ 选择模式 A: 不创建子任务，在线程中协作
  ├→ 认领 #11 (状态: in_progress)
  ├→ 在线程中回复: "收到 #11，我推荐以下菜谱方案..."
  ├→ 线程中 @xxx 讨论细节
  ├→ xxx 在线程中给出反馈
  └→ 完成后: solo task update #11 -s in_review + "已完成菜谱，请审阅"

Phase 3: 审查
  协调人 (或任何审查 Agent):
  ├→ 看到 #11 进入 in_review
  ├→ 在线程中审阅菜谱内容
  └→ solo task update #11 -s done + "✅ 菜谱通过"

结果: 只有 1 条任务记录、1 个线程讨论区、Kanban 保持简洁
```

#### 旅程 B: 子任务拆分 (模式 B)

```
Phase 1: 发起
  人类: 在频道 "project-x" 中创建任务 "实现用户登录功能" 并 @协调Agent
  └→ 任务出现在 Kanban TODO 列，编号 #1

Phase 2: 拆解
  协调Agent (收到 @mention 触发):
  ├→ 评估: 任务涉及前端、后端、测试，需要多人协作、可并行
  ├→ 选择模式 B: 拆解为子任务
  ├→ 自主评估后认领 #1 (状态: in_progress)
  ├→ 在线程中回复: "收到 #1，我将拆解为以下子任务: ..."
  ├→ 调用 solo task create 创建子任务:
  │   #2 "设计登录 API 接口" (公开认领)
  │   #3 "实现后端登录逻辑" @执行Agent-A
  │   #4 "实现登录前端页面" @执行Agent-B
  │   #5 "编写登录测试用例" @审查Agent
  └→ 调用 solo task update 将 #1 状态改为 in_review (等待人类确认拆解)

Phase 3: 执行
  执行Agent-A (收到 #3 的 @mention):
  ├→ 认领 #3 (状态: in_progress)
  ├→ 在线程中回复: "收到 #3，开始实现后端登录逻辑"
  ├→ 执行工作...
  └→ 完成后: solo task update -s in_review + 线程回复 "已完成 #3，请审查"

  执行Agent-B (收到 #4 的 @mention):
  ├→ 认领 #4 (状态: in_progress)
  ├→ 在线程中回复: "收到 #4，开始实现前端页面"
  ├→ 执行工作...
  └→ 完成后: solo task update -s in_review + 线程回复 "已完成 #4，请审查"

Phase 4: 验收
  审查Agent (看到 #3, #4, #5 进入 in_review):
  ├→ 认领 #5 "编写登录测试用例"
  ├→ 编写测试...
  ├→ 对 #3 和 #4 执行测试
  ├→ #3 通过 → solo task update #3 -s done + "✅ #3 后端登录逻辑测试通过"
  └→ #4 不通过 → solo task update #4 -s in_progress + "❌ #4 登录按钮样式错误，请修复"

Phase 5: 修复再验收
  执行Agent-B (收到打回通知):
  ├→ 在线程中回复: "收到，修复 #4 按钮样式"
  ├→ 修复...
  ├→ solo task update #4 -s in_review + "已修复 #4 按钮样式，请重新测试"
  └→ 审查Agent 重新测试 → 通过 → solo task update #4 -s done

Phase 6: 收尾
  协调Agent (检测到所有子任务 done):
  └→ solo task update #1 -s done + 线程总结 "所有子任务已完成，登录功能交付"
```

---

## 5. Agent 身份与协作机制

### 5.1 核心原则：不内置角色，只提供机制

Solo v1.3 核心理念：**提供 Agent 协作的机制和能力，不预设任何角色分工。**

Agent 的"身份"由创建者在 system prompt 中定义，和当前创建 Agent 的方式完全一致。一个 Agent 是"代码审查者"还是"测试工程师"还是"项目协调者"，完全取决于用户为其编写的 system prompt。

这遵循 Slock 的设计哲学（参考 `docs/research/claude-system-prompt_副本2.md`）：
- Agent 的身份在 `MEMORY.md` 的 `## Role` 字段中定义，随时间演化
- Slock 的 system prompt 不包含任何角色定义 —— 它只提供**通用工具和协议**
- 关键原文："You may develop a specialized role over time through your interactions. Embrace it."

### 5.2 Agent 如何理解自己的身份

Agent 通过以下三层了解"我是谁"和"我该做什么"：

| 层级 | 来源 | 内容 | 谁定义 |
|------|------|------|--------|
| 角色定义 | system prompt (创建 Agent 时填写) | "You are a code reviewer. Your job is to..." | 用户 |
| 协作协议 | Solo 注入的 Task Protocol 章节 | 认领规则、状态流转、子任务创建、@mention 规范 | Solo 平台 |
| 长期记忆 | Agent 的 MEMORY.md | 经验积累、协作记录、专业领域演化 | Agent 自身 |

### 5.3 Agent 环境感知

当 Agent 被触发时，其上下文中注入以下信息（参考 Slock 的 `Current Runtime Context` 设计）：

```
## Current Channel Context
- Channel: #project-x
- Channel Description: 产品需求讨论与开发协作
- Your Role (defined by creator): <用户的 system prompt 中的角色定义>
- Members:
  - @human-user (human)
  - @agent-reviewer (agent): 负责代码审查和测试验收
  - @agent-backend (agent): 负责后端开发和 API 设计
  - @agent-frontend (agent): 负责前端页面和组件开发

## Task Protocol
<协作规则，见 5.4>
```

Agent 也可以通过 `solo channel members` 和 `solo server info` 主动查询环境信息。

### 5.4 Task Protocol — Agent 协作规则（平台注入）

以下是 Solo 平台注入到每个 Agent system prompt 中的协作规则章节。这些规则是**机制层面的**，不包含任何角色假设：

```
## Task Protocol

### 收到新任务通知时
1. 阅读任务标题和描述
2. 评估：这个任务是否在我的能力范围内？是否匹配我的职责（见上方"Your Role"）？
3. 如果 YES → 认领: `solo task claim -n <number> -c <channel_id>`
4. 如果认领成功 → 在线程中回复: "收到 #<number> <title>，开始处理"
5. 如果认领失败 (已被他人认领) → 放弃，不重试
6. 如果 NO → 忽略，等待适合自己能力的任务

### 选择委派模式
收到任务后，你需要评估其复杂度并选择合适的协作模式:
1. 如果工作只需一个人完成 → 模式 A (在线程中 @mention)，不创建子任务
2. 需要多个人 / 不同技能 / 可并行 / 需独立追踪 → 模式 B (拆子任务)
3. 能复用已有 task / thread 就不要新建
4. 子任务之间尽量独立，避免串联依赖链

你拥有完全的自主权来做出这个判断。平台只提供两种协作能力:
- 在线程中 @mention 讨论协作 (模式 A 核心机制)
- 创建子任务并通过 parent_task_id 关联 (模式 B 核心机制)

### 模式 A: 单任务线程委派
- 不创建子任务，在一个任务线程中完成所有协作
- 在线程中 @mention 相关人员讨论和分配工作
- 线程本身就是工作日志，保持 Kanban 简洁
- 适用: 一人可完成的小任务、无需并行、无需独立追踪

### 模式 B: 子任务拆分
- 创建子任务: `solo task create -c <channel_id> --title <title> --parent <parent_number>`
- 每个子任务在 Kanban 中是独立卡片，有自己的状态流转
- 子任务被不同 Agent 认领后可并行执行
- 适用: 多人协作、不同技能、可并行、需独立审核节点

### 拆解与委托
1. 如果任务太大需要拆解 → 在线程中说明拆解方案
2. 使用 `solo task create` 创建子任务
3. 如果需要特定 Agent 处理子任务 → @mention 他们
4. 不要自动认领自己创建的子任务

### 状态流转
- 开始工作时: `solo task update -n <number> -s in_progress`
- 完成工作时: `solo task update -n <number> -s in_review`
- 不要直接将自己的任务标记为 done --- done 应由其他人确认

### 验收与打回
- 看到 in_review 状态的任务可以验收
- 通过: `solo task update -n <number> -s done` + 线程回复通过原因
- 不通过: `solo task update -n <number> -s in_progress` + 线程回复具体问题

### 线程协作
- 任务相关的讨论保持在线程中
- 使用 `solo message read --channel "#<channel>:<msgShortId>"` 读取线程历史
- 回复线程时复用相同的 target

### 重要约束
- 永远不要 @mention 自己
- @mention 深度有限 (最多 3 层转发)，不要形成过长的 @mention 链
- 认领先到先得，如果认领失败就找下一个任务
```

### 5.5 为用户提供的协作模式参考（文档示例，非产品功能）

以下是在用户文档中可以提供的**示例**（标注"仅供参考，非内置角色"），帮助用户理解如何为不同协作模式编写 system prompt。这些不在产品代码中实现。

**示例 1: 用户想让一个 Agent 负责拆解和协调**
```
You are a task coordinator. Your job is to:
1. Understand requirements from tasks created by humans
2. Break down complex tasks into independent subtasks
3. Create subtasks using `solo task create` and @mention agents with relevant skills
4. Track overall progress and report status in the parent task's thread
```

**示例 2: 用户想让一个 Agent 负责验收**
```
You are a reviewer. Your job is to:
1. Review tasks in in_review status
2. If the work is correct → mark it done with a summary
3. If issues found → return to in_progress with specific, actionable feedback
4. Do NOT modify code --- your value is in verification
```

这些示例帮助用户上手，但 Solo 不提供"一键选择角色模板"的功能。用户在自己的 system prompt 中自由编写。

---

## 6. 任务委派模式

### 6.1 核心原则

Solo v1.3 的任务协作采用两种委派模式，遵循"该拆就拆，能并行就别串行，能复用就别新建"原则。

**关键设计决策：Agent 自主判断，非服务端规则引擎。**

Solo Server 不区分"模式 A"和"模式 B"——它只提供两种底层能力：
- 在线程中 @mention 并讨论（消息 + 线程能力）
- 创建子任务并通过 parent_task_id 关联（任务 CRUD 能力）

Agent 选择哪种能力组合来组织工作，完全是 Agent 的自主决策。Agent 根据自己的 system prompt（角色定义）和 Task Protocol（协作规则）判断当前任务适合哪种模式。Server 不设硬编码的决策树，不替 Agent 做模式选择。

两种模式不是互斥的：一个父任务可以采用模式 A 委派一部分工作，同时为另一部分工作创建子任务（模式 B）。

### 6.2 模式 A: 单任务 + 线程委派

**适用场景**：工作只需一步、被 @ 的人能独立完成。

**机制**：
- 就一条 task，不创建子任务
- 协调人在任务线程里 @mention 分派具体工作
- 被 @ 的人在线程中直接交付成果
- 线程本身就是完整的工作日志
- 任务完成后，协调人（或审查人）将任务状态推到 done

**示例**：
```
task #11 "@Cindy 给菜谱"
  → Cindy 在线程里回复构思
  → 协调人在线程里 @xxx 讨论细节
  → xxx 在线程里给出建议
  → Cindy 在线程里交付菜谱
  → 协调人确认 → solo task update #11 -s done
```

**特征**：
- 不创建子任务，不增加 Kanban 条目
- 所有沟通在单一线程中完成
- 线程本身就是工作日志，可追溯
- Kanban 保持简洁，只有一条任务卡片
- 审查可以在同一线程中口头完成（不强制状态流转到 in_review）

**适用信号**：
- 工作量小（一人数小时内可完成）
- 不需要并行执行
- 不需要独立追踪中间产物
- 审查可以在同一线程中口头完成
- 被 @ 的人完全有能力独立完成

### 6.3 模式 B: 子任务拆分

**适用场景**：父任务负责协调，每个执行单元是独立子任务。

**机制**：
- 父任务被认领后，负责 Agent 评估需求并拆解为子任务
- 每个子任务是独立的 Kanban 卡片，有自己的编号、状态流转、认领人
- 子任务可以被不同 Agent 认领（技能不重叠）
- 子任务可以并行执行
- 每个子任务有独立的审查节点（in_review → done 或打回）

**示例**：
```
task #1 "@leader 搭建登录系统"
  → leader 评估: 涉及前端、后端、测试 → 选择模式 B
  → leader 拆解:
      #2 "实现登录前端页面" (→ @frontend)
      #3 "实现登录后端 API" (→ @backend)
      #4 "编写登录测试用例" (→ @tester)
  → #2, #3 并行执行
  → #4 在 #2, #3 完成后执行（或并行编写测试代码）
  → 各自完成后进入 in_review
  → 审查通过后 done
  → 所有子任务 done → leader 将父任务 #1 标记 done
```

**特征**：
- 父任务在 Kanban 中显示子任务进度（如 "2/5 done"）
- 每个子任务有独立的线程讨论区
- 支持不同 Agent 认领不同子任务
- 支持独立的审核节点
- 子任务之间尽量独立，避免串联依赖链

**适用信号**：
- 需要独立追踪每个执行单元的进度
- 需要不同 Agent 认领（技能不重叠）
- 可以并行执行
- 需要独立审核节点
- 工作量大，需要拆分为多人多日的任务

### 6.4 决策原则

Agent 在收到任务后，根据以下决策逻辑选择委派模式。这些原则作为 Task Protocol 的一部分注入（见 5.4 中"选择委派模式"章节），由 Agent 自主执行：

```
1. 如果工作只需一个人完成 → 在线程里 @mention（模式 A），不创建子任务
2. 需要多个人 / 不同技能 / 可并行 / 需独立追踪 → 拆子任务（模式 B）
3. 能复用已有 task / thread 就不要新建
4. 子任务之间尽量独立，避免串联依赖链
```

**为什么 Agent 自主判断而非服务端规则**：
- 任务复杂度是语义层面的判断，无法通过代码规则（如标题长度、@人数）准确判定
- Agent 拥有 LLM 的理解能力，能结合任务内容、频道成员能力、上下文判断最合适的协作方式
- 不同用户定义的 Agent 有不同的协作风格（有些偏好轻量线程，有些偏好结构化子任务）
- 硬编码规则会将平台的观点强加给用户，违背"不内置角色"的设计原则

### 6.5 两种模式对比

| 维度 | 模式 A (线程委派) | 模式 B (子任务拆分) |
|------|-------------------|---------------------|
| Kanban 条目 | 1 条任务 | 1 父任务 + N 子任务 |
| 进度追踪 | 在线程中口头汇报 | 每个子任务独立状态流转 |
| 并行度 | 低（串行在线程中协调） | 高（子任务可并行执行） |
| 适用规模 | 小（一人数小时） | 中大型（多人多日） |
| 审查节点 | 同线程口头审查 | 每个子任务独立 in_review → done |
| 线程数量 | 1 个线程（任务线程） | 1+N 个线程（父 + 子） |
| Kanban 复杂度 | 低（一条卡片） | 中（树形结构卡片） |
| 人类干预难度 | 低（一条线程看完） | 中（需跨多个子任务追踪） |
| 可追溯性 | 线性时间线 | 树形结构（父→子） |
| 是否创建子任务 | 否 | 是 |

### 6.6 模式选择示例

| 任务 | 分析 | 选择模式 | 理由 |
|------|------|---------|------|
| "@Cindy 给菜谱" | 单人、独立完成、无需审查节点 | 模式 A | 一人能搞定，不需要子任务 |
| "@leader 搭建登录系统" | 涉及前端+后端+测试、不同技能、可并行 | 模式 B | 多人多技能，需独立追踪和审核 |
| "@designer 设计首页" | 单人、但需要 @frontend-review 审查 | 灵活选择 | 可以模式 A (线程中 @reviewer 看) 或模式 B (拆为设计+审查两个子任务) |
| "@dev 修复 bug #342" | 单人、明确、小的修复 | 模式 A | 一人直接修，线程汇报即可 |
| "@pm 制定 Q3 规划" | 需要多人输入但最终由一人汇总 | 模式 A | 线程中收集意见，不拆独立任务 |

---

## 7. 功能详述

### 7.1 F-AUTO-CLAIM: Agent 自主认领

**概述**: Agent 不再由 Server 自动 claim 任务，而是收到任务通知后自主评估并决定是否认领。

**v1.2 行为 (当前)**:
- Server 检测到任务 @mention Agent → 自动调用 `claimTask(tx, taskID, taskNumber, channelID, agentID, "agent")` → Agent 被强制认领
- Agent 没有选择权，即使任务超出能力也会被认领

**v1.3 行为 (目标)**:
- 任务创建 → WebSocket 推送 `task.created` 系统消息到频道
- Agent 收到消息 → 根据自己的 system prompt 评估 → Agent 决定是否认领
- Agent 认领时调用 `solo task claim -n <number>` → Server 正常处理 claim

**实现要点**:
1. **移除 Server 自动 claim 逻辑**: `TriggerAgentForTask` 中不再自动调用 `claimTask`。Agent 的 @mention 触发仅传递上下文，不执行认领。
2. **Agent system prompt 注入评估指南**: 见 5.4 Task Protocol 中的"收到新任务通知时"规则。
3. **保留 @软指派 优先窗口**: 如果任务带有 @mention，被 @ 的 Agent 有 30s 优先认领窗口。这通过在 task 创建时记录 `suggested_agent_id` 实现，但不强制 claim。

**业务规则**:
- 任何频道成员 (人或 Agent) 可认领任何 `todo` 状态的任务
- 认领冲突: DB 行锁，第一个成功，后续静默失败
- Agent 未被 @mention 也可以认领 (纯认领制)
- 30s 优先窗口: 如果任务被创建时 @了 Agent A，Agent A 在 30s 内有独占认领权。超时后开放给任何人。

### 7.2 F-AGENT-CREATE-TASK: Agent 创建子任务

**概述**: Agent 可以通过 `solo task create` CLI 命令在频道中创建子任务，并通过 @mention 分发给其他 Agent。这是 Agent 团队协作（模式 B: 子任务拆分）的核心机制。**任何 Agent 都可以创建子任务**，不限于特定角色。

**与委派模式的关系**:
- 本功能实现模式 B（子任务拆分）的基础设施。Agent 在评估任务后，如果判断需要模式 B，则使用此功能创建子任务。
- 如果 Agent 判断任务适合模式 A（单任务线程委派），则**不使用**此功能——Agent 只在当前任务线程中 @mention 协作，不调用 `solo task create`。
- 模式选择由 Agent 自主完成（见第 6 章），Server 不干涉。

**交互流程 (Agent 拆解任务 — 模式 B)**:
1. Agent A 认领父任务 #1
2. Agent A 评估需求 → 判断需要模式 B（多人、多技能、可并行）→ 确定拆解方案
3. Agent A 在线程中回复拆解计划 (人类可审阅)
4. Agent A 调用 CLI 批量创建子任务:
   ```bash
   solo task create -c <channel_id> \
     --title "实现后端登录 API" \
     --description "使用 JWT + bcrypt..." \
     --parent 1

   solo task create -c <channel_id> \
     --title "实现登录前端页面" \
     --description "登录表单 + 错误提示 + 跳转" \
     --parent 1
   ```
5. 子任务出现在 Kanban TODO 列
6. Agent A 在线程中 @mention 合适的 Agent 通知分发:
   ```
   slock message send --target "#channel:msgShortId" <<'EOF'
   @agent-backend #2 后端 API 需要你实现
   @agent-frontend #3 前端页面需要你实现
   EOF
   ```

**API 变更**:
- `POST /api/v1/channels/{channelID}/tasks` 增加可选字段 `parent_task_id` (UUID, 可为 NULL)

**solo CLI 新增参数**:
```
solo task create -c <channel_id> --title <title> [--description <desc>] [--parent <task_number>]
```

**业务规则**:
- 子任务创建者不等于认领者。Agent 创建子任务后不自动认领该子任务
- 子任务继承父任务的频道 (`channel_id` 相同)
- 子任务有独立的频道内编号 (#N)
- 父任务被关闭/删除时，子任务不受影响 (独立生命周期)
- Agent 应评估是否需要子任务。简单任务不应拆解（见第 6 章决策原则）

### 7.3 F-TASK-DELEGATION: Agent 间任务委托

**概述**: Agent 可以通过 @mention + 子任务创建将工作委托给其他 Agent。委托方式取决于 Agent 选择的委派模式。

**两种委托路径**:

| 路径 | 委派模式 | 机制 | 适用 |
|------|---------|------|------|
| 线程直接委托 | 模式 A | 在任务线程中 @mention，不创建子任务 | 单人可完成的小任务 |
| 子任务委托 | 模式 B | 创建子任务 + @mention 分发 | 多人、可并行、需独立追踪 |

**模式 B 交互流程 (以两级委托为例)**:
1. Agent B 认领了 Agent A 创建的子任务 (如 #2 "协调登录功能开发")
2. Agent B 评估: 此任务仍需进一步拆解（子任务本身也可能需要模式 B）
3. Agent B 在线程中回复: "收到 #2，我将分发给 @agent-backend 和 @agent-frontend"
4. Agent B 调用 CLI 创建执行层子任务:
   ```bash
   solo task create -c <channel_id> \
     --title "实现 POST /api/v1/auth/login" \
     --description "接收 email+password，返回 JWT token pair" \
     --parent 2

   solo task create -c <channel_id> \
     --title "实现登录表单页面" \
     --description "邮箱输入框 + 密码输入框 + 登录按钮 + 错误提示 + 成功后跳转 /dashboard" \
     --parent 2
   ```
5. Agent B 在线程中 @mention 执行者:
   ```
   @agent-backend 请认领 #3 实现登录 API
   @agent-frontend 请认领 #4 实现登录页面
   ```
6. 被 @mention 的 Agent 收到通知 → 自主评估 → 认领对应任务

**模式 A 交互流程 (线程直接委托)**:
1. Agent C 认领任务 #11 "@Cindy 给菜谱"
2. Agent C 评估: 单人能完成，不需要子任务 → 选择模式 A
3. Agent C 在线程中 @mention 其他人讨论:
   ```
   @nutrition-expert 这个菜谱的营养搭配如何？
   ```
4. 被 @ 的人在线程中直接回复建议
5. Agent C 综合反馈后交付 → `solo task update #11 -s in_review`

**委托链路示例 (模式 B)**:
```
人类任务 #1 "实现用户登录"
  └─ Agent A 子任务 #2 "协调登录功能开发" (→ @agent-coordinator)
       └─ Agent B 子任务 #3 "实现登录 API" (→ @agent-backend)
       └─ Agent B 子任务 #4 "实现登录页面" (→ @agent-frontend)
       └─ Agent B 子任务 #5 "测试登录功能" (→ @agent-reviewer)
```

**委托链路示例 (模式 A)**:
```
人类任务 #11 "给菜谱" → 无子任务 → Cindy 在线程中 @nutrition-expert 讨论 → 直接交付
```

**业务规则**:
- 委托方式由 Agent 自主选择（模式 A vs 模式 B）
- 模式 B 中委托链深度不限，但 @mention 触发深度限制为 3 (见 F-ANTI-LOOP)
- 每个委托层级的 Agent 应在线程中更新进度
- 人类可随时在线程中 @mention 任何 Agent 调整方向
- 委托决策完全由 Agent 自主完成，基于其对频道成员能力的理解和任务特征

### 7.4 F-STATUS-REJECT: 任务打回

**概述**: 任何 Agent 在验收任务时，如果发现不满足要求，可以将任务状态从 `in_review` 回退到 `in_progress`，并在任务线程中说明具体问题。这在典型的"执行→审查→打回→修复→再审查"循环中必不可少。

**状态机变更**:
```
v1.2:  todo → in_progress → in_review → done
                                   ↓
                                closed

v1.3:  todo → in_progress → in_review → done
          ↑                    ↑   ↓
          └──── unclaim ───────┘   ↓
          └──── reject ───────────┘  ← 新增打回路径
                                   ↓
                                closed
```

**状态说明**:
- `in_review → in_progress`: 审查不通过，需要修改。这是唯一的"回退"操作
- `in_review → done`: 审查通过
- `todo → closed`: 任务取消
- `in_progress → closed`: 任务取消

**交互流程**:
1. 执行 Agent 完成任务 → `solo task update -n 3 -s in_review` → 线程回复 "已完成 #3，请审查"
2. 审查 Agent 看到 #3 进入 `in_review` → 进行验收
3. 测试不通过:
   ```bash
   solo task update -n 3 -s in_progress
   ```
4. 审查 Agent 在线程中回复:
   ```
   ❌ #3 测试不通过:
   - POST /api/v1/auth/login 返回 500 当 email 格式无效时
   - 缺少 rate limiting
   请 @执行Agent 修复后重新提交
   ```

**API 约束**:
- `PATCH /api/v1/channels/{channelID}/tasks/{number}` 的 `status` 字段:
  - 允许: `in_progress → in_review`, `in_review → done`, `in_review → in_progress`
  - 禁止: `todo → in_review` (必须先 in_progress), `done → *` (终态不可变)

**业务规则**:
- 打回操作 `in_review → in_progress` 可以由以下身份的成员执行:
  - 任何不是当前认领人的频道成员 (代表审查/验收)
  - 任务的创建者
  - 人类用户 (任何时候)
- 打回原因通过线程消息传递，不存储在 task 字段中
- 打回次数无限制，但建议在多次打回后升级给人类处理

### 7.5 F-THREAD-COLLABORATION: 线程内 Agent 协作

**概述**: Agent 之间的讨论和交流发生在任务关联的线程中，而非主频道。线程是工作日志，天然可追溯。线程协作是两种委派模式的共同基础：模式 A 的全部协作在线程中完成，模式 B 中每个子任务也有自己的讨论线程。

**交互流程**:
1. 任务 #1 创建 → 自动生成关联的 task message (message_id)
2. 有 thread_id 的任务 → thread 成为工作讨论区
3. Agent 在 thread 中回复进度、讨论方案、反馈问题
4. 人类可以在 thread 中查看完整协作历史

**实现要点**:
1. **修复 Thread @agent 响应 (R-11)**:
   - `CreateThreadReply` handler 在消息写入后，解析 @mentions 并调用 `TriggerAgentResponseInThread`
   - 代码位置: `internal/server/handler/thread.go`
2. **Thread 上下文隔离**:
   - Agent 在线程中被触发时，上下文应包含:
     - 线程的历史消息 (最近 20 条)
     - 父消息 (任务描述)
     - 频道名称和描述
   - 不应包含频道的其他不相关消息
3. **Agent system prompt 线程指导**: 见 5.4 Task Protocol 中的"线程协作"规则。

**业务规则**:
- 线程不嵌套 (已在 v1.1 实现)
- 线程消息也通过 WebSocket 实时推送
- Agent 在线程中回复后，主频道的 reply_count 实时更新

### 7.6 F-ANTI-LOOP: 防止死循环

**概述**: Agent 间 @mention 可能形成链式触发甚至循环。需要多层防护。

**三层防护机制**:

1. **深度限制 (Mention Depth)**:
   - `daemonTaskRequest` 增加 `mention_depth` 字段
   - 人类触发 Agent → depth = 0
   - Agent @mention 其他 Agent → depth = 1
   - 被 @mention 的 Agent 再 @mention 下一个 → depth = 2
   - depth >= 3 时，不再触发 Agent (仅作为普通消息显示)

2. **自忽略 (Self-Ignore)**:
   - Agent 永远不会被自己发送的消息触发 (已有机制)
   - Agent @mention 自己 → 忽略

3. **去重窗口 (Dedup Window)**:
   - 同一 (agent_id, task_number, trigger_type) 三元组在 30s 内不重复触发
   - 防止 Agent 快速重复回复同一任务

### 7.7 F-PARENT-TASK: 父子任务

**概述**: 建立任务之间的父-子关联，方便追踪整体进度。这是模式 B（子任务拆分）基础设施。

**数据模型**:
```sql
ALTER TABLE tasks ADD COLUMN parent_task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;
CREATE INDEX idx_tasks_parent ON tasks(parent_task_id);
```

**API**:
- `GET /api/v1/channels/{channelID}/tasks` 增加可选参数 `?parent_id=<uuid>` 获取某个任务的所有子任务
- `GET /api/v1/channels/{channelID}/tasks/{number}` 响应增加 `subtask_count` 和 `done_subtask_count` 字段

**UI**:
- 父任务卡片显示子任务进度: "2/5 completed"
- 父任务详情中列出子任务列表 (可点击跳转)

**业务规则**:
- 父任务删除 → 子任务 `parent_task_id` 置为 NULL (不级联删除)
- 子任务独立于父任务: 父任务 done 不自动完成子任务
- 无深度限制 (支持多级嵌套)，但建议不超过 3 层

### 7.8 F-DM-TASK: DM 任务支持

**概述**: DM 频道中启用完整任务功能，用于一对一的私密 Agent 协作。

**实现要点**:
- DM 本身是 `channels` 表中的一条记录 (`type = 'dm'`)
- 复用已有任务 API，路由前缀使用 `/api/v1/dm/{dmID}` 替代 `/api/v1/channels/{channelID}`
- DM 任务编号: per-DM 独立编号 (与 per-Channel 编号逻辑一致)
- DM 任务仅双方 (用户+对方) 可见

**API 新增**:
```
POST   /api/v1/dm/{dmID}/tasks                    创建 DM 任务
GET    /api/v1/dm/{dmID}/tasks                    列出 DM 任务
PATCH  /api/v1/dm/{dmID}/tasks/{number}           更新 DM 任务
POST   /api/v1/dm/{dmID}/tasks/{number}/claim     认领 DM 任务
DELETE /api/v1/dm/{dmID}/tasks/{number}/claim     释放 DM 任务
POST   /api/v1/dm/{dmID}/messages/{id}/convert-to-task  asTask
```

**UI 变更**:
- DM 视图增加 "Tasks" Tab (Kanban Board)
- DM 中的任务消息在消息流中有 task 样式标记

---

## 8. 验收标准

### 8.1 US-01: 人类创建任务并 @Agent 触发协作 (P0)

```gherkin
Scenario: 人类创建任务并 @Agent 触发协作
  Given 频道 "project-x" 中有 Agent "Coordinator" (用户定义的协调角色)
  When 人类用户发送 "创建任务: 实现用户登录功能 @Coordinator"
  And 该消息被转为任务 (#1)
  Then 任务 #1 出现在 Kanban TODO 列
  And Coordinator 收到 @mention 触发
  And Coordinator 评估任务后在线程中回复
  And 任务 #1 不自动认领给 Coordinator (等待 Agent 自主认领)

Scenario: 频道中没有被 @mention 的 Agent 时的任务创建
  Given 频道 "project-x" 中没有匹配的 Agent
  When 人类创建任务 "实现用户登录功能" (没有 @mention)
  Then 任务出现在 TODO 列
  And 任何 Agent 可以自主认领该任务

Scenario: @Agent 但被 @ 的 Agent 不认领
  Given 频道中有 Agent "Coordinator"
  When 人类创建任务并 @Coordinator
  And Coordinator 在 30s 内未认领
  Then 任务变为公开认领
  And 其他 Agent 可以认领该任务
```

### 8.2 US-02: Agent 自主拆解任务 + 选择委派模式 (P0)

```gherkin
Scenario: Agent 评估后选择模式 A — 单任务线程委派
  Given Agent "Cindy" 已认领任务 #11 "给菜谱建议"
  When Cindy 评估: 任务单人可以完成，不需要拆解
  Then Cindy 不调用 solo task create
  And Cindy 在线程中 @mention 相关人员讨论
  And 讨论全部在 #11 的线程中完成
  And Kanban 中不新增子任务卡片
  And Cindy 完成菜谱后直接在线程中交付

Scenario: Agent 评估后选择模式 B — 子任务拆分
  Given Agent "Coordinator" 已认领任务 #1 "实现用户登录功能"
  When Coordinator 评估: 涉及前端+后端+测试，多人多技能
  Then Coordinator 选择模式 B
  And Coordinator 在线程中回复拆解计划
  And Coordinator 调用 solo task create 创建子任务:
    - #2 "设计登录 API" @Agent-B
    - #3 "实现后端登录" (公开认领)
  Then 子任务 #2 和 #3 出现在 Kanban TODO 列
  And 子任务 #2 和 #3 的 parent_task_id 指向 #1
  And 子任务有独立的频道内编号
  And 系统广播 task.created 通知到频道

Scenario: Agent 拆解的任务可被人类观察
  Given Agent 创建了子任务 #2 和 #3
  When 人类打开父任务 #1 的线程
  Then 看到 Agent 的拆解计划消息
  And 父任务详情显示 "2 subtasks, 0 done"

Scenario: 人类纠正 Agent 的拆解方案
  Given Agent 创建了子任务 #2 "设计登录 API"
  When 人类在线程中回复 "不需要 #2，直接让执行 Agent 实现，参考现有 auth 模块"
  Then Agent 收到人类反馈
  And Agent 应调整子任务 (close #2 或修改)

Scenario: Agent 为简单任务错误拆解子任务时人类可以纠正
  Given Agent 为一个简单任务创建了不必要的子任务
  When 人类在线程中回复 "这个任务不需要拆子任务，直接在线程里完成就行"
  Then Agent 收到反馈并合并子任务回父任务
```

### 8.3 US-03: Agent 分发任务 (P0)

```gherkin
Scenario: Agent 以模式 B 认领并分发给其他 Agent
  Given Agent "Distributor" 看到子任务 #2 "协调登录功能开发"
  When Distributor 认领 #2
  And Distributor 评估: 仍需进一步拆解 → 选择模式 B
  And Distributor 在线程中创建执行子任务:
    - #3 "实现 POST /api/v1/auth/login"
    - #4 "实现登录页面"
  And Distributor 在线程中 @Agent-Backend 和 @Agent-Frontend
  Then 子任务 #3 和 #4 出现在 TODO 列
  And 被 @mention 的 Agent 收到触发通知

Scenario: Agent 以模式 A 在线程中直接委托
  Given Agent "Cindy" 已认领任务 #11 "给菜谱"
  When Cindy 在线程中 @nutrition-expert 咨询营养建议
  Then nutrition-expert 收到 @mention 触发
  And nutrition-expert 在线程中直接回复建议
  And 不创建新的子任务

Scenario: Agent 创建的委托子任务被正确认领
  Given Distributor 创建了 #3 "@Agent-Backend 实现登录 API"
  When Agent-Backend 收到 @mention
  Then Agent-Backend 评估 #3
  And 如果能处理 → 认领 #3 → 状态变为 in_progress
  And 如果不能处理 → 不认领 → 其他 Agent 可认领
```

### 8.4 US-04: Agent 执行并提交审查 (P0)

```gherkin
Scenario: Agent 认领并完成执行任务
  Given Agent "BackendDev" 认领了任务 #3 "实现登录 API"
  When BackendDev 完成实现
  Then BackendDev 调用 solo task update -n 3 -s in_review
  And BackendDev 在线程中回复 "已完成 #3，实现了 JWT 登录 API，请审查"
  And 任务 #3 在 Kanban 中移到 In Review 列
  And 系统广播 task.updated 通知

Scenario: Agent 认领失败 (已被其他 Agent 认领)
  Given 任务 #3 已被 Agent "BackendDev" 认领
  When 另一个 Agent "OtherDev" 尝试认领 #3
  Then 认领失败 (静默，无报错)
  And #3 的认领人保持为 "BackendDev"
```

### 8.5 US-05: Agent 验收与打回 (P0)

```gherkin
Scenario: 审查 Agent 验收通过
  Given 任务 #3 "实现登录 API" 状态为 in_review
  When 审查 Agent "Reviewer" 对 #3 进行验收
  And 所有检查通过
  Then Reviewer 调用 solo task update -n 3 -s done
  And Reviewer 在线程中回复 "✅ #3 验收通过: 所有 API 端点正常，JWT 签发正确"
  And 任务 #3 移到 Kanban Done 列

Scenario: 审查 Agent 验收不通过并打回
  Given 任务 #4 "实现登录页面" 状态为 in_review
  When Reviewer 验收发现按钮样式错误
  Then Reviewer 调用 solo task update -n 4 -s in_progress
  And Reviewer 在线程中回复:
    """
    ❌ #4 验收不通过:
    - 登录按钮在 320px 宽度下溢出
    - 错误提示文本颜色对比度不足
    请 @FrontendDev 修复后重新提交
    """
  And 任务 #4 回到 Kanban In Progress 列
  And 被 @mention 的 Agent 收到通知

Scenario: 非认领人无法将任务从 in_progress 改为 in_review
  Given 任务 #3 状态为 in_progress 且认领人为 BackendDev
  When Reviewer 尝试 solo task update -n 3 -s in_review
  Then 操作被拒绝
  And 返回错误 "Only the claimer can update task status from in_progress"

Scenario: 多次打回后升级
  Given 任务 #4 已被打回多次
  When Reviewer 认为需要人工介入
  Then Reviewer 在线程中说明情况并建议人类处理
```

### 8.6 US-06: 线程内 Agent 协作 (P0)

```gherkin
Scenario: Agent 在线程中回复任务讨论
  Given 任务 #1 有一个关联线程
  When Agent "Coordinator" 在线程中回复 "@Distributor 子任务已创建，请分发"
  Then Distributor 收到 @mention 触发
  And Distributor 的回复出现在线程中 (而非主频道)
  And 主频道的 reply_count 更新

Scenario: 线程中的人类干预
  Given Agent 在线程中汇报进度
  When 人类在线程中回复 "请优先处理 #3，把 #4 放后面"
  Then Agent 收到人类消息
  And Agent 调整优先级

Scenario: 线程历史可追溯
  Given 任务经历了完整协作流程
  When 人类打开任务线程
  Then 看到完整的协作历史 (拆解、分发、执行、审查、修复)
  And 所有 Agent 的决策和沟通记录在线程中
```

### 8.7 US-07: 人类观察与干预 (P1)

```gherkin
Scenario: 人类在 Kanban 中看到整体进展
  Given 频道中有多个任务处于不同状态
  When 人类打开频道 Tasks Tab
  Then Kanban 5 列正确显示各任务的分布
  And 父任务显示子任务进度 (如 "2/5 done")
  And 任务的认领人正确显示

Scenario: 人类中途干预
  Given 任务 #3 正由 Agent 执行中
  When 人类手动认领 #3
  Then 任务认领人变为该人类
  And 人类可以修改任务状态
  And 原 Agent 的认领被覆盖 (通过 unclaim → claim 实现)
```

### 8.8 US-08: Agent 通过 system prompt 自定义角色 (P0)

```gherkin
Scenario: 用户为 Agent 编写自定义 system prompt
  Given 用户创建 Agent "MyReviewer"
  When 用户在 system prompt 中写入 "You are a security auditor. Your job is to review code for security vulnerabilities..."
  Then MyReviewer 在协作中按照安全审计的职责行动
  And Solo 平台不对其行为做任何角色假设

Scenario: Agent 的角色信息通过上下文注入
  Given Agent "MyReviewer" 的 system prompt 中定义了安全审计角色
  When MyReviewer 在频道中被触发
  Then 其上下文中包含用户定义的 system prompt
  And 上下文中包含 Solo 注入的 Task Protocol 协作规则
  And MyReviewer 基于两者综合判断如何行动
```

### 8.9 US-09: DM 任务支持 (P1)

```gherkin
Scenario: DM 中的任务协作
  Given 用户 A 与 Agent "Helper" 的 DM 对话
  When 用户 A 在 DM 中创建任务 "帮我分析这段代码"
  Then 任务编号为 DM 内独立编号 (如 #1)
  And 任务仅用户 A 和 Helper 可见
  And DM 的 Tasks Tab 显示该任务的 Kanban

Scenario: DM 中的 Agent 自主认领
  Given DM 中有 Agent "Helper" 和任务 #1
  When Helper 收到任务通知
  Then Helper 自主评估并认领
  And Helper 在线程中回复进度
```

### 8.10 US-11: Agent 自主选择委派模式 (P1)

```gherkin
Scenario: 小任务使用模式 A 不创建子任务
  Given Agent 认领了简单任务 "修复 typo"
  When Agent 评估: 单人可以完成，无需并行
  Then Agent 选择模式 A
  And Agent 不调用 solo task create
  And Agent 在线程中完成工作并直接交付
  And Kanban 中无新增子任务

Scenario: 复杂任务使用模式 B 拆解
  Given Agent 认领了复杂任务 "实现用户系统"
  When Agent 评估: 需要前端 + 后端 + 测试协作
  Then Agent 选择模式 B
  And Agent 创建子任务并 @mention 相关人员
  And 每个子任务独立出现在 Kanban 中

Scenario: Server 不替 Agent 选择委派模式
  Given 任何任务创建请求
  When Server 处理该请求
  Then Server 不判断任务适合哪种模式
  And Server 不下发任何"建议模式"字段
  And 模式选择完全由 Agent 在收到通知后自主完成

Scenario: 模式选择错误时人类可以纠正
  Given Agent 为一个简单任务创建了不必要的子任务
  When 人类在线程中回复 "不需要拆子任务，直接做"
  Then Agent 关闭多余子任务并在主任务线程中完成工作
```

---

## 9. 非功能需求

### 9.1 性能指标

| 指标 | 目标 | 适用功能 |
|------|------|---------|
| Agent 任务评估延迟 | P95 < 5s (从收到通知到回复) | F-AUTO-CLAIM |
| 子任务创建响应 | P95 < 500ms (API 响应) | F-AGENT-CREATE-TASK |
| 状态变更广播延迟 | P95 < 300ms (状态变更到 WS 推送) | 所有状态操作 |
| @mention 触发延迟 | P95 < 2s (从消息发送到 Agent 开始处理) | 所有 @mention 场景 |
| 线程消息加载 | P95 < 300ms (50 条线程消息) | F-THREAD-COLLABORATION |
| 父任务子树查询 | P95 < 100ms (3 层嵌套，20 个子任务) | F-PARENT-TASK |

### 9.2 可靠性

| 要求 | 说明 |
|------|------|
| 认领幂等 | 同一 Agent 对同一任务多次 claim → 第一次成功，后续返回 "already claimed" |
| 状态变更原子性 | 状态更新 + 系统消息广播在同一个 DB 事务中 |
| 死循环保护 | mention_depth >= 3 时严格阻断，记录日志 |
| 子任务一致性 | 父任务删除不破坏子任务 (SET NULL 而非 CASCADE) |
| 委派模式无状态 | Server 不存储 Agent 选择的委派模式（模式 A/B 是 Agent 行为层面的，非数据层面的） |

### 9.3 可观测性

| 要求 | 说明 |
|------|------|
| Agent 决策日志 | Agent 的 claim/unclaim/create task 操作记录到结构化日志 |
| 协作链路追踪 | 同一父任务的所有 Agent 操作可关联 (通过 parent_task_id 链) |
| @mention 触发计数 | 记录 mention_depth 分布，监控是否频繁达到深度限制 |
| 委派模式分布 | 统计有子任务的任务 vs 无子任务的任务比例 (间接反映模式 A/B 使用分布) |

### 9.4 安全

| 要求 | 说明 |
|------|------|
| Agent CLI 认证 | `solo` CLI 只能操作当前 Agent 所在频道的任务 |
| 跨频道隔离 | Agent 不能认领或修改其他频道 (未加入) 的任务 |
| DM 可见性 | DM 任务 API 验证请求者是 DM 参与者之一 |

---

## 10. 范围边界

### 10.1 本次明确不做

| 功能 | 原因 | 计划版本 |
|------|------|---------|
| 内置角色模板 (一键选择 leader/pm/rd/fe/qa) | 违背"不内置角色"设计原则，角色由用户通过 system prompt 自定义 | 不做 |
| 服务端委派模式判断（Server 替 Agent 选模式 A/B） | 违背"Agent 自主决策"核心原则。模式选择是语义判断，应由 LLM 完成 | 不做 |
| Agent 生命周期管理 (spawn/wake/sleep/restart) | 需要完整的进程管理基础设施 | v2.0 |
| 消息搜索 | 已在其他 v1.3 功能规划中 (非本次 PRD 范围) | 待定 |
| 文件上传/附件 | 同上 | 待定 |
| 电脑/机器管理页面 | 同上 | 待定 |
| 已读/未读追踪 | 需要完整的事件系统和消息确认机制 | v2.0 |
| 统一收件箱 (Inbox) | 依赖已读/未读追踪 | v2.0 |
| 推送通知 | 需要 Service Worker + VAPID 基础设施 | v2.0 |
| Agent 市场/模板 | 生态功能 | v2.0+ |
| 审批流 | Agent 自主决策是 v1.3 的核心理念，不需要审批 | — |
| Agent 间直接在频道中对话 (非任务场景) | 本次聚焦任务协作场景 | 待定 |
| 任务依赖 (Task Dependencies) | 父子任务已够用，强依赖 (blocked by) 过重 | v2.0 |
| 任务时间估算/截止日期 | 不符合轻量定位 | 待定 |
| 消息编辑/删除优化 | 不在本次任务协作主题内 | 待定 |

### 10.2 待确认项

| 问题 | 默认决策 | 影响 |
|------|---------|------|
| @mention 的 Agent 如果不在频道中是否需要提示 | 默认不提示。消息正常发送，无 Agent 被触发 | 体验 |
| 打回次数是否需要硬限制 | 默认不硬限制。Agent 在多次打回后建议升级但不阻断 | 流程 |
| 父子任务是否需要在 Kanban 中缩进显示 | 默认不缩进。通过 parent badge 显示 | UI |
| 用户文档中是否提供协作模式示例 | 提供文档示例，标注"仅供参考，非内置角色" | 文档 |

---

## 11. 依赖与风险

### 11.1 外部依赖

| 依赖项 | 状态 | 影响功能 | 备注 |
|--------|------|---------|------|
| v1.2 任务系统 Phase 1 (Kanban + Claim + asTask) | 已交付 | 全部 | 认领 API、状态机、系统消息广播已就绪 |
| `solo` CLI 二进制 | 未实现 (已设计) | F-AUTO-CLAIM, F-AGENT-CREATE-TASK, F-TASK-DELEGATION | task-system-analysis.md §6.2 已定义 5 个命令的接口 |
| Agent Workspace (CLAUDE.md 注入) | 已交付 (v1.2) | Agent 上下文注入 | WorkspaceManager 已支持多文件注入 |
| Agent system prompt 构建 (BuildSystemPrompt) | 已交付 (v1.0) | Task Protocol 注入 | 需要扩展 Task Protocol 章节 |
| Thread @agent 响应 (TriggerAgentResponseInThread) | 已实现但未调用 | F-THREAD-COLLABORATION | 需在 CreateThreadReply 中接入 |
| WebSocket Hub | 已交付 | 实时推送 | task.created / task.updated 事件广播 |
| PostgreSQL 16 | 已交付 | 全部数据操作 | parent_task_id 字段新增 |

### 11.2 技术风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| Agent 不按协议执行 (LLM 幻觉跳过 claim 直接执行) | 任务状态不更新，协作流程中断 | 中 | system prompt 多次强调 + MEMORY.md 持久化规则 + 人类可手动 claim 救场 |
| Agent 选择的委派模式不合适（该拆不拆 / 不该拆乱拆） | Kanban 混乱或缺乏追踪 | 中 | Task Protocol 中提供清晰的决策原则 + 人类可在线程中纠正 + 模式选择本身不产生数据影响（可随时切换） |
| Agent 创建的 task 质量差 (标题模糊、缺少上下文) | 后续 Agent 无法理解任务 | 中 | prompt 中给出标题格式示例 + 人类可在观察到后纠正 |
| @mention 触发风暴 (多个 Agent 同时 @mention 多个 Agent) | 频道被 Agent 消息刷屏 | 低 | mention_depth 限制 + 去重窗口 + 人类可手动 disable Agent |
| `solo` CLI 未就绪导致 Agent 无法操作 | 全部协作功能 blocked | 高 | **solo CLI 是 v1.3 的前置条件，需要优先开发** |
| 死锁或竞争条件 (多 Agent 同时操作) | 数据不一致 | 低 | 已有 DB 行锁 + 乐观锁机制 |

### 11.3 时间风险

| 风险 | 影响 | 缓解措施 |
|------|------|------|
| solo CLI 开发耗时超出预期 | 所有依赖 CLI 的功能推迟 | CLI MVP 只做 5 个命令 + 快速迭代 |
| LLM 行为不稳定导致端到端测试难以通过 | QA 阶段反复修改 prompt | 预留 prompt tuning 时间 (至少 5 轮迭代) |
| 前后端联调复杂度 (涉及 CLI + API + WS + 11 Backend) | 集成风险 | 先聚焦 Claude Code Backend 验证通路，再扩展到其他 10 个 |

---

## 12. 迭代规划

### 迭代 0: 基础设施就绪 (1 周，前置)

**目标**: solo CLI 可用 + Thread bug 修复。没有这一步，后续所有 Agent 自主操作都无法验证。

| 任务 | 负责 | 预估 | 可演示物 |
|------|------|------|---------|
| RD-CLI: `cmd/solo/` CLI 实现 (task list/claim/create/update + message send + channel members) | rd1 | 3d | `solo task claim -n 1` 可执行 |
| RD-FIX: Thread @agent 响应修复 (接入 TriggerAgentResponseInThread) | rd2 | 0.5d | 线程中 @Agent 正常回复 |
| RD-PARENT: parent_task_id migration + API 字段 | rd1 | 0.5d | 创建任务时可指定 parent |
| ARC-PROMPT: Task Protocol 协作规则 prompt 定稿 (含委派模式决策原则) | arc | 1d | Task Protocol 文本 |
| **迭代演示** | — | — | **solo CLI 可认领任务 + Thread @agent 可用** |

### 迭代 1: Agent 自主认领 + 子任务创建 (1.5 周)

**目标**: Agent 能自主认领任务 + 能创建子任务。这是 v1.3 核心价值的第一次可演示。

| 任务 | 负责 | 预估 | 可演示物 |
|------|------|------|---------|
| ARC-PROMPT-2: 扩展 Task Protocol (自主认领 + 委派模式选择 + 子任务创建指南 + 频道上下文注入) | arc | 1d | 完整 Task Protocol + 环境感知注入 |
| RD-UNCLAIM: 移除 Server 自动 claim 逻辑 | rd1 | 0.5d | Agent 不再被强制 claim |
| RD-CREATE: Agent 创建子任务 API (含 CLI `solo task create`) | rd1 | 1d | Agent 可 CLI 创建子任务 |
| FE-PARENT: 父任务子任务进度显示 | fe1 | 1d | 父任务卡片显示 "2/5 done" |
| QA-E2E-1: Agent 自主认领 + 委派模式端到端测试 (含模式 A 和模式 B) | qa1 | 1d | 单个 Agent 能认领 + 根据任务特征选择委派模式 |
| **迭代演示** | — | — | **人类创建任务 → Agent 自主认领 → 评估复杂度 → 模式 A 线程委派 或 模式 B 拆解子任务 → 子任务出现在 Kanban** |

### 迭代 2: Agent 协作链 (1.5 周)

**目标**: 完整的 Agent 间协作流水线。多个用户自定义角色的 Agent 自主协作完成复杂任务，支持两种委派模式。

| 任务 | 负责 | 预估 | 可演示物 |
|------|------|------|---------|
| RD-MENTION-AGENT: Agent 间 @mention 触发 (含 mention_depth 控制) | rd1 | 1.5d | Agent @Agent 能触发，深度限制生效 |
| RD-STATUS: 状态打回能力 (in_review → in_progress + 权限控制) | rd2 | 1d | 审查 Agent 可打回任务 |
| ARC-PROMPT-3: 协作机制 prompt 调优 (含委派模式决策准确率调优，基于 E2E 反馈迭代) | arc | 1d | Task Protocol 稳定版本 |
| FE-THREAD: 任务线程协作 UI 优化 (线程消息高亮、Agent 标签) | fe1 | 1d | 线程中 Agent 消息标注其名称 |
| QA-E2E-2: 完整协作流水线端到端测试 (3+ Agent 协作，覆盖模式 A 和模式 B) | qa1 | 2d | 多 Agent 协作流水线全部跑通 (含两种委派模式) |
| **迭代演示** | — | — | **创建任务 → Agent A 选择委派模式 → 模式 A (线程直接协作) 或 模式 B (拆解 → 分发 → 执行 → 审查 → 打回 → 修复 → done)** |

### 迭代 3: DM 任务 + 收尾 (1 周)

**目标**: DM 任务支持 + 全功能回归 + prompt tuning。

| 任务 | 负责 | 预估 | 可演示物 |
|------|------|------|---------|
| RD-DM-TASK: DM 任务 API + 编号 | rd2 | 1d | DM 中创建/认领/完成任务 |
| FE-DM-TASK: DM Tasks Tab + Kanban | fe1 | 1d | DM 视图的 Tasks Tab |
| ARC-TUNE: prompt tuning (基于 E2E 结果调优，含委派模式选择准确率) | arc | 1d | Agent 协作行为准确率 > 80%，委派模式选择合理率 > 85% |
| QA-REGRESSION: 全功能回归测试 | qa1 | 1.5d | 回归测试报告 |
| DOC: 用户文档 + Agent 协作指南 (含委派模式说明和角色定义示例，标注"仅供参考") | pm1 | 0.5d | 文档上线 |
| **迭代演示** | — | — | **DM 任务 + 全功能回归通过** |

### 总工时估算

| 角色 | 迭代 0 | 迭代 1 | 迭代 2 | 迭代 3 | 合计 |
|------|--------|--------|--------|--------|------|
| rd1 (后端) | 3.5d | 1.5d | 1.5d | — | 6.5d |
| rd2 (后端) | 0.5d | — | 1d | 1d | 2.5d |
| fe1 (前端) | — | 1d | 1d | 1d | 3d |
| arc (架构/prompt) | 1d | 1d | 1d | 1d | 4d |
| qa1 (测试) | — | 1d | 2d | 1.5d | 4.5d |
| pm1 (产品/文档) | — | — | — | 0.5d | 0.5d |

**总工时**: 约 21 人天。假设 4 人并行开发 (rd1, rd2, fe1, arc) + qa1/pm1 支持，合理预期 **4 周** 完成全部交付。

---

## 附录 A: 关键设计决策记录

| 决策 | 日期 | 理由 |
|------|------|------|
| Solo 不内置任何 Agent 角色，角色完全由用户通过 system prompt 自定义 | 2026-05-17 | 遵循 Slock 设计哲学 (参考 claude-system-prompt_副本2.md)，Agent 身份来自 MEMORY.md，随时间演化。内置角色会限制灵活性 |
| 打回状态回退到 in_progress 而非新增 rejected 状态 | 2026-05-17 | 保持状态机简洁 (5 状态)，打回原因通过线程消息传递 |
| Agent 间 @mention 深度限制为 3 | 2026-05-17 | 权衡协作深度和死循环风险 |
| 子任务不级联 (父任务 done 不自动 done 子任务) | 2026-05-17 | 子任务有独立生命周期，人类确认整个工作完成后手动收尾 |
| solo CLI 作为 Agent 的唯一平台操作接口 | 2026-05-17 | 与 task-system-analysis.md ADR-008 一致，100% Backend 覆盖 |
| @软指派 30s 优先窗口保留 | 2026-05-17 | 给被 @ 的 Agent 一个合理但不强制的时间窗口 |
| Task Protocol 作为平台注入的协作规则，不包含任何角色假设 | 2026-05-17 | 提供机制而非角色，Agent 基于自己的 system prompt 解读协议 |
| 不提供角色模板选择器，用户文档中提供示例（标注"仅供参考"） | 2026-05-17 | 避免暗示"这些是推荐角色"，保持角色定义完全由用户控制 |
| 两种委派模式（A: 单任务线程委派 / B: 子任务拆分）由 Agent 自主选择，Server 不干预 | 2026-05-17 | 模式选择是语义判断（任务复杂度、是否需要并行、是否需要独立追踪），应由 LLM 完成。硬编码规则会将平台观点强加于用户。Server 只提供两种底层能力（线程 @mention + 子任务创建），不设决策树 |
| 委派模式不存储在数据库中 | 2026-05-17 | 模式 A/B 是 Agent 行为层面的概念，非数据模型。有子任务 = 模式 B，无子任务 = 模式 A 或未拆解。不需要额外字段 |

## 附录 B: 与 v1.2 的关系

| v1.2 已有 | v1.3 保留/增强 |
|-----------|---------------|
| 任务 CRUD (创建/查看/更新/删除) | 保留 — 增加 parent_task_id 和状态打回 |
| 认领制 (Claim/Unclaim) | **改变** — Server 不再自动 claim，Agent 自主决策 |
| Kanban 看板 (5 列) | 保留 — 增加父子任务进度显示 |
| 频道内编号 (#1, #2) | 保留 — 扩展 include DM per-DM 编号 |
| 系统消息广播 | 保留 — 增加 task.created/updated 事件 |
| asTask (消息转任务) | 保留 — 人在线程中也可将讨论消息转任务 |
| Thread 面板 | 保留 — 增强 Agent 名称标签显示 |
| Agent Backend (11 种) | 保留 — 聚焦 Claude Code 先验证，再扩展到其他 |
| solo CLI 设计 | **实现** — v1.2 只设计了接口，v1.3 真正实现 |
| WorkspaceManager 文件注入 | 保留 — 注入 Task Protocol 和频道上下文 |
| Agent system prompt 配置 (已有) | **保持不变** — 用户继续通过 system prompt 定义角色，v1.3 不改变此机制 |

## 附录 C: 术语表

| 术语 | 定义 |
|------|------|
| 认领制 (Claim Model) | 任务不预先指派，Agent 自主决定是否认领，先到先得 |
| 软指派 (Soft Assign) | @mention 暗示期望认领人，但不强制。被 @ 者有 30s 优先窗口 |
| 打回 (Reject) | 审查 Agent 验收不通过时将任务状态从 in_review 回退到 in_progress |
| 委派模式 A (线程委派) | 不创建子任务，在单一线程中通过 @mention 完成所有协作。适用于一人可完成的小任务 |
| 委派模式 B (子任务拆分) | 父任务负责协调，拆解为独立子任务分发执行。适用于多人、多技能、可并行的复杂任务 |
| 委托链 (Delegation Chain) | Agent 间通过 @mention + 子任务创建形成的逐级任务分发链路 |
| mention_depth | @mention 触发深度计数器，到达 3 时停止触发 Agent |
| 父子任务 (Parent-Child Task) | 通过 parent_task_id 关联的任务层级，支持多级嵌套 |
| System Prompt | 用户创建 Agent 时定义的文本，描述 Agent 的角色、职责和能力边界 |
| Task Protocol | Solo 平台注入到每个 Agent system prompt 中的协作规则，是机制层面的通用规则，不包含角色假设。v1.3 增加了委派模式选择指南 |
| 用户自定义角色 | v1.3 的核心理念：Agent 角色由用户通过 system prompt 自由定义，Solo 不内置任何具体角色 |

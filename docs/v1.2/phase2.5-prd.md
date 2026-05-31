# Solo Phase 2.5 PRD — Thread 消息展示增强 + Task 状态展示

> 版本: 1.0
> 创建日期: 2026-05-17
> 负责人: pm agent (产品经理)
> 状态: 待评审
> 前置条件: v1.2 Phase 1 完成 (Task CRUD / Claim / Kanban / asTask)，Phase 2 (Agent 任务协议) 已完成或同步进行中

---

## 目录

1. [产品背景与目标](#1-产品背景与目标)
2. [需求清单](#2-需求清单)
3. [需求澄清 Q&A](#3-需求澄清-qa)
4. [用户故事地图](#4-用户故事地图)
5. [功能详述](#5-功能详述)
6. [验收标准](#6-验收标准)
7. [前端 UI 设计要点](#7-前端-ui-设计要点)
8. [后端 API 与 WS 事件改动](#8-后端-api-与-ws-事件改动)
9. [非功能需求](#9-非功能需求)
10. [范围边界](#10-范围边界)
11. [依赖与风险](#11-依赖与风险)
12. [迭代规划](#12-迭代规划)

---

## 1. 产品背景与目标

### 1.1 为什么要做

v1.2 Phase 1 完成后，任务系统的基础能力（Task CRUD、Claim/Unclaim、Kanban、asTask）已经可用。但用户在频道消息流中**看不到任务状态**，必须切换到 Tasks Tab 才能知道某条消息是否被转为了任务、任务当前是什么状态、由谁在处理。

同时，Thread 回复计数的基础展示已经存在（代码中已有 `reply_count` 字段和 "N 条回复" 按钮），但缺少**新回复的醒目提示**，用户容易错过线程中的新讨论。

此外，Agent 在回复时存在两个体验问题：
- Agent 回复不 @提及用户，用户无法第一时间收到通知感知
- Agent 对任务的首次回复在频道和线程同时出现（刷新后只在线程），行为不一致且造成频道消息冗余

### 1.2 成功指标

| 指标 | 测量方式 | 目标 |
|------|---------|------|
| 任务可见性 | 频道消息中 task badge 的展示率（关联了 task 的消息占比中 badge 正确渲染） | 100% |
| 线程回复感知 | 有新回复时 "new" 标记展示率 | 100% |
| Agent @提及覆盖率 | Agent 对用户消息的回复中包含 @mention 的比例 | > 95% |
| Agent 回复一致性 | Agent 对 task 的回复仅出现在 thread 的比例 | 100% |

---

## 2. 需求清单

| ID | 需求 | 描述 | 来源 | 优先级 | 类型 |
|----|------|------|------|--------|------|
| P25-01 | Thread 回复计数展示增强 | 频道消息已有的 "N 条回复" 基础上，新增回复时显示 "new" 标记或小红点 | 用户反馈 | P1 | 增强 |
| P25-02 | Task 状态内联展示 | 关联了 task 的频道消息显示任务状态徽章（如 `#3 in_progress · 由 @Helper 处理`） | Slock 对标 | P0 | 新增 |
| P25-03 | Agent 回复 @提及用户 | Agent 的回复自动 @提及触发它的用户，让用户收到通知 | 用户反馈 | P1 | 修复 |
| P25-04 | Agent 回复一致性 | Agent 对 task 的回复始终只出现在 thread 中，不会在频道消息流中重复出现 | 用户反馈 / bug | P0 | 修复 |

### 2.1 需求依赖关系

```
P25-04 (Agent 回复一致性)  ← 无依赖，纯后端修复
P25-03 (Agent @提及用户)   ← 无依赖，纯 system prompt 修改
P25-02 (Task 状态展示)     ← 依赖 tasks 表已有数据 (Phase 1 已完成)
P25-01 (Thread 回复计数)   ← 依赖 ThreadReplyNotify WS 事件 (Phase 1 已有)
```

P25-01 和 P25-02 之间存在**前端渲染协同**：同一个 MessageItem 需要同时展示 reply_count badge 和 task_status badge，两者需在布局上协调。

---

## 3. 需求澄清 Q&A

### P25-01: Thread 回复计数展示增强

**Q1: "new" 标记的触发条件是什么？**
A: 当用户上次查看该消息的线程后，有新的回复产生。实现方式：对比 `thread.last_reply_at` 与用户上次查看该线程的时间戳。MVP 阶段简化：在接收到 `thread.reply` WS 事件时，对该消息显示 "new" 标记，用户点击进入线程后清除。

**Q2: "new" 标记的生命周期？**
A: 用户点击线程面板查看后立即清除。跨会话不持久化（MVP 简化，不做 `thread_read_cursor` 表）。

**Q3: 小红点 vs "new" 文字标记？**
A: 两者结合 —— 已有未读时显示实心圆点 + "N 条回复" 文字，无未读时仅显示 "N 条回复"。

### P25-02: Task 状态内联展示

**Q1: 哪些消息需要展示 task 状态？**
A: 仅当 `tasks.message_id = messages.id` 且 task 未被删除时展示。一条消息最多关联一个 task。

**Q2: 展示哪些字段？**
A: 展示 task_number（如 `#3`）、status（如 `in_progress`）、claimer_name（如 `@Helper`）。具体格式：`[#3 in_progress · @Helper]`

**Q3: 如果没有 claimer？**
A: 状态为 `todo` 且无 claimer 时，显示 `[#3 todo · 待认领]`。

**Q4: task 状态变化时如何实时更新？**
A: 通过已有的 `task.updated` WS 事件。前端需监听该事件，当 `task.message_id` 匹配时更新对应消息的 task 展示。

### P25-03: Agent 回复 @提及用户

**Q1: Agent 应该 @谁？**
A: Agent 回复时 @提及触发它的那条消息的发送者。在 thread 中触发 → @thread 中最新的用户消息发送者。在 channel 中触发 → @该 channel 消息的发送者。

**Q2: 如果触发者是另一个 Agent？**
A: 不 @提及 Agent，仅 @提及人类用户。Agent 之间的互动不需要通知。

**Q3: 实现方式？**
A: 在 Agent 的 system prompt 中注入指令，让 Agent 在回复时自觉 @提及用户。这是 Slock 的做法 —— 不依赖后端解析和修改消息内容，保持消息生成的纯粹性。

### P25-04: Agent 回复一致性

**Q1: 问题根因是什么？**
A: 需要代码走查确认，但现象是：Agent 对 task 消息在线程中的首次回复，既出现在 thread 中也出现在 channel 消息流中。刷新页面后只在 thread 中出现（因为 `thread_id IS NOT NULL` 的消息在 channel 消息查询中被排除）。根因可能是 WS 推送时 `message.new` 事件的 `thread_id` 字段未正确设置，导致前端将其渲染为频道消息。

**Q2: 修复范围？**
A: 确认 Daemon 生成的消息在推送 WS 事件时正确携带了 `thread_id`，并确认前端在渲染时正确过滤了带 `thread_id` 的消息（不在频道消息流中展示）。

---

## 4. 用户故事地图

| ID | 用户故事 | 优先级 | 工作量 | 迭代 |
|----|---------|--------|--------|------|
| US-25.1 | 作为频道成员，我希望在频道消息流中看到线程回复计数，以便了解哪些消息有讨论 | P1 | S | Iter 1 |
| US-25.2 | 作为频道成员，我希望看到线程有新回复时显示醒目提示，以便不遗漏重要讨论 | P1 | S | Iter 1 |
| US-25.3 | 作为频道成员，我希望在频道消息流中直接看到消息关联的任务状态，以便在不切换 Tab 的情况下了解任务进展 | P0 | M | Iter 1 |
| US-25.4 | 作为频道成员，我希望任务状态变化时频道消息中的状态徽章实时更新，以便获得最新信息 | P0 | S | Iter 1 |
| US-25.5 | 作为用户，我希望 Agent 回复时 @提及我，以便我能收到通知及时查看 | P1 | S | Iter 1 |
| US-25.6 | 作为用户，我希望 Agent 对任务的回复始终只出现在线程中，以便频道消息流不被冗余的 Agent 回复干扰 | P0 | S | Iter 1 |

### 用户故事详情

**US-25.1: 作为频道成员，我希望在频道消息流中看到线程回复计数**
- 优先级: P1
- 工作量: S (前端已有基础实现，仅需 UI 优化)
- 当前状态: message-list.tsx 中 MessageItem 已渲染 `reply_count > 0` 时的 "N 条回复"
- 需求: 优化样式，确保计数准确更新

**US-25.2: 作为频道成员，我希望看到线程有新回复时显示醒目提示**
- 优先级: P1
- 工作量: S
- 依赖: thread.reply WS 事件 (已有)
- 新增: 本地状态追踪 `unreadThreads: Set<messageId>`

**US-25.3: 作为频道成员，我希望在频道消息流中直接看到消息关联的任务状态**
- 优先级: P0
- 工作量: M (前后端均需改动)
- 后端: 消息查询 JOIN tasks 表，返回 `task_number`, `task_status`, `claimer_name`
- 前端: MessageItem 新增 task badge 渲染

**US-25.4: 作为频道成员，我希望任务状态变化时频道消息中的状态徽章实时更新**
- 优先级: P0
- 工作量: S
- 依赖: task.updated WS 事件 (已有)
- 前端: useMessages hook 中监听 `task.updated` 事件更新消息的 task 元数据

**US-25.5: 作为用户，我希望 Agent 回复时 @提及我**
- 优先级: P1
- 工作量: S
- 实现: 修改 Agent system prompt 注入指令
- 不需要后端代码变更

**US-25.6: 作为用户，我希望 Agent 对任务的回复始终只出现在线程中**
- 优先级: P0
- 工作量: S
- 根因: Daemon 产生的 WS message.new 事件 `thread_id` 字段可能为空
- 修复: 确保 WS 推送时正确设置 `thread_id`

---

## 5. 功能详述

### 5.1 P25-01: Thread 回复计数展示增强

#### 5.1.1 当前状态

`message-list.tsx` 中的 `MessageItem` 组件已经在消息内容下方渲染了 thread reply count：

```tsx
{(message.reply_count ?? 0) > 0 && onReply && (
  <div className="mt-2">
    <button onClick={() => onReply(message)} className="...">
      <MessageSquare className="h-3 w-3" />
      <span>{message.reply_count} 条回复</span>
    </button>
  </div>
)}
```

#### 5.1.2 增强内容

**新增 "new" 未读标记：**

1. 前端维护一个 `unreadThreads` 状态（`Set<string>`），记录有未读回复的消息 ID
2. 收到 `thread.reply` WS 事件时，如果当前 ThreadPanel 未打开或打开的不是该消息的线程，将该消息 ID 加入 `unreadThreads`
3. 用户点击 "N 条回复" 打开 ThreadPanel 时，从 `unreadThreads` 中移除该消息 ID
4. 有未读时，回复计数按钮样式变为醒目状态：
   - 显示实心圆点（直径 8px，颜色 `--brutal-pink`）+ "N 条回复" 加粗文字
   - 或显示 "N 条回复 · 新"（带背景色标记）

**视觉设计（参考 Slock）：**

```
┌─────────────────────────────────────────────────┐
│ @张三  14:30                                    │
│ 这个架构方案大家看一下                             │
│                                                 │
│ ● 3 条回复  ← 有新回复时（圆点 + 加粗）             │
│   2 条回复   ← 无新回复时（灰色文字）               │
└─────────────────────────────────────────────────┘
```

#### 5.1.3 交互规则

| 场景 | 行为 |
|------|------|
| 收到 thread.reply 事件，ThreadPanel 未打开 | `unreadThreads.add(messageId)`，显示 "new" 标记 |
| 收到 thread.reply 事件，ThreadPanel 已打开且正是该线程 | 不标记未读（用户正在看） |
| 用户点击 "N 条回复" 打开 ThreadPanel | `unreadThreads.delete(messageId)`，清除 "new" 标记 |
| 用户关闭 ThreadPanel | 不做特殊处理（已读不回退） |
| 页面刷新 | unreadThreads 重置为空（MVP 不持久化） |
| reply_count 从 0 变为 > 0 | 显示 "1 条回复" + "new" 标记 |

---

### 5.2 P25-02: Task 状态内联展示

#### 5.2.1 功能描述

当一条频道消息被转为了 Task（通过 asTask 或直接创建任务时勾选"发送到频道"），该消息在频道消息流中应内联展示任务状态徽章。

#### 5.2.2 展示格式

参考 Slock 的 `[task #3 status=in_progress]` 格式，Solo 的 Task Badge 设计如下：

**基础格式：**
```
[#N] STATUS · @claimer_name
```

**各状态展示示例：**

| 状态 | 无 claimer | 有 claimer |
|------|-----------|-----------|
| todo | `[#3] 待处理 · 待认领` | `[#3] 待处理 · @张三` |
| in_progress | `[#3] 处理中` (无 claimer 不合法) | `[#3] 处理中 · @Helper` |
| in_review | `[#3] 待审核` (无 claimer 不合法) | `[#3] 待审核 · @Helper` |
| done | `[#3] 已完成` | `[#3] 已完成 · @Helper` |
| closed | `[#3] 已关闭` | `[#3] 已关闭 · @张三` |

**状态颜色映射：**

| 状态 | 颜色 | Tailwind class |
|------|------|----------------|
| todo | 灰色 | `bg-gray-100 text-gray-700 border-gray-300` |
| in_progress | 蓝色 | `bg-blue-100 text-blue-700 border-blue-300` |
| in_review | 黄色/橙色 | `bg-yellow-100 text-yellow-700 border-yellow-300` |
| done | 绿色 | `bg-green-100 text-green-700 border-green-300` |
| closed | 灰色 + 删除线 | `bg-gray-50 text-gray-400 border-gray-200 line-through` |

#### 5.2.3 视觉设计

Task Badge 位于消息内容下方、Thread Reply Count 上方：

```
┌─────────────────────────────────────────────────┐
│ @李四  14:25                                    │
│ 修复登录页面那个无限循环的 bug                      │
│                                                 │
│ ┌──────────────────────────────┐                │
│ │ [#3] 处理中 · @Helper       │ ← Task Badge   │
│ └──────────────────────────────┘                │
│                                                 │
│ ● 5 条回复                      ← Reply Count   │
└─────────────────────────────────────────────────┘
```

Badge 样式：
- 内联 flex 布局，左对齐
- 高度 28px，圆角 4px
- 左边框 3px 实色（颜色跟随状态）
- 背景色浅色跟随状态
- 字体大小 12px，font-mono（等宽字体）
- `#N` 加粗，状态名加粗，claimer 名正常字重
- claimer 名前有 `@` 符号
- 整个 badge 可点击，点击行为 = 打开该消息的 ThreadPanel（与点击 "N 条回复" 行为一致）

#### 5.2.4 点击行为

点击 Task Badge 的任何位置 → 打开该消息的 ThreadPanel（等同于点击 "回复"）。

#### 5.2.5 实时更新

1. 监听 `task.updated` WS 事件
2. 当 `event.message_id` 匹配某条已加载的消息时，更新该消息的 `task_status` 和 `claimer_name`
3. 状态变更时可以有短暂的 CSS 过渡动画（`transition-colors duration-300`）

#### 5.2.6 后端数据获取

消息列表查询需要 LEFT JOIN tasks 表获取关联的 task 信息：

```sql
SELECT m.id, m.channel_id, m.sender_type, m.sender_id,
       COALESCE(u.display_name, a.name, '') as sender_name,
       m.content, m.content_type, m.created_at,
       COALESCE(t_cnt.reply_count, 0) as reply_count,
       t.task_number,
       t.status as task_status,
       COALESCE(t_claimer_u.display_name, t_claimer_ag.name, '') as claimer_name
FROM messages m
LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
LEFT JOIN LATERAL (
    SELECT COUNT(*) as reply_count FROM messages tm WHERE tm.thread_id = m.id
) t_cnt ON true
LEFT JOIN tasks t ON t.message_id = m.id
LEFT JOIN users t_claimer_u ON t.claimer_id IS NOT NULL AND t_claimer_u.id = t.claimer_id
LEFT JOIN agents t_claimer_ag ON t.claimer_id IS NOT NULL AND t_claimer_ag.id = t.claimer_id
WHERE m.channel_id = $1 AND m.thread_id IS NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT $2
```

**注意**: 需要确认现有查询是否已经包含 `reply_count`。当前代码中 `MessageResponse` 结构体没有 `reply_count` 字段，前端 `mapMessageResponse` 读取了它但后端可能没返回。需要在本次 PRD 中一并确认和修复。

#### 5.2.7 MessageResponse 扩展

`MessageResponse` 结构体增加以下字段：

```go
type MessageResponse struct {
    // ... 现有字段 ...
    ReplyCount  int    `json:"reply_count,omitempty"`
    TaskNumber  int    `json:"task_number,omitempty"`
    TaskStatus  string `json:"task_status,omitempty"`
    ClaimerName string `json:"claimer_name,omitempty"`
}
```

---

### 5.3 P25-03: Agent 回复 @提及用户

#### 5.3.1 实现方式

在 Agent 的 system prompt 中注入 `@mention` 指令（参考 Slock 做法）。当 Daemon 构建 Agent 的 system prompt 时，追加以下内容：

```
## @Mention Protocol
- When replying to a human user's message, always @mention that user at the start of your reply.
- Example: @username 收到，我来处理这个任务。
- Do NOT @mention other agents.
- In a thread, @mention the user who sent the latest message you're replying to.
```

#### 5.3.2 实施位置

1. Daemon 中 `PromptBuilder` 构建 system prompt 时注入上述指令
2. 确认 `@mention` 指令已在现有的 `## Task Protocol` 之外独立存在
3. 不需要修改后端 Go 代码的消息处理逻辑

#### 5.3.3 边界条件

| 场景 | 行为 |
|------|------|
| Agent 回复 Agent | 不 @mention |
| Agent 回复自己的消息 | 不 @mention |
| Agent 回复 system 消息 | 不 @mention |
| Agent 在线程中回复 | @mention 线程中最新的用户消息发送者 |
| 用户名为中文/特殊字符 | @mention 使用用户的 display_name（系统内唯一标识） |

---

### 5.4 P25-04: Agent 回复一致性

#### 5.4.1 问题描述

当前现象：
1. 用户在频道中创建 task（或 asTask）
2. Agent 在线程中回复 task
3. Agent 的回复**同时出现在频道消息流中**（用户感觉消息被发了两次）
4. 刷新页面后，频道消息流中不再显示（因为后端按 `thread_id IS NULL` 过滤，Agent 的回复带 `thread_id`）
5. 行为不一致：首次加载显示在频道，刷新后消失

#### 5.4.2 根因分析（待代码走查确认）

推测根因：当 Agent 回复线程消息时，WS `message.new` 事件的负载中 `thread_id` 字段为空或缺失。前端 `flatToMessage` 函数在 `thread_id` 为空时将消息渲染为频道消息。刷新后，REST API 查询正确过滤了带 `thread_id` 的消息（`WHERE thread_id IS NULL`），所以消息消失。

**需要确认的关键点：**
1. Daemon 在生成回复时是否将 `thread_id` 传入消息创建
2. `message.new` WS 事件是否携带 `thread_id` 字段
3. 前端 `flatToMessage` 是否正确处理 `thread_id`

#### 5.4.3 修复方案

**后端修复（主要）：**

1. 在 `AgentService.TriggerAgentResponseInThread` 中，确保 Daemon 创建的消息记录包含正确的 `thread_id`
2. 在 WS `message.new` 事件的 `MessageNewPayload` 中确保 `ThreadID` 字段被正确填充
3. 添加后端单元测试：验证线程回复的消息在 WS 推送中 `thread_id` 不为空

**前端防御（辅助）：**

1. `flatToMessage` 函数中，如果消息有 `thread_id`，确保设置 `thread_parent_id`（与 REST 响应保持一致）
2. MessageList 渲染时，如果消息有 `thread_parent_id` 或收到了带 `thread_id` 的 WS 事件，应仅在线程面板中渲染

#### 5.4.4 验证方法

验收测试步骤：
1. 用户在频道中创建 task
2. Agent 在线程中回复
3. 断言：频道消息流中没有出现 Agent 的回复
4. 断言：线程面板中出现了 Agent 的回复
5. 刷新页面
6. 断言：频道消息流中仍然没有 Agent 的回复（一致性验证）
7. 断言：线程面板中 Agent 的回复仍然存在

---

## 6. 验收标准

### US-25.1: Thread 回复计数展示

```gherkin
Scenario: 频道消息显示已有线程的回复计数
  Given 频道 "general" 中有一条消息 M1
  And M1 的线程中有 3 条回复
  When 我查看频道消息列表
  Then M1 下方显示 "3 条回复"
  And 点击 "3 条回复" 打开线程面板

Scenario: 频道消息无线程回复时不显示计数
  Given 频道 "general" 中有一条消息 M2
  And M2 没有线程回复
  When 我查看频道消息列表
  Then M2 下方不显示任何回复计数
```

### US-25.2: 新回复醒目提示

```gherkin
Scenario: 收到新线程回复时显示未读标记
  Given 频道 "general" 中有一条消息 M1
  And M1 的线程已有 1 条回复
  And 我未打开 M1 的 ThreadPanel
  When 另一个用户在 M1 的线程中发送了新回复
  Then M1 的回复计数更新为 "2 条回复"
  And 显示未读圆点标记（或 "new" 标记）

Scenario: 打开线程后清除未读标记
  Given M1 显示未读标记
  When 我点击 M1 的 "N 条回复" 打开线程面板
  Then 未读标记被清除
  And 回复计数保持显示但样式变为普通

Scenario: 正在查看线程时不触发未读标记
  Given 我已打开 M1 的 ThreadPanel 正在查看
  When 另一个用户在 M1 的线程中发送了新回复
  Then 线程面板中出现新消息
  And M1 的回复计数更新但无未读标记
```

### US-25.3: Task 状态内联展示

```gherkin
Scenario: 消息关联了 task 时显示 task badge
  Given 频道 "general" 中有一条消息 M1
  And M1 通过 asTask 被转为任务 #3
  And 任务 #3 状态为 in_progress，由 Agent "Helper" 认领
  When 我查看频道消息列表
  Then M1 下方显示 task badge "[#3] 处理中 · @Helper"
  And badge 颜色为蓝色（in_progress 状态色）

Scenario: 消息未关联 task 时不显示 task badge
  Given 频道 "general" 中有一条普通消息 M2
  And M2 未被转为任务
  When 我查看频道消息列表
  Then M2 下方不显示 task badge

Scenario: 未认领的任务显示"待认领"
  Given 消息 M1 关联了任务 #5
  And 任务 #5 状态为 todo，无人认领
  When 我查看频道消息列表
  Then M1 下方显示 task badge "[#5] 待处理 · 待认领"

Scenario: 已完成的任务显示绿色 badge
  Given 消息 M1 关联了任务 #3
  And 任务 #3 状态为 done
  When 我查看频道消息列表
  Then M1 下方显示 task badge "[#3] 已完成 · @Helper"
  And badge 颜色为绿色

Scenario: 点击 task badge 打开线程面板
  Given M1 显示 task badge
  When 我点击 task badge 的任意位置
  Then 右侧打开 M1 的线程面板
  And 线程面板显示关联的 task 信息
```

### US-25.4: Task 状态实时更新

```gherkin
Scenario: Agent 认领任务后 badge 实时更新
  Given M1 下方显示 "[#3] 待处理 · 待认领"
  When Agent "Helper" 认领了任务 #3（状态变为 in_progress）
  Then 通过 WS 收到 task.updated 事件
  And M1 下方的 badge 实时更新为 "[#3] 处理中 · @Helper"
  And badge 颜色从灰色变为蓝色

Scenario: 任务完成后 badge 更新
  Given M1 下方显示 "[#3] 处理中 · @Helper"
  When Agent "Helper" 将任务 #3 状态更新为 done
  Then M1 下方的 badge 实时更新为 "[#3] 已完成 · @Helper"
  And badge 颜色变为绿色
```

### US-25.5: Agent 回复 @提及用户

```gherkin
Scenario: Agent 回复时 @提及消息发送者
  Given 频道 "general" 中有用户 "张三"
  When "张三" 发送消息 "@Helper 帮我查一下"
  And Agent "Helper" 被触发
  Then Helper 的回复以 "@张三" 开头
  And 回复内容包含对查询的回应

Scenario: Agent 不 @提及其他 Agent
  Given Agent "Helper" 在线程中回复
  And 线程中最后一条消息来自 Agent "Reviewer"
  When Helper 生成回复
  Then Helper 的回复不以 "@Reviewer" 开头
  And Helper 不 @提及任何 Agent

Scenario: Agent 不 @提及自己
  Given Agent "Helper" 在频道中
  When Helper 发送了一条消息
  And 另一个 Agent "Reviewer" 回复时
  Then Reviewer 的回复不以 "@Helper" 开头（Agent 间不互相 @）
```

### US-25.6: Agent 回复一致性

```gherkin
Scenario: Agent 对 task 的回复仅出现在线程中
  Given 频道 "general" 中有 task #3（关联消息 M1）
  And Agent "Helper" 在线程中回复 task #3
  When Helper 发送回复
  Then 频道消息流中不出现 Helper 的回复
  And 线程面板中出现 Helper 的回复

Scenario: 刷新后 Agent 回复仍仅出现在线程中
  Given Helper 已在线程中回复了 task #3
  When 我刷新页面
  And 重新进入频道 "general"
  Then 频道消息流中仍然不出现 Helper 的回复
  And 打开 M1 的线程面板，Helper 的回复存在

Scenario: Agent 对普通线程消息的回复也仅在线程中
  Given 频道 "general" 中有一条普通消息 M2
  And M2 的线程中有用户发送的消息
  When Agent 在 M2 的线程中回复
  Then 频道消息流中不出现 Agent 的回复
  And 线程面板中出现 Agent 的回复
```

---

## 7. 前端 UI 设计要点

### 7.1 MessageItem 布局结构

增强后的 `MessageItem` 布局（从上到下）：

```
┌──────────────────────────────────────────────────┐
│ [Avatar]  @sender_name    14:30                  │ ← 头部（不变）
│                                                    │
│          message content                          │ ← 内容（不变）
│                                                    │
│          ┌─────────────────────────────────┐      │
│          │ [#3] 处理中 · @Helper           │      │ ← Task Badge (新增)
│          └─────────────────────────────────┘      │
│                                                    │
│          ● 5 条回复 · 新                         │ ← Reply Count (增强)
│                                                    │
│                              [编辑] [删除] [回复]  │ ← Hover Actions (不变)
└──────────────────────────────────────────────────┘
```

**间距规范：**
- Task Badge 上边距: `mt-2` (8px)
- Task Badge 与 Reply Count 间距: `mt-1.5` (6px)
- Reply Count 上边距（无 Task Badge 时）: `mt-2` (8px)

### 7.2 Task Badge 组件规格

```
高度: 28px (h-7)
内边距: px-2 py-0.5
圆角: rounded (4px)
左边框: border-l-[3px] (颜色跟随状态)
背景: 状态对应浅色 (如 bg-blue-50)
文字: text-xs (12px)
字体: font-mono
布局: flex items-center gap-1.5

状态指示器: 小圆点 (直径 6px)，颜色跟随状态
任务编号: font-bold
状态名: font-bold
分隔符: "·" (middle dot, text-muted-foreground)
认领人: normal weight，前有 "@"
```

**颜色定义（CSS 变量或 Tailwind 预设）：**

| 状态 | 边框色 | 背景色 | 文字色 | 圆点色 |
|------|--------|--------|--------|--------|
| todo | border-gray-400 | bg-gray-50 | text-gray-600 | bg-gray-400 |
| in_progress | border-blue-500 | bg-blue-50 | text-blue-700 | bg-blue-500 |
| in_review | border-yellow-500 | bg-yellow-50 | text-yellow-700 | bg-yellow-500 |
| done | border-green-500 | bg-green-50 | text-green-700 | bg-green-500 |
| closed | border-gray-300 | bg-gray-50/50 | text-gray-400 | bg-gray-300 |

### 7.3 Reply Count 增强规格

**无未读：**
```
[MessageSquare icon 14px] 5 条回复
颜色: text-muted-foreground
字体: text-xs font-heading font-bold
```

**有未读：**
```
[实心圆点 8px pink] [MessageSquare icon 14px] 5 条回复 · 新
圆点颜色: bg-brutal-pink
文字颜色: text-brutal-pink
字体: text-xs font-heading font-bold
"· 新" 文字: bg-brutal-pink text-white px-1.5 py-0.5 rounded text-[10px]
```

### 7.4 响应式考量

- Task Badge 在小屏幕（< 640px）下最大宽度 100%，不能超出消息区域
- 当 claimer_name 过长（> 20 字符）时 truncate 显示 `@verylongnametrunc...`
- Reply Count 和 Task Badge 在移动端保持相同布局

### 7.5 交互状态覆盖

| 组件 | 加载态 | 空态 | 错误态 | 边缘情况 |
|------|--------|------|--------|----------|
| Task Badge | 消息加载时自然等待（属于消息的一部分） | 消息无关联 task 时不显示 | task 数据加载失败时不显示 badge（降级为普通消息） | task 被删除后 badge 消失 |
| Reply Count | 消息加载时自然等待 | reply_count=0 时不显示 | - | reply_count 从 >0 变为 0 时隐藏 |
| Unread Dot | - | - | - | 用户快速切换频道时清理 unreadThreads |

---

## 8. 后端 API 与 WS 事件改动

### 8.1 需要新增的后端能力

#### 8.1.1 消息列表查询增强

**GET /api/v1/channels/{channelID}/messages** 的响应增加字段：

```json
{
  "messages": [
    {
      "id": "uuid",
      "channel_id": "uuid",
      "sender_type": "user",
      "sender_id": "uuid",
      "sender_name": "张三",
      "content": "修复登录页面那个无限循环的 bug",
      "content_type": "text",
      "created_at": "2026-05-17T14:25:00Z",
      "reply_count": 5,
      "task_number": 3,
      "task_status": "in_progress",
      "claimer_name": "Helper"
    }
  ],
  "has_more": false
}
```

**改动点：**
1. `MessageResponse` 结构体增加 `ReplyCount`, `TaskNumber`, `TaskStatus`, `ClaimerName` 字段
2. SQL 查询增加 LEFT JOIN threads (或 COUNT subquery) + LEFT JOIN tasks + LEFT JOIN users/agents (for claimer)
3. 确保 `reply_count` 正确计算（线程根消息的子消息数量）

#### 8.1.2 WS message.new 事件增强

`MessageNewPayload` 已有 `ThreadID` 字段。需要确保：
1. 对于线程中的消息，`ThreadID` 字段被正确填充
2. 前端根据 `thread_id` 将消息路由到线程面板而非频道消息流

**不需要新增 WS 事件类型**，但需要修复现有 `message.new` 事件中 `thread_id` 为空导致的消息重复渲染问题。

#### 8.1.3 WS thread.reply 事件（已有，无需改动）

已有的 `thread.reply` 事件：

```json
{
  "type": "thread.reply",
  "channel_id": "uuid",
  "thread_id": "uuid",
  "reply_count": 5,
  "last_reply_at": "2026-05-17T14:30:00Z",
  "latest_reply": {
    "id": "uuid",
    "sender_type": "user",
    "sender_id": "uuid",
    "sender_name": "张三",
    "content": "最新回复的内容...",
    "created_at": "2026-05-17T14:30:00Z"
  }
}
```

前端通过此事件更新 `reply_count` 和未读标记。

### 8.2 前端需要新增的类型定义

```typescript
// 扩展 Message 类型
interface Message {
  // ... 现有字段 ...
  reply_count?: number;
  task_number?: number;       // 新增
  task_status?: TaskStatus;   // 新增
  claimer_name?: string;      // 新增
}
```

### 8.3 改动汇总

| 改动项 | 层级 | 类型 | 工作量 |
|--------|------|------|--------|
| MessageResponse 增加 task 字段 | 后端 | 新增 | S |
| 消息查询 SQL LEFT JOIN tasks | 后端 | 修改 | S |
| 确保 message.new WS 事件 thread_id 正确 | 后端 | Bug 修复 | S |
| Agent system prompt 增加 @mention 指令 | Daemon | 修改 | XS |
| Message 类型扩展 | 前端 | 修改 | XS |
| useMessages hook 处理 task 字段 | 前端 | 修改 | S |
| MessageItem 增加 TaskBadge 组件 | 前端 | 新增 | M |
| MessageItem 增加 unread 标记逻辑 | 前端 | 新增 | S |
| useMessages hook 监听 task.updated | 前端 | 修改 | S |

---

## 9. 非功能需求

### 9.1 性能

| 指标 | 目标 | 说明 |
|------|------|------|
| 消息列表查询增加 JOIN 后的响应时间 | P95 < 250ms | 原有 P95 < 200ms，增加 2 个 LEFT JOIN，预期增加 < 50ms |
| Task Badge 渲染 | 不增加消息列表首屏渲染时间 | 纯客户端渲染，无额外网络请求 |
| WS 事件处理延迟 | 与现有一致 | 不引入新的 WS 事件类型 |

### 9.2 向后兼容

- 新增的 `task_number`、`task_status`、`claimer_name` 字段使用 `omitempty`，老版本客户端忽略
- 消息查询在没有关联 task 时这些字段为 null/空，不影响现有功能
- System prompt 修改仅影响新创建的 Agent 调用，不影响已有对话

### 9.3 数据库索引

- `tasks.message_id` 已有索引 `idx_tasks_message`（迁移 000013 已创建）
- `messages.thread_id` 已有索引 `idx_messages_thread`
- 新的 LEFT JOIN tasks 查询走 `idx_tasks_message` 索引，无需新增

---

## 10. 范围边界

### 本次 Phase 2.5 包含

- [x] Thread 回复计数的新回复 "未读" 标记
- [x] 频道消息中的 Task 状态内联展示
- [x] Agent 回复 @提及用户（system prompt 注入）
- [x] Agent 回复一致性修复（thread_id 正确传递）

### 本次 Phase 2.5 不包含

- 跨会话持久化未读线程状态（不做 `thread_read_cursor` 表）
- Thread 消息列表中显示 task 状态（那是 ThreadPanel 的责任，可在后续迭代处理）
- @提及用户的浏览器通知/推送通知（那是通知系统的事，v1.3+）
- 消息搜索中展示 task 状态
- DM 中的 task 状态展示（DM 的 task 功能在 Phase 3 处理）
- Task Badge 的编辑功能（不能通过点击 badge 直接修改状态——需在 TaskBoard 或 ThreadPanel 操作）
- task 被删除后频道消息中 badge 的"已删除"残留状态（task 删除时通过 WS 通知清理）

---

## 11. 依赖与风险

### 11.1 依赖

| 依赖项 | 状态 | 说明 |
|--------|------|------|
| tasks 表 (000011) | ✅ 已完成 | Phase 1 已创建 |
| tasks.message_id 索引 | ✅ 已完成 | 000013 迁移已创建 |
| thread.reply WS 事件 | ✅ 已完成 | thread.go 已实现 |
| task.updated WS 事件 | ✅ 已完成 | task handler 已实现 |
| 前端 MessageItem 组件 | ✅ 已完成 | message-list.tsx |
| 前端 useMessages hook | ✅ 已完成 | use-messages.ts |

### 11.2 风险

| # | 风险 | 概率 | 影响 | 缓解措施 |
|---|------|------|------|----------|
| R1 | LEFT JOIN tasks 导致消息查询性能下降 | 低 | 中 | 已有索引 `idx_tasks_message`，单条消息最多一个 task，JOIN 开销可控 |
| R2 | reply_count 当前后端未返回（前端读取了但后端没填充） | 中 | 低 | 需在 SQL 中增加 COUNT subquery 或 LEFT JOIN threads 表 |
| R3 | Agent @mention 指令可能与现有 system prompt 冲突 | 低 | 低 | 追加指令，不与现有 ## Task Protocol 冲突 |
| R4 | P25-04 根因不在 WS thread_id，而在 Daemon 消息创建 | 中 | 中 | 代码走查时覆盖 Daemon 消息创建 → WS 推送整条链路 |

---

## 12. 迭代规划

### Iter 1 (预计 2-3 天)

**迭代目标**: 完成全部 4 个需求的开发、前后端联调、并通过验收测试。

**可演示产出**:
1. 频道消息流中可见 task 状态 badge，实时更新
2. 线程回复计数带未读标记
3. Agent 回复以 @用户 开头
4. Agent 对 task 的回复始终只在线程中出现

| ID | 任务 | 负责人 | 预估 | 依赖 |
|----|------|--------|------|------|
| P25-B-01 | 修复 Agent 回复一致性 (thread_id 正确填充) | rd1 | 2h | 无 |
| P25-B-02 | 消息查询 SQL LEFT JOIN tasks + reply_count | rd1 | 2h | 无 |
| P25-B-03 | MessageResponse 结构体扩展 + 序列化 | rd1 | 1h | P25-B-02 |
| P25-B-04 | Agent system prompt @mention 指令注入 | rd1 | 1h | 无 |
| P25-F-01 | Message 类型扩展 (task_number, task_status, claimer_name) | fe1 | 0.5h | P25-B-03 |
| P25-F-02 | TaskBadge 组件开发 | fe1 | 3h | P25-F-01 |
| P25-F-03 | MessageItem 集成 TaskBadge + Unread Dot | fe1 | 2h | P25-F-02 |
| P25-F-04 | useMessages hook 监听 task.updated 事件 | fe1 | 1.5h | P25-F-01 |
| P25-QA-01 | 验收测试：全部 4 个需求的验收标准 | qa1 | 3h | 全部完成 |

**并行策略**:
- rd1 4 个后端任务可顺序执行（总计 6h）
- fe1 的 P25-F-01 依赖 P25-B-03，但可以先用 mock 数据并行开发 P25-F-02 组件
- P25-B-01（Agent 回复一致性）和 P25-B-02（消息查询）完全独立，可并行
- qa1 在所有开发完成后介入

---

## 附录 A: 参考实现 (Slock message 格式)

Slock 中消息的 task 标注格式：

```
@Alice: Fix the login bug [task #3 status=in_progress]
```

Solo 采用 Badge 组件方式替代内联文本，提供更好的视觉层次和可点击性。

## 附录 B: 与现有代码的精确对照

### 需要修改的文件

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `internal/server/handler/message.go` | 修改 | SQL 查询 + MessageResponse 结构体扩展 |
| `internal/server/service/agent.go` | 修改 | 确保 TriggerAgentResponseInThread 正确传递 thread_id |
| `internal/server/ws/message.go` | 确认 | 确认 MessageNewPayload.ThreadID 在正确场景被填充 |
| `cmd/daemon/main.go` 或 prompt builder | 修改 | Agent system prompt 注入 @mention 指令 |
| `frontend/lib/types.ts` | 修改 | Message 类型扩展 |
| `frontend/lib/hooks/use-messages.ts` | 修改 | 处理 task 字段 + 监听 task.updated |
| `frontend/components/dashboard/message-list.tsx` | 修改 | MessageItem 集成 TaskBadge + Unread Dot |

### 需要新增的文件

| 文件 | 说明 |
|------|------|
| `frontend/components/dashboard/task-badge.tsx` | TaskBadge 组件 |
| 对应的后端单元测试 + 前端组件测试 | 如有必要 |

---

> 文档结束。本 PRD 待评审后进入开发。

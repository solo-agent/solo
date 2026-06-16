# Agent 协作 — 事项 1 深度设计

> 版本: 2.0（实施后更新）
> 日期: 2026-06-14（设计）/ 2026-06-16（实施后修订）
> 状态: 已实施，文档反映实际代码
> 范围: 仅事项 1(Agent 关系→行为接线)
> 后续: 事项 2-5 单独设计,本文末尾列待办

---

## 1. 背景与目标

### 1.1 问题

Solo 已通过 6 步路线图实现 Agent 协作基础:
- `agent_relationships` 表(原 4 类:`reports_to` / `delegates_to` / `collaborates_with` / `escalates_to`)
- `task_dependencies` 表 + `parent_task_id`(sub-task)
- `agent_delegations` 表 + 状态机

**但是运行时行为接线不全**:`escalates_to` 是唯一接通的关系,其余 3 类仅停留在 DB + 图谱层。

### 1.2 目标

把 Agent 关系从"声明式数据"升级为"行为驱动图谱":
- 2 类关系(精简自原 4 类):`assigns_to` + `collaborates_with`
- 5 个保留子功能:**sub-task 通知、Watchdog 升级、事件总线、Template 库、动态 MD 文档**
- **关系是 task 行为模式,不是认知 hint**——不直接写进 prompt,通过 `RELATIONSHIPS.md` 动态文档作为信息出口

### 1.3 实施后变更

原设计有 8 个子功能，实施中经过讨论删除了 3 个，架构上做了重大调整：

| 子功能 | 设计 | 实施结果 |
|--------|------|---------|
| `assigns_to` mention 校验 | server 强制拦截 | **删除** — 失去灵活性 |
| `assigns_to` sub-task 通知 | 频道 WS 广播 | **改为 DM 系统消息** — `AgentNotifier.NotifyClaim/Complete`，落库 + `thread_id=NULL` + `TriggerAgentResponse` |
| `assigns_to` Watchdog 升级 | 频道 WS 广播 | **改为 DM 系统消息** — `AgentNotifier.NotifyEscalation/NotifyRemind`，新增 server 统一调度器替代 daemon ticker |
| `assigns_to` smart default | UI 预填 | **删除** — 将重新设计 |
| `collaborates_with` smart default | 协作者排序 | **删除** — 将重新设计 |
| 关系变更事件总线 | WebSocket 事件 | **保留** — 驱动 RELATIONSHIPS.md 重生成 |
| Template Library | 预制团队模板 | **保留** — 5 个内置模板 |
| RELATIONSHIPS.md | 动态 MD 文档 | **保留** — agent Read 引用 |

---

## 2. 设计决策(讨论沉淀)

### 2.1 关系类型精简:4 类 → 2 类

| 原关系 | 新归属 | 理由 |
|--------|--------|------|
| `reports_to` + `delegates_to` + `escalates_to` | 合并为 **`assigns_to`** | 3 类语义重叠,UX 合并为单箭头 |
| `collaborates_with` | 保留为 **`collaborates_with`** | 对称 peer 协作,语义独立 |

**最终关系模型**:

| 关系 | UI | 含义 | 行为接线 |
|------|-----|------|---------|
| `assigns_to(A, B)` | 单箭头 A→B | A 经常创建 sub-task @mention B,或 B 经常 claim A 的 sub-task | DM 系统消息通知 A + 事件总线更新 RELATIONSHIPS.md |
| `collaborates_with(A, B)` | 双箭头 A↔B | A 和 B 经常互相 mention,经常 claim 同一类 task | 事件总线更新 RELATIONSHIPS.md |

### 2.2 关系 ≠ 认知 hint

**关键决策**:
- ❌ 关系**不存**"擅长什么"
- ✅ "擅长什么"写在 `agents.instructions`(system prompt initial role)
- ✅ 关系是 **task 行为模式**(可观察、可度量)
- ✅ 关系**不直接写进 prompt**——通过 `RELATIONSHIPS.md` 动态文档
- ✅ 不存 `instruction` 字段（迁移 000036 已删除）

### 2.3 Solo 任务模型:claim + mention

- Solo migration 000013 把 `assignee` 替换为 `claimer`
- 委托 = A 创建 sub-task + 描述里 @mention B,B 自己 claim
- **没有"assign to"字段**,只有"谁 claim 了"

### 2.4 消息通知架构（实施后新增）

所有 agent 通知统一走 DM 系统消息模式：

```
AgentNotifier.NotifyXxx
  → ensureAgentDM(agentID) → 查找/创建 owner↔agent DM 通道
  → INSERT INTO messages (sender_type='system', content_type='system', thread_id=NULL)
  → WS message.new → DM channel
  → TriggerAgentResponse(dmID, msgID) → agent 立即唤醒
```

`thread_id=NULL` 确保消息能被 `getRecentMessages` 查询到（该 SQL 过滤 `WHERE thread_id IS NULL`）。

---

## 3. 保留的子功能设计

### 3.1 `assigns_to` sub-task 通知（AgentNotifier DM）

**功能**: sub-task 被 B claim 或完成,通过 DM 系统消息通知 A(sub-task 创建者)。

**不校验 `assigns_to` 边** —— 所有 sub-task creator 都会收到通知。关系边仅用于信息展示（RELATIONSHIPS.md），不作为硬约束。

**行为**:
- B claim T5.1 → `task.go:ClaimTask` → `agentNotifier.NotifyClaim(ctx, taskID, claimerID=B)`
  - 查 task info（title, task_number, channel, creator_id=A）
  - self-claim 跳过（A == B）
  - `ensureAgentDM(A)` → 查/建 A 与其 owner 之间的 DM
  - INSERT system message → WS 推 DM → `TriggerAgentResponse` 唤醒 A
- B 完成 T5.1 → `agentNotifier.NotifyComplete(...)` → 同上

**消息格式**:
- claim: `📋 Task claimed — #N title in #channel was claimed by @name.` + 引导
- complete: `✅ Task completed — #N title in #channel was completed by @name.` + 引导

**关键代码路径**:
```
B claim T5.1
  POST /api/v1/tasks/{T5.1}/claim → task.go:ClaimTask
    → agentNotifier.NotifyClaim(ctx, taskID, claimerID)
      → lookupTaskNotification → task info
      → ensureAgentDM(creatorID) → DM channel ID
      → sendSystemMessage(dmID, content)
        → INSERT INTO messages (..., 'system', ..., NULL)
        → WS BroadcastToChannel
        → TriggerAgentResponse(dmID, msgID, "system", "system", ...)
```

### 3.2 `assigns_to` Watchdog 升级（AgentNotifier DM + Server Scheduler）

**功能**: sub-task 超时,Watchdog 通过 DM 通知相关 agent。

**行为**:
- server `scheduler.Start()` 每 30s:
  - `tickWatchdog()` → `ScanTimeouts()` → `CheckOverdueTasks()`
  - 对每个超时 task:
    - `timeout_action=remind` → `AgentNotifier.NotifyRemind(claimer, deadline)` → DM
    - `timeout_action=escalate` → `AgentNotifier.NotifyEscalation(task, claimer)` → DM to creator
    - `timeout_action=unclaim` → 直接 UPDATE tasks,不通知

**消息格式**:
- remind: `⏰ Task overdue — #N title in #channel is past deadline (time).` + 引导
- escalation: `⚠️ Task overdue — #N title in #channel is overdue and has been escalated to you.` + 3 步引导

**关键代码路径**:
```
scheduler.Start() [server 进程]
  → 30s ticker
  → tickWatchdog()
    → watchdogSvc.ScanTimeouts()
      → CheckOverdueTasks()
      → HandleOverdueTask(w)
        ├── remind → agentNotifier.NotifyRemind(taskID, claimerID, deadline) → DM
        ├── escalate → agentNotifier.NotifyEscalation(taskID, claimerID) → DM
        └── unclaim → UPDATE tasks SET claimer_id=NULL

scheduler.Start()
  → tickReminders()
    → reminderSvc.CheckDueReminders()
    → agentNotifier.Notify(agentID, content) → DM
    → MarkFired / RescheduleRecurring
```

**架构变更**: 原 daemon 侧 `startTicker`/`checkReminders`/`checkWatchdog`/`deliverReminderMessage` 已删除。定时扫描统一在 server 侧，daemon 仅负责 agent 执行调度。

### 3.3 关系变更事件总线

**功能**: 关系 CRUD 触发事件,驱动 RELATIONSHIPS.md 更新 + UI 视图同步。

**事件类型**:
- `relationship:created` / `relationship:updated` / `relationship:deleted`

**行为**:
- 关系 INSERT/UPDATE/DELETE → server 触发事件
- 监听器:
  1. RELATIONSHIPS.md 重新生成
  2. UI 关系图更新（WebSocket）
  3. 前端 `use-relationships.ts` 增量更新本地缓存

**关键代码路径**:
```
internal/server/service/relationship_events.go:PublishCreated/PublishUpdated/PublishDeleted
  → hub.Broadcast()
  → 前端 use-relationships.ts WebSocket 监听 → 增量更新
internal/server/service/agent_relationship.go:Create/Delete
  → eventPub.PublishXxx()
  → mdGen.GenerateForAgent(fromAgentID + toAgentID)
```

### 3.4 Template Library(预制团队模板)

**功能**: Solo 提供预制团队模板,一键创建 N 个 agent + 预制关系集。

**新表**:`agent_templates` (migration 000036)

**Solo 内置模板**:
- `dev-team`(PM / TPM / FE / BE / QA) —— leader 到所有 member 建 `assigns_to`
- `product-team`(PM / Designer / Researcher)
- `research-team`(Researcher / Analyst / Writer)
- `marketing-team`(Marketer / Writer / Designer)
- `customer-support-team`(CS Lead / CS Agent / Escalation Lead)

**关键代码路径**:
```
POST /api/v1/templates/{id}/apply
  → template.go:Apply
    → INSERT agents
    → INSERT agent_relationships (leader→member, assigns_to, weight=1.0)
    → mdGen.GenerateForAgent(each agentID)
```

### 3.5 `RELATIONSHIPS.md` 动态生成文档

**功能**: server 在关系变更时,自动生成/更新 `~/.solo/agents/<id>/workspace/RELATIONSHIPS.md`。

**生成时机**:
- 关系 CRUD 事件
- task claim/complete 事件
- ~~5 分钟定时刷新~~（未实施）

**文档结构**:
```markdown
# Agent @Bob — Relationships

## 我委托给 (@assigns_to from me)
- @Carol (active: 1, total: 12)

## 委托给我 (@assigns_to to me)
- @PM (active: 0, total: 3)

## 我的协作者 (@collaborates_with)
- @Eve (weight: 5)

## 最近活动
- 2026-06-14 09:00 — @Carol claim T5.1
```

**Agent 使用方式**:
- prompt 留 hint: `Read ~/.solo/agents/<id>/workspace/RELATIONSHIPS.md for current relationships.`
- agent 主动 Read / cat / grep 引用

---

## 4. 已删除的子功能

### 4.1 `assigns_to` mention 校验

**原设计**: A @mention B 时 server 校验 `assigns_to(A, B)` 是否存在，不存在则三选项拦截。

**删除原因**: 失去灵活性——agent 应该能自由 @mention 任何人。

**删除文件**: `mention_validator.go` + `mention_validator_test.go` + `task.go` 中的校验块和 `parseMentionsFromDescription`

### 4.2 `assigns_to` smart default (UI 预填)

**原设计**: @mention 候选按 assigns_to 排序预填。

**删除原因**: 用户反馈"完全没看到你在干什么"，将重新设计。

**删除文件**: `mention-candidates.tsx` + `agents-api.ts` + handler `ListMentionCandidates`

### 4.3 `collaborates_with` smart default (协作者排序)

**原设计**: 任务分解面板按 weight 排序协作者。

**删除原因**: 同上。

**删除文件**: `collaborator-suggestions.tsx` + `agents-api.ts` + handler `ListCollaborators`

---

## 5. Schema 改动(migration 000033 / 000036)

```sql
-- 1. 合并 3 类为 assigns_to
UPDATE agent_relationships SET rel_type = 'assigns_to'
WHERE rel_type IN ('reports_to', 'delegates_to', 'escalates_to');

-- 2. CHECK 约束收紧为 2 类
ALTER TABLE agent_relationships
    ADD CONSTRAINT agent_relationships_rel_type_check
    CHECK (rel_type IN ('assigns_to', 'collaborates_with'));

-- 3. 唯一索引
CREATE UNIQUE INDEX idx_rel_assigns_to_unique
    ON agent_relationships(from_agent_id, to_agent_id)
    WHERE rel_type = 'assigns_to';

-- 4. 删 instruction 字段
ALTER TABLE agent_relationships DROP COLUMN IF EXISTS instruction;

-- 5. 新增 agent_templates 表
CREATE TABLE agent_templates (...);
```

---

## 6. 新增文件清单

| 文件 | 用途 |
|------|------|
| `internal/server/service/agent_notifier.go` | DM 系统消息通知服务（NotifyClaim/Complete/Escalation/Remind/Notify） |
| `internal/server/service/scheduler.go` | 统一 server 侧定时调度器（reminder + watchdog） |
| `docs/desc/message-types.md` | 消息类型全景文档（56 种消息） |

---

## 7. 删除文件清单

| 文件 | 原因 |
|------|------|
| `internal/server/service/mention_validator.go` + test | mention 校验删除 |
| `internal/server/service/subtask_notifier.go` + test | 被 AgentNotifier 替代 |
| `internal/server/service/watchdog_escalation_test.go` | helper 缺失，需重写 |
| `frontend/components/teams/mention-candidates.tsx` | smart default 删除 |
| `frontend/components/teams/collaborator-suggestions.tsx` | smart default 删除 |
| `frontend/lib/agents-api.ts` | smart default API 删除 |
| Daemon 侧 `startTicker`/`checkReminders`/`checkWatchdog`/`deliverReminderMessage`/`verifyEscalationRelationship`/`computeNextCron` | 合并到 server scheduler |

---

## 8. 代码路径（实施后）

### 8.1 Agent DM 通知 (AgentNotifier)

```
AgentNotifier (internal/server/service/agent_notifier.go)
  ├── NotifyClaim(taskID, claimerID) → DM to creator
  ├── NotifyComplete(taskID, claimerID) → DM to creator
  ├── NotifyEscalation(taskID, claimerID) → DM to creator
  ├── NotifyRemind(taskID, claimerID, deadline) → DM to claimer
  └── Notify(agentID, content) → DM to agent (通用)

  ensureAgentDM(agentID) → owner↔agent DM 通道
  lookupTaskNotification(taskID, actorID) → task info
  sendSystemMessage(dmID, content)
    → INSERT messages (system, thread_id=NULL)
    → WS BroadcastToChannel(dmID, message.new)
    → TriggerAgentResponse(dmID, msgID, ...)
```

### 8.2 统一调度 (Scheduler)

```
scheduler.Start()
  ├── tickReminders()
  │     → CheckDueReminders()
  │     → AgentNotifier.Notify(agentID, "⏰ Reminder: msg")
  │     → MarkFired / RescheduleRecurring
  └── tickWatchdog()
        → ScanTimeouts()
          ├── remind → AgentNotifier.NotifyRemind
          ├── escalate → AgentNotifier.NotifyEscalation
          └── unclaim → UPDATE tasks
```

### 8.3 事件总线 (Relationship Events)

```
agent_relationship.go:Create/Delete
  → eventPub.PublishXxx()
    → hub.Broadcast(relationship:created/deleted/updated)
    → mdGen.GenerateForAgent(fromAgentID + toAgentID)
```

---

## 9. 借鉴源

| 子功能 | 借鉴 | 关键文件 |
|--------|------|---------|
| DM 通知系统 | Paperclip system_notice comment + Rudder chat system_event | `agent_notifier.go` |
| Watchdog 升级 | Paperclip recovery escalation + Solo task_watchdog | `watchdog.go` |
| 事件总线 | Multica events.Bus | `relationship_events.go` |
| 模板库 | Alook templates | `template.go` |
| RELATIONSHIPS.md | Solo 原创(动态 MD 文档模式) | `relationships_md.go` |
| Server 统一调度 | Paperclip / Rudder server-side setInterval | `scheduler.go` |

---

## 10. 风险与权衡

| 风险 | 缓解 |
|------|------|
| DM 通知依赖 owner-agent DM 存在 | `ensureAgentDM` 自动创建，lazy 初始化 |
| 删除 mention 校验后可能滥用 | 由 agent 自行判断，关系在 RELATIONSHIPS.md 中可见 |
| Scheduler 单点故障 | server 进程内定时器，server 挂了 agent 也不工作 |

---

## 11. 事项 2-5 待办(后续会话设计)

| 事项 | 范围 | 与事项 1 的关系 |
|------|------|----------------|
| 2: 任务编排 | DAG 方向、block UI、Swarm 分解、完成通知 | AgentNotifier 可复用 |
| 3: 共享知识 | CHANNEL.md、knowledge 表、跨频道搜索 | RELATIONSHIPS.md 是参照 |
| 4: 自动化 | Reminder、Watchdog、Escalation | scheduler 已实现，可扩展 |
| 5: 工作区 | 频道绑定、worktree、多 agent 冲突 | RELATIONSHIPS.md 路径可借鉴 |

# Solo 消息类型全景

> 最后更新: 2026-06-16
> 范围: 所有消息的产生场景、落库策略、传输通道、agent 感知路径

---

## 一、聊天消息（落库）

用户和 agent 通过输入框或 CLI 发出的消息。

| # | 谁 | 做了什么 | 出现在哪 | sender_type | 会唤醒 agent | 说明 |
|---|----|---------|---------|-------------|-------------|------|
| 1 | 用户 | 在频道输入框打字发送 | 频道主时间线 | `user` | ✅ 立即 | Web UI → WS → 落库 → `TriggerAgentResponse` |
| 2 | agent | `solo message send -c #xxx` | 频道主时间线 | `agent` | ✅ 立即 | daemon 代理 HTTP → 落库 → `TriggerAgentResponse` |
| 3 | 用户 | 在 thread 里回复 | thread 内 | `user` | ✅ 立即 | Web UI → WS → 落库 → `TriggerAgentResponseInThread` |
| 4 | agent | `solo message send --target '#xxx:msgid'` | thread 内 | `agent` | ✅ 立即 | daemon 代理 HTTP → 落库 → `TriggerAgentResponseInThread` |
| 5 | 用户/agent | 在 DM 聊天框发消息 | DM 主时间线 | `user`/`agent` | ✅ 立即 | REST API → 落库 → `TriggerAgentResponse` |
| 6 | 用户/agent | 创建 task（DM / 全局） | 频道/DM 主时间线 | `user`/`agent` | ❌ | task 标题作为消息落库，trigger 走 `TriggerAgentForTask` |
| 7 | agent | 任务执行完毕，输出结果 | 频道/thread | `agent` | ❌ | daemon 回调后 upsert，或 `persistAgentMessage` |

**同一种行为的两个入口**：用户在 Web UI 发频道消息走 WS（#1），agent 通过 `solo message send` 走 HTTP 代理（#2）。消息格式完全一样，区别只在 `sender_type`。

---

## 二、System 消息（落库 → `messages` 表）

由系统自动产生，`sender_type='system'`、`content_type='system'`、`sender_id='00000000-0000-0000-0000-000000000000'`。

### 2.1 Task 事件（频道）

| # | 场景 | 用户做了什么 | 出现在哪 | agent 可见 | 为什么 |
|---|------|------------|---------|-----------|--------|
| 10 | 创建 task | 在频道里新建 task | 频道主时间线 | ✅ | `thread_id=NULL`，且后续调用 `TriggerAgentForTask` |
| 11 | claim task | 点击 claim 按钮 | task thread 内 | ❌ | `thread_id` 有值，被 `getRecentMessages` 过滤 |
| 12 | unclaim task | 点击 release 按钮 | task thread 内 | ❌ | 同上 |
| 13 | 更新 task 状态 | 拖拽看板或直接改状态 | task thread 内 | ❌ | 同上 |
| 14 | 删除 task | 删除 task | task thread 内 | ❌ | 同上 |

**关键规律**：创建 task 的消息在频道主时间线，agent 可见。claim/unclaim/update/delete 的消息全在 task 的 thread 里，agent 不可见——这些是给人看的操作日志。

### 2.2 Task 事件（DM）

| # | 场景 | 用户做了什么 | 出现在哪 | agent 可见 | 为什么 |
|---|------|------------|---------|-----------|--------|
| 15 | claim task | 在 DM 里 claim | task thread 内 | ❌ | `thread_id` 有值 |
| 16 | unclaim task | 在 DM 里 release | task thread 内 | ❌ | 同上 |

### 2.3 Agent 通知（DM，server scheduler 统一调度）

由 `AgentNotifier` 发送，走 owner-agent DM 通道，`thread_id=NULL`，agent 立即可见。

| # | 场景 | 触发条件 | 调度方 | 完整格式 |
|---|------|---------|--------|---------|
| 21 | task 被 claim | B claim 了 A 创建的 task | `task.go:ClaimTask` handler | `📋 Task claimed — #N title in #channel was claimed by @name.\n\n→ Next: @name is working on this. Wait for completion, then check if the result is correct and close the parent task if satisfied.` |
| 22 | task 被完成 | B 完成了 A 创建的 task | `task.go` completion handler | `✅ Task completed — #N title in #channel was completed by @name.\n\n→ Next: Go to #channel to review the result. If OK, close the parent task to unblock the chain. If not, leave feedback for @name.` |
| 23 | task 超时升级 | watchdog 发现 task 超时 | `scheduler` → `ScanTimeouts` | `⚠️ Task overdue — #N title in #channel is overdue and has been escalated to you.\n\n→ Next:\n1. Delegate to another agent with similar skills\n2. Or check with @claimer in #channel for status\n3. If you cannot handle this, inform the user directly` |
| 24 | task 超时提醒 | task 超时，`timeout_action=remind`，通知 claimer | `scheduler` → `ScanTimeouts` → `AgentNotifier.NotifyRemind` | `⏰ Task overdue — #N title in #channel is past deadline (time).\n\n→ Next: Check progress and update the task status, or extend the deadline.` |
| 25 | 定时提醒到期 | `reminders` 表到期 | `scheduler` → `CheckDueReminders` | `⏰ Reminder: <message>` |

所有通知都是 **落库 → WS 推 DM → `TriggerAgentResponse` 唤醒**，一条链路。

### 2.4 其他

| # | 场景 | 时机 | 出现在哪 | agent 可见 |
|---|------|------|---------|-----------|
| 19 | 新用户注册 | 注册完成时 | 欢迎频道主时间线 | ✅ |
| 20 | `TriggerAgentForTask` fallback | task 没有关联消息时 | 频道主时间线 | ✅ |

---

## 三、System 消息（不落库 → 直接注入 agent context）

这些消息不存 `messages` 表，由 server 构造后直接塞进 daemon 的 `Messages` 数组。

| # | 场景 | 触发时机 | agent 看到的 role | 触发方式 | 完整格式 |
|---|------|---------|------------------|---------|---------|
| 26 | 被 @mention 拉入 thread | 有人在 thread 里 @mention agent | `role: system`（唯一一处） | `TriggerAgentResponseInThread` → prepend 到 context 最前面 | `[target=#channel:shortid msg=shortid time=iso type=system]\nYou were @mentioned in a new thread. Reply here using the target above.\nRead thread history: solo message read --target '#channel:shortid'` |
| 27 | Lucy onboarding 问候 | 管理员创建 Lucy agent | `role: user` | `TriggerAgentGreeting` → 直接构造 context，不落库 | `[target=#channel msg=shortid time=iso type=system] @Lucy:\nYou have just been created as the onboarding lead...` |
| 28 | agent 加入频道 | 管理员把 agent 加进频道 | `role: user` | `TriggerAgentGreeting("")` → 默认模板，不落库 | `[target=#channel msg=shortid time=iso type=system] @agentname:\nYou have just joined the channel #xxx. Please introduce yourself...` |

---

## 四、WebSocket 事件（不落库，纯实时推送）

这些事件只推送到已连接的 WebSocket 客户端（浏览器/daemon），不存 `messages` 表，不进 agent JSONL。

### 4.1 前端 UI 实时更新

| # | 事件 | 推送范围 | 作用 |
|---|------|---------|------|
| 31-38 | `agent.thinking/typing/status/error/chunk/done/activity/agent_typing` | 频道/DM 订阅者 | agent 执行过程的实时状态展示 |
| 39-40 | `member.joined/left` | 频道订阅者 | 成员列表实时更新 |
| 41-43 | `task.created/updated/deleted` | 频道订阅者 | 任务看板实时刷新 |
| 44-45 | `task.blocked/unblocked` | 频道订阅者 | 任务依赖关系变更 |
| 46 | `relationship_created/deleted/updated` | 频道订阅者 | agent 关系图更新 |
| 47 | `memory_updated` | 频道订阅者 | 频道记忆变更 |
| 48-49 | `dm.message.new/updated` | DM 订阅者 | DM 列表实时更新 |
| 50 | `inbox.updated` | 用户连接 | 收件箱未读红点 |

### 4.2 Toast / 通知事件

| # | 事件 | 推送范围 | 触发条件 |
|---|------|---------|---------|
| 51 | `reminder_fired` | 用户连接 | 定时提醒到期（WS 侧 toast） |
| 52 | `task_escalated` | 用户连接 | watchdog 升级（WS 侧 toast） |
| 53 | `task_unclaimed_auto` | 用户连接 | watchdog 自动释放 |
| 54 | `swarm_decomposed` | 频道订阅者 | 任务拆解为 swarm |
| 55 | `swarm_all_done` | 频道订阅者 | swarm 全部完成 |

---

## 五、Agent 唤醒路径

agent 被触发执行任务的 4 条路径：

```
A. TriggerAgentResponse(channelID, messageID)
   触发方: 频道/DM 新消息、AgentNotifier DM 通知（#21-25）
   → getRecentMessages(channelID, 1)
     SQL: WHERE thread_id IS NULL ORDER BY created_at DESC LIMIT 1
   → 格式: [target=#xxx msg=shortid time=iso type=xxx] @sender: content
   → JSONL role: user (非 agent 发送者) / assistant (agent 发送者)

B. TriggerAgentResponseInThread(threadID)
   触发方: thread 新回复
   → GetThreadContextMessages(threadID)
     SQL: WHERE thread_id=$1 ORDER BY created_at ASC (全部消息)
   → 如有 @mention: 在消息数组头部插入一条 role:system 系统头
   → JSONL role: user / assistant / system(仅 @mention 头)

C. TriggerAgentForTask(taskID)
   触发方: task 创建、daemon task callback
   → 构造单条消息，附带 task 上下文: [task #N status=xxx channel=xxx]
   → JSONL role: user

D. TriggerAgentGreeting(channelID, agentID, greeting)
   触发方: agent 加入频道、Lucy onboarding 创建
   → 直接构造单条 context message，不落 messages 表
   → JSONL role: user
```

---

## 六、调度架构

所有定时任务统一由 server 侧 `Scheduler`（`scheduler.go`）管理，30s 一次：

```
scheduler.Start()
  ├── tickReminders()
  │     → CheckDueReminders()
  │     → AgentNotifier.Notify(#25)
  │     → MarkFired / RescheduleRecurring
  │
  └── tickWatchdog()
        → ScanTimeouts()
          ├── remind → AgentNotifier.Notify(#24)
          ├── escalate → AgentNotifier.NotifyEscalation(#23)
          └── unclaim → 直接 UPDATE tasks
```

daemon 不再运行任何定时任务，仅负责 agent 执行调度。

---

## 七、关键规律

1. **Agent JSONL 可见 = `thread_id IS NULL`**。`getRecentMessages` 的 SQL 只查主时间线，thread 消息走独立路径
2. **落库 ≠ agent 可见**。落库了但没有 `TriggerAgentResponse` 的消息，agent 只在下次被触发时才能看到
3. **System 消息在 JSONL 里是 `role: user`**，`type=system` 体现在 `[target=...]` header 中，不是真正的 system role
4. **真正 `role: system` 仅一处**：thread @mention 系统头，直接注入 context，不落库
5. **DM 和 channel 走完全相同的 agent 触发路径**，`TriggerAgentResponse` 和 `getRecentMessages` 对两者通用
6. **同一个"发消息"动作有两种入口**：用户通过 Web UI → WS，agent 通过 solo CLI → HTTP 代理。消息落库后格式相同
7. **所有定时扫描统一在 server 侧**，daemon 不再运行定时器。paperclip / rudder 同款架构

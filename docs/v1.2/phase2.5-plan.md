# Solo v1.2 Phase 2.5 -- 迭代计划

> 版本: 1.0
> 创建日期: 2026-05-17
> 负责人: tpm agent
> 对齐: docs/v1.2/task-system-spec.md, docs/v1.2/TASKS-v1.2.md, docs/STATUS.md

---

## 1. Phase 2.5 概述

**插入时机**: Phase 2 (Agent 任务协议, task-system-spec Phase 2) 之后

**目标**: 修复 Phase 1/2 遗留的 5 个体验缺陷，补齐 channel 消息的 rich metadata 展示，强化 Agent 任务协议的可靠性。

**时间**: 1 周 (5 个工作日, 2026-05-19 ~ 2026-05-23)

**团队**: rd1 (后端全栈 1 人) + fe1 (前端 1 人) + qa1 (测试 0.3 人)

---

## 2. 需求与当前状态 Gap

| # | 需求 | 当前状态 | Gap |
|---|------|---------|-----|
| R1 | Thread 回复计数展示 | `threads.reply_count` 列已存在，`CreateThreadReply` 已自增；但消息列表 REST API 的 SELECT 不含 `reply_count`，`MessageResponse` 无此字段。前端仅从 WS 事件收到 reply_count，页面刷新后丢失 | API 缺字段 |
| R2 | 新增回复 "new" 标记/红点 | 无 thread 已读/未读追踪机制，无 `thread_reads` 表 | 全新功能 |
| R3 | Task 消息在 channel 显示状态+处理者 | `MessageResponse` 仅有 `task_number`，无 `task_status`/`claimer_name`；前端 `Message` 类型无 task 字段 | API + 前端 缺字段 |
| R4 | Agent 回复时 @用户 | System prompt 仅在 `# Communication` 提及 "可用 @username"，`TriggerThread` 分支无 @mention 指令 | Prompt 弱 |
| R5 | Agent 认领 @优先级 | `TriggerAllAgentsForTask` 对所有 active agent 并发触发，先到先得。无 @mention 优先窗口。与 task-system-spec §1.4 决策 3 偏离 | 逻辑缺失 |
| R6 | Agent 对 task 回复始终在 thread 中 | `TriggerAgentForTask` 已设置 ThreadID 路由到 thread，但 `TriggerAgentResponse`（channel 级触发）不走 task 感知路径。system prompt 虽有指引但 agent 可能违反 | 路由不完整 |

---

## 3. 任务分解 (WBS)

### 3.1 任务总览

| ID | 任务标题 | 负责 | 优先级 | 预估(h) | 依赖 |
|----|---------|------|--------|---------|------|
| P25-01-B | 消息列表 API 补充 reply_count | rd1 | P0 | 2 | 无 |
| P25-02-B | thread_reads 表 + 已读/未读 API | rd1 | P1 | 4 | P25-01-B |
| P25-03-B | 消息列表 API 补充 task 元数据 | rd1 | P0 | 3 | 无 |
| P25-04-B | Task 认领 @优先级窗口 (30s) | rd1 | P0 | 8 | 无 |
| P25-05-B | Agent 回复路由强制进 thread | rd1 | P0 | 4 | 无 |
| P25-06-B | System prompt: @回复用户 + task thread 一致性 | rd1 | P0 | 2 | 无 |
| P25-07-F | 消息卡片 reply_count 展示 | fe1 | P0 | 1 | P25-01-B |
| P25-08-F | Thread "new" 标记/红点 | fe1 | P1 | 4 | P25-02-B |
| P25-09-F | Task 消息状态标签 + 处理者展示 | fe1 | P0 | 3 | P25-03-B |
| P25-10-F | ThreadPanel reply_count 联动 | fe1 | P0 | 1 | P25-01-B |
| P25-11-QA | Phase 2.5 集成验证 | qa1 | P0 | 4 | 全部任务 |

**总计**: 11 个任务, 36h (rd1: 23h, fe1: 9h, qa1: 4h)

### 3.2 详细任务说明

---

#### P25-01-B: 消息列表 API 补充 reply_count (P0, 2h)

**描述**: 在消息列表查询的 SELECT 中 LEFT JOIN `threads` 表，通过 `threads.root_message_id = m.id` 关联，追加 `COALESCE(t.reply_count, 0) AS reply_count`。在 `MessageResponse` 中添加 `ReplyCount` 字段。WS `message.new` 事件已有的 `reply_count` 保持不变。

**不改**: thread 创建/追加逻辑（已正确）、前端 hooks（已处理 reply_count）
**不涉及**: 性能优化（索引已存在）

**验收标准**:
- `GET /api/v1/channels/{id}/messages` 返回的每个 message 包含 `reply_count` 字段
- 有 thread 的消息 reply_count >= 0，无 thread 的消息 reply_count = 0
- 与 WS 推送的 reply_count 值一致
- 现有测试通过

**改动文件**: `internal/server/handler/message.go`, `frontend/lib/hooks/use-messages.ts`（确认消费路径）

---

#### P25-02-B: thread_reads 表 + 已读/未读 API (P1, 4h)

**描述**: 新建 `thread_reads` 表 (user_id, thread_id, last_read_at, UNIQUE(user_id, thread_id))。提供两个 API:
- `POST /api/v1/threads/{threadID}/mark-read` — 更新 last_read_at = now()
- 在消息列表查询中 LEFT JOIN `thread_reads`，对比 `threads.last_reply_at > thread_reads.last_read_at OR thread_reads.last_read_at IS NULL` 判断 `has_unread`

**不改**: 不影响现有消息查询路径
**关联**: 前端打开 ThreadPanel 时自动调用 mark-read

**验收标准**:
- migration up 后表创建成功
- mark-read API 正确 upsert
- 消息列表返回的 message 包含 `has_unread_thread: true/false`
- 无 read 记录时 has_unread_thread = true（有新回复时）

**改动文件**: `migrations/`, `internal/server/handler/thread.go`, `internal/server/handler/message.go`

---

#### P25-03-B: 消息列表 API 补充 task 元数据 (P0, 3h)

**描述**: 在消息列表查询中 LEFT JOIN `tasks` 表 (`tasks.message_id = m.id`)，追加 `tasks.status AS task_status`, `tasks.claimer_name AS task_claimer_name`。在 `MessageResponse` 中添加 `TaskStatus` 和 `TaskClaimerName` 字段（omitempty）。

**不改**: 任务 CRUD 逻辑、`TaskResponse` 结构
**性能**: tasks 表有 `message_id` 索引，JOIN 开销可忽略

**验收标准**:
- task 消息包含 `task_status` (todo/in_progress/in_review/done) 和 `task_claimer_name`
- 非 task 消息这两个字段为 omitempty（不返回）
- 与 task 实际状态一致

**改动文件**: `internal/server/handler/message.go`

---

#### P25-04-B: Task 认领 @优先级窗口 30s (P0, 8h)

**描述**: 实现 task-system-spec §1.4 决策 3：创建任务时如果 content/description 包含 @Agent，该 Agent 获得 30 秒独占认领窗口。具体逻辑:

1. `CreateTask` 时检测 `mentioned_agent_ids`
2. 如果任务有 @Agent 且非空：
   - 只向 @mentioned 的 Agent 发送触发（不触发其他 Agent）
   - 设置 30 秒定时器
3. 30 秒后如果未被认领：
   - 触发所有其他 active Agent（放开认领）
4. 如果任务无 @Agent：
   - 保持现有"全部触发，先到先得"行为
5. Claim API 不变（已有并发控制）

**服务端强制**: 30s 窗口内非 @Agent 的 claim 请求返回 409 + "优先认领窗口未结束"

**不改**: `TriggerAllAgentsForTask` 签名（保持兼容）
**风险**: 30s 定时器需要 goroutine 管理，注意 goroutine 泄漏

**验收标准**:
- @AgentA 的任务 → 只有 AgentA 在前 30s 可认领
- 30s 后其他 Agent 可认领
- 未 @ 的任务保持先到先得
- Claim API 的优先窗口检查正确
- 单元测试覆盖: @认领成功 / 窗口内被他人认领被拒 / 窗口过期后认领成功

**改动文件**: `internal/server/service/agent.go`, `internal/server/handler/task.go`, `internal/server/handler/task_test.go`

---

#### P25-05-B: Agent 回复路由强制进 thread (P0, 4h)

**描述**: 当 Agent 因 task 相关事件被触发时，其回复必须进入 task 的 thread，绝不出现在 channel 主时间线。需要在两处加固：

1. `TriggerAgentForTask` — 已有 thread_id 路由。补充：如果 thread_id 获取失败，不降级到 channel 回复，而是返回 error
2. `TriggerAgentResponse` — channel 级自动响应。需要判断：如果触发消息是 task 消息（有对应的 task 记录），则强制路由到 task thread

**验证点**: `daemonTaskRequest.ThreadID` 在 task 触发场景下必须非空

**不改**: Daemon 的 SSE 转发和 WS 广播（它们已经正确处理 ThreadID）

**验收标准**:
- Agent 对 task 的回复出现在 task thread 中，不出现在 channel 主消息流
- 无 thread_id 时不发送回复（不降级）
- channel 级触发也感知 task 上下文
- 现有 thread 回复测试通过

**改动文件**: `internal/server/service/agent.go`

---

#### P25-06-B: System prompt 强化 (P0, 2h)

**描述**: 在 `prompt.go` 中做三处修改:

1. **`# Communication`**: 在 "@mentions" 说明后追加："When replying to a user's message in a thread, always @mention that user so they are notified."
2. **`TriggerMention`**: 追加 "Start your reply with @username to address the person who mentioned you."
3. **`TriggerThread`**: 追加 "Always start your reply with @username to address the person you are replying to."
4. **`# Message Routing / Task Thread Protocol`**: 强化 "Your task-related replies MUST go to the task's thread, NEVER to the main channel. The platform will route your reply correctly if you respond in the thread context."

**不改**: prompt 结构、其他 section

**验收标准**:
- Agent 在 thread 中回复时默认 @用户
- Agent 不会在 channel 中回复 task 的工作进度
- `go test ./pkg/agent/ -run TestBuildSystemPrompt` 通过

**改动文件**: `pkg/agent/prompt.go`

---

#### P25-07-F: 消息卡片 reply_count 展示 (P0, 1h)

**描述**: `MessageList` 组件已有 reply_count 展示逻辑（`message.reply_count ?? 0 > 0`），确认其消费 REST API 返回的 `reply_count` 字段。补充状态处理:
- reply_count = 0 时不显示
- reply_count > 0 显示 "{N} 条回复" 链接（已有）
- 新增回复时通过 WS 实时更新计数

**不改**: ThreadPanel 组件、reply 操作

**验收标准**:
- 有 thread 的消息显示正确计数
- WS 收到新 thread 回复后计数实时 +1
- 无 thread 的消息不显示计数

**改动文件**: `frontend/components/dashboard/message-list.tsx`, `frontend/lib/hooks/use-messages.ts`

---

#### P25-08-F: Thread "new" 标记/红点 (P1, 4h)

**描述**: 在消息卡片上显示 thread 未读标记:
1. 红点: 父消息左侧显示 4px 粉色实心圆（或右上角 badge）
2. 触发条件: `has_unread_thread = true`（来自 P25-02-B）且用户未打开过 ThreadPanel
3. 消除条件: 用户打开 ThreadPanel → 自动调用 `POST /threads/{id}/mark-read` → 红点消失
4. 动画: 红点出现时微动画（fade-in）

**不改**: ThreadPanel 核心逻辑
**设计**: 粉色圆点 (#fe7da8)，4px，在消息左侧 margin 处

**验收标准**:
- 未读 thread 的消息显示红点
- 打开 ThreadPanel 后红点消失
- WS 新 thread 回复到达时红点再现
- 红点不出现闪烁/重复

**改动文件**: `frontend/components/dashboard/message-list.tsx`, `frontend/components/dashboard/thread-panel.tsx`, `frontend/lib/hooks/use-messages.ts`

---

#### P25-09-F: Task 消息状态标签 + 处理者展示 (P0, 3h)

**描述**: 当消息是 task 消息（有 `task_status` 字段）时，在消息卡片中显示:
1. 状态徽章: 4 色标签
   - `todo`: 灰色 bg + "TODO" 文字
   - `in_progress`: 蓝色 bg + "处理中" 文字
   - `in_review`: 黄色 bg + "待审核" 文字
   - `done`: 绿色 bg + "已完成" 文字
2. 处理者行: 如果 `task_claimer_name` 非空，显示 "@claimer_name" + 小图标
3. 标签位于消息内容上方、sender 名称下方

**设计**: Neubrutalist 风格 — 2px 边框 + 5px 阴影 + 粗字体

**不改**: TaskBoard、TaskDetailPanel、ThreadPanel

**验收标准**:
- task 消息显示对应状态标签
- 非 task 消息无标签
- 状态更新后标签实时变化（WS 事件）
- 所有 4 个状态视觉差异明显

**改动文件**: `frontend/components/dashboard/message-list.tsx`, `frontend/lib/types.ts`

---

#### P25-10-F: ThreadPanel reply_count 联动 (P0, 1h)

**描述**: ThreadPanel 头部显示 "N 条回复" 计数。当前已有 Thread 数据传入，确认 ThreadPanel 消费 `thread.reply_count` 并在头部展示。实时更新通过 `useThread` hook 的 WS 订阅。

**不改**: ThreadPanel 核心布局、消息输入

**验收标准**:
- ThreadPanel 打开时头部显示正确计数
- 发送新回复后计数 +1
- 关闭后 message 卡片计数同步更新

**改动文件**: `frontend/components/dashboard/thread-panel.tsx`

---

#### P25-11-QA: Phase 2.5 集成验证 (P0, 4h)

**描述**: 端到端验证全部 5 个需求:

| 场景 | 验证点 |
|------|--------|
| **S1: 回复计数** | 消息卡片显示 reply_count；刷新页面后计数保持；新回复后计数 +1 |
| **S2: New 标记** | 未读 thread 显示红点；打开 ThreadPanel 后消失；新回复到达再现 |
| **S3: Task 状态** | channel 消息显示状态标签和认领人；状态变更后标签实时更新 |
| **S4: Agent @回复** | Agent 在 thread 中回复时包含 @用户名 |
| **S5: 认领优先级** | @Agent 的任务 30s 内仅该 Agent 可认领；30s 后开放 |
| **S6: Thread 一致性** | Agent task 回复始终在 thread 中；不出现在 channel 主时间线 |

**验收标准**:
- 6 个场景全部通过
- 无 blocking/critical bug
- 回归测试: 现有 143 测试全部通过

---

## 4. 优先级排序

### P0 — 必须交付 (阻塞后续)

| ID | 任务 | 负责 | 工时 | 理由 |
|----|------|------|------|------|
| P25-01-B | reply_count API | rd1 | 2h | R1 基础依赖 |
| P25-03-B | task 元数据 API | rd1 | 3h | R3 基础依赖 |
| P25-04-B | @认领优先级窗口 | rd1 | 8h | R5 核心逻辑，最大单项 |
| P25-05-B | Agent 回复强制进 thread | rd1 | 4h | R6 路由安全 |
| P25-06-B | System prompt 强化 | rd1 | 2h | R4 + R6 的 prompt 层保障 |
| P25-07-F | reply_count 展示 | fe1 | 1h | R1 前端，依赖 P25-01-B |
| P25-09-F | task 状态标签 | fe1 | 3h | R3 前端，依赖 P25-03-B |
| P25-10-F | ThreadPanel 计数联动 | fe1 | 1h | R1 补充 |

**P0 合计**: rd1 19h + fe1 5h = 24h

### P1 — 目标交付

| ID | 任务 | 负责 | 工时 | 理由 |
|----|------|------|------|------|
| P25-02-B | thread_reads API | rd1 | 4h | R2 后端 |
| P25-08-F | "new" 标记/红点 | fe1 | 4h | R2 前端，依赖 P25-02-B |

**P1 合计**: rd1 4h + fe1 4h = 8h

---

## 5. 迭代计划 (每日)

```
Week 1 (5 天): 2026-05-19 ~ 2026-05-23

Day 1 (Mon)   Day 2 (Tue)   Day 3 (Wed)   Day 4 (Thu)   Day 5 (Fri)
├────────────┼────────────┼────────────┼────────────┼────────────┤
rd1:
P25-01-B(2h) P25-04-B(8h) P25-04-B    P25-03-B(3h) P25-02-B(4h) 
P25-06-B(2h) continued... continued... P25-05-B(4h) P25-02-B    
P25-03-B(3h)                                      P25-05-B     
                                                  buffer/QA fix
fe1:
P25-07-F(1h) P25-09-F(3h) P25-09-F    P25-10-F(1h) P25-08-F    
(P25-01 等  (P25-03 等  continued... P25-08-F(4h) continued...
 后端先做)    后端先做)                          
                                                  buffer/QA fix
qa1:                                         P25-11-QA(4h)
                                             全场景验证
```

**Dependency chain**:
```
P25-01-B (reply_count API) ──→ P25-07-F (reply_count 展示)
                           ──→ P25-10-F (ThreadPanel 计数)

P25-03-B (task 元数据 API) ──→ P25-09-F (task 状态标签)

P25-02-B (thread_reads API) ──→ P25-08-F (new 标记)

P25-04-B (认领优先级) —— 独立，无前端依赖
P25-05-B (回复进 thread) —— 独立
P25-06-B (prompt 强化) —— 独立
```

**Day 1-2 策略**:
- rd1 先做 P25-01-B (2h) + P25-06-B (2h) + P25-03-B (3h) = 7h，Day 1 产出 3 个基础 API，解除 fe1 阻塞
- rd1 Day 2 启动 P25-04-B (最大任务 8h)，占用 Day 2-3
- fe1 Day 1 等待 rd1 完成 P25-01-B 和 P25-03-B 后开始 P25-07-F + P25-09-F

**Day 3-4 策略**:
- rd1 完成 P25-04-B，启动 P25-03-B + P25-05-B
- fe1 推进 P25-09-F 和 P25-10-F

**Day 5 策略**:
- rd1 P25-02-B + 根据 QA 反馈修 bug
- fe1 P25-08-F + 根据 QA 反馈修 bug
- qa1 全天 E2E 验证

---

## 6. 容量检查

| 角色 | 总可用(h) | P0 排期(h) | P1 排期(h) | 总计(h) | 利用率 | 状态 |
|------|----------|-----------|-----------|---------|--------|------|
| rd1 | 40 | 19 | 4 | 23 | 58% | 健康 |
| fe1 | 40 | 5 | 4 | 9 | 23% | 低负荷 |
| qa1 | 16 | 4 | 0 | 4 | 25% | 按需 |

**结论**: rd1 58% 利用率合理（P25-04-B 的 8h 可能有溢出），fe1 23% 偏低但 P1 任务可提前启动。

---

## 7. 风险登记册

| # | 风险描述 | 概率 | 影响 | 触发条件 | 缓解措施 | 应急预案 | 负责人 |
|---|---------|------|------|----------|----------|----------|--------|
| R1 | P25-04-B @认领优先级窗口实现比预估复杂，30s 定时器管理、并发安全、窗口状态存储需要额外设计 | 中 | 高 | Day 2 结束时未完成核心逻辑 | Day 1 提前写好详细设计，明确窗口状态存储方案（内存 map + goroutine 定时器 vs DB 字段 + 定时轮询） | 降级方案：仅 @mention Agent 获得首次触发，不实现 30s 排他窗口，改为 system prompt 强化 @优先级 | rd1 |
| R2 | thread_reads 表写入频率高（每次打开 ThreadPanel → 1 次 UPDATE），频道规模大时可能产生写入热点 | 低 | 中 | QA 负载测试发现延迟 | 使用 UNIQUE 约束 + ON CONFLICT UPDATE 优化；可后续加 Redis 缓存层 | 降级为客户端 localStorage 方案（无服务端持久化），简化实现 | rd1 |
| R3 | Agent 回复路由强制进 thread 可能影响现有 DM 场景（DM 也走 TriggerAgentResponse） | 低 | 高 | QA 发现 DM Agent 回复消失 | 在路由判断中显式区分 DM/Channel 场景；DM 不强制 thread 路由 | 添加 DM 判断分支，DM 保持现有行为 | rd1 |
| R4 | P25-06-B system prompt 修改可能改变 Agent 行为模式，需要观察 | 中 | 低 | Agent 开始过度 @mention 或行为反常 | prompt 改动最小化，只修改 3 个触发分支的尾部追加行 | 回滚 prompt 变更，保留原版行为 | rd1 |

---

## 8. 关键设计决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| 1 | reply_count 来源 | REST API 查询时 JOIN 获取，非 WS-only | 页面刷新后数据不丢失；REST 是权威数据源 |
| 2 | 已读/未读存储 | 服务端 `thread_reads` 表 | 跨设备/会话持久化；未来收件箱功能的基础设施 |
| 3 | 认领窗口实现 | 内存 map + goroutine 定时器 (30s) | 简单直接，无需额外 DB 迁移；30s 短周期内存管理负担小 |
| 4 | Task 元数据在消息中的位置 | 消息响应增加专属字段 `task_status`/`task_claimer_name` | 不污染现有消息字段；omitempty 对非 task 消息透明 |
| 5 | Agent 回复路由 | task 场景 ThreadID 必须非空，无 thread 不发送 | 安全优先；不降级到 channel 避免信息泄露/混乱 |
| 6 | System prompt 修改粒度 | 仅追加，不重写 | 最小变更原则；避免破坏已稳定运行的 prompt 结构 |

---

## 9. 附录 A: 与 task-system-spec 的对齐

Phase 2.5 覆盖了 task-system-spec §1.4 中尚未完成的决策:

| 决策 | Spec | Phase 1 | Phase 2 | Phase 2.5 |
|------|------|---------|---------|-----------|
| 1 (asTask) | P0 | ✅ | — | — |
| 2 (直接创建) | P0 | ✅ | — | — |
| 3 (@软指派+认领) | P0 | ⚠️ 部分 | ❌ 未实现 | ✅ **P25-04-B** |
| 4 (View in Channel) | P1 | — | — | — (v1.3) |
| 5 (Reply 复用 Thread) | P0 | ✅ | — | — |
| 6 (Agent 线程内回应) | P0 | ⚠️ 部分 | ❌ 未实现 | ✅ **P25-05-B + P25-06-B** |

---

## 10. 附录 B: 改动文件清单

### Backend
| 文件 | 任务 | 改动类型 |
|------|------|---------|
| `internal/server/handler/message.go` | P25-01-B, P25-03-B | SELECT 扩展 + Response 字段 |
| `internal/server/handler/thread.go` | P25-02-B | 新增 mark-read API |
| `internal/server/service/agent.go` | P25-04-B, P25-05-B | @优先级窗口 + thread 强制路由 |
| `internal/server/handler/task.go` | P25-04-B | Claim 优先窗口检查 |
| `pkg/agent/prompt.go` | P25-06-B | System prompt 追加 |
| `migrations/` | P25-02-B | thread_reads 表 |

### Frontend
| 文件 | 任务 | 改动类型 |
|------|------|---------|
| `frontend/components/dashboard/message-list.tsx` | P25-07-F, P25-08-F, P25-09-F | reply_count 展示 + new 标记 + task 标签 |
| `frontend/components/dashboard/thread-panel.tsx` | P25-08-F, P25-10-F | mark-read 调用 + reply_count 头部 |
| `frontend/lib/types.ts` | P25-09-F | Message 类型 task 字段 |
| `frontend/lib/hooks/use-messages.ts` | P25-07-F, P25-08-F | reply_count 消费 + has_unread 处理 |

### Tests
| 文件 | 任务 | 改动类型 |
|------|------|---------|
| `internal/server/handler/task_test.go` | P25-04-B | @认领窗口测试 |
| `internal/server/handler/message_test.go` | P25-01-B, P25-03-B | 消息字段测试 |
| `pkg/agent/prompt_test.go` | P25-06-B | Prompt 断言更新 |
| `frontend/e2e/` | P25-11-QA | Phase 2.5 E2E spec |

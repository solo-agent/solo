# Agent 上下文压缩观测与 Session 自动换代

Status: implementation design

Scope: 普通 Channel 与其 Thread 共享的持久 Agent Session。Thinking Node 继续使用现有独立 Session 和 checkpoint/handoff 生命周期；Pi 当前是 one-shot，不参与自动换代。

## 1. 结论

Solo 不再把“长期同事”理解成“永远不换 provider Session”，而是把长期连续性放在 Agent 身份、Task、消息、Run、项目文件和 Agent memory 上。provider Session 仍优先长期复用，并先使用 Runtime 自带的 compaction；只有 Runtime 已经压不动，或压完仍接近窗口上限时，Solo 才在当前 turn 完整结束后换一个新 Session。

初版采用以下边界：

- Claude 和 Codex 接入明确的 compaction 信号。
- OpenCode、OpenClaw、Hermes 通过 ACP 的上下文占用 `used/size` 使用统一压力判断。
- 不把现有 `TokenUsage` 当成上下文占用；它是 Run 的计费消耗。
- 不新增 Epoch、Checkpoint 表或新的持久化表；只增加 `rollover_pending` Session 状态和 `agent_runs.rollover_from_session_id` 一个 nullable 外键，保证新旧 Session 交接可恢复。
- 不在 Agent 输出、工具调用或 Task 提交中途关闭 Session。
- 不物理删除历史消息、Run 或 transcript。所谓裁剪，只是新 Session 不再注入已经结束或不属于当前 Agent 的工作历史。

## 2. 现状与不变量

Solo 当前的普通持久会话作用域是 `Agent × Channel`，不是 Agent 全局唯一 Session。同一 Channel 的主频道和各个 Thread 共享该 Session；Thinking Node 使用独立 Session key。

```text
Agent × Channel ──> provider Session ──> Run 1, Run 2, Run 3 ...
Thinking Node   ──> isolated Session
```

Server 会为新 Run 查找该 Agent 在该 Channel 最近的 `active` Session，再把 provider session ID 发给 Daemon 恢复。Daemon 已有关闭 scoped session 和 `ForceFreshSession` 的能力。数据库已有：

- `agent_sessions`：provider Session，已有 `active/closed` 实际语义；
- `agent_runs.session_id`：本次 Run 使用的 Session；
- `agent_run_task_links`：Run 与 Task 的可靠关联；
- `agent_run_events(type, payload)`：可扩展的可观测事件。

Task 和 Run 必须分开判断：Run `completed` 只表示一次执行结束，不表示 Task 完成。Task 的状态才是工作是否仍需保留的依据：

```text
todo -> in_progress -> in_review -> done
  \-----------> closed
```

`done/closed` 是终态；`todo/in_progress/in_review` 都不是。

## 3. Runtime 能提供什么

| Runtime | 可用信号 | 初版处理 |
| --- | --- | --- |
| Claude Code | `system/compact_boundary.compactMetadata`，wire 字段为 `trigger`、`preTokens`，当前版本可能同时提供 `postTokens` | 直接记录 compaction；`postTokens` 缺失时不猜压缩率 |
| Codex | `item/started`、`item/completed` 的 `contextCompaction`；另有 `thread/tokenUsage/updated` | 用 compaction 边界前后的 `last.totalTokens` snapshot 配对；不用累计 `total` |
| ACP / OpenCode / OpenClaw / Hermes | `usage_update {used,size}` | 只判断上下文压力，不声称观察到了 compaction |
| Pi | `compaction_start/end`，after 为估算值 | 先不接自动换代；Solo 当前每次 Execute 都创建新 Pi Session |

Claude 的 `postTokens`、Codex 的边界后 snapshot 都必须允许缺失。缺值的结果是 `unknown`，不是 `0`，也不触发基于猜测的换代。

参考：[Claude Agent SDK 消息类型](https://code.claude.com/docs/en/agent-sdk/typescript)、[Codex App Server compaction](https://developers.openai.com/codex/app-server/#trigger-thread-compaction)、[Pi compaction](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/compaction.md)、[ACP usage/compaction 讨论](https://github.com/orgs/agentclientprotocol/discussions/871)。

## 4. 最小领域模型与所有权

Runtime adapter 只负责把各家协议转成同一个事实事件，不负责决定是否换 Session：

```go
type ContextEvent struct {
    Type         string // usage | compaction_start | compaction_end
    UsedTokens   *int64
    WindowTokens *int64
    BeforeTokens *int64
    AfterTokens  *int64
    Accuracy     string // reported | snapshot | estimated
    Reason       string // auto | manual | threshold | overflow
}
```

`OutputChunk` 增加可选的 `Context *ContextEvent`，并新增 `MessageContext`。字段使用指针，明确区分真实的 `0` 与“不知道”。provider、Run ID 和 Session ID 已由外围执行上下文持有，不复制进事件。

`agent_runs.rollover_from_session_id` 只表示“这个 Run 奉命替换哪个旧 Session”。它不是 Epoch：不参与消息排序，不改变 Agent/Channel 作用域，也不会随每次 compact 增长。

每个普通 scoped Session 只在 Daemon 内存中保留两个计数：

- 连续低效 compaction 次数；
- 连续高压力 turn 次数。

这些计数不是业务事实，不需要新表。Daemon 重启后归零，最坏结果是多给 Runtime 一次自我恢复机会；已经提交的换代请求则通过旧 `agent_sessions.status='rollover_pending'` 持久化。

所有权如下：

- Runtime：决定何时原生 compact，并报告它知道的数字；
- adapter：解析、配对并标注测量准确性；
- Daemon Session Manager：维护短期计数，只在 turn 结束时提出换代建议；收到 Server 后续明确的 fresh 指令才关闭本地 scoped Session；
- Server：拥有 Task、Run 和 Session 的持久化事实，提交换代请求，生成 Task-aware Continuity Packet，并在新 Session 建立后关闭旧记录；
- frontend：只展示结果，不参与判断。

## 5. 判断规则

只使用 Runtime 报告的上下文占用，绝不使用 `Result.Usage`、`agent_runs.usage_json` 或累计账单 token 估算上下文。

```text
saved_ratio   = 1 - after_tokens / before_tokens
pressure_ratio = used_tokens / window_tokens
```

初版使用一组固定的保守规则，不增加产品配置项：

- `saved_ratio < 10%`：一次低效 compaction；连续两次后换代；
- `pressure_ratio >= 90%`：一次高压力 turn。`reported/snapshot` 连续两个已完成 turn 仍高压后换代；ACP 默认按 `estimated` 处理，需要连续三个高压力 turn。

另外两条直接规则：

- 同一次有效观测同时具有 `AfterTokens` 和 `WindowTokens`，且 compaction 后仍 `>= 90%`，本 turn 结束后直接提交换代请求；
- Runtime 明确返回 context length/window overflow，旧 Session 直接标记不可再恢复。若 Run 关联仍处于 `todo/in_progress` 的 Task，复用现有最多三次 Task recovery，并选择 `fresh_session`。

一次达到有效节省阈值的 compaction 会清零“低效 compaction”计数；一次低于 90% 的可用 snapshot 会清零“高压力 turn”计数。缺失、非法或无法配对的观测会终止对应的连续计数，不能让“低效 -> 多次 unknown -> 低效”被算作连续两次。没有 before/after 或 used/window 的 Runtime 保持现状，不自动换代。

参与除法的 token 和 window 必须大于零。Claude 等原子 `reported` 观测若 `after > before`，按低效处理；Codex 的 `snapshot` 若 `after > before`，更可能是错配或压缩后继续增长，压缩率记为 unknown，只允许其独立的最终 occupancy 参与压力判断。

Codex 配对必须限定同一 `threadId`、同一 `turnId` 和通知顺序：compaction start 取此前最近 snapshot，compaction completed 后取第一条新 snapshot；下一次 compaction 或下一个 turn 开始时仍没有新 snapshot，则 after 为 unknown。`modelContextWindow` 为空时不能从模型名称或计费 usage 补猜。

ACP 的 `used/size` 默认标记为 `estimated`；只有协议明确声明非近似值时才可标记 `reported`。`_meta.approximate=true` 必须保持 `estimated`。估算值采用三次连续高压规则，不参与低效 compaction 比率。

阈值的目的不是提前替 Runtime 做 compaction，而是在 Runtime 已经连续无法把上下文拉回安全区时止损。上线后根据真实 `agent_run_events` 调整常量；初版不做用户配置和自适应算法。

## 6. Task-aware Handoff 与裁剪

初版 Handoff 是 Server 从持久化事实生成的 Continuity Packet，不要求已经拥挤的旧 Session 再运行一轮自我总结。这样即使旧 Runtime 已无法可靠响应，Task 仍能继续。

新 Session 的冷启动上下文按以下优先级生成：

1. 当前触发消息或当前 Task 的明确输入；
2. `claimer_id = 当前 Agent` 且为 `in_progress` 的 Task；
3. `claimer_id = 当前 Agent` 且为 `in_review` 的 Task，标明“等待 review，可能被退回”；
4. `creator_id = 当前 Agent` 且为 `in_review` 的 Task，只保留待 review 提醒，不复制其他 Agent 的执行历史；
5. 当前明确触发的 Task，无论它是否已经被认领；
6. Channel 中 `claimer_id IS NULL AND status='todo'` 的 Task，保持现有“可领取任务”语义；
7. Agent memory、项目目录和现有系统提示词。

明确排除：

- `done/closed` Task 的原始消息和执行历史；
- 被其他 Agent 认领的 Task 及其详细历史；
- 没有 `agent_run_task_links` 可靠关联、仅靠文本猜测“可能属于某 Task”的旧消息；
- 旧 provider transcript 的整段复制。

Continuity Packet 最长 2,000 字符，超限时按 `当前 Task > 自己执行中的 Task > 等待自己 review 的 Task > 自己提交 review 的 Task > todo` 截断，并保留 Task 编号以及让 Agent 重新读取 Task/Thread 的命令提示。该 Packet 写入触发换代的 `agent_run_events` 供审计；每次 cold start 都根据最新 Task 状态重新生成，不以“是否存在旧 active Session”猜测要不要注入，也不盲信旧快照。

示例：

```text
# Session Continuity
Previous provider Session was retired after ineffective compaction.

## Work you still own
- #42 [in_progress] 修复导出失败
  Goal: ...
  Continue from the current project files; inspect the Task thread before editing.
- #51 [in_review] 新登录页
  Waiting for creator review; do not redo unless it is rejected.

## Available work
- #57 [todo] 补充安装文档
```

Task 状态和 claimer 在换代过程中不发生变化。`in_review` 只进入 Continuity Packet，不触发自动重跑。`done/closed` 的数据也不删除，因此人工 reopen 后仍可从数据库、消息和项目文件恢复。

如果真实使用证明 2,000 字符不足，或者 Agent 反复重新发现 Task/项目中没有记录的关键决策，再增加显式语义 checkpoint。初版不为尚未观测到的损失预建 checkpoint 生命周期。

## 7. 端到端数据流

```text
Runtime protocol event
  -> adapter emits OutputChunk{Type: context}
  -> Daemon aggregates the current turn and forwards useful events
  -> one common turn finalizer runs for Result, stream error, timeout and cancel
  -> Session Manager evaluates the counters
       -> healthy: keep the existing Session
       -> rollover: report session_rollover_requested, but keep the Session idle
  -> Daemon terminal payload carries the recommendation
  -> Server transaction:
       lock and validate this Run + run.session_id
       append session_rollover_requested exactly once
       set exactly run.session_id to rollover_pending
       snapshot current Task-aware Continuity Packet
  -> the authoritative dispatch resolver sees rollover_pending
  -> Server stores newRun.rollover_from_session_id before dispatch
     clears resume ID and sends ForceFreshSession + retire_session_id
  -> Daemon acquires the same scoped turn gate used by Send/Start
     rechecks the provider ID, closes the matching old Session, and starts fresh
  -> provider creates a new Session ID
  -> one Server transaction upserts and binds the new active Session
     marks rollover_from_session_id closed
     and appends session_rollover_completed
```

`retire_session_id` 是旧 provider session ID。Daemon 新增一个原子的 `RetireAndStartFresh` 路径：先获取与 `GetOrCreateScopedSession` 相同的 scoped turn gate，等待前一 turn 真正结束，再在锁内重读 entry/session ID。只有仍匹配时才移除并关闭；如果本地已经是另一个新 Session，则不得误关，直接使用新 Session。fresh 启动若仍返回同一个 provider ID，视为换代失败，不能把旧数据库记录重新激活。

Server 增加唯一的 Session dispatch resolver，所有普通 Channel 消息、Thread、Task、artifact、result reminder、自动 recovery 和远程持久化 dispatch 都必须在最终发送/accept 前经过它。`rollover_pending` 的优先级高于调用方显式传入的 resume ID：候选 ID 一旦是 pending/closed，就清空 resume 并改为 fresh。现有分散在消息与普通 Run 路径里的 resume 查询收敛到该 resolver。

resolver 通过 `agent_runs` 关联按 `(agent_id, channel_id, provider)` 查 Session，不给 `agent_sessions` 补一个重复的 Channel 字段：

| 最新有效状态 | dispatch |
| --- | --- |
| `active` | resume 该 provider ID |
| `rollover_pending` | 写入 `rollover_from_session_id`，发送 fresh + exact retire ID |
| `closed` 或从未有 Session | 普通 cold start |
| 旧 pending 之后已经有不同的 active Session | 幂等收敛旧 pending 为 closed，resume 新 active |

显式 recovery resume ID 若已 pending/closed，同样必须被 resolver 覆盖。

message-wake single-flight 只覆盖消息入口，Task、artifact 或已经被 Daemon 接收的请求仍可能提前排队。它们可以再安全执行一个旧 Session turn，但该 Session 级 `rollover_pending` 不会生成第二个 requested event；下一条经过 resolver 的请求完成换代。尚未被远程 Computer accept 的 dispatch payload 在读取时重新经过 resolver，不信任其中可能过期的 resume ID。

Server 永远按 `run.session_id` 提交换代，按 `retire_session_id` 关闭本机 entry，不能按“该 Agent 最新 Session”操作，以免影响其他 Channel。

## 8. 持久化与事件

初版只有一个增量 migration：

```sql
ALTER TABLE agent_runs
ADD COLUMN rollover_from_session_id UUID
REFERENCES agent_sessions(id) ON DELETE SET NULL;
```

其余数据复用现有表。该列把 fresh dispatch 的旧 Session 意图持久化，避免 Server 在“新 Session 已建立、旧 Session 尚未关闭”之间崩溃后只能靠事件或时间猜测。

每个 Run 最多保存：

- 一个最终 `context_snapshot`；
- 每次已完成的 `context_compaction`；
- 一个幂等的 `session_rollover_requested`；
- 新 Session 建立时一个 `session_rollover_completed`。

不保存每条高频 usage update。建议 payload：

```json
{
  "provider": "codex",
  "before_tokens": 190000,
  "after_tokens": 182000,
  "window_tokens": 200000,
  "saved_ratio": 0.0421,
  "accuracy": "snapshot",
  "reason": "ineffective_compaction",
  "continuity": "# Session Continuity\n..."
}
```

`saved_ratio` 由统一策略计算，adapter 只报告原始数字。`agent_runs.usage_json` 继续只承载计费 token，不能加入 context 字段。

Server 增加一个专用事务方法，而不是拼接现有非事务版 `AppendEvent`：

```text
SELECT Run FOR UPDATE
-> 验证 run.session_id 非空，且 Session 属于 run.agent_id
-> 锁定该 Session
-> 若 Session 已 rollover_pending，返回已提交，不重复写事件
-> UPDATE exact Session: active -> rollover_pending
-> INSERT session_rollover_requested with next seq
-> COMMIT
```

幂等键是旧 Session，不只是当前 Run：同一个 pending Session 后续多执行的旧 turn 不能各写一次 requested。重复或重放的请求返回已提交状态。现有普通 `AppendEvent` 没有共享该 Run row lock，因此 `MAX(seq)+1` 仍可能与并发事件冲突；专用事务遇到 `(run_id, seq)` 唯一键冲突时整体回滚并有限重试，不能留下只改状态、没写事件的半提交。`run.session_id` 为空、Session 不属于该 Agent、状态不是 `active/rollover_pending` 时，不得宣称换代已经提交。

dispatch resolver 在发送前把旧 Session 的数据库 UUID 写进新 Run 的 `rollover_from_session_id`。新 provider ID 到达后，不再串联现有两个独立调用，而是使用一个可重放事务：

```text
lock new Run + rollover_from_session_id
-> 验证 old Session 仍 pending
-> 拒绝 new external_session_id == old external_session_id
-> upsert new active Session
-> bind new Run to new Session
-> old pending -> closed
-> insert session_rollover_completed exactly once
-> commit
```

Server 在其中任一步崩溃都不会留下“新 active 已绑定但旧 pending 无法追溯”的半状态；相同 session event 重放会收敛到相同结果。`agent_sessions` 的状态因此是单调的：

```text
active -> rollover_pending -> closed
```

现有 `UpsertSession` 必须改为不再把 `rollover_pending/closed` 无条件更新回 `active`。迟到事件或已经排队的非-fresh Run 仍可绑定旧 Session 以保持审计正确，但不能复活状态；只有携带 rollover intent 的 fresh Run 返回同一个旧 external ID 时才拒绝绑定。初版不增加 `superseded_by` 或 checkpoint 表。

## 9. API、远程 Computer 与前端

Daemon 到 Server 沿用现有开放的 SSE/远程 Run event 管道，增加可选 `context` 事件和 terminal payload 中的可选 rollover 字段。Server 到 Daemon 的请求复用 `ForceFreshSession`，并增加可选 `retire_session_id`，用于验证本机即将关闭的确实是被 Server 提交换代的旧 Session。

兼容语义写死：

- `ForceFreshSession=true && retire_session_id!=""`：走新原子路径，只关闭 ID 匹配的 scoped Session；
- `ForceFreshSession=true && retire_session_id==""`：保持现有 missing-visible-result recovery 行为，在 turn gate 内关闭当前 scoped Session；
- `ForceFreshSession=false`：`retire_session_id` 必须为空，否则拒绝请求。

Daemon 注册时增加 `context_rollover_v1` capability。远程 control hello 也要携带 capability 列表，Server 不再把远程能力硬编码成只有 `llm`，而是保存并原样用于 dispatch gate；字段缺失按不支持处理。只有 Server 与 Daemon 同时支持该能力时，Server 才提交 `rollover_pending` 和发送 fresh 指令；无需仅为可选事件升级整个 daemon protocol version。

- 新 Daemon + 旧 Server：旧 Server 忽略未知事件，Daemon 只报告、不自行关闭，行为退化为当前版本。
- 旧 Daemon + 新 Server：没有新信号，保持当前 Session 行为。
- 新字段全部 optional；现有 Run status、Task API 和 WebSocket envelope 不变。

Server 把事件继续通过现有 `agent.run.event` 广播。frontend 初版只改 Agent Observability：

- 为 `context_snapshot`、`context_compaction`、`session_rollover_requested/completed` 增加中文标签；
- 展示压前、压后、节省比例、测量类型和换代理由；
- 为嵌套 context payload 使用专用 renderer，不能显示成 `[object Object]`；
- 收到 rollover event 或 `agent.run.updated` 首次带上新 `session_id` 时刷新 Session 列表；
- Task 看板和主聊天区不增加新状态，不把内部 compaction 当成一条聊天消息。

## 10. 失败恢复

- **Task 未完成**：只要 Task 仍是 `todo/in_progress/in_review`，换代不改变状态或 claimer；新 Session 通过最新 Continuity Packet 继续。
- **缺少 post snapshot**：记录 `unknown`，不计算虚假比例，不换代；若同时有完整 `used/window`，才走高压力规则。
- **当前 turn 失败**：所有 Result、stream error、timeout 和 cancel 都必须经过同一个 context finalizer；不能在 `MessageError` 分支提前 return 绕开它。新增结构化 `context_exhausted` failure code，并明确映射到 `fresh_session`；其他错误沿用现有分类。
- **Daemon 在决定前重启**：内存计数归零，Session 可照常 resume；不会丢 Task 或消息。
- **Daemon 在建议后重启**：若 Server 尚未提交，旧 Session 可继续；若 Server 已提交 `rollover_pending`，下一请求仍会明确发送 fresh，不依赖 Daemon 内存。
- **Server 在事件处理时重启**：远程 delivery event 可重放；本地重复请求由 `run_id + session_rollover_requested` 幂等检查收敛。`run.session_id` 保证迟到事件只能作用于当时的 Session。
- **关闭进程失败**：`retire_session_id` 匹配后，从 session pool 移除旧 entry，再做现有 graceful close；必要时走现有强制清理。不会删除 transcript 或项目文件。
- **换代后新 Session 启动失败**：Task 保持原状态，Run 按现有失败与最多三次恢复处理；不能重新启用已判定耗尽的旧 Session。

`in_review` Task 只保留上下文，不进入自动 recovery；当前 retry 查询继续只处理 `todo/in_progress`。

`context_exhausted` 只能来自 adapter 解析到的稳定结构化错误类型/错误码，并必须先由真实 Runtime fixture 锁定：Claude 只接受其结构化 result/API error type，Codex 只接受 `turn.error` 或 app-server error 的稳定 code，ACP 只接受 JSON-RPC `error.code/data` 中 Runtime 明确定义的 context error。未验证的错误文案匹配只记日志，不能自动关闭 Session；某 Runtime 没有稳定映射时，先依靠压力规则。

因为判断发生在同一台 Computer、同一 Runtime stream 的 terminal 边界，且 Server 按 Run 绑定的精确 Session 幂等提交，不需要用 Epoch 解决跨设备迟到消息归属问题。

## 11. 兼容现有 Solo 行为

- Channel 与 Thread 继续共享 provider Session；不会退化成每个 Thread 一套 Session。
- Thinking Node 不进入本策略，继续使用现有 checkpoint/fork/return handoff。
- Agent 删除、成员移除、Channel/Workspace 删除或归档等生命周期清理中的 Session 条件都从仅 `active` 扩展为 `active OR rollover_pending`；只有 resume 查询仍限定 `active`。模型或项目切换继续走 fresh 路径。
- idle sleep 继续保留 provider session ID；内存健康计数随 scoped entry 一起保留。
- 可见回复、freshness、result reminder、Task submit/review 和 token budget 不改变。
- 普通聊天与 Task 的历史归属只相信数据库关系，不增加文本分类器或向量检索。

## 12. 实施顺序

1. 增加统一 `ContextEvent`，实现 Claude、Codex 解析，并修正 ACP `used/size`；为缺值、错误字段和 Codex snapshot 配对写最小解析测试。
2. 在 Daemon 聚合每 turn 的最终 snapshot/compaction，加入纯函数策略和覆盖所有退出路径的 context finalizer；此阶段只上报建议，不关闭 Session。
3. 增加 `agent_runs.rollover_from_session_id` migration；Server 实现 Session 级幂等的 `active -> rollover_pending`、sticky upsert 和统一 dispatch resolver。
4. 在 Server 请求中加入 `retire_session_id`；Daemon 在 scoped turn gate 内原子完成匹配校验、关闭旧 Session 和 fresh start。
5. Server 用一个可重放事务完成“upsert/bind 新 Session + 关闭旧 pending Session + completed event”，并把已验证的 context overflow 接到现有 `fresh_session` Task recovery。
6. 把现有 open-task summary 扩展成按 Agent 过滤的 Continuity Packet；不改 Task 状态机。
7. 扩展所有 Agent/Channel/Workspace 生命周期清理，使其同时处理 `active/rollover_pending`。
8. Agent Observability 增加事件标签、专用摘要和 Session 刷新。
9. 实现本地注册和远程 control hello 的 `context_rollover_v1` capability gate；E2E 显式启用它，真实链路通过后生产 Daemon 才正式声明该能力。没有该能力时只观测，不改变 Session 生命周期。

最后一条是发布门禁，不是永久 feature flag：在真实 Claude/Codex 事件形状没有验证前，不允许仅凭单元测试直接自动关闭用户 Session。

## 13. 验证

单元检查覆盖：

1. Claude 有/无 `postTokens`；
2. Codex 只使用 `tokenUsage.last.totalTokens`，不误用累计 `total`；
3. Codex compaction 后没有新 snapshot 时 after 为 unknown；
4. ACP 扁平 `used/size`；
5. 两次低效 compaction、两次持续高压、有效压缩清零计数；
6. unknown 中断 streak，ACP estimated 使用三次规则；
7. pending/closed Session 不被 upsert 复活，pending 覆盖显式 resume；
8. `retire_session_id` 匹配、不匹配和缺省的三种兼容路径；
9. context 事件不进入 `agent_runs.usage_json`。

产品 E2E 必须使用 make 管理的真实 frontend、API server、PostgreSQL、Daemon 和本机真实 Agent Runtime，不 mock route、service、database 或 provider。至少验证：

1. Claude 与 Codex 的真实 compaction 事件到达 `agent_run_events`；
2. Observability 页面展示 before/after/ratio/accuracy；
3. 达到规则后，旧 Session 为 `closed`、新 Run 绑定新的 `active` Session；
4. 原 Task 的 status、claimer、Thread 消息和项目文件不变；
5. 新 Session 能看到 Continuity Packet 并继续未完成 Task；
6. 整个 stack 重启后不会 resume 已淘汰 Session；
7. 已经排队的旧 turn 不会重复 requested，之后的 Task/recovery/remote dispatch 都被统一 resolver 强制 fresh；
8. Agent/Channel/Workspace 清理不会遗留 `rollover_pending`；
9. 未达到规则时，同一 Channel 仍跨 Run、idle sleep 和 stack restart 恢复同一 Session。

真实 Runtime 的 compaction 使用 provider 原生显式 compact 入口触发，验证 wire event，不伪造 adapter 消息。two-strike 策略由纯函数单测确定；完整 rollover E2E 在既有 E2E 环境标记下允许覆盖阈值/strike 数，以低成本触发同一套生产决策和真实 Session 切换链路。该覆盖不暴露成产品配置，测试仍使用真实 Runtime、Daemon、Server、数据库与前端。

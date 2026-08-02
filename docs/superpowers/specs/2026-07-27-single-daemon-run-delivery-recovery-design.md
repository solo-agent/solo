# Solo 单用户、单 Daemon 运行与恢复架构

日期：2026-07-27

## 1. 目标与边界

本设计解决三个用户可感知的问题：

1. Agent 后台显示完成，但消息没有真正送达频道或 Thread。
2. Server、Daemon 或前端短暂重启后，运行状态无法可靠恢复。
3. Agent 失败后 Task 长时间停留在已领取状态，不能自动换人继续。

当前产品约束是单用户、单 Server、单 Daemon、本地 PostgreSQL。设计明确不引入多租户隔离、Daemon 选主、分布式锁、租约、消息队列或跨节点共识。以后如果产品约束改变，再在持久化状态机外增加协调层；当前状态机和结果契约可以保留。

## 2. 核心判断

`agent_runs.id` 是一次 Agent 尝试的唯一运行身份，也是 Server、Daemon、CLI、事件和消息之间共同使用的 `run_id`。不再为同一次尝试生成第二个独立的 Daemon `task_id`。

运行成功由结果契约决定，而不是由模型进程正常退出、SSE 收到 `complete`、产生过文本，或命令文本中出现过 `solo message send` 决定。

普通消息和 Task 的结果契约是 `visible_message`：只有消息已经持久化到 `messages`，且消息的 `metadata.agent_run_id` 指向本次运行，运行才能进入 `completed`。

Thinking 内部交接使用 `handoff` 契约；纯后台动作使用 `none`。第一阶段只强制普通消息和 Task 的 `visible_message`，现有 Thinking 交接保持兼容；后续在 Thinking 恢复阶段把交接写入同一确认事件。

## 3. 领域模型与所有权

### Task

Task 表示用户目标。它可以经历多次 Agent 尝试，但同一时刻最多有一个有效的主运行。

- 所有者：Server。
- 持久化：`tasks`。
- 生命周期：`todo -> in_progress -> done`，失败重派期间保持 `in_progress`。
- 与运行的关系：`agent_run_task_links` 记录每次尝试，不覆盖历史。

### AgentRun

AgentRun 表示一次不可复用的执行尝试。

- 所有者：Server 创建并决定终态。
- 执行者：唯一 Daemon。
- 唯一身份：`agent_runs.id`。
- 生命周期：`queued -> running/streaming -> completed|failed|cancelled|timeout`。
- 终态不可逆，首个终态生效。
- `usage_json` 保存本次尝试的 input、output、cache read 和 cache write token 使用量。

### AgentSession

Session 表示 provider 可恢复的上下文。

- 所有者：Server 保存映射，Daemon/runtime 产生 provider session ID。
- 生命周期跨多个 Run。
- 普通频道、Thread、Thinking node 的恢复必须带明确 scope，不能仅按 Agent 找“最近一次”。

### ResultContract

ResultContract 是运行成功必须满足的交付条件：

- `visible_message`：必须存在与 Run、Agent、Channel、Thread/Thinking scope 一致的持久化消息。
- `handoff`：必须存在已提交的 Thinking checkpoint/fork/return 交接。
- `none`：backend 成功即完成，限无用户交付的后台动作。

第一阶段把契约写入 `run_started` 事件 payload，避免为了单个枚举扩展 `agent_runs` 以及所有扫描查询。恢复逻辑需要读取时，从首个 `run_started` 事件取得。若未来按契约做高频筛选，再提升为列。

### Message

Message 是 `visible_message` 的唯一交付证明。

- 所有者：Message API。
- 只有数据库 INSERT 成功后才算送达。
- Agent 消息写入 `metadata.agent_run_id` 和 `metadata.delivery = "visible"`。
- Message API 校验 Run、Agent、Channel 及 Thread/Thinking scope，防止伪造或串线。
- WebSocket 只是入库后的通知，不是事实来源。

### RunEvent

RunEvent 是追加式时间线和恢复证据。

- `assistant_message` 只表示模型内部输出，不再表示用户可见结果。
- 新增 `visible_message_sent`，只在消息成功入库后追加，payload 含 `message_id`。
- `run_started` payload 保存 `result_contract`。
- `usage` 和终态事件继续作为可观测时间线。

## 4. 端到端数据流

### 普通消息或 Task

1. 前端消息先写入 Server/PostgreSQL。
2. Server 解析目标 Agent，创建 `agent_runs` 记录。
3. Server 将该 `agent_runs.id` 同时作为 Daemon `task_id` 和 `run_id`。
4. Daemon 启动本地 Agent runtime，并注入 `SOLO_RUN_ID`。
5. Agent 用 `solo message send` 交付。
6. CLI 将 `run_id` 传给 Daemon proxy；直连 Server 的 fallback 也携带同一值。
7. Message API 校验 scope，在同一数据库事实链上写入消息 metadata。
8. 消息入库成功后追加 `visible_message_sent`。
9. Daemon 上报 provider complete 和 usage。
10. Server 收到 complete 后检查带本次 `agent_run_id` 的持久化消息；存在才完成，否则以 `missing_visible_result` 失败。`visible_message_sent` 只作为审计事件。

模型纯文本、thinking、tool use 和 tool result 仍可进入 Agent 观察面，但不进入频道，也不满足结果契约。

Provider final result 没有 usage，或持久 Session 返回的是累计 usage 时，Server 使用本次 Run 时间窗口内的本地 transcript usage 作为每次尝试的权威值。

### Thinking 交接

Thinking node 的消息仍由现有 protocol handler 处理。目标形态是在 checkpoint/fork/return 提交成功后追加 `handoff_committed`，再满足 `handoff` 契约。第一阶段不改变其对用户隐藏的交接行为，避免把协议消息误当频道消息。

## 5. 持久化设计

第一阶段不新增表和列：

- `agent_runs.id`：统一运行身份。
- `agent_runs.usage_json`：保存 `{input_tokens, output_tokens, cache_read_tokens, cache_write_tokens}`。
- `messages.metadata`：保存 `agent_run_id`、`delivery`。
- `agent_run_events`：
  - `run_started.payload.result_contract`
  - `visible_message_sent.payload.message_id`
  - `error.payload.failure_code`

这复用现有 schema，迁移成本为零。历史消息没有 `agent_run_id`，历史 Run 也没有结果契约；它们只读兼容，不回填，不参与自动重派。

自动重派阶段再增加最小必要字段：

- Task 的当前尝试次数优先通过 `agent_run_task_links` 聚合，不新增计数列。
- 是否可重试由本次终态事件的 `failure_code/retryable` 决定。
- 只有确认旧 Run 已终态且本地 runtime 已停止，才创建下一 Run。

## 6. API 与进程边界

### Server -> Daemon

现有流式请求继续保留 `task_id` 与 `run_id` 以兼容混合版本，但新 Server 对两者赋相同值。Daemon 内部任务管理、SSE 路由、取消和日志都以该值工作。

### Daemon -> Agent

新增环境变量：

```text
SOLO_RUN_ID=<agent_runs.id>
```

该值属于 runtime，Agent 自定义环境不能覆盖。

### Agent CLI -> Daemon/Server

`solo message send` 自动读取 `SOLO_RUN_ID`：

- Daemon proxy body 携带 `run_id`。
- direct API fallback body 也携带 `run_id`。

用户手工运行 CLI 时通常没有该变量，行为保持不变；只有 Agent 身份提交带 `run_id` 的消息才参与运行交付确认。

### Message API

`CreateMessageRequest` 增加可选 `run_id`。当 sender 是 Agent 且提供 `run_id` 时：

- Run 必须存在。
- `run.agent_id == sender_id`。
- `run.channel_id == channel_id`。
- Run 的 Thread/Thinking node scope 与最终解析后的消息 scope 一致时，消息才计为本次 Run 的交付。
- Run 必须尚未进入终态。

Run 不存在、已结束或不属于当前 Agent 时返回 409。Agent 在一次运行中发往其他合法 scope 的协作消息仍可正常落库，但不写交付 metadata，也不满足本次 Run 的结果契约；这样不会破坏现有跨频道协作。

## 7. 前端状态

前端不维护独立真相：

- 页面刷新后从 API 重取 Run 和消息。
- WebSocket/SSE 只做增量更新。
- `agent.run.finished=completed` 意味着结果契约已经满足。
- `failed + missing_visible_result` 显示 Agent 未交付结果；当前阶段复用 `agent.error` 通知和 Run 失败状态。
- 自动重派阶段显示系统消息：“本次尝试未成功交付，Solo 正在自动改派（第 N/3 次）”。
- 三次尝试耗尽后显示持久化系统消息，并把 Task 退回 `todo`、清空 claimer。

无需为 `run_id` 增加新的前端 store；消息 metadata 和运行面板可用于诊断，但普通消息渲染不变。

## 8. 失败恢复

### Daemon 或本地 runtime 异常

Server 以 Run 持久化状态为准。单 Daemon 重连后：

- Daemon 能证明本地 task 仍在运行：重新订阅同一 `run_id`。
- Daemon 无该 task：旧 Run 进入 `failed/daemon_lost`。
- 不把旧 Run 改回 active，也不复用 Run ID 创建第二次尝试。
- Daemon 正常退出时先强制关闭并回收全部 provider runtime，停止发送正常的 cancelled/complete 终态；随后注销，由 Server 统一写入 `daemon_lost`，最后才释放机器锁，避免旧 runtime 在重派后继续修改 Task。

### Server 重启

启动后扫描 active Run：

- 对照唯一 Daemon 的运行列表。
- 一致则恢复流订阅和前端投影。
- 不一致则收敛为 `daemon_lost`，再按自动重派策略处理。

### 前端断线

前端重连后重新拉取消息和 Run 列表。因为可见结果与终态都已入库，不依赖补齐丢失的实时事件。

### 缺少可见结果

backend complete 但结果契约未满足：

- Run 终态为 `failed`。
- failure code 为 `missing_visible_result`。
- 不把模型纯文本补写为频道消息。
- 普通消息触发只通知失败；Task 触发进入自动重派判断。

### 自动重派

仅以下失败自动重派：

- `daemon_lost`
- `timeout`
- `provider_transient`
- `missing_visible_result`

以下失败不重派：

- 用户取消
- 权限拒绝
- 无效输入
- Task 已完成或已被用户改派

策略：

1. 最多 3 次总尝试。
2. 旧 Run 必须先终态，且确认 runtime 已停止。
3. 优先排除刚失败的 Agent；没有其他符合条件的 Agent 时可在下一次重新选择原 Agent。
4. Task 在尝试期间保持 `in_progress`，每次尝试保留独立 Run。
5. 每次改派写可见系统消息。
6. 三次耗尽后 Task 回到 `todo` 并清空 claimer，等待用户处理。

单用户、单 Daemon 下不需要并发租约；数据库事务内检查 Task 当前状态和最新主 Run 足以防止 watchdog 与回调重复重派。

## 9. 兼容与迁移

- 不做数据库 schema 迁移。
- 旧 Daemon 不认识 `run_id` 时仍可使用 `task_id`；新 Server 发送相同值。
- 旧 CLI 不携带 `run_id` 的 Agent 消息仍能发送，但不能满足新 Run 的 `visible_message` 契约，因此 Server 与 Daemon 应作为一个版本通过 `make rebuild` 升级。
- 用户手工消息、用户手工 CLI、历史消息和历史 Run 行为不变。
- Thinking protocol 第一阶段保持现状，后续只增加交接确认事件。

## 10. 验证

第一阶段成功标准：

1. Daemon 收到的 `task_id == run_id == agent_runs.id`。
2. Agent runtime 环境中存在不可覆盖的 `SOLO_RUN_ID`。
3. Agent 消息入库后 `messages.metadata.agent_run_id` 正确。
4. 同一 Run 的 `visible_message_sent` 事件只在入库成功后出现。
5. 只有内部文本而没有消息入库时，Run 不能是 `completed`。
6. 正常交付时 Run 为 `completed`，usage 写入 `agent_runs.usage_json`。
7. 前端可见消息和数据库记录一致。

验证顺序：

- Go 单元/集成测试覆盖 scope 校验、消息确认、统一身份、缺失结果和 usage。
- `make rebuild` 启动真实前端、API、Daemon、PostgreSQL。
- Playwright 通过真实 Agent runtime 触发一次正常交付和一次无法交付。
- 同时断言浏览器可见结果、`messages.metadata`、`agent_run_events`、`agent_runs.status` 和 `usage_json`。

## 11. 第二阶段实现边界

### 第 1 轮：单 Daemon 重启收敛

不新增恢复表或租约。Daemon 注册请求直接携带其内存中已知任务的
`task_id` 列表；由于 `task_id == run_id`，Server 可与 PostgreSQL 中的
active Run 做集合对账：

- Server 内存仍在跟踪的 Run 不重复订阅。
- Daemon 仍保留的 active/terminal Run 重新订阅原 SSE；Daemon 的 replayable
  事件恢复 backend/session/complete/error/done，沿用原 Run，不创建新 Run。
- PostgreSQL active、但 Daemon 快照不存在的 Run 以
  `failed/daemon_lost` 收敛，并追加带 `failure_code` 与 `retryable` 的 error
  事件。
- Daemon 主动注销时同样使用 `daemon_lost`，不再误记为 timeout。

恢复只依赖 Run 行、首个 `run_started.result_contract`、主 Task link 和 Agent
配置；前端继续以现有 Run API/WebSocket 投影状态，不增加 store。旧 Daemon
不发送任务快照时，active Run 会按单 Daemon 前提收敛为 `daemon_lost`，因此
Server 与 Daemon 仍要求通过 `make rebuild` 同版本升级。

### 第 2 轮：自动重派

自动重派只挂在统一的 Run 终态入口之后，并且只处理带 primary Task link 的
Run。数据库事务锁定 Task 行后检查：

1. Task 仍未结束，且当前 claimer 未被用户改成其他 Agent。
2. 本 Run 是该 Task 最新一次 primary 尝试。
3. primary 尝试总数未超过 3。
4. error 事件明确标记 `retryable=true`，且 failure code 属于
   `daemon_lost/timeout/provider_transient/missing_visible_result`。

满足条件时优先选择同 Channel 内另一个 active Agent；没有其他 Agent 才回退
原 Agent。事务内把 Task 保持为 `in_progress` 并更新 claimer，事务提交后写一条
持久化 system 消息并触发下一 Run。三次耗尽时事务内把 Task 退回 `todo`、
清空 claimer，再写最终 system 消息。系统消息复用 `messages` 表和现有
WebSocket 投影，不建重试表；尝试次数由 `agent_run_task_links` 聚合。

重派触发必须在旧 Run 已终态之后。`daemon_lost` 表示旧 Daemon 已注销、重启后
快照中不存在该任务或进程已不可达；timeout 先请求取消；provider/missing-result
只在 Daemon 已发出 terminal SSE 后进入重派。这样新 Run 不与旧 runtime 并行。
重派 Run 的任务上下文必须以数据库中的当前 `claimer_id` 为准，明确告知新 Agent
它已是当前执行者；原 Task 创建消息中的旧 @mention 只保留为历史内容，不能用于
拒绝本次执行。

### 每轮门禁

每轮都执行：

- 正确性 Review：检查 frontend/server/daemon/runtime/session/database 全链路和
  与历史行为的兼容性。
- Ponytail Review：只保留现有表、事件、任务快照和一个事务入口；若能删减则先
  删减再复测。
- 真实 E2E：使用 `make rebuild` 管理真实 frontend/API/Daemon/PostgreSQL 和本地
  Agent runtime；断言浏览器可见结果及数据库 Run/Event/Task/Message 状态。
- 全量 Go 测试、前端 build/lint 和 `git diff --check`。

## 12. 实施顺序与工作量

1. 统一运行身份、交付确认、usage：4–6 人日。
2. Server/Daemon 重启恢复与 SSE 重连：3–5 人日。
3. Task 自动重派和三次熔断：5–8 人日。
4. Channel/Thread/Thinking session 精确恢复：3–5 人日。
5. 本地权限 profile：5–8 人日。
6. 兼容清理、指标和故障演练：2–4 人日。

实施按上述顺序分轮推进：先建立后续功能依赖的唯一运行身份和可信终态，再补恢复、自动重派与 Session 连续性；权限 profile 不在本 MR 范围内。

## 13. 普通 Channel Session 重启连续性修订

### 边界

现有 scope 设计保持不变：

- 普通 Channel、DM 和其 Thread 共用 `Channel + Agent` provider Session。
- Thread 的消息、Run 和交付证明继续由 `thread_id` 精确路由；不为每个 Thread
  新建 provider 进程。
- Thinking node 继续使用独立 node Session 和现有 `ResumeSessionID`。

本轮只验证并补齐普通 Channel provider Session 在 Daemon 重启后的连续性，不改变
消息路由、Task 重派、Thinking 生命周期或前端状态模型。

### 所有权与生命周期

Server 继续拥有 `agent_sessions` 与 `agent_runs.session_id` 映射；Daemon 继续拥有
活跃 provider 进程。Daemon 内存中已有 Channel Session 时直接发送新 turn；内存
Session 不存在时，Server 从 PostgreSQL 查找同一 Agent、provider、Channel 且非
Thinking scope 的最近 active provider session ID，并通过现有
`resume_session_id` 传给 Daemon；closed Session 不恢复。

Thread 与 Task thread 复用同一 Channel scope，因此恢复查询允许最近 Run 带或不带
`thread_id`，但必须匹配 Agent、provider、Channel 且 `thinking_node_id IS NULL`。

### 数据流、持久化和 API

1. Run 收到 Daemon 的 session 事件后，现有逻辑继续 upsert `agent_sessions` 并写入
   `agent_runs.session_id`。
2. 后续普通 Channel/Thread/Task Run 在公共调度入口读取最近
   `agent_sessions.external_session_id`。
3. Server 复用现有 `daemonTaskRequest.ResumeSessionID`；不新增 HTTP 字段。
4. Daemon 复用现有 `GetOrCreateScopedSession(..., resumeSessionID, ...)`。

不新增表、列、索引或前端 store。历史 Run 没有 session 映射时按现有冷启动行为
执行；查询失败也只记录日志并冷启动，不阻断用户消息。

### 失败恢复与兼容

- provider 拒绝 resume 时，现有 backend 行为回退到创建新 Session。
- 活跃 Daemon 内存 Session 优先，数据库 resume ID 不替换正在运行的进程。
- Thinking node 不进入普通 Channel 恢复查询。
- 默认行为对旧数据和当前 Channel/Thread 路由完全兼容。

### 真实验证

真实 E2E 使用 Claude 和 make 管理的 frontend/API/Daemon/PostgreSQL：

1. 在 Channel 中让 Agent 记住一个随机值并交付确认。
2. 记录 Run 绑定的 `external_session_id`。
3. 执行 `make rebuild`。
4. 在同一 Channel 再次触发 Agent，验证用户可见回复仍包含该随机值。
5. 验证新 Run 仍绑定同一个 `external_session_id`，且消息 metadata 指向新 Run。

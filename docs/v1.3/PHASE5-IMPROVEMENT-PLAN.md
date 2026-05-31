# Solo v1.3 Phase 5 — Slock 对齐改进计划

> 基于 Slock 4 个 agent（Cindy/产品/前端/后端）完整行为日志分析
> 前置：Phase 4（架构基础 + Prompt 重写 + Session 模型）

---

## 一、已完成的改进（Phase 4 交付）

### 架构层
| # | 改进项 | 文件 | Slock 对标 |
|---|--------|------|-----------|
| A1 | PersistentBackend 接口 (Start/Send/Close) | `pkg/agent/backend.go` | daemon agent process lifecycle |
| A2 | ClaudeBackend 持久化模式 | `pkg/agent/claude.go` | persistentStreamLoop = stdin 保持 open |
| A3 | AgentSessionManager | `pkg/agent/session.go` | session 池 + per-agent turn 串行锁 |
| A4 | 启动排队 (max=5, 500ms) | `pkg/agent/session.go` | Start queued / Dequeued start |
| A5 | Crash 恢复 (--resume) | `pkg/agent/session.go` | Process exited 143 → auto-restart |
| A6 | 无 timer sleep | `pkg/agent/session.go` | 进程长期存活，仅 crash/stop 终止 |
| A7 | NewPersistentBackend 工厂 | `pkg/agent/factory.go` | 按 provider type 创建 |

### Prompt 层
| # | 改进项 | 文件 | Slock 对标 |
|---|--------|------|-----------|
| P1 | System Prompt 重写 (350 行, 14 Section) | `pkg/agent/prompt.go` | Slock 350 行 prompt 结构 |
| P2 | CRITICAL RULES (5 条 NON-NEGOTIABLE) | `pkg/agent/prompt.go` | "Stay in your lane" / "Silence is the default" |
| P3 | mentionedNames 参数 | `pkg/agent/prompt.go` | "You WERE @mentioned" / "You were NOT @mentioned" |
| P4 | 消息格式说明 | `pkg/agent/prompt.go` | `[type=human\|agent] @sender_name: content` |
| P5 | Decision Flow (7 步) | `pkg/agent/prompt.go` | 匹配 Slock agent 实际决策逻辑 |
| P6 | CLAUDE.md 命令更新 | `pkg/agent/claude_template.go` | message check + server info + silence 规则 |

### 触发层
| # | 改进项 | 文件 | Slock 对标 |
|---|--------|------|-----------|
| T1 | 移除 @mention 过滤 (全量触发) | `internal/server/service/agent.go` | 所有 agent 接收所有消息 |
| T2 | daemonTaskRequest.MentionedNames | 同上 | @mention 感知传递链路 |
| T3 | resolveMentionedNames() | 同上 | agent ID → human-readable name |
| T4 | 消息格式 `@sender_name: content` | 同上 (getRecentMessages) | Slock inline @mention 格式 |
| T5 | Task 通知格式 `[task #N status=todo]` | 同上 (TriggerAgentForTask) | Slock task 嵌入格式 |
| T6 | "Respond as appropriate" 指令 | 同上 | Slock per-message 路由指令 |
| T7 | resolveSenderName() | 同上 | 消息中显示发送者名称 |

### Agent 生命周期
| # | 改进项 | 文件 | Slock 对标 |
|---|--------|------|-----------|
| L1 | TriggerAgentGreeting | `agent_helpers.go` + `member.go` | "First message task (system-triggered)" |
| L2 | Slock 对齐的欢迎格式 | 同上 | `[target=#channel type=system] @Solo: ...` |

### CLI
| # | 改进项 | 文件 | Slock 对标 |
|---|--------|------|-----------|
| C1 | solo message check | `cmd/solo/main.go` | slock message check |
| C2 | solo server info | 同上 | slock server info |

---

## 二、改进实施（全部完成 ✅）（按优先级排序）

### ✅ P0-1: Freshness Hold / Draft 机制

**Slock 行为**：Agent 准备发消息时，若 daemon 检测到新消息已到达，不直接发送，保存为 draft，向 agent 展示新消息，agent 决定 `--send-draft` 或修改重发。

**Cindy 证据**：3 分钟内触发 5 次 freshness hold。每次通过 `--send-draft` 处理。没有这个机制，欢迎消息只提到 @后端（没提到刚加入的 @前端 @产品）。

**Solo 实现方案**：

在 `processTaskWithBackend` 的 streaming loop 中，监听到 agent 准备输出（收到 "complete" 事件）但尚未 persist 到 DB 时：

```
1. agent 输出 complete → daemon 持有 finalContent
2. daemon 检查该 agent 的 pendingMessages 队列是否非空
3. 若非空 → 不 persist，将新消息注入 agent session（Send() 新 turn）
4. agent 看到新消息，决定：修改回复 / 直接发送 / 放弃
5. 若空 → 正常 persist + broadcast
```

**改动文件**：
- `cmd/daemon/handler.go`: `processTaskWithBackend` 中 "complete" 分支加 hold 逻辑
- `pkg/agent/session.go`: `AgentSessionManager` 加 `pendingMessages` 队列

**预估**：4-6h

### ✅ P0-2: Inbox Notification（轻量 ping）

**Slock 行为**：有新消息时先发送 `"1 pending inbox message(s)"` 通知到 stdin，不注入完整内容。Agent 在断点处主动调用 `slock message check` 拉取。

**Cindy 证据**：日志中频繁出现 `[System notification: You have 1 pending inbox message. Call check_messages to read it when you're ready.]`。Agent 在完成当前工作后才 `slock message check`。

**Solo 实现方案**：

```
Server → daemon Run() → 检查 agent 是否正在处理 turn
  若 activeTurn == true → 不立即 deliver，写入 pending queue
  向 agent stdin 写入轻量通知：
    "[Solo] 1 pending message. Use `solo message check` when ready."
  agent 在断点处调用 solo message check → daemon 返回 pending 消息内容
```

**改动文件**：
- `cmd/daemon/handler.go`: 添加 pending queue + notification 写入
- `cmd/solo/main.go`: `message check` 命令对接 daemon pending queue API
- `pkg/agent/session.go`: `deliverToSession` 检查 turn lock 状态

**预估**：4-6h

### ✅ P1-3: 消息注入时的上下文保护

**Slock 行为**：Agent 处理消息时，新消息不打断当前 turn。新消息排队，当前 turn 完成后再处理。

**当前 Solo 问题**：我们的 turn lock 已经做了串行化，但新消息到达时是**直接写入 stdin**。如果 agent 正在处理上一个 turn，stdin 写入会与 Claude Code 的输出交错。

**实现方案**：在 `deliverToSession` 中，如果 turn lock 已被持有（agent 正在处理），将消息加入 pending queue，不写入 stdin。锁释放后，从 queue 取出下一条消息写入。

**改动文件**：
- `pkg/agent/session.go`: `deliverToSession` 中加 pending queue 逻辑

**预估**：2-3h（与 P0-2 部分重叠）

### ✅ P1-4: Cross-agent Workspace 访问

**Slock 行为**：Agent 可以用 `find` 和 `Read` 访问其他 agent 的 workspace 文件。

**Cindy 证据**：
```
find /Users/langgengxin/.slock/agents -name "server.js" -o -name "index.html"
Read /Users/langgengxin/.slock/agents/486673a5/.../prd-pixel-snake.md
```

Cindy 通过文件系统直接读取了 @产品、@后端、@前端 的交付物来验证和写报告。

**Solo 实现方案**：在 daemon 创建 agent workspace 时，将其他 agent 的 workspace 路径通过环境变量或 CLAUDE.md 注入：

```
## Other Agent Workspaces
- @产品: /solo/agents/<产品-id>/
- @后端: /solo/agents/<后端-id>/
- @前端: /solo/agents/<前端-id>/

You can read files from other agents' workspaces to verify their work.
```

**改动文件**：
- `pkg/agent/claude_template.go`: 动态注入其他 agent workspace 路径
- `cmd/daemon/handler.go`: `processTaskWithBackend` 收集 channel agent 列表

**预估**：2-3h

### ✅ P1-5: Agent Stdout 日志带 Agent ID 前缀

**Slock 行为**：`[Agent 63e5dcd8] Process exited with code 143`

**Solo 当前**：`[claude:stderr]` 通用标签

**实现方案**：`stderrTail` 创建时传入 agent ID 作为前缀。

**改动文件**：
- `pkg/agent/claude.go`: `newStderrTail` 接受 agentID 参数

**预估**：0.5h

### ✅ P2-6: CLI 命令补全（核心子集）

**Slock**：26 个命令。**Solo 当前**：9 个。

| Slock 命令 | Solo 等效 | 优先级 |
|-----------|----------|--------|
| slock message read | `solo message read` | P1 |
| slock message search | `solo message search` | P2 |
| slock message react | - | P3 |
| slock task unclaim | `solo task unclaim` (已有) | ✅ |
| slock channel join/leave | - | P3 |
| slock thread unfollow | - | P2 |
| slock attachment upload/view | - | P3 |
| slock profile show/update | - | P3 |
| slock reminder schedule/list | - | P3 |

**优先实施**：`solo message read` — Agent 需要读取线程历史来判断上下文。

**改动文件**：
- `cmd/solo/main.go`: 新增 `message read` 子命令

**预估**：2h

### ✅ P2-7: Thread Unfollow

**Slock 行为**：Agent 在 thread 中完成工作后可以 `slock thread unfollow`，停止接收该 thread 的普通消息投递。

**当前 Solo**：无此机制，agent 会收到所有 thread 消息。

**实现方案**：在 channel_members 或新建 agent_thread_subscriptions 表中记录 unfollow 状态。消息投递时跳过 unfollow 的 thread。

**改动文件**：
- `internal/server/handler/`: 新增 unfollow endpoint
- `cmd/solo/main.go`: 新增 `thread unfollow` 命令
- `internal/server/service/agent.go`: 投递时检查 unfollow 状态

**预估**：3-4h

---

## 三、实施路线

```
Phase 5 (已完成):
  P0-1: Freshness Hold / Draft ──────── 核心体验
  P1-4: Cross-agent Workspace ────────── Cindy 式验收
  P1-5: Agent 日志前缀 ──────────────── 快速改进


  P0-2: Inbox Notification ──────────── 改变消息处理节奏
  P1-3: 消息注入上下文保护 ──────────── 配合 P0-2
  P1-6: solo message read CLI ──────── Agent 读历史能力


  P2-7: Thread Unfollow ────────────── 减少噪音
  P2+: 完整 CLI 补全 ───────────────── 逐步对齐 Slock
```

## 四、case.md 场景覆盖率最终评估

| # | 场景 | Phase 4 状态 | Phase 5 改进后 |
|---|------|-------------|---------------|
| 1 | 新建 agent → 打招呼 | ✅ TriggerAgentGreeting | ✅ + Slock 对齐格式 |
| 2 | @当前 agent 会话 | ✅ mentionedNames + CRITICAL RULES | ✅ |
| 3 | 直接 channel 会话 | ✅ 全量触发 + 自主判断 | ✅ |
| 4 | 私聊 agent | ✅ DM 隔离 | ✅ |
| 5 | @当前 agent asTask | ✅ task 通知含 @mention | ✅ |
| 6 | @其他 agent asTask | ✅ "addressed to OTHER agents" | ✅ |
| 7 | asTask 认领失败 | ✅ DB 行锁 + exit 1 | ✅ |
| 8 | asTask 认领成功 | ✅ /claim #N 解析 | ✅ |
| 9 | thread 直接追问 | ✅ TriggerAgentResponseInThread | ✅ |
| 10 | thread @追问当前 agent | ✅ threadMentionedNames | ✅ |
| 11 | thread 追问其他 agent | ✅ @mention chain 触发 | ✅ |
| 12 | agent 委托其他 agent | ✅ Mode A/B + --parent + OtherAgents | ✅ 机制就绪 |
| 13 | 私聊 asTask | ✅ DM task 隔离 | ✅ |
| + | Cindy 式统筹协调 | ✅ OtherAgents + Freshness Hold | ✅ |
| + | Freshness hold | ✅ QueueIfBusy + holdAndRevise | ✅ |
| + | Inbox notification | ✅ notifyInbox + message check | ✅ |

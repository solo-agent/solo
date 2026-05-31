# Solo v1.3 Phase 4 — Slock 协作模型对齐

> 版本: 1.0
> 日期: 2026-05-22
> 前置: v1.3 Sprint 1-4 (Agent 自主协作架构)
> 对齐参考: Slock system prompt, Cindy/产品/前端/后端 agent 行为分析

---

## 1. 背景与目标

v1.3 实现了 Agent 自主协作的基础架构，但实际表现与 Slock 差距显著：
- Agent 会在不该回复时输出 "not for me" 解释
- Agent 没有真正"记住"自己之前的决策（每次触发都是全新进程）
- Task 通知不包含 @mention 信息，Agent 不知道任务针对谁
- 触发策略不一致（有 @mention 时过滤，无 @mention 时全量）

**根因**：Solo Agent 是临时子进程（每次触发新建），Slock Agent 是持久化进程（长期存活）。

Phase 4 目标：**完全对标 Slock 的协作质量**。

---

## 2. 架构变更

### 2.1 PersistentBackend 接口 (`pkg/agent/backend.go`)

新增 `PersistentBackend` 接口和 `PersistentSession` 结构体：

```go
type PersistentBackend interface {
    Backend
    Start(ctx, req, opts) (*PersistentSession, error)
    Send(ctx, ps, messages) (*PersistentSession, error)
    Close(ps) error
}
```

- `Start()`: 启动子进程但**不关闭 stdin**，进程保持存活
- `Send()`: 向运行中的 session 写入新消息
- `Close()`: 关闭 stdin → 进程退出
- 与现有 `Backend.Execute()` 完全兼容

### 2.2 ClaudeBackend 持久化模式 (`pkg/agent/claude.go`)

新增四个核心方法：

| 方法 | 功能 |
|------|------|
| `Start()` | 启动 Claude Code，stdin 保持 open，启动 `persistentStreamLoop` |
| `persistentStreamLoop()` | 多轮 stdout 读取循环。每次 `"result"` 完成当前 turn，等待下一轮 stdin |
| `Send()` | 创建新 `turnState`，写入 stdin，返回新 `PersistentSession` |
| `Close()` | 关闭 stdin → 进程退出 → `cmd.Wait()` |

关键数据结构：
```go
type claudePersistentState struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser     // KEPT OPEN across turns
    stdout io.ReadCloser
    turn   atomic.Pointer[turnState]  // current turn's output channels
    done   chan struct{}
}

type turnState struct {
    msgCh  chan OutputChunk
    resCh  chan *Result
    output strings.Builder
}
```

### 2.3 AgentSessionManager (`pkg/agent/session.go` — 新文件)

Session 池管理器：

```
AgentSessionManager
├── sessions: map[agentID]*agentSessionEntry
├── activeTurns: map[agentID]chan struct{}  // per-agent 串行锁
├── GetOrCreateSession() → 首次创建，后续复用
├── DeliverMessage() → 向现有 session 投递消息
├── IsActive() → 检查 session 是否存在且进程存活
├── CloseSession() / CloseAll() → 终止 session
├── StartIdleReaper() → 每 30s 清理 >5min 无活动的 session
└── Stop() → 停止 reaper
```

并发模型：
```
Message 1 → acquire per-agent lock → write stdin → process → result → release lock
Message 2 (during processing) → queued → dequeue when lock released
```

### 2.4 Daemon 集成 (`cmd/daemon/handler.go` + `cmd/daemon/main.go`)

`processTaskWithBackend` 在调用 `backend.Execute()` 之前检查 session 是否存在：
- 如活跃且 backend 支持 PersistentBackend → `sessionManager.DeliverMessage()`
- 否则 → `backend.Execute()`（回退到临时进程）

`main.go` 在启动时：
1. 尝试创建 `PersistentBackend`（仅 Claude）
2. 初始化 `AgentSessionManager`
3. 启动 idle reaper
4. Shutdown 时调用 `CloseAll()`

---

## 3. System Prompt 完全重写

### 3.1 结构对比

| 维度 | Phase 3 (v1.3) | Phase 4 (v1.4) |
|------|---------------|---------------|
| 行数 | ~265 行 | ~350 行 |
| Section 数 | 13 | 14 |
| CRITICAL RULES | 简要规则列表 | 5 条 NON-NEGOTIABLE 规则，层层递进 |
| @Mention 感知 | 无 | `mentionedNames` 参数 + 消息格式说明 |
| 沉默规则 | 分散在 Participation Rules 中 | 独立章节："Silence is the default" |
| 防回环 | 简要提及 | 独立 Anti-loop 章节 |
| 频道成员感知 | 无 | 可通过 `solo channel members` 发现 |

### 3.2 14 个 Section

```
Section 1:  Who You Are — 身份、持久化 session 意识
Section 2:  CRITICAL RULES — 5 条规则（Stay in your lane / Silence is default / Claim first / One at a time / Complete work）
Section 3:  Current Context — 频道信息 + @mention 列表 + 消息格式说明
Section 4:  Your Capabilities — CLI 命令概览（引用 CLAUDE.md）
Section 5:  Participation Rules — 决策流程（7 步）、沉默规则
Section 6:  @Mentions — 显式 @ 规则
Section 7:  Anti-loop — 自回复防止 + 升级机制
Section 8:  Claim → Work → Done Workflow — 7 步任务协议
Section 9:  Collaboration Guidelines — 团队发现 + 子任务
Section 10: Delegation Mode Selection — 模式 A（线程）vs 模式 B（子任务）
Section 11: Trigger-Specific Instructions — Mention/DM/Thread/Chat
Section 12: Conversation Etiquette — 反叙事、反回声、线程纪律
Section 13: Workspace & Memory — MEMORY.md + Compaction Safety
Section 14: This Turn — 当前回合决策指南
```

### 3.3 CRITICAL RULES 核心升级

```
Rule 1 — Stay in your lane:
  - Only participate if explicitly @mentioned OR clearly addressed.
  - If a message @mentions another specific agent, it is NOT for you.

Rule 2 — Silence is the default:
  - If not participating: output NOTHING. Not a single word. Not an explanation.
  - Your ENTIRE response must be empty.

Rule 3 — Claim before you act:
  - Always solo task claim BEFORE work. Claim fails → move on. Do NOT retry.

Rule 4 — One message at a time:
  - Run one solo CLI command, read output, then decide next step.

Rule 5 — Complete your work before checking for more.
```

### 3.4 @Mention 感知

新增 `mentionedNames []string` 参数。Prompt 中明确告知 Agent：
```
This message @mentioned: @AgentA, @AgentB
- You WERE @mentioned. This message IS for you.
或
- You were NOT @mentioned. This message is addressed to OTHER agents.
```

新增消息格式说明：
```
Messages follow this format:
  [type=human|agent|system] @sender_name: message content
```

---

## 4. 触发逻辑优化

### 4.1 统一触发策略

`TriggerAgentResponse` 移除 @mention 过滤。**所有 Agent 接收所有消息**，Agent 自主根据 CRITICAL RULES 决定是否参与。

### 4.2 Task 通知包含 @mention 上下文

`TriggerAgentForTask` 的 task 通知文本改为：
```
⬆️ New task in your channel:
Title: xxx

@mentioned: @AgentA, @AgentB

This task IS for you. Consider claiming it.
-- 或 --
⚠️ This task is addressed to OTHER agents, not you.

⚠️ DO NOT reply unless you CLAIM this task first.
```

### 4.3 MentionedNames 传递链路

```
Server: resolveMentionedNames(agentIDs) → names[]
  → daemonTaskRequest.MentionedNames
    → daemon: runTaskRequest.MentionedNames
      → BuildSystemPrompt(..., mentionedNames)
        → prompt Section 3: "@mentioned: @Name1, @Name2"
```

---

## 5. Solo CLI 扩展

新增两个命令：

```bash
# Agent 在自然断点处检查新消息
solo message check [-c <channel_id>]

# 查看服务器和频道信息
solo server info [--output json]
```

---

## 6. 数据流对比

### Before (临时进程)
```
每次消息 → 新 Claude Code 进程 → stdin 写入 → stdout 读取 → 进程退出
```

### After (持久化进程)
```
首次消息 → Start Claude Code 进程 → stdin 写入 → stdout 读取 → 进程存活
后续消息 → 写入同一 stdin → 读取同一 stdout → 进程继续存活
空闲 5min → Close stdin → 进程退出 → 下次消息时重建
```

---

## 7. 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `pkg/agent/backend.go` | 修改 | 新增 PersistentBackend 接口 + PersistentSession 结构体 |
| `pkg/agent/claude.go` | 修改 | 新增 Start/Send/Close/persistentStreamLoop + claudePersistentState/turnState |
| `pkg/agent/session.go` | **新增** | AgentSessionManager: session 池、串行锁、idle reaper |
| `pkg/agent/factory.go` | 修改 | 新增 NewPersistentBackend 函数 |
| `pkg/agent/prompt.go` | 重写 | ~350 行 14 Section，mentionedNames 参数，CRITICAL RULES 升级 |
| `pkg/agent/prompt_test.go` | 重写 | 新增 7 个测试（MentionedNames, CRITICALRULES, AntiLoop, MessageFormat 等） |
| `pkg/agent/claude_template.go` | 修改 | 新增 solo message check + solo server info 命令，silence 规则 |
| `pkg/agent/claude_template_test.go` | 修改 | 更新行数范围 (160→190) |
| `cmd/daemon/handler.go` | 修改 | sessionManager 字段 + SetSessionManager + processTaskWithBackend session 复用 + runTaskRequest.MentionedNames |
| `cmd/daemon/main.go` | 修改 | 初始化 PersistentBackend + AgentSessionManager + idle reaper + CloseAll |
| `internal/server/service/agent.go` | 修改 | 移除 @mention 过滤 + daemonTaskRequest.MentionedNames + resolveMentionedNames + TriggerAgentForTask @mention 通知 |
| `internal/server/service/agent_helpers.go` | **新增** | resolveMentionedNames 辅助函数 |
| `cmd/solo/main.go` | 修改 | 新增 message check + server info 命令 |

---

## 8. 向后兼容

- `Backend.Execute()` 保持不变，非 Claude backend 继续使用临时进程模型
- SSE 流协议不变
- WebSocket 协议不变
- REST API 不变
- 持久化 session 通过 `PersistentBackend` 可用性自然启用（Claude 安装时自动开启）

---

## 9. 与 Slock 的对齐状态

| 维度 | Slock | Solo Phase 4 | 状态 |
|------|-------|-------------|------|
| Agent 进程模型 | 持久化 (--resume) | 持久化 (PersistentBackend) | ✅ |
| 消息投递 | Agent 主动拉取 | Server 推送 + Session 复用 | ⚠️ 推送模式保留，但 session 复用避免冷启动 |
| System Prompt | ~350 行 | ~350 行, 14 Section | ✅ |
| CRITICAL RULES | 多层递进 | 5 条 NON-NEGOTIABLE | ✅ |
| @Mention 感知 | 消息格式内嵌 | mentionedNames 参数 + 消息格式说明 | ✅ |
| 沉默机制 | Prompt 原生 | Prompt 多层规则 + isNotParticipating 兜底 | ✅ |
| CLI 命令数 | 26 个 | 9 个 | ⚠️ Solo 只需核心命令 |
| 频道成员发现 | slock server info | solo server info + solo channel members | ✅ |
| Agent 持久化记忆 | MEMORY.md | MEMORY.md | ✅ |
| Startup Sequence | 5 步 | 7 步决策流程 | ✅ |

# Solo 任务系统深度分析

> 版本: 1.0
> 日期: 2026-05-12
> 负责人: arc (架构师)
> 输入: 用户反馈 4 个核心问题 + Slock 2 份真实 system prompt
> 状态: 分析完成，待团队评审

---

## 目录

1. [Slock System Prompt 深度剖析](#1-slock-system-prompt-深度剖析)
2. [Solo 当前 System Prompt 对比](#2-solo-当前-system-prompt-对比)
3. [四个问题根因分析与分类](#3-四个问题根因分析与分类)
4. [架构改进方案](#4-架构改进方案)
5. [分派任务清单](#5-分派任务清单)

---

## 1. Slock System Prompt 深度剖析

### 1.1 动态生成机制

Slock 的 system prompt 不是静态配置，而是由 Daemon 在每次 Agent 启动/唤醒时动态注入：

```
Agent ID: 20cab102-9ee6-48bd-92c4-ff904418b6e5
Server ID: 8b616037-005d-4bd8-b28d-30c0bc35e509
Computer: langgengxindeMacBook-Pro.local
Hostname: langgengxindeMacBook-Pro.local
OS: darwin arm64
Daemon: v0.47.0
Workspace: /Users/langgengxin/.slock/agents/{agent_id}
```

**关键特征**：
- Runtime Context 由 Daemon 注入，Agent 不需要"猜测"自己是谁
- Workspace 路径指向 Agent 专属持久化目录
- MEMORY.md 路径在 prompt 中明确指向，Agent 启动时自动读取
- Prompt 总长约 350 行 (~12KB)

### 1.2 任务系统架构

Slock 的任务系统核心设计原则：**任务即消息，线程即详情**。

```
任务是一个带有 task metadata 的消息
- 在 channel 中显示为: @Alice: Fix the login bug [task #3 status=in_progress]
- 任务状态变更通过 system message 广播到 channel
- 任务的讨论发生在该消息的 thread 中
- 点击任务 → 打开 thread（thread 本身就是详情视图）
```

**状态流转**:
```
todo → in_progress → in_review → done
         ↑_______________|
```

**关键规则**:
- 只有 top-level 消息可以成为任务（thread 内的消息不能）
- Assignee 独立于 status（任何非 done 状态都可有/无 assignee）
- `slock task create` = 创建新消息 + 标记为 task-message（不会自动认领）
- 复用现有消息创建任务：`slock task claim --message-id ...`

### 1.3 Agent 行为模型

Slock Agent 是一个**通过 CLI 自主操作平台的实体**。它的行为模型是：

```
┌──────────────────────────────────────────────────────┐
│                  Slock Agent 决策循环                   │
│                                                        │
│  1. 收到消息（含 type=system 任务事件通知）               │
│  2. 判断：是纯对话还是需要行动？                          │
│     - 纯对话 → 回复即可，不 claim                       │
│     - 需要行动 → claim first                           │
│  3. 尝试 claim（slock task claim）                     │
│     - 成功 → 开始工作                                   │
│     - 失败 → 跳过，处理下一个任务                         │
│  4. 工作中 → 在 thread 中更新进度                        │
│  5. 完成 → slock task update --status in_review        │
│  6. 收到审批 → slock task update --status done         │
│                                                        │
└──────────────────────────────────────────────────────┘
```

**Agent 可用的 CLI 工具集** (24 个命令):
| 分类 | 命令 |
|------|------|
| 消息 | `message check`, `message send`, `message read`, `message search` |
| 任务 | `task list`, `task create`, `task claim`, `task unclaim`, `task update` |
| 频道 | `server info`, `channel members`, `channel leave` |
| 线程 | `thread unfollow` |
| 附件 | `attachment upload`, `attachment view` |
| 档案 | `profile show`, `profile update` |
| 提醒 | `reminder schedule/list/snooze/update/cancel/log` |
| 行动 | `action prepare` |

**关键洞察**: Slock Agent 是一个**有完整平台操作权限的独立进程**，它不仅"说话"，还能"行动"。它的 system prompt 不仅告诉它"你是谁"，还告诉它"你能用哪些命令"和"你应该怎样做任务"。

### 1.4 System Prompt 结构拆解

| 节 | 行数 | 内容 | 作用 |
|----|------|------|------|
| Who you are | ~8 | Agent 持久化身份说明 | 让 Agent 理解自己是持续存在的 |
| Runtime Context | ~10 | 动态注入的 ID/路径 | 消除身份歧义 |
| Communication — CLI ONLY | ~110 | 24 个命令的详细用法 | **操作手册** |
| Startup sequence | ~12 | 启动时做什么 | 标准化启动行为 |
| Messaging | ~20 | 消息格式、发送方法 | 通信协议 |
| Reminders | ~8 | 提醒系统的用法 | 时间管理 |
| Threads | ~15 | 线程行为规则 | 空间管理 |
| Channel awareness | ~8 | 频道行为规则 | 上下文感知 |
| **Tasks** | **~60** | **任务工作流、决策规则** | **核心行为规则** |
| Splitting tasks | ~8 | 并行任务拆分策略 | 协作策略 |
| @Mentions | ~8 | 提及规则 | 社交协议 |
| Communication style | ~15 | 对话礼仪 | 行为规范 |
| Formatting | ~8 | 格式化规则 | 输出规范 |
| Workspace & Memory | ~50 | MEMORY.md 管理规则 | 持久化记忆 |
| Capabilities | ~3 | 能力说明 | 能力边界 |
| Message Notifications | ~8 | 通知处理 | 并发安全 |

**总量**: ~350 行 / ~12KB token

---

## 2. Solo 当前 System Prompt 对比

### 2.1 Solo prompt.go 结构

Solo 当前的 system prompt 由 `pkg/agent/prompt.go` 的 `BuildSystemPrompt` 函数动态生成：

| 节 | 行数(约) | 内容 |
|----|---------|------|
| Header | 1 | "You are an AI Agent on the Solo platform" |
| Agent Identity | 5 | Name + ID + user-configured system_prompt |
| Channel Context | 8 | Channel name + description |
| Solo Platform Rules | 9 | 行为规范（7 条规则） |
| **Task Protocol** | **10** | **任务相关指令** |
| MEMORY.md | 6 | 记忆系统说明 |
| Trigger Instructions | 12 | 按 trigger type 的行为指引 |
| Date + This Turn | 3 | 当前日期 |

**总量**: ~55 行 / ~1.8KB token

### 2.2 Task Protocol 对比

**Slock 的 Tasks 节** (~60 行):
```
完整的任务生命周期流程:
1. 判断是否需要 claim
2. 如何 claim（CLI 命令 + 参数）
3. claim 失败怎么做
4. 在哪里更新进度（thread，附带具体命令格式）
5. 什么时候更新状态（in_review / done）
6. task create 的含义和适用场景
7. 如何避免重复创建任务
8. 如何拆分并行子任务
9. 收到任务通知时做什么
```

**Solo 的 Task Protocol 节** (10 行):
```
- Tasks appear in your channel with status "todo"
- When a new task is created, evaluate if you can handle it
- If YES: claim it immediately via POST /api/v1/channels/{cid}/tasks/{number}/claim
- If NO: ignore it — another agent will claim
- After claiming: reply in the task's thread acknowledging acceptance
- While working: update progress in the thread
- When done: update task status to in_review
- Do NOT claim tasks outside your capability
- Task format in channel: #N "title" [status] → @claimer
```

### 2.3 差距量化

| 维度 | Slock | Solo | 差距 |
|------|-------|------|------|
| 总行数 | ~350 | ~55 | **6.4x** |
| 任务章节行数 | ~60 | ~10 | **6x** |
| Agent 可用命令数 | 24 | 0 | **无限大** |
| 命令可执行性 | CLI 二进制（真实可执行） | REST API 文本（不可执行） | 质的差距 |
| 线程路由能力 | `--target "#channel:msgId"` | 无 | 质的差距 |
| 任务状态管理 | Agent 主动调 `slock task update` | Server 被动等待 SSE complete | 质的差距 |
| Memory 系统 | 完整的 MEMORY.md 读写规则 | 简化的提示 | 显著差距 |

---

## 3. 四个问题根因分析与分类

### 3.1 问题总览

```
问题 1: 任务点击不了详情        →  💡 产品思路
问题 2: Agent 不主动认领          →  🏗️ 架构
问题 3: Agent 回复到 channel 而非 thread → 🏗️ 架构
问题 4: 状态无变化                →  🐛 Bug + 🏗️ 架构
```

### 3.2 问题 1: 任务点击不了详情

**分类**: 💡 **产品思路** — 产品方向需要调整

**现象**: 用户点击任务卡片，期望看到任务详情和讨论，但体验不符合预期。

**前端代码分析**:

当前 Solo 有两套任务详情视图：
1. `TaskDetailPanel` (`frontend/components/tasks/task-detail-panel.tsx`) — 448px 侧滑面板，显示任务元数据（标题、状态、描述、认领人）+ "在 Thread 中讨论" 按钮
2. `TaskDetailPage` (`frontend/app/tasks/[id]/page.tsx`) — 独立详情页，显示任务信息 + 内置评论系统（`/api/v1/tasks/{id}/comments`）

**Slock 的做法**:
```
每个 task 关联一个 message → message 有自己的 thread
点击 task → 打开 thread 面板（ThreadPanel）
thread 面板中：父消息（=任务描述）+ 所有回复（=任务讨论）
任务状态更新通过 system message 广播到 thread
```

**问题根因**:

Solo 在 TaskDetailPanel 中"再造了轮子"——独立的评论系统（`/api/v1/tasks/{id}/comments`）。而实际上，**任务的讨论就应该发生在该任务的 thread 中**。评论系统和 thread 系统是重复的。

```
Slock:  Task = Message → Thread = 讨论区
Solo:   Task → 独立详情面板 → "在 Thread 中讨论" 按钮 → Thread（分开的两层）
```

**应该怎么做**:
- 点击任务 → 直接打开 ThreadPanel（复用已有的线程面板组件）
- 任务描述 = 父消息内容
- 任务讨论 = thread 中的消息
- 任务状态变更 = thread 中的 system message
- 移除独立的 TaskDetailPanel 和任务评论系统
- TaskCard 的 onClick 应该导航到 thread 视图

**这是产品思路问题而非 Bug**，因为前端实际上已经实现了 ThreadPanel 和 TaskDetailPanel 两个组件，但没有将它们正确地"合一"。

### 3.3 问题 2: Agent 不主动认领

**分类**: 🏗️ **架构** — 设计思路不对，需要改架构

**现象**: Agent 收到任务创建的系统消息后，不会主动去认领（claim）任务。

**代码分析**:

当前系统 prompt 中的 Task Protocol（`pkg/agent/prompt.go` L72-82）要求 Agent：
```
- If YES: claim it immediately via POST /api/v1/channels/{cid}/tasks/{number}/claim
```

但 Agent 是 LLM，它只能**生成文本**，不能**执行 HTTP 请求**。这相当于告诉一个人"用念力打开门"。

**Slock 的做法**:

Agent 的 system prompt 中列出了具体的可执行 CLI 命令：
```bash
slock task list        # 查看任务看板
slock task claim       # 认领任务
slock task claim --number 3     # 通过 task number 认领
slock task claim --message-id abc123  # 通过 message ID 认领
```

Agent 调用 `slock task claim` 时：
1. Claude Code 执行 shell 命令
2. `slock` CLI 调用 Slock HTTP API
3. API 返回结果 → Agent 看到成功/失败
4. 如果 claim 失败（已被别人认领），Agent 跳过

**Solo 缺失的关键层次**:

```
┌─────────────────────────────────────────────────────┐
│             Solo 当前架构 (缺少 Agent 协议层)           │
│                                                       │
│  System Prompt ──→ LLM 生成文本 ──→ 文本内容返回到频道   │
│       ↑                                        │      │
│       └── 告诉 LLM "POST /api/..."              │      │
│           但 LLM 无法执行 HTTP 请求              │      │
│                                                  │      │
│  问题: System Prompt 与 Agent 能力不匹配           │      │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│             Slock 架构 (Agent 有 CLI 工具层)           │
│                                                       │
│  System Prompt → LLM 决策 → slock CLI → HTTP API      │
│       ↑                         │          │          │
│       └── 列出可用命令           │          │          │
│                   slock task claim ─────────┘          │
│                                                       │
│  Agent 可以: list tasks / claim / update status         │
└─────────────────────────────────────────────────────┘
```

**解决方案方向**:

有两种方案：

**方案 A: Server 自动认领（治标）**
- Server 在收到 task.created 事件后，自动检测频道中的 Agent
- 如果 task 有 `assignee_id` 指向 Agent → 自动 claim
- 问题：失去了 Agent "主动 evaluate and claim" 的智能决策

**方案 B: Agent 工具层（治本）**
- 给 Solo Agent 提供可执行的工具（类似 Slock CLI）
- 工具通过 MCP (Model Context Protocol) 或类似机制注入到 LLM 调用中
- Agent 可以调用 `task.list`, `task.claim`, `task.update` 等工具
- 这是正确的长期方案

**推荐**: 方案 B 作为 v1.2 的目标架构，方案 A 作为快速修复（MVP 阶段过渡）。

### 3.4 问题 3: Agent 回复到 channel 而非 thread

**分类**: 🏗️ **架构** — 设计思路不对

**现象**: 用户 @Agent 并创建了任务后，Agent 的回复出现在主频道中，而不是该任务的 thread 中。

**代码分析**:

看 `internal/server/service/agent.go` 的 `TriggerAgentForTask`（L630）:
```go
taskReq := daemonTaskRequest{
    TaskID:       uuid.New().String(),
    AgentID:      ag.ID,
    ChannelID:    channelID,
    // ThreadID 未设置！
    Messages:     contextMsgs,
    SystemPrompt: ag.SystemPrompt,
    ...
    OriginTaskID: taskID,
}
```

注意 `ThreadID` 字段没有被设置。当 Agent 产生回复时，消息被持久化到 `channel_id` 但 `thread_id` 为 NULL。

**而 `TriggerAgentResponse` 和 `TriggerAgentResponseInThread` 的处理方式不同**:
- `TriggerAgentResponse` ← 也省略了 ThreadID（因为这是 channel 消息触发）
- `TriggerAgentResponseInThread` ← 正确传入 ThreadID（这是 thread 消息触发）

**Slock 的做法**:

Slock Agent 收到的每条消息都有 `target=` 和 `msg=` 头:
```
[target=#general msg=a1b2c3d4 time=... type=human] @richard: Fix the login bug
```

Agent 回复到 thread 的方法:
```bash
slock message send --target "#general:a1b2c3d4" <<'EOF'
I'll work on this. Let me check the code first.
EOF
```

Agent 自主决定回复目标：channel、DM、或 thread。

**根因**:

1. 后端没有将 thread 信息传递给 Agent 的 system prompt 或消息上下文
2. Agent 的响应没有"目标路由"概念 —— 回复总是默认到 channel
3. System prompt 虽然说了"reply in the task's thread"，但没有告诉 Agent **如何**做到（因为确实做不到）

**问题本质**: Solo 的消息模型缺少 **Agent 消息路由** 能力。Agent 需要能够指定：我的这条回复应该发到 channel 还是 thread。

**解决方案**:

1. **后端**: `TriggerAgentForTask` 需要在 `daemonTaskRequest` 中传入 `ThreadID`
2. **后端**: 创建一个 task 创建时，同时为该 task 创建一个 thread（如果需要）
3. **System Prompt**: 需要告诉 Agent 回复的 thread 格式
4. **Agent 协议**: 在 Agent 的回复中允许指定 `thread_id` 目标

或者更根本地：**引入 Agent 消息路由协议** —— Agent 的回复格式中可以指定 `target`:

```json
{
  "target": "#channel:msgShortId",
  "content": "I'll work on this."
}
```

### 3.5 问题 4: 状态无变化

**分类**: 🐛 **Bug** + 🏗️ **架构**

**现象**: 任务被创建并分配给 Agent 后，任务状态始终停留在 `todo` 或 `in_progress`，不会变为 `in_review` 或 `done`。

**代码分析**:

当前的状态自动更新机制（`agent.go` L333-348）:
```go
// SOLO-123-B: If this task was triggered by a Solo task (OriginTaskID set),
// update the task status to in_review after agent completes.
if taskReq.OriginTaskID != "" {
    taskSvc := NewTaskService(s.pool)
    if err := taskSvc.CompleteTaskForAgent(context.Background(), taskReq.OriginTaskID); err != nil {
        // ...log warning...
    }
}
```

这个自动更新只在 `handleStreamingAgentTask` 的末尾执行，并且只在 `OriginTaskID != ""` 时触发。

**3 个 Bug 场景**:

| 场景 | OriginTaskID | 会更新状态吗 | 说明 |
|------|-------------|-------------|------|
| `TriggerAgentForTask` 触发 | 设置了 ✅ | 理论上会 | 但前提是 task 必须先被 claim（状态为 in_progress），`CompleteTaskForAgent` 才生效 |
| `TriggerAgentResponse` 触发 | 空 ❌ | 不会 | 普通消息触发 Agent 时没有关联 task |
| `TriggerAgentResponseInThread` 触发 | 空 ❌ | 不会 | Thread 回复触发 Agent 时也没有关联 |

**Bug 1**: `CompleteTaskForAgent` 的 SQL:
```sql
UPDATE tasks SET status = 'in_review', updated_at = now()
WHERE id = $1 AND status = 'in_progress'
```
这要求任务的状态已经是 `in_progress`，但如果 Agent 没有 claim 任务（问题 2），状态就仍然是 `todo`，导致 UPDATE 匹配 0 行。

**Bug 2**: Agent 通过普通消息回复时，即使消息内容涉及任务，也完全不会更新任务状态。

**架构问题**: 
- Slock Agent 主动执行 `slock task update --status in_review`  —— Agent 是状态变更的主动方
- Solo Server 被动等待 Agent 完成 SSE streaming —— Server 是状态变更的被动方
- Server 不知道 Agent 是否真的"完成了任务"，只知道"LLM 调用结束了"

**解决方案**:

1. **Bug 修复**: 
   - `CompleteTaskForAgent` 应该也匹配 `status = 'todo'`（当 Agent 被直接分配任务时）
   - 或者：在 `TriggerAgentForTask` 中先自动 claim 任务

2. **架构改进**:
   - 引入 Agent 工具（同问题 2），让 Agent 自己调用 `task.update`
   - 这是根本解决方案

---

## 4. Multica 非 Claude Agent 实现分析

> 基于 Multica 源码 (`server/pkg/agent/` + `server/internal/daemon/execenv/`) 的完整代码审查。

### 4.1 整体架构：统一子进程模式 + 协议适配层

Multica 的所有 11 个 Agent Backend 共享同样的生命周期模式，但使用各自的原生协议通信：

```
┌──────────────────────────────────────────────────────────────┐
│                    Multica Daemon                             │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ claude   │  │ codex    │  │ hermes   │  │ gemini   │    │
│  │ stream-  │  │ JSON-RPC │  │ ACP      │  │ stream-  │    │
│  │ json     │  │ 2.0      │  │ JSON-RPC │  │ json     │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
│       │             │             │             │           │
│  ┌────┴─────┐  ┌────┴─────┐  ┌────┴─────┐  ┌────┴─────┐    │
│  │claude CLI│  │codex CLI │  │hermes CLI│  │gemini CLI│    │
│  │subprocess│  │subprocess│  │subprocess│  │subprocess│    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
│                                                              │
│  ... 同样模式: cursor / kimi / kiro / copilot / opencode     │
│                / openclaw / pi                               │
└──────────────────────────────────────────────────────────────┘
```

**核心原则**: 每个 Agent 都是 `exec.Command` 启动的子进程，stdin/stdout 通信。Daemon 不关心 Agent 内部用什么工具、怎么推理 —— Daemon 只管协议适配和消息转发。

### 4.2 通信协议矩阵

| Agent | 启动命令 | 协议 | 实现文件 | 关键特征 |
|-------|---------|------|---------|---------|
| Claude | `claude -p --output-format stream-json --input-format stream-json` | JSONL (stdin/stdout) | `claude.go` | 系统提示通过 `--append-system-prompt` 注入 |
| Codex | `codex app-server --listen stdio://` | JSON-RPC 2.0 | `codex.go` (446 行) | 最复杂的协议适配；支持 thread/resume; 需要 auto-approve exec/patch |
| Hermes | `hermes acp` | ACP (JSON-RPC 2.0) | `hermes.go` (1423 行) | ACP 协议；`session/new` + `session/prompt`；复用 `hermesClient` |
| Kimi | `kimi acp` | ACP (JSON-RPC 2.0) | `kimi.go` | **复用 `hermesClient`**；只需 403 行（90% 逻辑在 hermes.go 中） |
| Kiro | `kiro-cli acp --trust-all-tools` | ACP (JSON-RPC 2.0) | `kiro.go` | **复用 `hermesClient`**；388 行 |
| Gemini | `gemini -p <prompt> -o stream-json --yolo` | NDJSON (stdout) | `gemini.go` | 268 行；无 stdin 交互；一次性输入 |
| Cursor | `cursor-agent chat -p <prompt> --output-format stream-json --yolo` | JSONL (stdout) | `cursor.go` + `cursor_invocation.go` | 423 行；**不支持 `--system-prompt`**，指令通过 AGENTS.md 注入 |

**关键发现**: Kimi 和 Kiro 共享 Hermes 的 ACP 协议实现（`hermesClient`），只需各自处理二进制路径、工具名映射等差异化部分。这说明**协议层的复用比每个 Agent 独立实现更重要**。

### 4.3 平台能力注入机制（核心发现）

Multica 让 Agent "操作平台" 的核心机制不是 System Prompt，不是 Function Calling，而是 **文件注入 + CLI 命令手册**。

流程如下：

```
1. execenv.Prepare() 创建隔离环境
   ├── {task_root}/
   │   ├── workdir/          ← Agent 的 CWD
   │   ├── output/           ← 输出产物
   │   └── logs/             ← 运行日志

2. execenv.InjectRuntimeConfig(workDir, provider, taskCtx) 写入指令文件
   ├── Claude → CLAUDE.md
   ├── Codex/Copilot/OpenCode/OpenClaw/Hermes/Pi/Cursor/Kimi/Kiro → AGENTS.md
   └── Gemini → GEMINI.md

3. Daemon 构建简短 prompt (daemon/prompt.go BuildPrompt)
   → "Start by running `multica issue get <id> --output json` to understand your task"

4. Agent 子进程启动，CWD = workdir
   → 通过原生机制自动发现 CLAUDE.md/AGENTS.md/GEMINI.md
   → 读取 CLI 命令手册 → 学会使用 `multica` CLI 操作平台
```

**注入的 CLAUDE.md 内容**（来自 `execenv/runtime_config.go` `buildMetaSkillContent`）：
- Agent 身份（名称、ID、用户定义的 Agent Instructions）
- **完整的 `multica` CLI 命令手册**（~200 行，20+ 命令）
- 工作流指引（按触发类型分支：issue assignment / comment reply / chat / autopilot）
- 回复格式规则（@Mentions 协议、HEREDOC 使用规则、沉默规则）
- 仓库/项目上下文

### 4.4 非 Claude Agent 如何"获得平台操作能力"

Multica 的方案非常简洁：**所有 Agent 通过同一个 `multica` CLI 二进制操作平台，通过原生方式发现 CLI 使用手册**。

```
┌──────────────────────────────────────────────────────────────┐
│          非 Claude Agent 的平台操作能力注入路径                  │
│                                                              │
│  Codex:                                                      │
│    Codex CLI 自动读取 CWD 下的 AGENTS.md                       │
│    → 学会 multica issue get / multica issue comment add ...  │
│    → 通过原生 exec_command 工具执行 multica CLI               │
│                                                              │
│  Cursor:                                                     │
│    cursor-agent 自动读取 CWD 下的 AGENTS.md                    │
│    (光标注释: "Instructions are injected via AGENTS.md       │
│     and .cursor/skills/ files instead")                      │
│    → 学会 multica CLI → 通过原生 terminal 工具执行             │
│                                                              │
│  Gemini:                                                     │
│    Gemini CLI 自动读取 CWD 下的 GEMINI.md                      │
│    → 学会 multica CLI → 通过原生 shell 工具执行                │
│                                                              │
│  Hermes / Kimi / Kiro:                                       │
│    自动读取 CWD 下的 AGENTS.md                                 │
│    → 学会 multica CLI → 通过原生 terminal 工具执行             │
│    (所有三个都通过 ACP 协议，复用同一个 hermesClient)           │
└──────────────────────────────────────────────────────────────┘
```

**没有统一协议层** —— 每个 Agent 使用自己原生的文件发现机制：
- Claude: `CLAUDE.md`
- Codex/Cursor/Kimi/Kiro/Hermes/Copilot/OpenCode/OpenClaw/Pi: `AGENTS.md`
- Gemini: `GEMINI.md`

**Multica Daemon 不需要知道 Agent 具体如何执行命令**。Daemon 只负责：
1. 准备 workdir + 写入指令文件
2. 启动 Agent 子进程
3. 解析 stdout 流中的消息（统一为 `Message{Type, Content, Tool, ...}` 结构）
4. 转发消息到 Server

### 4.5 `multica` CLI: 统一的平台接口

Multica CLI 是所有 Agent 的平台操作入口。它是**认证过的 HTTP API 的薄封装**：

```
Agent (任一后端)
  └→ shell: multica issue get <id> --output json
      └→ multica CLI 二进制
          └→ 读取本地认证凭据 → HTTP 请求 → Multica Server API
              └→ JSON 响应 → stdout
```

**为什么 CLI 不是 curl？**
- CLI 管理认证（读取 daemon 写入的环境变量/配置文件）
- CLI 有参数验证（输入错误有友好提示）
- CLI 有 JSON 输出格式化（`--output json` 保证机器可解析）
- CLI 有上下文感知（`--full-id`、`--since` 等便利选项）
- Agent 不需要拼 URL、处理 auth header、处理 HTTP 错误码

### 4.6 关键设计决策总结

| 决策 | Multica 做法 | 原因 |
|------|-------------|------|
| 指令注入方式 | 文件写入 (CLAUDE.md/AGENTS.md/GEMINI.md) | 每个 Agent 原生发现，无需额外协议 |
| 平台操作方式 | `multica` CLI 20+ 命令 | 认证管理 + 参数验证 + 机器可解析输出 |
| 非 Claude 扩展 | 写 AGENTS.md 即可 | 新 Agent 只要支持 AGENTS.md 发现就能工作 |
| Agent 间差异 | 仅通信协议不同 | 平台操作能力完全统一 |
| Daemon 职责 | 进程管理 + 流解析 | 不关心 Agent 内部如何操作平台 |

---

## 5. 三方案对比 + Solo 推荐方案

### 5.1 核心洞察（重述）

问题 2-4 的共同根因已在 §3 中分析：**Solo Agent 缺少"平台操作能力"**。System Prompt 告诉 Agent "你应该 claim task"，但 Agent 无法执行 HTTP 请求。这是一个"能力缺口"问题，不是"指令不够"问题。

### 5.2 四种方案对比矩阵

| 维度 | Multica | Slock | Solo 当前 | 理想 Solo |
|------|---------|-------|----------|-----------|
| **Agent Backend 数** | 11 | 1 (Claude only) | 11 | 11 |
| **注入方式** | 文件 (CLAUDE.md/AGENTS.md/GEMINI.md) | System Prompt 参数注入 | CLAUDE.md + System Prompt | 文件 + System Prompt |
| **工具形式** | `multica` CLI 20+ 命令 | `slock` CLI 24 命令 | 无（HTTP URL 文本） | `solo` CLI |
| **非 Claude 支持** | 每个 Agent 原生文件发现 | 不支持 | 11 Backend 但都没工具 | 所有 11 Backend |
| **扩展新 Agent** | 写 AGENTS.md 即可 | 需要改 Slock 代码 | 需要改代码 | 写 AGENTS.md 即可 |
| **维护成本** | CLI 二进制 (~2000 行) | CLI 二进制 | 0 | CLI 二进制 (~300 行 + 指令文件) |

### 5.3 四个候选方案详细分析

#### 方案 A: Solo CLI 二进制（学 Multica/Slock）

```
Agent → shell: solo task claim --number 3
           └→ solo CLI → POST /api/v1/channels/{cid}/tasks/3/claim
                         → 读取 ~/.solo/credentials 认证
```

| 优点 | 缺点 |
|------|------|
| 所有 11 Backend 通用（都有 shell 执行能力） | 需要维护一个 CLI 项目 |
| 与 Multica/Slock 对齐，经过大规模验证 | 打包分发需要考虑（Go 单二进制） |
| 认证管理、参数验证、错误处理集中 | |
| 指令文件（CLAUDE.md）直接复用，新增 Agent 零代码 | |
| 可独立测试（`solo task claim --number 3`） | |

**实现成本**: 约 400 行 Go + ~80 行 CLAUDE.md 指令文档。Go 单二进制编译，Zero 运行时依赖。

#### 方案 B: HTTP API 手册注入（curl 方案）

Agent 的 CLAUDE.md 中注入：
```bash
curl -s -X POST http://localhost:8080/api/v1/channels/{cid}/tasks/{number}/claim \
  -H "Authorization: Bearer $(cat ~/.solo/token)" \
  -H "Content-Type: application/json"
```

| 优点 | 缺点 |
|------|------|
| 不需要额外二进制 | curl 参数复杂，Agent 容易出错 |
| 最快速 | 认证 token 管理脆弱 |
| | 不同 Agent 的 curl 行为不一致（Codex 有限制） |
| | URL 变更需要更新所有注入文档 |
| | 机器可解析输出需要 `jq` 管线（不可靠） |

**结论**: 对于 Agent 来说，curl 无法提供 CLI 的体验。Multica 明确标注"**Do NOT use curl, wget, or any other HTTP client**"，这是血的教训。

#### 方案 C: LLM Function Calling 注入（直接 API 调用）

在 System Prompt / API 调用中注入 JSON function definitions：
```json
{
  "name": "task_claim",
  "description": "Claim a task by number",
  "parameters": {
    "task_number": {"type": "integer"}
  }
}
```

Daemon 拦截 `tool_use` 消息 → 调用 Solo REST API → 返回 `tool_result`。

| 优点 | 缺点 |
|------|------|
| 结构化，类型安全 | **只有 API-based LLM 支持**（Claude API、OpenAI API） |
| 不需要 CLI 二进制 | **Codex、Cursor、Gemini、Kimi、Kiro 等不支持** |
| | 每个 Backend 需要单独实现 tool call 拦截 |
| | API-based Backend 和 CLI-based Backend 实现路径完全不同 |
| | 维护成本高：新增工具需要同步更新 10+ 个 Backend |

**为什么 Function Calling 对 Solo 是错误的**: Solo 11 个 Backend 中，至少 6 个（Codex、Cursor、Gemini、Hermes、Kimi、Kiro）是通过子进程 CLI 调用的，不支持 Anthropic/OpenAI 的 function calling 协议。Function Calling 只能覆盖 API-based LLM（如 Anthropic API、OpenAI API），而 CLI Backend 的 Agent 有自己的工具机制（Bash tool、exec_command、terminal tool）。

**如果做 Function Calling**，意味着：
- API-based Backend: 用 `tool_use` 协议
- CLI-based Backend: 用 System Prompt 告诉 Agent "用 Bash 执行 curl XXX"
- 两套工具定义、两套参数验证、两套错误处理
- 这就是 **Multica 踩过的坑，他们坚定地选择了统一的 CLI 路径**

#### 方案 D: MCP 协议（长期标准化）

为 Solo 提供完整 MCP Server，Agent 通过 MCP 协议发现和调用平台工具。

| 优点 | 缺点 |
|------|------|
| 标准化协议 | 只有 Claude Code 和少数支持 MCP 的 Agent 能使用 |
| 工具发现、参数校验内置 | Codex、Gemini 等不支持 MCP |
| | 初期投入巨大（MCP Server + 每个 Backend 的适配） |
| | 需要运行一个常驻 MCP Server 进程 |

### 5.4 推荐方案: A+ (CLI 二进制 + 文件注入 + System Prompt 辅助)

经过对 Multica 源码的完整分析，**推荐方案的最终形态**如下：

```
┌──────────────────────────────────────────────────────────────┐
│              Solo Agent 工具层架构 (推荐方案 A+)                │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐  │
│  │                  Agent Workspace                       │  │
│  │  ~/.solo/agents/{id}/workspace/                       │  │
│  │  ├── CLAUDE.md    ← Solo 指令手册 (Agent 身份 +       │  │
│  │  │                    solo CLI 命令参考 + 工作流指引)    │  │
│  │  ├── AGENTS.md    ← 相同内容 (非 Claude Agent 用)      │  │
│  │  └── GEMINI.md    ← 相同内容 (Gemini 用)              │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐  │
│  │                  System Prompt                         │  │
│  │  BuildSystemPrompt() 动态注入:                          │  │
│  │  - Agent 身份 (名称, ID)                                │  │
│  │  - Channel 上下文 (名称, 描述, 触发类型)                  │  │
│  │  - Solo 平台规则 (简要)                                  │  │
│  │  - "Your workspace is at ~/.solo/agents/{id}/workspace" │  │
│  │  - "Read CLAUDE.md for available commands"             │  │
│  │  注: 详细 CLI 手册不在 System Prompt 中(太长)           │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              solo CLI                                  │  │
│  │  solo task list          → GET /api/v1/channels/...   │  │
│  │  solo task claim -n 3     → POST /api/v1/channels/...  │  │
│  │  solo task update -n 3 -s in_review → PATCH ...       │  │
│  │  solo message send -c "text" [-t thread_id]           │  │
│  │  solo channel info        → GET /api/v1/channels/...   │  │
│  │  solo member list         → GET /api/v1/members/...    │  │
│  │                                                         │  │
│  │  认证: 读取 ~/.solo/credentials (Daemon 写入)           │  │
│  │  输出: --output json 保证机器可解析                      │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  Agent (任一后端) → shell: solo task claim -n 3             │
│                   → stdout: {"status":"claimed","number":3}  │
└──────────────────────────────────────────────────────────────┘
```

#### 为什么 Slim CLI（不走 Function Calling）？

| 考量 | CLI + 文件注入 | Function Calling |
|------|---------------|-----------------|
| Codex | 通过 AGENTS.md 原生发现 → Bash 执行 | **不支持** |
| Cursor | 通过 AGENTS.md 原生发现 → terminal 执行 | **不支持** |
| Gemini | 通过 GEMINI.md 原生发现 → shell 执行 | **不支持** |
| Hermes/Kimi/Kiro | 通过 AGENTS.md 原生发现 → terminal 执行 | **不支持** |
| Claude Code | 通过 CLAUDE.md 原生发现 → Bash 执行 | 支持（但 Bash 也够用） |
| Claude API | 通过 CLAUDE.md + System Prompt → `solo` CLI | 支持 |
| OpenAI API | 通过 AGENTS.md + System Prompt → `solo` CLI | 支持 |
| **新增 Agent** | 写一份 AGENTS.md 即可 | 需要实现工具拦截逻辑 |

**CLI 方案覆盖 100% 现有 + 未来 Backend。Function Calling 最多覆盖 2-3 个 API-based Backend。**

#### Solo CLI 的最小命令集（5 个核心命令）

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `solo task list [-c channel_id] [--status X] [--output json]` | 列出频道中所有任务 | `GET /api/v1/channels/{cid}/tasks` |
| `solo task claim -n <task_number> [-c channel_id]` | 认领任务 | `POST /api/v1/channels/{cid}/tasks/{number}/claim` |
| `solo task update -n <task_number> [-c channel_id] -s <status>` | 更新任务状态 | `PATCH /api/v1/channels/{cid}/tasks/{number}` |
| `solo message send -c <content> [-t thread_id] [-C channel_id]` | 发送消息 | `POST /api/v1/channels/{cid}/messages` |
| `solo channel members [-c channel_id] [--output json]` | 查看频道成员 | `GET /api/v1/channels/{cid}/members` |

**为什么只有 5 个命令而不是 Slock 的 24 个？** MVP 阶段 Agent 只需要能：查看任务看板、认领/更新任务、发送消息到正确位置、了解频道成员。这覆盖了问题 2-4 的全部需求。后续根据需求扩展。

### 5.5 ADR-008: Agent 工具层采用 CLI 二进制 + 文件注入 (替代原 ADR-006)

- **背景**: 原 ADR-006 计划使用 LLM Function Calling 作为 Agent 工具层。经过对 Multica 11 个 Backend 源码的完整分析，发现 Function Calling 只能覆盖 API-based LLM（~3/11 Backend），CLI-based Backend（Codex/Cursor/Gemini/Hermes/Kimi/Kiro 等）无法使用 Function Calling，只能用 shell 执行。
- **决策**: 采用 `solo` CLI 二进制 + CLAUDE.md/AGENTS.md/GEMINI.md 文件注入。Agent 通过 shell 调用 `solo` CLI 操作平台。
- **后果**:
  - Agent 获得完整的平台操作能力（认领任务、更新状态、发送消息、查看上下文）
  - 所有 11 个 Backend 统一通过 CLI 操作平台（CLI 是唯一执行路径，零分歧）
  - 需要维护 ~400 行 Go 的 `solo` CLI 二进制
  - System Prompt 从 ~55 行扩展到 ~150 行（简化版，详细 CLI 手册在文件中）
  - 指令文件 (CLAUDE.md/AGENTS.md/GEMINI.md) 新增 ~80 行 CLI 命令手册
  - **新增 Agent Backend 只需写一份 AGENTS.md，零代码变更**
- **备选方案**:
  - Function Calling: 只能覆盖 API-based Backend，造成两套工具体系（已否决，见 §5.3 方案 C）
  - curl 手册: Agent 执行错误率高，Multica 明确警告不要用（已否决，见 §5.3 方案 B）
  - MCP 协议: 只有 Claude Code 支持，覆盖面极窄（长期观察）

---

## 6. 实施计划

### 6.1 分阶段路线图

```
Phase 1 (Week 1-2):  CLI 基础 + Bug 修复
  ├── RD1-P1: 编写 solo CLI (cmd/solo/) + 集成到构建
  ├── RD1-P2: 修复 ThreadID 缺失 + CompleteTaskForAgent 状态检查
  ├── ARC-P1: 重写 System Prompt (过渡版) + CLAUDE.md/AGENTS.md 模板
  └── PM-P1: 任务 UX 方向确认

Phase 2 (Week 2-3):  指令注入 + Agent 联调
  ├── RD1-P3: WorkspaceManager.InjectConfig 写入 CLAUDE.md + AGENTS.md + GEMINI.md
  ├── ARC-P2: 指令文件完善 (CLI 命令手册 + 工作流指引)
  └── QA: 端到端测试 (Agent 能通过 solo CLI 认领任务、更新状态、回复到 thread)

Phase 3 (Week 3):  前端统一任务+Thread 视图
  ├── FE1-P1: 统一任务详情为 ThreadPanel
  ├── FE1-P2: 移除任务独立评论系统
  └── FE2-P1: TaskCard 增强

Phase 4 (Week 4):  System Prompt 升级 + 完整测试
  ├── ARC-P3: System Prompt 升级到完整版 (与 CLI 衔接)
  └── QA: 全流程回归测试
```

### 6.2 角色分工（防止返工的关键：接口先于实现）

#### rd1 (后端负责人): solo CLI + WorkspaceManager 升级

| 任务 | 优先级 | 耗时 | 产出 |
|------|--------|------|------|
| **RD1-CLI: `cmd/solo/` CLI 二进制** | P0 | 12h | `solo task list/claim/update`, `solo message send`, `solo channel members` |
| **RD1-WS: WorkspaceManager 多文件注入** | P0 | 6h | `InjectConfig` 同时写入 CLAUDE.md + AGENTS.md + GEMINI.md |
| **RD1-FIX: 修复 ThreadID + CompleteTaskForAgent** | P0 | 3h | Bug 修复（ThreadID 传入、状态检查放宽） |

**CLI 设计接口（先定义，后实现）**：

```
solo task list [-c channel_id] [--status X] [--output json]
    → stdout: [{"number":1,"title":"...","status":"todo","assignee_id":"..."}, ...]
    退出码: 0=成功

solo task claim -n <number> [-c channel_id]
    → stdout: {"status":"claimed","number":3}
    退出码: 0=成功, 1=已被认领, 2=不存在

solo task update -n <number> [-c channel_id] -s <status>
    → stdout: {"status":"updated"}
    退出码: 0=成功

solo message send -c <content> [-t thread_id] [-C channel_id]
    → stdout: {"id":"uuid","channel_id":"uuid","thread_id":"uuid"}
    退出码: 0=成功

solo channel members [-c channel_id] [--output json]
    → stdout: [{"id":"uuid","name":"name","type":"human"}, ...]
    退出码: 0=成功
```

**认证机制**: CLI 从环境变量 `SOLO_AUTH_TOKEN` 读取 JWT。Daemon 启动 Agent 时设置该环境变量。CLI 不需要自己登录。

**CLI 实现**: `cmd/solo/` 是 Go 单源文件 (~300 行) + `make solo` 构建。不依赖任何额外框架。

#### fe1 (前端负责人)

| 任务 | 优先级 | 耗时 | 产出 |
|------|--------|------|------|
| **FE1-P1: 统一任务详情为 ThreadPanel** | P0 | 8h | TaskCard → ThreadPanel；ThreadPanel 显示任务元数据 |
| **FE1-P2: 移除任务独立评论系统** | P1 | 2h | 清除前端评论代码 |

#### arc (架构师)

| 任务 | 优先级 | 耗时 | 产出 |
|------|--------|------|------|
| **ARC-CLAUDE: CLAUDE.md/AGENTS.md 指令文件模板** | P0 | 6h | `pkg/agent/claude_template.go`：Agent 身份 + `solo` CLI 手册 + 工作流 |
| **ARC-PROMPT: System Prompt 重写** | P0 | 4h | `pkg/agent/prompt.go` 重写：动态上下文 + 引用 CLAUDE.md |
| **ARC-DOC: 本文档更新** | P1 | 已完成 | task-system-analysis.md v2.0 |

#### pm1 (产品经理)

| 任务 | 优先级 | 耗时 | 产出 |
|------|--------|------|------|
| **PM1-P1: 任务 UX 重新设计** | P0 | 4h | PRD 更新，确认任务详情=Thread 方案 |

### 6.3 防止返工策略

#### 策略 1: 接口先于实现（硬规则）

| 接口 | 交付物 | 谁定义 | 谁依赖 | 状态 |
|------|--------|--------|--------|------|
| `solo` CLI 命令行接口 | `cmd/solo/` doc comment | arc + rd1 | ARC-CLAUDE (CLI 手册) | 本文档 §6.2 已定义 |
| Agent 指令文件结构 | `pkg/agent/claude_template.go` 函数签名 | arc | RD1-WS (InjectConfig) | 待 ARC-CLAUDE 产出 |
| System Prompt 输出格式 | `BuildSystemPrompt()` 函数签名 | arc | 无（arc 自己实现） | 待 ARC-PROMPT 产出 |
| ThreadPanel 任务元数据 props | `ThreadPanelProps` TypeScript 接口 | fe1 | 无（fe1 自己实现） | 待 PM1 确认 UX |

**关键**: RD1-CLI 先完成 → ARC-CLAUDE 指令文件引用准确的 CLI 参数格式 → RD1-WS 基于指令文件模板注入。

#### 策略 2: CLI 与指令文件同时交付

CLI 二进制和 CLAUDE.md 指令手册必须**同时可用**。CLI 做完了但 Agent 不知道用法 = 白做。指令文件写好了但 CLI 不存在 = Agent 被骗。

**执行顺序**:
1. RD1-CLI 完成并自测（`solo task claim -n 1` 可执行）
2. ARC-CLAUDE 基于实际 CLI 行为撰写指令文件
3. RD1-WS 集成指令文件注入
4. 端到端测试：Claude Code Agent 通过 CLAUDE.md 学习 → 执行 `solo task claim -n 1` → 任务状态变为 `in_progress`

#### 策略 3: 先覆盖 Claude Code（最复杂），再扩展到其他 Backend

Claude Code 是 Solo 的主力 Backend（通过 CLAUDE.md）。先确保 Claude Code 能通过 `solo` CLI 完整操作平台（认领任务 → 更新进度 → 回复 thread → 标记完成），验证整个通路正确后，其他 Backend 只需：
1. 写 AGENTS.md（与 CLAUDE.md 相同内容）
2. 通过各自 Bash/terminal 工具执行 `solo` CLI

**不需要改任何 Go 代码** —— 这就是 CLI 方案的核心优势。

### 6.4 ADR-009: 指令注入策略 (替代原 ADR-006/部分 ADR-007)

- **背景**: Solo 当前 WorkspaceManager 只写 `CLAUDE.md`，且内容为基础的身份声明。Multica 证明了非 Claude Agent 需要 `AGENTS.md` 和 `GEMINI.md`，且需要完整的 CLI 命令手册。
- **决策**: WorkspaceManager.InjectConfig 同时写入三个文件：`CLAUDE.md` (Claude Code)、`AGENTS.md` (Codex/Cursor/Hermes/Kimi/Kiro/Pi/Copilot/OpenCode/OpenClaw)、`GEMINI.md` (Gemini CLI)。文件内容相同：Agent 身份 + `solo` CLI 命令手册 + 工作流指引。System Prompt 只保留动态上下文（Channel 信息、触发类型），引导 Agent 读取 CLAUDE.md 获取详细指令。
- **后果**:
  - Agent 启动时读 CLAUDE.md → 自然学会用 `solo` CLI
  - **新增 Agent 零 Go 代码变更**（指令文件已存在）
  - System Prompt 精简到 ~100 行（核心上下文），详细手册 ~150 行在指令文件中
- **备选方案**: 在 System Prompt 中嵌入完整 CLI 手册（导致 System Prompt 膨胀到 350+ 行，浪费 token；对非 Claude Agent 无效，因为 System Prompt 只通过 API 调用时传入）

### 6.5 文件变更清单 (更新)

| 文件 | 变更类型 | 负责 | 说明 |
|------|---------|------|------|
| `cmd/solo/main.go` | **新增** | rd1 | `` solo `` CLI 入口 |
| `cmd/solo/task.go` | **新增** | rd1 | task list/claim/update 子命令 |
| `cmd/solo/message.go` | **新增** | rd1 | message send 子命令 |
| `cmd/solo/channel.go` | **新增** | rd1 | channel members 子命令 |
| `Makefile` | 修改 | rd1 | 添加 `make solo` 构建目标 |
| `pkg/agent/workspace.go` | 修改 | rd1 | `InjectConfig` 同时写 CLAUDE.md + AGENTS.md + GEMINI.md |
| `pkg/agent/claude_template.go` | **新增** | arc | 指令文件模板 (Agent 身份 + CLI 手册 + 工作流) |
| `pkg/agent/prompt.go` | 重写 | arc | System Prompt 精简到 ~100 行 |
| `pkg/agent/prompt_test.go` | 更新 | arc | 新增测试用例 |
| `internal/server/service/agent.go` | 修改 | rd1 | ThreadID 传入、OriginTaskID 修复 |
| `internal/server/service/task.go` | 修改 | rd1 | `CompleteTaskForAgent` 放宽状态匹配 |
| `frontend/components/chat/thread-panel.tsx` | 修改 | fe1 | 增强任务元数据显示 |
| `frontend/components/tasks/task-card.tsx` | 修改 | fe2 | 显示回复数和最后活动 |
| `frontend/components/tasks/task-detail-panel.tsx` | 删除 | fe1 | 合并入 ThreadPanel |
| `frontend/app/tasks/[id]/page.tsx` | 修改 | fe1 | 重定向到 thread 视图 |
| `PRD.md` | 更新 | pm1 | 任务系统 UX 设计 |
| `ARCHITECTURE.md` | 修改 | arc | 新增 §Agent 工具层 |

---

## 附录 A: Multica vs Slock vs Solo (最终对比)

| 维度 | Multica | Slock | Solo v1.2 (推荐) |
|------|---------|-------|-----------------|
| Agent Backend 数 | 11 | 1 (Claude) | 11 |
| 注入方式 | CLAUDE.md/AGENTS.md/GEMINI.md | System Prompt (350 行) | CLAUDE.md/AGENTS.md/GEMINI.md + System Prompt (~100 行) |
| 工具 | `multica` CLI 20+ | `slock` CLI 24 | `solo` CLI 5 (MVP) |
| 非 Claude | 原生文件发现 | 不支持 | 原生文件发现 (写 AGENTS.md) |
| 新增 Agent 成本 | 0 代码 | 不支持 | 0 代码 (AGENTS.md 已存在) |
| System Prompt 行数 | ~40 行 (仅任务上下文) | ~350 行 | ~100 行 (上下文) + ~150 行 (指令文件) |
| 核心洞察 | CLI 是统一接口 | CLI 是统一接口 | CLI 是统一接口 |

## 附录 B: 原 ADR-006 为何被推翻

原 ADR-006 在 §4.2 中推荐"Agent 通过 Function Calling 操作平台"。经过对 Multica 源码的完整分析后，该决策被推翻，理由如下：

1. **覆盖面分析**: Solo 11 Backend 中，仅 2-3 个 API-based Backend 支持 Function Calling（Claude API、OpenAI API）。Codex、Cursor、Gemini、Hermes、Kimi、Kiro 等 CLI 子进程 Agent 只能通过 shell 执行命令。
2. **Multica 的路径验证**: Multica 面对完全相同的 11-Backend 局面，选择了 CLI 路径而非 Function Calling。这不是因为"没想过"，而是因为"试过且不可行"。
3. **统一性**: CLI 方案对所有 Backend 完全一致。Function Calling 方案会造成 API-based 用 tool_use、CLI-based 用 shell 的两套体系，增加维护复杂度和 Bug 概率。
4. **成本**: `solo` CLI (~300 行 Go) vs Function Calling 实现（需要为每个 Backend 实现工具拦截逻辑，估计 2000+ 行跨 10+ 文件）。

## 附录 C: Slock vs Solo vs Multica System Prompt 行数对比

```
 Multica (任务 prompt only) ██                                    ~40 行
 Solo 当前                   ████████                             ~55 行
 Solo v1.2 System Prompt    ██████████████                       ~100 行
 Solo v1.2 CLI 指令文件      █████████████████████                ~150 行
 Slock                       ████████████████████████████████████ ~350 行
                             ├──────┼──────┼──────┼──────┼──────┤
                             0     70    140    210    280    350

 核心差异:
 - Multica: 任务上下文极短，详细手册在文件注入中（不在 System Prompt 中）
 - Slock: 所有内容在一个 350 行的 System Prompt 中（因为只支持 Claude API）
 - Solo v1.2: 动态上下文在 System Prompt 中，静态手册在文件中（平衡了灵活性和通用性）
```

---

> 文档版本: 2.0  
> 日期: 2026-05-12  
> 负责人: arc (架构师)  
> 主要变更:  
> - 新增 §4: Multica 非 Claude Agent 实现分析（源码级）  
> - 重写 §5: 四方案对比 + 推荐方案 A+（CLI 二进制 + 文件注入）  
> - 重写 §6: 实施计划（CLI 优先，防止返工）  
> - 推翻原 ADR-006 (Function Calling) → 新 ADR-008 (CLI + 文件注入)  
> - 新增 ADR-009 (指令注入策略)

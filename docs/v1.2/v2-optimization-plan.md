# Solo v2 优化方案 — 综合对比分析与实施计划

> 版本: 2.0 [已修正]
> 作者: arc agent (架构师)
> 日期: 2026-05-11
> 范围: Slock.ai vs Multica vs Solo 三方案对比 + Agent 系统深度优化 + 实施路线图
>
> **修正说明**: 本文档第 1 版对 Multica 的架构存在理解偏差。本版本基于 Multica 实际源代码深度阅读修正，确保对比准确。所有修正处标记 `[已修正]`。

---

## 目录

1. [三方案对比矩阵](#1-三方案对比矩阵)
   - [1.4 修正记录](#14-修正记录)
2. [可直接借鉴的设计](#2-可直接借鉴的设计)
3. [Agent 系统深度优化方案](#3-agent-系统深度优化方案)
4. [架构调整建议](#4-架构调整建议)
5. [实施优先级与路线图](#5-实施优先级与路线图)
6. [附录：关键决策记录 ADR](#6-附录关键决策记录-adr)

---

## 1. 三方案对比矩阵

### 1.1 全维度对比表

| 维度 | Slock.ai | Multica | Solo (当前) | Solo (目标) |
|------|----------|---------|-------------|-------------|
| **Agent 通信协议** | MCP stdio transport (chat-bridge.js) | **[已修正] 11 个 Backend 通过 stdin/stdout 管道统一**<br>Claude Code: stream-json<br>Codex: JSON-RPC 2.0<br>Kimi/Hermes/Kiro: ACP<br>其他: 自定义 JSON<br><br>统一接口 `Execute(ctx, prompt, opts) → Session`<br><br>MCP config 仅作为 `--mcp-config` 参数（让 Agent 访问外部 MCP 工具），**不是 Multica 自己的通信层** | `-p` 传参 + `--print` 一次性输出 | **[已修正] Backend 接口 (stdin/stdout 管道)**<br>参照 Multica 的 `Execute()` 设计，适配 Solo 的频道/消息模型 |
| **Agent 工作空间隔离** | `~/.slock/agents/<id>/` 完整目录隔离 | **[已修正] per-task workspace**<br>`~/multica_workspaces/<ws_id>/<task_id>/workdir/`<br>每任务独立 CWD，非 per-agent | 无隔离，所有 Agent 共享 CWD | `~/.solo/agents/<id>/` + workspace/ 执行隔离 |
| **Agent 内存/记忆** | `MEMORY.md` 文件系统持久化，Agent 自维护 | **[已修正] 无 Agent 级长期记忆**<br>每次任务创建新 worktree，没有 MEMORY.md 或等效机制。这是 **Slock 独有的设计** | 无 | `MEMORY.md` 文件系统 + 自动记忆摘要 |
| **System Prompt 生成** | 317 行动态生成，包含完整 Slock 协议规范 | `BuildPrompt()` 按任务类型分类 (chat/mention/issue)，信息通过 **CLAUDE.md + .agent_context/** 文件注入 | `buildPrompt()` 简单拼接 system_prompt + messages | 按触发类型分类的 PromptBuilder (chat/mention/dm/thread) + MEMORY.md 注入 |
| **Agent 生命周期** | spawn/wake/sleep/restart + session resume | 每次新任务启动新进程，task 级别生命周期 | 无生命周期管理，每次触发新建 goroutine | Daemon 进程级别 + task 执行级别 |
| **文件系统结构** | `~/.slock/machines/<id>/` + `agents/<id>/` | `~/.multica/config.json` + **CLAUDE.md 注入** + `.agent_context/` 目录配置 | 无 | `~/.solo/agents/<id>/` + `~/.solo/daemon/` |
| **认证体系** | 三层: Machine API Key + Agent Token + Human Session | 两层: Bearer Token `mul_xxx` + JWT | 两层: JWT access/refresh + Daemon Internal-Token | 维持 JWT + 加强 Daemon 通信安全 |
| **消息/事件格式** | `[target=#channel msg=... time=... type=...]` 结构化 header | REST JSON via HTTP | 标准 REST JSON + WebSocket JSON | 维持现有格式 |
| **任务/工单系统** | Task 引擎 (todo→in_progress→in_review→done) | Issue 系统 (todo→in_progress→in_review→done) + Label/Priority/Subscriber | 无 | 暂不引入，保持频道消息流模式 |
| **技能/工具系统** | MCP 工具 (via chat-bridge) | Skills 目录 + Provider 原生 `.claude/skills/` 注入 | 无 | **[已修正] 继承 Multica 模式**<br>skills/ 目录注入（非 MCP 工具系统），内置工具通过 LLM function calling |

### 1.2 架构复杂度对比

| 维度 | Slock.ai | Multica | Solo (当前) | Solo (目标) |
|------|----------|---------|-------------|-------------|
| 后端语言 | Node.js/TypeScript | Go | Go | Go |
| 后端框架 | Express (推测) | chi | chi | chi |
| 数据库 | PostgreSQL + Redis | PostgreSQL + Redis | PostgreSQL | PostgreSQL (+ 可选的 Redis) |
| Agent 执行层 | Node.js daemon + MCP bridge | **[已修正] Go daemon + Backend 接口 (stdin/stdout 管道)** | Go daemon + llm.Provider | **[已修正] Go daemon + Backend 接口 (stdin/stdout 管道)** |
| UI 技术 | 未探查 | 未探查 | Next.js + shadcn/ui | Next.js + shadcn/ui (风格改造) |
| 实时通信 | WebSocket | WebSocket | WebSocket | WebSocket |
| 部署 | 未探查 | Docker Compose | Docker Compose | Docker Compose |

### 1.3 各自优势总结

**Slock.ai 优势**:
- 最成熟的 Agent 生命周期管理 (spawn/wake/sleep + session resume)
- MCP Chat Bridge 设计——标准化的 Agent 工具调用协议
- 动态 System Prompt 生成 (317 行)——非常详尽
- Trace Engine——端到端可观测
- Machine Lock 防止同机多开
- 结构化消息头——人机皆可读

**Multica 优势**:
- **[已修正] 最成熟的 Backend 接口体系**——11 种 CLI Agent 通过 stdin/stdout 管道统一抽象。Claude (stream-json), Codex (JSON-RPC 2.0), Kimi/Hermes/Kiro (ACP), 其他 (自定义 JSON)
- 完整的 Skills 系统——CLAUDE.md + .agent_context/ + skills/ 目录与 Agent 原生集成
- Issue 工单系统——精细化的任务管理 (status/priority/label/subscriber)
- Autopilot——定时自动触发
- execenv 任务隔离——真正的 per-task workspace (每任务独立 CWD)
- Prompt 构建策略——按任务类型分类 + 文件注入
- **[已修正] InjectRuntimeConfig 模式**——动态写入 CLAUDE.md，Agent CLI 原生发现项目配置
- CLI 命令体系——完整的 agent/issue/autopilot 命令行

**Solo 优势**:
- 频道/消息/线程的实时协作——Slock.ai 和 Multica 都没有的级别
- 已经实现的 @mention + Agent 自动触发
- 成熟的 WebSocket Hub 架构
- 无状态 Server（水平扩展友好）
- Agent-to-Agent 协作的前期设计（trigger chain）
- 开源可自托管

### 1.4 修正记录 [新增]

#### 修正前 vs 修正后

| # | 维度 | 修正前错误理解 | 修正后正确理解 | 修正依据 |
|---|------|--------------|--------------|---------|
| 1 | **Agent 通信协议** | Multica 使用 MCP 或类似 MCP 的协议层 | Multica 的 11 个 Backend 全部通过 **stdin/stdout 管道** 通信。MCP 只是 Claude Code 的 `--mcp-config` 参数，不是 Multica 自身通信层 | 阅读 multica/pkg/backend/ 源文件 |
| 2 | **Agent 级记忆** | Multica 有关似 MEMORY.md 的记忆机制 | Multica **没有** Agent 级长期记忆。每次任务创建新 worktree。MEMORY.md 是 Slock 独有的设计 | 阅读 multica 工作流代码 |
| 3 | **Machine Lock** | Multica 也有 Machine Lock | Machine Lock 是 **Slock 独有**，Multica 不存在 | 阅读 multica 配置/启动代码 |
| 4 | **Workspace 粒度** | — | 确认 Multica 是 **per-task workspace** (`<ws_id>/<task_id>/workdir/`)，与 Solo 的 per-agent workspace 路线不同 | 阅读 multica execenv 模块 |
| 5 | **配置注入方式** | — | Multica 使用 **CLAUDE.md + .agent_context/** 注入配置而非 MEMORY.md | 阅读 multica 运行时配置模块 |
| 6 | **工程布局** | — | Multica 按 provider 分离 `.claude/skills/` 目录，Backend 输出统一规范化为 `Message{Type, Content, ...}` | 阅读 multica/skills/ 和 backend/ |

#### 新结论: Solo v2 = Slock 产品模型 x Multica 工程实现

| 层 | 学谁 | 具体内容 |
|------|------|---------|
| 产品形态 | Slock | 频道式协作、Agent 作为频道成员、对话式交互 |
| Workspace | Slock | `~/.solo/agents/<id>/` Agent 级隔离 |
| 记忆 | Slock | MEMORY.md 会话间持久化 |
| System Prompt | 两者结合 | Slock 的动态生成 + Multica 的按场景分类 |
| Backend 接口 | Multica | `Execute(ctx, prompt, opts) → Session` 11 种统一抽象 |
| 通信管道 | Multica | 非 MCP，exec.Command → stdin JSON → stdout JSONL |
| CLAUDE.md 注入 | Multica | 写入 workdir，Agent CLI 原生发现 |
| Skills 目录 | Multica | `.claude/skills/` 按 provider 分离 |
| PromptBuilder | Multica | 按场景（聊天/提及/任务）生成不同 prompt |
| 消息规范化 | Multica | 所有 backend 输出统一为 `Message{Type, Content, ...}` |

---

## 2. 可直接借鉴的设计

### 2.1 P0 — 需要立即做（当前迭代 v1.1）

| # | 设计来源 | 具体内容 | Solo 当前状态 | 实施方式 |
|---|---------|---------|-------------|---------|
| P0-1 | **[已修正] Multica** | **完整 Backend 接口 + 11 种实现 (stdin/stdout 管道)**<br>Claude (stream-json), Codex (JSON-RPC 2.0), Kimi/Hermes/Kiro (ACP) 等 | `local.go` 只用 `-p prompt --print`，无流式、无 system prompt 正确传递。旧方案误认为需要自建 MCP Bridge | 从 Multica 直接搬 `Backend` 接口设计 + CLI 参数构造 + stream-json 解析。**不是自己写 MCP Bridge** |
| P0-2 | **Slock.ai** | **Agent workspace + MEMORY.md**<br>`~/.solo/agents/<id>/` 目录结构，含 MEMORY.md 文件持久化 | 无隔离，无记忆 | 实现 workspace 管理，Agent 在独立目录执行。MEMORY.md 自动读写 |
| P0-3 | **[已修正] Multica** | **`InjectRuntimeConfig` 模式——动态写入 CLAUDE.md**<br>在 Agent workdir 中自动生成 CLAUDE.md，包含 Agent 能力说明、项目配置、运行时上下文 | 无 | 每次任务前写入 CLAUDE.md，Agent CLI 原生发现。这是 Multica 的配置注入模式而非 Slock 的全量 system prompt |
| P0-4 | **Both** | **PromptBuilder 按触发类型分类**<br>chat/mention/dm/thread 四种场景生成不同 prompt | `buildPrompt()` 简单把 system_prompt + messages 拼在一起 | **[已修正] 此条目从 v1.1 提升为 P0**，因 prompt 构建是 Agent 体验核心 |

**为什么这些是 P0**:
- P0-1: Backend 接口替换是解决 `--append-system-prompt` 缺失、流式输出、多 Agent CLI 统一等所有问题的根因。不是"功能缺失"而是"架构错误"——当前 `llm.Provider` 接口设计不适合 CLI Agent
- P0-2: 无工作空间隔离使得多 Agent 数据不隔离——安全风险。无记忆使得 Agent 每次对话都是"全新"的——产品核心价值缺失
- P0-3: CLAUDE.md 注入是 Multica 验证的配置模式，Agent CLI (特别是 Claude Code) 原生读取，比 `--append-system-prompt` 更可靠
- P0-4: prompt 拼接策略决定了 Agent 能否正确理解上下文——核心体验

### 2.2 P1 — 可以后续做（v1.2/v1.3）

| # | 设计来源 | 具体内容 | 目标版本 | 实施方式 |
|---|---------|---------|---------|---------|
| P1-1 | **[已修正] Multica** | **Skills 目录系统**<br>`.claude/skills/` 按 provider 分离的技能注入 | v1.2 | 在 workspace 中创建 `.claude/skills/` 等目录，参照 Multica 的按 provider 分离模式 |
| P1-2 | **Slock.ai** | 动态 System Prompt 生成（类似 317 行完整协议） | v1.2 | 在 PromptBuilder 基础上扩展，包含 Solo 协议规范、工具列表、memory |
| P1-3 | **Multica** | 内置工具系统 (read_file, write_file, search, execute) | v1.2 | 实现 `Tool` 接口 + `ToolRegistry` + LLM function calling 集成 |
| P1-4 | **[已修正] Multica** | **消息规范化**<br>所有 Backend 输出统一为 `Message{Type, Content, ...}` | v1.2 | 实现 Multica 的消息规范化模式，适配 Solo 的频道消息模型 |
| P1-5 | **Slock.ai** | Agent 生命周期管理 (spawn/wake/sleep) | v1.3 | Daemon 端实现进程状态跟踪 + session 持久化 |
| P1-6 | **Multica** | Autopilot 定时任务 | v2.0 | 实现 cron 触发 + Agent 自动执行 |
| P1-7 | **Slock.ai** | Delivery Manager (消息投递保证、gated delivery、recovery) | v2.0 | Daemon 端实现消息队列 + 重试逻辑 |
| P1-8 | **Multica** | Task/Issue 工单系统 | 暂不引入 | 评估是否适合 Solo 的频道模式 |
| P1-9 | **Slock.ai** | Trace Engine (JSONL + OpenTelemetry Span) | v2.0 | 当前 Prometheus 已覆盖基础观测 |

### 2.3 P2 — 远期做或不适合做

| # | 设计来源 | 具体内容 | 判断 | 原因 |
|---|---------|---------|------|------|
| P2-1 | **Slock.ai** | Machine Lock (PID + token 文件锁) | 远期做 | 当前单 Daemon 场景不必要，多 Daemon 时再做 |
| P2-2 | **Slock.ai** | MCP Chat Bridge | **不适合** | **[已修正] Multica 的实践已证明：AI CLI 通过 stdin/stdout 管道通信效率更高，不需要 MCP 作为中间层**。MCP 插件的使用场景是"Agent 访问外部工具"，而 Solo 的 Backend 与 Agent CLI 之间用 stdin/stdout 直连即可。 |
| P2-3 | **Multica** | Autopilot cron 触发 | 远期做 (v2.0) | cron 定时任务超出 MVP 范围 |
| P2-4 | **Multica** | Issue 工单系统 (todo→in_progress→in_review→done) | **不适合** | Solo 的核心模式是"频道消息流"，不是"工单系统"。引入 Issue 会分裂用户体验——Slack 是消息，Jira 是工单，不要混。Agent 任务应该在频道中以消息 + 线程方式表达。 |
| P2-5 | **Multica** | Per-task workspace (每任务独立工作目录) | **不适合** | 当前 Agent 级别的 workspace 足够。每任务 workspace 增加复杂度，且 Agent 在频道中的上下文是连续的，不需要每次隔离。 |
| P2-6 | **Slock.ai** | 结构化消息头 `[target=... msg=... time=... type=...]` | **不适合** | Solo 已有 JSON 格式的 WebSocket 消息，REST API + WebSocket 对前端更友好。结构化消息头是为 CLI Agent 设计的，在 Solo 的架构下不需要。 |
| P2-7 | **Slock.ai** | 三层认证 (Machine Key + Agent Token + Human Session) | **不适合** | 当前两层认证足够。三层认证增加复杂度，且 Solo 的单 Server 架构不需要 Machine 级别的认证。 |

---

## 3. Agent 系统深度优化方案

### 3.1 Agent 通信方案: stdin/stdout 管道 [已修正 — 全文重写]

#### 关键修正

第 1 版错误的将 Multica 的通信方式归类为 MCP。**Multica 实际全部使用 stdin/stdout 管道**。

```
修正前: "Multica 的 11 个 Backend 使用 MCP 协议"
修正后: "Multica 的 11 个 Backend 全部通过 exec.Command → stdin JSON → stdout JSONL"
         MCP 仅作为 Claude Code 的 --mcp-config 参数（让 Agent 访问外部工具）
```

#### 结论: 不用 MCP，用 stdin/stdout 管道 + Backend 接口

#### Multica 的 Backend 实现模式

```go
// Multica 的统一 Backend 接口
type Backend interface {
    Execute(ctx context.Context, req *ExecuteRequest, opts ...ExecuteOption) (*Session, error)
}

// 每个 Backend 实现的核心逻辑:
// 1. 构造 CLI 参数 (每个 CLI 的语法不同)
// 2. exec.Command() 启动子进程 (共享 stdin/stdout 管道)
// 3. 向 stdin 写入 JSON 格式的 prompt
// 4. 从 stdout 逐行读取 JSONL 格式的输出
// 5. 解析输出为统一的 Message{Type, Content, ...}
// 6. 处理错误、超时、取消
```

| Backend | CLI | stdin 格式 | stdout 格式 | 特点 |
|---------|-----|-----------|------------|------|
| Claude | `claude` | stream-json | stream-json | `--append-system-prompt`, `--permission-mode bypassPermissions` |
| Codex | `codex` | JSON-RPC 2.0 | JSON-RPC 2.0 | 标准 JSON-RPC 协议 |
| Kimi | `kimi` | ACP | ACP | 自定义 Agent 协议 |
| Hermes | `hermes` | ACP | ACP | 同 Kimi 协议族 |
| Kiro | `kiro` | ACP | ACP | 同 Kimi 协议族 |
| Cursor | `cursor` | 自定义 JSON | 自定义 JSON | Cursor 专属 |
| Copilot | `copilot` | 自定义 JSON | 自定义 JSON | GitHub Copilot CLI |
| OpenCode | `opencode` | 自定义 JSON | 自定义 JSON | 开源 Cursor 替代 |
| OpenClaw | `openclaw` | 自定义 JSON | 自定义 JSON | 定制 Agent CLI |
| Gemini | `gemini` | 自定义 JSON | 自定义 JSON | Google Gemini CLI |
| Pi | `pi` | 自定义 JSON | 自定义 JSON | Pi CLI |

#### Solo 的 Backend 设计

Solo 需要复用 Multica 的接口设计，但适配 Solo 的频道/消息模型:

```go
// Solo 的 Backend 接口 (参考 Multica，但适配 RunRequest)
type Backend interface {
    // Execute 执行 Agent 任务
    // - req 包含 Solo 的频道上下文、消息历史、触发类型等
    // - 返回 Session 包含整个执行过程的完整输出
    Execute(ctx context.Context, req *RunRequest, opts ...RunOption) (*Session, error)

    // Name 返回 Backend 名称
    Name() string
}

// Solo 的 RunRequest (频道上下文)
type RunRequest struct {
    AgentID     string          // 目标 Agent
    ChannelID   string          // 频道 ID
    TriggerType TriggerType     // chat / mention / dm / thread
    Messages    []Message       // 消息历史 (含当前触发消息)
    SystemInfo  *SystemContext  // 系统上下文 (频道名、成员、协议规则)
    Workspace   *Workspace      // Agent workspace 路径
}

// Session 包含执行过程
type Session struct {
    ID        string
    Status    SessionStatus     // running / completed / failed / cancelled
    Stream    <-chan OutputChunk // 流式输出通道
    Error     error
    StartTime time.Time
    EndTime   time.Time
}

// OutputChunk 是流式输出片段
type OutputChunk struct {
    Type    ChunkType   // text / tool_call / error / done
    Content string
    Meta    map[string]any
}
```

#### 架构演进

```
v1.1 (P0):
  Server ──HTTP/SSE──→ Daemon ──Backend.Execute()──→ 子进程 (stdin/stdout)
                                    ↑
                               WorkspaceManager 准备 workspace
                               InjectRuntimeConfig 注入配置
                               MEMORY.md 读取

v1.2 (P1):
  Backend 接口扩展为多实现:
    ClaudeBackend (stream-json, 主实现)
    OpenAICompatBackend (HTTP API, 已有 llm.Provider 封装)
    OllamaBackend (HTTP API)

v2.0 (远期):
  如需 MCP 插件能力，通过 Backend 的 --mcp-config 参数注入
  而非自建 MCP 通信层
```

### 3.2 System Prompt 生成: Multica 模式 vs Slock 模式

**结论: Multica 的按任务分类模式更适合 Solo，Slock 的全量协议注入模式过于重。但随着演进，Slock 的详尽注入方式值得参考。**

#### Multica 模式（选择）

Multica 的 `BuildPrompt()` 按触发类型生成不同的 prompt:
- Chat: 简单频道上下文
- Mention: @提及场景，强调需要回复
- DM: 私信场景
- Issue: 工单分配场景

每条 prompt 保持**最小**，详细的 Agent 配置通过文件注入 (`CLAUDE.md`, `.agent_context/`)。

优点: 简洁、聚焦、Agent 不会因为过多的协议规范而困惑
缺点: 信息密度低，Agent 可能需要额外步骤获取信息

#### Slock 模式（参考）

Slock 生成 317 行 System Prompt，包含:
- 完整 Slock 协议规范
- 所有可用的 CLI 命令
- 消息格式约束
- 协作规则
- 工具使用说明

优点: Agent 一次性获得完整上下文，不需要额外查询
缺点: 过度注入——317 行中大部分是 Agent 不需要随时了解的协议细节

#### Solo 的决策

采用 **Multica 的分类型 prompt** 作为基础，在 Agent 能力升级时参考 **Slock 的详尽注入**:

```
v1.1:
  PromptBuilder.buildChatPrompt()   → 频道上下文 + 最近消息
  PromptBuilder.buildMentionPrompt() → @提及上下文 + 回复要求
  PromptBuilder.buildDMPrompt()      → 私信上下文

  CLAUDE.md 注入 → Agent CLI 原生读取，包含 Solo 能力说明

v1.2:
  在 prompt 中增加工具列表和调用方式
  MEMORY.md 注入到 system prompt
  Skills 目录注入

v2.0:
  参考 Slock 的协议注入方式
  但通过文件系统（CLAUDE.md）而非文本注入
```

### 3.3 Agent Workspace 设计 [已修正 — 目录结构更新]

**结论: 采用 Slock 的 `~/.solo/agents/<id>/` 模式（Agent 级隔离），而非 Multica 的 per-task workspace（任务级隔离）。**

#### Solo 的 Workspace 结构

```
~/.solo/
├── agents/
│   └── <agent-id>/
│       ├── MEMORY.md              # [Slock] 长期记忆，Agent 自维护
│       ├── solo-system-prompt.md  # [Slock] 动态生成，频道上下文 + Agent 配置
│       ├── CLAUDE.md              # [Multica] 通过 InjectRuntimeConfig 写入
│       │                             Agent 能力说明、Solo 协议规范、项目资源
│       ├── workspace/             # 执行时的 CWD (Agent 在此工作)
│       └── skills/                # [Multica] 按 provider 分离的技能目录
│           ├── claude/            #   Claude Code 原生 skill
│           ├── vscode/            #   VSCode 相关 skill (可选)
│           └── custom/            #   自定义 skill
└── daemon/
    ├── lock.json                  # [Slock] Machine Lock (PID + token)
    └── traces/                    # [Slock] JSONL 执行记录
        └── <agent-id>.jsonl       #   逐行 JSON，每行一个 trace event
```

**为什么选择 Agent 级 workspace**:
1. **Agent 生命周期连续** — Agent 在频道中的对话是连续的，不需要每次重新创建 workspace
2. **记忆持久化** — MEMORY.md 在相同路径，Agent 自然能读写
3. **文件简单** — Agent 级目录，不是 task 级，清理和管理都简单
4. **与 Slock 对齐** — 竞品设计验证过

#### Multica 模式（不采用但参考其配置方式）

```
~/multica_workspaces/<ws-id>/<task-id>/
  ├── workdir/             # 每任务独立 CWD
  ├── output/              # 执行产物
  └── .gc_meta.json        # GC 元数据
```

**为什么不用 per-task**:
1. **按任务创建和清理** — 需要 GC 机制 (GCTTL=24h)，增加复杂度
2. **无连续性** — 每次任务都是全新目录，Agent 无法自然积累 context
3. **过度隔离** — Solo 的 Agent 在频道中工作，不是 Multica 的"一次 Issue 一次执行"模式

**但是借鉴其 CLAUDE.md 注入方式**: InjectRuntimeConfig 模式，在任务执行前自动写入 CLAUDE.md。

#### WorkspaceManager 的职责

```go
type WorkspaceManager struct {
    BasePath string // ~/.solo/agents/
}

func (wm *WorkspaceManager) Prepare(agentID string, config *AgentConfig) error {
    // 1. 确保 ~/.solo/agents/<id>/ 存在
    // 2. 写入 CLAUDE.md (InjectRuntimeConfig 模式)
    // 3. 写入 solo-system-prompt.md (动态生成)
    // 4. 确保 skills/ 目录按 provider 分离
    // 5. 读取 MEMORY.md (如果存在)
    // 6. 设置 CWD 为 workspace/
}

func (wm *WorkspaceManager) ReadMemory(agentID string) (string, error) {
    // 读取 MEMORY.md 内容
}

func (wm *WorkspaceManager) SaveMemory(agentID string, content string) error {
    // 追加或覆写 MEMORY.md
}
```

### 3.4 Agent 生命周期管理方案

#### 设计目标

| 状态 | 说明 | 触发条件 |
|------|------|---------|
| **spawn** | 创建 Agent 进程（首次启动） | Agent 创建、Daemon 启动 |
| **idle** | 进程存在但空闲（等待消息） | spawn 完成、wake 后无任务 |
| **running** | 正在执行任务 | 收到触发消息 |
| **sleep** | 进程释放（保留 session 数据） | 长时间空闲、资源回收 |
| **restart** | 重新启动（恢复 session） | sleep 状态被触发 |

#### 实现方案（简化版 — v1.1/v1.2）

Solo 的 Agent 生命周期需要比 Slock 简单，因为:
1. Solo 是 Goroutine 架构，不是 Node.js 进程架构
2. **[已修正] Multica 模式也是每次启动新进程** — 通过 exec.Command 创建子进程，完成后退出。即使 Claude Code 支持 stream-json 流式输出，每次触发仍然是独立进程
3. 当前 MVP 不需要 session resume

```
v1.1 简单模型（P0）:
  spawn = WorkspaceManager 创建 ~/.solo/agents/<id>/ 目录
  running = `exec.CommandContext(agentCLI, args...)` 执行
            stdin → JSON prompt (stream-json 格式)
            stdout → 逐行读取 JSONL
  done = 进程退出，读取最终输出

v1.2 增强模型（P1）:
  spawn = 同上
  running = Backend.Execute() 全流程:
            PrepareWorkspace → InjectConfig → BuildPrompt → ReadMemory
            → exec.Command → stdin.Write → stdout.Read → ParseOutput
  done/失败 = 记录执行日志、error 处理、资源清理

v2.0 完整模型（P2）:
  引入 Agent 进程保活 (goroutine pool)
  但每次任务仍然是独立子进程 (exec.Command)
  "生命周期" = workspace 存在 + goroutine 跟踪，不是子进程常驻
```

**对当前架构的影响**:

```go
// v1.1 — 生命周期状态机（简化）
type AgentStatus string

const (
    AgentStatusSpawned  AgentStatus = "spawned"   // workspace 已创建
    AgentStatusIdle     AgentStatus = "idle"      // 可接受任务
    AgentStatusRunning  AgentStatus = "running"   // 执行中
    AgentStatusSleeping AgentStatus = "sleeping"  // 进程释放，数据保留
)

type AgentLifecycle struct {
    AgentID    string
    Status     AgentStatus
    Workspace  *Workspace
    LastActive time.Time
    Config     AgentConfig
}
```

### 3.5 动态 System Prompt 生成方案（参考 Slock 317 行 + Multica 文件注入）

#### 目标

为 Agent 生成的 System Prompt 需要包含:

| 内容块 | 来源 | 动态性 | 注入方式 | 示例长度 |
|--------|------|--------|---------|---------|
| Agent Identity | agents.system_prompt | 静态（用户配置） | solo-system-prompt.md | 5-20 行 |
| Memory | MEMORY.md | 动态（自动维护） | MEMORY.md | 5-20 行 |
| Channel Context | 频道信息 | 动态（每次触发） | CLAUDE.md | 5-10 行 |
| Recent Messages | 消息历史 | 动态（每次触发） | 通过 stdin 传递 | 10-50 行 |
| Solo Platform Rules | 系统配置 | 静态 | CLAUDE.md | 10-20 行 |
| Available Tools | Agent 配置 | 动态（工具启用时） | CLAUDE.md / skills/ | 5-10 行 |
| Reply Instructions | 触发类型 | 动态 | 通过 stdin 传递 | 3-5 行 |

**生成流程**:

```
Agent 被触发
  →
  1. WorkspaceManager.Prepare() 准备 workspace
     - 写入 CLAUDE.md (InjectRuntimeConfig)
     - 写入 solo-system-prompt.md (Agent 配置)
     - 确保 skills/ 目录存在
  2. PromptBuilder 构建 prompt
     - 读取 MEMORY.md
     - 读取频道上下文 (名称、描述、最近消息)
     - 确定触发类型 (mention/chat/dm/thread)
     - 拼接 Message 列表:
       [SystemContext] + [Memory] + [ChannelMessages] + [ReplyInstruction]
  3. Backend.Execute()
     - exec.Command 启动子进程
     - stdin 写入 prompt (stream-json 格式)
     - stdout 逐行读取 JSONL 输出
     - 解析为统一的 OutputChunk
  →
  通过 SSE 转发到 Server → WebSocket 推送到前端
```

**生成的 CLAUDE.md 结构 (InjectRuntimeConfig 模式)**:

```markdown
# Solo Agent Configuration

You are an AI Agent on the Solo platform.

## Your Identity
{agent.system_prompt}

## Solo Platform Rules
- You participate in channel conversations
- You are triggered when a user sends a message or @mentions you
- Use MEMORY.md to store information across conversations
- Update MEMORY.md when you learn important information
- Available skills are in the .claude/skills/ directory

## Channel Context (updated per execution)
Channel: #{channel_name}
Description: {channel_description}
Trigger type: {chat / mention / dm / thread}

## Available Tools
{通过 --mcp-config 或工具系统注入}
```

**通过 stdin 传递的消息结构 (stream-json 格式)**:

```json
{
  "messages": [
    {"role": "system", "content": "## Current Task\n{reply_instructions}\n\n## Recent Messages\n{formatted_messages}"},
    {"role": "user", "content": "{trigger_message}"}
  ]
}
```

---

## 4. 架构调整建议

### 4.1 Daemon 新增组件

#### 4.1.1 当前 Daemon 架构（过于简单）

```
┌─────────────────────────────────────┐
│ Daemon (cmd/daemon)                  │
│                                     │
│ handler.go: HTTP POST /run          │
│   → provider.CompleteStream()       │
│   → SSE stream back to Server       │
│                                     │
│ ❌ 无 Backend 接口                   │
│ ❌ 无 WorkspaceManager               │
│ ❌ 无 PromptBuilder                  │
│ ❌ 无 ToolRegistry                   │
│ ❌ 无 Agent Lifecycle                │
│ ❌ 无 MemoryManager                  │
└─────────────────────────────────────┘
```

#### 4.1.2 新 Daemon 架构（v1.1 → v1.2 → v2.0 逐步增量）

```
┌──────────────────────────────────────────────┐
│ Daemon                                        │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ TaskRouter (v1.1 新增)                    │ │
│  │  → 解析任务类型 (chat/mention/dm/thread)  │ │
│  │  → 调用 PromptBuilder 构造 prompt        │ │
│  │  → 选择 Backend (claude/codex/opencode)  │ │
│  │  → 调用 WorkspaceManager.Prepare()       │ │
│  │  → 调用 MemoryManager.ReadMemory()       │ │
│  │  → Backend.Execute() 并流式返回           │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ Backend Factory (v1.1 新增) [已修正]       │ │
│  │  → Backend 接口定义                       │ │
│  │  → Claude Backend (stdin/stdout 管道)     │ │
│  │  → Codex Backend (JSON-RPC 2.0)          │ │
│  │  → OpenAI Backend (HTTP API, 已有封装)    │ │
│  │  → Ollama Backend (HTTP API, 已有封装)    │ │
│  │  → 全部通过 exec.Command 启动子进程       │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ WorkspaceManager (v1.1 新增)              │ │
│  │  → ~/.solo/agents/<id>/ 目录管理          │ │
│  │  → workspace 创建/同步/清理               │ │
│  │  → InjectRuntimeConfig (CLAUDE.md 写入)   │ │
│  │  → solo-system-prompt.md 写入              │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ MemoryManager (v1.1 新增)                 │ │
│  │  → MEMORY.md 读写                        │ │
│  │  → 记忆摘要生成（LLM 二次调用）            │ │
│  │  → 记忆片段数量控制                       │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ PromptBuilder (v1.1 新增)                 │ │
│  │  → buildChatPrompt() / buildMentionPrompt()│ │
│  │  → buildDMPrompt() / buildThreadPrompt()  │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ ToolRegistry (v1.2 新增)                  │ │
│  │  → 内置工具注册 (read/write/search)       │ │
│  │  → LLM function calling 集成             │ │
│  │  → 安全等级控制                          │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ Agent Lifecycle (v2.0 新增)               │ │
│  │  → spawned/idle/running/sleeping 状态机   │ │
│  │  → session 持久化                        │ │
│  │  → 资源回收策略                          │ │
│  └──────────────────────────────────────────┘ │
└──────────────────────────────────────────────┘
```

#### 4.1.3 [已修正] 参考 Multica 设计但简化

**[已修正] 之前的架构对比中错误认为 Multica 使用 MCP。实际 Multica 和 Solo 都使用 stdin/stdout 管道 + Backend 接口模式。**

| Multica 组件 | Solo 实现方案 | 说明 |
|-------------|-------------|------|
| Backend.Execute() | 复用接口设计 | Multica 已验证的 `Execute(ctx, prompt, opts) → Session` 直接复用 |
| InjectRuntimeConfig | WorkspaceManager 写入 CLAUDE.md | Agent CLI (尤其是 Claude Code) 原生读取 |
| ExtractPrompt | PromptBuilder 构建 prompt | 按场景分类 + 文件注入 |
| Skills 目录系统 | 复用 `.claude/skills/` | 按 provider 分离 |
| 消息规范化 | 复用 `Message{Type, Content, ...}` | Backend 输出统一格式 |

| Slock 组件 | Solo 替代方案 | 说明 |
|-----------|-------------|------|
| Machine Lock | daemon/lock.json | 单 Daemon 场景可暂缓 |
| Trace Engine | Prometheus 指标已有 + JSONL traces | 后续可补充 JSONL trace |
| Delivery Manager | Server 端消息持久化 + WebSocket 推送 | 当前架构已覆盖投递保证 |
| Agent Lifecycle | 简化为 spawned/idle/running/sleeping | 不需要进程级 spawn，Goroutine 池即可 |

### 4.2 Server 新增模块

#### 4.2.1 需要新增

| 模块 | 功能 | 来源参考 | 目标版本 |
|------|------|---------|---------|
| **Agent Config API** | 扩展 Agent CRUD: tools, memory_enabled, execution_mode | Multica | v1.2 |
| **Workspace API** | Daemon 注册 workspace 状态、Agent 同步请求 | Slock | v1.1 |
| **Search API** | 全文搜索消息历史 | Multica | v1.3 |
| **Usage Records** | LLM token/费用统计 | Multica | v1.3 |
| **Tool Config API** | Agent 工具配置 CRUD | Multica | v1.2 |
| **Agent Activity API** | Agent 活动历史查询 | Multica | v1.3 |

#### 4.2.2 不需要新增

| 模块 | 原因 |
|------|------|
| Issue/工单系统 | 不符合 Solo 的频道消息流模式 |
| Autopilot 服务器端 | cron 触发可以在 Daemon 端或通过简单定时任务实现 |
| Skills 市场 | 当前不需要包管理能力 |
| MCP 通信层 | **[已修正] 第 1 版认为 MCP 是未来方向。修正后结论: 始终不使用 MCP 作为通信层**。如需 Agent 访问外部工具，通过 `--mcp-config` 参数注入，而非自建 MCP Server |

### 4.3 数据模型扩展

#### 4.3.1 新增表

```sql
-- v1.1: Agent Workspace 状态追踪（可选——当前在文件系统管理即可）
-- 是否需要数据库表取决于是否需要跨 Daemon 实例查询 workspace 状态

-- v1.2: Agent 工具配置
CREATE TABLE agent_tools (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,           -- read_file, write_file, search
    enabled     BOOLEAN DEFAULT true,
    config      JSONB DEFAULT '{}',      -- 工具专属配置（如允许的路径）
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(agent_id, name)
);

CREATE INDEX idx_agent_tools_agent ON agent_tools(agent_id);

-- v1.2: Agent 执行记录
CREATE TABLE agent_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id),
    channel_id  UUID REFERENCES channels(id),
    thread_id   UUID REFERENCES messages(id),  -- 触发消息
    trigger_type TEXT NOT NULL,          -- mention, chat, dm
    status      TEXT DEFAULT 'running',  -- running, completed, failed
    started_at  TIMESTAMPTZ DEFAULT now(),
    finished_at TIMESTAMPTZ,
    error       TEXT,
    token_usage JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_agent_runs_agent ON agent_runs(agent_id);
CREATE INDEX idx_agent_runs_channel ON agent_runs(channel_id);

-- v1.3: LLM 用量记录
CREATE TABLE usage_records (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    UUID NOT NULL REFERENCES agents(id),
    channel_id  UUID REFERENCES channels(id),
    run_id      UUID REFERENCES agent_runs(id),
    provider    TEXT NOT NULL,            -- anthropic, openai, local
    model       TEXT NOT NULL,
    input_tokens  INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    cost_micro_usd BIGINT DEFAULT 0,     -- 微美元
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_usage_agent ON usage_records(agent_id);
CREATE INDEX idx_usage_created ON usage_records(created_at);

-- v1.3: Agent 活动日志
CREATE TABLE agent_activity_log (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    UUID NOT NULL REFERENCES agents(id),
    run_id      UUID REFERENCES agent_runs(id),
    action      TEXT NOT NULL,            -- triggered, responded, tool_call, memory_update
    details     JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_activity_agent ON agent_activity_log(agent_id, created_at DESC);
```

#### 4.3.2 现有表扩展

```sql
-- v1.2: agents 表增加字段
ALTER TABLE agents ADD COLUMN IF NOT EXISTS execution_mode TEXT DEFAULT 'chat';  -- chat, interactive
ALTER TABLE agents ADD COLUMN IF NOT EXISTS memory_enabled BOOLEAN DEFAULT true;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tools JSONB DEFAULT '[]';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS total_runs INT DEFAULT 0;
```

---

## 5. 实施优先级与路线图

### 5.1 总体时间线

```
2026年5月           2026年6月           2026年7月           2026年8月
─────────────────────────────────────────────────────────────────────────►

v1.1 (4周)          v1.2 (6周)          v1.3 (6周)          v2.0 (8周)
┌──────────┐       ┌───────────┐       ┌───────────┐       ┌──────────────┐
│ Backend   │        │ Skills     │        │ 搜索/文件    │        │ 审批流         │
│ Workspace │        │ 内置工具    │        │ 多 Agent    │        │ 定时任务      │
│ Memory    │        │ Agent 管理  │        │ 用量统计     │        │ 插件/MCP      │
│ System    │        │ Lifecycle  │        │ UI 打磨    │        │ OAuth        │
│ Prompt    │        │ 增强       │        │             │        │              │
│ UI 风格改造 │        │            │        │             │        │              │
└──────────┘       └───────────┘       └───────────┘       └──────────────┘

  P0 核心            P1 增强            P1 体验            P2 扩展
```

### 5.2 v1.1 详细计划（当前迭代，4 周）[已修正]

#### Week 1: Backend 接口 + WorkspaceManager + InjectRuntimeConfig

| 任务 | 模块 | 工作量 | 验收 |
|------|------|--------|------|
| **定义 Backend 接口** | `pkg/agent/backend.go` | 1 天 | `Execute(ctx, req, opts) → Session` 接口定义 |
| **实现 Claude Backend (stdin/stdout)** | `pkg/agent/claude.go` | 2 天 | 从 Multica 搬 stream-json 参数构造 + 解析逻辑 |
| **实现 WorkspaceManager** | `pkg/agent/workspace.go` | 2 天 | Agent 创建时生成 `~/.solo/agents/<id>/` 目录结构 |
| **实现 InjectRuntimeConfig** | `pkg/agent/workspace.go` | 1 天 | 动态写入 CLAUDE.md，Agent CLI 原生发现 |
| **Daemon 集成** | `cmd/daemon/task_router.go` | 1 天 | TaskRouter 串联 Backend + Workspace + PromptBuilder |
| **测试** | | 1 天 | Backend 单元测试、workspace 隔离验证 |

**关键里程碑**: Backend 接口 + Claude Backend 替换旧的 `local.go`。CLAUDE.md 注入正确生效。**(替代原有的 "修复 local.go")**

#### Week 2: PromptBuilder + MEMORY.md + 流式输出

| 任务 | 模块 | 工作量 | 验收 |
|------|------|--------|------|
| **实现 PromptBuilder** | `pkg/agent/prompt.go` | 2 天 | chat/mention/dm/thread 四类 prompt |
| **实现 MemoryManager** | `pkg/agent/memory.go` | 1 天 | MEMORY.md 读写 + 注入 system prompt |
| **Claude Backend 流式改进** | `pkg/agent/claude.go` | 1 天 | StdoutPipe 逐行读取，分块推送 (K-03) |
| **消息编辑/删除 API** | `internal/server/handler/channel.go` | 1 天 | PATCH/DELETE /messages/{id} |
| **前端消息编辑/删除 UI** | `frontend/` | 1 天 | 消息悬停菜单 |
| **测试** | | 1 天 | prompt 构建测试、记忆集成测试 |

**关键里程碑**: Local Provider 变 Backend 接口，**system_prompt 通过注入方式正确生效**，消息可编辑删除

#### Week 3-4: 前端 UI 风格改造

| 任务 | 模块 | 工作量 | 验收 |
|------|------|--------|------|
| **CSS 变量 + Tailwind 主题** | `globals.css` | 1 天 | 暗色 + 绿色高亮 + 等宽字体基调 |
| **基础组件改造** | button/input/card/dialog/avatar | 1 天 | 全部 squared + monospace + green accent |
| **侧栏 + 频道列表 + DM** | sidebar/channel-list/dm-list | 2 天 | IRC 风格侧栏 |
| **消息列表 + 输入框** | message-list/message-input | 2 天 | 终端风格 |
| **Agent 消息 + 流式** | agent-message/streaming-message | 1 天 | 绿色高亮 Agent 消息 |
| **Auth 页面** | login/register | 1 天 | 终端窗口风格 |
| **打磨** | 动画/微交互/对比度 | 1 天 | 风格统一 |

**关键里程碑**: 前端视觉风格从"通用 SaaS"变为"终端协作"风格 (K-04)

### 5.3 v1.2 详细计划（6 周）[已修正]

#### Week 5-6: 多 Backend 支持 + Skills 系统

| 任务 | 模块 | 工作量 | 验收 |
|------|------|--------|------|
| **实现 Backend 工厂** | `pkg/agent/factory.go` | 1 天 | 根据 provider 选择 Backend |
| **实现 Codex Backend** | `pkg/agent/codex.go` | 2 天 | JSON-RPC 2.0 协议 |
| **实现 OpenAI/Ollama Backend** | `pkg/agent/openai.go` | 1 天 | HTTP API SDK (复用已有代码) |
| **Skills 目录注入系统** | `pkg/agent/skills.go` | 2 天 | `.claude/skills/` 按 provider 分离 |
| **实现 Tool 接口** | `pkg/agent/tool.go` | 1 天 | Tool 接口定义 + JSON Schema |
| **实现内置工具** | `pkg/agent/tools/` | 3 天 | read_file, write_file, list_files, search_files |
| **消息规范化** | `pkg/agent/message.go` | 1 天 | 所有 Backend 输出统一为 `Message{Type, Content, ...}` |
| **测试** | | 2 天 | 多 Backend 集成测试、工具调用测试 |

**关键里程碑**: 多 Backend 支持 (Codex/Kimi/其他)，Skills 系统可用

#### Week 7-8: Agent 管理增强 + Lifecycle

| 任务 | 模块 | 工作量 | 验收 |
|------|------|--------|------|
| **Agent Lifecycle 状态跟踪** | `pkg/agent/lifecycle.go` | 2 天 | spawned/idle/running/sleeping 状态机 |
| **Agent Config API 扩展** | `internal/server/handler/agent.go` | 1 天 | tools/memory_enabled/execution_mode |
| **数据库扩展** | `migrations/` | 1 天 | agent_tools 表、agent_runs 表 |
| **Agent 执行记录 API** | `internal/server/handler/agent.go` | 1 天 | GET /agents/{id}/runs |
| **Agent 管理页改造** | `frontend/` | 3 天 | Memory 查看/编辑、工具配置、执行模式切换 |
| **Agent 活动历史前端** | `frontend/` | 2 天 | 活动列表、状态显示 |
| **安全审查** | | 1 天 | 工具安全等级、路径限制 |
| **测试** | | 2 天 | Lifecycle E2E 测试 |

**关键里程碑**: Agent 生命周期管理、记忆持久化、工具调用 (K-05/K-06/K-07)

### 5.4 v1.3 详细计划（6 周）

#### Week 9-10: 文件上传 + 消息搜索

| 任务 | 模块 | 工作量 |
|------|------|--------|
| 文件上传后端 | `internal/server/handler/file.go` | 2 天 |
| 文件上传前端 | `frontend/` | 2 天 |
| 全文搜索后端 | `internal/server/handler/search.go` | 2 天 |
| 搜索前端 UI | `frontend/` | 2 天 |

#### Week 11-12: 多 Agent 协作 + 用量统计

| 任务 | 模块 | 工作量 |
|------|------|--------|
| Agent-to-Agent 协作协议 | trigger_chain + @mention 逻辑 | 3 天 |
| 协作可视化前端 | `frontend/` | 2 天 |
| 用量统计后端 | usage_records 表 + API | 2 天 |
| 用量统计前端 | `frontend/` | 2 天 |

#### Week 13-14: UI 打磨 + 集成测试

| 任务 | 模块 | 工作量 |
|------|------|--------|
| UI 打磨 (Phase 3) | 动画、微交互、键盘快捷键 | 3 天 |
| 频道归档 | 后端 + 前端 | 2 天 |
| 全功能回归测试 | | 3 天 |

### 5.5 v2.0 方向性计划（8 周）

| 功能 | 模块 | 工作量 | 优先级 |
|------|------|--------|--------|
| 审批流 | Tool 安全等级 + 人类审批 | L | P2 |
| 定时任务 (Autopilot) | cron 触发 + Agent 自动执行 | M | P2 |
| Trace Engine | JSONL + daemon/traces/ | M | P2 |
| Agent 市场 | 预配置模板 + 社区共享 | L | P2 |
| Machine Lock | daemon/lock.json | S | P2 |
| OAuth/SSO | GitHub/Google/Email | M | P2 |

### 5.6 工作量总结

| 版本 | 时间 | 关键交付 | 工作量估计 |
|------|------|---------|-----------|
| **v1.1** | Week 1-4 (4周) | **[已修正] Backend 接口 + Workspace + Memory + Prompt + UI风格** | 240-280h |
| **v1.2** | Week 5-14 (10周) | 多Backend + Skills + Tools + Lifecycle + 文件 + 搜索 + 多Agent | 600-680h |
| **v2.0** | Week 15-22 (8周) | 审批流 + 定时 + Trace + 市场 + Machine Lock + OAuth | 400-440h |
| **总计** | **22 周** | | **1240-1400h** |

> 假设团队 6 人 (rd1 + rd2 + fe1 + fe2 + qa1 + arc)，每周可用 6*5*6=180h。

---

## 6. 附录: 关键决策记录 ADR

### ADR-20260511-001: 使用 stdin/stdout 管道而非 MCP [已修正]

**背景**: 第 1 版认为 Multica 使用 MCP 协议层，Solo 应该"先不用 MCP，v2.0 评估"。[已修正] Multica 实际使用 **exec.Command → stdin JSON → stdout JSONL**，11 个 Backend 全部通过进程管道通信，MCP 仅作为 `--mcp-config` CLI 参数。

**决策**: **始终不使用 MCP 作为通信层**。Agent CLI 与 Daemon 之间通过 stdin/stdout 管道通信，参照 Multica 的 Backend 接口设计。

**理由**:
1. Multica 已验证 11 种 CLI Agent 通过 stdin/stdout 管道统一抽象可行
2. MCP 在 Solo 架构中增加不必要的中间层（Daemon ↔ MCP Server ↔ Agent CLI）
3. Agent CLI 原生支持 stdin/stdout 管道（stream-json, JSON-RPC 2.0 等），不需要额外转换
4. 如需 Agent 访问外部 MCP 工具，通过 CLI 的 `--mcp-config` 参数注入 MCP Server 配置，而非自建 MCP 网关层

**后果**:
- 通信层比 MCP 方案更轻量（少一个网络跳转）
- 需要为每种 CLI 实现不同的参数构造 + 输出解析（但 Multica 已有完整的参考实现）
- v2.0 如需插件系统，通过 `--mcp-config` 透传而非自建 MCP Server

**备选方案**: 使用 MCP 作为通信层（[已修正] 引入不必要的中间层，增加复杂度）

### ADR-20260511-002: Agent 记忆存在文件系统而非数据库

**背景**: Agent 记忆需要持久化，有文件系统（MEMORY.md）和数据库（PostgreSQL agent_memories 表）两种方案。

**决策**: 使用文件系统 `~/.solo/agents/<id>/MEMORY.md`。

**理由**:
1. 记忆本质是纯文本 Markdown，文件系统更直接
2. Claude Code CLI 可以直接读写 MEMORY.md（Agent 自维护）
3. 文件系统模式与 Slock.ai 一致（已验证）
4. 数据库存储需要额外的序列化/反序列化

**后果**: 记忆不在 Server DB 中，无法通过 REST API 直接搜索。需要额外 API 转发读写请求。

**备选方案**: PostgreSQL 存储（更标准化，可搜索，但增加了与文件系统的同步复杂度）。

### ADR-20260511-003: Agent 级 Workspace 而非 Task 级

**背景**: [已修正] Multica 使用 per-task workspace (`~/multica_workspaces/<ws_id>/<task_id>/workdir/`)，Slock 使用 per-agent workspace。

**决策**: 使用 Agent 级 workspace (`~/.solo/agents/<id>/`)。

**理由**:
1. Solo 的 Agent 在频道中连续对话，不需要每次重新创建 workspace
2. 每 Agent 一个目录管理简单，不需要 GC 机制
3. MEMORY.md 在同一路径，Agent 能自然跨会话读写
4. 匹配 Solo 的产品模型（Agent = 团队成员，不是一次性的任务执行器）

**后果**: 需要处理 Agent 级别的磁盘清理（长时间积累后）。可以在 v2.0 引入简单的空间管理策略。

**备选方案**: 每任务 workspace（更隔离，但增加 GC 复杂度，不适合频道对话模式）。

### ADR-20260511-004: 不使用 Multica 的 Issue 工单系统

**背景**: Multica 围绕 Issue (todo→in_progress→in_review→done) 组织 Agent 工作。

**决策**: 不引入 Issue 工单系统，保持频道消息流模式。

**理由**:
1. Solo 的核心模式是频道消息流（类 Slack），不是工单看板（类 Jira）
2. 引入 Issue 会分裂用户体验——Solo 应该聚焦"协作实时性"而非"任务管理"
3. Agent 的"任务"应该在频道中以消息 + 线程方式表达，不是独立的工单
4. 团队精力有限，聚焦频道模式比模仿工单系统更有利于产品差异化

**后果**: Agent 无法做"任务认领""状态跟踪"等精细管理。但可以通过消息标签、线程标记等轻量方式替代。

**备选方案**: 引入 Issue 系统（偏离产品定位，增加分裂复杂度）。

### ADR-20260511-005: Backend 接口复用 Multica 设计模式 + 具体代码 [已修正]

**背景**: Multica 的 Backend 接口 (`Execute(ctx, prompt, opts) -> Session`) 设计成熟，且有完整的 11 种实现代码。

**决策**: [已修正] **不仅复用接口设计模式，还直接复用 Multica 的 CLI 参数构造 + stream-json 解析等协议级代码**，但 Solo 调度层（TaskRouter + WorkspaceManager + PromptBuilder）需要自己实现。

**理由**:
1. Multica 的 Backend 接口设计清淅简洁，是经过验证的设计
2. **CLI 参数构造和 stream-json 解析是协议级的，不耦合 Multica 业务**——直接复用风险低、收益高
3. 但 Solo 的输入结构（`RunRequest` 含 channel context, messages, trigger type）与 Multica（`prompt string`）不同——调度层需要自己实现
4. 直接复用 Multica 的 Backend 实现代码可以节省 2-4 天开发时间

**复用范围**:
| 复用 | 不复用 |
|------|--------|
| `Backend` 接口定义 (`Execute(ctx, prompt, opts) → Session`) | TaskRouter / PromptBuilder |
| CLI 参数构造 (`buildClaudeArgs`) | WorkspaceManager |
| stream-json 解析逻辑 | MemoryManager |
| Backend 工厂模式 | 与 Server 的通信协议 (HTTP/SSE) |
| 消息规范化 (`Message{Type, Content, ...}`) | 鉴权/配置 |

**后果**: 直接复用 Multica 代码减少重复开发。Solo 调度层独立实现确保适配频道/消息模型。

**备选方案**: 只复用设计模式，全部自己实现（增加 3-5 天开发工作量）；

---

## 文档结束

> 本优化方案与以下文档对齐:
> - `product-roadmap-v2.md` — 产品路线图 v1.0 → v2.0
> - `architecture-review-v2.md` — 架构深度评估报告
> - `frontend-redesign-v2.md` — 前端 UI 改造方案
> - Slock.ai 架构文档 — 竞品分析参考
> - Multica 架构文档（实际源代码） — 技术方案参考 [已修正]

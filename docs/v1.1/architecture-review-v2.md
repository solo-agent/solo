# Solo 架构深度评估报告 v2

> 版本: 2.0
> 作者: arc agent (架构师)
> 日期: 2026-05-11
> 范围: Solo 当前架构评估 + multica 深度分析 + Agent 层改进方案

---

## 目录

1. [当前架构评估](#1-当前架构评估)
2. [multica 深度分析](#2-multica-深度分析)
3. [关键差距对比](#3-关键差距对比)
4. [Q1: 是否正确复用了 multica？](#4-q1-是否正确复用了-multica)
5. [Q2: Agent prompt 为什么没用？](#5-q2-agent-prompt-为什么没用)
6. [Q3: 是否需要 Solo 自己的 workspace 和 memory？](#6-q3-是否需要-solo-自己的-workspace-和-memory)
7. [架构改进方案](#7-架构改进方案)
8. [实施路线图](#8-实施路线图)
9. [附录：关键代码对比](#9-附录关键代码对比)

---

## 1. 当前架构评估

### 1.1 分层架构总览（现状）

```
Solo 当前架构
─────────────────────────────────────────────────────────
Layer 1: 前端层 (Next.js 16 App Router)
  - app/       — 页面路由
  - components/ — UI 组件
  - lib/       — API 客户端封装

Layer 2: 后端层 (Go Server on Chi)
  - handler/   — REST handler (auth, channel, message, agent, thread, dm)
  - service/   — 业务逻辑 (agent, channel, daemon, thread, mention)
  - ws/        — WebSocket Hub (复用 multica 架构)
  - middleware/ — auth JWT, CORS, logging, ratelimit, security

Layer 3: Agent 执行层 (Go Daemon 独立进程)
  - 接收 HTTP task → 调用 llm.Provider.CompleteStream() → SSE 推回

Layer 4: 数据层 (PostgreSQL 16)
  - migrations/ — golang-migrate
  - internal/db/ — pgxpool 配置
```

**合理性判断: 整体分层合理，但 Agent 执行层过薄。**

| 层面 | 评价 | 风险 |
|------|------|------|
| 前端层 | 合理的 App Router 分层，SWR + WebSocket 分工明确 | 无显著风险 |
| 后端层 | handler/service/ws 三层清晰，middleware 链完整 | 无显著风险 |
| 中间件层 | JWT auth + CORS + logging + ratelimit + security headers 齐全 | 无显著风险 |
| WS 层 | Hub 模式正确，支持 channel-scoped 订阅 | 无显著风险 |
| **Agent 接口层** (pkg/agent) | **定义了 AgentRuntime 接口但无实际实现** | **高风险** |
| **LLM Provider 层** (pkg/llm) | **定义了 Provider 接口，local.go 实现过于简陋** | **高风险** |
| **Daemon 层** (cmd/daemon) | **直接调用 llm.Provider.CompleteStream，无 Agent Workspace 概念** | **高风险** |
| 数据层 | 表结构设计完整，索引合理 | 无显著风险 |

### 1.2 可独立扩展的模块

| 模块 | 可独立扩展性 | 理由 |
|------|-------------|------|
| **Server** | 好 | 无状态设计，可水平扩展；唯一状态在 WS Hub 内存中 |
| **Daemon** | 好 | 独立进程，可通过 DaemonManager 注册多实例 |
| **前端** | 好 | SPA 架构，独立部署 |
| **数据库** | 一般 | 单库无分片，但预留了读写分离接口 |

### 1.3 瓶颈识别

| 瓶颈 | 类型 | 严重程度 | 说明 |
|------|------|---------|------|
| **WS Hub 单点** | 部署架构 | 中 | 单实例时所有 WebSocket 连接绑定到一个进程；跨实例广播需 Redis |
| **Daemon 执行无隔离** | 资源隔离 | **高** | 每个任务在同一进程中执行，共享内存和 LLM Provider 实例 |
| **local.go CLI 调用阻塞** | 执行模型 | **高** | `exec.Command("claude", "-p", prompt, "--print")` 阻塞等待完整输出，不支持真正的流式 |
| **System Prompt 拼接** | Agent 配置 | **高** | buildPrompt 只是把 system_prompt 拼到 prompt 前，Claude Code CLI 的 --append-system-prompt 被忽略了 |
| **无 Agent 隔离** | 数据安全 | 中 | 所有 Agent 共享同一个 Daemon 进程，无 workspace 隔离 |
| **LLM API Key 透明** | 安全 | 中 | API Key 在 daemon 进程环境中以明文传递 |

### 1.4 安全性评估

| 维度 | 现状 | 评价 |
|------|------|------|
| **认证** | JWT access(15min) + refresh token + bcrypt 密码 | 合理，与 ARCHITECTURE.md 一致 |
| **授权** | handler 级别检查 owner_id；频道成员级别 RBAC | 基本覆盖，但缺少"频道内角色"不同权限的细粒度控制 |
| **数据隔离** | 频道成员隔离；消息按 channel_id 隔离 | 合理 |
| **WebSocket 安全** | 连接时 JWT 验证 | 合理 |
| **Daemon 通信安全** | Internal-Token 简单拦截 | 不够严格；应该用预共享密钥 + 签名 |
| **XSS/注入防护** | 参数化查询 (pgx) | 合理 |
| **速率限制** | 认证 10 req/min，其他 100 req/s | 合理 |
| **Agent 工具安全** | 未实现（MVP 范围外） | N/A |

**安全性结论**: 整体安全基线合理，但 Daemon 内部通信过于信任——Internal-Token 仅通过 JWT Secret 派生，无独立 rotation 机制。

---

## 2. multica 深度分析

### 2.1 Agent 管理系统

multica 的 Agent 管理核心在于 `server/internal/daemon/` 包，其设计远比 Solo 复杂：

**Agent 注册方式**:
- 启动时自动探测系统 PATH 中的 CLI 二进制
- 支持 11 种 agent 类型: claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor, kimi, kiro
- 通过 `MULTICA_CLAUDE_PATH`, `MULTICA_CODEX_PATH` 等环境变量覆盖路径
- 通过 `MULTICA_CLAUDE_MODEL` 等环境变量设置默认模型

**模型配置**: 每个 agent 类型有 `AgentEntry` 结构体:
```go
type AgentEntry struct {
    Path  string // 二进制路径
    Model string // 默认模型
}
```

### 2.2 Agent 执行隔离

multica 使用 `execenv` 包实现完整的执行隔离：

**目录结构** (`~/multica_workspaces/`):
```
~/multica_workspaces/
  └── {workspace_id}/
      └── {task_id_short}/
          ├── workdir/              # Agent 工作目录 (Cwd)
          │   └── .agent_context/
          │       └── issue_context.md  # 任务上下文
          ├── output/               # 输出产物
          ├── logs/                 # 执行日志
          └── .gc_meta.json         # GC 元数据
```

**隔离机制**:
- 每个 task 有独立的 `workdir`
- Agent CLI 以该目录为 `Cwd` 启动
- 通过 `.claude/skills/`, `.github/skills/`, `.opencode/skills/` 等 Provider 原生路径注入技能
- Git worktree 管理 (通过 `repocache` 包)
- GC 机制自动清理过期 workspace (GCTTL=24h)
- Per-task `CODEX_HOME` 隔离 (仅 Codex)

### 2.3 Agent Backend 接口 (`pkg/agent/`)

**核心接口 `Backend`**:
```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}
```

与 Solo 的 `AgentRuntime` 接口对比：

| 维度 | multica `Backend` | Solo `AgentRuntime` |
|------|------------------|-------------------|
| 输入 | `prompt string` + `ExecOptions` | `RunRequest` 结构体 |
| 输出 | `Session` (streaming channel) | `RunResponse` (直接结果) 或 channel |
| 流式 | 原生支持（`Session.Messages` channel） | 需通过 `Stream()` 接口 |
| 配置 | `Config{ ExecutablePath, Env, Logger }` | 无配置对象 |
| 实现数 | **11 个** (claude, codex, copilot, etc.) | **0 个实际实现** |
| Prompt 构造 | 由 daemon 的 `BuildPrompt()` 负责 | 由 `LocalProvider.buildPrompt()` 简单拼接 |

**11 种 Backend 实现一览**:

| Backend | 文件名 | 通信协议 | 核心参数 |
|---------|--------|---------|---------|
| Claude | claude.go | stream-json over stdin | -p, --output-format, --input-format, --permission-mode, --append-system-prompt, --model |
| Codex | codex.go | JSON-RPC 2.0 over stdio | --listen stdio:/ |
| Copilot | copilot.go | JSON over stdio | 最少配置（GitHub 认证） |
| OpenCode | opencode.go | JSON lines | 模型自发现 |
| OpenClaw | openclaw.go | ACP protocol | agents list 模型枚举 |
| Hermes | hermes.go | ACP protocol | session/new 模型发现 |
| Gemini | gemini.go | JSON lines | -m 模型选择 |
| Pi | pi.go | JSON over stdio | --list-models 发现 |
| Cursor | cursor.go | stream-json | cursor-agent 协议 |
| Kimi | kimi.go | ACP protocol | session/new 模型发现 |
| Kiro | kiro.go | ACP protocol | session/new 模型发现 |

**每个 Backend 的职责**:
1. 构造 CLI 参数（`buildClaudeArgs`, `buildCodexArgs` 等）
2. 启动子进程（`exec.CommandContext`）
3. 解析 stdout 输出（stream-json / JSON-RPC / JSON lines）
4. 将事件转换为统一的 `Message` 类型
5. 阻塞等待进程退出，返回 `Result`

**关键设计**: `filterCustomArgs()` 机制——每个 Backend 定义一组 `BlockedArgs`，过滤掉用户自定义参数中会破坏通信协议的标志。例如 Claude 阻止用户覆盖 `-p`, `--output-format`, `--permission-mode` 等。

### 2.4 技能系统 (Skills)

multica 有完整的技能注入系统：

**技能注入路径** (Provider 原生):

| Provider | 路径 | 说明 |
|----------|------|------|
| Claude | `.claude/skills/{name}/SKILL.md` | Claude Code 原生发现 |
| Copilot | `.github/skills/{name}/SKILL.md` | 项目级技能 |
| OpenCode | `.opencode/skills/{name}/SKILL.md` | 原生发现 |
| Hermes | `.agent_context/skills/{name}/SKILL.md` | 通过 meta skill 注入 |
| Pi | `.pi/skills/{name}/SKILL.md` | 原生发现 |
| Cursor | `.cursor/skills/{name}/SKILL.md` | 原生发现 |
| Kimi | `.kimi/skills/{name}/SKILL.md` | 原生发现 |
| Kiro | `.kiro/skills/{name}/SKILL.md` | 原生发现 |

**技能内容**: 每个技能包含 `SKILL.md` + 支持文件，通过 `SkillContextForEnv` 结构体传递：
```go
type SkillContextForEnv struct {
    Name    string
    Content string
    Files   []SkillFileContextForEnv
}
```

**没有独立的 Agent memory 系统**。multica 的"记忆"通过以下方式实现：
- 任务上下文文件 (`.agent_context/issue_context.md`)
- Issue 评论历史（通过 `multica issue comment list`）
- 会话恢复 (通过 `--resume` flag 恢复 Claude Code session)

### 2.5 Daemon 架构对比

| 维度 | multica Daemon | Solo Daemon |
|------|---------------|-------------|
| **通信** | WebSocket (daemonws) + Server | HTTP + SSE |
| **任务来源** | 轮询 (poll) + WebSocket 推送 | HTTP push |
| **执行隔离** | execenv 完整隔离 | 无隔离 |
| **最大并发** | 20 个任务 | 10 个任务 |
| **LLM 调用** | 通过 `pkg/agent.Backend` (11 种 CLI) | 通过 `llm.Provider` (3 种 API) |
| **Prompt 构造** | `BuildPrompt()` 针对不同任务类型定制 | `buildPrompt()` 简单拼接 |
| **工作目录** | 每个 task 独立 workdir | 无 |
| **技能注入** | 完整的 skills 系统 | 无 |
| **MCP 支持** | 完整 (--mcp-config) | 无 |
| **GC 清理** | 自动过期清理 (24h TTL) | 无 |
| **代码质量** | 成熟，有完整测试 | 初期实现，测试覆盖率低 |

### 2.6 Agent Prompt 管理

multica 的 `BuildPrompt()` 根据任务类型生成不同 prompt：

| 任务类型 | Prompt 模板 | 关键内容 |
|----------|------------|---------|
| Issue 任务 | `BuildPrompt` | "Your assigned issue ID is: X. Run `multica issue get X`" |
| 评论触发 | `buildCommentPrompt` | 嵌入触发评论内容 + Agent 间回复规则 |
| Chat 任务 | `buildChatPrompt` | 简单转发用户消息 |
| Quick Create | `buildQuickCreatePrompt` | 详细的 issue 创建指南 |
| Autopilot | `buildAutopilotPrompt` | 根据 autopilot 配置内容 |

**核心原则**: Prompt 保持最小——详细的 agent 行为和配置通过 `execenv.InjectRuntimeConfig` 注入到 `AGENTS.md` / `CLAUDE.md` 中，由 Agent CLI 原生加载。

---

## 3. 关键差距对比

### 3.1 `pkg/agent/` 接口差异

```go
// ── Solo 当前 ──
type AgentRuntime interface {
    Run(ctx context.Context, req *RunRequest) (*RunResponse, error)
    Stream(ctx context.Context, req *RunRequest) (<-chan StreamEvent, error)
    Stop(agentID string) error
}

// ── multica ──
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}
```

**关键差异**:
1. Solo 的 `RunRequest` 包含 `Messages []Message` —— 这是"LLM API 风格"的请求格式，适用于直接调用 API (OpenAI/Anthropic SDK)
2. multica 的 `Backend` 接收 `prompt string` —— 这是"CLI Agent 风格"，因为 CLI agent (claude code/codex) 期望的是单段文本 prompt
3. Solo 的 `StreamEvent` 只有 `token/tool_call/error/done` 四种类型
4. multica 的 `Message` 有 `text/thinking/tool-use/tool-result/status/error/log` 七种类型

### 3.2 LLM 调用方式差异

```go
// ── Solo local.go 当前实现 ──
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    // buildPrompt: 把 system_prompt + 消息拼接成文本
    prompt := p.buildPrompt(req)
    cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt, "--print")
    // 等待 CLI 执行完毕，一次性返回 stdout
}

// ── multica claudeBackend 实现 ──
func (b *claudeBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
    args := []string{
        "-p",
        "--output-format", "stream-json",    // 流式 JSON 输出
        "--input-format", "stream-json",      // 流式 JSON 输入
        "--verbose",
        "--permission-mode", "bypassPermissions",  // 自主模式
    }
    if opts.Model != "" {
        args = append(args, "--model", opts.Model)
    }
    if opts.SystemPrompt != "" {
        args = append(args, "--append-system-prompt", opts.SystemPrompt)  // 正确使用
    }
    // stream-json 解析: 实时解析 assistant/user/system/result/log 事件
}
```

### 3.3 Prompt 拼接方式差异

```go
// ── Solo buildPrompt (错误做法) ──
func (p *LocalProvider) buildPrompt(req *CompletionRequest) string {
    b.WriteString(req.SystemPrompt)     // ❌ 直接拼到 prompt 开头
    b.WriteString("User: " + msg.Content)
    b.WriteString("Assistant:")          // 硬编码结束标记
}

// ── multica BuildPrompt (正确做法) ──
// 只构造任务指令 prompt
func BuildPrompt(task Task) string {
    b.WriteString("Your assigned issue ID is: ...")
    // system_prompt 通过 --append-system-prompt 传入
    // 或通过 AGENTS.md / CLAUDE.md 注入
}
```

---

## 4. Q1: 是否正确复用了 multica？

**结论: 未正确复用。**

### 4.1 当前复用状态

| multica 模块 | 复用状态 | 评价 |
|-------------|---------|------|
| `internal/realtime/hub.go` | 已复用 | 正确 |
| `internal/auth/jwt.go` | 已复用 | 正确 |
| `pkg/agent/agent.go` | 仅接口定义 | **接口签名不同，无法直接复用 Backend 实现** |
| `pkg/llm/` | 已复制 3 个 Provider | 但 local.go 过于简化，失去了 CLI Agent 全部能力 |
| `internal/daemon/` | 未复用 | Solo 自己实现的 daemon 过于简单 |
| `internal/daemonws/` | 未复用 | Solo 用 HTTP+SSE 替代了 WebSocket 通信 |

### 4.2 为什么不能直接复用

1. **接口签名不兼容**: Solo 的 `AgentRuntime` 接口接收 `RunRequest` (含 `[]Message`)，而 multica 的 `Backend` 接收 `prompt string` + `ExecOptions`。要复用 claude.go/codex.go 等实现，需要修改接口或者加适配层。

2. **Daemon 架构不同**: multica daemon 通过 WebSocket 与 server 通信，使用轮询拉取任务；Solo daemon 通过 HTTP + SSE 被动接收任务。两套通信协议不同。

3. **上下文管理不同**: multica 通过 `execenv` 管理 task 上下文（文件系统注入）；Solo 通过 `Messages []Message` 传递上下文。这是两种完全不同的设计范式。

### 4.3 正确复用策略

不应直接复制 multica 的 `pkg/agent/`，而应:

1. **复用 Backend 接口设计模式**（而非代码）—— Solo 需要自己的 `Backend` 实现，适配 Solo 的 `RunRequest` 结构
2. **复用 claude.go 的 CLI 参数构造逻辑**——特别是 `--output-format stream-json`, `--append-system-prompt`, `--permission-mode bypassPermissions`
3. **复用 execenv 的 workspace 隔离设计**——需要为 Solo 实现 `~/.solo/agents/<agent-id>/workspace/` 目录结构
4. **复用 BuildPrompt 的任务分类逻辑**——Issue / Chat / Mention 等不同触发方式应有不同的 prompt 模板
5. **复用模型发现机制**——`models.go` 的静态 + 动态模型发现

---

## 5. Q2: Agent prompt 为什么没用？

**结论: Solo 的 system_prompt 在 LocalProvider 路径上基本被忽略，在 API Provider 路径上(部分)起作用。**

### 5.1 问题根因

**问题路径 A: `local.go` (LocalProvider)**

```go
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    prompt := p.buildPrompt(req)       // ❌ system_prompt 拼入 prompt 文本
    cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt, "--print")
    // ❌ 没有 --append-system-prompt 标志
    // ❌ Claude Code CLI 的 -p 模式启动后，传入的是一个"完整 prompt"，
    //    system_prompt 只是其中的一段文本，没有特殊意义
    // ❌ Claude Code CLI 有自己预配置的 prompt (通过 CLAUDE.md / AGENTS.md)
    //    这些文件中的 prompt 优先级高于通过 -p 传入的文本
}
```

**问题路径 B: `anthropic.go` / `openai.go`（API Provider — 正常工作）**

通过 `anthropic.go` 的 API Provider 路径调用时：
```go
// pkg/llm/anthropic.go (推测)
func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    resp, err := p.client.CreateMessage(ctx, &anthropic.MessageCreateParams{
        System: anthropic.NewStringSystemPrompt(req.SystemPrompt),  // ✅ 正确传递给 API
        Messages: convertMessages(req.Messages),
    })
}
```

这是正确的——system_prompt 被正确地作为 API 的 system parameter 传递。所以**system_prompt 在通过 anthropic/openai API 路径调用时是起作用的**。

**问题路径 C: 缺少 `--append-system-prompt` 标志**

当使用 Claude Code CLI 时，正确的调用方式是:
```bash
claude -p "task prompt" --append-system-prompt "Your system_prompt here"
```

但 Solo 的 `local.go` 没有使用 `--append-system-prompt`。这意味着 **如果用户选择"local" provider，system_prompt 无效**。

### 5.2 影响范围

| Provider 类型 | System Prompt 是否生效 | 说明 |
|---------------|----------------------|------|
| `anthropic` | 是 | 通过 API system parameter |
| `openai` | 是 | 通过 API system message |
| `local` (当前) | **否** | 拼入 prompt 文本被忽略 |

### 5.3 为什么被忽略

Claude Code CLI (claude) 的 `-p` 模式设计为接收一个"任务 prompt"，它会将其放入 $CURRENT_TASK 变量。Agent 的 system_prompt / persona 是通过 `--append-system-prompt` 或 `CLAUDE.md` 文件注入的。

**Claude Code CLI 的 prompt 优先级**:
1. `CLAUDE.md` (项目根目录) — 最高优先级
2. `--append-system-prompt` 标志
3. `.claude/skills/` 中的 skills
4. `-p` 传入的 prompt — 最底层，作为当前任务的上下文

### 5.4 正确做法

```go
// 正确实现
args := []string{
    "-p", prompt,
    "--output-format", "stream-json",
    "--input-format", "stream-json",
    "--permission-mode", "bypassPermissions",
}
if systemPrompt != "" {
    args = append(args, "--append-system-prompt", systemPrompt)  // ✅ 正确
}
if model != "" {
    args = append(args, "--model", model)
}
```

---

## 6. Q3: 是否需要 Solo 自己的 workspace 和 memory？

**结论: 是的，非常重要。**

### 6.1 为什么需要 workspace

**当前现状**: 所有 Agent 共享同一个 Daemon 进程，无文件系统隔离。

**引发的问题**:
1. 多 Agent 同时工作时，文件系统无隔离——Agent A 创建的临时文件可能被 Agent B 访问
2. 无工作目录管理——Agent 不知道自己的"工作空间"在哪里
3. 无持久化上下文——Agent 重启后丢失历史状态
4. 无技能/工具注入点——无法在 Agent 的工作目录中放置配置文件
5. 无法使用 Claude Code 等 CLI 的 full capability——这些 CLI 期望在项目目录中运行

### 6.2 为什么需要 memory

**当前现状**: 无 Agent 记忆系统。

**引发的问题**:
1. Agent 对每个消息都是"全新"的——不记得之前的对话
2. 频道消息历史做上下文——上下文窗口不断增长，成本高
3. 无跨频道记忆——Agent 在频道 A 中学到的信息无法在频道 B 中复用
4. 无长期记忆——Agent 无法记住用户偏好、过往决策

### 6.3 设计的两种记忆类型

| 类型 | 作用域 | 示例 | 存储 |
|------|--------|------|------|
| **频道上下文** | 频道级别 | "在这个频道中，我们之前讨论了 X 方案" | 消息历史 + summaries |
| **Agent 私有记忆** | Agent 级别 | "作为 code reviewer，我更喜欢严格的 linting" | `~/.solo/agents/<id>/MEMORY.md` |

### 6.4 推荐目录结构

```
~/.solo/
├── agents/
│   └── {agent-id}/
│       ├── config.json          # Agent 配置（JSON 完整版）
│       ├── MEMORY.md             # Agent 长期记忆（auto-updated）
│       ├── CLAUDE.md             # Claude Code 原生配置
│       ├── skills/               # Agent 可用技能（可选）
│       │   └── {skill-name}/
│       │       └── SKILL.md
│       ├── tools/                # Agent 可用工具配置
│       │   └── mcp-config.json   # MCP 服务器配置
│       ├── workspace/            # Agent 工作目录（Cwd for CLI agents）
│       │   └── ...
│       └── output/               # 执行输出产物
│
├── channels/
│   └── {channel-id}/
│       ├── summary.md            # 频道对话摘要
│       └── shared-context.md     # 共享上下文（人类编写的）
│
└── global-config.json            # 系统级别配置
```

**注意与 CLAUDE.md 的关系**:
- `~/.solo/agents/<agent-id>/CLAUDE.md` 用于注入 Claude Code CLI 原生配置
- 与项目根目录的 `CLAUDE.md` 不冲突——Claude Code 会合并两者

### 6.5 复用 Claude Code 执行能力而非配置

**核心原则**: Solo 管理 Agent 的配置（persona、tools、memory），但由 Claude Code CLI 执行。

- **Solo 的职责**: 决定 "Agent 是什么"（persona, prompt, tools, memory, workspace）
- **Claude Code 的职责**: 执行 "Agent 怎么做"（代码搜索、文件编辑、Git 操作）

```
┌──────────────────────────────────────────────────────────┐
│ Solo 平台                                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────┐  │
│  │ Agent 配置    │  │ Memory 系统   │  │ Workspace 管理   │  │
│  │ (persona)    │  │ (MEMORY.md)  │  │ (工作目录)       │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬────────┘  │
│         │                 │                    │           │
│         ▼                 ▼                    ▼           │
│  ┌─────────────────────────────────────────────────────┐  │
│  │ Agent Runtime (Backend Interface)                    │  │
│  │  → 参数: prompt + ExecOptions                        │  │
│  │  → 通过 CLI agent 执行                                │  │
│  └────────────────────┬────────────────────────────────┘  │
│                       │                                   │
└───────────────────────┼───────────────────────────────────┘
                        │
                        ▼
              ┌─────────────────────┐
              │ Claude Code CLI     │
              │ (执行能力)           │
              │ 搜索/编辑/Git/思考   │
              └─────────────────────┘
```

---

## 7. 架构改进方案

### 7.1 Agent Backend 体系重构

#### 7.1.1 目标

将 Solo 当前的 `AgentRuntime` + `LocalProvider` 架构迁移为 multica 风格的 `Backend` 接口体系，支持完整的 CLI Agent 能力。

#### 7.1.2 新接口设计

```go
// pkg/agent/backend.go — 新 Backend 接口

// Backend 是执行 Agent 任务的统一接口。
// 每个 CLI agent (claude, codex, etc.) 实现此接口。
type Backend interface {
    // Execute 运行一个 prompt 并返回用于流式读取的 Session。
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

// ExecOptions 配置一次执行。
type ExecOptions struct {
    AgentID      string          // Agent 唯一 ID
    ChannelID    string          // 频道 ID
    ThreadID     string          // 线程 ID（可选）
    Cwd          string          // Agent 工作目录（workspace 根）
    Model        string          // 模型名称
    SystemPrompt string          // 系统提示词（通过 --append-system-prompt 传入）
    MaxTurns     int             // 最大交互轮数
    Timeout      time.Duration   // 超时时间
    McpConfig    json.RawMessage // MCP 工具配置
    ExtraArgs    []string        // Daemon 级别默认参数
    CustomArgs   []string        // Agent 自定义参数
}

// Session 表示一个正在执行的 Agent 任务。
type Session struct {
    Messages <-chan Message  // 流式事件
    Result   <-chan Result   // 最终结果
}

// Message 是 Agent 执行过程中的一个统一事件。
type Message struct {
    Type      MessageType    // text, thinking, tool-use, tool-result, error
    Content   string
    Tool      string         // 工具名称
    CallID    string         // 工具调用 ID
    Input     map[string]any // 工具输入
    Output    string         // 工具输出
    SessionID string         // CLI session ID (用于恢复)
}

// Result 是任务完成后的最终结果。
type Result struct {
    Status     string  // completed, failed, aborted, timeout
    Output     string  // 输出文本
    Error      string
    DurationMs int64
    Usage      map[string]TokenUsage
}
```

#### 7.1.3 计划实现的 Backend

| Backend | 优先级 | 说明 |
|---------|--------|------|
| **Claude** | **P0** | 复用 multica `claude.go` 的参数构造和 stream-json 解析 |
| OpenAI | P1 | 复用 multica 失败，因为 OpenAI 无 CLI，需用 API SDK |
| Ollama | P1 | 本地模型，通过 API 调用 |
| Codex | P2 | 复用 multica `codex.go` 的 JSON-RPC 实现 |

#### 7.1.4 Claude Backend 实现关键点

```go
// pkg/agent/claude.go — Claude Backend 实现

func buildClaudeArgs(opts ExecOptions) []string {
    args := []string{
        "-p",                              // 非交互模式
        "--output-format", "stream-json",  // 流式 JSON 协议
        "--input-format", "stream-json",   // 流式 JSON 输入
        "--permission-mode", "bypassPermissions", // 自主模式
    }
    if opts.Model != "" {
        args = append(args, "--model", opts.Model)
    }
    if opts.MaxTurns > 0 {
        args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
    }
    if opts.SystemPrompt != "" {
        // ✅ 正确：使用 --append-system-prompt
        args = append(args, "--append-system-prompt", opts.SystemPrompt)
    }
    // 注入 MCP 工具配置
    if len(opts.McpConfig) > 0 {
        mcpPath := writeMcpConfigToTemp(opts.McpConfig)
        args = append(args, "--mcp-config", mcpPath)
    }
    // 过滤用户自定义参数（防止覆盖协议关键参数）
    args = append(args, filterCustomArgs(opts.CustomArgs, claudeBlockedArgs)...)
    return args
}
```

### 7.2 Agent Workspace 架构

#### 7.2.1 Workspace 管理器

```go
// pkg/agent/workspace.go — Agent Workspace 管理

type WorkspaceManager struct {
    rootDir string // ~/.solo/agents/
}

// Prepare 准备 Agent 的工作目录。
// 创建一个隔离的 workspace，注入配置和上下文文件。
func (wm *WorkspaceManager) Prepare(ctx context.Context, agentID, channelID string, context Context) (*Workspace, error) {
    ws := &Workspace{
        RootDir: filepath.Join(wm.rootDir, agentID),
        WorkDir: filepath.Join(wm.rootDir, agentID, "workspace"),
    }
    
    // 创建目录结构
    os.MkdirAll(ws.WorkDir, 0755)
    os.MkdirAll(filepath.Join(ws.RootDir, "output"), 0755)
    
    // 写入 Agent 配置（只写一次，后续增量更新）
    wm.writeConfig(ws.RootDir, agentID)
    
    // 写入频道上下文（每次触发重新写入）
    wm.writeChannelContext(ws.WorkDir, channelID, context)
    
    // 写入 CLAUDE.md（如果不存在）
    wm.writeCLAUDEConfig(ws.RootDir, agentID)
    
    return ws, nil
}
```

#### 7.2.2 Workspace 目录结构创建逻辑

```go
func (wm *WorkspaceManager) writeConfig(rootDir, agentID string) {
    // config.json — Agent 完整配置
    config := map[string]interface{}{
        "agent_id": agentID,
        "memory_file": filepath.Join(rootDir, "MEMORY.md"),
        "tools": []map[string]interface{}{
            {"name": "search", "enabled": true},
        },
    }
    writeJSON(filepath.Join(rootDir, "config.json"), config)
}

func (wm *WorkspaceManager) writeCLAUDEConfig(rootDir, agentID string) {
    path := filepath.Join(rootDir, "workspace", "CLAUDE.md")
    if _, err := os.Stat(path); os.IsNotExist(err) {
        content := fmt.Sprintf(`# Agent: %s

## Identity
You are an AI agent on the Solo platform. You participate in channel conversations.

## Guidelines
- Be concise and helpful
- Use markdown for formatting
- When you learn something important, update MEMORY.md

## Tools
- web_search: Search the internet
`, agentName)
        os.WriteFile(path, []byte(content), 0644)
    }
}

func (wm *WorkspaceManager) writeChannelContext(workDir, channelID string, context Context) {
    // 写入频道上下文到工作目录
    content := fmt.Sprintf(`# Channel Context

Channel ID: %s

## Recent Messages
%s

## Instructions
%s
`, channelID, formatMessages(context.Messages), context.Instructions)
    
    os.WriteFile(filepath.Join(workDir, ".solo-context.md"), []byte(content), 0644)
}
```

### 7.3 Agent Memory 架构

#### 7.3.1 Memory 管理器

```go
// pkg/agent/memory.go — Agent Memory 系统

type MemoryManager struct {
    rootDir string // ~/.solo/agents/
}

// GetMemory 读取 Agent 的 MEMORY.md
func (mm *MemoryManager) GetMemory(agentID string) string {
    path := filepath.Join(mm.rootDir, agentID, "MEMORY.md")
    data, _ := os.ReadFile(path)
    return string(data)
}

// AppendMemory 追加一条记忆
func (mm *MemoryManager) AppendMemory(agentID, entry string) error {
    path := filepath.Join(mm.rootDir, agentID, "MEMORY.md")
    f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
    if err != nil {
        return err
    }
    defer f.Close()
    
    timestamp := time.Now().UTC().Format(time.RFC3339)
    _, err = fmt.Fprintf(f, "\n- [%s] %s\n", timestamp, entry)
    return err
}

// GetChannelContext 读取指定频道的共享上下文
func (mm *MemoryManager) GetChannelContext(channelID string) string {
    path := filepath.Join(mm.rootDir, "channels", channelID, "shared-context.md")
    data, _ := os.ReadFile(path)
    return string(data)
}

// SetChannelContext 设置指定频道的共享上下文
func (mm *MemoryManager) SetChannelContext(channelID, content string) error {
    dir := filepath.Join(mm.rootDir, "channels", channelID)
    os.MkdirAll(dir, 0755)
    return os.WriteFile(filepath.Join(dir, "shared-context.md"), []byte(content), 0644)
}
```

#### 7.3.2 记忆更新触发时机

| 触发时机 | 操作 | 说明 |
|----------|------|------|
| Agent 每次响应后 | 可选更新 MEMORY.md | Agent 可自决是否记住关键信息 |
| 频道新消息 | 无（消息历史已做上下文） | 频道上下文已在 workspace 中 |
| 用户手动指定 | Appending to MEMORY.md | "记住我更喜欢英文回复" |
| Daemon 关闭前 | 持久化 MEMORY.md | 自动保存 |
| Agent 重新启动时 | 读取 MEMORY.md | MEMORY.md 注入到 system prompt |

#### 7.3.3 记忆的内容管理

MEMORY.md 的内容通过 `--append-system-prompt` 或 `CLAUDE.md` 注入到 Agent 的上下文中。

```
# Agent: CodeReviewer

## User Preferences
- 用户使用英文沟通
- 代码审查关注 point: 安全性 > 性能 > 代码风格

## Past Decisions (Last 10)
- [2026-05-10] 决定使用 JWT 替代 session 认证
- [2026-05-09] 推荐使用 pgx 而非 database/sql

## Important Facts
- 项目使用 Go 1.22+
- 项目使用 Chi 路由
```

### 7.4 Daemon 改造方案

#### 7.4.1 新 Daemon 架构

```
当前架构:
┌─────────────────────────────┐
│ Daemon                      │
│  llm.Provider.CompleteStream() │
│  → anthropic / openai / local │
│   (local: exec claude -p ... |  ← 太简陋)
└─────────────────────────────┘

新架构:
┌──────────────────────────────────────────────┐
│ Daemon                                        │
│  ┌──────────────────────────────────────────┐ │
│  │ TaskRouter                                │ │
│  │  → 解析任务类型 (chat/mention/auto)       │ │
│  │  → 构建 prompt (参考 multica BuildPrompt) │ │
│  │  → 选择 Provider Backend                 │ │
│  └──────────────┬───────────────────────────┘ │
│                 │                              │
│  ┌──────────────▼───────────────────────────┐ │
│  │ Backend (pkg/agent)                      │ │
│  │  → Claude (stream-json with stdin/stdout) │ │
│  │  → OpenAI (API SDK, fallback)            │ │
│  │  → Ollama (API SDK, fallback)            │ │
│  └──────────────┬───────────────────────────┘ │
│                 │                              │
│  ┌──────────────▼───────────────────────────┐ │
│  │ WorkspaceManager (pkg/agent/workspace)    │ │
│  │  → 为每个 task 创建隔离 workspace          │ │
│  │  → 注入 context files + memory            │ │
│  │  → 设置 Cwd 为 workspace./workdir         │ │
│  └──────────────────────────────────────────┘ │
└──────────────────────────────────────────────┘
```

#### 7.4.2 Task 处理流程

```
Server 下发 task
       │
       ▼
Daemon 接收 task
       │
       ▼
TaskRouter 分类:
  ├─ 普通频道消息 → BuildPrompt.chatPrompt("channel context", agentPersona)
  ├─ @提及触发 → BuildPrompt.mentionPrompt("You were @mentioned in...", channelContext)
  ├─ DM → BuildPrompt.dmPrompt("User is DMing you", dmContext)
  └─ 线程回复 → BuildPrompt.threadPrompt("Reply in thread", threadContext)
       │
       ▼
WorkspaceManager.Prepare(agentID, channelID, context)
  → 创建 ~/.solo/agents/<id>/workspace/
  → 写入 .solo-context.md (频道上下文)
  → 读取 MEMORY.md → 追加到 system prompt
  → 设置 Cwd = workspace 目录
       │
       ▼
backend.Execute(ctx, prompt, ExecOptions{Cwd, SystemPrompt, Model, ...})
  → Claude Backend: exec claude with stream-json
  → 实时解析 JSON events, 推送到 SSE channel
  → Agent 完成执行 → 读取 stdout (结果)
       │
       ▼
后处理:
  → 提取结果文本
  → 可选: MEMORY.md 更新（由 Agent 决定）
  → 可选: 频道上下文总结更新
  → SSE complete event → Server 持久化
```

### 7.5 Prompt 管理系统

#### 7.5.1 Prompt Builder

```go
// pkg/agent/prompt.go — Prompt 构造

// BuildChatPrompt 构造频道对话场景的 prompt
func BuildChatPrompt(ctx ChatContext) string {
    var b strings.Builder
    
    // Agent persona
    fmt.Fprintf(&b, "You are %s in a channel conversation.\n\n", ctx.AgentName)
    
    // Memory（如果有）
    if ctx.Memory != "" {
        b.WriteString("## Your Memory\n")
        b.WriteString(ctx.Memory)
        b.WriteString("\n\n")
    }
    
    // Channel context
    b.WriteString("## Channel Context\n")
    b.WriteString(fmt.Sprintf("Channel: #%s\n", ctx.ChannelName))
    if ctx.ChannelDescription != "" {
        b.WriteString(fmt.Sprintf("Description: %s\n", ctx.ChannelDescription))
    }
    b.WriteString("\n")
    
    // Recent messages
    b.WriteString("## Recent Messages\n")
    for _, msg := range ctx.RecentMessages {
        b.WriteString(fmt.Sprintf("%s: %s\n", msg.SenderName, msg.Content))
    }
    b.WriteString("\n")
    
    // Reply instruction
    b.WriteString("## Instructions\n")
    b.WriteString("Respond naturally to the latest message. If you have nothing relevant to add, don't reply.\n")
    
    return b.String()
}

// BuildMentionPrompt 构造 @提及 场景的 prompt
func BuildMentionPrompt(ctx MentionContext) string {
    var b strings.Builder
    
    fmt.Fprintf(&b, "You have been @mentioned in channel #%s.\n\n", ctx.ChannelName)
    b.WriteString("The user specifically wants your input. Prioritize responding.\n\n")
    
    if ctx.Memory != "" {
        b.WriteString("## Your Memory\n")
        b.WriteString(ctx.Memory)
        b.WriteString("\n\n")
    }
    
    // ... 其余上下文
    return b.String()
}

// BuildDMPrompt 构造私信场景的 prompt
func BuildDMPrompt(ctx DMContext) string {
    var b strings.Builder
    
    b.WriteString("You are in a private conversation with a user.\n")
    b.WriteString("Respond to their message naturally.\n\n")
    
    // ... DM 特定的 prompt
    return b.String()
}
```

### 7.6 多 Agent 协作架构

#### 7.6.1 当前状态
- 多个 Agent 加入同一频道 → 每条消息触发所有活跃 Agent
- Agent 之间不直接通信，都通过频道消息

#### 7.6.2 改进方向

**阶段 1 (当前 MVP)**: 保持简单
- 每个 Agent 独立响应
- 通过频道消息间接协作
- @提及 控制触发哪些 Agent

**阶段 2 (v0.3+)**: Agent 间直接协作
- Agent 可以调用另一个 Agent（通过频道 @提及 或内部 API）
- 主 Agent 可以委派子任务给副 Agent
- 支持 Agent 间结果传递

**阶段 3 (v1.0+)**: 编排能力
- Agent 工作流（A → B → C）
- Agent 任务队列
- Agent 市场

---

## 8. 实施路线图

### 8.1 改进优先级

| 优先级 | 改进项 | 工作量 | 影响 | 依赖 |
|--------|--------|--------|------|------|
| **P0** | 修复 local.go: `--append-system-prompt` | 1天 | 高（让 system_prompt 在 local 路径生效） | 无 |
| **P0** | 实现 claude.go Backend (stream-json + stdin) | 3天 | 高（完整 Claude Code CLI 能力） | 以上 |
| **P0** | 实现 WorkspaceManager | 3天 | 高（Agent 隔离基础） | 无 |
| **P1** | 实现 MemoryManager + MEMORY.md | 2天 | 中（长期记忆能力） | WorkspaceManager |
| **P1** | 实现 PromptBuilder (chat/mention/dm/thread) | 2天 | 中（统一的 prompt 管理） | 无 |
| **P1** | 封装 Backend 工厂 (agent.New) | 1天 | 中（Provider 选择逻辑） | claude.go |
| **P2** | 实现模型发现系统 | 2天 | 低 | Backend 工厂 |
| **P2** | 追加 Agent 间协作支持 | 5天 | 低（MVP 后） | 以上所有 |

### 8.2 实施步骤

#### Step 1: 修复 LocalProvider（1天）

**目标**: 让 `local.go` 正确传递 system_prompt。

**改动范围**: `pkg/llm/local.go`

```
- cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt, "--print")
+ args := []string{"-p", prompt, "--output-format", "stream-json", "--input-format", "stream-json"}
+ if req.SystemPrompt != "" {
+     args = append(args, "--append-system-prompt", req.SystemPrompt)
+ }
+ cmd := exec.CommandContext(runCtx, p.binary, args...)
```

**同时需要**: 将 `CompleteStream` 从同步包装改为真正的流式解析。

#### Step 2: 实现 WorkspaceManager（3天）

**目标**: 为每个 Agent 创建隔离的工作目录。

**新增文件**: `pkg/agent/workspace.go`

**核心功能**:
1. `Prepare(agentID, channelID, context) (*Workspace, error)` — 创建 workspace
2. `Cleanup(agentID)` — 清理 workspace
3. 自动创建 `~/.solo/agents/<id>/workspace/` 目录
4. 写入上下文文件（频道消息、memory、CLAUDE.md）

#### Step 3: 实现 Claude Backend（3天）

**目标**: 复用 multica 的 Claude Code CLI 集成方式。

**新增文件**: `pkg/agent/claude.go`

**迁移自 multica**:
- `buildClaudeArgs()` — 参数构造逻辑
- Claude SDK JSON 类型定义 (`claudeSDKMessage`, `claudeMessageContent`)
- `handleAssistant()`, `handleUser()` — stream-json 解析
- `trySend()` — 非阻塞 channel send
- `filterCustomArgs()` — 参数过滤

**Solo 特有适配**:
- 集成 `WorkspaceManager`（设置 Cwd）
- 集成 `PromptBuilder`
- 集成 `MemoryManager`（读取 MEMORY.md）

#### Step 4: 实现 PromptBuilder（2天）

**新增文件**: `internal/daemon/prompt.go`

**参考 multica**: `server/internal/daemon/prompt.go` 的 `BuildPrompt()` 模式

**Solo 特有 prompt 类型**:
- `BuildChatPrompt(ctx ChatContext)` — 频道对话
- `BuildMentionPrompt(ctx MentionContext)` — @提及
- `BuildDMPrompt(ctx DMContext)` — 私信
- `BuildThreadPrompt(ctx ThreadContext)` — 线程回复

#### Step 5: 实现 MemoryManager（2天）

**新增文件**: `pkg/agent/memory.go`

**核心功能**:
1. MEMORY.md 的读写
2. 记忆追加（Agent 自决或用户手动）
3. 频道共享上下文管理

### 8.3 文件改动清单

```
新增文件:
  pkg/agent/claude.go          — Claude Backend (stream-json)
  pkg/agent/workspace.go       — Workspace 管理器
  pkg/agent/memory.go          — Memory 系统
  pkg/agent/prompt.go          — Prompt 构造器
  pkg/agent/models.go          — 模型发现（可选）
  internal/daemon/prompt.go    — Daemon 端 prompt builder

修改文件:
  pkg/llm/local.go             — 修复 --append-system-prompt + 流式
  pkg/llm/llm.go               — 补充 Provider 接口（可选扩展）
  cmd/daemon/handler.go        — 集成 Backend + WorkspaceManager
  cmd/daemon/main.go           — 初始化 WorkspaceManager
  internal/server/service/agent.go  — 适配新 Backend

无需改动:
  internal/server/              — handler/middleware/ws 不变
  frontend/                     — 不变
  migrations/                   — 不变
  internal/auth/                — 不变
  internal/realtime/            — 不变
```

---

## 9. 附录：关键代码对比

### 9.1 Solo 当前 `local.go` vs multica `claude.go`

| 维度 | Solo local.go | multica claude.go |
|------|--------------|-------------------|
| 参数 | `-p prompt --print` | `-p --output-format stream-json --input-format stream-json --permission-mode bypassPermissions` |
| 输出格式 | 纯文本 stdout | JSON streams (assistant/user/system/result/log events) |
| 流式 | 假的（CompleteStream 包装 Complete） | 真流式（scanner loop） |
| SystemPrompt | 拼入 prompt 文本 | `--append-system-prompt` |
| 模型选择 | 不支持 | `--model` |
| 最大轮数 | 不支持 | `--max-turns` |
| 工具 | 不支持 | MCP config |
| 权限控制 | 不支持 | `--permission-mode bypassPermissions` |
| 会话恢复 | 不支持 | `--resume` |
| 自定义参数 | 不支持 | `ExtraArgs` + `CustomArgs` |
| 错误处理 | 简单 exit code | stderr tail capture + 错误分类 |

### 9.2 Solo 当前 `buildPrompt` vs multica `BuildPrompt`

```go
// Solo (错误)
func (p *LocalProvider) buildPrompt(req *CompletionRequest) string {
    b.WriteString(req.SystemPrompt)        // ❌ 拼到 prompt 开头
    for _, msg := range req.Messages {
        b.WriteString("User: " + msg.Content)
    }
    b.WriteString("Assistant:")            // ❌ 硬编码结束标记
}

// multica (正确)
func BuildPrompt(task Task) string {
    // 只构建任务指令
    b.WriteString("Your assigned issue ID is: ...")
    b.WriteString("Start by running `multica issue get ...`")
    // SystemPrompt 通过 --append-system-prompt 注入
    // Memory 通过 CLAUDE.md 注入
}
```

### 9.3 Solo 当前 Daemon 任务处理 vs multica Daemon

```go
// Solo (过于简单)
func (h *daemonHandler) processTaskStreaming(ctx context.Context, req runTaskRequest) {
    // 1. 构造 LLM request (Messages → []llm.Message)
    // 2. provider.CompleteStream(ctx, llmReq)  ← 直接调 API
    // 3. 流式推回 SSE
    // ❌ 没有 workspace
    // ❌ 没有 prompt builder
    // ❌ system_prompt 只是 llmReq 的一个字段
}

// multica (完整流程)
func (d *Daemon) executeTask(ctx context.Context, task Task) {
    // 1. execenv.Prepare(workspacesRoot, task)  ← 创建隔离 workspace
    // 2. prompt = BuildPrompt(task)             ← 根据任务类型构造
    // 3. backend = agent.New(provider, config)  ← 选择 Backend
    // 4. agent.Execute(ctx, prompt, opts{Cwd: workDir, SystemPrompt: agentInstructions})
    // 5. 流式处理 Message 事件 → 发送通知
    // 6. 处理 Result → 标记任务完成
}
```

---

## 总结

| 问题 | 结论 | 严重程度 | 优先级 |
|------|------|---------|--------|
| **Q1: 是否正确复用了 multica？** | 未正确复用。pkg/agent 只有接口定义无实现，local.go 过于简陋 | 高 | P0 |
| **Q2: Agent prompt 为什么没用？** | local.go 没有使用 `--append-system-prompt`，system_prompt 拼入 prompt 文本被忽略 | 高 | P0 |
| **Q3: 是否需要 workspace 和 memory？** | 是。缺少文件隔离和长期记忆是两个最大的架构空白 | 高 | P0 |

**核心改进**: 构建 Solo 自己的 Agent Backend 体系，复用 multica 的设计模式和关键代码（参数构造、stream-json 解析、参数过滤），但接口和架构适配 Solo 的频道模型。

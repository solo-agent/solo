# Solo v1.1 — 任务分解与迭代规划 (WBS)

> 版本: 1.1
> 创建日期: 2026-05-11
> 负责人: tpm agent
> 对齐文档: v2-optimization-plan.md v2.0, frontend-redesign-v2.md v3.0, architecture-review-v2.md v2.0, product-roadmap-v2.md v2.1
> 迭代周期: 4 周 (2026-05-12 ~ 2026-06-08)

---

## 1. 项目概览

### v1.1 目标

解决已知 P0 问题 (K-01/K-02/K-03/K-04/K-05)，建立 Agent 系统基础能力，并完成前端 Neubrutalism 风格改造。

| 问题 | 解决方式 | v1.1 交付物 |
|------|---------|------------|
| K-01: Agent system prompt 被覆盖 | Backend 接口重构 + InjectRuntimeConfig | CLAUDE.md 注入系统 |
| K-02: 本地 Agent 共享工作目录 | Workspace 隔离 | `~/.solo/agents/<id>/` 目录结构 |
| K-03: Local Provider 无真正流式 | Claude Backend stdin/stdout 管道 | 逐行流式输出 |
| K-04: 前端 UI 偏 SaaS | Neubrutalism 主题改造 | 粗野主义设计系统 |
| K-05: Agent 无 memory 隔离 | MemoryManager + MEMORY.md | Agent 级长期记忆 |

### 团队

| 角色 | 代号 | 技能栈 | 本周容量 |
|------|------|--------|---------|
| 后端开发 (Lead) | rd1 | Go + Chi + PostgreSQL + Agent 架构 | 40h/w |
| 后端开发 | rd2 | Go + PostgreSQL + Backend 接口 | 40h/w |
| 前端开发 (Lead) | fe1 | Next.js 16 + Tailwind + shadcn/ui + SWR | 40h/w |
| 前端开发 | fe2 | Next.js + Tailwind + shadcn/ui | 40h/w |
| 测试 | qa1 | API 测试 + E2E 测试 | 20h/w |
| 架构师 | arc | 代码审查 + 架构决策 | 20h/w |

**可用容量**: rd1(40h) + rd2(40h) + fe1(40h) + fe2(40h) + qa1(20h) + arc(20h) = **200h/w**

**缓冲策略**: 每迭代预留 20% 缓冲 (40h/周)，应对意外延迟和 Bug 修复。

---

## 2. 任务编号约定

```
SOLO-v11-{迭代周}-{序号}-{角色}
```

- `BE` = 后端任务 (rd1/rd2)
- `FE` = 前端任务 (fe1/fe2)
- `QA` = 测试任务 (qa1)

---

## 3. 任务总表 (Roadmap 视图)

| 任务 ID | 标题 | 角色 | 优先级 | 预估 | 关键路径 |
|---------|------|------|--------|------|---------|
| **Week 1: Backend 基础 + 前端主题** | | | | |
| SOLO-v11-W1-01-BE | 定义 Backend 接口 | rd1 | P0 | 8h | Y |
| SOLO-v11-W1-02-BE | 实现 WorkspaceManager | rd1 | P0 | 8h | Y |
| SOLO-v11-W1-03-BE | 实现 InjectRuntimeConfig | rd1 | P0 | 4h | Y |
| SOLO-v11-W1-04-BE | 实现 Claude Backend | rd2 | P0 | 16h | Y |
| SOLO-v11-W1-05-BE | 实现 Backend 工厂 | rd2 | P0 | 4h | N |
| SOLO-v11-W1-06-FE | 搭建 Neubrutalism 主题系统 | fe1 | P0 | 8h | Y |
| SOLO-v11-W1-07-FE | 改造基础 UI 组件 | fe1 | P0 | 8h | Y |
| SOLO-v11-W1-08-FE | 改造登录/注册页 | fe2 | P1 | 8h | N |
| SOLO-v11-W1-09-FE | 改造 Dashboard 布局 | fe2 | P0 | 8h | Y |
| **Week 2: Agent 能力 + 消息编辑** | | | | |
| SOLO-v11-W2-01-BE | 实现 PromptBuilder | rd1 | P0 | 16h | Y |
| SOLO-v11-W2-02-BE | Daemon TaskRouter 集成 | rd1 | P0 | 8h | Y |
| SOLO-v11-W2-03-BE | 实现 MemoryManager | rd2 | P0 | 8h | Y |
| SOLO-v11-W2-04-BE | 消息编辑/删除 API | rd2 | P1 | 8h | N |
| SOLO-v11-W2-05-FE | 消息组件 Neubrutalism 改造 | fe1 | P0 | 8h | Y |
| SOLO-v11-W2-06-FE | 消息编辑/删除 UI | fe1 | P1 | 8h | N |
| SOLO-v11-W2-07-FE | Agent 管理页改造 | fe2 | P1 | 8h | N |
| SOLO-v11-W2-08-FE | 新增 UI 组件 (badge/toggle/checkbox/codeblock) | fe2 | P1 | 8h | N |
| **Week 3: 集成 + 流式 + 打磨** | | | | |
| SOLO-v11-W3-01-BE | Memory 摘要生成与自动更新 | rd1 | P1 | 8h | N |
| SOLO-v11-W3-02-BE | PromptBuilder + MemoryManager 集成 | rd1 | P0 | 8h | Y |
| SOLO-v11-W3-03-BE | Machine Lock 实现 | rd2 | P2 | 4h | N |
| SOLO-v11-W3-04-BE | Claude Backend 流式改进 | rd2 | P0 | 8h | Y |
| SOLO-v11-W3-05-BE | 后端集成测试 | rd2 | P1 | 8h | N |
| SOLO-v11-W3-06-FE | Agent 消息 + 流式渲染 | fe1 | P0 | 8h | Y |
| SOLO-v11-W3-07-FE | 响应式布局 + 动画/微交互 | fe1 | P1 | 8h | N |
| SOLO-v11-W3-08-FE | 侧栏频道列表 + DM 改造 | fe2 | P1 | 8h | N |
| SOLO-v11-W3-09-FE | 空/加载/错误状态处理 | fe2 | P1 | 4h | N |
| **Week 4: 打磨 + E2E** | | | | |
| SOLO-v11-W4-01-QA | Daemon 端全链路测试 | qa1+rd1 | P0 | 16h | Y |
| SOLO-v11-W4-02-QA | 消息编辑/删除集成测试 | qa1+rd2 | P1 | 8h | N |
| SOLO-v11-W4-03-QA | Backend + Workspace + Memory E2E | qa1+rd1+rd2 | P0 | 16h | Y |
| SOLO-v11-W4-04-FE | 主题打磨 (颜色/阴影/字体统一) | fe1+fe2 | P1 | 8h | N |
| SOLO-v11-W4-05-FE | 可访问性 (skip link, focus, reduced motion) | fe1+fe2 | P1 | 8h | N |
| SOLO-v11-W4-06-FE | 全量页面 Neubrutalism 适配 | fe1+fe2 | P1 | 8h | N |

---

## 4. 详细任务分解

---

### Week 1 (2026-05-12 ~ 2026-05-18) — Backend 基础 + 前端主题

**Sprint Goal**: Backend 接口可执行、前端主题系统可用

---

#### SOLO-v11-W1-01-BE: 定义 Backend 接口

| 字段 | 值 |
|------|-----|
| **标题** | 定义 Backend 接口 + 核心类型 |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W1-04-BE, W2-01-BE, W2-02-BE） |
| **依赖** | 无 |

**描述**:
根据 v2-optimization-plan.md 和 architecture-review-v2.md 的设计，在 `pkg/agent/` 下定义新的 Backend 接口和核心类型。替换当前 `agent.go` 中的 `AgentRuntime` 接口。

**不做什么**: 不删除旧的 `AgentRuntime` 接口（rd2 的 W1-04-BE 完成后统一替换引用）。

**验收标准**:
- `pkg/agent/backend.go` 包含以下定义：
  - `type Backend interface { Execute(ctx, prompt, opts) (*Session, error); Name() string }`
  - `type ExecuteRequest struct` — 包含通道上下文、AgentID、消息历史、触发类型
  - `type ExecuteOptions struct` — AgentID, ChannelID, ThreadID, Cwd, Model, SystemPrompt, MaxTurns, Timeout, McpConfig, CustomArgs
  - `type Session struct` — ID, Status, Stream (`<-chan OutputChunk`), Error
  - `type OutputChunk struct` — Type (text/tool_call/error/done), Content, Meta
  - `type MessageType string` — text, thinking, tool-use, tool-result, error, log
  - `type Result struct` — Status, Output, Error, Usage, Duration
- 编译通过：`go build ./...`
- 所有类型有 Go doc 注释

**参考**: v2-optimization-plan.md §3.1 "Solo 的 Backend 设计", architecture-review-v2.md §7.1 "Agent Backend 体系重构", multica `server/pkg/agent/backend.go`

---

#### SOLO-v11-W1-02-BE: 实现 WorkspaceManager

| 字段 | 值 |
|------|-----|
| **标题** | 实现 WorkspaceManager |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W1-04-BE, W2-02-BE） |
| **依赖** | W1-01-BE |

**描述**:
实现 Agent 工作空间隔离管理器 `~/.solo/agents/<id>/`（per-Agent 级别），参考 architecture-review-v2.md §7.2 的设计。

**不做什么**: MEMORY.md 读写延迟到 W2-03-BE (MemoryManager)。Skills 目录系统延迟到 v1.2。

**验收标准**:
- `pkg/agent/workspace.go` 实现 `WorkspaceManager` 结构体和以下方法：
  - `Prepare(agentID, config) (*Workspace, error)` — 创建目录结构
  - `Cleanup(agentID) error` — 清理工作空间
  - `WorkspacePath(agentID) string`
- 创建的目录结构：
  ```
  ~/.solo/agents/<id>/
    ├── workspace/          # Agent 执行 CWD
    ├── output/             # 输出产物目录
    └── solo-config.json    # Agent 配置缓存
  ```
- 写入 `solo-config.json`（Agent 配置的本地缓存）
- 跨平台路径正确（Unix/Darwin 兼容）
- 单元测试覆盖：创建、重复创建（幂等）、清理、权限错误

**参考**: architecture-review-v2.md §6.4 "推荐目录结构" + §7.2 "Agent Workspace 架构"

---

#### SOLO-v11-W1-03-BE: 实现 InjectRuntimeConfig

| 字段 | 值 |
|------|-----|
| **标题** | 实现 InjectRuntimeConfig — CLAUDE.md 动态注入 |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 4h |
| **关键路径** | 是（下游依赖：W1-04-BE） |
| **依赖** | W1-02-BE (WorkspaceManager 的目录结构) |

**描述**:
在 WorkspaceManager 中实现 InjectRuntimeConfig 模式：每次 Agent 任务执行前动态写入 CLAUDE.md，包含 Agent 配置、Solo 协议规则和当前通道上下文。Agent CLI 原生读取此文件。

**验收标准**:
- WorkspaceManager 新增 `InjectConfig(agentID, channelID, triggerType) error` 方法
- 写入的 `workspace/CLAUDE.md` 包含：
  - Agent Identity（来自 system_prompt）
  - Solo Platform Rules（协议规则）
  - Channel Context（更新于每次执行）
- 不会覆盖 Agent 已有的手动修改（只在文件不存在或非执行期间写入）
- 与 W1-04-BE (Claude Backend) 集成：Backend 将 CWD 设置为 workspace/，Claude Code CLI 原生读取 CLAUDE.md
- 单元测试：文件写入验证、幂等性

**参考**: v2-optimization-plan.md §3.3 "生成的 CLAUDE.md 结构", architecture-review-v2.md §7.2.2 "WorkspaceManager writeCLAUDEConfig"

---

#### SOLO-v11-W1-04-BE: 实现 Claude Backend (stdin/stdout 管道)

| 字段 | 值 |
|------|-----|
| **标题** | 实现 Claude Backend — stream-json stdin/stdout |
| **角色** | rd2 |
| **优先级** | P0 |
| **预估工时** | 16h |
| **关键路径** | 是（下游依赖：W2-02-BE, W3-04-BE） |
| **依赖** | W1-01-BE (接口定义), W1-03-BE (CLAUDE.md 注入) |

**描述**:
从 Multica 移植 Claude Backend 实现。核心逻辑：`exec.Command` 启动 `claude` 子进程，stdin 写入 stream-json 格式 prompt，stdout 逐行解析 stream-json JSONL 输出。

**不做什么**: 先做同步模式（Complete），流式改进放在 W3-04-BE。其他 Backend (Codex/Kimi/OpenAI) 延迟到 v1.2。

**验收标准**:
- `pkg/agent/claude.go` 实现 `Backend` 接口：
  - `buildClaudeArgs()` — 构造 CLI 参数 (参照 multica claude.go)
  - `execute()` — exec.CommandContext 启动，stdin 写入 JSON
  - `parseOutput()` — stdout 逐行解析 stream-json
- CLI 参数正确包含：
  - `-p`, `--output-format stream-json`, `--input-format stream-json`
  - `--append-system-prompt` (来自 system_prompt 配置)
  - `--permission-mode bypassPermissions`
  - `--model` （可选）, `--max-turns`（可选）
- `filterCustomArgs()` — 过滤掉用户设置中与协议冲突的参数
- `Session` 返回包含 `<-chan OutputChunk` 的流式通道
- 替换旧的 `pkg/llm/local.go`：Daemon 使用新 Backend 而非 LocalProvider
- `go build ./...` 编译通过
- 单元测试：参数构造验证（mock exec 调用）、stream-json 解析

**参考**: architecture-review-v2.md §9.1 "Solo local.go vs multica claude.go", §7.1.4 "Claude Backend 实现关键点", multica `server/pkg/agent/claude.go`

---

#### SOLO-v11-W1-05-BE: 实现 Backend 工厂

| 字段 | 值 |
|------|-----|
| **标题** | 实现 Backend 工厂 (agent.New) |
| **角色** | rd2 |
| **优先级** | P0 |
| **预估工时** | 4h |
| **关键路径** | 否 |
| **依赖** | W1-01-BE, W1-04-BE |

**描述**:
在 `pkg/agent/factory.go` 中实现 Backend 工厂函数，根据 provider 类型选择正确的 Backend 实现。v1.1 只支持 `claude` 和 `openai`（HTTP API fallback）。

**验收标准**:
- `NewBackend(providerType, apiKey) (Backend, error)` — 根据类型返回实现
  - `"claude"` → `ClaudeBackend`
  - `"openai"` → `OpenAIBackend` (复用已有 pkg/llm/openai.go 封装)
  - `"anthropic"` → `AnthropicBackend` (复用已有 pkg/llm/anthropic.go 封装)
- 未知类型返回 error
- 单元测试：所有类型正确映射

---

#### SOLO-v11-W1-06-FE: 搭建 Neubrutalism 主题系统

| 字段 | 值 |
|------|-----|
| **标题** | 搭建 Neubrutalism 主题系统 |
| **角色** | fe1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W1-07-FE, W2-05-FE, W3-06-FE） |
| **依赖** | 无 |

**描述**:
根据 frontend-redesign-v2.md v3.0 的设计，在 `app/globals.css` 中创建 Neubrutalism 主题系统。替换现有的 shadcn/ui slate HSL 变量为硬编码品牌色。

**不做什么**: 不修改组件文件（W1-07-FE 做）。不修改字体加载 `layout.tsx`（文件内添加 Inter 字体导入）。

**验收标准**:
- `app/globals.css` 替换为 v3.0 `@theme inline` 块 (参照 frontend-redesign-v2.md §4.1)
- 包含完整品牌色板：
  - `brutal-bg: #fffaef`（奶油白）
  - `brutal-fg: #000000`（纯黑）
  - `brutal-pink: #fe7da8`
  - `brutal-yellow: #ffd440`
  - `brutal-lavender: #bbafe6`
  - `brutal-cyan: #27ccf3`
  - `brutal-lime: #a9d877`
  - `brutal-orange: #f8a16f`
  - `brutal-red: #f97264`
  - `brutal-stone: #c0b9b1`
- 功能性 CSS 变量映射正确 (`--color-background: #fffaef`, `--color-foreground: #000000` 等)
- `--radius: 0px` 全局零圆角
- 包含 brutaliest 工具类：
  - `shadow-brutal(-sm/-lg/-xl)` — 硬偏移阴影
  - `border-brutal(-thick)` — 黑色边框
  - `btn-brutal` — 按钮基类
  - `card-brutal` — 卡片基类
  - `input-brutal` — 输入框基类
  - `badge-brutal` — 徽标基类
- `prefers-reduced-motion` 媒体查询
- `*:focus-visible` 全局焦点样式：3px solid #74b9FF
- 应用编译通过：`npm run build`
- `app/layout.tsx` 更新字体导入，包含 Inter + Space Grotesk + Space Mono + Syne
- 删除旧的 HSL CSS 变量 (slate 主题)

**参考**: frontend-redesign-v2.md §4.1 "Complete @theme Block", §4.2 "Brutalist Utility Classes", §12.3 "Reduced Motion"

---

#### SOLO-v11-W1-07-FE: 改造基础 UI 组件

| 字段 | 值 |
|------|-----|
| **标题** | 改造基础 UI 组件 (Button/Input/Card/Dialog) |
| **角色** | fe1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W2-05-FE, W2-07-FE） |
| **依赖** | W1-06-FE |

**描述**:
将 shadcn/ui 基础组件 (Button, Input, Card, Dialog, Textarea, Label, Skeleton) 改造为 Neubrutalism 风格：零圆角、黑色边框、硬阴影、品牌色。

**不做什么**: badge/toggle/checkbox 等新组件延迟到 W2-08-FE。

**验收标准**:
- **Button** (`components/ui/button.tsx`): variant 映射为 brutalist 样式
  - `default` → 粉色背景 + 黑色边框 + 5px 阴影
  - `destructive` → 红色背景
  - `outline` → 白色背景 + 黑色边框
  - `ghost` → 无边框 (btn-flat)
  - hover: translate(-1px,-1px) + 7px 阴影
  - active: translate(3px,3px) + none 阴影
- **Input** (`components/ui/input.tsx`): 2px 黑框 + 3px 阴影 + 蓝色焦点轮廓
- **Card** (`components/ui/card.tsx`): 2px 黑框 + 5px 阴影, border-radius 0
- **Dialog** (`components/ui/dialog.tsx`): 3px 厚框 + 10px 阴影 (shadow-xl)
- 所有过渡动画 0.1-0.15s ease
- TypeScript 编译通过：`npx tsc --noEmit`

**参考**: frontend-redesign-v2.md §10.1 "Button", §10.2 "Card", §10.3 "Input", §10.6 "Modal"

---

#### SOLO-v11-W1-08-FE: 改造登录/注册页

| 字段 | 值 |
|------|-----|
| **标题** | 改造登录/注册页为 Neubrutalism 风格 |
| **角色** | fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-06-FE, W1-07-FE |

**描述**:
将登录和注册页面改造为 Neubrutalism 风格。使用 cream 背景、card-brutal、input-brutal、btn-brutal-pink 等组件。

**验收标准**:
- 登录页 (`app/auth/login/page.tsx`):
  - 居中卡片（card-brutal）
  - 表单使用 input-brutal
  - 提交按钮为 btn-brutal (粉色)
  - 背景为 brutal-bg (奶油白)
- 注册页 (`app/auth/register/page.tsx`): 同上模式
- Auth layout (`app/auth/layout.tsx`): cream 背景, 无 gradient
- 表单验证状态样式正确（错误时红色边框）
- 响应式：移动端全宽卡片，桌面端居中

**参考**: frontend-redesign-v2.md §15 "Phase 3: Layout Update"

---

#### SOLO-v11-W1-09-FE: 改造 Dashboard 布局

| 字段 | 值 |
|------|-----|
| **标题** | 改造 Dashboard 布局 (侧栏 + 内容区) |
| **角色** | fe2 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W2-05-FE, W3-08-FE） |
| **依赖** | W1-06-FE, W1-07-FE |

**描述**:
将 Dashboard 主布局改造为 Neubrutalism 风格。侧栏使用 cream 背景、2px 黑分割线、硬边框。内容区干净奶油白。

**验收标准**:
- 侧栏 (Sidebar): cream bg + 黑色右边框 2px
- 侧栏频道列表项: 2px 黑底边 + bold Space Grotesk 字体
- 内容区: cream bg (与全局一致)
- 2px 黑色 divider 作为区域分割
- 响应式: `<640px` 侧栏为 overlay, `640-768px` compact, `>=768px` 固定宽度
- Bootstrap 一致的边框和阴影系统

**参考**: frontend-redesign-v2.md §10.9 "Divider Variants", §11 "Responsive Breakpoints"

---

### Week 2 (2026-05-19 ~ 2026-05-25) — Agent 能力 + 消息编辑

**Sprint Goal**: Agent 按场景构建 prompt、记忆系统工作、消息可编辑删除

---

#### SOLO-v11-W2-01-BE: 实现 PromptBuilder

| 字段 | 值 |
|------|-----|
| **标题** | 实现 PromptBuilder (chat/mention/dm/thread) |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 16h |
| **关键路径** | 是（下游依赖：W2-02-BE, W3-02-BE） |
| **依赖** | W1-01-BE (Backend 接口类型), W1-02-BE (Workspace 路径) |

**描述**:
实现 `pkg/agent/prompt.go`，根据触发类型（chat/mention/dm/thread）生成不同的 prompt。核心策略：prompt 最小化，Agent 配置通过 CLAUDE.md 注入（W1-03-BE）。

**验收标准**:
- `BuildChatPrompt(ctx ChatContext) string` — 频道对话 prompt
  - Agent persona 标识
  - 频道名 + 频道描述
  - 最近 N 条消息（支持配置）
  - 回复指令 "Respond naturally"
- `BuildMentionPrompt(ctx MentionContext) string` — @提及 prompt
  - "You were @mentioned" + 优先级回复要求
  - 同上上下文
- `BuildDMPrompt(ctx DMContext) string` — 私信 prompt
  - "You are in a private conversation"
  - 用户消息 + 上下文
- `BuildThreadPrompt(ctx ThreadContext) string` — 线程 prompt
  - 线程源消息上下文
  - 回复要求
- 所有 prompt 为纯文本 + Markdown，stdin 兼容 stream-json 格式
- 单元测试：每个场景至少 3 个 case（含不同消息数、含 memory/不含）

**参考**: architecture-review-v2.md §7.5 "Prompt 管理系统", v2-optimization-plan.md §3.2 "System Prompt 生成"

---

#### SOLO-v11-W2-02-BE: Daemon TaskRouter 集成

| 字段 | 值 |
|------|-----|
| **标题** | Daemon TaskRouter 串联 Backend + Workspace + PromptBuilder |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W3-02-BE） |
| **依赖** | W1-04-BE (Claude Backend), W2-01-BE (PromptBuilder) |

**描述**:
在 `cmd/daemon/` 中创建 `task_router.go`，重构现有的 `processTaskStreaming` 逻辑。任务流程图：
```
Server POST /run
  → TaskRouter 解析请求
  → PromptBuilder 构建 prompt
  → WorkspaceManager.Prepare() 准备 workspace
  → InjectRuntimeConfig 写入 CLAUDE.md
  → Backend.Execute() 执行
  → 流式输出（SSE 转发）
```

**不做什么**: MEMORY.md 读取延迟到 W3-02-BE。不修改 Server 通信协议。

**验收标准**:
- `cmd/daemon/task_router.go` 包含 `TaskRouter` 结构体
- Router 按 `runTaskRequest.TriggerType`（chat/mention/dm/thread）分派
- 调用链完整：Prepare → Inject → BuildPrompt → Execute → Stream
- 旧的 `processTaskStreaming` 被新的 TaskRouter 替换
- 所有事件（token/error/done）通过 SSE 正确推送到 Server
- 编译通过：`go build ./cmd/daemon/`
- 集成测试：mock Server 发送请求，验证执行流程被触发

---

#### SOLO-v11-W2-03-BE: 实现 MemoryManager

| 字段 | 值 |
|------|-----|
| **标题** | 实现 MemoryManager + MEMORY.md |
| **角色** | rd2 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W3-01-BE, W3-02-BE） |
| **依赖** | W1-02-BE (Workspace 路径结构) |

**描述**:
实现 `pkg/agent/memory.go` — Agent 长期记忆系统。MEMORY.md 存储在 `~/.solo/agents/<id>/MEMORY.md`，Agent 在每次对话后自动更新。这是 Slock 独有的设计，Agent 在对话后更新记忆。

**不做什么**: 记忆摘要生成（LLM 二次调用）延迟到 W3-01-BE。频道级 memory 延迟到 v1.2。

**验收标准**:
- `GetMemory(agentID) (string, error)` — 读取 MEMORY.md
- `AppendMemory(agentID, entry string) error` — 追加记忆条目（时间戳前缀）
- `ClearMemory(agentID) error` — 清理记忆
- `MemoryPath(agentID) string` — 返回路径
- MEMORY.md 格式：
  ```markdown
  # Agent Memory

  ## Past Decisions
  - [2026-05-11T10:00:00Z] 决策内容

  ## User Preferences
  - 偏好内容
  ```
- 文件不存在时优雅处理（返回空字符串而非 error）
- 单元测试：读写、追加、清空、并发写入安全

**参考**: v2-optimization-plan.md §3.3 "Agent Memory 架构", architecture-review-v2.md §7.3 "Agent Memory 架构"

---

#### SOLO-v11-W2-04-BE: 消息编辑/删除 API

| 字段 | 值 |
|------|-----|
| **标题** | 实现消息编辑/删除 REST API |
| **角色** | rd2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | 无（独立的新增功能） |

**描述**:
在 `internal/server/handler/message.go` 中新增 PATCH 和 DELETE 端点，支持消息编辑（更新 content）和删除（软删除或硬删除）。

**验收标准**:
- `PATCH /api/v1/channels/{channelID}/messages/{messageID}`
  - 验证当前用户为消息发送者
  - 更新 `content` + `updated_at`
  - 通过 WebSocket 广播消息更新事件
  - 返回更新后的消息对象
- `DELETE /api/v1/channels/{channelID}/messages/{messageID}`
  - 验证当前用户为消息发送者
  - 硬删除或标记为已删除
  - 通过 WebSocket 广播消息删除事件
  - 返回 204 No Content
- 权限验证：只能编辑/删除自己的消息，频道管理员可删除任何消息
- 不能编辑/删除 Agent 消息（v1.1 范围外）
- 历史消息编辑次数上限 5 次
- `go test ./internal/server/handler/` 通过
- WebSocket 事件类型：`message:updated`, `message:deleted`

---

#### SOLO-v11-W2-05-FE: 消息组件 Neubrutalism 改造

| 字段 | 值 |
|------|-----|
| **标题** | 消息组件改造 (消息项、输入框、Agent 消息区分) |
| **角色** | fe1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是（下游依赖：W3-06-FE） |
| **依赖** | W1-06-FE, W1-07-FE |

**描述**:
将消息列表组件和消息输入组件改造为 Neubrutalism 风格。消息项使用纯黑 text + Inter body 字体、2px 分割线。输入框使用 input-brutal。

**验收标准**:
- 消息列表: 纯黑 `#000` text, Inter 字体
- 消息项: sender name bold Space Grotesk, body Inter
- 消息输入框: input-brutal + 粉色发送按钮
- Agent 消息区分: 粉色边框左侧条 + lavender badge
- 流式消息: 光标闪烁动画（打字指示器）
- 时间戳: Space Mono 灰色字体
- 2px 黑色分割线分隔每条消息

**参考**: frontend-redesign-v2.md §6 "Typography", §5 "Color System"

---

#### SOLO-v11-W2-06-FE: 消息编辑/删除 UI

| 字段 | 值 |
|------|-----|
| **标题** | 消息编辑/删除前端 UI |
| **角色** | fe1 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W2-05-FE (消息组件), W2-04-BE (API 可用) |

**描述**:
实现消息编辑/删除的前端交互。用户悬停消息时显示编辑/删除操作按钮，编辑时切换到编辑模式（预填文本、保存/取消按钮）。

**验收标准**:
- 消息悬停菜单: 编辑(小铅笔图标) + 删除(垃圾桶图标) 按钮
- 编辑模式: 输入框预填原内容 + "保存" + "取消" 按钮
- 编辑历史标记: 已编辑消息在时间戳旁显示 "(edited)"
- 删除确认: 轻量确认弹窗
- 禁用对 Agent 消息的编辑/删除操作
- 编辑/删除后乐观更新 UI
- API 调用失败回退

---

#### SOLO-v11-W2-07-FE: Agent 管理页改造

| 字段 | 值 |
|------|-----|
| **标题** | Agent 管理页 Neubrutalism 改造 |
| **角色** | fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-06-FE, W1-07-FE |

**描述**:
将 Agent 管理页面（列表 + 创建/编辑表单）改造为 Neubrutalism 风格。

**验收标准**:
- Agent 列表页: card-brutal 卡片网格 / 列表
- 每个 Agent 卡片: 名称(bold)、system prompt 预览、在线状态 badge-brutal(lime)
- 创建 Agent 表单: input-brutal + 粉色提交按钮
- 编辑 Agent 表单: 同上 + 删除按钮(红色)
- 空状态: 居中图标 + 描述 + "创建 Agent" CTA
- 加载状态: skeleton 占位

---

#### SOLO-v11-W2-08-FE: 新增 UI 组件

| 字段 | 值 |
|------|-----|
| **标题** | 新增 Neubrutalism UI 组件 (badge/toggle/checkbox/codeblock) |
| **角色** | fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-06-FE |

**描述**:
根据 frontend-redesign-v2.md 的设计创建新的 UI 组件文件。

**验收标准**:
- `components/ui/badge.tsx` — badge-brutal 带颜色变体 (pink/yellow/lavender/lime/cyan/red/black)
- `components/ui/toggle.tsx` — nb-toggle 样式（黑色边框 + lime 选中状态）
- `components/ui/checkbox.tsx` — nb-checkbox（24x24 方形 + yellow 选中 + 勾号）
- `components/ui/code-block.tsx` — 深色背景代码块（#1a1a2e bg + 语法颜色）
- 所有新组件通过 TypeScript 编译
- 与 theme token 系统一致

**参考**: frontend-redesign-v2.md §10.4 "Badge", §10.5 "Form Elements", §10.8 "Code Block"

---

### Week 3 (2026-05-26 ~ 2026-06-01) — 记忆 + 流式 + 打磨

**Sprint Goal**: 记忆系统可工作、流式输出真正流式、全覆盖测试

---

#### SOLO-v11-W3-01-BE: Memory 摘要生成与自动更新

| 字段 | 值 |
|------|-----|
| **标题** | Memory 摘要生成与自动更新机制 |
| **角色** | rd1 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W2-03-BE (MemoryManager), W2-01-BE (PromptBuilder) |

**描述**:
实现自动记忆更新机制。Agent 每次响应后，Daemon 根据对话内容生成记忆摘要，追加到 MEMORY.md。摘要生成可以是 LLM 二次调用（昂贵但精确）或基于规则的提取（轻量）。

**验收标准**:
- 记忆更新点：Agent 完成响应后
- 记忆摘要生成：
  - 基于规则提取：提取对话中的"用户偏好"、"决策"、"事实"类内容
  - 可选 LLM 二次调用（通过 flag 控制）
- 记忆大小控制：最大 50 条，超出时淘汰最早记录
- `MemoryManager.UpdateMemory(agentID, conversation)` 方法
- 单元测试：记忆摘要生成、大小控制、淘汰策略

---

#### SOLO-v11-W3-02-BE: PromptBuilder + MemoryManager 集成

| 字段 | 值 |
|------|-----|
| **标题** | PromptBuilder 与 MemoryManager 集成 |
| **角色** | rd1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是 |
| **依赖** | W2-01-BE, W2-03-BE, W3-01-BE |

**描述**:
将 MemoryManager 集成到 TaskRouter 和 PromptBuilder 中。Agent 执行时，先读取 MEMORY.md，将其内容注入到 prompt 的 "Your Memory" 部分。

**验收标准**:
- TaskRouter 调用 MemoryManager.GetMemory() 获取记忆
- PromptBuilder 组装 prompt 时注入记忆内容
- 记忆格式化为符合 prompt 上下文的 Markdown
- 空记忆（新 Agent）不注入
- 集成测试：带记忆 vs 不带记忆的 prompt 输出

---

#### SOLO-v11-W3-03-BE: Machine Lock 实现

| 字段 | 值 |
|------|-----|
| **标题** | Machine Lock — 防止 Daemon 重复运行 |
| **角色** | rd2 |
| **优先级** | P2 |
| **预估工时** | 4h |
| **关键路径** | 否 |
| **依赖** | 无 |

**描述**:
在 Daemon 启动时检查 `~/.solo/daemon/lock.json`。如果文件存在且 PID 存活，则退出（防止重复运行）。

**验收标准**:
- Daemon 启动时写入 `~/.solo/daemon/lock.json`
- lock.json 包含：`daemon_id`, `pid`, `started_at`, `host`
- 启动时检查 lock 文件：PID 存活则退出
- Daemon 停止时清理 lock 文件
- 强制启动选项 `--force`

**参考**: v2-optimization-plan.md §3.4 "Machine Lock"

---

#### SOLO-v11-W3-04-BE: Claude Backend 流式改进

| 字段 | 值 |
|------|-----|
| **标题** | Claude Backend 真正流式输出 (StdoutPipe) |
| **角色** | rd2 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是 |
| **依赖** | W1-04-BE (Claude Backend 基础实现) |

**描述**:
将 Claude Backend 从同步执行改为真正流式。使用 `exec.Command` 的 `StdoutPipe()` 逐行读取输出，每读取到内容就推送一个 OutputChunk。

**验收标准**:
- 使用 `cmd.StdoutPipe()` 而非 `cmd.Stdout = &bytes.Buffer`
- `bufio.Scanner` 逐行扫描 stdout
- 每行 JSON 解析为 stream-json 事件
- 事件类型正确映射：`text` → `OutputChunk{Type: text, Content: ...}`
- 流式通道非阻塞写入（缓冲 64）
- Agent 停止（context cancel）时立即中断子进程
- 测试：mock CLI 验证逐行输出被流式传输

**参考**: architecture-review-v2.md §9.1 "Solo local.go vs multica claude.go"

---

#### SOLO-v11-W3-05-BE: 后端集成测试

| 字段 | 值 |
|------|-----|
| **标题** | 后端集成测试 (Backend + Workspace + Daemon) |
| **角色** | rd2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-01-BE ~ W2-04-BE (除 08,09,10 外所有后端任务) |

**描述**:
编写后端集成测试，覆盖 Backend 接口、WorkspaceManager、MemoryManager、PromptBuilder 的核心流程。

**验收标准**:
- WorkspaceManager: 创建/清理/幂等性
- PromptBuilder: 4 种场景 prompt 生成
- MemoryManager: 读写/追加/清理
- Backend 工厂: 类型映射
- `go test ./pkg/agent/...` 覆盖率 > 70%
- `go test ./cmd/daemon/...` 通过

---

#### SOLO-v11-W3-06-FE: Agent 消息 + 流式渲染

| 字段 | 值 |
|------|-----|
| **标题** | Agent 消息 + 流式渲染组件改造 |
| **角色** | fe1 |
| **优先级** | P0 |
| **预估工时** | 8h |
| **关键路径** | 是 |
| **依赖** | W2-05-FE (消息组件改造) |

**描述**:
改造 Agent 消息显示和流式输出体验。Agent 消息以独特样式区分（粉色边框/lavender badge），流式输出显示打字动画。

**验收标准**:
- Agent 信息：lavender badge + bold Space Grotesk 名称
- Agent 消息：左侧粉色 3px 边框 + white bg
- 流式输出：文本逐字出现
- "Thinking..." 状态：橙色脉冲指示器
- 错误状态：红色边框 + 错误消息
- Agent 头像：圆形 + 首字母

**参考**: frontend-redesign-v2.md §10.4 "Badge"

---

#### SOLO-v11-W3-07-FE: 响应式布局 + 动画/微交互

| 字段 | 值 |
|------|-----|
| **标题** | 响应式布局适配 + 微交互动画 |
| **角色** | fe1 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-09-FE |

**描述**:
完善响应式布局和交互动画，确保移动端可用性。

**验收标准**:
- 移动端 (`<640px`): sidebar 覆盖层 + 底部导航
- 平板 (`640-768px`): compact sidebar
- 桌面 (`>768px`): 三栏布局
- 所有按钮/卡片/组件的 hover/active 动画
- 面板打开/关闭的滑入/滑出动画
- 过度动画统一 0.1-0.15s ease
- `prefers-reduced-motion` 支持

**参考**: frontend-redesign-v2.md §11 "Responsive Breakpoints", §12 "Animation & Transition Rules"

---

#### SOLO-v11-W3-08-FE: 侧栏频道列表 + DM 改造

| 字段 | 值 |
|------|-----|
| **标题** | 侧栏频道列表 + DM 列表 Neubrutalism 改造 |
| **角色** | fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W1-09-FE |

**描述**:
将侧栏的频道列表和 DM 列表改造为 Neubrutalism 风格。

**验收标准**:
- 频道项：bold Space Grotesk + # 前缀 + 未读计数(badge-brutal)
- DM 项：用户头像 + 名称 + 在线状态(lime 小圆点)
- 选中项：粉色左侧边框 + 粉色高亮背景
- 频道/DM 分组标题：uppercase Space Mono small + stone 色
- 创建按钮：btn-brutal-sm 粉色

---

#### SOLO-v11-W3-09-FE: 空/加载/错误状态处理

| 字段 | 值 |
|------|-----|
| **标题** | 空状态/加载中/错误状态 UI |
| **角色** | fe2 |
| **优先级** | P1 |
| **预估工时** | 4h |
| **关键路径** | 否 |
| **依赖** | W1-06-FE, W1-07-FE |

**描述**:
为所有主要页面实现规范化状态处理。

**验收标准**:
- 空状态：居中图标 + 描述文案 + 可选 CTA
- 加载状态：card-brutal skeleton + pulse 动画
- 错误状态：红色 accent 边框 + 错误文本 + 重试按钮

**参考**: frontend-redesign-v2.md §14 "Edge Cases & States"

---

### Week 4 (2026-06-02 ~ 2026-06-08) — 打磨 + E2E

**Sprint Goal**: 全功能可用、全链路集成测试通过、UI 风格统一

---

#### SOLO-v11-W4-01-QA: Daemon 端全链路测试

| 字段 | 值 |
|------|-----|
| **标题** | Daemon 端全链路集成测试 |
| **角色** | qa1 (协作: rd1) |
| **优先级** | P0 |
| **预估工时** | 16h |
| **关键路径** | 是 |
| **依赖** | W3-02-BE, W3-04-BE, W3-05-BE |

**描述**:
编写 daemon 端全链路集成测试，覆盖 TaskRouter → WorkspaceManager → PromptBuilder → Backend 的完整流程。

**验收标准**:
- TaskRouter 端到端测试：mock Server 请求，验证完整执行链
- WorkspaceManager 集成测试：目录创建/清理/内容正确性
- 模拟 CLI 执行（mock claude 二进制），验证参数传递正确
- 并发任务隔离测试（2 个 Agent 同时运行互不干扰）
- 错误场景：无效 Agent ID、超时、子进程崩溃
- 测试覆盖率 > 80%

---

#### SOLO-v11-W4-02-QA: 消息编辑/删除集成测试

| 字段 | 值 |
|------|-----|
| **标题** | 消息编辑/删除 API + WebSocket 集成测试 |
| **角色** | qa1 (协作: rd2) |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W2-04-BE |

**描述**:
对消息编辑/删除 API 进行集成测试，覆盖 REST 端点和 WebSocket 广播。

**验收标准**:
- PATCH /messages/{id}: 200、401、403、404 用例
- DELETE /messages/{id}: 204、401、403、404 用例
- WebSocket 事件 `message:updated` 和 `message:deleted` 正确广播
- 并发编辑测试
- `go test ./internal/server/handler/... -v -count=1` 全部通过

---

#### SOLO-v11-W4-03-QA: Backend + Workspace + Memory E2E

| 字段 | 值 |
|------|-----|
| **标题** | Backend + Workspace + Memory 端到端测试 |
| **角色** | qa1 (协作: rd1+rd2) |
| **优先级** | P0 |
| **预估工时** | 16h |
| **关键路径** | 是 |
| **依赖** | W3-02-BE, W3-04-BE, W3-05-BE |

**描述**:
端到端测试覆盖：创建 Agent → 发送消息触发 → TaskRouter 调度 → Workspace 准备 → MEMORY.md 读取 → PromptBuilder 构建 → Backend 执行 → 流式输出 → 记忆更新。

**验收标准**:
- E2E 流程通过（可脚本化运行）
- 多轮对话记忆持续
- Agent 工作目录隔离（Agent A 的文件 Agent B 不可见）
- Mock CLI 环境：模拟 "claude" 命令，返回预设输出
- 测试清理：测试完成后删除 ~/.solo/agents/ 测试目录
- 记录测试日志供调试

---

#### SOLO-v11-W4-04-FE: 主题打磨

| 字段 | 值 |
|------|-----|
| **标题** | Neubrutalism 主题统一打磨 |
| **角色** | fe1 + fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | 所有 FE 任务完成 |

**描述**:
全站主题统一审查和打磨。检查所有页面、组件、状态的颜色/阴影/字体一致性。

**验收标准**:
- 无硬编码颜色（全部使用 CSS 变量）
- 所有阴影符合 shadow-brutal scale (sm/md/lg/xl)
- 所有边框 2px #000
- 所有圆角为 0
- 字体使用正确（Inter body / Space Grotesk UI / Space Mono code）
- 对比度检查通过 (black on cream = 18.5:1)
- 清理未使用的 CSS 和冗余 class

---

#### SOLO-v11-W4-05-FE: 可访问性

| 字段 | 值 |
|------|-----|
| **标题** | 可访问性改造 (a11y) |
| **角色** | fe1 + fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | W4-04-FE |

**描述**:
根据 frontend-redesign-v2.md §13 和 §12.3 的要求，完成可访问性改造。

**验收标准**:
- Skip link 添加到根 layout
- `prefers-reduced-motion` 禁用所有动画
- 所有焦点可见指示器使用 3px #74b9FF 蓝色
- ARIA 标签: 图标按钮、导航、错误消息（role="alert"）
- `aria-expanded`, `aria-controls` 在可展开元素上
- 键盘导航完整（Tab 顺序合理）
- 触摸目标 >= 44x44px

**参考**: frontend-redesign-v2.md §13 "Accessibility", §12.3 "Reduced Motion"

---

#### SOLO-v11-W4-06-FE: 全量页面适配

| 字段 | 值 |
|------|-----|
| **标题** | 全量页面 Neubrutalism 适配 |
| **角色** | fe1 + fe2 |
| **优先级** | P1 |
| **预估工时** | 8h |
| **关键路径** | 否 |
| **依赖** | 所有 FE 任务完成 |

**描述**:
审查并适配所有剩余页面（Settings 页、not-found 页、频道详情页等）为 Neubrutalism 风格。

**验收标准**:
- Settings 页面: card-brutal 表单 + input-brutal + 粉色保存按钮
- 404 页面: 大号粗野主义标题 + 返回按钮
- 频道详情页: card-brutal 成员列表
- 所有路由页面风格一致

---

## 5. 关键路径 (Critical Path)

关键路径上的任务延迟会直接影响交付日期。v1.1 关键路径：

```
W1-01-BE → W1-04-BE → W2-02-BE → W3-02-BE → W4-03-QA
  ↓                      ↓           ↓
W1-02-BE ⟷ W1-03-BE    W2-01-BE   W3-04-BE
  ↓
W1-06-FE → W1-07-FE → W2-05-FE → W3-06-FE → W4-04-FE → W4-05-FE
  ↓
W1-09-FE
```

**关键路径总工时**: 119h (rd1: 40h + rd2: 24h + fe1: 24h + fe2: 8h + qa1: 16h + arc: 7h)

**缓冲**: 关键路径预留 20% = ~24h 缓冲。

---

## 6. 多提拉复用表 (v1.1 范围)

| Multica 代码 | Solo 路径 | 复用方式 |
|-------------|-----------|---------|
| `server/pkg/agent/backend.go` 接口定义 | `pkg/agent/backend.go` (新) | 参考设计 + 适配 Solo 的 RunRequest |
| `server/pkg/agent/claude.go` 参数构造 + stream-json 解析 | `pkg/agent/claude.go` (新) | 直接移植协议级代码 |
| `server/pkg/agent/factory.go` | `pkg/agent/factory.go` (新) | 参考设计 |
| `server/pkg/agent/models.go` | 暂不移植 | v1.2 |
| `server/internal/daemon/prompt.go` BuildPrompt | `pkg/agent/prompt.go` (新) | 参考设计模式 + Solo 场景定制 |
| `server/internal/daemon/` daemon 架构 | `cmd/daemon/task_router.go` (新) | 参考流程设计, 适配 Solo 的 HTTP/SSE 通信 |

---

## 7. 文件改动清单

### 新增文件

| 文件路径 | 负责人 | Week |
|---------|--------|------|
| `pkg/agent/backend.go` | rd1 | W1 |
| `pkg/agent/workspace.go` | rd1 | W1 |
| `pkg/agent/claude.go` | rd2 | W1 |
| `pkg/agent/factory.go` | rd2 | W1 |
| `cmd/daemon/task_router.go` | rd1 | W2 |
| `pkg/agent/prompt.go` | rd1 | W2 |
| `pkg/agent/memory.go` | rd2 | W2 |
| `components/ui/badge.tsx` | fe2 | W2 |
| `components/ui/toggle.tsx` | fe2 | W2 |
| `components/ui/checkbox.tsx` | fe2 | W2 |
| `components/ui/code-block.tsx` | fe2 | W2 |

### 修改文件

| 文件路径 | 修改内容 | 负责人 | Week |
|---------|---------|--------|------|
| `app/globals.css` | 替换为 Neubrutalism 主题 | fe1 | W1 |
| `app/layout.tsx` | 添加 Inter 字体导入 | fe1 | W1 |
| `components/ui/button.tsx` | 替换为 btn-brutal variant | fe1 | W1 |
| `components/ui/input.tsx` | 替换为 input-brutal | fe1 | W1 |
| `components/ui/card.tsx` | 替换为 card-brutal | fe1 | W1 |
| `components/ui/dialog.tsx` | 3px thick border + shadow-xl | fe1 | W1 |
| `app/auth/login/page.tsx` | Neubrutalism | fe2 | W1 |
| `app/auth/register/page.tsx` | Neubrutalism | fe2 | W1 |
| `app/auth/layout.tsx` | cream bg | fe2 | W1 |
| `app/dashboard/page.tsx` | Neubrutalism 布局 | fe2 | W1 |
| `cmd/daemon/handler.go` | 添加 Backend + Workspace 引用 | rd1 | W2 |
| `cmd/daemon/main.go` | 初始化 Backend 工厂 + WorkspaceManager | rd1 | W2 |
| `internal/server/handler/message.go` | 新增 PATCH/DELETE | rd2 | W2 |
| `pkg/agent/agent.go` | 保留旧接口（兼容） | rd1 | W1 |

---

## 8. 风险登记册

| # | 风险描述 | 概率 | 影响 | 触发条件 | 缓解措施 | 应急预案 | 负责人 |
|---|---------|------|------|---------|---------|---------|--------|
| R1 | **Multica 代码版本不兼容** — Multica 的 Backend 代码使用 Go 1.23+ 而 Solo 使用 1.22 | 中 | 高 | 编译错误 | 移植前确认 Go 版本差异, 语法兼容性审查 | 手动改写不兼容部分 | rd1 |
| R2 | **Claude Code CLI 版本行为差异** — Multica 使用的 claude 版本与用户本地版本不同 | 中 | 高 | stream-json 格式不匹配 | 使用 `claude --version` 检测, 参数适配层 | fallback 到旧的 `--print` 模式 | rd2 |
| R3 | **Neubrutalism 主题影响现有功能** — 全局样式覆盖导致某些页面布局错乱 | 中 | 中 | QA 发现布局断裂 | 渐进式改造 (先 theme → 再组件 → 再页面) | 回滚 globals.css, 分模块验证 | fe1 |
| R4 | **Daemon 重构阻塞 Agent 响应** — TaskRouter 替换过程中 Agent 响应中断 | 高 | 高 | Server 调用 Daemon 失败 | 新旧代码并行运行, 通过 feature flag 切换 | 回退到旧 handler 路径 | rd1 |
| R5 | **消息编辑冲突** — 编辑已删除消息或并发编辑 | 低 | 中 | 404 / 数据竞争 | 加乐观锁 (updated_at 检查) | 返回 409 Conflict | rd2 |
| R6 | **前端容量不足** — fe1/fe2 4 周 160h 无法完成全部 FE 任务 | 高 | 中 | Sprint 中 task 未完成 | W1/W2 优先 P0 任务, P1 可延至 W4 | W4 P1 任务移至 v1.2 | tpm |

---

## 9. 里程碑

| 里程碑 | 日期 | 验收标准 | 负责人 |
|--------|------|---------|--------|
| M1: Backend 接口 + Workspace | 2026-05-16 | Backend 接口编译通过, Workspace 目录创建测试通过 | rd1 |
| M2: Claude Backend 可用 | 2026-05-18 | Claude CLI 可调用并返回结果 | rd2 |
| M3: 主题系统上线 | 2026-05-14 | globals.css + 基础组件改造完成, Dashboard 可预览 | fe1 |
| M4: Agent 按场景对话 | 2026-05-23 | chat/mention/dm/thread 四类 prompt 正确生成 | rd1 |
| M5: 记忆系统可用 | 2026-05-23 | MEMORY.md 读写 + 注入 prompt | rd2 |
| M6: 消息编辑/删除 | 2026-05-25 | API 和前端 UI 可用 | rd2+fe1 |
| M7: 真正流式输出 | 2026-05-30 | Claude Backend 逐行流式输出 | rd2 |
| M8: 记忆注入完成 | 2026-06-01 | PromptBuilder + MemoryManager 集成测试通过 | rd1 |
| M9: 全链路测试通过 | 2026-06-06 | E2E 测试脚本通过 | qa1+rd1+rd2 |
| M10: v1.1 Release | 2026-06-08 | 所有 P0 任务完成, 回归测试通过 | tpm |

---

## 10. 沟通计划

| 频率 | 事项 | 参与者 | 形式 |
|------|------|--------|------|
| 每日 | 站会 | 全员 | 异步 (Slack): 昨日完成 / 今日计划 / 阻塞项 |
| 周二/周四 | 进度检查 | tpm + rd1/fe1 | 同步 15min: 关键路径状态 |
| 周五 | 周报 | 全员 | 文档: 本周完成 vs 计划, 燃尽图, 风险更新 |
| 按需 | 架构问题升级 | tpm + arc | 随时: rd1/fe1 提出, arc 评审 |

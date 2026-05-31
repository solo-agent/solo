# Solo v1.2 — 任务分解与迭代规划 (WBS)

> 版本: 1.0
> 创建日期: 2026-05-12
> 负责人: tpm agent
> 对齐文档: docs/v2-optimization-plan.md, docs/product-roadmap-v2.md, TASKS.md, ARCHITECTURE.md

---

## 目录

1. [项目概览](#1-项目概览)
2. [v1.2 范围与优先级](#2-v12-范围与优先级)
3. [团队角色与分工](#3-团队角色与分工)
4. [multica 迁移映射表](#4-multica-迁移映射表)
5. [Sprint 1 (Week 1-2): Backend 迁移 Wave 1 + Agent 详情面板](#5-sprint-1-week-1-2-backend-迁移-wave-1--agent-详情面板)
6. [Sprint 2 (Week 3-4): Backend 迁移 Wave 2 + MEMORY + Tools + 工具配置 UI](#6-sprint-2-week-3-4-backend-迁移-wave-2--memory--tools--工具配置-ui)
7. [Sprint 3 (Week 5-6): 任务系统 + 流式 + 交互模式 + 前端冲刺](#7-sprint-3-week-5-6-任务系统--流式--交互模式--前端冲刺)
8. [跨 Sprint 依赖关系](#8-跨-sprint-依赖关系)
9. [风险登记册](#9-风险登记册)
10. [附录 A: 任务统计汇总](#10-附录-a-任务统计汇总)
11. [附录 B: 团队容量检查](#11-附录-b-团队容量检查)
12. [附录 C: 各 Backend 迁移工作量评估](#12-附录-c-各-backend-迁移工作量评估)

---

## 1. 项目概览

**项目名称**: Solo v1.2 — Agent 系统深度增强

**核心目标**: 从 Multica 迁移 10 个 Backend 实现，构建 Agent 记忆和工具系统，交付 Agent 详情/工具配置/任务系统的前端 UI。

**时间线**: 6 周 (2026-05-12 ~ 2026-06-22)

**前提条件**: v1.1 已交付，包含 Backend 接口定义 (`pkg/agent/backend.go`)、ClaudeBackend (`pkg/agent/claude.go`)、WorkspaceManager、MemoryManager、PromptBuilder、MachineLock，以及所有 v0.1-v0.4 + RC 的核心 MVP 功能 (83 测试通过)。

**团队规模**: 3 人 (rd1 + fe1 + qa1 每 2 周介入)

---

## 2. v1.2 范围与优先级

### P0 — 必须交付 (阻塞后续版本)

| 编号 | 功能 | 涉及模块 | 预估工时 | 负责 |
|------|------|---------|---------|------|
| D-01 | 10 个 Backend 迁移 (Codex/Cursor/Gemini/Kimi/Kiro/Copilot/OpenCode/OpenClaw/Hermes/Pi) | `pkg/agent/*.go` | 72h | rd1 |
| D-02 | MEMORY.md 自动摘要 — LLM 对话后自动生成记忆摘要写入 MEMORY.md | `pkg/agent/memory.go` | 16h | rd1 |
| D-03 | Agent 工具系统 — read_file/write_file/list_files/search_files 内置工具 | `pkg/agent/tools/*.go` | 20h | rd1 |
| D-04 | Agent 详情面板 — runtime 标签、状态、头像、执行历史 | `frontend/app/agents/[id]/` | 24h | fe1 |
| D-05 | Agent 工具配置 UI — 为 Agent 选择/配置工具 | `frontend/app/agents/[id]/tools` | 16h | fe1 |
| D-06 | 任务系统 UI — `/tasks` 创建、列表、指派 | `frontend/app/tasks/` | 20h | fe1 |

### P1 — 目标交付 (有 1 周缓冲)

| 编号 | 功能 | 涉及模块 | 预估工时 | 负责 |
|------|------|---------|---------|------|
| D-07 | 真正流式输出 — stream-json 逐 token 推送 (替代 local.go 一次性返回) | `cmd/daemon/` + `pkg/agent/` | 16h | rd1 |
| D-08 | Skills 系统 — `.claude/skills/` 按 provider 分离目录注入 | `pkg/agent/skills.go` | 12h | rd1 |
| D-09 | 任务系统后端 MVP — `/api/v1/tasks` API + 数据模型 | `internal/server/handler/task.go` + 迁移 | 20h | rd1 |
| D-10 | Agent 交互模式 UI — "免打搅" vs "主动参与" 切换 | `frontend/app/agents/[id]/` | 8h | fe1 |

### 超出范围

- 文件上传后端/前端 (v1.3)
- 消息全文搜索 (v1.3)
- 用量统计 (v1.3)
- Agent-to-Agent 协作协议 (v1.3)
- Agent 市场/模板 (v2.0)

---

## 3. 团队角色与分工

| 角色 | 代号 | 技能栈 | 投入 | v1.2 职责 |
|------|------|--------|------|----------|
| 后端开发 | rd1 | Go + Chi + pgx + Multica 源码熟悉 | 1 人全职 | 全部后端工作: Backend 迁移、MEMORY、Tools、Skills、任务系统、流式 |
| 前端开发 | fe1 | Next.js 16 + Tailwind + shadcn/ui + SWR | 1 人全职 | 全部前端工作: Agent 详情面板、工具配置 UI、任务系统 UI、交互模式 |
| 测试 | qa1 | API 测试 + E2E + 验收测试 | 0.3 人 | 每 Sprint 末 2 天集成验证 |

> 注意: v1.2 团队收缩为 3 人 (rd1 + fe1 + qa1 50%)。rd2 和 fe2 暂时调离。rd1 需独立承担全部后端工作，fe1 独立承担全部前端工作。

---

## 4. Multica 迁移映射表

### 4.1 Backend 迁移 (10 个)

每个 Backend 从 Multica 的 `pkg/agent/` 搬到 Solo 的 `pkg/agent/`，适配 Solo 的 `Backend` 接口 (`Execute(ctx, *ExecuteRequest, *ExecuteOptions) (*Session, error)`)。

| Multica 文件 | Solo 目标文件 | 行数 | 适配要点 | 预估(h) |
|-------------|-------------|------|---------|--------|
| `codex.go` | `pkg/agent/codex.go` | 1179 | stdin → JSON-RPC 2.0 请求; stdout ← JSON-RPC 2.0 响应解析; `buildCodexArgs()` 适配 | 8 |
| `cursor.go` | `pkg/agent/cursor.go` | 422 | stdin → 自定义 JSON; stdout ← 自定义 JSON; `buildCursorArgs()` 适配 | 5 |
| `cursor_invocation.go` | `pkg/agent/cursor_invocation.go` | (合并) | Cursor 进程映射到 Solo 的 invocation schema | 3 |
| `gemini.go` | `pkg/agent/gemini.go` | 267 | stdin → Gemini JSON; 小文件, 适配简单 | 3 |
| `kimi.go` | `pkg/agent/kimi.go` | 403 | stdin → ACP 格式; stdout ← ACP 格式 | 4 |
| `kiro.go` | `pkg/agent/kiro.go` | 387 | stdin → ACP 格式; 同 Kimi 协议族 | 3 |
| `copilot.go` | `pkg/agent/copilot.go` | 440 | stdin → 自定义 JSON; stdout ← 自定义 JSON; 适配 `buildCopilotArgs()` | 4 |
| `opencode.go` | `pkg/agent/opencode.go` | 431 | stdin → 自定义 JSON; stdout ← JSONL; `buildOpenCodeArgs()` | 5 |
| `openclaw.go` | `pkg/agent/openclaw.go` | 629 | stdin → 自定义 JSON; stdout ← JSONL; 自定义 session 管理 | 6 |
| `hermes.go` | `pkg/agent/hermes.go` | 1422 | 最大文件; stdin → ACP; stdout ← ACP; 复杂会话管理; tools 注入 | 10 |
| `pi.go` | `pkg/agent/pi.go` | 399 | stdin → 自定义 JSON; stdout ← JSONL | 4 |

**总计**: ~5979 行 / 55h (不含测试)

### 4.2 不复用

| Multica 模块 | 原因 |
|-------------|------|
| `agent.go` (Multica 的 Backend 接口) | Solo 已有自己的 `Backend` 接口定义 (`backend.go`), 不直接复用 |
| `models.go` (Multica 的 ListModels) | Solo 用不同的 provider 注册机制, 各 backend 的 `ListModels()` 单独适配 |
| `version.go` | Multica 版本信息, 不适用 |
| `stderr_tail.go` | Solo 已有自己的实现 (在 `claude.go` 中) |
| `proc_*.go`, `proc_windows.go` | 平台相关代码, Solo ClaudeBackend 已有 `hideAgentWindow()` |
| `exec_fixture_*.go` | 测试 fixture, 不适用 |

### 4.3 直接引用 (不改代码, 仅测试)

| Multica 文件 | Solo 路径 | 适配说明 |
|-------------|-----------|----------|
| `testdata/` | `pkg/agent/testdata/` | 测试 fixture 数据, 直接复制 |

---

## 5. Sprint 1 (Week 1-2): Backend 迁移 Wave 1 + Agent 详情面板

**Sprint 目标**: 完成 5 个 Backend 迁移 (Codex/Cursor/Gemini/Kimi/Kiro)，Agent 详情面板可展示运行信息。

**可演示产出 (第 2 周末)**:
1. rd1: Codex/Cursor/Gemini/Kimi/Kiro 5 个 Backend 单元测试通过, `NewBackend("codex")` 返回正确实例
2. fe1: Agent 详情页显示 runtime 信息、状态指示器、基本执行历史列表

### 5.1 Week 1

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-100-B** | 迁移 Codex Backend | 从 Multica 复制 `pkg/agent/codex.go`, 适配 Solo 的 `Backend.Execute(ctx, *ExecuteRequest, *ExecuteOptions)` 接口; 实现 `buildCodexArgs()` 构造 JSON-RPC 2.0 CLI 参数; 实现 stdout JSON-RPC 2.0 响应解析; 实现 `Name() string` 返回 "codex" | rd1 | P0 | SOLO-01-B (Backend 接口已就绪) | 8 | `codexBackend.Execute()` 可正常调用; 单元测试覆盖正常/超时/取消路径; `Name()` 返回 "codex" |
| **SOLO-101-B** | 迁移 Cursor Backend | 从 Multica 复制 `pkg/agent/cursor.go` + `cursor_invocation.go`, 适配 Solo 接口; 实现 Cursor 进程发现和 invocation 逻辑 | rd1 | P0 | SOLO-100-B (参考架构) | 5 | `cursorBackend.Execute()` 可正常调用; 单元测试通过; `Name()` 返回 "cursor" |
| **SOLO-102-B** | 迁移 Gemini Backend | 从 Multica 复制 `pkg/agent/gemini.go`, 适配 Solo 接口; Gemini 是最小 backend (~267 行), 适配简单 | rd1 | P0 | SOLO-100-B | 3 | `geminiBackend.Execute()` 可正常调用; 单元测试通过; `Name()` 返回 "gemini" |
| **SOLO-103-B** | 迁移 Kimi Backend | 从 Multica 复制 `pkg/agent/kimi.go`, 适配 Solo 接口; 实现 ACP 协议 stdin/stdout 通信 | rd1 | P0 | SOLO-100-B | 4 | `kimiBackend.Execute()` 可正常调用; 单元测试通过 |
| **SOLO-104-B** | 迁移 Kiro Backend | 从 Multica 复制 `pkg/agent/kiro.go`, 适配 Solo 接口; Kimi/Kiro 同属 ACP 协议族, 可参考 SOLO-103-B | rd1 | P0 | SOLO-103-B | 3 | `kiroBackend.Execute()` 可正常调用; 单元测试通过; `Name()` 返回 "kiro" |
| **SOLO-105-B** | 更新 Factory 注册 + 测试 | 在 `factory.go` 中注册 codex/cursor/gemini/kimi/kiro 到 `NewBackend()`; 编写集成测试验证所有 backend 可通过 factory 创建 | rd1 | P0 | SOLO-100-B ~ SOLO-104-B | 4 | `NewBackend("codex")` 等全部返回正确实例; 无效 provider 返回错误; 集成测试覆盖 |
| **SOLO-106-F** | Agent 详情页路由 + 骨架 | 创建 `/agents/[id]` 页面路由和布局; 实现页面骨架: 左侧信息区 (头像+名称+状态) + 右侧 tab 区 (runtime/工具/历史); 加载态骨架屏 | fe1 | P0 | 无 (可独立开始, mock 数据) | 4 | 访问 `/agents/{id}` 显示骨架布局; 三个 tab 占位符显示; 加载态骨架屏正常 |
| **SOLO-107-F** | Agent 状态指示器组件 | 实现 Agent 状态指示器: 绿(在线)/黄(思考中)/蓝(输出中)/灰(离线); 显示在 Agent 详情页头部和列表卡片上; 支持 tooltip 提示状态含义 | fe1 | P0 | SOLO-106-F | 3 | 四种状态正确显示; 状态切换动画流畅; tooltip 显示状态说明 |
| **SOLO-108-F** | Agent Runtime Tab | 实现详情页 "运行时" tab: 显示 model provider、model name、temperature、max_tokens 配置; 显示当前 Backend 类型 (claude/codex/等); 可编辑保存 (PATCH /agents/{id}) | fe1 | P0 | SOLO-106-F | 5 | Runtime 信息正确展示; 编辑后保存成功; 表单校验正常 |
| **SOLO-109-F** | Agent 执行历史列表 | 实现详情页 "执行历史" tab: 列表展示最近执行记录 (时间、频道、触发类型、状态、耗时); 空态引导 "Agent 还没有执行过"; 加载态骨架屏; 点击记录可展开详情 | fe1 | P0 | SOLO-106-F, 后端 AgentRun API | 6 | 历史列表加载正常; 空态引导显示; 记录可展开查看详情; 分页/滚动加载 |
| **SOLO-110-QA** | Sprint 1 集成验证 | 验证 5 个 migrated backend 回归测试; Agent 详情页 UI 检查; 状态指示器功能验证 | qa1 | P0 | Sprint 1 所有任务 | 4 | Backend 单元测试全部通过; Agent 详情页 UI 无严重 Bug |

**Week 1 并行策略**:
- rd1 从 SOLO-100-B 开始, 按大小递减顺序推进: Codex(8h) → Cursor(5h) → Kimi(4h) → Gemini(3h) → Kiro(3h) → Factory(4h)
- rd1 的小 backend (Gemini/Kiro) 可并行推进 (3-4h 一个, 一天可做 2 个)
- fe1 从 SOLO-106-F (详情页骨架) 开始, 完全独立, 可 mock 数据开发 UI
- fe1 推进 SOLO-107-F (状态指示器) 也无需后端
- rd1(27h) + fe1(18h) = 45h, 周总容量 80h, 负荷 56%

### 5.2 Week 2

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-111-B** | 5 Backend 测试强化 | 为 Week 1 迁移的 5 个 Backend 补充完整测试: 正常执行路径、超时取消、无效 CLI 路径、stdout 异常格式、EOF before complete | rd1 | P0 | SOLO-100-B ~ SOLO-105-B | 6 | 每个 Backend 覆盖率 > 70%; 边界条件覆盖完整 |
| **SOLO-112-B** | Skills 系统 — Skills 目录注入 | 实现 `pkg/agent/skills.go`: SkillsManager 在 workspace 中创建 `.claude/skills/` 目录; 按 provider 分离 (claude/codex/cursor); `.md` 技能文件的读取和注入到 CLAUDE.md | rd1 | P1 | SOLO-100-B (workspace 就绪) | 8 | Skills 目录创建成功; 各 provider 子目录分离; 技能文件注入 CLAUDE.md; 单元测试通过 |
| **SOLO-113-B** | Skills 系统 — Skills 注册 API | 在 Server 端新增 Skills API: GET /api/v1/agents/{id}/skills (列出技能), POST /api/v1/agents/{id}/skills (上传新技能); 技能存储在 workspace skills/ 目录 | rd1 | P1 | SOLO-112-B | 4 | API 返回技能列表; 上传技能后文件写入 skills/ 目录 |
| **SOLO-114-F** | Agent 详情页 — 错误/空态/边缘 | 补充 Agent 详情页全部状态: 404 (Agent 不存在) 页面、403 (无权限) 提示、API 错误重试、编辑保存失败提示; Agent 暂无工具的引导 | fe1 | P0 | SOLO-106-F | 4 | 404/403/API 错误全部覆盖; 引导提示清晰; 所有状态可恢复 |
| **SOLO-115-F** | Agent 头像 + 自定义编辑 | Agent 详情页头部头像显示: 使用 `lucide-react` Bot 图标作为默认; 支持从预定义图标列表中选择; 支持显示在线/离线状态圆点覆盖 | fe1 | P0 | SOLO-106-F | 3 | 头像显示正常; 状态圆点覆盖正确; 图标选择 UI 工作正常 |
| **SOLO-116-F** | Agent 执行历史 — 后端联调 | 对接后端 Agent Runs API (GET /api/v1/agents/{id}/runs); 移除 mock 数据; 联调前端 WS 事件更新 | fe1 | P0 | SOLO-109-F, 后端 Agent 执行记录 API (SOLO-137-B 后续, 先 mock) | 3 | 数据来源切换为真实 API; 执行历史实时更新 |
| **SOLO-117-QA** | Sprint 1 完整回归 | 5 个 Backend 回归测试 + Agent 详情页完整 UI 验收 | qa1 | P0 | Sprint 1 所有任务 | 4 | Backend 测试全部通过; Agent 详情页三个 tab 可用 |

**Week 2 并行策略**:
- rd1 推进 SOLO-111-B (测试强化, 6h) 和 SOLO-112-B (Skills 系统, 8h), 两者完全独立
- fe1 推进 SOLO-114-F/115-F/116-F, 全独立任务, 可并行
- rd1(14h) + fe1(10h) = 24h, 周总容量 80h, 负荷 30%

---

## 6. Sprint 2 (Week 3-4): Backend 迁移 Wave 2 + MEMORY + Tools + 工具配置 UI

**Sprint 目标**: 完成剩余 5 个 Backend 迁移 (Copilot/OpenCode/OpenClaw/Hermes/Pi)，MEMORY.md 自动摘要可用，Agent 工具系统可用，前端工具配置 UI 可操作。

**可演示产出 (第 4 周末)**:
1. rd1: 全部 10 个 Backend 迁移完成, `NewBackend("hermes")` 等全部注册
2. rd1: Agent 对话后自动写入 MEMORY.md 摘要
3. rd1: Agent 可通过 read_file/write_file/list_files/search_files 操作 workspace
4. fe1: Agent 详情页"工具 tab"显示可用工具列表, 可启用/禁用和配置参数

### 6.1 Week 3

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-118-B** | 迁移 Copilot Backend | 从 Multica 复制 `copilot.go` (440 行), 适配 Solo 接口; 实现 GitHub Copilot CLI 的 stdin/stdout 通信 | rd1 | P0 | SOLO-100-B (参考架构) | 4 | `copilotBackend.Execute()` 可正常调用; `Name()` 返回 "copilot" |
| **SOLO-119-B** | 迁移 OpenCode Backend | 从 Multica 复制 `opencode.go` (431 行), 适配 Solo 接口; 实现 OpenCode CLI 通信; 构造自定义 JSON stdin; 解析 JSONL stdout | rd1 | P0 | SOLO-100-B | 5 | `opencodeBackend.Execute()` 可正常调用; 单元测试通过 |
| **SOLO-120-B** | 迁移 OpenClaw Backend | 从 Multica 复制 `openclaw.go` (629 行), 适配 Solo 接口; 实现自定义 session 管理和 JSONL 解析 | rd1 | P0 | SOLO-100-B | 6 | `openclawBackend.Execute()` 可正常调用; 单元测试通过 |
| **SOLO-121-B** | 迁移 Hermes Backend | 从 Multica 复制 `hermes.go` (1422 行), 适配 Solo 接口; 最大的迁移, 包含 ACP 协议栈完整实现 + 工具注入层 | rd1 | P0 | SOLO-100-B (参考架构) | 10 | `hermesBackend.Execute()` 可正常调用; ACP 通信协议正确; 工具注入正常 |
| **SOLO-122-B** | 迁移 Pi Backend | 从 Multica 复制 `pi.go` (399 行), 适配 Solo 接口 | rd1 | P0 | SOLO-100-B | 4 | `piBackend.Execute()` 可正常调用; `Name()` 返回 "pi" |
| **SOLO-123-B** | Factory 更新 + Wave 2 测试 | 在 factory.go 注册 copilot/opencode/openclaw/hermes/pi; 编写完整测试套件 | rd1 | P0 | SOLO-118-B ~ SOLO-122-B | 6 | `NewBackend("hermes")` 等全部返回正确实例; 全部 Backend 集成测试通过 |
| **SOLO-124-F** | Agent 工具列表组件 | 在 Agent 详情页新增"工具"tab: 显示可用工具列表 (read_file/write_file/list_files/search_files); 每个工具卡片: 名称+描述+启用开关; 空态 "该 Agent 没有配置工具" | fe1 | P0 | SOLO-106-F | 4 | 工具列表正确显示; 启用/禁用开关工作; 空态引导显示 |
| **SOLO-125-F** | 工具配置表单 | 实现每个工具的配置表单: 文件工具 (允许的路径白名单, 第一行); 搜索工具 (搜索深度限制); 保存后调用 PATCH /agents/{id}/tools | fe1 | P0 | SOLO-124-F | 5 | 工具参数可配置; 表单校验正常; 保存后持久化; 取消恢复原值 |

**Week 3 并行策略**:
- rd1 全力推进 Backend 迁移: Copilot(4h) → Pi(4h) → OpenCode(5h) → OpenClaw(6h) → Hermes(10h) → Factory+Tests(6h)
- 小 Backend (Copilot/Pi) 可各半天完成; OpenCode/OpenClaw 各一天; Hermes 最大, 留 1.5 天
- fe1 推进 SOLO-124-F (工具列表组件) 和 SOLO-125-F (配置表单), mock 数据开发
- rd1(35h) + fe1(9h) = 44h, 周总容量 80h, 负荷 55%

### 6.2 Week 4

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-126-B** | MEMORY.md 自动摘要 — 后处理 | Agent 执行完成后, MemoryManager.SummarizeAfterRun 自动调用 LLM 生成对话摘要, 写入 MEMORY.md; 需要: 从运行结果提取完整对话 + 调用 LLM 简短摘要 + 写入 memory 文件 | rd1 | P0 | SOLO-01-B (AgentRuntime 就绪), MemoryManager 已存在 | 8 | Agent 对话后 MEMORY.md 自动包含摘要; 摘要准确概括对话内容; 长对话不截断 |
| **SOLO-127-B** | MEMORY.md 自动摘要 — 注入到 Prompt | 在 PromptBuilder 中, 自动从 MEMORY.md 读取最新记忆, 注入到 system prompt 的 "Your Memory" 片段; 确保记忆内容不超过 token 限制 (max 4000 chars) | rd1 | P0 | SOLO-126-B | 4 | Agent 每次执行时 system prompt 中包含前次对话记忆; 超长记忆自动截断 |
| **SOLO-128-B** | MEMORY.md 自动摘要 — 配置开关 | agents 表增加 `memory_enabled` 字段; Agent 配置页显示记忆开关; 关闭时跳过后处理和注入; 默认开启 | rd1 | P0 | SOLO-126-B | 4 | 记忆开关影响所有 Agent 执行; DB migration 正确; 开关可通过 API 设置 |
| **SOLO-129-B** | Agent 工具系统 — Tool 接口 + Registry | 实现 `pkg/agent/tools/tool.go`: Tool 接口 (Name/Description/Execute/Parameters); ToolRegistry (注册/查找); JSON Schema 生成器用于 LLM function calling | rd1 | P0 | SOLO-01-B | 6 | Tool 接口编译通过; Registry 注册查找正确; JSON Schema 格式正确 |
| **SOLO-130-B** | Agent 工具系统 — 内置文件工具 | 实现 `pkg/agent/tools/read_file.go` (读取 workspace 内文件, 安全路径检查); `pkg/agent/tools/write_file.go` (写入 output 目录); `pkg/agent/tools/list_files.go` (列出 workspace 文件); `pkg/agent/tools/search_files.go` (grep 搜索) | rd1 | P0 | SOLO-129-B | 8 | 文件读取正常; 路径逃逸被拒绝; 写入限制在 output/; 搜索返回匹配行 |
| **SOLO-131-B** | Agent 工具系统 — 工具注入到 StreamEvent | 在 Backend Execute 流程中, 将启用的工具注入到 LLM function calling 框架; 处理 tool_call → tool_result 循环; 收集 token 用量 | rd1 | P0 | SOLO-130-B | 6 | 工具被正确注入到 LLM 请求; 工具调用结果正确返回; 调用前后用量统计 |
| **SOLO-132-F** | 工具配置 UI — 后端联调 | 对接后端 Agent Tools API (GET/PATCH /api/v1/agents/{id}/tools); 移出 mock 数据; 工具配置保存后实时生效 | fe1 | P0 | SOLO-125-F, SOLO-129-B | 3 | 数据来源切换为真实 API; 配置保存后 Agent 下次执行生效 |
| **SOLO-133-F** | 工具配置 UI — 状态覆盖补充 | 补充工具 tab 的: 加载态骨架屏、API 错误重试、保存失败提示、配置冲突提示、表单脏状态指示 | fe1 | P0 | SOLO-132-F | 3 | 全部状态覆盖; 表单脏状态正确显示; 错误可恢复 |
| **SOLO-134-QA** | Sprint 2 集成验证 | 10 个 Backend 回归测试; MEMORY.md 自动摘要验证; 工具系统功能验证; 工具配置 UI 验收 | qa1 | P0 | Sprint 2 所有任务 | 4 | 全部 Backend 测试通过; 记忆摘要可用; 工具调用 E2E 通过 |

**Week 4 并行策略**:
- rd1 推进 MEMORY (SOLO-126-B/127-B/128-B) + Tools (SOLO-129-B/130-B/131-B)
- MEMORY 和 Tools 可并行 (两套独立模块)
- fe1 推进工具配置 UI 联调和状态覆盖 (SOLO-132-F/133-F), 独立任务
- rd1(28h) + fe1(6h) = 34h, 周总容量 80h, 负荷 43%

---

## 7. Sprint 3 (Week 5-6): 任务系统 + 流式 + 交互模式 + 前端冲刺

**Sprint 目标**: 任务系统后端+前端可用, 流式输出 E2E 验证通过, Agent 交互模式 UI 提供切换。

**可演示产出 (第 6 周末)**:
1. fe1: `/tasks` 可创建/列表/指派任务
2. rd1: 任务系统 API 可用, 数据持久化
3. rd1: Agent 流式输出 E2E (Daemon → SSE → Server → WS → 前端) 工作
4. fe1: Agent 详情页"免打搅/主动参与" 开关生效

### 7.1 Week 5

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-135-B** | 任务系统 — 数据模型 + 迁移 | 创建 tasks 表 (id, title, description, channel_id, assignee_type/assignee_id, creator_id, status, priority, due_date, created_at, updated_at); 任务状态枚举 (todo/in_progress/done/cancelled); 优先级枚举 (p0/p1/p2/p3) | rd1 | P1 | 无 (独立新模块) | 4 | 迁移 up 后表创建成功; 索引存在; 基础 CRUD 查询可运行 |
| **SOLO-136-B** | 任务系统 — REST API | CRUD /api/v1/tasks: POST (创建, 含指派), GET (列表, 支持 status/assignee 过滤, 分页), GET /{id}, PATCH (更新状态/指派/优先级), DELETE; 权限验证 (只能操作自己创建或指派的) | rd1 | P1 | SOLO-135-B, SOLO-05-B (auth) | 8 | 任务 CRUD 正常; 过滤/分页正常; 权限正确; 指派变更通知 |
| **SOLO-137-B** | 任务系统 — WS 事件 | WS 事件 task.created / task.updated / task.assigned: 创建/更新/指派后广播给相关用户; 支持订阅频道级别的任务通知 | rd1 | P1 | SOLO-136-B, SOLO-15-B (WS Hub) | 4 | WS 推送任务事件; 频道成员收到指派通知; 前端 WS 订阅正常 |
| **SOLO-138-B** | 任务系统 — Agent 执行记录表 | 创建 agent_runs 表 (id, agent_id, channel_id, thread_id, trigger_type, status, started_at, finished_at, error, token_usage); GET /api/v1/agents/{id}/runs 查询接口 | rd1 | P1 | SOLO-135-B (参考架构) | 4 | agent_runs 表创建; 查询接口返回正确数据; 运行记录自动写入 |
| **SOLO-139-F** | 任务列表页 | 实现 `/tasks` 路由和页面: 任务卡片列表 (标题/状态/优先级/指派对象); 过滤栏 (全部/我的/待办/已完成); 空态"还没有任务"; 加载态骨架屏 | fe1 | P0 | 无 (可 mock 数据先开发) | 6 | 任务列表正确显示; 过滤条件生效; 空态引导显示; 分页/滚动加载 |
| **SOLO-140-F** | 任务创建表单 | 任务创建 Modal/页面: 标题必填、描述选填、优先级选择 (P0-P3)、指派人选择 (用户/Agent)、频道关联 (可选); 表单校验; 创建成功后跳转到任务详情 | fe1 | P0 | SOLO-139-F | 6 | 创建表单校验正常; 指派人选择器可用; 创建后跳转到详情 |
| **SOLO-141-F** | 任务详情页 | 实现 `/tasks/{id}` 页面: 任务信息展示 (标题/描述/优先级/状态/指派人/创建者/频道/时间); 状态变更按钮 (todo→in_progress→done); 编辑描述; 删除确认 | fe1 | P0 | SOLO-140-F, SOLO-136-B (后端 API) | 6 | 任务信息展示完整; 状态变更正确; 编辑描述保存成功; 删除正常 |

**Week 5 并行策略**:
- rd1 从 SOLO-135-B (迁移) → SOLO-136-B (API) → SOLO-137-B (WS) → SOLO-138-B (执行记录)
- fe1 从 SOLO-139-F (任务列表) → SOLO-140-F (创建表单) → SOLO-141-F (详情页), mock 数据并行开发
- rd1(20h) + fe1(18h) = 38h, 周总容量 80h, 负荷 48%

### 7.2 Week 6

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| **SOLO-142-B** | 流式输出 — Daemon SSE 管道验证 | 确保 Agent 执行全程 SSE 推送: event:thinking → token → tool_call → tool_result → complete/error; 检查 Daemon handler 的 SSE 实现, 确保所有 StreamEvent 类型正确转发到 HTTP Response; 补充超时断开 SSE 连接逻辑 | rd1 | P1 | SOLO-01-B (Daemon handler 就绪) | 6 | SSE 推送覆盖全部 StreamEvent 类型; 前端收到实时 token; 流式中断正常 |
| **SOLO-143-B** | 流式输出 — Server WS 转发 | Server 接收 Daemon SSE 事件后, 转换为 WS 事件 `message.agent_typing` / `message.new` (逐 token); 流式过程中 Agent 消息 ID 不变, content 持续追加; 完成后持久化到 DB | rd1 | P1 | SOLO-142-B, SOLO-17-B (WS message) | 6 | Agent 流式内容通过 WS 实时推送到前端; 消息 ID 在流式中一致; 完成持久化 |
| **SOLO-144-B** | 流式输出 — 前端联调验证 | WS `message.agent_typing` + 逐 token `message.new` 前端消费; Agent 生成内容实时渲染; 验证首 token 延迟 < 2s; 同步测试 | rd1 | P1 | SOLO-143-B | 4 | 首 token < 2s; 逐 token 渲染无丢失; 完成事件后内容稳定 |
| **SOLO-145-B** | Agent 交互模式后端 | agents 表增加 `interaction_mode` 字段 ('active'/'passive'); 自动响应逻辑根据 mode 判断是否触发; passive 模式仅 @mention 触发 | rd1 | P1 | SOLO-43-B (自动响应逻辑) | 4 | passive 模式下 Agent 不自动响应频道消息; active 模式恢复自动响应 |
| **SOLO-146-F** | 任务系统 UI — 后端联调 | 任务列表/创建/详情页切换为真实 API 数据; 移出 mock; 联调 WS task.created/updated 实时更新 | fe1 | P0 | SOLO-139-F/140-F/141-F, SOLO-136-B/137-B | 4 | 数据来源全部切换为真实 API; WS 推送实时更新任务状态 |
| **SOLO-147-F** | 任务系统 UI — 状态覆盖 | 补充全部状态: 加载态、空态、错误重试、404 (任务不存在) 页面、编辑保存失败、状态变更冲突提示 | fe1 | P0 | SOLO-146-F | 4 | 全部状态覆盖; 错误提示可读; 编辑冲突提示 |
| **SOLO-148-F** | Agent 交互模式 UI 开关 | Agent 详情页"基本设置"tab 或 header 显示交互模式开关: "免打搅" vs "主动参与"; 切换时调用 PATCH /agents/{id}; 保存后立即生效; 提示交互模式影响范围 | fe1 | P1 | SOLO-145-B | 4 | 开关 UI 可用; 切换后持久化; 提示说明清晰 |
| **SOLO-149-F** | Agent 详情页 — 全面 UI 打磨 | 补充 Agent 详情页: 所有 tab 的状态覆盖 (loading/empty/error/edge); 页面响应式; 键盘快捷键 (Escape 关闭弹窗等); 中文文案统一 | fe1 | P1 | Sprint 1-3 全部 Agent 前端任务 | 6 | Lighthouse > 80; 响应式布局正常; 全部状态覆盖; 文案统一 |
| **SOLO-150-QA** | v1.2 全功能回归测试 | 10 个 Backend 回归; MEMORY.md 自动摘要验证; 工具系统 E2E 测试; 任务系统 CRUD 验证; 流式输出验证; Agent 交互模式验证 | qa1 | P0 | Sprint 3 所有任务 | 6 | 全部回归测试通过; 无 blocking/critical Bug |

**Week 6 并行策略**:
- rd1 推进流式 (SOLO-142-B/143-B/144-B) + 交互模式 (SOLO-145-B)
- fe1 推进任务系统联调 (SOLO-146-F/147-F) + 交互模式 UI (SOLO-148-F) + 打磨 (SOLO-149-F)
- rd1(16h) + fe1(18h) = 34h, 周总容量 80h, 负荷 43%

---

## 8. 跨 Sprint 依赖关系

```
Sprint 1 (W1-2)                    Sprint 2 (W3-4)                     Sprint 3 (W5-6)
┌──────────────────────────┐       ┌───────────────────────────┐       ┌────────────────────────────┐
│ Codex/Cursor Backend     │       │ Copilot/OpenCode Backend  │       │ 任务系统后端 API (rd1)      │
│ Gemine/Kimi/Kiro Backend │───┐   │ OpenClaw/Hermes/Pi Backend│       │ 任务系统 UI (fe1)           │
│ Factory注册 + 测试       │   │   │ 测试强化(rd1)             │       │                            │
│ Skills 系统 (P1, rd1)    │   │   │                           │       │ 流式输出 E2E (rd1)          │
└──────────────────────────┘   │   └───────────────────────────┘       │ Agent交互模式后端 (rd1)     │
                               │   ┌───────────────────────────┐       │ Agent交互模式 UI (fe1)      │
┌──────────────────────────┐   │   │ MEMORY.md 自动摘要 (rd1)   │       │ Agent 详情页打磨 (fe1)      │
│ Agent详情页骨架 (fe1)     │   └───│ Agent 工具系统 (rd1)       │       └────────────────────────────┘
│ 状态指示器组件 (fe1)      │       │ Agent 工具配置 UI (fe1)    │
│ Runtime Tab + 历史 (fe1)  │       └───────────────────────────┘
└──────────────────────────┘

Sprint 1 完成后:
  - Skills 系统 (rd1, P1) → 为 Sprint 2 的工具系统提供技能注入渠道
  - Agent 详情页 (fe1)     → 为 Sprint 2 的工具配置 UI 提供 tab 容器

Sprint 2 完成后:
  - Agent 工具系统 (rd1)   → 为 Sprint 3 的流式输出提供工具执行能力
  - 工具配置 UI (fe1)      → 下游无阻塞, 独立交付

Sprint 3 中:
  - 任务系统 API (rd1)     → 前端联调依赖于后端 API 完成 (~Week 5 中)
  - 流式输出 E2E (rd1)     → 前端联调依赖于 SSE/WS 管道就绪 (~Week 6 中)
```

### 关键路径 (Critical Path)

```
Week 1    Week 2    Week 3    Week 4    Week 5    Week 6
├─────────┼─────────┼─────────┼─────────┼─────────┼─────────┤
Codex → Cursor → Gemine → Kimi → Kiro → Factory+Test           ← Backend Wave 1 关键路径 (rd1)
                                            │
                                    Copilot → OpenCode → OpenClaw → Hermes → Pi → Factory+Test  ← Backend Wave 2 关键路径 (rd1)
                                                                                  │
                                                                       MEMORY → Tools          ← MEMORY+Tools 关键路径 (rd1)
                                                                                  │
                                                                       Task API → WS Events ← 任务系统关键路径 (rd1)
                                                                                  │
                                                                       SSE → WS → 联调       ← 流式输出关键路径 (rd1)

Agent详情骨架 → 状态组件 → RuntimeTab → 历史Tab → 状态覆盖         ← Agent 详情关键路径 (fe1)
                                                      │
                                       工具列表 → 配置表单 → 联调   ← 工具配置关键路径 (fe1)
                                                      │
                                       任务列表 → 创建 → 详情 → 联调 ← 任务系统 UI 关键路径 (fe1)
                                                                      交互模式 UI → 打磨    ← 前端冲刺关键路径 (fe1)
```

---

## 9. 风险登记册

| # | 风险描述 | 概率 | 影响 | 触发条件 | 缓解措施 | 应急预案 | 负责人 |
|---|---------|------|------|----------|----------|----------|--------|
| R1 | Multica Backend 代码与 Solo 接口不兼容, 每个迁移需要额外适配时间 | 中 | 高 | 迁移时发现接口类型不匹配 | Week 1 先做 Codex (最大文件, 暴露问题最早); 代码走读明确适配点 | 优先迁移波次中调整最复杂的; 小的 backend (Gemini/Pi) 留到后尾做缓冲 | rd1 |
| R2 | rd1 单点过载 — 6 周内承担 10 个 Backend + MEMORY + Tools + 任务系统 + 流式 (总计 ~105h 后端工作) | 高 | 高 | Week 3 发现 rd1 进度落后 | Week 1-2 严格按计划推进; P1 任务 (Skills/流式) 在 P0 完成后才启动 | 将 P1 的 Skills 系统推迟到 v1.3; 流式输出降级为非流式 fallback | tpm |
| R3 | fe1 单点 — 需独立完成 Agent 详情+工具配置+任务系统+交互模式全部前端 (总计 ~60h) | 中 | 高 | Week 5 前发现 fe1 任务积压 | Week 1-2 优先完成 Agent 详情页核心; 工具配置 UI 在 Sprint 2 完整交付 | 任务系统 UI 降级为最小可行版本 (仅列表+创建无指派) | tpm |
| R4 | Hermes Backend (1422 行) 迁移量最大, 可能超预估 | 中 | 中 | Week 3 末 Hermes 未完成 | Week 3 优先做小 Backend, Hermes 留到 Week 3 第 1 天最后一个 | 分两天做 Hermes (先裸 CLI 通信, 再 ACP 协议解析) | rd1 |
| R5 | MEMORY.md 自动摘要的 LLM 二次调用增加成本 (每次 Agent 对话 +1 次 LLM 调用) | 高 | 中 | 记忆摘要导致 API 费用翻倍 | 默认只对 >5 轮对话做摘要; 提供 `memory_summary_threshold` 配置 | 关闭自动摘要, 改为定时批量摘要 (每天 1 次) | rd1 |
| R6 | 工具系统安全: 文件工具路径逃逸可能导致 Server 文件泄露 | 中 | 高 | 工具实现路径校验不完整 | 路径严格限制在 workspace 目录内; 使用 `filepath.Clean` + 前缀检查; code review 重点审查 | 关闭工具系统降级到安全模式, 仅允许 read_file | rd1 |
| R7 | 任务系统前端 API 联调时间超出预期 | 中 | 中 | Week 5 末联调未完成 | fe1 先 mock 数据并行开发 UI; rd1 优先完成 API 再联调 | 前端先支持任务列表 mock, 联调在 Week 6 第一个工作日专门安排 | rd1+fe1 |

---

## 10. 附录 A: 任务统计汇总

| Sprint | 周次 | 后端任务数 | 前端任务数 | 测试任务数 | 总计 | 预估总工时(h) | 关键交付 |
|--------|------|-----------|-----------|-----------|------|-------------|---------|
| Sprint 1 | Week 1 | 6 | 4 | 0 | 10 | 45 | Codex/Cursor/Gemini/Kimi/Kiro Backend + Agent 详情骨架 |
| Sprint 1 | Week 2 | 2 | 3 | 1 | 6 | 24 | Skills 系统 + Agent 详情打磨 |
| Sprint 2 | Week 3 | 6 | 2 | 0 | 8 | 44 | Copilot/OpenCode/OpenClaw/Hermes/Pi Backend + 工具列表组件 |
| Sprint 2 | Week 4 | 6 | 2 | 1 | 9 | 38 | MEMORY + 工具系统 + 工具配置 UI |
| Sprint 3 | Week 5 | 4 | 3 | 0 | 7 | 38 | 任务系统后端 + 任务列表/创建/详情 UI |
| Sprint 3 | Week 6 | 4 | 4 | 1 | 9 | 40 | 流式输出 + 交互模式 + 打磨回归 |
| **总计** | **6 周** | **28** | **18** | **3** | **49** | **229h** | |

### 按优先级分类

| 优先级 | 任务数 | 总工时 | 占比 |
|--------|-------|--------|------|
| P0 | 26 | 137h | 60% |
| P1 | 23 | 92h | 40% |
| P2 | 0 | 0h | 0% |

### 按负责人分类

| 负责人 | 任务数 | 总工时 | 周均负荷 |
|--------|-------|--------|---------|
| rd1 | 28 | 105h | 17.5h/周 (44%) |
| fe1 | 18 | 60h | 10h/周 (25%) |
| qa1 | 3 | 12h | 按 Sprint 末 2 天 |
| **总计** | **49** | **229h** | |

---

## 11. 附录 B: 团队容量检查

| 角色 | 周可用(h) | 实际排期(h) | 峰值(h) | 是否超载 | 说明 |
|------|----------|------------|--------|----------|------|
| rd1 | 40 | W1:27 / W2:14 / W3:35 / W4:28 / W5:20 / W6:16 | W3:35 (88%) | **⚠️ W3 接近超载** | W3 是 Backend Wave 2 核心周, Hermes(10h) 是最大单项; W1 的 Factory Tests(4h) 也可适度减负 |
| fe1 | 40 | W1:18 / W2:10 / W3:9 / W4:6 / W5:18 / W6:18 | W1/5/6:18 (45%) | 否 | 峰值在 Sprint 1 和 3, 只要完成核心组件, 时间相当充裕 |
| qa1 | 16 (40%) | W2:4 / W4:4 / W6:6 + W3:4 | W6:6 | 否 | 50% 投入, 仅 Sprint 末介入 |

**容量总计**: 6 周总可用容量 = rd1(240h) + fe1(240h) + qa1(96h) = 576h。实际排期 = 229h, 总体利用率 40%。rd1 W3 利用率 88% 是唯一需要注意的瓶颈期。

**建议**: 如果 rd1 W3 的 35h 排期导致压力, 可将 SOLO-112-B (Skills 系统, 8h) 从 W2 推迟到 W4 末或 W5 初。

---

## 12. 附录 C: 各 Backend 迁移工作量评估

| Backend | 原始行数 | CLI 名称 | 通信协议 | 适配复杂度 | 预估(h) |
|---------|---------|---------|---------|-----------|--------|
| Codex | 1179 | `codex` | JSON-RPC 2.0 | 高 — 最复杂协议 | 8 |
| Hermes | 1422 | `hermes` | ACP | 高 — 最大文件 + 工具注入 | 10 |
| OpenClaw | 629 | `openclaw` | 自定义 JSON | 中 — 自定义 session | 6 |
| OpenCode | 431 | `opencode` | 自定义 JSON | 中 | 5 |
| Cursor | 422 | `cursor` | 自定义 JSON | 中 — 含 invocation 逻辑 | 5 |
| Copilot | 440 | `copilot` | 自定义 JSON | 中 | 4 |
| Kimi | 403 | `kimi` | ACP | 低 — 有 ACP 参考 | 4 |
| Pi | 399 | `pi` | 自定义 JSON | 低 | 4 |
| Kiro | 387 | `kiro` | ACP | 低 — 同 Kimi 协议族 | 3 |
| Gemini | 267 | `gemini` | 自定义 JSON | 低 — 最小文件 | 3 |
| **总计** | **5979** | | | | **52h** |

> 每个 Backend 的工作量包含: 复制 + 接口适配 + arg 适配 + 输出解析 + 单元测试。
> 按平均 2-3 个/周, 6 周内 10 个全部完成是可行的 (52h / 6 周 = 8.7h/周, 但在 Sprint 激增周实际是 Wave 1 的 20h 和 Wave 2 的 35h)。

---

## 附录 D: 启动消息

### 给 rd1 的启动消息

> **主题**: Solo v1.2 启动 — Backend 迁移 + MEMORY + Tools (6 周)
>
> rd1 你好,
>
> v1.1 交付得非常棒（83 测试通过）。v1.2 的核心是完成 Agent 系统的深度增强，让 Solo 支持 11 种 Agent Backend（含 Claude）。
>
> 你的 6 周任务概览:
>
> **P0 — 必须完成 (核心交付)**
> 1. Sprint 1 (Week 1-2): 迁移 Codex/Cursor/Gemini/Kimi/Kiro Backend (20h)
> 2. Sprint 2 (Week 3-4): 迁移 Copilot/OpenCode/OpenClaw/Hermes/Pi Backend (29h) + 测试 (6h)
> 3. Sprint 2 (Week 4): MEMORY.md 自动摘要 (12h) + 工具系统 (20h)
>
> **P1 — 目标交付 (尽力而为)**
> 4. Sprint 2 (Week 2): Skills 系统 (12h)
> 5. Sprint 3 (Week 5-6): 任务系统后端 (16h) + 流式输出 E2E (16h) + 交互模式后端 (4h)
>
> **Week 1 启动任务** (按优先级):
> 1. [P0] SOLO-100-B: 迁移 Codex Backend — 8h (最大, 先消化最难)
> 2. [P0] SOLO-101-B: 迁移 Cursor Backend — 5h
> 3. [P0] SOLO-102-B: 迁移 Gemini Backend — 3h
> 4. [P0] SOLO-103-B: 迁移 Kimi Backend — 4h
> 5. [P0] SOLO-104-B: 迁移 Kiro Backend — 3h
> 6. [P0] SOLO-105-B: Factory 更新 + 测试 — 4h
>
> **迁移注意事项**:
> - 每个 Backend 的接口适配模式: 将 `Execute(ctx, prompt string, opts ExecOptions)` 改为 `Execute(ctx, *ExecuteRequest, *ExecuteOptions)`, 实现 `Name() string`
> - CLI 参数构造和输出解析逻辑**直接复用** Multica 代码（协议级, 无耦合）
> - testdata 从 Multica 直接复制
> - Week 1 先做 Codex (1179 行, JSON-RPC 2.0 最复杂的一个), 尽早暴露集成问题
> - Hermes (1422 行) 留到 Week 3 最后做, 前面的 4 个 Backend 积累经验后更有把握
>
> **Week 3 是峰值周** (35h 排期), 主要是 Hermes(10h) + 其他 Wave 2 Backend。届时可根据进度灵活调整。
>
> 输出: Sprint 1 结束 (Week 2 末) 时 5 个 Backend 单元测试通过; Sprint 2 结束 (Week 4 末) 时全部 10 个完成 + MEMORY + Tools 可用; Sprint 3 结束 (Week 6 末) 时任务系统 + 流式可用。

### 给 fe1 的启动消息

> **主题**: Solo v1.2 启动 — Agent 详情面板 + 工具配置 UI + 任务系统 UI (6 周)
>
> fe1 你好,
>
> v1.1 前端框架已经就绪 (Agent 列表、频道、消息等)。v1.2 的前端目标是完成 Agent 系统三个核心 UI:
>
> - Agent 详情面板 (运行时/状态/历史)
> - Agent 工具配置 UI (启用/禁用/参数配置)
> - 任务系统 UI (创建/列表/指派)
>
> 你的 6 周任务概览:
>
> **P0 — 必须完成 (核心交付)**
> 1. Sprint 1 (Week 1-2): Agent 详情页骨架 + 状态指示器 + Runtime Tab + 执行历史
> 2. Sprint 2 (Week 3-4): Agent 工具配置 UI (列表/表单/联调)
> 3. Sprint 3 (Week 5-6): 任务系统 UI (列表/创建/详情/联调)
>
> **P1 — 目标交付 (尽力而为)**
> 4. Sprint 3 (Week 6): Agent 交互模式 UI 开关
>
> **Week 1 启动任务** (按优先级):
> 1. [P0] SOLO-106-F: Agent 详情页路由 + 骨架 — 4h (独立, 可 mock 数据)
> 2. [P0] SOLO-107-F: Agent 状态指示器组件 — 3h
> 3. [P0] SOLO-108-F: Agent Runtime Tab — 5h
> 4. [P0] SOLO-109-F: Agent 执行历史列表 — 6h
>
> **前两周的** Agent 详情页完全独立, 可以 mock 数据开发 UI, 不需要等后端。
>
> **设计方向**:
> - Agent 详情页: `/agents/[id]`, 三 tab 布局 (运行时/工具/执行历史)
> - 状态指示器: 绿(在线) 黄(思考) 蓝(输出) 灰(离线), 尺寸适配列表卡片和详情页
> - 工具配置: 卡片列表 + 开关 + 可折叠参数配置面板
> - 任务系统: 全屏页面, 类看板列表布局
>
> **联调节点**:
> - Week 4: 工具配置 UI 联调 (SOLO-132-F)
> - Week 5: 任务系统 UI 联调 (SOLO-146-F)
> - Week 6: 交互模式 UI 联调 (SOLO-148-F)
>
> 输出: Sprint 1 结束 (Week 2 末) 时 Agent 详情页三个 tab 可用; Sprint 2 结束 (Week 4 末) 时工具配置 UI 可用; Sprint 3 结束 (Week 6 末) 时任务系统 UI + 交互模式可用。

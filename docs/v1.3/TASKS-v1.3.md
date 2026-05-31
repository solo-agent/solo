# Solo v1.3 — 任务分解与迭代规划 (WBS) v3.0

> 版本: 3.0
> 创建日期: 2026-05-17
> 最后更新: 2026-05-30 (v3.0: Phase 5 收尾后全面重评估，合并 pm1 + tpm 双视角)
> 对齐文档: docs/v1.3/PRD-v1.3.md, docs/v1.3/ARCHITECTURE-v1.3.md, docs/v1.3/PHASE5-WRAP-UP.md

---

## 目录

1. [Phase 5: 架构转型 (Sprint 1, 已完成)](#1-phase-5-架构转型-sprint-1-已完成)
2. [Phase 5 额外交付 — 原 Sprint 2-4 提前实现的任务](#2-phase-5-额外交付--原-sprint-2-4-提前实现的任务)
3. [Phase 5 废弃的任务](#3-phase-5-废弃的任务)
4. [Sprint 2': 前端统一 + 委派模式 + Token 刷新 + E2E](#4-sprint-2-前端统一--委派模式--token-刷新--e2e)
5. [Sprint 3': DM UI + 搜索 UI + 团队管理 + 文件补齐](#5-sprint-3-dm-ui--搜索-ui--团队管理--文件补齐)
6. [Sprint 4': 电脑 UI + 文件上传 UI + 全功能回归 + P2](#6-sprint-4-电脑-ui--文件上传-ui--全功能回归--p2)
7. [汇总对比](#7-汇总对比)
8. [关键路径与依赖](#8-关键路径与依赖)
9. [风险登记册](#9-风险登记册)
10. [团队角色与分工](#10-团队角色与分工)

---

## 1. Phase 5: 架构转型 (Sprint 1, 已完成)

**日期**: 2026-05-19 ~ 2026-05-27
**目标**: 对标 Slock 协作质量 — Agent 自主决策、持久化会话、双通道架构、完整 System Prompt

### 1.1 架构变更

#### 双通道架构

```
Agent 思考过程 → 本地（不推送前端）
Agent CLI 消息 → daemon proxy → server API → WS message.new → 前端
```

- SSE 不再流式推送 MessageText（仅累积供日志）
- 消息投递完全走 proxy → API → message.new 路径
- `skip_persist` 已移除，complete event 仅通知 usage

#### 持久化会话

- `PersistentBackend` 接口：Start/Send/Close
- `AgentSessionManager`：会话池、turn 串行锁、idle reaper (5min)
- `--system-prompt-file .solo/system-prompt.md` 替代 `--append-system-prompt`

#### System Prompt

- 完全对齐 Slock，15 节，~350 行
- 包含：Runtime Context、CRITICAL RULES、Startup sequence、Messaging、Tasks、@Mentions、Communication style、Workspace & Memory、Compaction safety

#### 部署架构

```
Docker:  postgres + server
Host:    daemon + solo CLI
```

- `docker compose up -d` 启动 postgres + server
- daemon 在主机运行（需启动 Claude Code 进程）
- `make start` 全栈启动

### 1.2 关键文件变更

| 文件 | 变更 |
|------|------|
| `pkg/agent/prompt.go` | System Prompt 完全重写，15 节 ~387 行 |
| `pkg/agent/claude.go` | --system-prompt-file + persistent session |
| `pkg/agent/session.go` | AgentSessionManager 新增 |
| `pkg/agent/workspace.go` | 移除 InjectInstructionFiles / buildRuntimeCLAUDE |
| `pkg/agent/claude_template.go` | 移除 BuildCLAUDEBytes / BuildAGENTSBytes / BuildGEMINIBytes |
| `internal/server/handler/message.go` | asTask 先 INSERT 再 ConvertToTask、thread reply 实时广播 |
| `internal/server/handler/task.go` | broadcastSystemMessageWithID showInChannel bool、claim 不广播 channel |
| `internal/server/handler/dm.go` | DM 任务 CRUD + ConvertMessageToTask |
| `internal/server/handler/search.go` | PostgreSQL FTS 搜索 + ts_headline 高亮 + 游标分页 |
| `internal/server/handler/computer.go` | 电脑管理 CRUD API |
| `internal/server/handler/attachment.go` | 文件上传 API + 缩略图生成 |
| `internal/server/service/agent.go` | TriggerAgentForTask Slock 格式消息、agentChain 追踪、防死循环 |
| `internal/server/service/task.go` | creator_name JOIN、UUID WHERE 修复、FOR UPDATE 防冲突 |
| `cmd/solo/main.go` | 12 命令 Slock 风格输出、-m flag、stdin heredoc、proxy 路由 |
| `cmd/daemon/handler.go` | proxy 路由、SOLO_AUTH_TOKEN 注入 |
| `frontend/components/dashboard/channel-view.tsx` | asTask 走 sendMessage，不再用 createTask |
| `frontend/components/dashboard/thread-panel.tsx` | 任务元数据展示 |
| `frontend/components/tasks/task-card.tsx` | 父子任务视觉关联 (parent badge + progress bar) |
| `frontend/components/dashboard/message-input.tsx` | 拖拽上传 + 粘贴支持 |
| `frontend/app/teams/page.tsx` | Agent 团队管理页 (490 行) |
| `frontend/app/computers/page.tsx` | 电脑管理页 (530 行) |
| `frontend/components/search/` | CMD+K 全局搜索 + 频道内搜索 |

### 1.3 CLI 输出格式

- claim 成功: `Claim results (1 claimed): #N (msg:XXXX): claimed` + thread target
- claim 失败: `Claim results (0 claimed, 1 failed): #N: FAILED — already assigned. Do not reply.`
- task update: `#N moved to in_review.`
- message send: `Message sent to thread. Message ID: UUID (to reply in this thread, use target "channel:id")`

### 1.4 死代码清理

- `broadcastClaimSystemMessage`、`broadcastUnclaimSystemMessage`
- `buildRuntimeCLAUDE` (workspace.go)
- `BuildCLAUDEBytes`、`BuildAGENTSBytes`、`BuildGEMINIBytes`
- `InjectInstructionFiles`、`WriteSystemPromptFile`
- `claude_template_test.go`

### 1.5 已知限制

- Docker compose 需要网络（拉取 golang:1.22-alpine 和 go modules）
- 持久化 session token 超过 15 分钟过期——需 daemon proxy 刷新（见 Sprint 2' R10 修复）
- 只读 CLI 命令（message read、message check 等）未全部走 proxy
- 非 proxy 命令在持久化 session 中存在 token 过期风险
- 4 个测试失败需修复

---

## 2. Phase 5 额外交付 — 原 Sprint 2-4 提前实现的任务

Phase 5 实施过程中，以下原 Sprint 2-4 任务已被提前实现（代码验证确认）：

| ID | 标题 | 原 Sprint | 实现证据 |
|----|------|----------|----------|
| SOLO-217a-B | Task API — parent_task_id 过滤 + 子任务计数 | Sprint 2 W3 | migration 000016 已建立字段+索引；task.go 支持 `?parent_id=<uuid>`；响应含 subtask_count/done_subtask_count |
| SOLO-220-B | Agent 自主认领 — 状态机 + 防冲突 | Sprint 2 W4 | `task.go:287` FOR UPDATE 事务锁；状态验证 (todo/in_progress)；认领人冲突检测 |
| SOLO-221-B | Agent 自主认领 — 触发通知优化 | Sprint 2 W4 | Slock 格式通知；角色由 system prompt 定义；不依赖 DB role 字段 |
| SOLO-227-B | Agent-to-Agent @mention 触发链路 | Sprint 3 W5 | agentChain 追踪；TriggerAgentResponse 含 mention 检测；System Prompt 含 @Mentions 章节 |
| SOLO-228-B | Agent-to-Agent 防死循环 | Sprint 3 W5 | maxAgentChainDepth=3 + 重复检测；线程侧同样有检测 |
| SOLO-229-B | DM 任务 API (大部分) | Sprint 3 W5 | dm.go 中 CreateTask/ListTasks/GetTask/UpdateTask/ClaimTask/UnclaimTask |
| SOLO-230-B | DM 任务 — asTask 支持 | Sprint 3 W5 | `dm.go:943` ConvertMessageToTask handler |
| SOLO-234-B | 消息搜索 — PostgreSQL FTS | Sprint 3 W6 | search.go 完整实现；plainto_tsquery + GIN 索引 + 游标分页 + 权限过滤 |
| SOLO-235-B | 消息搜索 — 高亮 + 分页 | Sprint 3 W6 | search.go 包含 ts_headline 高亮、limit/before 游标、频道过滤 |
| SOLO-241-B | 电脑管理 — 数据模型 + API | Sprint 4 W7 | migration 000018 computers 表；computer.go CRUD handler |
| SOLO-243-B | 文件上传 — API + 存储 | Sprint 4 W7 | migration 000019 attachments 表；attachment.go Upload/Serve handler |
| SOLO-248-B | 文件上传 — 消息关联 (大部分) | Sprint 4 W8 | thread.go:229 支持 attachment_ids；migration 000020 |
| — | 前端 — teams 页面 | Sprint 3 | teams/page.tsx 490 行 |
| — | 前端 — computers 页面 | Sprint 4 | computers/page.tsx 530 行 |
| — | 前端 — 搜索面板 | Sprint 3 | global-search.tsx + global-search-trigger.tsx + channel-search.tsx |
| — | 前端 — 任务卡片父子关联 | Sprint 2 | task-card.tsx 含 parent_task_id/isChild/parent badge/subtask progress |
| — | 前端 — 拖拽/粘贴上传 | Sprint 4 | message-input.tsx 含 isDragging/dragCounter/onPaste |
| — | 前端 — ThreadPanel 任务元数据 | Sprint 2 | thread-panel.tsx 含 task_number/task_status/assignee/priority |

---

## 3. Phase 5 废弃的任务

| 编号 | 原 Sprint | 废弃原因 |
|------|----------|----------|
| A-03 / SOLO-213-ARC | Sprint 2 | CLAUDE.md 生成——System Prompt 是唯一真相来源，CLI 手册内联 |
| A-04 / SOLO-214-ARC | Sprint 2 | AGENTS.md/GEMINI.md 生成——同上 |
| SOLO-215-B | Sprint 2 | WorkspaceManager 多文件注入——InjectInstructionFiles 已移除 |
| A-06 / SOLO-209-ARC | Sprint 1 | 角色模板——角色由用户自定义 system_prompt 实现 |
| SOLO-216-B | Sprint 2 | Regex claim 协议解析——改为 CLI-based (`solo task claim`) |
| SOLO-217-B | Sprint 2 | Regex update 协议解析——改为 CLI-based (`solo task update`) |
| SOLO-222-B | Sprint 2 | System Prompt 引用 CLAUDE.md——CLAUDE.md 已移除 |
| SOLO-210-F | Sprint 1 | 模板选择器 UI——不再需要角色模板预设 |
| SOLO-211-F | Sprint 1 | Agent 列表 system prompt 预览——角色 badge 不依赖 DB 字段 |

---

## 4. Sprint 2': 前端统一 + 委派模式 + Token 刷新 + E2E

**日期**: Week 1-2 (2026-06-01 ~ 2026-06-12)
**Sprint 目标**: 修复测试基线 + Token 刷新 + 前端 ThreadPanel 统一 + CLI E2E 验证 + 委派决策注入

**可演示产出**:
1. 4 个测试失败全部修复，全量测试通过为后续 QA 提供基线
2. 持久化 session token 自动刷新，Agent 长任务不再中断
3. 点击任务 → ThreadPanel 展开，元数据完整（状态/认领人/优先级/子任务计数）
4. Kanban 上父子任务视觉关联
5. Claude Code Agent 走通 CLI claim → work → done 闭环
6. System Prompt 含委派模式决策树 (模式 A vs B)

### Week 1

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-256-B** | 修复 4 个测试失败 | 修复 TestHandleTaskClaimConflict、TestHandleMessageSend、TestMessageHandler_List_ValidCursorFormat、TestHubUnregisterRemovesFromChannels | rd2 | P0 | 4 | 4 个测试全部通过；`go test ./...` 零失败 |
| **SOLO-254-B** | 持久化 session token 自动刷新 | Daemon proxy 层实现: access token 过期前自动用 refresh token 续期；所有 proxy 路由命令自动获取有效 token；非 proxy 命令降级提示 | rd1 | P0 | 6 | Agent session >15min 后 CLI 命令不 401；token refresh 透明进行 |
| **SOLO-255-B** | 只读 CLI 命令走 proxy | message read、message check 等只读命令通过 proxy 路由，享受 token 刷新保护 | rd1 | P1 | 3 | 所有只读 CLI 命令可走 proxy；token 过期场景不受影响 |
| **SOLO-257-ARC** | System Prompt 委派决策树追加 | 在现有 15 节 Prompt 中追加 ~15 行委派决策: 模式 A (线程委派 @mention) vs 模式 B (子任务拆分 `solo task create --parent`) 的判断标准、场景信号、决策原则 | arc | P1 | 2 | 决策树定位合适插入点；通过评审；Agent 能基于决策树自主选择模式 |
| **SOLO-218-F** | 统一任务详情为 ThreadPanel | TaskCard onClick → 构造 message object → 打开 ThreadPanel；ThreadPanel 头部显示任务元数据（标题/状态徽章/认领人/优先级）；移除旧 TaskDetailPanel 引用（如有残留） | fe1 | P0 | 3 | 点击任务 → ThreadPanel 正确打开；元数据显示完整 |
| **SOLO-219-F** | 移除任务独立评论系统 | 清理 tasks/ 路由中的独立评论组件引用；确认 ThreadPanel 完全替代 | fe2 | P1 | 1 | 前端代码无 task-comment 引用残留 |
| **SOLO-224-F** | ThreadPanel 任务元数据增强验证 | 验证 thread-panel.tsx 中 task_number/task_status/assignee/priority 展示完整性；缺失则补充；补充 "View in Kanban" 跳转链接 | fe2 | P0 | 2 | 头部元数据完整；状态变更实时更新；跳转链接正确 |

**并行策略**:
- rd2: SOLO-256-B (Day 1-2)
- rd1: SOLO-254-B (Day 1-3) → SOLO-255-B (Day 3-5)
- arc: SOLO-257-ARC (Day 1)
- fe1: SOLO-218-F (Day 1-2)
- fe2: SOLO-219-F (Day 2) + SOLO-224-F (Day 1-2)
- 周总工时: rd1(9h) + rd2(4h) + arc(2h) + fe1(3h) + fe2(3h) = 21h

### Week 2

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-224a-F** | TaskBoard 父子任务视觉关联验证 | 验证 task-card.tsx 中 parent_task_id 判断、isChild 标记、parent badge ("子任务 of #N")、subtask progress bar；缺失则修补 | fe2 | P1 | 2 | 父任务进度徽章正确；子任务 parent badge 可点击跳转；视觉层级清晰 |
| **SOLO-225-F** | 频道消息流任务样式验证 | 验证消息流中 task 消息视觉区分度（task 标记图标、状态标签 4 色、认领人 badge）；缺失则补充 | fe1 | P1 | 2 | 任务消息与普通消息一眼可区分；状态标签正确；点击跳转 ThreadPanel |
| **SOLO-253-B** | CLI --parent 参数补充 | `solo task create` 增加 `--parent <task_number>` flag；查询 task number 获取 UUID 作为 parent_task_id | rd2 | P1 | 2 | `solo task create --parent 5` 创建子任务 parent_task_id 正确；不传 --parent 行为不变 |
| **SOLO-223-B** | CLI 路径 E2E 联调 | Claude Code Agent 完整走通: 启动 → 读 system-prompt.md → 看到任务 → `solo task claim -n N` → API 防冲突 → 执行任务 → `solo task update -n N -s done` → 状态变更 | rd1 | P0 | 4 | 3 次成功执行；覆盖 claim 成功/冲突/done 三种场景 |
| **SOLO-229-B** | DM 任务 API 查漏补缺 | 验证 DM 任务 CRUD + Claim 端点完整性；确认 per-DM 编号独立递增；缺失则补充 | rd2 | P1 | 2 | DM 任务 API 端点完整；per-DM 编号正确 |
| **SOLO-226-QA** | Sprint 2' 集成验证 | 测试修复确认；CLI 路径 E2E 通过；token 刷新验证；父子任务 API + 视觉正确；ThreadPanel 统一视图无回归 | qa1 | P0 | 4 | CLI claim→work→done 全流程；ThreadPanel 元数据正确；token 刷新透明 |

**并行策略**:
- fe2: SOLO-224a-F (Day 1)
- fe1: SOLO-225-F (Day 1)
- rd2: SOLO-253-B (Day 1) + SOLO-229-B (Day 1-2)
- rd1: SOLO-223-B (Day 1-2)
- qa1: SOLO-226-QA (Day 3-4)
- 周总工时: rd1(4h) + rd2(4h) + fe1(2h) + fe2(2h) + qa1(4h) = 16h

**Sprint 2' 总计**: 37h

---

## 5. Sprint 3': DM UI + 搜索 UI + 团队管理 + 文件补齐

**日期**: Week 3-4 (2026-06-15 ~ 2026-06-26)
**Sprint 目标**: DM 完整任务看板、CMD+K 全局搜索、团队管理页、图片缩略图、消息附件完善

**可演示产出**:
1. DM 中 TaskBoard 显示 DM 独有任务，"As Task" 消息转换可用
2. CMD+K → 输入关键词 → 跨频道搜索结果高亮 → 点击跳转
3. `/teams` 页面 → 团队分组展示 → 结构可视化
4. 图片上传后自动生成缩略图
5. 消息中附件展示完整（图片内联 + 文件卡片）

### Week 3

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-231-F** | DM 任务 UI — TaskBoard | DM 视图中新增 "Tasks" Tab；复用 TaskBoard 组件；per-DM 独立看板数据；创建任务弹窗适配 DM 上下文 | fe1 | P1 | 4 | DM Tasks Tab 切换正常；Kanban 显示 DM 独有任务；创建弹窗适配 |
| **SOLO-232-F** | DM 任务 UI — 消息转换 + 任务消息样式 | DM 消息 hover 显示 "As Task" 操作；转换后消息样式切换为 task 样式；DM 内任务编号显示 | fe1 | P1 | 2 | DM 中 asTask 可用；任务消息样式正常 |
| **SOLO-236-F** | 搜索 UI — CMD+K 面板验证 | 验证 global-search.tsx: CMD+K 唤起、实时搜索 debounce 300ms、结果高亮、点击跳转；对接到真实搜索 API；缺失则修补 | fe2 | P1 | 3 | CMD+K 唤起面板；搜索 debounce 正确；结果高亮；跳转正确 |
| **SOLO-237-F** | 搜索 UI — 频道内搜索验证 | 验证 channel-search.tsx 在频道视图中的集成；搜索范围限定当前频道；UI 与全局搜索一致 | fe1 | P1 | 2 | 频道内搜索正确；范围限定当前频道 |
| **SOLO-233-F** | Agent 团队管理页面验证 | 验证 teams/page.tsx: Agent 按团队分组展示、拖拽调整、团队 CRUD；对接真实 API | fe2 | P0 | 3 | 团队分组显示正确；拖拽功能可用；数据来源真实 API |

**并行策略**:
- fe1: SOLO-231-F (Day 1-2) + SOLO-232-F (Day 3) + SOLO-237-F (Day 4)
- fe2: SOLO-236-F (Day 1-2) + SOLO-233-F (Day 3-4)
- rd1+rd2: 协助前端联调、缓冲
- 周总工时: fe1(8h) + fe2(6h) = 14h

### Week 4

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-238-F** | Agent 团队管理 — 后端联调验证 | 确认 teams 页面对接真实 API（非 mock）；CRUD 正常；system prompt 变更后列表更新 | fe1 | P0 | 1 | 数据来源真实 API；CRUD 正常 |
| **SOLO-239-F** | 团队结构可视化验证 | 验证 teams/page.tsx 是否按 system prompt 角色关键词分组显示 Agent；空态引导；缺失则补充 | fe2 | P0 | 2 | 结构图清晰；分组合理；空态引导友好 |
| **SOLO-244-B** | 文件上传 — 图片缩略图 | 图片上传后生成缩略图 (max 400x400)；使用 Go 标准库 image 包；支持 jpg/png/gif/webp；非图片不生成 | rd1 | P1 | 3 | 缩略图生成正确；尺寸 max 400x400；非图片文件不生成缩略图 |
| **SOLO-248-B** | 文件上传 — 消息附件完善 | GET messages 返回完整 attachment 元数据 (URL/缩略图/mime/文件名)；确认前端消息组件可读取展示 | rd2 | P1 | 2 | API 返回完整 attachment 元数据；前端可正常渲染 |
| **SOLO-242-B** | 电脑管理 — 心跳验证 | 确认 daemon heartbeat 端点更新 last_heartbeat_at + status 字段；确认离线检测 goroutine 存在 (>60s → offline)；缺失则补充 | rd1 | P1 | 2 | 心跳正常更新；离线检测正确；状态变更在 API 返回中体现 |
| **SOLO-240-QA** | Sprint 3' 集成验证 | DM 任务闭环；搜索全流程 (中文+英文)；团队管理 UI；A→B 协作+防死循环 verified；文件上传+附件展示 | qa1 | P0 | 4 | DM 任务闭环通过；搜索结果准确；团队管理无严重 Bug；文件上传正常 |

**并行策略**:
- fe1: SOLO-238-F (Day 1)
- fe2: SOLO-239-F (Day 1)
- rd1: SOLO-244-B (Day 1-2) + SOLO-242-B (Day 2)
- rd2: SOLO-248-B (Day 1)
- qa1: SOLO-240-QA (Day 3-4)
- 周总工时: rd1(5h) + rd2(2h) + fe1(1h) + fe2(2h) + qa1(4h) = 14h

**Sprint 3' 总计**: 28h

---

## 6. Sprint 4': 电脑 UI + 文件上传 UI + 全功能回归 + P2

**日期**: Week 5-6 (2026-06-29 ~ 2026-07-10)
**Sprint 目标**: 电脑管理 UI 验证上线、文件上传全流程验证、P2 打磨、v1.3 全功能回归

**可演示产出**:
1. `/computers` 显示所有电脑状态卡片（绿/灰指示灯 + 心跳时间）
2. 拖拽文件到消息框 → 上传 → 内联预览 → 发送 全流程可用
3. 消息编辑删除键盘快捷键可用
4. v1.3 全部回归测试通过，可正式发布

### Week 5

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-245-F** | 电脑管理 UI — 列表页验证 | 验证 computers/page.tsx: 卡片网格、状态指示灯(绿/灰)、心跳时间人性化、空态引导；缺失则修补 | fe2 | P1 | 3 | 卡片列表正确；状态实时更新；心跳人性化显示 |
| **SOLO-246-F** | 电脑管理 UI — 详情 + 操作验证 | 验证侧滑面板/详情页: 完整信息、编辑名称、移除电脑(确认弹窗)；操作反馈 | fe2 | P1 | 2 | 信息完整；编辑/删除功能正常；操作反馈 |
| **SOLO-247-F** | 文件上传 UI — 拖拽 + 粘贴验证 | 验证 message-input.tsx: 拖拽区域高亮、Ctrl+V 粘贴、上传进度条、图片内联预览；缺失则修补 | fe1 | P1 | 3 | 拖拽/粘贴上传正常；进度条可见；图片预览正确 |
| **SOLO-249-F** | 文件上传 UI — 消息内展示验证 | 验证 message-attachments.tsx: 图片内联预览 + lightbox 放大、文件卡片 (文件名+大小+下载)、多附件网格布局 | fe1 | P1 | 2 | 图片内联+lightbox 正常；文件卡片信息正确；下载可用 |

**并行策略**:
- fe2: SOLO-245-F (Day 1-2) + SOLO-246-F (Day 3)
- fe1: SOLO-247-F (Day 1-2) + SOLO-249-F (Day 3)
- rd1+rd2: 协助联调、修 Bug
- 周总工时: fe1(5h) + fe2(5h) = 10h

### Week 6

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|---------|----------|
| **SOLO-251-F** | P2: 消息编辑删除完善 | hover 操作菜单动画优化；键盘快捷键: E 编辑、Delete 删除 (需确认)；编辑模式 ESC 取消、Enter 保存；删除确认弹窗样式统一 | fe2 | P2 | 4 | 快捷键可用；编辑/删除体验升级 |
| **SOLO-258-F** | 清理废弃路由 | 确认 `frontend/app/tasks/` 路由是否可安全删除；清理不再引用的组件和 hooks | fe1 | P2 | 1 | 无死路由残留；构建无警告 |
| **SOLO-250-B** | P2: Daemon function calling 预留 | [有缓冲] 在 Daemon 中增加 tool_use 消息拦截架构: ToolCall 接口 + taskClaimTool/taskUpdateTool/messageSendTool；仅 API-based LLM 启用 | rd1 | P2 | 16 | Tool call 拦截架构就绪；与 CLI 路径不冲突 |
| **SOLO-252-QA** | v1.3 全功能回归测试 | 全功能 E2E: Solo CLI、Agent 自主认领闭环、A→B 协作+防死循环、DM 任务、消息搜索、文件上传、电脑管理、团队管理、双通道消息流、持久化会话 token 刷新；现有测试全部通过 | qa1 | P0 | 8 | 全部 P0/P1 功能通过；无 blocking/critical Bug；回归测试 100% 通过 |

**并行策略**:
- fe2: SOLO-251-F (Day 1-2)
- fe1: SOLO-258-F (Day 1)
- rd1: SOLO-250-B (Day 1-5，仅缓冲充足时)
- qa1: SOLO-252-QA (Day 1-5 全天)
- rd2: 协助 QA、修 Bug
- 周总工时: rd1(0-16h) + fe1(1h) + fe2(4h) + qa1(8h) = 13-29h

**Sprint 4' 总计**: 23-39h

---

## 7. 汇总对比

### v3.0 新排期 vs 原 v2.1 排期

| Sprint | 原周次 | 原工时 | 新周次 | 新工时 | 变化 | 关键差异 |
|--------|--------|--------|--------|--------|------|----------|
| Phase 5 (Sprint 1) | W1-2 | 65h | W1-2 (已完成) | ~65h | 持平 | 实际交付远超原 Sprint 1 范围 |
| Sprint 2 | W3-4 | 70h | W1-2 | 37h | **-47%** | 6 任务废弃 + 后端全部已实现 |
| Sprint 3 | W5-6 | 69h | W3-4 | 28h | **-59%** | A→B 协作+防死循环+搜索后端已实现 |
| Sprint 4 | W7-8 | 60-76h | W5-6 | 23-39h | **-49%** | Computer API+Attachment API 已实现 |
| **总计** | **8 周** | **264-280h** | **6+2 周** | **88-104h (剩余)** | **-60%** | 16 个任务废弃或提前实现；关键路径从后端+前端双线收敛为前端单线 |

> 注: Phase 5 2 周已完成。新 Sprint 2'-4' 共 6 周。总计 8 周与原计划持平，但实际工时减少 60%。

### 按优先级分类 (Sprint 2'-4')

| 优先级 | 任务数 | 总工时 | 占比 |
|--------|-------|--------|------|
| P0 — 阻塞发布 | 10 | 42h | 43% |
| P1 — 功能补齐 | 14 | 36h | 36% |
| P2 — 锦上添花 | 3 | 21h | 21% |

### 按角色分类 (Sprint 2'-4')

| 角色 | 任务数 | 总工时 | 周均负荷 |
|------|-------|--------|---------|
| rd1 | 6 | ~18h (34h 含 P2) | 3.0h/周 |
| rd2 | 4 | ~12h | 2.0h/周 |
| fe1 | 10 | ~24h | 4.0h/周 |
| fe2 | 10 | ~23h | 3.8h/周 |
| arc | 1 | 2h | Sprint 2' W1 一次性 |
| qa1 | 3 | 16h | 按 Sprint 末 2 天 |
| **总计** | **34** | **~88h (104h 含 P2)** | |

---

## 8. 关键路径与依赖

### 关键路径（最长依赖链）

```
Phase 5 (已完成)
    │
    ▼
Sprint 2' W1:  测试修复(rd2) → Token刷新(rd1) → Proxy补齐(rd1)
                ThreadPanel统一(fe1) → 元数据增强(fe2)
                委派决策树(arc)
    │
    ▼
Sprint 2' W2:  父子视觉(fe2) → CLI E2E(rd1) → QA集成(qa1)
    │
    ▼
Sprint 3' W3:  DM TaskBoard(fe1) → 消息转换(fe1)
                搜索UI验证(fe2) + 团队管理(fe2)
    │
    ▼
Sprint 3' W4:  团队联调(fe1) + 结构可视化(fe2)
                图片缩略图(rd1) + 心跳验证(rd1) → QA集成(qa1)
    │
    ▼
Sprint 4' W5:  电脑UI验证(fe2) + 上传UI验证(fe1)
    │
    ▼
Sprint 4' W6:  P2打磨(fe2) → 全功能回归(qa1) → v1.3 发布
```

### 新的最长路径

```
Token刷新(rd1, 6h) → CLI E2E(rd1, 4h) → QA Sprint 2'(qa1, 4h) 
  → 图片缩略图(rd1, 3h) → QA Sprint 3'(qa1, 4h)
  → 全功能回归(qa1, 8h)
```

总关键路径: 6h + 4h + 4h + 3h + 4h + 8h = 29h。在 6 周内绰绰有余。

### 依赖关系图

```
                    ┌─────────────────────┐
                    │   Phase 5 已完成     │
                    │ CLI / SP / 双通道 /  │
                    │ 搜索 / 协作 / API    │
                    │ 前端页面 (大部分)    │
                    └──────────┬──────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
    ┌────▼──────┐       ┌──────▼──────┐      ┌──────▼──────┐
    │ Sprint 2' │       │  Sprint 3'  │      │  Sprint 4'  │
    │ W1-2      │       │  W3-4       │      │  W5-6       │
    │           │       │             │      │             │
    │ 测试修复  │──────►│ DM UI+搜索  │─────►│ 电脑+上传   │
    │ Token刷新 │       │ 团队管理    │      │ 全回归+P2   │
    │ ThreadPanel│      │ 图片缩略图  │      │             │
    │ CLI E2E   │       │ 心跳验证    │      │             │
    └───────────┘       └─────────────┘      └─────────────┘
```

**关键观察**: 后端不再是瓶颈。所有核心后端能力已在 Phase 5 交付。前端页面大部分已存在，Sprint 2'-4' 重点是验证、修补和 QA。

---

## 9. 风险登记册

### 已消除的风险 (Phase 5 解决)

| 原编号 | 风险 | 消除原因 |
|--------|------|----------|
| R1 | Agent 不按预期使用 solo CLI | CLI 12 命令已验证可用；System Prompt 对 CLI 使用强调明确；fallback regex 已废弃 |
| R4 | 指令文件体积过大 | CLAUDE.md/AGENTS.md/GEMINI.md 已移除；system-prompt.md 是唯一指令来源 |
| R8 | rd1/rd2 CLI 合并冲突 | CLI 已在 Phase 5 完成，不再多人同时开发 |

### 当前风险

| # | 风险描述 | 概率 | 影响 | 缓解措施 | 应急预案 | 负责人 |
|---|---------|------|------|----------|----------|--------|
| **R10** | **持久化 session JWT token >15min 过期，Agent 长任务 CLI 命令 401** | 高 | 高 | **Sprint 2' SOLO-254-B 作为 P0 最高优先级**；daemon proxy 统一 refresh token 续期；所有代理命令强制走 proxy | 降级为短任务模式；或 daemon 层自动 refresh token | rd1 |
| R2 | 非 proxy CLI 命令 token 过期 | 中 | 中 | SOLO-255-B 只读命令走 proxy；非 proxy 命令增加明确错误提示 | CLI 支持 --token 参数手动传入 | rd1 |
| R3 | 多 Agent 竞争认领 DB 死锁 | 低 | 中 | FOR UPDATE 已实现；事务持锁时间极短 | 优化为 SKIP LOCKED | rd1 |
| R5 | PostgreSQL FTS 中文搜索效果差 | 中 | 中 | simple 字典基本可用；观察用户反馈 | 降级 ILIKE 模糊搜索；或 zhparser 扩展 | rd1+rd2 |
| R6 | Agent-to-Agent 死循环 | 低 | 低 | maxAgentChainDepth=3 已实现 + 重复检测 | 增加 Agent 协作白名单 | rd1 |
| R7 | 文件上传磁盘耗尽 | 低 | 高 | 50MB 单文件硬限制 | 管理员手动清理 + >30 天未引用自动清理 | rd2 |
| R11 | System Prompt 350 行导致响应质量下降 | 中 | 中 | 监控 token 使用量；关键段落标注 | 提供精简版 SP (~150 行) 作为配置选项 | arc |
| R12 | 双通道架构消息丢失/重复触发 | 低 | 中 | agent_chain 去重 + message ID 幂等 | 触发去重表 processed_message_ids | rd1 |
| R13 | ThreadPanel 改造引入 Kanban 回归 | 中 | 中 | 渐进式改造；保留 Kanban 交互 | 回退到旧 TaskDetailPanel fallback | fe1+fe2 |

### 风险汇总

| 级别 | 数量 | 风险 |
|------|------|------|
| 高×高 (Critical) | 1 | R10: token 过期 — Sprint 2' W1 解决 |
| 中×高 | 0 | — |
| 中×中 (Medium) | 4 | R2: CLI 认证, R5: 中文搜索, R11: SP 长度, R13: ThreadPanel 回归 |
| 低×中 (Low) | 2 | R3: 竞争认领, R12: 消息重复 |
| 低×低 (Monitor) | 2 | R6: 死循环, R7: 磁盘耗尽 |

---

## 10. 团队角色与分工

| 角色 | 代号 | 技能栈 | Sprint 2'-4' 职责 | 工时 |
|------|------|--------|-------------------|------|
| 后端开发 1 | rd1 | Go + Chi + pgx + Daemon | Token 刷新 + Proxy 补齐 + CLI E2E + 图片缩略图 + 心跳验证 | ~18h |
| 后端开发 2 | rd2 | Go + Chi + pgx + Daemon | 测试修复 + CLI --parent + DM API 验证 + 消息附件完善 | ~12h |
| 前端开发 1 | fe1 | Next.js 16 + Tailwind + shadcn/ui | ThreadPanel 统一 + 消息样式 + DM UI + 频道搜索 + 团队联调 + 上传 UI + 路由清理 | ~24h |
| 前端开发 2 | fe2 | Next.js 16 + Tailwind + shadcn/ui | 元数据增强 + 父子视觉 + 评论清理 + 搜索 UI + 团队管理 + 电脑 UI + P2 优化 | ~23h |
| 架构师 | arc | System Prompt 设计 | 委派决策树追加 | 2h (一次性) |
| 测试 | qa1 | API 测试 + E2E + 验收 | Sprint 2'/3'/4' 集成验证 + 全功能回归 | 16h |

---

> 文档版本: 3.0
> 日期: 2026-05-30
> 评估方法: pm1 + tpm 双视角独立评估 → 代码库验证 → 取并集/保守方案
> 变更记录:
> - v1.0: 初始版本 (rd1+fe1 单点配置)
> - v2.0: 均匀分配 (rd1/rd2 50/50, fe1/fe2 50/50)
> - v2.1: 加入 5 个任务委派模式任务
> - v3.0: Phase 5 收尾后全面重评估 — 16 个任务废弃或提前实现；剩余 88-104h 重排为 3 Sprint (6 周)；关键路径从后端+前端双线收敛为前端单线

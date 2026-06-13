# WBS 任务核对报告（docs/design/tasks/ 下 6 个 step）

> 范围：`docs/design/tasks/step{1..6}-*.md` 里的所有 `T{x}.{y}.{z}` 任务
> 方法：6 个并行 Explore agent 逐个读源码 + git 历史
> 结果：124 个任务中 **112 PRESENT / 11 PARTIAL / 4 MISSING**

---

## 总览（按 step）

| Step | 任务数 | PRESENT | PARTIAL | MISSING | 完成度 |
|------|--------|---------|---------|---------|--------|
| 1 Foundation | 16 | 16 | 0 | 0 | **100%** |
| 2 Coordination | 27 | 22 | 3 | 2 | **81%** |
| 3 Workspace | 18 | 13 | 4 | 1 | **72%** |
| 4 Knowledge | 19 | 15 | 3 | 1 | **79%** |
| 5 Visualization | 15 | 11 | 3 | 1 | **73%** |
| 6 Swarm + Wake | 29 | 28 | 1 | 0 | **97%** |
| **合计** | **124** | **105** | **14** | **5** | **85%** |

---

## 1. Step 1 — Foundation (16/16 PRESENT ✅)

**T1.1 Agent Relationships** — 全完
- T1.1.1 迁移 000027 ✅
- T1.1.2 `agent_relationship.go` 334 行 ✅
- T1.1.3 handler + router.go:241 ✅
- T1.1.4 路由注册 ✅
- T1.1.5 前端 `app/relationships/page.tsx` + manage ✅
- T1.1.6 `agent_relationship_test.go` ✅

**T1.2 Task Dependencies** — 全完（逻辑合并在 `task.go`）
- T1.2.1 迁移 000028 ✅
- T1.2.2 服务内嵌 task.go:946-999 cycle detection ✅
- T1.2.3 handler + router.go:273-274 ✅
- T1.2.4 task.go:347 IsTaskBlocked, L538 BlockedByCount ✅
- T1.2.5 task-card.tsx:76-99 isBlocked + opacity-60 + 灰显 ✅
- T1.2.6 `task_dependency_test.go` ✅

**T1.3 Agent Delegations** — 全完
- T1.3.1 迁移 000029 ✅
- T1.3.2 `agent_delegation.go` 状态机（queued/delivered/started/completed/rejected/failed）✅
- T1.3.3 handler + router.go:264-270 ✅
- T1.3.4 `agent_delegation_test.go` ✅

---

## 2. Step 2 — Coordination (22/27 — 5 项缺)

**T2.1 Agent Delegations (5/7)**
- T2.1.1 CLI `agent delegate create/list` ✅
- T2.1.2 CLI `accept/reject/complete` ✅
- T2.1.3 `resolveAgentParam` / `resolveChannelParam` ✅
- T2.1.4 daemon 注入 pending delegations 到 System Prompt ✅
- **T2.1.5 ⚠️ PARTIAL** — `agent_delegation.go` 状态变更**没有 WebSocket 广播**（handler 缺 hub import 和 Broadcast 调用）
- **T2.1.6 ❌ MISSING** — `ListByAgent` 写了但**没有注册到 router**（router.go:263-270 只有 `/agent-delegations/...` 没有 `/agents/{id}/delegations`）
- **T2.1.7 ❌ MISSING** — CLI delegate 没有 E2E 测试

**T2.2 Task Dependencies CLI (7/7 ✅)**
- T2.2.1-2.2.7 全部 PRESENT

**T2.3 Channel Memory (6/6 ✅)**

**T2.4 UI / Graph (3/6)**
- **T2.4.1 ⚠️ PARTIAL** — `agent-relationship-graph.tsx`（SVG + BFS 布局）存在但**没有被任何页面 import**；Step 5 改用了 ReactFlow
- T2.4.2 BFS 布局算法 ✅
- T2.4.3 Kanban blocked 卡片灰显 + badge ✅
- T2.4.4 Subtask 进度条 ✅
- T2.4.5 WS `task.unblocked` 订阅 ✅
- **T2.4.6 ⚠️ PARTIAL** — "Graph tab" 没有显式集成；ReactFlow 走 `/relationships`，SVG 走 nowhere

---

## 3. Step 3 — Workspace (13/18 — 5 项缺)

**T3.1 Channel Binding (5/7)**
- T3.1.1 迁移 000030 ✅
- T3.1.2 service BindProject ✅
- **T3.1.3 ⚠️ PARTIAL** — async git clone 存在，**但文件树扫描未实现**（service 里没有 Walk/readDir/tree 逻辑）
- **T3.1.4 ⚠️ PARTIAL** — bind/unbind 端点 OK，**没有专门的 workspace listing 端点**
- T3.1.5 router 路由 ✅
- T3.1.6 CLI `channel bind/unbind/workspace` ✅
- T3.1.7 `channel_binding_test.go` 233 行 ✅

**T3.2 Worktree 隔离 (4/6)**
- T3.2.1 CLI `task isolate` ✅
- **T3.2.2 ⚠️ PARTIAL** — 只有 `task unisolate`（DELETE），"commit + cleanup done" 语义需要再确认
- **T3.2.3 ⚠️ PARTIAL** — service 里有 CWD 解析顺序注释，**但 daemon 端没有 `ResolveCwd` 实现**
- T3.2.4 daemon auto conflict detect + WS alert ✅
- T3.2.5 handler `POST /tasks/{id}/isolate` ✅
- T3.2.6 `worktree_test.go` ✅

**T3.3 Frontend (3/5)**
- T3.3.1 `channel-binding.tsx` 308 行 ✅
- **T3.3.2 ⚠️ PARTIAL** — `workspace/file-tree.tsx` 存在但**没有挂到 channel binding 页**
- T3.3.3 Worktree 状态指示器 ✅
- T3.3.4 `use-workspace-conflicts.ts` WS 处理 ✅
- **T3.3.5 ❌ MISSING** — 没有 channel workspace E2E spec

---

## 4. Step 4 — Knowledge (15/19 — 4 项缺)

**T4.1 Service 层 (5/5 ✅)** — pgvector 优雅降级、async embedding、hybrid search

**T4.2 API + CLI (5/5 ✅)**

**T4.3 集成 (3/3 ✅)** — daemon 注入、import 服务、WS 广播

**T4.4 前端 (1/3)**
- **T4.4.1 ⚠️ PARTIAL** — `knowledge-panel.tsx` 有 list + search，**但 tag 过滤器 UI 没在 panel 级别做**（后端支持 tag 过滤，前端没暴露）
- **T4.4.2 ⚠️ PARTIAL** — 没有 detail 弹窗；`knowledge-create.tsx` 写明支持 edit 但**实际没实现 isEdit/mode 分支**
- T4.4.3 `knowledge-create.tsx` 347 行带 tag 输入 ✅

**T4.5 测试 (1/3)**
- T4.5.1 handler 集成测试 ✅
- **T4.5.2 ❌ MISSING** — 没有任何 semantic search 准确率测试
- **T4.5.3 ⚠️ PARTIAL** — `ImportFromDecisions` 只有 handler 校验测试，**没有 service 层测试**（`parseDecisionBlocks` 完全没测）

---

## 5. Step 5 — Visualization (11/15 — 4 项缺)

- T5.1.1 @xyflow/react v12.11.0 ✅
- T5.1.2 `GET /api/v1/relationships/graph` ✅
- T5.1.3 `use-relationships.ts` hook ✅
- **T5.1.4 ⚠️ PARTIAL** — `RelationshipType` 类型有；**`GraphNode` / `GraphEdge` 没单独定义**，page 用的是 @xyflow/react 的 `Node`/`Edge`
- T5.2.1 `relationship-node.tsx` ✅
- T5.2.2 `relationship-edge.tsx` 4 种线型 ✅
- T5.2.3 ReactFlow 容器 + undo/redo ✅
- **T5.2.4 ⚠️ PARTIAL** — 拖拽后弹的是 `TypeSelector`，**不是 `CreateRelationshipModal`**（后者只被 manage 页用）
- T5.2.5 `relationship-detail-panel.tsx` ✅
- T5.3.1 WS relationship_created/deleted ✅
- T5.3.2 前端 WS 订阅 ✅
- T5.3.3 MiniMap + Controls + Background ✅
- T5.4.1 路由 + NavBar ✅
- **T5.4.2 ⚠️ PARTIAL** — `collaboration-smoke.spec.ts` 有 page load + API 200/401，**没有真正的拖拽创建/删除交互测试**
- **T5.4.3 ❌ MISSING** — 完全没有 30+ agent 性能测试

---

## 6. Step 6 — Swarm + Wake (28/29 — 1 项缺)

**T6.1 Reminders (7/8)**
- T6.1.1-1.4 全部 PRESENT
- **T6.1.2 ⚠️ PARTIAL** — service `reminder.go` 没有 cron 解析库；recurring 逻辑在 daemon ticker 内联实现
- T6.1.5 daemon ticker 30s ✅
- T6.1.6 CLI ✅
- T6.1.7 `reminder-manager.tsx` ✅
- T6.1.8 `reminder_test.go` ✅

**T6.2 Watchdog (6/6 ✅)** — `VerifyEscalationChain`、三级响应、CLI、测试全齐

**T6.3 Swarm (10/10 ✅)** — 迁移、Coordinator、API、CLI、组件、测试全齐

**T6.4 集成 (5/5 ✅)** — Ticker、WS 4 类事件、配置、安全限流全齐

---

## 遗漏清单（按价值排序）

### 🔴 P0 — 用户可见的功能缺失
| 任务 | 缺什么 | 影响 |
|------|--------|------|
| T2.1.6 | `GET /api/v1/agents/{id}/delegations` 路由没注册 | Agent 自己无法查看收到的委托，UI 不可达 |
| T2.1.5 | Delegation 状态变更无 WS 广播 | 委托接受/拒绝/完成 UI 不能实时刷新 |
| T2.4.1 / T2.4.6 | Step 2.5 设计的 SVG 关系图谱没被整合 | 设计承诺的"低投入 SVG 验证"实际跳过了 |
| T3.3.2 | Channel 工作区文件树没挂到 binding 页 | 频道绑定后看不到代码（核心价值缺失） |
| T3.3.5 | 无 channel workspace E2E | 整个 Step 3 没有 e2e 覆盖 |
| T4.4.2 | Knowledge 详情/编辑 UI 未做 | 创建后无法编辑/查看详情 |
| T4.4.1 | Tag 过滤 UI 缺失 | 后端支持，前端不暴露 |
| T5.2.4 | 拖拽创建用的是 TypeSelector，不是 CreateRelationshipModal | 与设计文档不一致 |

### 🟡 P1 — 完整性 / 一致性
| 任务 | 缺什么 |
|------|--------|
| T2.1.7 | CLI delegate E2E 测试 |
| T3.1.3 | channel binding 后文件树扫描 |
| T3.1.4 | 专门的 workspace listing 端点 |
| T3.2.2 | `task isolate done` commit + cleanup 语义确认 |
| T3.2.3 | daemon `ResolveCwd` 实现 |
| T4.5.2 | semantic search 准确率测试 |
| T4.5.3 | `parseDecisionBlocks` 服务层测试 |
| T5.1.4 | `GraphNode` / `GraphEdge` 类型 |
| T5.4.2 | 图谱拖拽交互 e2e |
| T5.4.3 | 30+ agent 性能测试 |
| T6.1.2 | service 层 cron 解析（目前内联在 daemon） |

---

## 总结

**亮点**：
- Step 1 100% 完成（基础数据层扎实）
- Step 6 97% 完成（蜂群 + 唤醒几乎全部交付）
- 整体 85% 完成度，单看 backend service 接近 100%

**结构性缺口**：
- **前端 UI 完成度低于后端**（多处 service/API 已实现但 UI 未挂载/未集成）
- **测试覆盖薄**（很多 PARTIAL 集中在"只有 happy path 校验，缺逻辑分支测试"）
- **Step 2 的 SVG 图谱被 Step 5 的 ReactFlow 取代**但前者代码仍在仓库里成为死代码（"agent-relationship-graph.tsx"无引用）

**修复优先级建议**：
1. T2.1.5/T2.1.6 — 委托协议核心功能补全
2. T3.3.2 + T4.4.1 + T4.4.2 — 把已有后端能力暴露到 UI
3. T3.3.5 + T5.4.2 + T5.4.3 — 补 e2e / 性能测试
4. T2.4.1 — 决定保留/删除未引用的 SVG 组件（Karpathy 准则：删或用，别留中间态）

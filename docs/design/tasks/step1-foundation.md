# Step 1 — Foundation: 关系与依赖建模

> 任务分解 | 2-3 weeks | 2026-06-13
> 目标：Agent Relationships 表 + Task Dependencies 表 + Delegations 表，全部含 API

---

## 概览

| 维度 | 数值 |
|------|------|
| 总任务数 | 14 |
| 预估总工时 | 56 h |
| 建议团队规模 | 3 dev + 1 QA |
| 建议迭代 | Sprint 1 (1 周): 1.1 全部 + 1.2 DB/Service + 1.3 DB/Service; Sprint 2 (1 周): 1.2 Handler/Frontend + 1.3 Handler + 全部测试 |

---

## WBS 总览

```
Step 1 — Foundation (56h)
│
├── 1.1 Agent Relationships (22h)
│   ├── T1.1.1  Migration: agent_relationships 表 + 索引        [S, 2h, backend]
│   ├── T1.1.2  Service: AgentRelationshipService + 循环检测     [M, 6h, backend]
│   ├── T1.1.3  Handler: REST API 端点                          [M, 5h, backend]
│   ├── T1.1.4  Router: 注册路由 + API 路径设计                  [S, 2h, backend]
│   ├── T1.1.5  Frontend: Agent 关系管理页                       [M, 5h, frontend]
│   └── T1.1.6  Test: 集成测试                                   [S, 2h, QA]
│
├── 1.2 Task Dependencies (22h)
│   ├── T1.2.1  Migration: task_dependencies 表                  [S, 1h, backend]
│   ├── T1.2.2  Service: TaskDependencyService + DAG 检测        [M, 6h, backend]
│   ├── T1.2.3  Handler: 依赖 CRUD 端点                          [M, 4h, backend]
│   ├── T1.2.4  Extend TaskService: 阻塞检查 + 完成通知          [M, 4h, backend]
│   ├── T1.2.5  Frontend: Kanban 卡片依赖展示                    [M, 5h, frontend]
│   └── T1.2.6  Test: 集成测试                                   [S, 2h, QA]
│
└── 1.3 Delegations (12h)
    ├── T1.3.1  Migration: agent_delegations 表                  [S, 1h, backend]
    ├── T1.3.2  Service: DelegationService + 状态机              [M, 5h, backend]
    ├── T1.3.3  Handler: 委托 CRUD + accept/reject 端点          [M, 4h, backend]
    └── T1.3.4  Test: 集成测试                                   [S, 2h, QA]
```

---

## 依赖关系图

```
T1.1.1 ──→ T1.1.2 ──→ T1.1.3 ──→ T1.1.4 ──→ T1.1.5
                                        │
                                        └──→ T1.1.6

T1.2.1 ──→ T1.2.2 ──→ T1.2.3 ──→ T1.2.4
                 │         │
                 │         └──→ T1.2.5
                 └──→ T1.2.6 (can start after T1.2.2)

T1.3.1 ──→ T1.3.2 ──→ T1.3.3 ──→ T1.3.4

Tracks 1.1 / 1.2 / 1.3 are fully independent — no cross-track dependencies.
```

**关键路径** (longest chain): T1.1.1 → T1.1.2 → T1.1.3 → T1.1.4 → T1.1.5 (18h sequential)

---

## 执行顺序建议

### Sprint 1 — Week 1 (Day 1-5)

| 天 | Backend Dev A (Track 1.1) | Backend Dev B (Track 1.2) | Backend Dev C (Track 1.3) |
|----|--------------------------|--------------------------|--------------------------|
| D1 | T1.1.1 (migration) | T1.2.1 (migration) | T1.3.1 (migration) |
| D2 | T1.1.2 (service 前半: CRUD) | T1.2.2 (service 前半: CRUD) | T1.3.2 (service: 状态机) |
| D3 | T1.1.2 (service 后半: 循环检测) | T1.2.2 (service 后半: DAG 检测) | T1.3.2 (继续) |
| D4 | T1.1.3 (handler) | T1.2.3 (handler) | T1.3.3 (handler) |
| D5 | T1.1.4 (router) + T1.1.3 收尾 | T1.2.4 (extend TaskService) | T1.3.3 (收尾) |

### Sprint 2 — Week 2 (Day 6-10)

| 天 | Frontend Dev | Backend Dev A | QA Engineer |
|----|-------------|--------------|-------------|
| D6 | T1.1.5 (Agent 关系页) | T1.2.4 收尾 + review | T1.1.6 (Agent Relationships 测试) |
| D7 | T1.1.5 (继续) | 协助 frontend 联调 | T1.2.6 (Task Dependencies 测试) |
| D8 | T1.2.5 (Kanban 依赖展示) | 协助 frontend 联调 | T1.3.4 (Delegations 测试) |
| D9 | T1.2.5 (继续) + 联调 | bug fix + 联调 | 补充测试用例 |
| D10 | 缓冲 / code review / 文档 | 缓冲 / code review | 回归测试 |

---

## 详细任务分解

---

### T1.1.1 — Migration: agent_relationships 表

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.1 |
| **预估** | S (2h) |
| **角色** | 后端工程师 |
| **依赖** | 无 |
| **文件** | `migrations/000027_create_agent_relationships.up.sql` / `.down.sql` |

**描述**:
创建 `agent_relationships` 表及两个 partial unique index。

**SQL DDL** (from roadmap):
```sql
CREATE TABLE agent_relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id UUID NOT NULL REFERENCES agents(id),
    to_agent_id   UUID NOT NULL REFERENCES agents(id),
    rel_type      VARCHAR(20) NOT NULL,
    channel_id    UUID REFERENCES channels(id),
    weight        REAL NOT NULL DEFAULT 1.0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_rel_global
  ON agent_relationships(from_agent_id, to_agent_id, rel_type)
  WHERE rel_type IN ('reports_to', 'escalates_to');

CREATE UNIQUE INDEX idx_rel_channel
  ON agent_relationships(from_agent_id, to_agent_id, rel_type, channel_id)
  WHERE rel_type IN ('delegates_to', 'collaborates_with');
```

**验收标准**:
- [ ] `migrate up` 成功创建表 + 2 个 partial unique index
- [ ] `migrate down` 成功删除
- [ ] 尝试插入同频道内重复 `delegates_to` 报 unique violation
- [ ] 尝试插入全局重复 `reports_to` 报 unique violation
- [ ] 不同频道内相同 `delegates_to` 不冲突

---

### T1.1.2 — Service: AgentRelationshipService

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.2 |
| **预估** | M (6h) |
| **角色** | 后端工程师 |
| **依赖** | T1.1.1 |
| **文件** | `internal/server/service/agent_relationship.go` |

**描述**:
创建 `AgentRelationshipService`，实现完整 CRUD + 循环检测。参照现有 `TaskService` 的模式（struct + pgxpool.Pool + 方法）。

**核心方法**:
```
CreateRelationship(ctx, fromAgentID, toAgentID, relType, channelID, weight) (*AgentRelationship, error)
  → 校验 rel_type 合法性 (reports_to|delegates_to|collaborates_with|escalates_to)
  → 校验 from_agent != to_agent
  → reports_to/escalates_to: channel_id 必须为 NULL
  → delegates_to/collaborates_with: channel_id 必填
  → reports_to: 调用 detectCycle 检查不会成环
  → INSERT + RETURNING

GetRelationship(ctx, id) (*AgentRelationship, error)
ListRelationships(ctx, filter) ([]AgentRelationship, error)
  → 支持 filter: from_agent_id, to_agent_id, rel_type, channel_id
UpdateRelationship(ctx, id, weight) (*AgentRelationship, error)
DeleteRelationship(ctx, id) error

detectCycle(ctx, fromAgentID, toAgentID) (bool, error)
  → BFS/DFS 沿 reports_to 边检查: 如果 toAgentID 沿 reports_to 链最终到达 fromAgentID → 成环，拒绝
```

**验收标准**:
- [ ] CRUD 全部可用，返回正确 JSON 结构
- [ ] `reports_to` 成环检测生效: A→B 已存在，创建 B→A 被拒绝
- [ ] `reports_to` 间接成环检测: A→B→C 已存在，创建 C→A 被拒绝
- [ ] `delegates_to` 无 channel_id 时创建被拒绝
- [ ] 全局关系重复创建报 unique violation
- [ ] 日志级别与现有 TaskService 一致 (slog.Info/Error)

---

### T1.1.3 — Handler: Agent Relationships REST API

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.3 |
| **预估** | M (5h) |
| **角色** | 后端工程师 |
| **依赖** | T1.1.2 |
| **文件** | `internal/server/handler/agent_relationship.go` |

**描述**:
创建 `AgentRelationshipHandler`，实现 REST 端点。参照 `TaskHandler` 的模式（handler struct + HTTP 方法 + JSON 编解码）。

**端点设计**:
```
POST   /api/v1/agent-relationships              → Create
GET    /api/v1/agent-relationships              → List (query: from_agent_id, to_agent_id, rel_type, channel_id)
GET    /api/v1/agent-relationships/{id}         → Get
PATCH  /api/v1/agent-relationships/{id}         → Update
DELETE /api/v1/agent-relationships/{id}         → Delete
GET    /api/v1/agent-relationships/check-cycle  → CheckCycle (query: from, to, rel_type)
```

**验收标准**:
- [ ] 所有端点返回符合现有 API 风格的 JSON（snake_case，时间 RFC3339）
- [ ] 创建成功返回 201
- [ ] 参数校验失败返回 400
- [ ] 循环检测失败返回 409 Conflict
- [ ] 资源不存在返回 404
- [ ] 需认证（复用现有 auth middleware）
- [ ] Request/Response struct 与 Service 层 struct 分离（handler 层有独立的 CreateAgentRelationshipRequest 等）

---

### T1.1.4 — Router: 注册路由

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.4 |
| **预估** | S (2h) |
| **角色** | 后端工程师 |
| **依赖** | T1.1.3 |
| **文件** | `internal/server/router.go` |

**描述**:
在现有 Chi router 的 protected routes group 中注册 Agent Relationships 路由。初始化 `AgentRelationshipHandler` 并注入 pool。

**变更点**:
```go
// 在 NewRouter 中:
agentRelSvc := service.NewAgentRelationshipService(pool)
agentRelHandler := handler.NewAgentRelationshipHandler(pool, agentRelSvc)

// protected routes group 内:
r.Route("/api/v1/agent-relationships", func(r chi.Router) {
    r.Get("/", agentRelHandler.List)
    r.Post("/", agentRelHandler.Create)
    r.Get("/check-cycle", agentRelHandler.CheckCycle)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", agentRelHandler.Get)
        r.Patch("/", agentRelHandler.Update)
        r.Delete("/", agentRelHandler.Delete)
    })
})
```

**验收标准**:
- [ ] `go build` 通过
- [ ] `GET /api/v1/agent-relationships` 返回 200（需带 JWT token）
- [ ] 路由不与现有路由冲突

---

### T1.1.5 — Frontend: Agent 关系管理页

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.5 |
| **预估** | M (5h) |
| **角色** | 前端工程师 |
| **依赖** | T1.1.3 (API 可用) |
| **文件** | `frontend/src/` (待确认具体路径) |

**描述**:
在 Agent 详情页或独立页面增加"关系"Tab/区域，支持查看和管理 Agent 间关系。

**功能清单**:
1. 关系列表（表格）：from_agent → to_agent | rel_type | channel | weight | 操作
2. 筛选：按 rel_type、channel 过滤
3. 创建关系对话框：
   - from_agent 下拉（当前 Agent 预填）
   - to_agent 下拉
   - rel_type 下拉（4 种，带中文说明）
   - channel 下拉（delegates_to/collaborates_with 时必填）
   - 提交前调用 check-cycle API，如有循环弹出警告
4. 删除确认对话框
5. 关系类型用不同颜色标签区分

**API 调用**:
- `GET /api/v1/agent-relationships?from_agent_id=X` — 获取某 Agent 的所有关系
- `POST /api/v1/agent-relationships` — 创建
- `DELETE /api/v1/agent-relationships/{id}` — 删除
- `GET /api/v1/agent-relationships/check-cycle?from=X&to=Y&rel_type=reports_to` — 循环检测

**验收标准**:
- [ ] 关系列表正确展示（含 agent name，而非仅 UUID）
- [ ] 创建关系时循环检测生效，弹出警告
- [ ] 创建成功/失败有 toast 提示
- [ ] 删除有二次确认
- [ ] 4 种 rel_type 用不同颜色视觉区分
- [ ] delegates_to/collaborates_with 必选 channel，reports_to/escalates_to 不可选 channel
- [ ] UI 符合 Solo 现有设计风格（Tailwind + 现有组件）

---

### T1.1.6 — Test: Agent Relationships 集成测试

| 属性 | 内容 |
|------|------|
| **ID** | T1.1.6 |
| **预估** | S (2h) |
| **角色** | QA工程师 |
| **依赖** | T1.1.3 (API 可用) |
| **文件** | `internal/server/handler/agent_relationship_test.go` |

**描述**:
编写 HTTP 级别的集成测试。参照现有 `task_test.go` 的模式。

**测试用例**:
1. 创建有效 `reports_to` 关系 → 201
2. 创建有效 `delegates_to` 关系（带 channel）→ 201
3. 创建 `reports_to` 带 channel_id → 400（全局关系不需要 channel）
4. 创建 `delegates_to` 无 channel_id → 400
5. 创建循环 `reports_to` → 409
6. 创建重复全局 `reports_to` → 409 unique violation
7. 创建相同 `delegates_to` 在不同 channel → 200（允许）
8. 列出某 Agent 的关系 → 200，数量正确
9. 删除关系 → 204
10. 更新 weight → 200

**验收标准**:
- [ ] 10 个用例全部通过
- [ ] 测试使用真实数据库（非 mock），参照现有测试模式 setUp/tearDown

---

### T1.2.1 — Migration: task_dependencies 表

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.1 |
| **预估** | S (1h) |
| **角色** | 后端工程师 |
| **依赖** | 无 |
| **文件** | `migrations/000028_create_task_dependencies.up.sql` / `.down.sql` |

**描述**:
创建 `task_dependencies` 表。

**SQL DDL** (from roadmap):
```sql
CREATE TABLE task_dependencies (
    blocker_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(blocker_task_id, blocked_task_id),
    CHECK(blocker_task_id != blocked_task_id)
);
```

**验收标准**:
- [ ] `migrate up` 成功创建
- [ ] 自引用 (blocker == blocked) 被 CHECK 拒绝
- [ ] 删除 task 时关联依赖自动 CASCADE 删除

---

### T1.2.2 — Service: TaskDependencyService + DAG 检测

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.2 |
| **预估** | M (6h) |
| **角色** | 后端工程师 |
| **依赖** | T1.2.1 |
| **文件** | `internal/server/service/task_dependency.go` |

**描述**:
创建 `TaskDependencyService`，管理任务间阻塞关系。DAG 检测防成环。

**核心方法**:
```
AddDependency(ctx, blockerTaskID, blockedTaskID) (*TaskDependency, error)
  → 校验 blocker != blocked
  → 校验两个 task 都存在
  → DAG 检测: BFS 检查 blocked → blocker 方向是否会成环
  → INSERT

RemoveDependency(ctx, blockerTaskID, blockedTaskID) error
  → DELETE by PK

GetBlockers(ctx, taskID) ([]TaskDependency, error)
  → SELECT blocker_task_id WHERE blocked_task_id = $1
  → 联查 task title + task_number

GetBlockedBy(ctx, taskID) ([]TaskDependency, error)
  → SELECT blocked_task_id WHERE blocker_task_id = $1

IsBlocked(ctx, taskID) (bool, error)
  → 检查是否有未完成的 blocker（blocker_task.status NOT IN ('done','closed')）
  → 只返回 true/false，用于 claim 前检查

detectDagCycle(ctx, blockerID, blockedID) (bool, error)
  → BFS: 从 blockedID 出发沿 blocker_task_id 方向走，看能否到达 blockerID
  → 如果能到达 → 成环，拒绝
```

**验收标准**:
- [ ] A blocks B, B blocks C → 创建 C blocks A 被拒绝（DAG 循环检测）
- [ ] 重复添加同一依赖 → PK conflict
- [ ] GetBlockers 返回 blocker 的 task_number + title
- [ ] IsBlocked: blocker 为 done 时返回 false；blocker 为 in_progress 时返回 true
- [ ] 删除不存在的依赖 → 正常返回（非错误）

---

### T1.2.3 — Handler: Task Dependencies REST API

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.3 |
| **预估** | M (4h) |
| **角色** | 后端工程师 |
| **依赖** | T1.2.2 |
| **文件** | `internal/server/handler/task_dependency.go` (或扩展 `task.go`) |

**描述**:
实现任务依赖的 REST 端点。可以加在 `TaskHandler` 中或独立 handler。

**端点设计**:
```
POST   /api/v1/tasks/{taskID}/dependencies         → AddDependency (body: {blocker_task_id})
DELETE /api/v1/tasks/{taskID}/dependencies/{blockerID} → RemoveDependency
GET    /api/v1/tasks/{taskID}/dependencies          → List (query: direction=blockers|blocked)
```

**验收标准**:
- [ ] `POST` 成功创建依赖 → 201 + 返回依赖对象
- [ ] `GET ?direction=blockers` 返回阻塞当前任务的任务列表
- [ ] `GET ?direction=blocked` 返回被当前任务阻塞的任务列表
- [ ] DAG 循环检测失败 → 409
- [ ] 请求体格式校验失败 → 400

---

### T1.2.4 — Extend TaskService: 阻塞检查 + 完成通知

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.4 |
| **预估** | M (4h) |
| **角色** | 后端工程师 |
| **依赖** | T1.2.2, T1.2.3 |
| **文件** | `internal/server/service/task.go` (修改) |

**描述**:
在现有 `TaskService` 中集成依赖检查。

**变更点**:

1. **ClaimTask**: 在 claim 之前增加阻塞检查
```go
// 在 status 校验之后、actual claim 之前:
isBlocked, err := depSvc.IsBlocked(ctx, taskID)
if isBlocked {
    return nil, ErrTaskBlocked  // "task is blocked by incomplete dependencies"
}
```

2. **UpdateTask**: 当 status 变为 done/closed 时，通知被阻塞的任务
```go
// 在 UpdateTask 成功后，如果 newStatus ∈ {done, closed}:
go func() {
    blockedTasks, _ := depSvc.GetBlockedBy(ctx, taskID)
    for _, bt := range blockedTasks {
        // 通过 WebSocket broadcast 通知频道: "Task #N is now unblocked"
        // 或发送 system message 到 blocked task 的 thread
    }
}()
```

3. **GetTask**: 查询结果中附带依赖信息
```go
// 在 GetTask 返回的 Task struct 中增加字段:
BlockedBy []BlockedByInfo  `json:"blocked_by,omitempty"`   // 谁在阻塞我
Blocking  []BlockingInfo   `json:"blocking,omitempty"`     // 我在阻塞谁
```

**验收标准**:
- [ ] 被阻塞的任务无法 claim → 返回 "task is blocked" 错误
- [ ] blocker 完成后，blocked task 可正常 claim
- [ ] GetTask 返回 `blocked_by` 数组，包含 blocker 的 id/task_number/title/status
- [ ] blocker 变更为 done 后，blocked task 的 claimer 收到通知
- [ ] 新增 `ErrTaskBlocked` 错误类型

---

### T1.2.5 — Frontend: Kanban 卡片依赖展示

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.5 |
| **预估** | M (5h) |
| **角色** | 前端工程师 |
| **依赖** | T1.2.3 (API 可用) |
| **文件** | `frontend/src/` (Kanban 相关组件) |

**描述**:
在 Kanban 视图中展示任务依赖关系。

**功能清单**:
1. 被阻塞的卡片灰显 (opacity-50) + 左上角显示 "等待 #N 完成" badge
2. badge 可点击跳转到 blocker 任务
3. 任务详情面板中显示 "阻塞关系" 区块：
   - "正在等待: #3 实现登录 API" (blocked_by)
   - "正在阻塞: #5 部署到生产" (blocking)
4. 在任务详情中支持添加/删除依赖（选择已有任务的搜索下拉框）
5. 依赖状态实时更新（WebSocket 推送 blocker 状态变更）

**API 调用**:
- `GET /api/v1/tasks/{id}/dependencies?direction=blockers`
- `GET /api/v1/tasks/{id}/dependencies?direction=blocked`
- `POST /api/v1/tasks/{id}/dependencies`
- `DELETE /api/v1/tasks/{id}/dependencies/{blocker_id}`

**验收标准**:
- [ ] 有 blocker 的任务卡片显示 "等待 #N 完成" badge
- [ ] 所有 blocker 都完成后 badge 消失，卡片恢复正常样式
- [ ] 任务详情中可查看完整依赖链
- [ ] 可添加/删除依赖（前端做简要校验：不能选自己）
- [ ] 添加依赖失败（如循环）有错误提示
- [ ] WebSocket 收到 blocker 完成事件后自动刷新依赖状态

---

### T1.2.6 — Test: Task Dependencies 集成测试

| 属性 | 内容 |
|------|------|
| **ID** | T1.2.6 |
| **预估** | S (2h) |
| **角色** | QA工程师 |
| **依赖** | T1.2.2 (Service 可用) |
| **文件** | `internal/server/handler/task_dependency_test.go` |

**描述**:
编写集成测试覆盖任务依赖的核心场景。

**测试用例**:
1. 添加依赖 A → B → 201
2. 添加自引用依赖 A → A → 400
3. 添加循环依赖 A→B, B→C, 尝试 C→A → 409
4. 有未完成 blocker 时 claim → 409/blocked
5. blocker 完成后 claim → 200/success
6. 完成 A 后，被 A 阻塞的 B 收到通知
7. 列出 blocker → 200，数量正确
8. 列出 blocked → 200，数量正确
9. 删除依赖 → 204
10. 删除 task 后，相关依赖 CASCADE 删除

**验收标准**:
- [ ] 10 个用例全部通过
- [ ] 测试隔离（每个用例创建独立的 task）

---

### T1.3.1 — Migration: agent_delegations 表

| 属性 | 内容 |
|------|------|
| **ID** | T1.3.1 |
| **预估** | S (1h) |
| **角色** | 后端工程师 |
| **依赖** | 无 |
| **文件** | `migrations/000029_create_agent_delegations.up.sql` / `.down.sql` |

**描述**:
创建 `agent_delegations` 表。

**SQL DDL** (from roadmap):
```sql
CREATE TABLE agent_delegations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id   UUID NOT NULL REFERENCES agents(id),
    to_agent_id     UUID NOT NULL REFERENCES agents(id),
    task_id         UUID REFERENCES tasks(id),
    channel_id      UUID NOT NULL REFERENCES channels(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'queued',
    message         TEXT,
    start_if_inactive BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**验收标准**:
- [ ] `migrate up` 成功创建
- [ ] status 默认值为 'queued'
- [ ] `start_if_inactive` 默认值为 false
- [ ] 外键约束生效

---

### T1.3.2 — Service: DelegationService + 状态机

| 属性 | 内容 |
|------|------|
| **ID** | T1.3.2 |
| **预估** | M (5h) |
| **角色** | 后端工程师 |
| **依赖** | T1.3.1 |
| **文件** | `internal/server/service/delegation.go` |

**描述**:
创建 `DelegationService`，实现 4 态委托状态机。

**状态机**:
```
queued → delivered → started → completed
                           ↘ failed
```

**核心方法**:
```
CreateDelegation(ctx, fromAgentID, toAgentID, taskID, channelID, message, startIfInactive) (*Delegation, error)
  → 校验 from != to
  → 校验 channel 存在且 from_agent 是 channel member
  → 如果 taskID 不为空，校验 task 存在且属于该 channel
  → status = 'queued'
  → INSERT + RETURNING

DeliverDelegation(ctx, delegationID) error
  → status: queued → delivered
  → 只能从 queued 流转

StartDelegation(ctx, delegationID) error
  → status: delivered → started
  → updated_at = now()

CompleteDelegation(ctx, delegationID) error
  → status: started → completed
  → updated_at = now()

FailDelegation(ctx, delegationID, reason string) error
  → status: 任意非终态 → failed
  → 写入 message 字段

AcceptDelegation(ctx, delegationID) error
  → DeliverDelegation + StartDelegation 合并
  → 即 queued → delivered → started

RejectDelegation(ctx, delegationID, reason string) error
  → queued → failed
  → 写入 reason 到 message

ListDelegations(ctx, filter) ([]Delegation, error)
  → filter: from_agent_id, to_agent_id, status, channel_id
```

**验收标准**:
- [ ] queued → delivered → started → completed 完整链条可走通
- [ ] 非法状态流转被拒绝（如 completed → started）
- [ ] Reject: queued → failed，reason 写入 message
- [ ] 重复 Accept 第二次调用被拒绝
- [ ] 记录每次状态变更的 updated_at

---

### T1.3.3 — Handler: Delegations REST API

| 属性 | 内容 |
|------|------|
| **ID** | T1.3.3 |
| **预估** | M (4h) |
| **角色** | 后端工程师 |
| **依赖** | T1.3.2 |
| **文件** | `internal/server/handler/delegation.go` |

**描述**:
创建 `DelegationHandler`，实现委托 REST 端点。

**端点设计**:
```
POST   /api/v1/delegations                    → CreateDelegation
GET    /api/v1/delegations                    → List (query: from_agent_id, to_agent_id, status, channel_id)
GET    /api/v1/delegations/{id}               → Get
POST   /api/v1/delegations/{id}/accept         → Accept (queued → started)
POST   /api/v1/delegations/{id}/reject         → Reject (body: {reason: "..."})
POST   /api/v1/delegations/{id}/complete       → Complete (started → completed)
POST   /api/v1/delegations/{id}/fail           → Fail (body: {reason: "..."})
```

**验收标准**:
- [ ] 创建委托 → 201 + delegation 对象
- [ ] Accept → 200，status 变为 started
- [ ] Reject → 200，status 变为 failed，message 含 reason
- [ ] Complete → 200，status 变为 completed
- [ ] Fail → 200，status 变为 failed
- [ ] 非法状态流转 → 409 Conflict
- [ ] 需要认证

---

### T1.3.4 — Test: Delegations 集成测试

| 属性 | 内容 |
|------|------|
| **ID** | T1.3.4 |
| **预估** | S (2h) |
| **角色** | QA工程师 |
| **依赖** | T1.3.2 (Service 可用) |
| **文件** | `internal/server/handler/delegation_test.go` |

**描述**:
编写集成测试覆盖委托状态机。

**测试用例**:
1. 创建委托 → 201, status=queued
2. Accept → 200, status=started
3. 从 queued 直接 complete → 409（必须先 started）
4. Reject queued 委托 → 200 -> status=failed
5. 完整流程: queued → delivered → started → completed → 200 each step
6. 列出某 agent 的 incoming delegations → 200
7. 列出某 agent 的 outgoing delegations → 200
8. 重复 Accept → 409
9. 已完成的委托 reject → 409
10. 创建委托时 from_agent == to_agent → 400

**验收标准**:
- [ ] 10 个用例全部通过

---

## 风险登记册 (Step 1)

| 风险 | 概率 | 影响 | 缓解措施 | 触发条件 |
|------|------|------|----------|----------|
| reports_to 循环检测漏掉间接环 | 低 | 高 | BFS 遍历完整路径，单元测试覆盖 3+ 层间接环 | BFS 只走 2 层就停止 |
| Task DAG 检测性能问题（大项目） | 低 | 中 | BFS 带 visited set，初期 task 数量少不是问题 | task 总数 > 1000 |
| 前端 Agent name 解析（只有 UUID） | 中 | 低 | API 返回时 JOIN agent name，前端不单独查询 | API 返回中只有 UUID |
| 三个 migration 并行开发有编号冲突 | 中 | 低 | 提前分配编号: 027/028/029 | 多人同时创建 migration 文件 |

---

## Definition of Done (Step 级别)

- [ ] 3 个 migration 全部可 up/down
- [ ] 3 组 REST API 全部可调通（curl 或 Postman 验证）
- [ ] 循环检测 / DAG 检测 / 状态机 核心逻辑有单元测试覆盖
- [ ] 前端页面可展示和创建关系
- [ ] Kanban 卡片显示依赖状态
- [ ] 所有集成测试通过
- [ ] Code review 完成
- [ ] API 文档（至少在 roadmap 中已有 SQL + 本文件中的端点描述可作参考）

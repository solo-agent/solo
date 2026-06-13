# Step 1 — Foundation: 关系与依赖建模

> 迁移编号: 000027–000029 | 预估工期: 2-3 weeks
> 新增表: `agent_relationships`, `task_dependencies`, `agent_delegations`

## 1. 数据库迁移

### 1.1 Migration 000027 — agent_relationships

**文件**: `migrations/000027_create_agent_relationships.up.sql`

```sql
CREATE TABLE agent_relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rel_type      VARCHAR(20) NOT NULL CHECK (rel_type IN ('reports_to', 'delegates_to', 'collaborates_with', 'escalates_to')),
    channel_id    UUID REFERENCES channels(id) ON DELETE CASCADE,  -- NULL = global
    weight        REAL NOT NULL DEFAULT 1.0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 自引用排除
ALTER TABLE agent_relationships ADD CONSTRAINT chk_no_self_relationship
    CHECK (from_agent_id != to_agent_id);

-- 全局关系（reports_to / escalates_to）：同一方向同一类型不重复
CREATE UNIQUE INDEX idx_rel_global
    ON agent_relationships(from_agent_id, to_agent_id, rel_type)
    WHERE rel_type IN ('reports_to', 'escalates_to');

-- 频道关系（delegates_to / collaborates_with）：同频道内不重复
CREATE UNIQUE INDEX idx_rel_channel
    ON agent_relationships(from_agent_id, to_agent_id, rel_type, channel_id)
    WHERE rel_type IN ('delegates_to', 'collaborates_with');

-- collaborates_with 是双向的，额外的唯一约束防止 (A,B) 和 (B,A) 同时存在
CREATE UNIQUE INDEX idx_collab_bidirectional
    ON agent_relationships(LEAST(from_agent_id, to_agent_id), GREATEST(from_agent_id, to_agent_id), rel_type, channel_id)
    WHERE rel_type = 'collaborates_with';

-- 查询索引
CREATE INDEX idx_rel_from_agent ON agent_relationships(from_agent_id);
CREATE INDEX idx_rel_to_agent ON agent_relationships(to_agent_id);
CREATE INDEX idx_rel_channel_id ON agent_relationships(channel_id);
CREATE INDEX idx_rel_type ON agent_relationships(rel_type);
```

**文件**: `migrations/000027_create_agent_relationships.down.sql`

```sql
DROP TABLE IF EXISTS agent_relationships;
```

### 1.2 Migration 000028 — task_dependencies

**文件**: `migrations/000028_create_task_dependencies.up.sql`

```sql
CREATE TABLE task_dependencies (
    blocker_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_task_id, blocked_task_id),
    CHECK (blocker_task_id != blocked_task_id)
);

-- 禁止循环依赖的触发器辅助查询（反向查询谁阻塞了我）
CREATE INDEX idx_deps_blocked ON task_dependencies(blocked_task_id);
-- 查询我阻塞了谁
CREATE INDEX idx_deps_blocker ON task_dependencies(blocker_task_id);
```

**文件**: `migrations/000028_create_task_dependencies.down.sql`

```sql
DROP TABLE IF EXISTS task_dependencies;
```

### 1.3 Migration 000029 — agent_delegations

**文件**: `migrations/000029_create_agent_delegations.up.sql`

```sql
CREATE TABLE agent_delegations (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id       UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id           UUID REFERENCES tasks(id) ON DELETE SET NULL,
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status            VARCHAR(20) NOT NULL DEFAULT 'queued'
                      CHECK (status IN ('queued', 'delivered', 'started', 'completed', 'failed', 'rejected')),
    message           TEXT,
    start_if_inactive BOOLEAN NOT NULL DEFAULT false,
    rejection_reason  TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 委托方查询（我委托了谁）
CREATE INDEX idx_delegations_from ON agent_delegations(from_agent_id, status);
-- 受托方查询（谁委托了我）
CREATE INDEX idx_delegations_to ON agent_delegations(to_agent_id, status);
-- 频道维度查询
CREATE INDEX idx_delegations_channel ON agent_delegations(channel_id);
-- 任务维度
CREATE INDEX idx_delegations_task ON agent_delegations(task_id);
```

**文件**: `migrations/000029_create_agent_delegations.down.sql`

```sql
DROP TABLE IF EXISTS agent_delegations;
```

---

## 2. Service 层

### 2.1 RelationshipService

**文件**: `internal/server/service/relationship.go`

```go
package service

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

var (
    ErrRelationshipNotFound    = errors.New("relationship not found")
    ErrSelfRelationship        = errors.New("cannot create relationship to self")
    ErrCircularReportsTo       = errors.New("reports_to relationship would create a cycle")
    ErrDuplicateRelationship   = errors.New("relationship already exists")
    ErrInvalidRelationshipType = errors.New("invalid relationship type")
    ErrChannelRequired         = errors.New("channel_id is required for this relationship type")
)

var ValidRelationshipTypes = []string{
    "reports_to", "delegates_to", "collaborates_with", "escalates_to",
}

// Relationship 代表 agent 之间的一个关系。
type Relationship struct {
    ID          string    `json:"id"`
    FromAgentID string    `json:"from_agent_id"`
    ToAgentID   string    `json:"to_agent_id"`
    RelType     string    `json:"rel_type"`
    ChannelID   string    `json:"channel_id,omitempty"`
    Weight      float64   `json:"weight"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type CreateRelationshipRequest struct {
    ToAgentID string  `json:"to_agent_id"`
    RelType   string  `json:"rel_type"`
    ChannelID string  `json:"channel_id,omitempty"`
    Weight    float64 `json:"weight,omitempty"`
}

type RelationshipService struct {
    pool *pgxpool.Pool
}

func NewRelationshipService(pool *pgxpool.Pool) *RelationshipService {
    return &RelationshipService{pool: pool}
}

// Create 创建一个 Agent 关系。
func (s *RelationshipService) Create(ctx context.Context, fromAgentID string, req CreateRelationshipRequest) (*Relationship, error) {
    // 1. 参数校验
    if fromAgentID == req.ToAgentID {
        return nil, ErrSelfRelationship
    }
    if !isValidRelType(req.RelType) {
        return nil, ErrInvalidRelationshipType
    }
    // 频道级别的关系必须有 channel_id
    if (req.RelType == "delegates_to" || req.RelType == "collaborates_with") && req.ChannelID == "" {
        return nil, ErrChannelRequired
    }
    if req.Weight == 0 {
        req.Weight = 1.0
    }

    // 2. 验证 Agent 存在
    if err := s.agentExists(ctx, fromAgentID); err != nil {
        return nil, err
    }
    if err := s.agentExists(ctx, req.ToAgentID); err != nil {
        return nil, err
    }

    // 3. reports_to 循环检测：BFS 从 to_agent 向上遍历 reports_to 链，
    //    如果遇到 from_agent，则创建此关系会成环。
    if req.RelType == "reports_to" {
        hasCycle, err := s.wouldCreateCycle(ctx, fromAgentID, req.ToAgentID)
        if err != nil {
            return nil, fmt.Errorf("cycle check: %w", err)
        }
        if hasCycle {
            return nil, ErrCircularReportsTo
        }
    }

    // 4. 插入
    id := uuid.New().String()
    now := time.Now()
    _, err := s.pool.Exec(ctx, `
        INSERT INTO agent_relationships (id, from_agent_id, to_agent_id, rel_type, channel_id, weight, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `, id, fromAgentID, req.ToAgentID, req.RelType, nullableStr(req.ChannelID), req.Weight, now, now)
    if err != nil {
        if isPgUniqueViolation(err) {
            return nil, ErrDuplicateRelationship
        }
        return nil, err
    }

    return &Relationship{
        ID:          id,
        FromAgentID: fromAgentID,
        ToAgentID:   req.ToAgentID,
        RelType:     req.RelType,
        ChannelID:   req.ChannelID,
        Weight:      req.Weight,
        CreatedAt:   now,
        UpdatedAt:   now,
    }, nil
}

// List 查询某个 Agent 的所有关系，可选过滤 rel_type 和 channel_id。
func (s *RelationshipService) List(ctx context.Context, agentID string, relType string, channelID string) ([]Relationship, error) {
    query := `SELECT id, from_agent_id, to_agent_id, rel_type, COALESCE(channel_id::text, ''), weight, created_at, updated_at
              FROM agent_relationships WHERE (from_agent_id = $1 OR to_agent_id = $1)`
    args := []any{agentID}
    argIdx := 2

    if relType != "" {
        query += fmt.Sprintf(" AND rel_type = $%d", argIdx)
        args = append(args, relType)
        argIdx++
    }
    if channelID != "" {
        query += fmt.Sprintf(" AND (channel_id = $%d OR channel_id IS NULL)", argIdx)
        args = append(args, channelID)
        argIdx++
    }
    query += " ORDER BY rel_type, weight DESC"

    rows, err := s.pool.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var rels []Relationship
    for rows.Next() {
        var r Relationship
        if err := rows.Scan(&r.ID, &r.FromAgentID, &r.ToAgentID, &r.RelType, &r.ChannelID, &r.Weight, &r.CreatedAt, &r.UpdatedAt); err != nil {
            return nil, err
        }
        rels = append(rels, r)
    }
    if rels == nil {
        rels = []Relationship{}
    }
    return rels, rows.Err()
}

// GetGraph 返回整个关系图数据，供前端渲染。
func (s *RelationshipService) GetGraph(ctx context.Context) ([]Relationship, error) {
    return s.List(ctx, "", "", "") // 全量查询，后续 Step 可加 channel 过滤
}

// Delete 删除一个关系。
func (s *RelationshipService) Delete(ctx context.Context, relID string, callerAgentID string) error {
    // 只有 from_agent 可以删除自己发出的关系
    result, err := s.pool.Exec(ctx,
        `DELETE FROM agent_relationships WHERE id = $1 AND from_agent_id = $2`,
        relID, callerAgentID)
    if err != nil {
        return err
    }
    if result.RowsAffected() == 0 {
        return ErrRelationshipNotFound
    }
    return nil
}

// wouldCreateCycle BFS 从 toAgent 向上遍历 reports_to 链。
func (s *RelationshipService) wouldCreateCycle(ctx context.Context, fromAgent, toAgent string) (bool, error) {
    visited := map[string]bool{fromAgent: true}
    queue := []string{toAgent}

    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]

        if current == fromAgent {
            return true, nil
        }
        if visited[current] {
            continue
        }
        visited[current] = true

        // 查询 current 的 reports_to 上级
        rows, err := s.pool.Query(ctx,
            `SELECT to_agent_id FROM agent_relationships WHERE from_agent_id = $1 AND rel_type = 'reports_to'`,
            current)
        if err != nil {
            return false, err
        }
        var managers []string
        for rows.Next() {
            var m string
            if err := rows.Scan(&m); err != nil {
                rows.Close()
                return false, err
            }
            managers = append(managers, m)
        }
        rows.Close()
        queue = append(queue, managers...)
    }
    return false, nil
]

func (s *RelationshipService) agentExists(ctx context.Context, agentID string) error {
    var exists bool
    err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, agentID).Scan(&exists)
    if err != nil {
        return err
    }
    if !exists {
        return errors.New("agent not found")
    }
    return nil
}
```

### 2.2 TaskDependencyService

**文件**: `internal/server/service/task_dependency.go`

```go
package service

// ... (imports)

var (
    ErrDependencyNotFound       = errors.New("dependency not found")
    ErrSelfDependency           = errors.New("task cannot depend on itself")
    ErrCircularDependency       = errors.New("this dependency would create a cycle")
    ErrDuplicateDependency      = errors.New("dependency already exists")
    ErrBlockerNotInSameChannel  = errors.New("blocker task must be in the same channel")
)

// TaskDependency 代表一个任务被另一个任务阻塞。
type TaskDependency struct {
    BlockerTaskID string    `json:"blocker_task_id"`
    BlockedTaskID string    `json:"blocked_task_id"`
    CreatedAt     time.Time `json:"created_at"`
}

type TaskDependencyService struct {
    pool *pgxpool.Pool
}

func NewTaskDependencyService(pool *pgxpool.Pool) *TaskDependencyService {
    return &TaskDependencyService{pool: pool}
}

// Block 让 blockedTaskID 依赖于 blockerTaskID。
func (s *TaskDependencyService) Block(ctx context.Context, blockedTaskID, blockerTaskID string) (*TaskDependency, error) {
    if blockedTaskID == blockerTaskID {
        return nil, ErrSelfDependency
    }

    // 确认两个 task 存在并且同频道
    var bChan, rChan string
    err := s.pool.QueryRow(ctx, `SELECT channel_id FROM tasks WHERE id = $1`, blockedTaskID).Scan(&bChan)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return nil, ErrTaskNotFound }
        return nil, err
    }
    err = s.pool.QueryRow(ctx, `SELECT channel_id FROM tasks WHERE id = $1`, blockerTaskID).Scan(&rChan)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return nil, ErrTaskNotFound }
        return nil, err
    }
    if bChan != rChan {
        return nil, ErrBlockerNotInSameChannel
    }

    // 循环检测
    hasCycle, _ := s.wouldCreateCycle(ctx, blockedTaskID, blockerTaskID)
    if hasCycle {
        return nil, ErrCircularDependency
    }

    now := time.Now()
    _, err = s.pool.Exec(ctx,
        `INSERT INTO task_dependencies (blocker_task_id, blocked_task_id, created_at) VALUES ($1, $2, $3)`,
        blockerTaskID, blockedTaskID, now)
    if err != nil {
        if isPgUniqueViolation(err) { return nil, ErrDuplicateDependency }
        return nil, err
    }

    return &TaskDependency{BlockerTaskID: blockerTaskID, BlockedTaskID: blockedTaskID, CreatedAt: now}, nil
}

// Unblock 移除一个阻塞关系。
func (s *TaskDependencyService) Unblock(ctx context.Context, blockedTaskID, blockerTaskID string) error {
    result, err := s.pool.Exec(ctx,
        `DELETE FROM task_dependencies WHERE blocker_task_id = $1 AND blocked_task_id = $2`,
        blockerTaskID, blockedTaskID)
    if err != nil { return err }
    if result.RowsAffected() == 0 { return ErrDependencyNotFound }
    return nil
}

// GetBlockers 返回阻塞该 task 的所有 task。
func (s *TaskDependencyService) GetBlockers(ctx context.Context, taskID string) ([]TaskDependency, error) {
    rows, err := s.pool.Query(ctx,
        `SELECT blocker_task_id, blocked_task_id, created_at FROM task_dependencies WHERE blocked_task_id = $1`, taskID)
    if err != nil { return nil, err }
    defer rows.Close()
    return scanDeps(rows)
}

// GetBlocked 返回被该 task 阻塞的所有 task。
func (s *TaskDependencyService) GetBlocked(ctx context.Context, taskID string) ([]TaskDependency, error) {
    rows, err := s.pool.Query(ctx,
        `SELECT blocker_task_id, blocked_task_id, created_at FROM task_dependencies WHERE blocker_task_id = $1`, taskID)
    if err != nil { return nil, err }
    defer rows.Close()
    return scanDeps(rows)
}

// IsBlocked 检查任务是否被未完成的 blocker 阻塞。
func (s *TaskDependencyService) IsBlocked(ctx context.Context, taskID string) (bool, []string, error) {
    rows, err := s.pool.Query(ctx, `
        SELECT td.blocker_task_id FROM task_dependencies td
        JOIN tasks t ON td.blocker_task_id = t.id
        WHERE td.blocked_task_id = $1 AND t.status NOT IN ('done', 'closed')
    `, taskID)
    if err != nil { return false, nil, err }
    defer rows.Close()
    var blockers []string
    for rows.Next() {
        var bid string
        if err := rows.Scan(&bid); err != nil { return false, nil, err }
        blockers = append(blockers, bid)
    }
    return len(blockers) > 0, blockers, nil
}

// wouldCreateCycle BFS 检查。
func (s *TaskDependencyService) wouldCreateCycle(ctx context.Context, blockedTaskID, blockerTaskID string) (bool, error) {
    visited := map[string]bool{blockedTaskID: true}
    queue := []string{blockerTaskID}
    for len(queue) > 0 {
        cur := queue[0]; queue = queue[1:]
        if cur == blockedTaskID { return true, nil }
        if visited[cur] { continue }
        visited[cur] = true
        rows, err := s.pool.Query(ctx,
            `SELECT blocked_task_id FROM task_dependencies WHERE blocker_task_id = $1`, cur)
        if err != nil { return false, err }
        var next []string
        for rows.Next() {
            var n string; rows.Scan(&n); next = append(next, n)
        }
        rows.Close()
        queue = append(queue, next...)
    }
    return false, nil
}
```

### 2.3 DelegationService

**文件**: `internal/server/service/delegation.go`

```go
package service

// ... (imports)

var (
    ErrDelegationNotFound         = errors.New("delegation not found")
    ErrDelegationInvalidTransition = errors.New("invalid delegation status transition")
    ErrDelegationNotRecipient      = errors.New("only the recipient can accept/reject this delegation")
)

// 状态机
var delegationTransitions = map[string]map[string]bool{
    "queued":    {"delivered": true, "rejected": true},
    "delivered": {"started": true, "rejected": true},
    "started":   {"completed": true, "failed": true},
    // completed / failed / rejected 是终态
}

type Delegation struct {
    ID               string     `json:"id"`
    FromAgentID      string     `json:"from_agent_id"`
    ToAgentID        string     `json:"to_agent_id"`
    TaskID           string     `json:"task_id,omitempty"`
    ChannelID        string     `json:"channel_id"`
    Status           string     `json:"status"`
    Message          string     `json:"message,omitempty"`
    StartIfInactive  bool       `json:"start_if_inactive"`
    RejectionReason  string     `json:"rejection_reason,omitempty"`
    CreatedAt        time.Time  `json:"created_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateDelegationRequest struct {
    ToAgentID       string `json:"to_agent_id"`
    TaskID          string `json:"task_id,omitempty"`
    ChannelID       string `json:"channel_id"`
    Message         string `json:"message,omitempty"`
    StartIfInactive bool   `json:"start_if_inactive,omitempty"`
}

type DelegationService struct {
    pool *pgxpool.Pool
}

func NewDelegationService(pool *pgxpool.Pool) *DelegationService {
    return &DelegationService{pool: pool}
}

func (s *DelegationService) Create(ctx context.Context, fromAgentID string, req CreateDelegationRequest) (*Delegation, error) {
    // 验证是否存在 delegations_to 关系
    // ...
    // 创建 delegation (状态=queued)
    // ...
}

func (s *DelegationService) TransitionStatus(ctx context.Context, delegationID, toAgentID, newStatus, reason string) (*Delegation, error) {
    // 状态机校验 + 只有受托方可以操作
    // ...
}

func (s *DelegationService) List(ctx context.Context, agentID string, status string) ([]Delegation, error) {
    // 查询收到或发出的委托
    // ...
}
```

---

## 3. HTTP Handler 层

### 3.1 Relationship Handler

**文件**: `internal/server/handler/relationship.go`

| Method | Path | Handler | 说明 |
|--------|------|---------|------|
| POST   | `/api/v1/agents/{agentID}/relationships` | `Create` | 创建关系 |
| GET    | `/api/v1/agents/{agentID}/relationships` | `List` | 查询关系（?rel_type= & ?channel_id=） |
| DELETE | `/api/v1/agents/{agentID}/relationships/{relID}` | `Delete` | 删除关系 |
| GET    | `/api/v1/relationships/graph` | `GetGraph` | 获取完整关系图 |

**请求/响应结构**:

```go
// POST /api/v1/agents/{agentID}/relationships
type CreateRelationshipRequest struct {
    ToAgentID string  `json:"to_agent_id"`
    RelType   string  `json:"rel_type"`      // reports_to|delegates_to|collaborates_with|escalates_to
    ChannelID string  `json:"channel_id,omitempty"`
    Weight    float64 `json:"weight,omitempty"`
}

type RelationshipResponse struct {
    ID          string  `json:"id"`
    FromAgentID string  `json:"from_agent_id"`
    FromName    string  `json:"from_name"`
    ToAgentID   string  `json:"to_agent_id"`
    ToName      string  `json:"to_name"`
    RelType     string  `json:"rel_type"`
    ChannelID   string  `json:"channel_id,omitempty"`
    Weight      float64 `json:"weight"`
    CreatedAt   string  `json:"created_at"`
    UpdatedAt   string  `json:"updated_at"`
}
```

### 3.2 TaskDependency Handler

**文件**: `internal/server/handler/task_dependency.go`

| Method | Path | Handler | 说明 |
|--------|------|---------|------|
| POST   | `/api/v1/tasks/{taskID}/block` | `Block` | 设置阻塞（body: `{"blocked_task_id": "..."}` 或反向） |
| DELETE | `/api/v1/tasks/{taskID}/block` | `Unblock` | 移除阻塞 |
| GET    | `/api/v1/tasks/{taskID}/blockers` | `Blockers` | 查看谁阻塞了当前任务 |
| GET    | `/api/v1/tasks/{taskID}/blocked` | `Blocked` | 查看当前任务阻塞了谁 |

### 3.3 Delegation Handler

**文件**: `internal/server/handler/delegation.go`

| Method | Path | Handler | 说明 |
|--------|------|---------|------|
| POST   | `/api/v1/agents/{agentID}/delegations` | `Create` | 创建委托 |
| GET    | `/api/v1/agents/{agentID}/delegations` | `List` | 查看委托列表 |
| PATCH  | `/api/v1/delegations/{id}/accept` | `Accept` | 接受委托 |
| PATCH  | `/api/v1/delegations/{id}/reject` | `Reject` | 拒绝委托 |
| PATCH  | `/api/v1/delegations/{id}/start` | `Start` | 开始执行 |
| PATCH  | `/api/v1/delegations/{id}/complete` | `Complete` | 标记完成 |
| PATCH  | `/api/v1/delegations/{id}/fail` | `Fail` | 标记失败 |

---

## 4. Router 集成

在 `internal/server/router.go` 的 `NewRouter` 中新增:

```go
// 初始化新服务
relSvc := service.NewRelationshipService(pool)
depSvc := service.NewTaskDependencyService(pool)
delSvc := service.NewDelegationService(pool)

// 初始化新 handler
relHandler := handler.NewRelationshipHandler(pool, relSvc)
depHandler := handler.NewTaskDependencyHandler(pool, depSvc)
delHandler := handler.NewDelegationHandler(pool, delSvc)

// ------ 路由注册 ------

// Agent Relationships（在 /api/v1/agents 路由组内）
r.Route("/api/v1/agents", func(r chi.Router) {
    r.Route("/{agentID}", func(r chi.Router) {
        r.Route("/relationships", func(r chi.Router) {
            r.Get("/", relHandler.List)
            r.Post("/", relHandler.Create)
            r.Delete("/{relID}", relHandler.Delete)
        })
        r.Route("/delegations", func(r chi.Router) {
            r.Get("/", delHandler.List)
            r.Post("/", delHandler.Create)
        })
    })
})

// 关系图谱
r.Get("/api/v1/relationships/graph", relHandler.GetGraph)

// 委托状态流转
r.Route("/api/v1/delegations/{id}", func(r chi.Router) {
    r.Patch("/accept", delHandler.Accept)
    r.Patch("/reject", delHandler.Reject)
    r.Patch("/start", delHandler.Start)
    r.Patch("/complete", delHandler.Complete)
    r.Patch("/fail", delHandler.Fail)
})

// Task Dependencies
r.Route("/api/v1/tasks/{taskID}", func(r chi.Router) {
    r.Post("/block", depHandler.Block)
    r.Delete("/block", depHandler.Unblock)
    r.Get("/blockers", depHandler.Blockers)
    r.Get("/blocked", depHandler.Blocked)
})
```

---

## 5. CLI 设计

在 `cmd/solo/main.go` 的 `runCLI` 中扩展 `switch` case:

```go
case "relationship":
    handleRelationship(args[1:], baseURL, token)
case "delegate":
    handleDelegate(args[1:], baseURL, token)
```

### 5.1 `solo relationship` 子命令

```bash
# 创建关系
solo relationship create --from @Alice --to @Bob --type reports_to

# 列出关系
solo relationship list --agent @Alice [--type reports_to] [--channel <id>]

# 删除关系
solo relationship delete <rel-id>
```

### 5.2 `solo task block/unblock` 子命令

扩展现有 `handleTask`:

```bash
# 设置依赖
solo task block -n 5 -c <channel_id> --on 3
solo task unblock -n 5 -c <channel_id> --on 3
solo task list --blocked    # 查看自己被阻塞的任务
```

### 5.3 `solo agent delegate` 子命令

```bash
# 委托任务
solo agent delegate --to @Bob --task 3 --msg "请实现 POST /api/login"
solo agent delegate list [--status queued]
solo agent delegate accept <delegation-id>
solo agent delegate reject <delegation-id> --reason "不擅长这块"
```

CLI 实现细节（`handleDelegate`）:

```go
func handleDelegate(args []string, baseURL, token string) {
    agentID := os.Getenv("SOLO_AGENT_ID")
    sub := args[0]
    switch sub {
    case "list":
        // GET /api/v1/agents/{agentID}/delegations?status=...
    case "accept":
        // PATCH /api/v1/delegations/{id}/accept
    case "reject":
        // PATCH /api/v1/delegations/{id}/reject
    default:
        // POST /api/v1/agents/{agentID}/delegations (create)
    }
}
```

---

## 6. Frontend 类型扩展

在 `frontend/lib/types.ts` 中新增:

```typescript
// ---- Relationship types ----
export type RelationshipType = 'reports_to' | 'delegates_to' | 'collaborates_with' | 'escalates_to';

export interface AgentRelationship {
  id: string;
  from_agent_id: string;
  from_name: string;
  to_agent_id: string;
  to_name: string;
  rel_type: RelationshipType;
  channel_id?: string;
  weight: number;
  created_at: string;
  updated_at: string;
}

// ---- Task Dependency types ----
export interface TaskDependency {
  blocker_task_id: string;
  blocked_task_id: string;
  created_at: string;
}

// ---- Delegation types ----
export type DelegationStatus = 'queued' | 'delivered' | 'started' | 'completed' | 'failed' | 'rejected';

export interface AgentDelegation {
  id: string;
  from_agent_id: string;
  from_name: string;
  to_agent_id: string;
  to_name: string;
  task_id?: string;
  channel_id: string;
  status: DelegationStatus;
  message?: string;
  start_if_inactive: boolean;
  rejection_reason?: string;
  created_at: string;
  updated_at: string;
}
```

扩展 `Task` 接口添加依赖字段:

```typescript
export interface Task {
  // ... 已有字段
  /** 被阻塞的依赖列表 */
  blockers?: TaskDependency[];
  /** 阻塞中的依赖数量 */
  active_blocker_count?: number;
}
```

---

## 7. 集成点汇总

| 组件 | 文件 | 操作 |
|------|------|------|
| Migration Runner | `cmd/migrate/main.go` | 无需改动，自动扫描新迁移 |
| Server Router | `internal/server/router.go` | 新增 3 个 handler 初始化 + 路由注册 |
| Task Handler | `internal/server/handler/task.go` | `Get` 方法追加 `blocker_count` 查询 |
| Agent Handler | `internal/server/handler/agent.go` | 可复用 `requireUserID` 模式 |
| CLI | `cmd/solo/main.go` | 新增 `relationship` / `delegate` 子命令 |
| Frontend Types | `frontend/lib/types.ts` | 新增关系、依赖、委托类型 |
| Frontend API | `frontend/lib/hooks/` | 新增 `useRelationships` / `useDependencies` / `useDelegations` hook |

## 8. WebSocket 事件

当 Task 的 blocker 完成时，通过 WebSocket 广播通知被阻塞 Task 的 claimer:

```go
// 在 task 状态变为 done/closed 时触发
hub.Broadcast(realtime.Envelope("task_unblocked", map[string]interface{}{
    "blocked_task_id": "uuid-of-blocked-task",
    "channel_id":      "uuid-of-channel",
}))
```

前端订阅 `task_unblocked` 事件，在 Kanban 卡片上移除此 Task 的阻塞标记。

## 9. 测试策略

- **Service 层**: 单元测试 (内存 PostgreSQL via testcontainers 或 `pgx/v5/stdlib`)
- **Handler 层**: 集成测试 (Chi `httptest` + 真实数据库)
- **循环检测**: BFS 算法的测试用例（链、环、DAG）
- **状态机**: delegation 状态流转的所有路径测试
- **唯一约束**: 并发插入冲突的边界测试

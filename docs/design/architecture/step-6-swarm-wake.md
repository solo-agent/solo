# Step 6 — 高级: Agent Swarm + 定时唤醒

> 依赖 Step 1-4 全部就绪 | Phase 4 | 预估工期: 4-6 weeks
> 新增: `reminders` 表、Swarm 协调器、看门狗超时检测

---

## 1. Agent Swarm — 多 Agent 并行协作

### 1.1 触发流程

```
用户 (或 Agent) 创建复杂任务
  → solo task create --title "重构用户系统" --swarm
  → Server 将任务拆分为 DAG 子任务
  → 多个 Agent 认领无依赖的子任务
  → 各自在隔离 worktree 中工作
  → 完成后 merge 回频道共享 repo
  → 下游子任务自动解除阻塞
```

### 1.2 数据结构 — Swarm 任务扩展

在 `tasks` 表上新增 swarm 相关字段:

**迁移**: `migrations/000032_add_swarm_fields.up.sql`

```sql
ALTER TABLE tasks ADD COLUMN is_swarm BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE tasks ADD COLUMN swarm_plan JSONB;  -- { "breakdown": [...], "strategy": "parallel|sequential" }

COMMENT ON COLUMN tasks.is_swarm IS 'Whether this is a swarm (multi-agent) task';
COMMENT ON COLUMN tasks.swarm_plan IS 'Swarm execution plan: subtask breakdown and strategy';
```

### 1.3 Swarm 协调器

**文件**: `internal/server/service/swarm.go`

```go
type SwarmCoordinator struct {
    pool        *pgxpool.Pool
    taskSvc     *TaskService
    depSvc      *TaskDependencyService
    workspaceSvc *ChannelWorkspaceService
    hub         realtime.Broadcaster
}

func NewSwarmCoordinator(pool *pgxpool.Pool, taskSvc *TaskService, depSvc *TaskDependencyService, wsSvc *ChannelWorkspaceService, hub realtime.Broadcaster) *SwarmCoordinator {
    return &SwarmCoordinator{pool: pool, taskSvc: taskSvc, depSvc: depSvc, workspaceSvc: wsSvc, hub: hub}
}

// DecomposeTask 使用 LLM 将复杂任务拆分为 DAG 子任务。
func (s *SwarmCoordinator) DecomposeTask(ctx context.Context, taskID, channelID string) ([]Task, error) {
    task, err := s.taskSvc.GetTask(ctx, channelID, taskID, "")
    if err != nil { return nil, err }

    // 调用 LLM 拆分任务
    prompt := fmt.Sprintf(
        "Break down this task into parallel subtasks as a DAG. Return JSON array of {title, description, depends_on_indices: [int]}.\nTask: %s\nDescription: %s",
        task.Title, task.Description,
    )
    response := s.callLLMForDecomposition(ctx, prompt)

    // 解析 LLM 返回的 JSON
    var subtasks []SwarmSubtaskDef
    json.Unmarshal([]byte(response), &subtasks)

    // 创建子任务并建立依赖关系
    var created []Task
    for i, st := range subtasks {
        req := TaskCreateRequest{
            Title:       st.Title,
            Description: st.Description,
            ParentTaskID: taskID,
        }
        subtask, err := s.taskSvc.CreateTask(ctx, channelID, "system", req)
        if err != nil { continue }

        for _, depIdx := range st.DependsOn {
            if depIdx >= 0 && depIdx < len(created) {
                s.depSvc.Block(ctx, subtask.ID, created[depIdx].ID)
            }
        }
        created = append(created, *subtask)
    }

    // 标记父任务为 swarm
    s.pool.Exec(ctx, `UPDATE tasks SET is_swarm = true, swarm_plan = $1, status = 'in_progress' WHERE id = $2`,
        mustMarshal(map[string]interface{}{"breakdown": subtasks, "strategy": "parallel"}), taskID)

    // 广播：子任务可供认领
    for _, st := range created {
        s.hub.Broadcast(realtime.Envelope("task_created", taskToPayload(&st)))
    }

    return created, nil
}
```

### 1.4 Swarm CLI

```bash
# 创建 swarm 任务
solo task create -c <channel_id> --title "重构用户系统" --swarm

# 让顶层任务自动拆分为子任务
# 后端收到 POST 后调用 SwarmCoordinator.DecomposeTask

# 查看 swarm 进展
solo task swarm status -n <N> -c <channel_id>
# 输出:
#  #1  重构用户系统 (swarm)
#    #2  拆分数据模型          done     (Alice)
#    #3  更新 API Handler      in_progress (Bob)
#    #4  迁移测试              blocked  (waiting #2, #3)
#  Progress: 33% (1/3 complete)
```

### 1.5 并行认领机制

多个 Agent 同时扫描待认领的 swarm 子任务:

```go
// task service 中新增方法
func (s *TaskService) ListClaimableSwarmTasks(ctx context.Context, channelID string) ([]Task, error) {
    // 查询 is_swarm 父任务的子任务中，没有 blocker 且未认领的
    rows, err := s.pool.Query(ctx, `
        SELECT t.* FROM tasks t
        JOIN tasks parent ON t.parent_task_id = parent.id
        WHERE parent.is_swarm = true
          AND t.channel_id = $1
          AND t.status = 'todo'
          AND t.claimer_id IS NULL
          AND NOT EXISTS (
            SELECT 1 FROM task_dependencies td
            JOIN tasks blocker ON td.blocker_task_id = blocker.id
            WHERE td.blocked_task_id = t.id AND blocker.status NOT IN ('done', 'closed')
          )
        ORDER BY t.created_at ASC
    `, channelID)
    // ... scan into tasks
}
```

Agent 认领:

```bash
# Agent 扫描可认领的 swarm 任务
solo task list --swarm-claimable -c <channel_id>

# 认领一个
solo task claim -n 2 -c <channel_id>
```

### 1.6 Swarm 完成检测

```go
// 在 TaskService.UpdateTask 中，当任务变为 'done' 时:
func (s *TaskService) afterTaskCompleted(ctx context.Context, task *Task) {
    // 1. 解除被此任务阻塞的所有任务
    s.pool.Exec(ctx, `DELETE FROM task_dependencies WHERE blocker_task_id = $1`, task.ID)

    // 2. 通知被阻塞任务的 claimer
    blocked, _ := s.depSvc.GetBlocked(ctx, task.ID)
    for _, dep := range blocked {
        s.hub.Broadcast(realtime.Envelope("task_unblocked", map[string]interface{}{
            "blocked_task_id": dep.BlockedTaskID,
            "channel_id":      task.ChannelID,
        }))
    }

    // 3. 如果是 swarm 子任务，检查父任务整体进度
    if task.ParentTaskID != nil && *task.ParentTaskID != "" {
        s.checkSwarmProgress(ctx, *task.ParentTaskID, task.ChannelID)
    }
}

func (s *TaskService) checkSwarmProgress(ctx context.Context, parentTaskID, channelID string) {
    // 计算所有子任务是否完成
    var total, done int
    s.pool.QueryRow(ctx, `SELECT COUNT(*), SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END) FROM tasks WHERE parent_task_id = $1`, parentTaskID).Scan(&total, &done)

    if total > 0 && total == done {
        // 所有子任务完成 → 父任务也标记完成
        s.pool.Exec(ctx, `UPDATE tasks SET status = 'done', updated_at = now() WHERE id = $1`, parentTaskID)
        s.hub.Broadcast(realtime.Envelope("task_completed", map[string]interface{}{
            "task_id":    parentTaskID,
            "channel_id": channelID,
        }))
    }
}
```

---

## 2. 定时唤醒 + 被动跟进

### 2.1 数据库 — reminders 表

**迁移**: `migrations/000033_create_reminders.up.sql`

```sql
CREATE TABLE reminders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel_id      UUID REFERENCES channels(id) ON DELETE CASCADE,
    task_id         UUID REFERENCES tasks(id) ON DELETE CASCADE,
    reminder_type   VARCHAR(30) NOT NULL CHECK (reminder_type IN ('task_deadline', 'stale_task', 'periodic_checkin', 'custom')),
    remind_at       TIMESTAMPTZ NOT NULL,
    message         TEXT NOT NULL,
    is_recurring    BOOLEAN NOT NULL DEFAULT false,
    recurring_rule  VARCHAR(100),  -- cron expression
    is_fired        BOOLEAN NOT NULL DEFAULT false,
    fired_at        TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reminders_pending ON reminders(remind_at) WHERE is_fired = false;
CREATE INDEX idx_reminders_agent ON reminders(agent_id, remind_at);
CREATE INDEX idx_reminders_task ON reminders(task_id);
```

### 2.2 看门狗超时检测 — task_watchdog 表

**迁移**: `migrations/000034_create_task_watchdog.up.sql`

```sql
CREATE TABLE task_watchdog (
    task_id        UUID PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    claimer_id     UUID NOT NULL REFERENCES agents(id),
    claimed_at     TIMESTAMPTZ NOT NULL,
    deadline       TIMESTAMPTZ NOT NULL,      -- 超过此时间未更新 = 超时
    last_activity  TIMESTAMPTZ NOT NULL DEFAULT now(),
    timeout_action VARCHAR(20) NOT NULL DEFAULT 'remind'
                  CHECK (timeout_action IN ('remind', 'escalate', 'unclaim')),
    escalate_to    UUID REFERENCES agents(id), -- 升级目标 agent
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_watchdog_deadline ON task_watchdog(deadline) WHERE deadline < now();
```

### 2.3 Daemon Ticker

**文件**: `cmd/daemon/ticker.go` (新增)

```go
// startTicker 是守护进程的定时扫描循环，每 30 秒检查一次。
func (h *daemonHandler) startTicker(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            h.checkReminders(ctx)
            h.checkWatchdog(ctx)
        case <-ctx.Done():
            return
        }
    }
}

// checkReminders 查询到期的提醒，并构造消息投递给目标 Agent。
func (h *daemonHandler) checkReminders(ctx context.Context) {
    rows, err := h.pool.Query(ctx, `
        SELECT id, agent_id, channel_id, task_id, reminder_type, message, is_recurring, recurring_rule
        FROM reminders
        WHERE remind_at <= now() AND is_fired = false
        LIMIT 50
    `)
    if err != nil { return }
    defer rows.Close()

    for rows.Next() {
        var r Reminder
        rows.Scan(&r.ID, &r.AgentID, &r.ChannelID, &r.TaskID, &r.Type, &r.Message, &r.IsRecurring, &r.RecurringRule)

        // 构造唤醒消息——通过 WebSocket 或 IM channel 投递
        h.deliverReminder(ctx, r)

        // 标记为已触发（非重复的）
        if r.IsRecurring && r.RecurringRule != "" {
            // 计算下次触发时间
            nextTime := computeNextCron(r.RecurringRule)
            h.pool.Exec(ctx, `UPDATE reminders SET remind_at = $1, updated_at = now() WHERE id = $2`, nextTime, r.ID)
        } else {
            h.pool.Exec(ctx, `UPDATE reminders SET is_fired = true, fired_at = now() WHERE id = $1`, r.ID)
        }
    }
}

// checkWatchdog 检查超时的认领任务。
func (h *daemonHandler) checkWatchdog(ctx context.Context) {
    rows, err := h.pool.Query(ctx, `
        SELECT tw.task_id, tw.claimer_id, tw.timeout_action, tw.escalate_to, tw.deadline
        FROM task_watchdog tw
        JOIN tasks t ON tw.task_id = t.id
        WHERE tw.deadline < now() AND t.status NOT IN ('done', 'closed')
    `)
    if err != nil { return }
    defer rows.Close()

    for rows.Next() {
        var taskID, claimerID, timeoutAction, escalateTo string
        var deadline time.Time
        rows.Scan(&taskID, &claimerID, &timeoutAction, &escalateTo, &deadline)

        switch timeoutAction {
        case "remind":
            // 给 claimer 发送提醒
            h.sendAgentMessage(ctx, claimerID, fmt.Sprintf("Task %s is overdue (deadline was %s)", taskID, deadline.Format(time.RFC3339)))

        case "escalate":
            // 通知升级目标
            if escalateTo != "" {
                h.sendAgentMessage(ctx, escalateTo, fmt.Sprintf("Escalation: Task %s assigned to agent %s is overdue", taskID, claimerID))
            }
            // 同时通知 claimer
            h.sendAgentMessage(ctx, claimerID, fmt.Sprintf("Task %s has been escalated due to timeout", taskID))

        case "unclaim":
            // 自动释放认领，让其他人可以领取
            h.pool.Exec(ctx, `UPDATE tasks SET claimer_id = NULL, status = 'todo', updated_at = now() WHERE id = $1`, taskID)
            h.hub.Broadcast(realtime.Envelope("task_unclaimed", map[string]string{"task_id": taskID}))
            // 删除 watchdog 记录
            h.pool.Exec(ctx, `DELETE FROM task_watchdog WHERE task_id = $1`, taskID)
        }
    }
}

func (h *daemonHandler) deliverReminder(ctx context.Context, r Reminder) {
    // 写入消息到 Agent 所在频道或 DM
    if r.TaskID != "" {
        // 通过任务的 thread 发送提醒
        var channelID string
        h.pool.QueryRow(ctx, `SELECT channel_id FROM tasks WHERE id = $1`, r.TaskID).Scan(&channelID)
        h.sendChannelMessage(ctx, channelID, r.AgentID, r.Message)
    } else if r.ChannelID != "" {
        h.sendChannelMessage(ctx, r.ChannelID, r.AgentID, r.Message)
    }
}
```

### 2.4 Ticker 在 daemon 主函数中启动

在 `cmd/daemon/main.go` 中:

```go
func main() {
    // ... 现有初始化代码

    handler := newDaemonHandler(pool, taskMgr, provider, serverURL, internalToken)

    // Start ticker for reminders + watchdog
    go handler.startTicker(context.Background())

    // ... 启动 HTTP server
}
```

---

## 3. CLI — 提醒与看门狗

```bash
# 创建提醒
solo remind create --agent @Alice --at "2026-06-14 09:00" \
    --msg "站会时间" [--recurring "0 9 * * 1-5"]

# 查看提醒
solo remind list [--agent @Alice]

# 删除提醒
solo remind delete <reminder-id>

# 设置任务超时（认领时自动创建 watchdog）
solo task claim -n 5 -c <channel_id> --deadline "2026-06-15" --escalate-to @Bob

# 查看被阻塞/超时的任务
solo task list --stale
```

### 3.1 CLI 实现

```go
// remind 子命令
case "remind":
    handleRemind(args[1:], baseURL, token)
```

```go
func handleRemindCreate(args []string, baseURL, token string) {
    var agentID, at, msg, recurring string
    fs := flag.NewFlagSet("remind create", flag.ExitOnError)
    fs.StringVar(&agentID, "agent", "", "Target agent (@name or ID)")
    fs.StringVar(&at, "at", "", "Remind time (RFC3339)")
    fs.StringVar(&msg, "msg", "", "Reminder message")
    fs.StringVar(&recurring, "recurring", "", "Cron expression for recurring")
    fs.Parse(args)

    reqBody, _ := json.Marshal(map[string]interface{}{
        "agent_id":       resolveAgent(agentID),
        "reminder_type":  "custom",
        "remind_at":      at,
        "message":        msg,
        "is_recurring":   recurring != "",
        "recurring_rule": recurring,
    })
    url := fmt.Sprintf("%s/api/v1/reminders", baseURL)
    statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
    if err != nil || statusCode >= 400 {
        fmt.Fprintf(os.Stderr, "solo: error: %s\n", string(body))
        doExit(exitUsage)
    }
    fmt.Println("Reminder created.")
    doExit(exitOK)
}
```

---

## 4. API 汇总

### 4.1 Swarm

| Method | Path | 说明 |
|--------|------|------|
| POST   | `/api/v1/tasks/{id}/decompose` | 将任务拆分为 swarm 子任务 |
| GET    | `/api/v1/tasks/{id}/swarm-status` | 获取 swarm 执行进度 |

### 4.2 Reminders

| Method | Path | 说明 |
|--------|------|------|
| POST   | `/api/v1/reminders` | 创建提醒 |
| GET    | `/api/v1/reminders` | 列表（?agent_id= & ?status=） |
| DELETE | `/api/v1/reminders/{id}` | 删除提醒 |

### 4.3 Watchdog

| Method | Path | 说明 |
|--------|------|------|
| GET    | `/api/v1/tasks/stale` | 查看所有超时/被阻塞的任务 |

---

## 5. PostgreSQL — 全部新迁移清单

```
000032_add_swarm_fields.up.sql    — tasks.is_swarm + tasks.swarm_plan
000033_create_reminders.up.sql     — reminders 表
000034_create_task_watchdog.up.sql — task_watchdog 表
```

---

## 6. WebSocket 事件

| 事件 | Payload |
|------|---------|
| `swarm_decomposed` | `{ parent_task_id, subtask_count }` |
| `swarm_all_done` | `{ parent_task_id, channel_id }` |
| `reminder_fired` | `{ reminder_id, agent_id, task_id, message }` |
| `task_escalated` | `{ task_id, claimer_id, escalate_to }` |
| `task_unclaimed_auto` | `{ task_id, reason: "timeout" }` |

---

## 7. 实施顺序

```
Phase 4a (2 weeks):
  - reminders 表 + API + daemon ticker
  - 定时提醒功能

Phase 4b (2 weeks):
  - task_watchdog 表 + 超时检测
  - 看门狗: remind → escalate → unclaim

Phase 4c (2 weeks):
  - Swarm 协调器
  - LLM 任务拆分
  - 并行认领 + 自动解除阻塞
```

## 8. 安全与治理

- 任务超时时间由频道 admin 在频道设置中配置（默认 48 小时）
- escalation 链的 `escalates_to` 关系必须在 `agent_relationships` 中预先建立
- Swarm 任务拆分需要消耗 LLM token，按频道限制每日 swarm 创建次数
- cron 表达式解析使用 `robfig/cron` 或手写简单解析器（5-field cron）

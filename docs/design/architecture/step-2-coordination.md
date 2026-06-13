# Step 2 — Coordination + 共享记忆 + 只读图谱

> 依赖 Step 1 | 预估工期: 3-4 weeks
> 新增: CLI delegate 协议、Task DAG CLI + UI、子任务批量拆分、频道共享记忆、SVG 图谱

---

## 1. `solo agent delegate` CLI — 完整实现

### 1.1 委托协议状态机

```
                ┌─────────┐
    create ──→  │ queued  │
                └────┬────┘
                     │ deliver (daemon 投递到目标 Agent 的 System Prompt)
                     ↓
                ┌──────────┐
                │ delivered │
                └────┬─────┘
                ┌────┴─────┐
                ↓          ↓
           ┌─────────┐  ┌──────────┐
           │ started │  │ rejected │  ← reject(reason)
           └────┬────┘  └──────────┘
           ┌────┴────┐
           ↓         ↓
      ┌──────────┐ ┌────────┐
      │ completed │ │ failed │
      └──────────┘ └────────┘
```

### 1.2 CLI 命令

```bash
# Agent Alice 委托任务给 Bob
solo agent delegate --to @Bob --task 3 --msg "请实现 POST /api/login"
# 或带自动唤醒
solo agent delegate --to @Bob --task 3 --msg "..." --start-if-inactive

# Bob 查看待处理委托
solo agent delegate list [--status queued|delivered|started]

# Bob 接受/拒绝
solo agent delegate accept <delegation-id>
solo agent delegate reject <delegation-id> --reason "不擅长这块"

# Bob 标记完成
solo agent delegate complete <delegation-id>
```

### 1.3 CLI 实现 (`cmd/solo/main.go` 新增 `handleDelegate`)

```go
func handleDelegate(args []string, baseURL, token string) {
    agentID := os.Getenv("SOLO_AGENT_ID")
    if agentID == "" {
        fmt.Fprintln(os.Stderr, "solo: error: SOLO_AGENT_ID not set")
        doExit(exitUsage)
    }

    if len(args) < 1 {
        fmt.Fprintln(os.Stderr, "solo: error: delegate subcommand required")
        printUsage()
        doExit(exitUsage)
    }

    switch args[0] {
    case "list":
        handleDelegateList(args[1:], baseURL, token, agentID)
    case "accept", "reject", "start", "complete", "fail":
        handleDelegateTransition(args[0], args[1:], baseURL, token, agentID)
    default:
        // --to @Bob --task 3 --msg "..." --start-if-inactive
        handleDelegateCreate(args, baseURL, token, agentID)
    }
}

func handleDelegateCreate(args []string, baseURL, token, agentID string) {
    var toAgent, channelID, msg string
    var taskNumber int
    var startIfInactive bool

    fs := flag.NewFlagSet("delegate", flag.ExitOnError)
    fs.StringVar(&toAgent, "to", "", "Target agent (@name or ID)")
    fs.IntVar(&taskNumber, "task", 0, "Task number")
    fs.StringVar(&taskNumberAlias, "n", 0, "Task number (alias)")
    fs.StringVar(&channelID, "c", "", "Channel ID")
    fs.StringVar(&msg, "msg", "", "Delegation message")
    fs.BoolVar(&startIfInactive, "start-if-inactive", false, "Wake target agent if inactive")
    fs.Parse(args)

    if toAgent == "" || channelID == "" {
        fmt.Fprintln(os.Stderr, "solo: error: --to and -c are required")
        doExit(exitUsage)
    }

    resolvedChannel, err := resolveChannelParam(baseURL, token, channelID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
        doExit(exitBusiness)
    }

    // 解析 to agent（支持 @name 简写）
    toAgentID, err := resolveAgentParam(baseURL, token, toAgent)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
        doExit(exitBusiness)
    }

    // 如果有 task number，先解析成 task_id
    taskID := ""
    if taskNumber > 0 {
        taskID = strconv.Itoa(taskNumber)
    }

    reqBody, _ := json.Marshal(map[string]interface{}{
        "to_agent_id":       toAgentID,
        "task_id":           taskID,
        "channel_id":        resolvedChannel,
        "message":           msg,
        "start_if_inactive": startIfInactive,
    })

    url := fmt.Sprintf("%s/api/v1/agents/%s/delegations", baseURL, agentID)
    statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: request failed: %v\n", err)
        doExit(exitUsage)
    }
    if statusCode >= 400 {
        handleNonProxyHTTPError(statusCode, body)
    }

    fmt.Println("Delegation created successfully.")
    doExit(exitOK)
}
```

### 1.4 投递机制 — daemon 注入 System Prompt

当受委托 Agent 在频道内被唤醒时，daemon 查询待处理委托并注入 System Prompt:

```go
// daemon 在构造 Agent System Prompt 时调用
func (h *daemonHandler) injectPendingDelegations(ctx context.Context, agentID, channelID string) string {
    rows, err := h.pool.Query(ctx, `
        SELECT d.id, a.name, d.message, d.task_id
        FROM agent_delegations d
        JOIN agents a ON d.from_agent_id = a.id
        WHERE d.to_agent_id = $1 AND d.channel_id = $2 AND d.status IN ('queued', 'delivered')
        ORDER BY d.created_at ASC
    `, agentID, channelID)
    if err != nil { return "" }
    defer rows.Close()

    var sb strings.Builder
    for rows.Next() {
        var id, fromName, msg, taskID string
        rows.Scan(&id, &fromName, &msg, &taskID)
        sb.WriteString(fmt.Sprintf(
            "[DELEGATION %s] From @%s: %s",
            id[:8], fromName, msg,
        ))
        if taskID != "" {
            sb.WriteString(fmt.Sprintf(" (task: %s)", taskID))
        }
        sb.WriteString("\n")
        // 标记为 delivered
        h.pool.Exec(ctx, `UPDATE agent_delegations SET status = 'delivered', updated_at = now() WHERE id = $1`, id)
    }
    return sb.String()
}
```

---

## 2. Task DAG — CLI + UI

### 2.1 CLI 扩展

```bash
# 创建任务时指定依赖
solo task create -c <channel_id> --title "部署到生产" --depends-on 5

# 查看被阻塞的任务
solo task list --blocked

# 批量设置依赖
solo task block -n 5 -c <channel_id> --on 3
solo task unblock -n 5 -c <channel_id> --on 3
```

### 2.2 `CreateTaskRequest` 扩展

```go
type CreateTaskRequest struct {
    Title        string     `json:"title"`
    Description  string     `json:"description,omitempty"`
    Priority     string     `json:"priority,omitempty"`
    DueDate      *time.Time `json:"due_date,omitempty"`
    ChannelID    string     `json:"channel_id,omitempty"`
    ParentTaskID string     `json:"parent_task_id,omitempty"`
    DependsOn    []string   `json:"depends_on,omitempty"`     // NEW: Task IDs to depend on
}
```

### 2.3 TaskService.CreateTask 扩展

在 `internal/server/service/task.go` 的 `CreateTask` 末尾追加:

```go
// 创建依赖关系
if len(req.DependsOn) > 0 {
    depSvc := NewTaskDependencyService(s.pool)
    for _, blockerID := range req.DependsOn {
        _, err := depSvc.Block(ctx, id, blockerID) // 新任务被 blockerID 阻塞
        if err != nil {
            slog.Warn("failed to create dependency", "blocked", id, "blocker", blockerID, "error", err)
        }
    }
}
```

### 2.4 TaskListResponse 阻塞信息嵌入

在 TaskResponse 和 `Task` struct 中新增:

```go
type TaskResponse struct {
    // ... 已有字段
    BlockerIDs       []string `json:"blocker_ids,omitempty"`        // 阻塞我的 task IDs
    BlockedByCount   int      `json:"blocked_by_count,omitempty"`   // 阻塞我的未完成任务数
}
```

`GetTask` 的 SQL 扩展:

```sql
SELECT ...,
    (SELECT COUNT(*) FROM task_dependencies td
       JOIN tasks t2 ON td.blocker_task_id = t2.id
       WHERE td.blocked_task_id = t.id AND t2.status NOT IN ('done','closed')
    ) AS blocked_by_count
FROM tasks t ...
```

### 2.5 Kanban 前端 — 被阻塞卡片

在 `TaskBoard` 组件中检测 `blocked_by_count > 0`，渲染时:

```tsx
// 在 task-card 组件中
{task.blocked_by_count > 0 && (
  <div className="task-blocked-badge">
    <LockIcon size={12} />
    等待 {task.blocked_by_count} 个依赖完成
  </div>
)}
```

前端 WebSocket 监听 `task_unblocked`:

```typescript
// 在 use-tasks.ts hook 中
ws.on('task_unblocked', (data: { blocked_task_id: string }) => {
  // 刷新对应 task，更新 blocker 状态
  refreshTask(data.blocked_task_id);
});
```

---

## 3. 子任务批量拆分

### 3.1 CLI

```bash
# 批量创建子任务
solo task split -n 5 -c <channel_id> --titles "A,B,C"

# 等价于创建 3 个子任务，parent_task_id = task #5 的 ID
```

### 3.2 实现 (`cmd/solo/main.go`)

```go
case "split":
    handleTaskSplit(args[1:], baseURL, token)
```

```go
func handleTaskSplit(args []string, baseURL, token string) {
    var channelID string
    var number int
    var titles string
    fs := flag.NewFlagSet("task split", flag.ExitOnError)
    fs.StringVar(&channelID, "c", "", "Channel ID or #name")
    fs.IntVar(&number, "n", 0, "Parent task number")
    fs.StringVar(&titles, "titles", "", "Comma-separated subtask titles")
    fs.Parse(args)

    if channelID == "" || number <= 0 || titles == "" {
        fmt.Fprintln(os.Stderr, "solo: error: -c, -n, and --titles are required")
        doExit(exitUsage)
    }

    resolved, _ := resolveChannelParam(baseURL, token, channelID)

    // 先获取父任务的 UUID
    parentTask, err := fetchTaskByNumber(baseURL, token, resolved, number)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: %v\n", err)
        doExit(exitBusiness)
    }

    titleList := strings.Split(titles, ",")
    for _, t := range titleList {
        t = strings.TrimSpace(t)
        if t == "" { continue }
        reqBody, _ := json.Marshal(map[string]string{
            "title":          t,
            "channel_id":     resolved,
            "parent_task_id": parentTask.ID,
        })
        url := fmt.Sprintf("%s/api/v1/channels/%s/tasks", baseURL, resolved)
        statusCode, body, err := doHTTP(http.MethodPost, url, token, reqBody)
        if err != nil || statusCode >= 400 {
            fmt.Fprintf(os.Stderr, "solo: error creating subtask %q: %s\n", t, string(body))
        } else {
            fmt.Printf("Created subtask: %s\n", t)
        }
    }
    doExit(exitOK)
}
```

### 3.3 父任务进度自动计算

在 `TaskService.GetTask` 中已有 `subtask_count` 和 `done_subtask_count` 子查询。无需额外改动。前端 Kanban 中显示进度条:

```tsx
{subtask_count > 0 && (
  <ProgressBar value={done_subtask_count} max={subtask_count} />
)}
```

---

## 4. 频道共享记忆

### 4.1 文件结构

```
~/.solo/channels/<channel-id>/
  └── memory/
      ├── CHANNEL.md     # 频道上下文（所有 Agent 可读写，启动时自动加载）
      └── decisions.md   # 频道内的技术决策记录（追加式写入）
```

### 4.2 Memory 访问控制

Channel members 表中 `member_type = 'agent'` 的 Agent 自动获得读写权。

### 4.3 Daemon 集成 — 启动时加载

在 `daemonHandler` 构造 Agent Prompt 时:

```go
func (h *daemonHandler) loadChannelMemory(channelID string) string {
    path := filepath.Join(os.Getenv("HOME"), ".solo", "channels", channelID, "memory", "CHANNEL.md")
    data, err := os.ReadFile(path)
    if err != nil {
        return ""
    }
    return fmt.Sprintf("\n## Channel Shared Memory\n\n%s\n", string(data))
}
```

### 4.4 Server 端 API — 内存读写

| Method | Path | 说明 |
|--------|------|------|
| GET    | `/api/v1/channels/{channelID}/memory` | 获取 CHANNEL.md 内容 |
| PUT    | `/api/v1/channels/{channelID}/memory` | 覆盖写入 CHANNEL.md |
| POST   | `/api/v1/channels/{channelID}/memory/decisions` | 追加 decisions.md |
| GET    | `/api/v1/channels/{channelID}/memory/decisions` | 获取 decisions.md |

### 4.5 Handler 实现

**文件**: `internal/server/handler/channel_memory.go`

```go
type ChannelMemoryHandler struct {
    pool *pgxpool.Pool
}

func (h *ChannelMemoryHandler) GetMemory(w http.ResponseWriter, r *http.Request) {
    channelID := chi.URLParam(r, "channelID")
    uid, _ := requireUserID(r)

    // 权限检查: 必须是频道成员
    if !isChannelMember(r.Context(), h.pool, channelID, uid) {
        writeError(w, http.StatusForbidden, "not a channel member")
        return
    }

    path := filepath.Join(memoryRoot(), channelID, "memory", "CHANNEL.md")
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        writeJSON(w, http.StatusOK, map[string]string{"content": ""})
        return
    }
    if err != nil {
        writeError(w, http.StatusInternalServerError, "read failed")
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"content": string(data)})
}

func memoryRoot() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".solo", "channels")
}
```

### 4.6 CLI 扩展

```bash
# 读取频道共享上下文
solo channel memory -c <channel_id>

# 写入频道上下文
solo channel memory set -c <channel_id> -f CHANNEL.md
```

### 4.7 与已有 Phase 0 个人笔记的区别

| 维度 | Phase 0 个人笔记 | 频道共享记忆 |
|------|----------------|--------------|
| 路径 | `~/.solo/channels/#<name>/notes/` | `~/.solo/channels/<id>/memory/` |
| 作用域 | 单个 Agent 的视角 | 频道内所有 Agent 共享 |
| 加载时机 | Agent 被 @mention 时 | Agent 在频道内被唤醒时 |
| 写入权 | 仅当前 Agent | 所有频道内 Agent |

---

## 5. 只读 SVG 关系图谱

### 5.1 设计约束

- 纯前端实现（约 150 行 JS）
- 零新 npm 依赖
- SVG tree layout，从 `GET /api/v1/relationships/graph` 获取数据
- 4 种关系用不同线型区分
- 节点可点击跳转 AgentDetail
- 支持 pan / zoom (使用 SVG `viewBox` + `transform`)
- 验证 "关系可视化有没有价值"

### 5.2 SVG 线型定义

```
reports_to:      实线 + 竖线（层级关系）  stroke="#4A90D9" stroke-dasharray="none"
delegates_to:    实线 + 横线  stroke="#7B6CF6" 
collaborates_with: 虚线  stroke="#10B981" stroke-dasharray="6,4"
escalates_to:    双实线 + 红色  stroke="#EF4444" stroke-width="2"
```

### 5.3 布局算法

逐级放置（hierarchical layout）:

```
1. BFS 从没有 reports_to 上级的节点开始（root nodes）
2. 每层 y += 80, 每节点 x += 200
3. collaborates_with 边低于同级节点之间
4. 使用 markdown arrowhead 在边上标注类型
```

### 5.4 组件位置

**文件**: `frontend/components/agents/relationship-graph.tsx`

```tsx
interface GraphNode {
  agent_id: string;
  agent_name: string;
  x: number;
  y: number;
}

interface GraphEdge {
  from: string;
  to: string;
  type: RelationshipType;
}

function RelationshipGraph({ relationships }: { relationships: AgentRelationship[] }) {
  const [nodes, edges] = useMemo(() => layoutGraph(relationships), [relationships]);
  const [viewBox, setViewBox] = useState({ x: 0, y: 0, w: 800, h: 600 });
  const [drag, setDrag] = useState<{ sx: number; sy: number; vx: number; vy: number } | null>(null);
  const router = useRouter();

  return (
    <svg
      viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.w} ${viewBox.h}`}
      onMouseDown={(e) => setDrag({ sx: e.clientX, sy: e.clientY, vx: viewBox.x, vy: viewBox.y })}
      onMouseMove={(e) => {
        if (!drag) return;
        setViewBox({ ...viewBox, x: drag.vx - (e.clientX - drag.sx), y: drag.vy - (e.clientY - drag.sy) });
      }}
      onMouseUp={() => setDrag(null)}
      onWheel={(e) => {
        e.preventDefault();
        const scale = e.deltaY > 0 ? 1.1 : 0.9;
        setViewBox({ ...viewBox, w: viewBox.w * scale, h: viewBox.h * scale });
      }}
    >
      {/* 边 */}
      {edges.map((e, i) => (
        <line key={i} x1={e.from.x} y1={e.from.y} x2={e.to.x} y2={e.to.y}
              stroke={edgeColor(e.type)} strokeWidth={e.type === 'escalates_to' ? 2 : 1}
              strokeDasharray={e.type === 'collaborates_with' ? '6,4' : 'none'} />
      ))}
      {/* 节点 */}
      {nodes.map((n) => (
        <g key={n.agent_id} onClick={() => router.push(`/agents/${n.agent_id}`)}>
          <circle cx={n.x} cy={n.y} r={24} fill="#1E293B" stroke="#4A90D9" strokeWidth={2} />
          <text x={n.x} y={n.y + 4} textAnchor="middle" fill="white" fontSize={10}>{n.agent_name[0]}</text>
          <text x={n.x} y={n.y + 40} textAnchor="middle" fill="#94A3B8" fontSize={11}>{n.agent_name}</text>
        </g>
      ))}
    </svg>
  );
}
```

### 5.5 前端路由

在 Agent 列表页面 `frontend/app/agents/page.tsx`（如已有）添加 tab: **关系图谱**。

或在 Dashboard 页面 `/frontend/app/dashboard/page.tsx` 中增加一个图表面板。

---

## 6. WebSocket 事件矩阵

| 事件 | 触发时机 | Payload |
|------|---------|---------|
| `delegation_created` | 新委托创建 | `{ delegation_id, from, to, task_id }` |
| `delegation_updated` | 状态变更 | `{ delegation_id, new_status }` |
| `task_blocked` | 新依赖创建 | `{ blocked_task_id, blocker_task_id }` |
| `task_unblocked` | blocker 完成 | `{ blocked_task_id, channel_id }` |
| `memory_updated` | CHANNEL.md 被修改 | `{ channel_id }` |

---

## 7. 迁移（无新表，Step 1 表已创建）

Step 2 不引入新数据库表。仅修改已有表:
- `tasks` 表的 `GetTask` SQL 新增 `blocker_count` 子查询

如需要，在 `000030` 中可添加优化索引:

```sql
-- 000030_add_blocker_optimization.up.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_task_dependencies_blocker_status
    ON task_dependencies(blocker_task_id)
    INCLUDE (blocked_task_id);
```

---

## 8. 集成检查列表

- [ ] CLI: `solo agent delegate --to` / `list` / `accept` / `reject` / `complete`
- [ ] CLI: `solo task block --on` / `unblock --on` / `list --blocked`
- [ ] CLI: `solo task split -n <N> --titles "A,B,C"`
- [ ] CLI: `solo channel memory` / `memory set`
- [ ] Server: Channel memory endpoints
- [ ] Server: Task Get 返回 `blocker_ids` + `blocked_by_count`
- [ ] Server: Delegation 创建后 WebSocket 广播
- [ ] Server: Task blocker 完成时 WebSocket 通知 blocked task claimer
- [ ] Daemon: 唤醒时注入 pending delegations
- [ ] Daemon: 启动时加载 CHANNEL.md
- [ ] Frontend: SVG 关系图组件
- [ ] Frontend: Kanban 被阻塞卡片 UI
- [ ] Frontend: 子任务进度条

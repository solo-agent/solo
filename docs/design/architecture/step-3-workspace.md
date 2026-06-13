# Step 3 — Workspace: 多层工作区

> 依赖 Step 1-2 | 预估工期: 2-3 weeks
> 新增: 频道共享工作区、项目仓库绑定、Git worktree 任务隔离

---

## 1. 架构总览

```
~/.solo/
├── agents/<agent-id>/
│   ├── workspace/              ✅ 已有 — Agent 个人工作区
│   │   ├── repo/               git clone 副本
│   │   └── notes/
│   └── worktrees/              ❌ 新增 — 临时隔离工作区
│       └── task-<n>/           task 的独立 worktree
│
├── channels/<channel-id>/
│   ├── workspace/              ❌ 新增 — 频道共享工作区
│   │   ├── repo/               git clone（频道绑定项目）
│   │   │   └── .solo/
│   │   │       └── channel-context.md
│   │   └── .solo/
│   │       └── bind.json       绑定配置
│   └── memory/                 ✅ Step 2 创建
│       ├── CHANNEL.md
│       └── decisions.md
│
└── attachments/                ✅ 已有
```

---

## 2. 频道共享工作区

### 2.1 数据库 — channel_bindings 表

**迁移**: `migrations/000030_create_channel_bindings.up.sql`

```sql
CREATE TABLE channel_bindings (
    channel_id   UUID PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    repo_url     TEXT NOT NULL,
    repo_branch  VARCHAR(200) NOT NULL DEFAULT 'main',
    bind_path    TEXT NOT NULL,  -- 在 .solo/channels/<id>/workspace/ 下的相对路径
    bound_by     UUID NOT NULL REFERENCES users(id),
    bound_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Go struct**:

```go
type ChannelBinding struct {
    ChannelID  string    `json:"channel_id"`
    RepoURL    string    `json:"repo_url"`
    RepoBranch string    `json:"repo_branch"`
    BindPath   string    `json:"bind_path"`
    BoundBy    string    `json:"bound_by"`
    BoundAt    time.Time `json:"bound_at"`
}
```

### 2.2 API

| Method | Path | 说明 |
|--------|------|------|
| POST   | `/api/v1/channels/{channelID}/bind` | 绑定项目仓库 |
| GET    | `/api/v1/channels/{channelID}/bind` | 获取绑定信息 |
| DELETE | `/api/v1/channels/{channelID}/bind` | 解绑（不删除本地文件） |
| GET    | `/api/v1/channels/{channelID}/workspace` | 浏览共享工作区文件树 |

#### POST /api/v1/channels/{channelID}/bind

```json
// Request
{
    "repo_url": "https://github.com/org/repo",
    "repo_branch": "main"
}

// Response 201
{
    "channel_id": "uuid",
    "repo_url": "https://github.com/org/repo",
    "repo_branch": "main",
    "bind_path": "~/.solo/channels/<id>/workspace/repo",
    "bound_by": "user-uuid",
    "bound_at": "2026-06-13T10:00:00Z"
}
```

#### GET /api/v1/channels/{channelID}/workspace?path=&depth=3

```json
// Response 200
{
    "root": "/Users/xxx/.solo/channels/<id>/workspace/repo",
    "files": [
        {"name": "src", "type": "directory", "children": [...]},
        {"name": "README.md", "type": "file", "size": 1024}
    ]
}
```

### 2.3 Service

**文件**: `internal/server/service/channel_workspace.go`

```go
type ChannelWorkspaceService struct {
    pool *pgxpool.Pool
}

func NewChannelWorkspaceService(pool *pgxpool.Pool) *ChannelWorkspaceService {
    return &ChannelWorkspaceService{pool: pool}
}

// BindProject 将项目仓库绑定到频道。
func (s *ChannelWorkspaceService) BindProject(ctx context.Context, channelID, repoURL, branch, userID string) (*ChannelBinding, error) {
    // 1. 权限检查: 必须是频道 owner/admin
    if !s.isChannelAdmin(ctx, channelID, userID) {
        return nil, errors.New("only channel admin can bind a project")
    }

    // 2. 检查是否已绑定
    var existing bool
    s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channel_bindings WHERE channel_id = $1)`, channelID).Scan(&existing)
    if existing {
        return nil, errors.New("channel already has a project binding")
    }

    // 3. 插入绑定记录（同步）
    bindPath := filepath.Join(channelWorkspaceRoot(channelID), "repo")
    _, err := s.pool.Exec(ctx, `
        INSERT INTO channel_bindings (channel_id, repo_url, repo_branch, bind_path, bound_by)
        VALUES ($1, $2, $3, $4, $5)
    `, channelID, repoURL, branch, bindPath, userID)
    if err != nil { return nil, err }

    // 4. 异步执行 git clone（避免阻塞 HTTP 请求）
    go s.cloneRepo(channelID, repoURL, branch, bindPath)

    return &ChannelBinding{
        ChannelID: channelID, RepoURL: repoURL, RepoBranch: branch,
        BindPath: bindPath, BoundBy: userID, BoundAt: time.Now(),
    }, nil
}

// cloneRepo 异步执行 git clone，结果写入 channel-context.md。
func (s *ChannelWorkspaceService) cloneRepo(channelID, repoURL, branch, bindPath string) {
    // 确保父目录存在
    os.MkdirAll(filepath.Dir(bindPath), 0755)

    cmd := exec.Command("git", "clone", "--branch", branch, "--single-branch", repoURL, bindPath)
    output, err := cmd.CombinedOutput()
    if err != nil {
        slog.Error("git clone failed", "channel_id", channelID, "error", err, "output", string(output))
        // 写入错误信息供查看
    } else {
        slog.Info("repo cloned", "channel_id", channelID, "path", bindPath)
    }
}
```

### 2.4 CLI

```bash
# 管理员绑定项目
solo channel bind --target '#frontend' --repo 'https://github.com/org/repo'

# 查看绑定
solo channel bind info --target '#frontend'

# 解绑
solo channel unbind --target '#frontend'

# Agent 浏览共享工作区
solo channel workspace --target '#frontend' [--path src/]
```

CLI 实现:

```go
case "bind":
    handleChannelBind(args[1:], baseURL, token)
case "workspace":
    handleChannelWorkspace(args[1:], baseURL, token)
```

---

## 3. Git Worktree 任务隔离

### 3.1 设计原则

- **非强制执行** — 多数任务不需要隔离
- **触发条件**:
  1. Agent 主动请求: `solo task isolate -n <N>`
  2. 自动触发: daemon 检测到多 Agent 同时修改同一文件时
- **生命周期**:
  ```
  create worktree → 任务执行 → merge → cleanup
  ```

### 3.2 工作流

```bash
# Agent Alice 认领 Task #3 后
solo task isolate -n 3 -c <channel_id>
# → 在 ~/.solo/agents/<alice-id>/worktrees/task-3/ 创建 worktree
# → 基于频道的共享 repo 创建

# 任务完成后
solo task isolate done -n 3 -c <channel_id>
# → git commit + push → merge 回频道 repo → 删除 worktree
```

### 3.3 CLI 实现

```go
func handleTaskIsolate(args []string, baseURL, token string) {
    var channelID string
    var number int
    var action string
    fs := flag.NewFlagSet("task isolate", flag.ExitOnError)
    fs.StringVar(&channelID, "c", "", "Channel ID")
    fs.IntVar(&number, "n", 0, "Task number")
    fs.Parse(args)

    if len(fs.Args()) > 0 {
        action = fs.Arg(0) // "done" 或空（start）
    }

    // 获取 channel binding
    binding, err := fetchChannelBinding(baseURL, token, channelID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: channel not bound to a repo\n")
        doExit(exitBusiness)
    }

    worktreePath := filepath.Join(agentWorktreeRoot(), fmt.Sprintf("task-%d", number))

    switch action {
    case "done":
        // merge + cleanup
        cleanupWorktree(worktreePath, binding.RepoURL)
    default:
        // create worktree
        createWorktree(binding.BindPath, worktreePath, number)
    }
}

func createWorktree(repoPath, worktreePath string, taskNumber int) {
    branchName := fmt.Sprintf("solo/task-%d-%s", taskNumber, time.Now().Format("20060102-150405"))
    cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, "main")
    cmd.Dir = repoPath
    if err := cmd.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "solo: error: git worktree add failed: %v\n", err)
        os.Exit(exitUsage)
    }
    fmt.Printf("Worktree created at %s (branch: %s)\n", worktreePath, branchName)
}

func cleanupWorktree(worktreePath, repoURL string) {
    // 1. git add + commit（如果未提交）
    // 2. git push origin <branch>
    // 3. git worktree remove <path>
    // 4. git branch -D <branch>（本地）
}
```

### 3.4 自动冲突检测

Daemon 在执行任务前扫描频道 repo 的 `git status`，如果发现有其他 Agent 的 worktree 修改了相同文件，通过 WebSocket 提醒:

```go
func (h *daemonHandler) detectConflicts(repoPath string, myFiles []string) []string {
    // git worktree list → 查找其他 active worktrees
    output, _ := exec.Command("git", "worktree", "list", "--porcelain").Output()
    // 解析 worktrees，对比正在修改的文件
    // 返回冲突文件列表
}
```

WebSocket 事件:

```json
{
    "type": "workspace_conflict",
    "payload": {
        "channel_id": "uuid",
        "task_id": "uuid",
        "conflicting_agent": "Bob",
        "files": ["src/handler/task.go"]
    }
}
```

---

## 4. 频道共享工作区 + Agent 交互协议

### 4.1 Agent 在频道工作区执行命令

当 Agent 在频道内被唤醒时，daemon 自动将 CWD 设置为频道共享 repo:

```go
func (h *daemonHandler) resolveWorkingDirectory(agentID, channelID string) string {
    // 1. 优先检查是否有 task worktree
    activeTask := h.taskManager.GetActiveTask(agentID)
    if activeTask != nil {
        worktreePath := filepath.Join(agentWorktreeRoot(), fmt.Sprintf("task-%d", activeTask.Number))
        if _, err := os.Stat(worktreePath); err == nil {
            return worktreePath
        }
    }

    // 2. 检查频道绑定
    var bindPath string
    err := h.pool.QueryRow(context.Background(),
        `SELECT bind_path FROM channel_bindings WHERE channel_id = $1`, channelID,
    ).Scan(&bindPath)
    if err == nil && bindPath != "" {
        if _, err := os.Stat(bindPath); err == nil {
            return bindPath
        }
    }

    // 3. 回退到 Agent 个人工作区
    return filepath.Join(os.Getenv("HOME"), ".solo", "agents", agentID, "workspace")
}
```

---

## 5. 数据流

```
用户绑定项目:
  POST /api/v1/channels/{id}/bind
    → ChannelWorkspaceService.BindProject()
      → INSERT channel_bindings
      → goroutine: git clone → channel workspace

Agent 认领任务:
  solo task claim -n 3 -c <channel>
    → (if bound) daemon 提示: "频道已绑定 repo，是否创建隔离 worktree?"

Agent 执行任务:
  daemon 设置 CWD = worktree (if exists) || channel repo || agent workspace
  → Agent 修改文件

Agent 完成任务:
  solo task isolate done -n 3
  → git commit + push (worktree)
  → 清理 worktree
  → task 状态更新
```

---

## 6. HTTP API 汇总

| Method | Path | 说明 | 权限 |
|--------|------|------|------|
| POST | `/api/v1/channels/{id}/bind` | 绑定 repo | 频道 admin |
| GET | `/api/v1/channels/{id}/bind` | 查看绑定 | 频道成员 |
| DELETE | `/api/v1/channels/{id}/bind` | 解绑 | 频道 admin |
| GET | `/api/v1/channels/{id}/workspace` | 浏览文件树 | 频道成员 |
| POST | `/api/v1/tasks/{id}/isolate` | 创建隔离 worktree | 任务 claimer |
| DELETE | `/api/v1/tasks/{id}/isolate` | 清理 worktree | 任务 claimer |

---

## 7. 迁移

```
000030_create_channel_bindings.up.sql
000030_create_channel_bindings.down.sql
```

---

## 8. 约束

- 一个频道最多绑定一个项目仓库
- Worktree 生命周期绑定到任务，任务完成后 24 小时内自动清理
- 共享工作区 repo 的修改需要 channel admin 审核（建议通过 PR 流程，自动 PR 后续 Phase 实现）
- 不强制使用 worktree — `solo task isolate` 是可选命令

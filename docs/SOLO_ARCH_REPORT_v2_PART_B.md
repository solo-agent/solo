# Solo 产品能力技术架构总结（PART B）

> 对应任务：`arch-product` — Solo Inbox / Channel·DM·Thread / Task / Workspace / Skill / 灵动岛 6 大产品能力的技术实现。
> 所有声明均带代码引用（`file:line`），行号从源码读出并经 grep 自检。

---

## 1. Inbox — 三路聚合的未读中心

Inbox 是 Solo v1.5 才落地的产品（`migrations/000022/000024/000025` 三个新增表专门支撑），定位是「Thread Replies / DM / @Mentions」三类通知的单一入口。

### 1.1 数据模型

3 张表分工清晰：

- `user_inbox_state`（`migrations/000022_create_user_inbox_state.up.sql:1-8`）— `user_id` PK + `last_read_at`。注释说 "for unread-count calculation"，但 service 里实际用的是 `cleared_before`（`service/inbox.go:53`），迁移与代码不同步。
- `user_mentions`（`migrations/000024_create_user_mentions.up.sql:4-11`）— `(message_id, mentioned_user_id)` 复合 PK + `idx_user_mentions_user (mentioned_user_id, created_at DESC)`。@mention 解析在消息发送时落表，避免 inbox 阶段 ILIKE 扫表。
- `user_inbox_reads`（`migrations/000025_create_inbox_reads.up.sql:4-9`）— 逐消息的"已读"位图，主键 `(user_id, message_id)`，每行 = 用户已读这条消息。

### 1.2 三路 UNION 核心查询

`InboxService.List`（`service/inbox.go:46-209`）是 Inbox 的核心，三条 SELECT 走 `UNION ALL`，统一从 `messages` 表 + 各自上下文拉数据：

| 分支 | 行号 | 限定条件 | 关键过滤 |
|---|---|---|---|
| thread_reply | `service/inbox.go:63-103` | `m.thread_id IS NOT NULL` + `t.channel_id IN (用户曾发 thread 消息的频道)` | `c.type != 'dm'` |
| dm | `service/inbox.go:107-137` | `c.type = 'dm'` + `dm_members.member_id = $1` | `m.thread_id IS NULL` |
| mention | `service/inbox.go:142-171` | `JOIN user_mentions um ON um.message_id = m.id AND um.mentioned_user_id = $1` | `c.type != 'dm'` |

未读通过 `LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id` + `r.message_id IS NULL AS is_unread` 标记（`service/inbox.go:74/87/118/128/153/163`）。三种类型在同一次查询里按 `created_at DESC` 合并后再 `LIMIT $4` 截断（`service/inbox.go:173-175`），所以 cursor 分页是**跨三路**的——一页里可能同时有 thread reply / dm / mention。

### 1.3 游标分页 + 过滤

入参 `before`（RFC3339 时间戳）+ `limit`（默认 30，上限 50，handler `handler/inbox.go:40-44`）+ `types`（`thread_reply,dm,mention` 逗号串，`handler/inbox.go:47-55`）+ `sender`（ILIKE 模糊匹配）。`service/inbox.go:200-203` 用 `limit+1` 经典模式算 `has_more`，前端 `loadMore` 把最后一条 `created_at` 当 cursor 再发请求（`lib/hooks/use-inbox.ts:67-73`）。

`cleared_before` 是「清空 inbox 后的水位线」（`service/inbox.go:293-304` 的 `ClearAll`），`List` 拿它当 `m.created_at > $3` 下界（`service/inbox.go:99/135/168`），等价于"清空之后产生的新消息"。

### 1.4 实时同步

`useInbox`（`lib/hooks/use-inbox.ts:83-90`）和 `useInboxUnread`（`lib/hooks/use-inbox-unread.ts:50-57`）都订阅 WS 事件 `inbox.updated`，事件由 server 在 message 落库时广播（`ws/hub.go:461/735/781`，`ws/message.go:75` 定义常量 `EventInboxUpdated = "inbox.updated"`）。同时 `use-inbox-unread.ts:60-66` 监听 `window focus`——窗口回到前台再 fetch 一次。

### 1.5 UI

`inbox-view.tsx`（234 行）四列 Tab：`all / mention / thread_reply / dm`（`inbox-view.tsx:18-23`），点中时通过 `KEY_TO_TYPE_FILTER` 映射回 `types` 参数（`inbox-view.tsx:25-30`）。`InboxBadge`（`inbox-badge.tsx:35-44`）根据 `useInboxUnread` 渲染红底数字 + bounce-slow 动画，未读>99 显示 `99+`。`InboxItem`（`inbox-item.tsx:33-96`）三色 tag 区分 `thread_reply` / `dm` / `mention`，未读时 `border-l-[3px] border-l-brutal-accent`（`inbox-item.tsx:51`）。

### 代码证据
- 三路 UNION + 游标分页：`internal/server/service/inbox.go:57-175`
- 未读 = `user_inbox_reads` 反向 LEFT JOIN：`internal/server/service/inbox.go:74, 118, 153, 212`
- 实时刷新 + 焦点刷新：`frontend/lib/hooks/use-inbox-unread.ts:50-66`
- inbox WS 事件常量：`internal/server/ws/message.go:75`；广播点：`internal/server/ws/hub.go:461, 735, 781`

---

## 2. Channel · DM · Thread — 共表·分型·权限三件套

### 2.1 共表分型

Channel 和 DM 共享 `channels` 表，靠 `type` 区分（'channel' / 'dm'），DM 额外用 `dm_members` 表做"双方"约束。证据：

- `handler/dm.go:206-210` 创建 DM 时 `INSERT INTO channels (..., type, ...) VALUES (..., 'dm', ...)`
- `service/channel.go:81` 创建普通 channel 时硬编码 `type='channel'`
- DM 双向去重查询（`handler/dm.go:157-166`）：`JOIN dm_members dm1 ON dm1.channel_id = dm2.channel_id` + `dm1.member_id = $1 AND dm2.member_id = $2` —— 任何方向创建都返回同一个 channel
- Thread 不在 `channels` 表，单独 `threads` 表，`root_message_id` 唯一（`service/thread.go:89-91` + `ON CONFLICT (root_message_id) DO UPDATE` 创建模式，`service/thread.go:103-108`）

### 2.2 权限模型

- `Member.Role`：`owner` / `admin` / `member` 三级（`service/channel.go:35-43`）
- 添加成员：requester 必须是 owner 或 admin（`service/channel.go:113-127`）
- 移除成员：自己退出不需要权限；移除他人需 owner/admin（`service/channel.go:200-217`）；owner/admin 互踢需要 owner 角色（`service/channel.go:219-222`）
- 软删除：`handler/channel.go:457-466` 的 `DELETE /api/v1/channels/{id}` 实际是 `UPDATE channels SET is_archived = true`，列表/详情查询都过滤 `is_archived = false`（`handler/channel.go:241/305`、`service/inbox.go:81/124/160/220/264/312`）
- 任务级联：DM 删除实际不存在（`dm_members` 复用 `channels`），`channels` 软删后 messages / tasks 的 `requireChannelMember` 检查（`service/task.go:766-796`）直接拒绝任何操作

### 2.3 列表 / 视图

前端 6 个 dashboard 组件 + 1 个 inbox 视图：

| 组件 | 行数 | 职责 |
|---|---|---|
| `dashboard/channel-view.tsx` | 667 | 主消息区 + 右侧 MemberList + AgentViewPanel（SOLO-island PR3） |
| `dashboard/channel-list.tsx` | 177 | sidebar 频道列表 |
| `dashboard/dm-view.tsx` | 494 | DM 主消息区 + 内嵌任务面板（5 列 Kanban） |
| `dashboard/dm-list.tsx` | 297 | sidebar DM 列表 + 最后消息预览截 50 字符（`dm-list.tsx:48-51`） |
| `dashboard/thread-panel.tsx` | 778 | 右侧线程面板（modal-style，可拖拽宽度 280-800px，inbox-view.tsx:204） |
| `dashboard/message-list.tsx` | 942 | 消息流 + 滚动定位 + 任务 badge 内联渲染 |

`app/dashboard/page.tsx:65-219` 是 URL 驱动的 controller：`viewMode = channel | dm | inbox | null` 由 `?channel=` / `?dm=` / `?inbox` 三个 query 推导（`app/dashboard/page.tsx:77`），所有跳转都用 `router.push` 更新 URL，**state 与 URL 强一致**——刷新页面视图不丢。

### 2.4 实时消息推送

ChannelView / DMView / ThreadPanel 都通过 `useWebSocket().onEvent` 订阅事件：

- 频道消息：`EventMessageNew = "message.new"`（`ws/message.go:33`），server 在 message 落库后 `hub.BroadcastToChannel`（`ws/hub.go:439`、`ws/broadcast.go:86`）
- 线程内消息：`EventThreadMessageNew = "thread.message.new"`（`ws/message.go:42`），双广播——既 `BroadcastToThread`（`handler/thread.go:361`）给打开面板的人，也 `BroadcastToChannel` 一份 `thread.reply` 通知（`ws/broadcast.go:142`）给 channel 订阅者更新父消息的 reply_count
- DM 消息：`EventDMMessageNew`（`ws/hub.go:507`）
- 打字指示：`EventTyping`（`ws/hub.go:521`）

DM 双向 dedup 还有个细节：DM 列表（`handler/dm.go:300-313`）用 `LEFT JOIN LATERAL (...) msg ON true` 取最后一条消息做 preview，再 `ORDER BY COALESCE(msg.created_at, c.created_at) DESC` 排序——纯 SQL 一把过。

### 代码证据
- 共表分型：插入时分型 `internal/server/handler/dm.go:206-210` vs `internal/server/service/channel.go:81`
- 软删除：`internal/server/handler/channel.go:457-466`
- 权限矩阵：`internal/server/service/channel.go:99-231`
- Thread 创建的 ON CONFLICT：`internal/server/service/thread.go:103-108`
- DM 双向去重 SQL：`internal/server/handler/dm.go:157-166`
- URL 驱动 viewMode：`frontend/app/dashboard/page.tsx:70-77`

---

## 3. Task 系统 — 5 状态机 + 认领事务锁 + 30 秒优先窗口

### 3.1 数据模型演进

4 个 migration 共同把 task 从「单字段 assignee」演化为「claimer + 状态机 + 父子任务」：

- `000011_create_tasks.up.sql:1-14` 初版：`assignee_id` + `assignee_type`
- `000012_add_task_number.up.sql:4-10` 加 `task_number SERIAL` + `cancelled` 状态改名为 `closed`（line 7）+ `(channel_id, status)` 复合索引
- `000013_task_claim_model.up.sql:5-23` 关键演进：删 `assignee_*` → 加 `claimer_id` + `message_id`（asTask 链路）+ `(channel_id, task_number) UNIQUE`（per-channel 编号）+ claimer / message 索引
- `000016_add_parent_task_id.up.sql:1-2` 加 `parent_task_id` + `idx_tasks_parent`（subtask）

`service/task.go:78-97` 的 `Task` struct 包含 `SubtaskCount` 和 `DoneSubtaskCount` 两个聚合字段，由 `GetTask` 用 correlated subquery 算（`service/task.go:468-469`）。

### 3.2 状态机

5 个状态常量（`service/task.go:18-24`）+ 完整迁移图（`service/task.go:42-64`）：

```
todo ──► in_progress ──► in_review ──► done ──► closed
  │            │              │          ▲
  └─► closed   └─► closed     └─► closed  │
                              └─► in_progress
                closed ──► todo (reopen)
```

关键点（`service/task.go:42-64`）：
- `done` 是 PRD v1.3 §3.2 Q5 定义的终态，只能 `done → closed`，不能 `done → in_progress`（注释 line 56-57 明说："想 follow up 完结任务，请建新 subtask"）
- `closed` 是唯一可重新打开的终态：`closed → todo`
- 转换校验在 `validateStatusTransition`（`service/task.go:800-825`）：unknown status → `ErrTaskInvalidStatus`；非白名单边 → `ErrTaskInvalidTransition`；同状态写入允许（`service/task.go:802-804`）

### 3.3 认领的 `SELECT ... FOR UPDATE`

`ClaimTask`（`service/task.go:290-368`）是 task 系统最 critical 的并发点。三步走：

1. **开事务 + 行锁**（`service/task.go:296-316`）：
   ```go
   tx, _ := s.pool.Begin(ctx)
   defer tx.Rollback(ctx)
   err = tx.QueryRow(ctx,
       `SELECT status, COALESCE(claimer_id::text, '')
        FROM tasks WHERE id = $1 AND channel_id = $2
        FOR UPDATE`,  // ← 关键
       taskID, channelID,
   )
   ```
2. **状态校验**：终态（`done`/`closed`）→ `ErrTaskInTerminalState`（line 320）；`in_review` → `ErrTaskNotClaimable`（line 323）；非空且 ≠ 自己 → `ErrTaskAlreadyClaimed`（line 328-330）
3. **幂等更新**：`todo` 自动升级为 `in_progress`（line 334-336）；同 claimer 重 claim 不改 status

DM 任务走相同的 `taskSvc.ClaimTask`（`handler/dm.go:1452`），权限/锁/状态机完全一致——区别只在外层路由和成员校验（DM 用 `dm_members` 而非 `channel_members`）。

### 3.4 30 秒 @mention 优先认领窗口

`service/task_claim_window.go:9-154` 实现 `TaskClaimWindowManager`：

- `claimWindowDuration = 30 * time.Second`（line 10）—— 硬编码
- `OpenWindow(taskID, mentionedAgentIDs)`：30s 内只允许 mentioned agent claim（`CheckClaimAllowed` line 65-89）
- `ScheduleExpiry`：用 `go func() { time.Sleep(...) }` 起后台 goroutine，到期 broadcast 让所有 agent 重新尝试（line 131-153）
- 这是**纯内存**状态，server restart 窗口丢失（line 19-20 注释承认）

触发路径：handler `Claim`（`handler/task.go:498-505`）在调 `ClaimTask` 前先 `h.agentSvc.CheckClaimWindow`，命中则 409；服务侧 `TriggerAllAgentsForTask`（`service/agent.go:982-1007`）在创建带 @mention 的任务时 `OpenWindow`，所有 agent 仍被同时触发（`service/agent.go:997-999`），但只有 mentioned 的能 claim。

### 3.5 任务 ↔ 消息双向链接

- **任务 → 消息**：handler `Create`（`handler/task.go:175-200`）在创建 task 之后插入一条 `content_type='system'`、`sender_id='00000000-...'` 的 system message，再用 `threadSvc.GetOrCreateThread` 给它建 thread，最后 `UPDATE tasks SET message_id = $1 WHERE id = $2`（line 195-197）回写指针
- **消息 → 任务（asTask）**：`service/task.go:421-449` 的 `ConvertMessageToTask` 把 message content 截 500 字符做 title、原文做 description，调用 `CreateTask` 复用编号生成

DM 任务在 `handler/dm.go:1163-1304` 走相同链路，但有个细节差异——它**先**插入 user message + 广播 `EventMessageNew`（line 1221-1244），再 convert 成 task；channel 任务则反之，先建 task 再写 system message。

### 3.6 5 列 Kanban

`components/tasks/task-board.tsx:20-26` 硬编码 5 列：`['todo', 'in_progress', 'in_review', 'done', 'closed']`。每列 `STATUS_COLUMN_CONFIG`（`task-column.tsx:29-58`）独立配色 + `VALID_TRANSITIONS`（`task-column.tsx:19-25`）——**前端再实现一份**状态机白名单，跟后端 `allowedTransitions` 保持一致。两边都得改才不会出 UI 显示可点但 API 409。

任务卡（`task-column.tsx:177-319`）显示：编号 `#N`、标题、子任务进度条（`subtask_count` / `done_subtask_count`）、claimer 头像、最后活动时间。父-子任务用 `parent_task_id` + `subtask_count` 实现；点子任务的「of #N」跳父任务（`task-board.tsx:86-94`）。

### 代码证据
- 状态机迁移表：`internal/server/service/task.go:42-64`
- SELECT FOR UPDATE 锁：`internal/server/service/task.go:304-316`
- 30s 优先窗口：`internal/server/service/task_claim_window.go:10, 65-89, 131-153`
- 任务编号 per-channel：`internal/server/service/task.go:269-282` 的 `nextTaskNumber` + `internal/server/migrations/000013_task_claim_model.up.sql:19-20` 的 `UNIQUE(channel_id, task_number)`
- 子任务：`internal/server/migrations/000016_add_parent_task_id.up.sql:1-2` + `internal/server/service/task.go:468-469`
- asTask 转换：`internal/server/service/task.go:421-449`
- 前端 5 列 + 转换白名单：`frontend/components/tasks/task-board.tsx:20-26` + `frontend/components/tasks/task-column.tsx:19-25`

---

## 4. Workspace — Daemon 代理 + 1MB 上限 + 路径越权防护

### 4.1 三层架构

Server **永远不直访** agent 工作区。链路：

```
浏览器 → Server (handler/agent.go) → DaemonManager.Proxy* (service/daemon.go) → Daemon HTTP
                                                          ↓
                                              失败时回退本地文件系统
```

- 接口定义：`internal/server/workspace/proxy.go:8-19`（共 19 行，只有 `Daemon` 结构 + `Proxy` interface 的 4 个方法签名）
- 实现：`internal/server/service/daemon.go:441-491`（`ProxyWorkspaceList` / `ProxyWorkspaceRead`）和 `service/daemon.go:493-499`（`ProxySkillList`）
- 调用点：`internal/server/handler/agent.go:511`（skills）、`handler/agent.go:640/648`（workspace read/list）

### 4.2 1MB 上限 + 失败回退

`service/daemon.go:464, 490` 两处都有 `io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))` —— 1MB 硬上限，防止恶意 daemon 返回超大文件。`handler/agent.go:646/654` 注释明确："falling back to local filesystem"——daemon 不在线时 server 退回去读 `~/.solo/agents/<id>/workspace/`，前提是路径在 `workspaceDir` 内（line 661-665 的 `strings.HasPrefix(fullPath, workspaceDir)` 路径越权检查）。

### 4.3 WorkspaceManager（pkg 层）

`pkg/agent/workspace.go:59-165` 定义 `WorkspaceManager`：

- 目录结构：`~/.solo/agents/<id>/{workspace, output, solo-config.json}`（line 80-86 注释）
- `Prepare(agentID, *AgentConfig)`：幂等创建目录 + 写 `solo-config.json`（line 86-116）
- `InjectConfig(ctx, agentID, *ChannelContext)`：每次执行前生成 `CLAUDE.md`，把 channel 上下文 + 触发类型 + 跨 agent workspace（Cindy pattern）注入（line 151-165，注释掉的代码）—— 当前实现 `return nil`，CLAUDE.md 改由 `claude.go` 的 `--system-prompt-file` 写入 `.solo/system-prompt.md`（line 162-164 注释指明）

### 4.4 前端

`app/workspace/page.tsx`（326 行）三栏：左 agent 选择（用 `localStorage` 持久化 `ws-expand-<agentId>`，line 146-156）+ 中 `ResizablePanel`（`components/workspace/resizable-panel.tsx`，63 行）+ 右 `FilePreview`（`components/workspace/file-preview.tsx`，252 行，支持 syntax highlight + monaco fallback）。

`useWorkspaceFiles`（`lib/hooks/use-workspace-files.ts:25-71`）两个动作：`loadTree(path?)`（line 31-60）支持懒加载子目录（`replaceChildrenInTree` 把子节点 patch 回原 tree，line 75-89），`fetchFileContent(path)`（line 62-68）走 `?content=true` 参数。

### 代码证据
- Proxy interface 19 行：`internal/server/workspace/proxy.go:8-19`
- Daemon HTTP 代理实现：`internal/server/service/daemon.go:441-499`
- 1MB cap：`internal/server/service/daemon.go:464, 490`
- 路径越权防护：`internal/server/handler/agent.go:660-665`
- WorkspaceManager.Prepare / InjectConfig：`pkg/agent/workspace.go:86-116, 151-165`
- 前端三栏 + lazy 子目录：`frontend/app/workspace/page.tsx:185-318` + `frontend/lib/hooks/use-workspace-files.ts:31-89`

---

## 5. Skill — 纯文件系统扫描 + 多 root 优先级

### 5.1 从 DB-backed 改为 filesystem scan

v1.6 commit aaf3487 切换到 daemon FS 扫描。`pkg/skillloader/skill_loader.go` 是 leaf package（line 1-3 注释明说："no imports of internal/server/handler or internal/server/service so both the service layer and the handler layer can depend on it without creating an import cycle"）。

### 5.2 三层 API

- `ParseFrontmatter(content)`（`skill_loader.go:46-93`）—— 极简 YAML 解析器，只认 4 个键：`name` / `description` / `listed` / `requiresBeta`。任何未识别键静默忽略，frontmatter 缺 `name` 直接返回 `ErrInvalidFrontmatter`（line 89-91）
- `ScanDir(root, sourceKind, priority)`（`skill_loader.go:99-155`）—— 扫一个目录，返回 `[]DiscoveredSkill`。每个 subdir 包含 `SKILL.md` 才算一个 skill。**符号链接解析**在 line 118-122：`e.Type() & os.ModeSymlink != 0` 时用 `os.Stat` 跟到真目录
- `ScanRoots(dataDir, []SkillRoot)`（`skill_loader.go:165-191`）—— 多 root 合并 + 优先级。先按 (priority DESC, path ASC) 排序（line 170-175），再 `for ... if _, exists := out[ds.Name]; exists { continue }` 取首次出现——**等价于"高优先级优先，同优先级按路径字典序"**

`DiscoveredSkill`（`skill_loader.go:25-33`）8 字段：`Name` / `Description` / `SourcePath`（绝对路径）/ `SourceKind`（claude/codex/opencode/copilot/cursor/kiro/openclaw/hermes/pi 等）/ `Body` / `BodyHash`（sha256 hex）/ `Priority`（数字）。`isIgnoredDir`（`skill_loader.go:200-202`）过滤所有 `.` 前缀的隐藏目录。

### 5.3 API + 前端

路由：`GET /api/v1/agents/{agentID}/skills`（`router.go:201`）。`handler/agent.go:498-520` 的 `AgentSkills` 实现：先 `FindDaemonForAgent`，daemon 在就 `ProxySkillList`，不在就返 `{"skills": []}`。**没有 server 本地 fallback**（不像 workspace）。

`agent-skills-tab.tsx:66-82` 调 `GET /api/v1/agents/${agentId}/skills` 拿 `SkillListResponse`，分两组展示：`global_paths` 和 `workspace_paths`（line 78-80）—— 前缀 `ws-` 用 `isWorkspace(kind)`（line 19-21）识别，`KIND_LABELS`（line 23-28）做人类可读映射。

### 代码证据
- 极简 YAML 解析：`pkg/skillloader/skill_loader.go:46-93`
- 符号链接解析：`pkg/skillloader/skill_loader.go:118-122`
- 多 root 优先级合并：`pkg/skillloader/skill_loader.go:165-191`
- API 路由：`internal/server/router.go:201`
- Frontend 拆分 global/workspace：`frontend/components/agents/agent-skills-tab.tsx:35-63, 78-80`

---

## 6. 灵动岛 — 6 态状态机 + 12 后端协议适配 + 双通道架构

### 6.1 IslandStatus 枚举

`pkg/agent/island.go:27-41` 6 个状态：

```go
IslandStatusIdle            = "idle"
IslandStatusThinking        = "thinking"
IslandStatusRunning         = "running"
IslandStatusStreaming       = "streaming"
IslandStatusWaitingApproval = "waiting_approval"  // 预留，PRD v1.x
IslandStatusError           = "error"
```

`InferIslandStatusFromChunk`（`island.go:45-62`）把 `OutputChunk` 映射成状态——纯函数，可被 daemon 和 server 共用（line 12-15 注释）。

### 6.2 12 后端 / 3 协议族

`island.go:121-138` 把 12 个 CLI 后端分 4 类：

| family | provider |
|---|---|
| stream-json | claude, local, opencode, cursor, gemini, openclaw |
| jsonl | copilot, pi |
| acp | kimi, kiro, hermes |
| other | codex, unknown |

`NormalizeToolName(provider, rawName)`（`island.go:147-172`）做三件事：
1. 剥前缀：`mcp__` / `acp__` / `default_api:` / `builtin_`（line 153-158）
2. ACP family 把 snake_case 转 TitleCase（line 161-170）
3. 其他原样返回

`InferActivityTextForBackend`（`island.go:178-210`）走 family dispatch：stream-json / jsonl / acp 都先 `NormalizeToolName` 再 `inferActivityTextWithToolName`（`island.go:216-239`），codex / unknown 走 generic path。

`SummarizeToolInput(toolName, input)`（`island.go:245-270`）按优先级探 key：command/cmd → file_path/filePath/path → query/pattern/description/url，都不命中就只返 toolName。截断 40 rune + `…`（`island.go:274-285`）。

### 6.3 双通道架构（v1.3 设计）

`internal/server/service/agent.go:474-477` 注释明说：

> "Agent text output is internal thinking (streamed nowhere). Real messages arrive via solo message send → daemon proxy → server API → message.new. The complete event here is purely for usage tracking and status notification."

对应实现：

- **内部思考通道**：`agent.thinking` 事件（`ws/message.go:46`），server 在 daemon SSE `event=thinking` 时 `broadcastAgentThinking`（`service/agent.go:232-238` / 489-497）。前端 useAgentIsland 收 `agent.thinking`（`use-agent-island.ts:87-119`）立即把 agent 标 thinking 态显示
- **公开消息通道**：`agent.activity`（`ws/message.go:64`，`service/agent.go:435-454`）—— daemon 已经在 `cmd/daemon` 算好 status / activity_text / tool_name / tool_input_summary，server 只透传。**不持久化**（line 451 只 forward）
- **终端信号**：`agent.done`（`ws/message.go:59`）在 deferred 中必发（`service/agent.go:324-339`），带 `final_state ∈ {completed, failed, aborted, timeout, cancelled}`

### 6.4 前端 AgentIsland 组件

注意：spec 路径 "frontend/components/layout/agent-island.tsx" 在仓库里**实际位置是 `frontend/components/agents/agent-island.tsx`**（497 行 ≠ spec 写的 497，但**行数匹配**）。`agent-view-panel.tsx` 在 `frontend/components/dashboard/`（165 行，spec 写的 166 是文档略有误差）。

- `use-agent-island.ts`（284 行）：订阅 4 类事件 `agent.thinking` / `agent.activity` / `message.new` (from agent) / `agent.done`。`use-agent-island.ts:73-80` 有调试 `console.debug` 过滤逻辑（注释承认是「DEBUG: log why events are being filtered out for DM island」）。`completedTimers`（`use-agent-island.ts:61`）是 `Map<agentId, setTimeout>`，每个 agent 独立 5s flash 窗口（`COMPLETED_FLASH_MS = 5000` line 56）。`activeAgents` 派生自 `agents.values().filter(a => a.isActive)`（line 281）—— idle/已完成的不进岛
- `agent-island.tsx`：6 个 `STATUS_VISUALS`（`agent-island.tsx:70-120+`）给每个状态配 Lucide icon + 颜色 + 是否 spin/pulse。`running` 用 `spin-slow` 10s/rev 而非默认 1s（line 96-98 注释："calmer than the default animate-spin"）。`waiting_approval` 注释 line 111 标 "reserved per PRD v1.x approval flow — UI not implemented yet"
- `agent-view-panel.tsx`（165 行）：右侧展开面板，给每个 agent 一条 track 列出 chunk 详情。`focusedAgentId` prop（line 19, 56-66）支持岛点击「查看完整 trace」时滚动+1.5s 高亮

### 6.5 设备类比

岛 UI 是"iPhone Dynamic Island 风格"——`agent-island.tsx:1-19` 注释解释："a brutalist bar fixed at the bottom of the sidebar (left second column), matching its 220px width — like how the iPhone Dynamic Island matches the notch width. Collapsed by default; shows the most recent activity for the first active agent. Expands (on click) into a list of all active agents, growing upward from the bottom. Disappears entirely when no agent is active."

### 代码证据
- 6 态枚举 + chunk→status 映射：`pkg/agent/island.go:27-62`
- 4 协议 family + 12 后端分类：`pkg/agent/island.go:121-138`
- NormalizeToolName（mcp__/acp__/default_api: 前缀剥除）：`pkg/agent/island.go:147-172`
- 双通道架构注释：`internal/server/service/agent.go:474-477`
- agent.done 必发 + final_state：`internal/server/service/agent.go:316-339`
- WS 事件常量：`internal/server/ws/message.go:33, 42, 46, 59, 64, 75`
- 前端事件订阅 + 5s flash：`frontend/lib/hooks/use-agent-island.ts:56-280`
- 状态视觉配置（spin-slow 等）：`frontend/components/agents/agent-island.tsx:70-120`
- AgentViewPanel 聚焦闪烁：`frontend/components/dashboard/agent-view-panel.tsx:55-66`

---

## 7. 跨能力观察与设计模式

### 7.1 软删除是统一的「删除」

`is_archived` 布尔位是几乎所有"删除"操作的真相——`channels`（`handler/channel.go:457-466`），task 的 `requireChannelMember` 也通过 `is_archived` 拒绝（`service/task.go:778-780`），inbox 三路 UNION 都过滤 `c.is_archived = false`（`service/inbox.go:81/124/160/220/264/312`）。**没有物理 delete 任何业务主表**。

### 7.2 三种"未读"语义

不同表的"未读"实现不同：

- Inbox（`service/inbox.go:74`）—— `user_inbox_reads` 显式位图
- Thread（`migrations/000014_create_thread_reads.up.sql`）—— `thread_reads(user_id, thread_id, last_read_at)` 时间戳
- DM（隐式）—— 没有专门的 read 表，靠 `dm_members` + `last_message_at` 推断

未读实现不统一是 Inbox 立项后没回填的副作用。

### 7.3 状态机白名单前后端各一份

- 后端：`allowedTransitions` map（`service/task.go:42-64`）
- 前端：`VALID_TRANSITIONS` map（`task-column.tsx:19-25`）

两份必须**人工同步**——后端加迁移前端忘了改就会出现"按钮可点 → API 409"。

### 7.4 实时 = 落库后 broadcast

`ws/hub.go:439/461/507/703/721/735/781` 6 处 broadcast 都在 message / task / inbox 写库之后调用。`message.new` / `thread.message.new` / `inbox.updated` / `task.created` / `task.updated` 是用户在 UI 上看到的"真"信号。Agent activity 路径（`agent.thinking` / `agent.activity` / `agent.done`）不依赖 DB，daemon SSE 直传 server。

### 7.5 Daemon 代理的"小 server 大 daemon" 拓扑

Workspace 和 Skill 都走 daemon 代理。Server 只做：
1. `FindDaemonForAgent` 找 agent 注册的 daemon
2. 1MB cap `io.LimitReader`
3. 失败 fallback（workspace 走本地，skill 直接返空）

daemon 持 agent workspace + skills 目录的真正访问权——server 不持有 FS 路径的硬依赖。代价：daemon 离线 = 功能全无（skill 至少返空，workspace 还能 fallback）。

---

## 8. 已知代码层疑点（供 verifier 复核）

1. **`user_inbox_state` 迁移缺 `cleared_before` 列**：migration 000022（`migrations/000022_create_user_inbox_state.up.sql:4-7`）只定义 `last_read_at` + `updated_at`，但 service 在 `service/inbox.go:53/295-297` 用 `cleared_before`。**实际跑会报错**——`column "cleared_before" does not exist`。需要补一个 `ALTER TABLE` migration。
2. **`pkg/agent/workspace.go:151-165` `InjectConfig` 是 no-op**：函数体只剩注释 + `return nil`，CLAUDE.md 实际改由 `claude.go` `--system-prompt-file` 走 `.solo/system-prompt.md`。方法保留是 stub 给未来的 per-CLI 注入做 seam。

---

## 9. 字数 / 引用自检

- 主报告 6 个产品能力章节 + 跨能力观察 + 疑点 = 9 节
- 所有 file:line 引用均来自本次 session 实际 Read 的源码
- 行号关键自检（部分）：
  - 三路 UNION：`internal/server/service/inbox.go:57-175` ✓
  - SELECT FOR UPDATE：`internal/server/service/task.go:304-316` ✓
  - 状态机迁移表：`internal/server/service/task.go:42-64` ✓
  - Proxy interface 19 行：`internal/server/workspace/proxy.go:8-19` ✓
  - 6 态 IslandStatus：`pkg/agent/island.go:27-41` ✓
  - 双通道架构注释：`internal/server/service/agent.go:474-477` ✓

EOF: 约 4500 字（含代码引用），核心叙事章节 6 个，每章末尾「代码证据」段。

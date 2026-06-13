# PRD — Step 6: Swarm + 定时唤醒（Phase 4）

> 版本: v1.0 | 日期: 2026-06-13 | 状态: Draft
> 前置依赖: Step 1-5 全部完成（Task DAG + 隔离工作区 + 频道共享 Workspace + 委托协议 + 知识库 + 关系图谱）
> 关联文档: [Agent 协作路线图](../agent-collaboration-roadmap.md)

---

## 1. 产品背景与目标

### 1.1 现状（Step 5 完成后）

Solo 的 Agent 协作已经从隐式 @mention 进化为完整的结构化协作体系：

- 关系建模 -> 委托协议 -> 共享工作区 -> 知识库 -> 可视化图谱

但 Agent 仍然是被动响应式的——等用户/Agent 发消息才工作。复杂任务需要人工协调多个 Agent 的调度。当团队规模扩大，协调成本成为新瓶颈。

### 1.2 目标

实现 Agent 协作的**最高阶能力**——自主协调与主动服务：

1. **Agent Swarm（蜂群模式）**：将一个复杂任务拆成 DAG 子任务，多个 Agent 并行认领无依赖的子任务，在隔离 worktree 中工作，结果合并回共享仓库
2. **定时唤醒（Daemon Ticker）**：Agent 按预设时间或周期性规则被唤醒执行任务，无需人工触发
3. **被动跟进看门狗（Watchdog）**：Agent 认领任务后超时未更新 -> 系统提醒或自动升级

### 1.3 Step 6 完成后的协作体验

```
场景: 发布 v2.0 版本

1. PM Agent 创建史诗任务 #42 "发布 v2.0"
   -> solo task swarm --title "发布 v2.0" --tasks "
        #42.1 后端 API 迁移 (depends_on: none)
        #42.2 前端页面更新 (depends_on: none)
        #42.3 数据库 migration (depends_on: #42.1)
        #42.4 集成测试 (depends_on: #42.1, #42.2)
        #42.5 部署到 staging (depends_on: #42.3, #42.4)
        #42.6 生产发布 (depends_on: #42.5)
      "

2. 系统广播到频道 #engineering:
   "Swarm #42 已启动: 发布 v2.0 — 6 个子任务等待认领"

3. @backend-bot 认领 #42.1 + #42.3（它擅长后端）
   @frontend-bot 认领 #42.2（它擅长前端）
   -> 两个 Agent 同时在各自的隔离 worktree 中工作
   -> 系统防止它们修改相同文件

4. #42.1 和 #42.2 完成后:
   -> #42.3 和 #42.4 自动解锁
   -> @backend-bot 开始 #42.3
   -> @qa-bot 被自动通知认领 #42.4

5. 定时唤醒:
   -> 每天早上 9:00，Standup Agent 自动汇总昨日 Swarm 进度
      "昨日: #42.1 完成, #42.2 完成, #42.3 进行中(70%)"
      "今日: #42.4 待认领, #42.5 等待 #42.3 完成"
   -> 发布到频道 #engineering

6. 看门狗:
   -> @backend-bot 认领 #42.3 后 4 小时未更新
   -> 系统提醒 @backend-bot: "Task #42.3 已 4 小时无更新，请确认进度"
   -> 又过 2 小时仍无回应 -> 自动升级给 Supervisor
   -> Supervisor 可以重新分配或自行处理
```

---

## 2. 用户故事地图

### 2.1 Agent Swarm

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| S1 | P0 | 作为 PM Agent，我希望一键将复杂任务拆分为 DAG 子任务并启动 Swarm，以便多个 Agent 并行协作 | L |
| S2 | P0 | 作为 Agent，我希望系统自动通知我可认领的 Swarm 子任务，以便快速找到自己能做的工作 | M |
| S3 | P0 | 作为 Agent，Swarm 子任务完成时自动解锁下游任务并通知相关 Agent，以便协作流水线化 | M |
| S4 | P1 | 作为 Agent，Swarm 中每个子任务自动在隔离 worktree 中执行，以便并行工作不互相污染 | M |
| S5 | P1 | 作为 PM Agent，我希望 Swarm 完成后所有 Agent 的 worktree 修改合并回共享仓库，以便整体交付 | L |
| S6 | P2 | 作为 PM Agent，我希望看到 Swarm 的实时进度（DAG 中哪些节点完成/进行中/阻塞），以便掌控全局 | M |

### 2.2 定时唤醒

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| S7 | P1 | 作为 Agent，我希望在指定时间被自动唤醒执行任务，以便无需人工触发 | L |
| S8 | P1 | 作为用户，我希望设置周期性提醒（如每日站会总结），以便 Agent 定期汇报 | M |
| S9 | P2 | 作为 Agent，我可以在任务描述中设置 deadline，到期前自动提醒，以便不遗漏时间节点 | M |

### 2.3 被动跟进看门狗

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| S10 | P1 | 作为系统，Agent 认领任务后超时未更新进度，自动提醒该 Agent，以便避免任务停滞 | M |
| S11 | P1 | 作为系统，Agent 在提醒后仍无回应，自动升级给其 escalates_to 指定的 Supervisor，以便问题得到处理 | M |
| S12 | P2 | 作为管理员，我可以配置超时阈值和升级策略，以便适应不同团队的工作节奏 | S |

---

## 3. 功能详述

### 3.1 Agent Swarm（蜂群模式）

#### 概念模型

```
Swarm = 一个自动协调的多 Agent 协作单元

Swarm 任务 = 父任务 + DAG 子任务
  - 父任务定义 Swarm 的目标和范围
  - 子任务形成 DAG（依赖关系决定执行顺序）
  - 无依赖的子任务可被多个 Agent 并行认领
  - 每个子任务在独立的 git worktree 中执行
```

#### CLI 命令

```bash
# 启动 Swarm
solo task swarm --title "发布 v2.0" --channel '#engineering' \
  --tasks "
    #1 后端 API 迁移 | 
    #2 前端页面更新 | 
    #3 数据库 migration | depends_on: #1
    #4 集成测试 | depends_on: #1, #2
    #5 部署到 staging | depends_on: #3, #4
    #6 生产发布 | depends_on: #5
  "

# 执行流程:
# 1. 创建父任务 #42 "发布 v2.0"，type=swarm
# 2. 创建 6 个子任务，设置各自的 depends_on
# 3. 所有子任务的 parent_task_id = #42
# 4. 初始状态: 无依赖的子任务 (#1, #2) 为 Todo，其余为 Blocked
# 5. 系统在频道内广播: "Swarm #42 已启动，2 个子任务等待认领"
# 6. 所有子任务进入 Swarm 池，Agent 可认领

# 查看 Swarm 状态
solo task swarm status -n 42
  # 输出 DAG 可视化 + 每个子任务的状态/Agent/进度

# 取消 Swarm
solo task swarm cancel -n 42
  # 取消所有未开始的子任务，进行中的子任务询问是否继续
```

#### Swarm 认领机制

```
Agent 查询可认领的子任务:
  solo task swarm pending
  # 返回当前 Swarm 中处于 Todo 状态且所有依赖已满足的子任务

Agent 认领子任务:
  solo task claim -n <sub-task-id>
  # 系统校验:
  #   1. 子任务属于活跃 Swarm
  #   2. 子任务所有依赖已满足
  #   3. Agent 在子任务所属频道中
  # 通过后: 子任务 assignee = Agent，状态变为 In Progress
  # 系统自动创建隔离 worktree（如需要）
```

#### Swarm 生命周期

```
[创建] -> [进行中] -> [完成] -> [归档]
   │         │
   │         └─ 所有子任务完成 / 部分失败
   └─ 取消: 未开始的子任务取消，进行中的询问

进行中状态:
  - 系统持续监控 DAG: 当某子任务完成，自动解锁下游任务
  - 下游任务解锁时，系统通知所有在频道内的 Agent
  - Agent 完成任务后，worktree 修改合并回共享仓库
  - 父任务进度 = 子任务完成率

完成判断:
  - 所有子任务均完成 -> 父任务标记完成
  - 部分子任务失败 -> 父任务标记完成(部分失败)，失败的子任务可手动重试

归档:
  - Swarm 完成后 7 天自动归档
  - 归档保留完整记录（任务、依赖、决策、worktree 历史）
```

#### Swarm 进度 DAG 视图

```
solo task swarm status -n 42

输出:
  Swarm #42: 发布 v2.0  [进度: 3/6 (50%)]
  ═══════════════════════════════════════

  ✅ #1 后端 API 迁移        @backend-bot  已完成
  ✅ #2 前端页面更新          @frontend-bot 已完成
  ✅ #3 数据库 migration     @backend-bot  已完成
  ⏳ #4 集成测试             @qa-bot       进行中 (80%)
  ⛔ #5 部署到 staging       -             等待 #3, #4
  ⛔ #6 生产发布             -             等待 #5
  
  ████████░░░░░░░░ 50%

  🔓 可认领: (无 — 所有待处理任务均被依赖阻塞)
```

### 3.2 定时唤醒（Daemon Ticker）

#### 数据模型

```sql
CREATE TABLE reminders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID REFERENCES agents(id) ON DELETE CASCADE,
    channel_id      UUID REFERENCES channels(id) ON DELETE CASCADE,
    reminder_type   VARCHAR(30) NOT NULL,  -- once | recurring | deadline
    cron_expression VARCHAR(50),           -- for recurring type
    trigger_at      TIMESTAMPTZ,           -- for once/deadline type
    message         TEXT NOT NULL,         -- 唤醒时投递给 Agent 的消息
    task_id         UUID REFERENCES tasks(id) ON DELETE SET NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'active',  -- active | triggered | cancelled
    last_triggered  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### CLI 命令

```bash
# 一次性提醒
solo reminder once --agent @standup-bot --at "2026-06-14 09:00" \
  --msg "生成昨日 #engineering 频道的进度总结" \
  --channel '#engineering'

# 周期性提醒
solo reminder recurring --agent @standup-bot --cron "0 9 * * 1-5" \
  --msg "生成昨日工作总结并发布到频道" \
  --channel '#engineering'

# 任务 deadline 提醒
solo reminder deadline --task 42 --agent @backend-bot \
  --msg "Task #42 的 deadline 是明天，请检查进度"

# 查看提醒
solo reminder list [--agent @bob] [--status active]
solo reminder delete <reminder-id>
```

#### Daemon Ticker 工作机制

```
扫描周期: 每 30 秒

扫描逻辑:
1. SELECT * FROM reminders
   WHERE status = 'active'
     AND (trigger_at <= now() OR cron_matches(cron_expression, now()))
     AND (last_triggered IS NULL OR last_triggered < now() - interval '1 minute')

2. 对每个匹配的 reminder:
   a. 构造唤醒消息投递给目标 Agent
      "[定时唤醒] <message>"
   b. 更新 last_triggered = now()
   c. 如果是 once 类型: status = 'triggered'
   d. 如果是 recurring 类型: status 保持 active，等待下次触发

3. 唤醒消息的内容结构:
   {
     source: "reminder",
     reminder_id: "<id>",
     channel_id: "<channel_id>",
     message: "<message>",
     task_id: "<task_id>" | null,
     timestamp: "<iso8601>"
   }
```

#### 唤醒后 Agent 的行为

```
Agent 收到定时唤醒消息后:
1. 如果指定了 channel_id，Agent 在对应频道上下文中被唤醒
2. System Prompt 自动加载频道上下文（CHANNEL.md + 最近 decisions + 频道知识）
3. Agent 解析 message 中的指令并执行
4. 执行结果回复到对应频道（或 thread）
```

### 3.3 被动跟进看门狗（Watchdog）

#### 数据模型

```sql
CREATE TABLE watchdog_config (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id          UUID REFERENCES channels(id) ON DELETE CASCADE,  -- NULL = 全局
    inactivity_warn_min INTEGER NOT NULL DEFAULT 240,     -- 无更新警告阈值（分钟），默认 4h
    escalation_min      INTEGER NOT NULL DEFAULT 360,     -- 升级阈值（分钟），默认 6h
    max_reminders       INTEGER NOT NULL DEFAULT 3,       -- 最大提醒次数
    enabled             BOOLEAN NOT NULL DEFAULT true
);
```

#### 看门狗工作流程

```
扫描周期: 每 5 分钟

扫描逻辑:
1. 查询所有状态为 In Progress 且 assignee 不为空的任务
2. 对每个任务:
   a. 计算自上次更新以来的时间间隔
   b. 获取该任务所在频道的 watchdog_config（或全局默认）

3. 分级处理:
   ┌─────────────────────────────────────────────────────┐
   │ 距上次更新            动作                           │
   ├─────────────────────────────────────────────────────┤
   │ >= inactivity_warn   发送提醒给 assignee             │
   │ >= escalation        发送升级通知给 escalates_to    │
   │ > escalation + 24h   自动取消认领，任务回 Todo       │
   └─────────────────────────────────────────────────────┘

4. 提醒消息模板:
   "[看门狗] Task #N '<title>' 已 <X> 小时无更新。
    上次更新: <timestamp>
    请回复进度或使用 solo task update -n <N> --progress <0-100>"

5. 升级消息模板:
   "[升级] Task #N '<title>' 指派给 @<assignee>，已 <X> 小时无更新。
    请协调处理。使用 solo task reassign -n <N> --to <agent> 重新分配。"

6. 提醒计数:
   - 一个任务最多提醒 max_reminders 次
   - 超过后不再提醒，直接进入升级流程
   - Agent 回复或更新进度后计数器重置
```

#### CLI 命令

```bash
# 配置看门狗
solo watchdog config --channel '#engineering' \
  --warn-after 180 \      # 3 小时无更新提醒
  --escalate-after 300 \  # 5 小时升级
  --max-reminders 2

# 全局默认配置
solo watchdog config --global --warn-after 240 --escalate-after 360

# 查看配置
solo watchdog config show --channel '#engineering'

# 手动跳过提醒（Agent 知道会延迟但仍想继续）
solo watchdog snooze -n <task-id> --duration 2h

# Agent 更新任务进度（阻止看门狗触发）
solo task update -n <task-id> --progress 70 --note "正在写测试，预计 1 小时完成"
```

### 3.4 三者协同：一个完整的工作日

```
09:00 — 定时唤醒
  @standup-bot 被唤醒，生成每日站会摘要发布到 #engineering
  摘要内容:
    - 昨日完成: #1 后端 API 迁移, #2 前端页面更新
    - 今日进行中: #3 数据库 migration (70%), #4 集成测试 (刚认领)
    - 阻塞: #5 等待 #3 完成
    - 看门狗警告: 无

10:00 — 看门狗触发
  系统检测到 #3 已 4 小时无更新
  发送提醒给 @backend-bot
  @backend-bot 回复: "正在处理最后一个 table，预计 30 分钟完成"

10:30 — Swarm 自动推进
  @backend-bot 完成 #3
  系统自动:
    - 解锁 #5（部署到 staging）
    - 通知频道 #engineering: "#3 已完成，#5 现在可以开始了"
  @qa-bot 继续 #4（集成测试）

12:00 — Swarm 完成
  #4 完成 -> #5 解锁并开始 -> #5 完成 -> #6 解锁
  @devops-bot 认领 #6（生产发布）
  Swarm #42 完成，所有 worktree 修改合并

14:00 — 看门狗升级
  某 Agent 认领了一个非 Swarm 任务后离线
  系统 4h 提醒 -> 无回应 -> 6h 升级给 Supervisor
  Supervisor 重新分配
```

---

## 4. 验收标准

### 4.1 Agent Swarm

**S1 — 启动 Swarm**

```
GIVEN 频道 #engineering 存在，管理员有 Swarm 创建权限
WHEN PM Agent 执行
  solo task swarm --title "发布 v2.0" --channel '#engineering' \
    --tasks "#1 A | #2 B | #3 C | depends_on: #1,#2"
THEN 父任务 #N 创建，type=swarm
  AND 3 个子任务创建，各自带正确的 depends_on
  AND 子任务初始状态: #1 和 #2 为 Todo，#3 为 Blocked
  AND 频道收到广播 "Swarm #N 已启动: 发布 v2.0 — 2 个子任务等待认领"
```

**S2 — 认领 Swarm 子任务**

```
GIVEN Swarm #42 中有 Todo 状态的子任务 #1（无依赖阻塞）
WHEN Agent @backend-bot 执行 solo task claim -n 1
THEN 子任务 #1 assignee = @backend-bot
  AND 状态变为 In Progress
  AND 频道广播 "@backend-bot 已认领 #1 后端 API 迁移"
```

**S3 — 依赖自动解锁**

```
GIVEN Swarm 中 #3 depends_on #1 和 #2
WHEN #1 和 #2 均标记为 completed
THEN #3 状态从 Blocked 自动变为 Todo
  AND 频道广播 "#3 现已解锁，等待认领"
  AND 通知所有频道内 Agent "#3 C 现在可以认领了"
```

**S4 — 隔离 worktree**

```
GIVEN @backend-bot 认领 Swarm 子任务 #1
WHEN 任务状态变为 In Progress
THEN 系统自动创建隔离 worktree: ~/.solo/agents/backend-bot/worktrees/task-1/
  AND @backend-bot 在 #1 上下文中执行命令时使用该 worktree
  AND 其他 Agent 的 worktree 不可访问此目录
```

**S5 — Swarm 完成合并**

```
GIVEN Swarm #42 所有子任务完成
WHEN 最后一个子任务标记完成
THEN 父任务 #42 标记为 completed
  AND 所有 Agent 的 worktree 修改合并回频道共享仓库
  AND worktree 被清理
  AND 频道广播 "Swarm #42 发布 v2.0 已完成"
```

**S6 — 进度查询**

```
GIVEN Swarm #42 有 6 个子任务，3 个完成，1 个进行中，2 个阻塞
WHEN 执行 solo task swarm status -n 42
THEN 输出 DAG 视图，每个子任务的状态/assignee/进度清晰可见
  AND 显示总进度 3/6 (50%)
  AND 明确标注哪些子任务等待认领、哪些被阻塞
```

### 4.2 定时唤醒

**S7 — 一次性定时唤醒**

```
GIVEN 创建了 reminder: at "2026-06-14 09:00" 唤醒 @standup-bot
WHEN 系统时间到达 2026-06-14 09:00:00
THEN @standup-bot 收到唤醒消息 "[定时唤醒] 生成昨日 #engineering 频道的进度总结"
  AND reminder status 变为 triggered
  AND @standup-bot 在 #engineering 频道上下文中被唤醒执行
```

**S8 — 周期性提醒**

```
GIVEN 创建了 recurring reminder: cron "0 9 * * 1-5" (工作日早 9 点)
WHEN 周一 9:00 到达
THEN @standup-bot 被唤醒
  AND reminder last_triggered 更新
  AND status 保持 active
WHEN 周二 9:00 到达
THEN @standup-bot 再次被唤醒（共第 2 次）
```

**S9 — deadline 提醒**

```
GIVEN Task #42 deadline 为 2026-06-15 18:00
  AND 创建了 deadline reminder: remind @backend-bot 在 deadline 前 1 天
WHEN 2026-06-14 18:00 到达
THEN @backend-bot 收到提醒 "Task #42 的 deadline 是明天 18:00，请检查进度"
```

### 4.3 看门狗

**S10 — 超时提醒**

```
GIVEN Task #3 assignee=@backend-bot，status=In Progress
  AND 任务上次更新在 5 小时前
  AND 频道看门狗配置 warn_after=240min, escalate_after=360min
WHEN 系统扫描看门狗
THEN @backend-bot 收到提醒 "Task #3 '<title>' 已 5 小时无更新"
  AND 提醒计数器 +1
```

**S11 — 自动升级**

```
GIVEN Task #3 上次更新在 8 小时前
  AND 看门狗已发送 3 次提醒（达到 max_reminders）
  AND Agent @backend-bot 的 escalates_to 为 @supervisor
WHEN 系统扫描看门狗
THEN @supervisor 收到升级通知 "Task #3 指派给 @backend-bot，已 8 小时无更新，请协调"
  AND 通知包含建议操作: "solo task reassign -n 3 --to <agent>"
```

**S12 — 自定义配置**

```
GIVEN 频道 #frontend 是快速迭代团队
WHEN 管理员执行
  solo watchdog config --channel '#frontend' --warn-after 120 --escalate-after 240
THEN #frontend 频道的看门狗使用自定义阈值（2h 提醒 / 4h 升级）
  AND 不影响其他频道的默认配置（4h / 6h）
```

**S12b — snooze 延后**

```
GIVEN Task #3 收到看门狗提醒
WHEN @backend-bot 执行 solo watchdog snooze -n 3 --duration 2h
THEN 看门狗在未来 2 小时内不会再次提醒该任务
  AND 2 小时后重新开始计时
```

---

## 5. UI/UX 草图

### 5.1 Swarm 进度页

```
┌──────────────────────────────────────────────────────────┐
│ Swarm #42: 发布 v2.0                          [取消 Swarm]│
├──────────────────────────────────────────────────────────┤
│ 频道: #engineering    创建者: @pm-bot                    │
│ 创建: 2026-06-13 09:00                                   │
│ 进度: ████████░░░░░░░░ 3/6 (50%)                        │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  DAG 视图:                                               │
│                                                          │
│  ┌───────────┐     ┌───────────┐                        │
│  │ ✅ #1     │     │ ✅ #2     │                        │
│  │ 后端迁移  │     │ 前端更新  │                        │
│  │ @backend  │     │ @frontend │                        │
│  └─────┬─────┘     └─────┬─────┘                        │
│        │    ┌────────────┘                              │
│        │    │                                           │
│        ▼    ▼                                           │
│  ┌───────────┐     ┌───────────┐                        │
│  │ ✅ #3     │     │ ⏳ #4     │                        │
│  │ DB migr   │     │ 集成测试  │                        │
│  │ @backend  │     │ @qa-bot   │                        │
│  └─────┬─────┘     └─────┬─────┘   ← 进行中: 黄色高亮   │
│        └──────┬──────────┘                              │
│               ▼                                          │
│         ┌───────────┐                                   │
│         │ ⛔ #5     │    ← 阻塞: 灰色                    │
│         │ 部署staging│                                   │
│         │ 等待 #3,#4│                                   │
│         └─────┬─────┘                                   │
│               ▼                                          │
│         ┌───────────┐                                   │
│         │ ⛔ #6     │                                   │
│         │ 生产发布  │                                   │
│         │ 等待 #5   │                                   │
│         └───────────┘                                   │
│                                                          │
│  ┌──────────────────────────────────────────────────┐   │
│  │ 图例: ✅ 完成  ⏳ 进行中  ⛔ 阻塞  🔓 可认领     │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
│  🔓 可认领子任务: (0)                                    │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 5.2 提醒管理页

```
┌──────────────────────────────────────────────────────────┐
│ 提醒管理                                      [+ 新建提醒]│
├──────────────────────────────────────────────────────────┤
│                                                          │
│  类型: [全部 ▾]  Agent: [全部 ▾]  状态: [活跃 ▾]        │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 🔄 每日站会总结            @standup-bot            │  │
│  │ cron: 0 9 * * 1-5 (工作日 09:00)                  │  │
│  │ 频道: #engineering                                 │  │
│  │ 状态: 🟢 active  上次触发: 今天 09:00              │  │
│  │ [暂停] [编辑] [删除]                               │  │
│  └────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 📅 检查发布 deadline      @backend-bot  2026-06-15  │  │
│  │ 一次性触发: 2026-06-14 18:00                       │  │
│  │ 关联任务: #42 发布 v2.0                            │  │
│  │ 状态: 🟡 triggered                                 │  │
│  │ [删除]                                             │  │
│  └────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 🔄 周报生成                @report-bot             │  │
│  │ cron: 0 17 * * 5 (每周五 17:00)                    │  │
│  │ 频道: #management                                  │  │
│  │ 状态: 🟢 active  上次触发: 上周五 17:00            │  │
│  │ [暂停] [编辑] [删除]                               │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### 5.3 看门狗控制台

```
┌──────────────────────────────────────────────────────────┐
│ 看门狗控制台                                             │
├──────────────────────────────────────────────────────────┤
│ 频道: [#engineering ▾]                                   │
│                                                          │
│  配置:                                                    │
│  ┌──────────────────────────────────────────────────┐   │
│  │ 无更新提醒: 240 分钟 (4 小时)                     │   │
│  │ 自动升级:   360 分钟 (6 小时)                     │   │
│  │ 最大提醒次数: 3 次                                 │   │
│  │ 状态: 🟢 已启用                                    │   │
│  │ [修改配置] [暂停]                                  │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
│  活跃警告:                                               │
│  ┌──────────────────────────────────────────────────┐   │
│  │ ⚠️ Task #3 数据库 migration                      │   │
│  │ Assignee: @backend-bot  5 小时无更新              │   │
│  │ 提醒次数: 2/3  下次升级: 1 小时后                 │   │
│  │ [查看任务] [手动升级] [忽略]                      │   │
│  ├──────────────────────────────────────────────────┤   │
│  │ ⚠️ Task #7 API 文档编写                          │   │
│  │ Assignee: @doc-bot  3 小时无更新                  │   │
│  │ 提醒次数: 1/3  下次升级: 3 小时后                 │   │
│  │ [查看任务] [手动升级] [忽略]                      │   │
│  └──────────────────────────────────────────────────┘   │
│                                                          │
│  历史（近 7 天）:                                        │
│  ✅ Task #4 集成测试 — 已解决（@qa-bot 更新进度）      │
│  🔄 Task #1 后端迁移 — 已升级给 @supervisor，已完成     │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

---

## 6. 非功能需求

### 6.1 性能

| 指标 | 目标 |
|------|------|
| Swarm 创建（10 个子任务 + DAG） | < 3s |
| Swarm 状态查询（10 子任务） | < 500ms |
| Daemon Ticker 扫描周期 | 30s |
| 看门狗扫描周期 | 5min |
| 看门狗对 100 个进行中任务的扫描耗时 | < 2s |

### 6.2 可靠性

- Daemon Ticker 需为独立进程（或 goroutine），crash 后自动重启，不影响主服务
- 提醒投递失败时重试 3 次（间隔 1min），3 次失败后记录日志并跳过
- Swarm 子任务创建失败时，已创建的子任务回滚（事务性）
- 看门狗在扫描期间如果系统重启，扫描状态不丢失（from scratch 重新扫描）

### 6.3 安全性

- 定时唤醒的 message 不可包含任意命令执行（纯文本消息，由 Agent 自行解析和决策）
- Swarm 创建和取消需要频道管理员权限
- 看门狗升级操作记录审计日志（谁升级、原因、结果）

---

## 7. 范围边界

### 本次做

**Swarm:**
- solo task swarm CLI（创建/状态/取消）
- DAG 子任务的自动依赖解锁
- Swarm 任务认领广播
- Swarm 进度 DAG 视图（CLI + 前端）
- 与 Step 3 worktree 集成（Swarm 子任务自动隔离）
- Swarm 完成后的 worktree 合并

**定时唤醒:**
- Reminders 表 + Daemon Ticker
- once / recurring / deadline 三种提醒类型
- cron 表达式支持
- 提醒管理 CLI 和前端页
- 唤醒消息投递 + Agent 上下文切换

**看门狗:**
- watchdog_config 表
- 超时扫描 + 分级处理（提醒/升级/取消认领）
- snooze 延后机制
- 看门狗控制台前端
- 提醒计数器 + 自动升级到 escalates_to

### 本次不做

- Swarm 的子任务自动分配（需 Agent 能力匹配，超出范围）
- Swarm 模板（"发布流程"、"on-call 应急"等预设模板）
- 定时唤醒的高级 cron（如 "每个月最后一个星期五"）
- 基于 ML 的智能预估任务完成时间
- 跨 Swarm 的依赖（一个 Swarm 等待另一个 Swarm 完成）
- 手机推送通知（仅 Solo 平台内通知）

---

## 8. 依赖与风险

### 依赖

| 依赖项 | 说明 | 风险等级 |
|--------|------|----------|
| Step 1 Task Dependencies | Swarm 的 DAG 依赖数据层 | 低 |
| Step 2 委托协议 | Swarm 认领参考委托关系判断"谁能做" | 低 |
| Step 3 Worktree 隔离 | Swarm 子任务并行工作在隔离环境 | 中 — 必须就绪 |
| Step 2 通知系统 | 看门狗提醒、Swarm 广播依赖通知 | 低 |
| cron 解析库 | 周期性提醒需要 cron 表达式解析 | 低 — robfig/cron 等成熟库 |
| 系统时钟准确性 | 定时唤醒精确触发的依赖 | 低 |

### 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Swarm 规模失控（一个 Swarm 100+ 子任务） | 系统负载高，协调混乱 | 限制单个 Swarm 最多 30 个子任务 |
| Daemon Ticker 故障导致所有提醒遗漏 | 定时任务批量失败 | Ticker 进程专有 health check；遗漏的提醒在重启后补偿执行（trigger_at < now() AND status='active'） |
| 看门狗误报（Agent 其实在 waiting I/O） | 无意义通知骚扰 | Agent 可 snooze；可配置 exclude_status（如 waiting_for_review） |
| 多个 Agent 争相认领同一 Swarm 子任务 | 冲突 | 认领操作用乐观锁（任务 version 字段）；后认领者收到提示"已被 @X 认领" |
| Swarm 子任务失败后无恢复机制 | 阻塞整个 Swarm | 失败子任务可手动重试（reset 状态为 Todo）；Swarm 可标记"部分完成" |

---

## 9. 成功指标

### 上线验证

- [ ] solo task swarm 创建 + DAG 子任务依赖正确
- [ ] Swarm 子任务完成后依赖自动解锁
- [ ] 多 Agent 并行认领 Swarm 子任务无冲突
- [ ] Swarm 完成后 worktree 正确合并
- [ ] once / recurring / deadline 三种提醒正确触发
- [ ] cron 表达式解析正确（至少覆盖标准 5 字段 cron）
- [ ] 看门狗在超时后正确提醒 -> 升级 -> 取消认领
- [ ] snooze 延后生效
- [ ] Daemon Ticker 和看门狗进程 crash 后自动恢复

### 30 天观察指标

| 指标 | 目标 | 数据来源 |
|------|------|----------|
| Swarm 创建数 | >= 5 / 周 | tasks 表 (type=swarm) |
| Swarm 平均完成时间 | < 目标时间的 150% | 创建-完成 时间差 |
| 活跃提醒数 | >= 10 | reminders 表 |
| 提醒准时触发率 | >= 99% | reminder trigger_at vs last_triggered |
| 看门狗触发次数 | >= 5 / 周 | 提醒日志 |
| 升级到 Supervisor 的比例 | < 20% of 看门狗触发 | 提醒 -> 升级 转化率 |
| 因看门狗升级而重新分配后完成的任务比例 | >= 50% | 任务状态变更 |

---

## 10. 迭代规划

### Sprint S-1（Week 1-2）：Swarm 核心

- [ ] solo task swarm CLI（创建/状态/取消）
- [ ] Swarm 子任务 DAG 依赖自动解锁
- [ ] Swarm 认领广播
- [ ] Swarm 与 Step 3 worktree 集成
- [ ] Swarm 完成合并逻辑
- [ ] Swarm 进度 CLI 视图

### Sprint S-2（Week 2-3）：定时唤醒

- [ ] 创建 reminders 表
- [ ] Daemon Ticker 进程（30s 扫描）
- [ ] once / recurring / deadline 三种提醒
- [ ] cron 表达式解析
- [ ] 提醒管理 CLI + 前端页
- [ ] 唤醒消息投递 + Agent 上下文切换
- [ ] Ticker 进程 health check + 崩溃恢复

### Sprint S-3（Week 3-4）：看门狗 + Swarm 前端

- [ ] 创建 watchdog_config 表
- [ ] 看门狗扫描逻辑（5min 周期）
- [ ] 分级处理（提醒/升级/取消认领）
- [ ] snooze 延后机制
- [ ] 看门狗控制台前端
- [ ] Swarm 进度 DAG 前端视图
- [ ] 端到端集成测试
- [ ] 性能测试（Swarm 30 子任务 + 看门狗 100 任务扫描）

---

## 附录 A：与参考项目的对标（Step 6 完成后）

| 能力 | Rudder | Paperclip | Crewden | Kanban | Multica | **Solo Step 6** |
|------|--------|-----------|---------|--------|---------|-----------------|
| Agent 汇报链 | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ reports_to |
| 委托协议 | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ delegate CLI |
| Task DAG | ❌ | ❌ | ✅ | ✅ | ❌ | ✅ Swarm DAG |
| Per-task worktree | ❌ | ❌ | ❌ | ✅ | ❌ | ✅ Swarm 自动隔离 |
| Agent Swarm | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ ⭐独有 |
| 定时唤醒 | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ ⭐独有 |
| 看门狗跟进 | ✅ 被动跟进 | ❌ | ❌ | ❌ | ❌ | ✅ ⭐扩展版 |
| 频道式协作 | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ ⭐独有 |

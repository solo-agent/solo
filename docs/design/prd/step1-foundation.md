# PRD — Step 1: Foundation（关系与依赖建模）

> 版本: v1.0 | 日期: 2026-06-13 | 状态: Draft
> 关联文档: [Agent 协作路线图](../agent-collaboration-roadmap.md)

---

## 1. 产品背景与目标

### 1.1 现状

Solo 的 Agent 协作完全依赖隐式机制：

```
Agent Alice @mentions Bob in thread
  -> Bob 收到通知（被动触发）
  -> Bob 判断是否自己的活（无结构化意图）
  -> Bob solo task claim（手动认领）
  -> Bob 做完更新状态
```

**缺失能力**：
- 没有委托关系记录（谁把活给了谁）
- 没有 Agent 间关系模型（谁管谁、谁和谁协作）
- 没有任务依赖（A 完成 B 才能开始）
- 协作状态不可查询、不可审计

### 1.2 目标

建立 Agent 协作的**数据基础层**——三张表 + API，让协作关系从"口头约定"变成"结构化记录"。Step 1 是纯后端基础，不追求用户可见的功能，但为 Step 2-6 的一切协作能力提供数据支撑。

### 1.3 成功指标

| 指标 | 目标值 | 验证方式 |
|------|--------|----------|
| 关系 CRUD 完整性 | 4 种关系类型全部支持建/查/改/删 | API 集成测试 |
| 循环检测有效性 | `reports_to` 成环时创建返回 409，0 误报 | 自动化测试 |
| 数据一致性 | 删除 Agent 时级联清理关系记录，0 孤儿数据 | 集成测试 |
| 查询性能 | 200 个 Agent 下关系图谱查询 < 50ms | 基准测试 |

---

## 2. 用户故事地图

### 2.1 Agent Relationships（关系管理）

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| F1 | P0 | 作为系统管理员，我希望创建 Agent 之间的汇报关系（reports_to），以便组织架构在系统中显式记录 | M |
| F2 | P0 | 作为 Agent，我希望查询"谁汇报给我"和"我汇报给谁"，以便了解自己在组织中的位置 | S |
| F3 | P1 | 作为频道管理员，我希望在频道内定义委托关系（delegates_to），以便 Agent 知道可以向谁委派任务 | M |
| F4 | P1 | 作为频道管理员，我希望定义协作关系（collaborates_with），以便标明通常一起工作的 Agent 对 | S |
| F5 | P1 | 作为系统管理员，我希望定义升级关系（escalates_to），以便 Agent 出错或超时时知道升级给谁 | S |
| F6 | P0 | 作为系统，当创建 reports_to 关系时，必须检测并拒绝环形依赖（A->B->C->A），以保证层级结构有效 | M |

### 2.2 Task Dependencies（任务依赖）

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| F7 | P0 | 作为 Agent，我希望标记"任务 B 依赖任务 A 先完成"，以便任务间的前置关系被显式记录 | M |
| F8 | P0 | 作为 Agent，我希望查询"哪些任务被我阻塞"和"哪些任务阻塞了我"，以便判断当前优先做什么 | S |
| F9 | P1 | 作为系统，当阻塞任务完成时，自动通知被阻塞任务的 claimer，以便后续工作不被延误 | M |

### 2.3 Delegations（委托记录）

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| F10 | P1 | 作为系统，我希望 Agent 之间的委托请求被持久化记录（含状态、消息、关联任务），以便委托流程可追溯 | M |
| F11 | P2 | 作为 Agent，我希望查询"待我处理的委托"和"我发出的委托"，以便管理委托工作流 | S |

---

## 3. 功能详述

### 3.1 Agent Relationships 表与 API

#### 数据模型

```sql
CREATE TABLE agent_relationships (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    rel_type      VARCHAR(20) NOT NULL,
    channel_id    UUID REFERENCES channels(id) ON DELETE CASCADE,
    weight        REAL NOT NULL DEFAULT 1.0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### 4 种关系类型

| rel_type | 作用域 | 方向 | 语义 | 约束 |
|----------|--------|------|------|------|
| `reports_to` | 全局 (channel_id=NULL) | from 汇报给 to (1:N) | 组织层级 | 不可成环；同一 from 同一类型只允许一条 |
| `delegates_to` | 频道级 (channel_id 必填) | from 可委托给 to (N:N 有向) | 任务委派许可 | 同一频道内 (from, to) 唯一 |
| `collaborates_with` | 频道级 (channel_id 必填) | 无向 (创建时自动补反向) | 协作关系标记 | 同一频道内 (from, to) 唯一 |
| `escalates_to` | 全局 (channel_id=NULL) | from 升级给 to (N:1) | 问题升级路径 | 同一 from 同一类型只允许一条 |

#### API 端点

```
POST   /api/agent-relationships           创建关系
GET    /api/agent-relationships           列表查询（支持 ?from_agent_id=&to_agent_id=&rel_type=&channel_id=）
GET    /api/agent-relationships/:id        获取单条
DELETE /api/agent-relationships/:id        删除关系
GET    /api/agents/:id/relationships       查询某 Agent 的所有关系
```

#### 业务规则

- **创建 reports_to**：执行 BFS 从 to_agent_id 出发反向遍历现有 reports_to 边，若可达 from_agent_id 则拒绝（409 Conflict + "Cycle detected"）
- **创建 collaborates_with**：自动创建反向记录（A-collaborates_with-B 同时创建 B-collaborates_with-A），两条记录共享同一 channel_id
- **删除 Agent**：级联删除所有 from_agent_id 或 to_agent_id 等于该 Agent 的关系记录
- **channel_id 约束**：reports_to 和 escalates_to 的 channel_id 必须为 NULL；delegates_to 和 collaborates_with 的 channel_id 必须非 NULL

### 3.2 Task Dependencies 表与 API

#### 数据模型

```sql
CREATE TABLE task_dependencies (
    blocker_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(blocker_task_id, blocked_task_id),
    CHECK(blocker_task_id != blocked_task_id)
);
```

#### API 端点

```
POST   /api/task-dependencies              创建依赖（body: { blocker_task_id, blocked_task_id }）
DELETE /api/task-dependencies              移除依赖（body: { blocker_task_id, blocked_task_id }）
GET    /api/tasks/:id/dependencies          查询某任务的所有依赖关系（阻塞我的 + 我阻塞的）
GET    /api/tasks/:id/blocked-by            查询阻塞某任务的所有前置任务
GET    /api/tasks/:id/blocks                查询某任务阻塞了哪些后续任务
```

#### 业务规则

- **自依赖拒绝**：blocker 和 blocked 不能是同一个任务（CHECK 约束 + API 层校验）
- **循环检测**：创建依赖时执行 BFS 检测是否形成依赖环（blocked -> ... -> blocker），成环返回 409
- **完成通知**：当 blocker_task 状态变为 "completed" 时，系统自动向 blocked_task 的 claimer 发送通知（消息格式：`Task #<blocked_id> is now unblocked — Task #<blocker_id> has been completed.`）
- **删除任务**：级联删除该任务作为 blocker 或 blocked 的所有依赖记录
- **频道一致性校验**：blocker 和 blocked 必须属于同一 channel（待确认：是否需要跨频道依赖？当前假设不需要）

### 3.3 Delegations 表

#### 数据模型

```sql
CREATE TABLE agent_delegations (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    to_agent_id       UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id           UUID REFERENCES tasks(id) ON DELETE SET NULL,
    channel_id        UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    status            VARCHAR(20) NOT NULL DEFAULT 'queued',
    message           TEXT,
    start_if_inactive BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### 状态机

```
queued -> delivered -> started -> completed
                            \-> failed
```

| 状态 | 含义 | 触发者 |
|------|------|--------|
| `queued` | 委托已创建，等待投递 | 系统（创建时默认） |
| `delivered` | 委托已投递给目标 Agent | 系统（通知发送后） |
| `started` | 目标 Agent 已接受并开始 | 目标 Agent |
| `completed` | 委托任务已完成 | 系统（关联任务完成时自动转换） |
| `failed` | 委托被拒绝或任务失败 | 目标 Agent 或系统 |

#### API 端点

```
POST   /api/agent-delegations              创建委托
GET    /api/agent-delegations              列表查询（支持 ?from_agent_id=&to_agent_id=&status=&channel_id=）
GET    /api/agent-delegations/:id           获取单条
PATCH  /api/agent-delegations/:id           更新状态（status, message）
```

#### 注意

Step 1 只建表和基础 CRUD。完整的委托协议（accept/reject/自动状态流转）在 Step 2.1 实现。

---

## 4. 验收标准

### 4.1 Agent Relationships

**F1 — 创建汇报关系**

```
GIVEN Agent Alice (id=a1) 和 Agent Bob (id=b1) 存在
WHEN 管理员 POST /api/agent-relationships with
     { from_agent_id: a1, to_agent_id: b1, rel_type: "reports_to" }
THEN 返回 201，关系记录创建成功
  AND 记录的 channel_id 为 NULL
  AND GET /api/agents/a1/relationships 返回包含该关系的列表
```

**F2 — 查询汇报关系**

```
GIVEN Alice reports_to Bob 已存在
WHEN 系统查询 GET /api/agents/a1/relationships?rel_type=reports_to
THEN 返回 to_agent_id=b1, rel_type=reports_to
WHEN 系统查询 GET /api/agents/b1/relationships?rel_type=reports_to&direction=incoming
THEN 返回 from_agent_id=a1, rel_type=reports_to
```

**F3 — 创建频道委托关系**

```
GIVEN Alice 和 Bob 存在，频道 #frontend (id=c1) 存在
WHEN 管理员 POST /api/agent-relationships with
     { from_agent_id: a1, to_agent_id: b1, rel_type: "delegates_to", channel_id: c1 }
THEN 返回 201
  AND channel_id=c1
  AND 同一 (a1, b1, delegates_to, c1) 重复创建返回 409
```

**F4 — 创建协作关系（自动补反向）**

```
GIVEN Alice 和 Bob 存在，频道 c1 存在
WHEN 管理员 POST /api/agent-relationships with
     { from_agent_id: a1, to_agent_id: b1, rel_type: "collaborates_with", channel_id: c1 }
THEN 返回 201
  AND 同时创建了反向记录 (b1, a1, collaborates_with, c1)
  AND 两条记录都可通过 API 查询到
```

**F5 — 创建升级关系**

```
GIVEN Alice 和 Supervisor Bob 存在
WHEN 管理员 POST /api/agent-relationships with
     { from_agent_id: a1, to_agent_id: b1, rel_type: "escalates_to" }
THEN 返回 201，channel_id 为 NULL
```

**F6 — 循环检测**

```
GIVEN Alice reports_to Bob，Bob reports_to Carol
WHEN 管理员尝试创建 Carol reports_to Alice
THEN 返回 409 Conflict
  AND 响应体包含 "Cycle detected: Carol -> Alice -> Bob -> Carol"
  AND 数据库中没有新增记录
```

**F6b — 自引用拒绝**

```
GIVEN Alice 存在
WHEN 管理员尝试创建 Alice reports_to Alice
THEN 返回 400 Bad Request
  AND 响应体包含 "Self-referencing relationship is not allowed"
```

**F6c — channel_id 约束校验**

```
GIVEN 频道 c1 存在
WHEN 管理员尝试创建 reports_to 并传入 channel_id=c1
THEN 返回 400 Bad Request
  AND 响应体包含 "reports_to must be global (channel_id must be null)"

GIVEN 频道 c1 存在
WHEN 管理员尝试创建 delegates_to 但不传 channel_id
THEN 返回 400 Bad Request
  AND 响应体包含 "delegates_to requires channel_id"
```

### 4.2 Task Dependencies

**F7 — 创建任务依赖**

```
GIVEN Task #2 (blocker) 和 Task #3 (blocked) 存在且属于同一频道
WHEN Agent POST /api/task-dependencies with
     { blocker_task_id: 2, blocked_task_id: 3 }
THEN 返回 201
  AND GET /api/tasks/3/blocked-by 返回 [Task #2]
  AND GET /api/tasks/2/blocks 返回 [Task #3]
```

**F8 — 查询依赖**

```
GIVEN Task #3 被 Task #2 阻塞，Task #3 阻塞 Task #5
WHEN Agent GET /api/tasks/3/dependencies
THEN 返回 blocked_by: [Task #2], blocks: [Task #5]
```

**F9 — 完成通知**

```
GIVEN Task #2 blocks Task #3，Bob 是 Task #3 的 claimer
WHEN Task #2 状态变为 "completed"
THEN 系统向 Bob 发送通知消息
  AND 消息包含 "Task #3 is now unblocked — Task #2 has been completed."
  AND 消息类型为 "task_unblocked"
```

**F9b — 循环依赖检测**

```
GIVEN Task #2 blocks Task #3，Task #3 blocks Task #5
WHEN Agent 尝试创建 Task #5 blocks Task #2
THEN 返回 409 Conflict
  AND 响应体包含 "Dependency cycle detected"
```

**F9c — 自依赖拒绝**

```
GIVEN Task #2 存在
WHEN Agent 尝试 POST { blocker_task_id: 2, blocked_task_id: 2 }
THEN 返回 400 Bad Request
  AND 响应体包含 "Task cannot depend on itself"
```

### 4.3 Delegations

**F10 — 创建委托记录**

```
GIVEN Alice (a1)、Bob (b1)、频道 c1、Task #3 存在
  AND Alice 在频道 c1 中有 delegates_to Bob 的关系
WHEN 系统 POST /api/agent-delegations with
     { from_agent_id: a1, to_agent_id: b1, task_id: 3, channel_id: c1, message: "请实现登录接口" }
THEN 返回 201
  AND status="queued"
  AND 记录可通过 GET /api/agent-delegations 查询
```

**F11 — 查询委托**

```
GIVEN 3 条委托存在：queued(Alice->Bob), delivered(Alice->Carol), completed(Bob->Alice)
WHEN Bob GET /api/agent-delegations?to_agent_id=b1&status=queued
THEN 返回 1 条记录（Alice->Bob queued）
WHEN Alice GET /api/agent-delegations?from_agent_id=a1
THEN 返回 2 条记录（Alice->Bob, Alice->Carol）
```

**F11b — 级联删除**

```
GIVEN Agent Bob 关联了 2 条委托记录 (from 和 to)
WHEN 管理员删除 Agent Bob
THEN 所有 from_agent_id=bob 或 to_agent_id=bob 的委托记录被级联删除
  AND 不出现孤儿记录
```

---

## 5. UI/UX 草图

Step 1 主要为后端基础，前端只有最小可用的管理界面。

### 5.1 Agent 详情页 — 关系 Tab（新增）

```
┌─────────────────────────────────────────────────────────┐
│ Agent: Alice                                    [编辑]  │
├─────────────────────────────────────────────────────────┤
│ [Profile] [Skills] [Tasks] [Relationships]  <-- 新增 Tab│
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ★ 组织关系 (全局)                                      │
│  ┌─────────────────────────────────────────────────┐   │
│  │ 汇报给: Bob (@bob)                        [解除] │   │
│  │ 升级给: Supervisor (@sup)                 [解除] │   │
│  │                                                   │   │
│  │ [+ 添加全局关系] 下拉: [reports_to | escalates_to]│   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ★ 频道关系                                             │
│  ┌─────────────────────────────────────────────────┐   │
│  │ #frontend                                         │   │
│  │   委托给: Carol (@carol)                   [解除] │   │
│  │   协作:   Bob (@bob), Dave (@dave)         [解除] │   │
│  │   [+ 添加频道关系] 下拉: [delegates_to | collab..]│   │
│  ├─────────────────────────────────────────────────┤   │
│  │ #backend                                          │   │
│  │   委托给: Eve (@eve)                       [解除] │   │
│  │   [+ 添加频道关系]                                │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 5.2 Kanban 任务卡片 — 依赖标记

```
┌──────────────────────┐
│ Task #3: 部署到生产   │
│ Assignee: @bob       │
│ ⛔ 等待 #2 完成       │  <-- 新增：红色标记
│ ⏳ 阻塞 #5            │  <-- 新增：黄色标记
│ [详情]               │
└──────────────────────┘
```

### 5.3 委托列表（最小可用版）

```
┌─────────────────────────────────────────────────────────┐
│ 委托管理                                                │
├─────────────────────────────────────────────────────────┤
│ 筛选: [全部 ▾] [queued] [delivered] [started] [completed]│
├─────────────────────────────────────────────────────────┤
│ 从        | 到      | 任务    | 状态      | 时间         │
│ Alice     | Bob     | #3      | queued    | 2 小时前    │
│ Bob       | Carol   | #5      | delivered | 昨天        │
│ Carol     | Alice   | -       | completed | 3 天前      │
└─────────────────────────────────────────────────────────┘
```

---

## 6. 非功能需求

### 6.1 性能

| 指标 | 目标 | 说明 |
|------|------|------|
| 关系查询 (单 Agent) | < 10ms | 索引覆盖，预期数据量小 |
| 关系图谱查询 (200 Agent) | < 50ms | 200 Agent 产生约 400-600 条关系边 |
| 循环检测 (深度 10) | < 5ms | BFS 内存内完成 |
| 任务依赖查询 | < 10ms | 单表主键查询 |

### 6.2 安全与权限

- 关系创建/删除需要管理员权限或 Agent 所有者权限
- `reports_to` 和 `escalates_to`（全局关系）只能由 server 级 admin 管理
- `delegates_to` 和 `collaborates_with`（频道关系）由频道 admin 或 Agent 所有者管理
- API 返回数据不泄露其他频道的 private 关系

### 6.3 兼容性

- `task_dependencies` 不能与现有 `parent_task_id` 冲突：parent_task_id 表示树形拆分，task_dependencies 表示 DAG 横向依赖，二者共存
- 删除任务的级联行为需与现有任务生命周期保持一致

---

## 7. 范围边界

### 本次做

- 三张表的 schema 定义与 migration
- 基础 CRUD API 端点
- `reports_to` 循环检测
- `collaborates_with` 自动补反向
- `channel_id` 作用域约束校验
- 完成任务后自动通知被阻塞任务的 claimer
- Agent 详情页新增 Relationships Tab（只读 + 简单创建/删除）
- Kanban 卡片显示依赖标记
- 委托记录的基础列表页

### 本次不做

- `solo agent delegate` CLI 命令（Step 2.1）
- 委托的 accept/reject 交互流程（Step 2.1）
- Task DAG 可视化（Step 2.2）
- 子任务批量拆分（Step 2.3）
- 频道共享记忆（Step 2.4）
- 关系图谱可视化（Step 2.5）
- 任何 Workspace 相关功能（Step 3）
- 知识库（Step 4）
- 拖拽关系编辑器（Step 5）
- Swarm / 定时唤醒（Step 6）

---

## 8. 依赖与风险

### 依赖

| 依赖项 | 说明 | 风险等级 |
|--------|------|----------|
| 现有 agents 表 | 需要外键引用 agents(id) | 低 — 已存在 |
| 现有 tasks 表 | 需要外键引用 tasks(id) 和任务完成事件 | 低 — 已存在 |
| 现有 channels 表 | 需要外键引用 channels(id) | 低 — 已存在 |
| 现有通知系统 | 任务完成时的自动通知依赖现有消息管道 | 中 — 需确认通知机制 |

### 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 关系数量膨胀（N 个 Agent 产生 N^2 条记录） | 查询性能下降 | 唯一索引限制重复创建；前端分页；如需要增加缓存层 |
| 循环检测 BFS 在极深层级下性能问题 | API 响应慢 | reports_to 深度通常 < 5，BFS 足够；设置最大遍历深度 50 |
| 与现有 parent_task_id 语义混淆 | 开发者误用 | 文档明确区分：parent = 树形拆分，dependency = DAG 横向依赖 |
| 频道删除后关系记录未清理 | 孤儿数据 | channel_id 外键 ON DELETE CASCADE |

---

## 9. 成功指标

### 上线验证

- [ ] Migration 在 staging 环境执行无错误
- [ ] 4 种关系类型的 CRUD API 全部通过集成测试
- [ ] 循环检测覆盖 3 层以上嵌套场景
- [ ] `collaborates_with` 自动补反向通过验证
- [ ] 任务完成 -> 通知被阻塞任务 claimer 的端到端流程通过
- [ ] Kanban 卡片显示依赖标记无 UI 报错

### 30 天观察指标

| 指标 | 目标 | 数据来源 |
|------|------|----------|
| 有至少 1 条关系记录的 Agent 比例 | > 60% | DB 查询 |
| delegations 表周新增记录数 | > 10 | DB 查询 |
| 任务依赖创建数 | > 5 / 周 | DB 查询 |
| API 错误率 | < 0.1% | 日志 |

---

## 10. 迭代规划

### Sprint F-1（Week 1-2）：建表 + Migration + API

- [ ] 编写三张表的 SQL migration 文件
- [ ] 实现 Agent Relationships CRUD（含循环检测、channel_id 约束）
- [ ] 实现 Task Dependencies CRUD（含循环检测、完成通知）
- [ ] 实现 Delegations CRUD
- [ ] API 集成测试覆盖所有验收标准

### Sprint F-2（Week 3）：前端最小适配

- [ ] Agent 详情页新增 Relationships Tab（只读列表 + 简单创建/删除）
- [ ] Kanban 任务卡片显示依赖标记（"等待 #N 完成" / "阻塞 #M"）
- [ ] 委托记录基础列表页（只读表格，筛选功能）
- [ ] 手动验收测试通过

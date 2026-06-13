# PRD — Step 2: Coordination + 共享记忆 + 只读图谱

> 版本: v1.0 | 日期: 2026-06-13 | 状态: Draft
> 前置依赖: [Step 1 — Foundation](step1-foundation.md)
> 关联文档: [Agent 协作路线图](../agent-collaboration-roadmap.md)

---

## 1. 产品背景与目标

### 1.1 现状（Step 1 完成后）

Step 1 建立了数据基础层——Agent 关系、任务依赖、委托记录三张表 + CRUD API。但用户仍无法**直接操作**这些关系：委托靠手动插数据库（或 API 调用），依赖标记不醒目，频道内 Agent 之间没有共享上下文，关系数据沉淀在表里但没有人能看到"全局图"。

### 1.2 目标

将 Step 1 的数据基础转化为**用户可感知、可操作的协作能力**：

1. **委托协议上线**：Agent 通过 CLI 正式委托任务，接受/拒绝有仪式感
2. **Task DAG 可见**：Kanban 卡片醒目标记依赖，阻塞任务灰显
3. **子任务拆分**：复杂任务一键拆成多个子任务，继承频道上下文
4. **频道共享记忆**：频道内所有 Agent 共享 CHANNEL.md 和 decisions.md，加入频道即获得上下文
5. **关系图谱初现**：纯 SVG 的只读组织图，可 pan/zoom，低投入验证"可视化有没有价值"

### 1.3 Step 2 完成后的协作体验

```
频道 #build-login 内:

1. 管理员绑定项目仓库，设置频道上下文
   -> CHANNEL.md: "本频道负责用户登录模块，技术栈 Go + PostgreSQL"

2. PM Agent 创建任务 #12 "实现 POST /api/login"
   -> solo task create --title "实现 POST /api/login" --depends-on 5

3. PM Agent 委托给 Backend Agent
   -> solo agent delegate --to @backend-bot --task 12 --msg "请实现登录接口"

4. Backend Agent 收到委托通知，查看频道上下文（CHANNEL.md）
   -> solo agent delegate list --status queued
   -> solo agent delegate accept <id>

5. Backend Agent 发现任务需要先等 #5 完成
   -> Kanban 卡片灰显 "等待 #5 完成"

6. Backend Agent 把任务拆成 3 个子任务
   -> solo task split -n 12 --titles "handler,service,test"

7. PM Agent 打开关系图谱，看到团队结构
   -> 只读 SVG: Alice -> Bob(reports_to), Alice -> Carol(delegates_to)

8. 任务完成后，Agent 把关键决策写入 decisions.md
   -> "2026-06-13: 登录接口用 bcrypt 而非 scrypt，因为 Go 标准库支持更好"
```

---

## 2. 用户故事地图

### 2.1 委托 CLI（delegate）

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| C1 | P0 | 作为 Agent，我希望通过 CLI 将任务正式委托给另一个 Agent，以便明确责任转移 | M |
| C2 | P0 | 作为 Agent，我希望查看待我处理的委托列表，以便知道有哪些任务被分配给我 | S |
| C3 | P0 | 作为 Agent，我希望接受或拒绝委托请求，以便对流入的工作有控制权 | M |
| C4 | P1 | 作为委托方，我希望收到委托被接受/拒绝的通知，以便了解任务是否被承接 | S |
| C5 | P1 | 作为 Agent，当委托任务完成时，委托方自动收到通知，以便闭环 | S |

### 2.2 Task DAG — CLI 与 Kanban

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| C6 | P0 | 作为 Agent，我希望创建任务时声明依赖关系（--depends-on），以便任务 DAG 在创建时就结构化 | S |
| C7 | P0 | 作为 Agent，我希望在 Kanban 上看到被阻塞的任务灰显并标注"等待 #N 完成"，以便一眼识别阻塞 | M |
| C8 | P0 | 作为 Agent，我希望查看自己被阻塞的任务列表（solo task list --blocked），以便优先解决阻塞 | S |
| C9 | P1 | 作为 Agent，当阻塞任务完成时，Kanban 卡片自动恢复可操作状态，以便无需手动刷新 | M |

### 2.3 子任务拆分

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| C10 | P1 | 作为 Agent，我希望将任务一键拆分为多个子任务，以便分解复杂工作 | M |
| C11 | P1 | 作为 Agent，我希望子任务自动继承父任务的频道和依赖，以便减少重复配置 | S |
| C12 | P2 | 作为 Agent，我希望父任务的完成进度自动根据子任务完成率计算，以便无需手动更新 | M |

### 2.4 频道共享记忆

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| C13 | P0 | 作为 Agent，加入频道时自动获得该频道的共享上下文（CHANNEL.md），以便理解频道的工作内容和规范 | M |
| C14 | P0 | 作为 Agent，我希望在频道内记录技术决策（decisions.md），以便其他 Agent 获取历史上下文 | M |
| C15 | P1 | 作为 Agent，我希望 CHANNEL.md 是所有频道成员可读写的，以便共同维护频道上下文 | S |
| C16 | P1 | 作为新加入频道的 Agent，我希望 System Prompt 自动注入 CHANNEL.md，以便无需手动翻阅 | M |

### 2.5 只读 SVG 关系图谱

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| C17 | P1 | 作为用户，我希望看到一个可视化图谱展示所有 Agent 之间的关系，以便理解团队结构 | L |
| C18 | P1 | 作为用户，我希望 4 种关系类型用不同线型区分，以便快速识别关系性质 | S |
| C19 | P2 | 作为用户，我希望图谱支持 pan 和 zoom，以便在 Agent 较多时也能浏览 | S |
| C20 | P2 | 作为用户，我希望点击图谱中的节点跳转到 Agent 详情页，以便深入查看 | S |

---

## 3. 功能详述

### 3.1 委托 CLI（solo agent delegate）

#### 命令清单

```bash
# 创建委托
solo agent delegate --to @Bob --task 3 --msg "请实现 POST /api/login"
# 可选参数 --start-if-inactive（Bob 处于 idle 超时时自动唤醒）

# 查看委托
solo agent delegate list                          # 所有相关委托
solo agent delegate list --status queued          # 待处理
solo agent delegate list --status delivered       # 已投递
solo agent delegate list --direction incoming     # 委托给我的
solo agent delegate list --direction outgoing     # 我发出的

# 接受/拒绝
solo agent delegate accept <delegation-id>
solo agent delegate reject <delegation-id> --reason "不擅长这块"
```

#### 委托状态机与交互流程

```
创建委托               投递通知           接受并开始          任务完成
  │                     │                  │                  │
  v                     v                  v                  v
queued ──────────> delivered ──────────> started ──────────> completed
                        │                    │
                        │ (目标拒绝)          │ (任务失败)
                        v                    v
                     failed               failed
```

| 状态转换 | 触发条件 | 副作用 |
|----------|----------|--------|
| queued -> delivered | 委托创建后系统自动投递通知 | 通知目标 Agent "你有一个新委托" |
| delivered -> started | 目标 Agent 执行 `solo agent delegate accept` | 关联任务的 claimer 变为目标 Agent；通知委托方 "Bob 已接受委托" |
| delivered -> failed | 目标 Agent 执行 `solo agent delegate reject` | 通知委托方 "Bob 拒绝了委托，原因：xxx" |
| started -> completed | 关联任务状态变为 completed | 通知委托方 "委托任务 #N 已完成" |
| started -> failed | 关联任务状态变为 failed/cancelled | 通知委托方 "委托任务 #N 失败" |

#### 与 @mention 的关系

```
简单协作: @Bob 帮我看下这个 → 走 @mention（非正式）
正式委托: solo agent delegate --to @Bob --task 3 → 走 delegate 协议（正式，有状态追踪）
```

委托不替代 @mention，两者共存。经验规则：涉及具体任务的用 delegate，临时讨论用 @mention。

#### 业务规则

- 委托创建前校验 `delegates_to` 关系：Alice 必须有对 Bob 的 `delegates_to` 关系（在委托所在频道内）
- 若目标 Agent 不存在 `delegates_to` 关系，返回提示 "Bob 未在频道 #xxx 中设置为你可委托的 Agent，请先创建 delegates_to 关系"
- 同一任务同一时刻只能有一个活跃委托（status IN queued/delivered/started）
- `start_if_inactive` 仅当目标 Agent 处于 idle 状态超过配置阈值（默认 30 分钟）时触发唤醒

### 3.2 Task DAG

#### CLI 扩展

```bash
# 创建任务时声明依赖
solo task create --title "部署到生产" --depends-on 5
# 支持多重依赖
solo task create --title "发布公告" --depends-on 8,9

# 事后添加/移除依赖
solo task block -n 3 --on 2       # Task #3 依赖 #2
solo task unblock -n 3 --on 2     # 移除依赖

# 查询阻塞
solo task list --blocked           # 列出我被阻塞的任务
solo task list --blocking          # 列出我阻塞了别人的任务
solo task deps -n 3                # 查看 Task #3 的完整依赖链（递归上游和下游）
```

#### Kanban 前端表现

**正常状态卡片：**
```
┌─────────────────────────────┐
│ #3 部署到生产                │
│ 🧑 @bob                     │
│ 📋 In Progress              │
│ [详情]                      │
└─────────────────────────────┘
```

**被阻塞卡片（灰显）：**
```
┌─────────────────────────────┐
│ #3 部署到生产                │  <- 整体 opacity 0.5
│ 🧑 @bob                     │
│ ⛔ 等待 #2 完成 (实现 CI/CD) │  <- 红色横幅
│ 📋 Blocked                  │  <- 状态自动更新为 Blocked
│ [详情] (不可拖拽移动)        │
└─────────────────────────────┘
```

**阻塞他人的卡片：**
```
┌─────────────────────────────┐
│ #2 实现 CI/CD                │
│ 🧑 @alice                   │
│ ⏳ 阻塞 #3, #5              │  <- 黄色标记
│ 📋 In Progress              │
│ [详情]                      │
└─────────────────────────────┘
```

#### 业务规则

- 当任务的所有 blocker 都完成后，任务状态从 Blocked 自动恢复为原来的状态（或 Todo）
- 任务被标记为 Blocked 时不可拖拽到其他泳道（除非手动 unblock）
- 依赖关系与 `parent_task_id` 独立：parent 是树形拆分，dependency 是 DAG 横向依赖
- 子任务可以依赖父任务之外的任意任务（跨树依赖）

### 3.3 子任务批量拆分

#### CLI

```bash
# 批量创建子任务
solo task split -n 12 --titles "handler,service,test"

# 等价于
# solo task create --title "handler" --parent 12
# solo task create --title "service" --parent 12
# solo task create --title "test"   --parent 12
```

#### 业务规则

- 子任务自动继承父任务的 `channel_id`
- 子任务自动继承父任务的 `assignee`（若父任务已分配）
- 子任务默认状态为 Todo
- 父任务进度计算公式：`completed_child_count / total_child_count`
- 子任务的子任务（嵌套拆分）最多 2 层（父 -> 子 -> 孙）。更深层级通过扁平化引导（拆成多个同级子任务）

#### 前端 — 父任务卡片内嵌子任务列表

```
┌─────────────────────────────────────┐
│ #12 实现 POST /api/login             │
│ 🧑 @backend-bot                     │
│ 📊 进度: 2/3 (67%)                  │  <- 进度条
│ ┌─────────────────────────────────┐ │
│ │ ✅ #12.1 handler                │ │
│ │ ✅ #12.2 service                │ │
│ │ ⬜ #12.3 test                   │ │
│ └─────────────────────────────────┘ │
│ [+ 添加子任务]                       │
│ [详情]                               │
└─────────────────────────────────────┘
```

### 3.4 频道共享记忆

#### 文件结构

```
~/.solo/channels/<channel-id>/
  └── memory/
      ├── CHANNEL.md       # 频道上下文（所有 Agent 可读写，启动时自动加载）
      └── decisions.md     # 频道内的技术决策记录（追加式写入，不可删除）
```

#### CHANNEL.md 约定格式

```markdown
# 频道: #build-login

## 目标
实现用户登录与注册功能模块。

## 技术栈
- 后端: Go 1.22 + net/http
- 数据库: PostgreSQL 16
- 认证: JWT + bcrypt

## 项目仓库
https://github.com/solo/login-service

## 约定
- API 路径统一使用 /api/v1/ 前缀
- 所有接口需要 rate limit
- commit message 格式: feat(scope): description

## 活跃 Agent
- @alice — PM / 协调
- @backend-bot — 后端开发
- @reviewer-bot — 代码审查
```

#### decisions.md 格式

```markdown
# 技术决策记录

## 2026-06-13 — 密码哈希方案
- 决策: 使用 bcrypt 而非 scrypt
- 原因: Go 标准库支持更好，性能满足需求（< 100ms/hash）
- 决策者: @backend-bot
- 状态: 已采纳

## 2026-06-14 — API 版本策略
- 决策: URL 路径版本（/api/v1/）而非 Header 版本
- 原因: 简单直观，团队一致同意
- 决策者: @alice
- 状态: 已采纳
```

#### System Prompt 注入

Agent 在频道内被唤醒或首次加入频道时，System Prompt 自动注入：

```
[Channel Context: #build-login]
<CHANNEL.md 的内容>

[Recent Decisions]
<decisions.md 最近 5 条记录>
```

#### 权限模型

| 操作 | Agent (频道成员) | Agent (非成员) | Admin |
|------|-----------------|----------------|-------|
| 读取 CHANNEL.md | ✅ | ❌ | ✅ |
| 修改 CHANNEL.md | ✅ | ❌ | ✅ |
| 读取 decisions.md | ✅ | ❌ | ✅ |
| 追加 decisions.md | ✅ | ❌ | ✅ |
| 删除 decisions.md 条目 | ❌ | ❌ | ✅ |

### 3.5 只读 SVG 关系图谱

#### 技术约束

- 纯 SVG 实现，约 150 行代码，**零新依赖**（不引入 d3/ReactFlow 等重量库）
- 数据来源：`GET /api/agent-relationships`
- 不依赖 Step 3 的 Workspace

#### 视觉设计

**线型区分：**

| 关系类型 | 线型 | 颜色 | 说明 |
|----------|------|------|------|
| `reports_to` | 实线竖线 | #4A90D9 (蓝) | 组织层级，从上到下 |
| `delegates_to` | 实线横线 | #7ED321 (绿) | 同级委托，从左到右 |
| `collaborates_with` | 虚线横线 | #F5A623 (橙) | 同级协作 |
| `escalates_to` | 双实线红色 | #D0021B (红) | 升级路径 |

**节点设计：**

```
     ┌─────────────────┐
     │   🟢 Alice       │   <- 绿点 = 在线
     │   PM Agent       │
     └────────┬────────┘
              │ (蓝色实线竖线 — reports_to)
     ┌────────┴────────┐
     │   🟢 Bob         │
     │   Backend Agent  │
     └──┬───────────┬──┘
        │ (绿实线)  │ (橙虚线)
   ┌────┴────┐  ┌──┴──────┐
   │ Carol   │  │  Dave   │
   │ Reviewer│  │  Front  │
   └─────────┘  └─────────┘
```

#### 交互

- **Pan**：鼠标拖拽空白区域平移画布
- **Zoom**：滚轮缩放（0.3x ~ 3x）
- **节点点击**：跳转到 `/agents/:id`
- **Hover 节点**：显示 tooltip（Agent 名称、角色、在线状态）
- **Hover 边**：显示关系类型和所属频道（如有）

#### 入口

- 侧边栏新增菜单项 "关系图谱"（仅当 Agent >= 3 时显示）
- 或在已有 Dashboard 页面嵌入小型图谱卡片

---

## 4. 验收标准

### 4.1 委托 CLI

**C1 — 创建委托**

```
GIVEN Alice 在频道 #frontend 中有对 Bob 的 delegates_to 关系
  AND Task #3 存在且属于频道 #frontend
WHEN Alice 执行
  solo agent delegate --to @Bob --task 3 --msg "请实现登录接口"
THEN 终端输出 "Delegation created: #<uuid> [queued]"
  AND POST /api/agent-delegations 被调用
  AND status="queued"
  AND 系统向 Bob 发送通知 "Alice 委托你处理 Task #3: 请实现登录接口"
```

**C1b — 无委托关系时拒绝**

```
GIVEN Alice 在频道 #frontend 中没有对 Bob 的 delegates_to 关系
WHEN Alice 执行 solo agent delegate --to @Bob --task 3
THEN 终端输出错误 "Bob 未在频道 #frontend 中设置为你可委托的 Agent"
  AND 退出码非 0
  AND 未创建委托记录
```

**C2 — 查看委托列表**

```
GIVEN Alice 有 2 条 incoming 委托（queued），1 条 outgoing（started）
WHEN Alice 执行 solo agent delegate list --direction incoming --status queued
THEN 终端显示 2 条记录，包含 id、from、task、message、created_at
WHEN Alice 执行 solo agent delegate list --direction outgoing
THEN 终端显示 1 条记录，status=started
```

**C3 — 接受委托**

```
GIVEN Bob 有一条来自 Alice 的 queued 委托 #D1（关联 Task #3）
WHEN Bob 执行 solo agent delegate accept D1
THEN 委托状态变为 started
  AND Task #3 的 claimer 变为 Bob
  AND Alice 收到通知 "Bob 已接受你的委托"
```

**C4 — 拒绝委托**

```
GIVEN Bob 有一条来自 Alice 的 queued 委托 #D1
WHEN Bob 执行 solo agent delegate reject D1 --reason "不擅长前端"
THEN 委托状态变为 failed
  AND Alice 收到通知 "Bob 拒绝了你的委托，原因: 不擅长前端"
```

**C5 — 完成闭环通知**

```
GIVEN 委托 #D1 状态为 started，关联 Task #3
WHEN Task #3 状态变为 completed
THEN 委托 #D1 状态自动变为 completed
  AND Alice 收到通知 "委托任务 #3 已完成"
```

### 4.2 Task DAG

**C6 — 创建带依赖的任务**

```
GIVEN Task #5 存在
WHEN Agent 执行 solo task create --title "部署" --depends-on 5
THEN 任务创建成功
  AND GET /api/tasks/<new_id>/blocked-by 返回 [Task #5]
  AND 新任务状态自动设置为 Blocked
```

**C7 — Kanban 阻塞显示**

```
GIVEN Task #3 depends_on Task #2
WHEN 用户打开 Kanban 视图
THEN Task #3 卡片显示为灰显（opacity 0.5）
  AND 卡片包含红色横幅 "等待 #2 完成"
  AND 卡片不可拖拽到其他状态泳道
  AND Task #2 卡片显示黄色角标 "阻塞 #3"
```

**C8 — CLI 查询阻塞**

```
GIVEN Task #3 被 #2 阻塞，Task #5 被 #3 阻塞
WHEN Agent 执行 solo task list --blocked
THEN 终端输出 Task #3 (blocked by #2) 和 Task #5 (blocked by #3)
WHEN Agent 执行 solo task deps -n 3
THEN 终端输出完整依赖链: #3 blocked by #2; #3 blocks #5
```

**C9 — 阻塞自动解除**

```
GIVEN Task #2 (blocker of #3) 状态变为 completed
WHEN 系统检测到该事件
THEN Task #3 状态从 Blocked 恢复为 Todo
  AND Kanban 卡片恢复原色，移除红色横幅
  AND 通知 Task #3 的 claimer
```

### 4.3 子任务拆分

**C10 — 批量创建子任务**

```
GIVEN Task #12 存在
WHEN Agent 执行 solo task split -n 12 --titles "handler,service,test"
THEN 3 个子任务创建成功
  AND 每个子任务的 parent_task_id=12
  AND 每个子任务的 channel_id 与父任务相同
  AND 终端输出 3 个子任务的 ID 和标题
```

**C11 — 子任务继承**

```
GIVEN Task #12 属于频道 #frontend，assignee=@backend-bot
WHEN 创建子任务
THEN 子任务 channel_id=#frontend
  AND 子任务 assignee=@backend-bot
```

**C12 — 父任务进度**

```
GIVEN Task #12 有 3 个子任务，其中 2 个 completed
WHEN 查询 Task #12
THEN progress=66.7%
  AND Kanban 卡片显示进度条 "2/3 (67%)"
```

### 4.4 频道共享记忆

**C13 — 频道上下文加载**

```
GIVEN 频道 #frontend 的 CHANNEL.md 包含技术栈信息
WHEN Backend Agent 加入频道 #frontend
THEN System Prompt 包含 CHANNEL.md 的完整内容
  AND 标记为 [Channel Context: #frontend]
```

**C14 — 技术决策追加**

```
GIVEN Backend Agent 在频道 #frontend 中做出技术决策
WHEN Backend Agent 执行
  solo channel decision --channel #frontend --msg "使用 bcrypt 而非 scrypt，因为 Go 标准库支持更好"
THEN decisions.md 末尾追加一条记录
  AND 记录包含时间戳、Agent ID、决策内容
  AND 记录不可被非管理员删除
```

**C15 — 多人读写**

```
GIVEN Alice 和 Bob 都是频道 #frontend 的成员
WHEN Alice 修改 CHANNEL.md 添加 "新约定: API 路径统一 /api/v1/"
  AND Bob 随后在频道中打开 System Prompt
THEN Bob 看到包含 Alice 最新修改的 CHANNEL.md
```

**C16 — System Prompt 注入**

```
GIVEN 频道 #frontend 的 CHANNEL.md 有 200 行，decisions.md 有 15 条记录
WHEN Agent 在频道 #frontend 中被唤醒
THEN System Prompt 注入完整 CHANNEL.md
  AND 注入最近 5 条 decisions（按时间倒序）
  AND 注入内容用分隔符与 Agent 自身 Prompt 区分
```

### 4.5 只读 SVG 图谱

**C17 — 图谱渲染**

```
GIVEN 系统中存在 4 个 Agent，5 条关系记录
WHEN 用户打开 "关系图谱" 页面
THEN 页面渲染 SVG 图谱
  AND 4 个 Agent 节点可见，位置按树状 layout 排列
  AND 5 条边可见，线型按关系类型区分
  AND 页面加载时间 < 500ms
```

**C18 — 线型区分**

```
GIVEN 图谱中存在 4 种关系类型各至少 1 条
WHEN 用户查看图谱
THEN reports_to 边为蓝色实线竖线
  AND delegates_to 边为绿色实线横线
  AND collaborates_with 边为橙色虚线横线
  AND escalates_to 边为红色双实线
```

**C19 — Pan/Zoom**

```
GIVEN 图谱页面已加载
WHEN 用户滚动鼠标滚轮
THEN 图谱按比例缩放（0.3x ~ 3x）
WHEN 用户拖拽空白区域
THEN 图谱平移
WHEN 用户双击空白区域
THEN 图谱重置为默认视图（fit to screen）
```

**C20 — 节点点击跳转**

```
GIVEN 图谱中显示 Agent Alice 的节点
WHEN 用户点击该节点
THEN 浏览器导航到 /agents/<alice-id>
```

---

## 5. UI/UX 草图

### 5.1 关系图谱页面（新增）

```
┌──────────────────────────────────────────────────────────┐
│ [关系图谱]                           [全局] [频道#frontend ▾]│
├──────────────────────────────────────────────────────────┤
│                                                          │
│                    ┌──────────┐                          │
│                    │ 🟢 Alice │  <- 节点（点击跳转详情）  │
│                    │ PM Agent │                          │
│                    └────┬─────┘                          │
│                    ─────┼─────  <- 蓝色实线竖线           │
│                         │        (reports_to)            │
│                    ┌────┴─────┐                          │
│                    │ 🟢 Bob   │                          │
│                    │ Backend  │                          │
│                    └──┬───┬───┘                          │
│              ─────────┘   └─────────                     │
│         (绿实线横)              (橙虚线横)                │
│      delegates_to         collaborates_with             │
│         ┌──┴──────┐     ┌──┴──────┐                     │
│         │ 🟡 Carol│     │ 🔴 Dave │  <- 黄=away 红=离线 │
│         │Reviewer │     │ Frontend│                     │
│         └─────────┘     └─────────┘                     │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 图例: ─── reports_to  ─── delegates_to            │  │
│  │       - - collaborates_with  ═══ escalates_to      │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  [筛选: 全部关系 ▾]  [导出 SVG]                          │
└──────────────────────────────────────────────────────────┘
```

### 5.2 Kanban — 带依赖和子任务

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│   Todo      │  │ In Progress │  │  Blocked    │  │   Done      │
├─────────────┤  ├─────────────┤  ├─────────────┤  ├─────────────┤
│┌───────────┐│  │┌───────────┐│  │┌───────────┐│  │┌───────────┐│
││ #4        ││  ││ #2 CI/CD  ││  ││ #3 部署   ││  ││ #1 Init   ││
││ Register  ││  ││ @alice    ││  ││ @bob      ││  ││ ✅        ││
││ API       ││  ││ ⏳阻塞 #3 ││  ││ ⛔等 #2   ││  ││           ││
││           ││  ││           ││  ││ (灰显)    ││  ││           ││
│└───────────┘│  │└───────────┘│  │└───────────┘│  │└───────────┘│
│             │  │             │  │             │  │             │
│┌───────────┐│  │┌───────────┐│  │             │  │             │
││ #5 公告   ││  ││ #12 Login ││  │             │  │             │
││ @eve      ││  ││ 📊2/3 67%││  │             │  │             │
││           ││  ││ ✅handler ││  │             │  │             │
││           ││  ││ ✅service ││  │             │  │             │
││           ││  ││ ⬜test    ││  │             │  │             │
│└───────────┘│  │└───────────┘│  │             │  │             │
└─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘
```

### 5.3 委托终端交互流程

```
$ solo agent delegate --to @bob --task 3 --msg "请实现登录接口"

Delegation #d4f2 created [queued]
  To:     @bob (Backend Agent)
  Task:   #3 实现 POST /api/login
  Status: queued → awaiting delivery

Notified @bob: "Alice 委托你处理 Task #3: 请实现登录接口"

---

$ solo agent delegate list --direction incoming --status queued

Pending delegations for @bob:
  D4F2  from @alice  Task #3  2 mins ago  "请实现登录接口"
  A7B3  from @carol  Task #7  1 hour ago  "Review PR #42"
Total: 2

---

$ solo agent delegate accept d4f2

Delegation #d4f2 accepted
  Status: queued → started
  Task #3 now assigned to you (@bob)

Notified @alice: "Bob 已接受你的委托"

---

$ solo agent delegate reject a7b3 --reason "正在忙 #3，无法同时处理"

Delegation #a7b3 rejected
  Status: delivered → failed
  Reason: 正在忙 #3，无法同时处理

Notified @carol: "Bob 拒绝了你的委托，原因: 正在忙 #3，无法同时处理"
```

---

## 6. 非功能需求

### 6.1 性能

| 指标 | 目标 |
|------|------|
| 委托 CLI 命令响应时间 | < 500ms（本地 CLI 调用 + API 往返） |
| Kanban 卡片依赖标记渲染 | 不增加首屏加载时间（< 额外 50ms） |
| SVG 图谱渲染（50 Agent, 200 条边） | < 1s |
| CHANNEL.md System Prompt 注入 | < 50ms（文件读取） |

### 6.2 安全与权限

- 委托创建需校验 `delegates_to` 关系是否存在
- decisions.md 追加操作记录 Agent ID，不可篡改历史
- CHANNEL.md 修改需频道成员权限
- 图谱数据不泄露其他频道的关系（频道级图谱隔离）

### 6.3 可用性

- CLI 命令输出使用统一的颜色方案：成功=绿，警告=黄，错误=红
- CLI 输出支持 `--json` flag 输出机器可读格式
- Kanban 依赖标记的灰显对比度需满足 WCAG AA（对比度 >= 4.5:1）
- SVG 图谱支持键盘导航（Tab 切换节点，Enter 跳转）

---

## 7. 范围边界

### 本次做

- solo agent delegate CLI（创建/查看/接受/拒绝）
- 委托状态机（queued -> delivered -> started -> completed/failed）
- 委托通知（创建/接受/拒绝/完成）
- Task DAG CLI（create --depends-on, block, unblock, deps, list --blocked）
- Kanban 卡片阻塞标记（灰显 + 横幅 + 禁止拖拽）
- 阻塞自动解除（状态恢复 + 通知）
- solo task split CLI
- 子任务继承（频道 + assignee）
- 父任务进度条
- CHANNEL.md / decisions.md 文件管理
- System Prompt 自动注入
- 只读 SVG 图谱（4 种线型 + pan/zoom + 点击跳转）

### 本次不做

- 拖拽编辑关系（Step 5）
- 频道共享工作区 / git worktree（Step 3）
- pgvector 知识库（Step 4）
- 委托中的 `start_if_inactive` 自动唤醒逻辑（与定时唤醒一起在 Step 6 实现）
- 委托的 WebSocket 实时推送（使用现有轮询或通知机制）
- decisions.md 的全文搜索（Step 4 知识库实现）
- 图谱的导出为图片功能

---

## 8. 依赖与风险

### 依赖

| 依赖项 | 说明 | 风险等级 |
|--------|------|----------|
| Step 1 三张表 + API | 委托、关系、依赖的数据层必须可用 | 低 — Step 1 已先行 |
| 现有通知系统 | 委托各阶段的通知依赖消息投递 | 中 — 需确认可靠投递 |
| 现有任务系统 | 子任务拆分依赖 parent_task_id | 低 — 字段已存在 |
| 现有文件系统访问 | CHANNEL.md 读写依赖 Agent 的文件系统权限 | 低 — 已有 |
| 现有前端路由 | 图谱页面需要新增路由 | 低 |

### 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 委托状态机在异常情况下卡死（如 Agent 离线） | 委托永远 stuck | 增加超时机制：delivered 状态超过 24h 未处理自动 failed |
| 子任务拆分过多导致 Kanban 视图混乱 | 用户体验差 | 单次拆分限制最多 10 个子任务 |
| CHANNEL.md 并发写入冲突 | 文件内容丢失 | 写入时使用文件锁（flock），冲突时提示重试 |
| SVG 图谱在大量 Agent 下性能差 | 页面卡顿 | 超过 50 个 Agent 时自动切换为"按频道筛选"模式；虚拟化渲染节点 |
| 委托与 @mention 语义重叠 | 用户困惑用哪个 | 在 CLI help 和文档中明确区分：正式任务用 delegate，临时交流用 @mention |

---

## 9. 成功指标

### 上线验证

- [ ] solo agent delegate 全流程（create/list/accept/reject）通过端到端测试
- [ ] 委托状态机 5 种状态转换全部通过
- [ ] 委托通知在创建/接受/拒绝/完成 4 个节点正确投递
- [ ] Kanban 卡片在被阻塞时灰显，阻塞解除后恢复
- [ ] solo task split 批量创建子任务通过
- [ ] 子任务自动继承频道和 assignee
- [ ] CHANNEL.md 修改后新 Agent 加入频道能看到最新内容
- [ ] decisions.md 追加后 System Prompt 能读取到最近条目
- [ ] SVG 图谱在 3/10/30 个 Agent 下都能正常渲染和交互
- [ ] 图谱 filter 切换"全局/频道"正确

### 30 天观察指标

| 指标 | 目标 | 数据来源 |
|------|------|----------|
| 委托创建数 | >= 20 / 周 | agent_delegations 表 |
| 委托接受率 | >= 70% | agent_delegations 表 (started+completed / total) |
| 带依赖的任务占比 | >= 10% | task_dependencies 表 |
| 频道中有 CHANNEL.md 的频道占比 | >= 50% | 文件系统 |
| decisions.md 周新增条目 | >= 5 | 文件系统 |
| 图谱页面 PV | >= 10 / 天 | 前端埋点 |
| 图谱页面跳出率 | < 60% | 前端埋点 |

---

## 10. 迭代规划

### Sprint C-1（Week 1-2）：委托 CLI + 通知

- [ ] 实现 solo agent delegate create/list/accept/reject
- [ ] 实现委托状态机
- [ ] 实现委托各阶段通知
- [ ] 委托 CLI 集成测试

### Sprint C-2（Week 2-3）：Task DAG + 子任务

- [ ] 实现 solo task create --depends-on / block / unblock / deps
- [ ] 实现 Kanban 阻塞标记（灰显 + 横幅 + 禁止拖拽）
- [ ] 实现 solo task split
- [ ] 实现父任务进度条
- [ ] 阻塞自动解除（任务完成事件 -> 状态恢复 + 通知）

### Sprint C-3（Week 3-4）：共享记忆 + SVG 图谱

- [ ] 实现 CHANNEL.md / decisions.md 文件管理
- [ ] 实现 System Prompt 注入
- [ ] 实现只读 SVG 图谱（节点 + 边 + 4 种线型）
- [ ] 实现 pan/zoom + 点击跳转
- [ ] 集成测试 + 手动验收

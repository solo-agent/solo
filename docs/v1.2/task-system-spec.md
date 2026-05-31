# Solo 任务系统 — 产品设计文档 v1.0

> 综合 Slock 任务系统设计 + Multica Issue PRD + Solo 当前实现
> 日期: 2026-05-15
> 替换旧版 task-system-spec.md

---

## 一、设计哲学：任务即消息

**核心决策：采用 Slock 模型，不采用 Multica Issue（工单）模型。**

| 设计原则 | 来源 | 说明 |
|---------|------|------|
| **任务即消息** | Slock | 任务是带元数据的消息，不另起炉灶 |
| **认领制，非指派制** | Slock | Agent 自主认领，静默冲突 |
| **状态广播** | Slock | system message 全频道透明 |
| **线程即工作日志** | Slock | 任务讨论天然绑定线程 |
| **频道级编号** | Slock | 每个频道独立 #1, #2, #3 |
| **Kanban 可视化** | Solo 自研 | 5 列看板，全局+频道+DM 统一 |

**为什么不采用 Multica Issue 模型**：
Multica 是工单系统（Jira-like），Solo 是频道协作平台（Slack-like）。Multica 的 assignee/labels/projects/subscribers/attachments/autopilot 太重，不符合"轻量协作"定位。吸收 Multica 精华（状态机、执行记录），保持 Slock 轻量哲学。


### 1.4 六个关键设计决策（Slock 借鉴评审）

| # | 功能 | 决策 | 优先级 | 说明 |
|---|------|------|--------|------|
| 1 | asTask（消息→任务） | ✅ 借鉴 | P0 | hover 消息 → "转为任务" → 标题预填 |
| 2 | 直接创建 task | ✅ 已有 | — | 弹窗创建，加"发送到频道"toggle |
| 3 | @软指派+认领 | ⚠️ 改良 | P0 | @Agent 有 30s 优先窗口，超时放开认领 |
| 4 | View in Channel | ✅ 借鉴 | P1 | 任务详情 → 跳转到频道消息位置 |
| 5 | Reply 复用 Thread | ✅ 借鉴 | P0 | 废弃独立评论区，用 ThreadPanel |
| 6 | Agent 线程内回应 | ✅ 借鉴 | P0 | system prompt 注入认领/完成协议 |

#### 决策 1: asTask
- 实现: `POST /api/v1/channels/{channelID}/messages/{messageID}/convert-to-task`
- 前端: MessageItem hover 时显示 "As Task" 按钮
- 标题预填为原消息内容（可编辑）

#### 决策 3: @软指派 + 认领制
- 创建任务时 @Agent → Agent 有 30s 优先认领窗口
- 30s 后未认领 → 其他任何人可认领
- 未 @ → 纯认领制，先到先得
- 认领失败 → 静默（无提示、无报错）

#### 决策 5: 废弃 TaskDetailPanel 评论区
- 当前: TaskDetailPanel 有独立评论输入框
- 目标: 去掉评论输入框，改为 "在 Thread 中讨论" 按钮
- 复用 ThreadPanel 组件
- 任务消息的 thread_id 绑定到 task.thread_id

#### 决策 6: Agent 任务协议
```
## Task Protocol
- New task in channel → evaluate if you can handle it
- Can handle → claim it immediately
- After claim → reply in thread "收到 #{number} {title}，开始处理"
- After completion → reply in thread "已完成 #{number}，请审核"
- Cannot handle → do NOT claim, ignore
```

## 二、数据模型

### 2.1 Task 实体（与当前实现的差异）

```
当前:  assignee_id + assignee_type（指派制）
目标:  claimer_id（认领制，单一 UUID，人或 Agent）

当前:  全局自增编号
目标:  频道内自增编号（每个频道独立 #1#2#3）

当前:  独立存在的实体
目标:  任务关联消息（task.message_id → messages.id）

移除:  priority, due_date（过重，不符合轻量定位）
```

### 2.2 状态机

```
todo ──claim──→ in_progress ──submit──→ in_review ──approve──→ done
 │                  │                      │
 └──close──→ closed  └──close──→ closed    └──close──→ closed

unclaim: 任意非终态 → todo
```

**关键约束**（来自 Slock）：
- 状态只能前进，不能回退
- done 和 closed 是终态
- 认领失败 = 静默（不抛错、不通知）

## 三、核心流程

### 3.1 创建任务
- **直接创建**：频道内/全局 → 弹窗（标题+描述）→ 发送 task message → 广播
- **As Task**：hover 消息 → "转为任务" → 标题预填 → 提交
- **Agent 创建**：Agent 在工作流中 create 子任务

### 3.2 认领制（Claim Model）
- 任何成员（人或 Agent）可认领
- 已被认领的 → 静默失败
- 认领人可释放（unclaim）
- @mention 是软建议，不强制指派

### 3.3 频道内展示
- 任务消息在消息流中有特殊样式
- 频道 Tasks Tab = Kanban Board 5 列（和全局 /tasks 一致）
- DM 也支持任务（独立编号）

## 四、与当前实现的 Gap（用户反馈的三个问题）

### 4.1 已评审的设计决策（详见 §1.4）

全部 6 个 Slock 功能已评审：1✅ 2✅ 3⚠️改良 4✅ 5✅ 6✅

---

### 问题 1: 频道任务和全局任务表现不一致 → P0 必修
- **根因**: 频道 Tasks Tab 用旧 TaskList 组件
- **修复**: 统一用 TaskBoard（Kanban 5 列）

### 问题 2: 任务和 Agent 毫无关联 → P0 必修
- **根因**: 无认领机制，无 As Task，无消息转任务
- **修复**: 
  - a) 认领 API + UI（claim/unclaim）
  - b) As Task（消息 → 任务转换）
  - c) Agent system prompt 加入任务认领指令

### 问题 3: 私聊没有任务 → P1
- **根因**: DM 频道未启用任务功能
- **修复**: DM 适配 TaskBoard，独立编号

## 五、实施计划

| Phase | 内容 | 预估 |
|-------|------|------|
| **Phase 1** | 认领制改造 + asTask + 频道看板统一 | 2-3 天 |
| **Phase 2** | Agent 任务协议 + 系统消息完善 | 1-2 天 |
| **Phase 3** | DM 任务支持 | 1 天 |

## 六、验收场景

**场景 1: 认领执行**
用户创建任务 → Agent 认领 → in_progress → submit → in_review → approve → done

**场景 2: As Task**
hover 消息 → "转为任务" → 新任务出现 TODO 列

**场景 3: 静默冲突**
task #1 已被 Agent A 认领 → Agent B 认领 → 无任何提示（静默失败）

**场景 4: DM 任务**
DM 对话中创建任务 → 仅双方可见 → 独立 TaskBoard

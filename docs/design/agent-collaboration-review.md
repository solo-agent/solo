# Agent 协作关系 — 深度设计评审

> 2026-06-14 | 从验收驱动转为场景驱动 | 5 个独立事项

---

## 背景

Agent 协作 6 步路线图已全部实现（14 commits, feat/collab-step1-foundation）。但验收测试发现：**建了表、画了图、API 通了——但关系的运行时行为接线不全。**

5 个事项中，事项 1 是本次重点（关系→行为接线），事项 2-5 是独立模块。

---

## 事项 1：Agent 关系 → 行为接线 🔴 核心

### 当前状态

```
agent_relationships 表 (4 种类型):
  reports_to       → DB ✓ → 图 ✓ → 行为 ✗
  delegates_to     → DB ✓ → 图 ✓ → 行为 ✗
  collaborates_with → DB ✓ → 图 ✓ → 行为 ✗
  escalates_to     → DB ✓ → 图 ✓ → 行为 ✓ (唯一接通)
```

### 为什么必须接

Solo 是频道式协作，没有 issue assign 机制。Agent 需要从关系推导出协作决策。参考项目（Rudder/Paperclip）不接是因为它们的 issue-driven 模型已经通过 assign 承担了"谁干什么"的决策。

### 三条待接线

| 关系 | 注入点 | 行为 |
|------|--------|------|
| `delegates_to` | delegate CLI / API | 委托前校验：只能委托给已声明该关系的 Agent |
| `reports_to` | System Prompt (daemon) | Agent 启动时注入"你报告给 @X"，完成任务后主动通知 |
| `collaborates_with` | System Prompt + Swarm | 任务分解和求助时，优先推荐协作者 |

---

## 事项 2：任务编排 → 独立模块

### 包含能力
- 任务依赖 DAG（`task_dependencies` + BFS 循环检测）
- 子任务拆分（`parent_task_id` + 批量创建）
- Swarm 蜂群分解（自动分解 + 并行认领 + 自动完成）
- Kanban 阻塞标记（灰色卡片 + Blocked badge）

### 待评审
- 依赖图的 UI 编辑完整性
- Swarm 分解策略
- 完成通知链路（blocker 完成→自动通知 blocked 的 claimer）

---

## 事项 3：共享知识系统 → 独立模块

### 包含能力
- 频道 CHANNEL.md（共享上下文，Agent 启动时注入）
- 频道 decisions.md（决策记录，可导入知识库）
- 结构化知识库（`knowledge` 表 + pgvector + semantic/FTS 混合搜索）
- 跨频道知识发现

### 待评审
- CHANNEL.md 的写入权限模型
- 知识条目的生命周期（谁可以编辑/删除）
- 搜索准确率

---

## 事项 4：自动化系统 → 独立模块

### 包含能力
- Reminders（定时提醒 + daemon 30s ticker + recurring + CLI）
- Watchdog（任务看门狗 + 3 级 escalation: remind → escalate → unclaim）
- Escalation chain（沿 `escalates_to` 升级，不存在则直接 unclaim）

### 待评审
- Reminder 的 cron 表达式覆盖度
- Watchdog 的超时策略
- 自动化触发对频道消息的噪音控制

---

## 事项 5：工作区系统 → 独立模块

### 包含能力
- 频道项目绑定（`channel_bindings` + repo clone + 文件树）
- Git worktree 任务隔离（创建/清理 + CWD 自动解析）
- Agent 启动时 CWD 3 层 fallback（worktree → channel → agent workspace）

### 待评审
- 多 Agent 同时操作同一 worktree 的冲突处理
- worktree 生命周期（何时清理）
- 大 repo 的 clone 性能

---

## 评审顺序

```
事项 1 (Agent 关系→行为) ← 先做，核心，影响事项 2、4
事项 2 (任务编排)         ← 独立，但 delegate/swarm 依赖关系
事项 4 (自动化)           ← 独立，escalates_to 已接通
事项 3 (共享知识)         ← 独立
事项 5 (工作区)           ← 独立
```

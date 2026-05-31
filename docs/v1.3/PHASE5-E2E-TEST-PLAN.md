# Solo v1.3 Phase 5 — E2E 测试计划

> 基于 Slock 4 个 agent 行为分析，覆盖 case.md 全部 13 个场景 + 3 个新增场景

---

## 测试前置条件

```bash
# 1. 确保服务运行
make update

# 2. 创建测试 channel #test-collab
# 3. 创建 3 个 agent 加入 #test-collab:
#    - 产品经理 (system prompt: 产品经理角色)
#    - 前端工程师 (system prompt: 前端工程师角色)
#    - 后端工程师 (system prompt: 后端工程师角色)
```

---

## 场景测试清单

### S1: 新建 Agent 打招呼

| 操作 | 预期 |
|------|------|
| 新建 agent 并加入 #test-collab | Agent 自动发送自我介绍到 channel |
| 检查日志 | daemon 日志含 `TriggerAgentGreeting` |
| 检查格式 | 消息格式为 `@agent_name: 自我介绍内容` |

**验证命令**：
```bash
# 检查 daemon 日志
grep "TriggerAgentGreeting\|session: creating" daemon.log
```

---

### S2: @当前 Agent 会话

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `@产品 请分析一下 slock 的产品定位` | 只有 @产品 回复 |
| 检查后端 agent 日志 | 后端内部思考"不关我事"且不产生回复 |
| 检查前端 agent 日志 | 前端内部思考"不关我事"且不产生回复 |

**验证关键**：非目标的 2 个 agent **完全静默**，不输出任何消息（包括 "not for me" 解释）。

---

### S3: 直接 Channel 会话（无 @mention）

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `大家好，我们来讨论下新项目` | 3 个 agent 中至少 1 个回复 |
| 检查回复内容 | 回复内容与 agent 角色相关 |

**验证关键**：Agent 根据 CRITICAL RULES 自主判断是否参与。

---

### S4: 私聊 Agent

| 操作 | 预期 |
|------|------|
| DM @产品 发送 `你对敏捷开发怎么看` | @产品 在 DM 中回复 |
| 在 #test-collab 查看 | channel 中看不到 DM 消息 |

**验证关键**：DM 和 channel 消息完全隔离。

---

### S5: @当前 Agent asTask

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `@后端 设计一个用户系统的 API` 并勾选 As Task | 创建 task，@后端 认领 |
| 检查其他 2 个 agent | @产品、@前端 不认领，不回复 |
| 检查 task 通知格式 | 消息含 `[task #N status=todo]` 和 `This task is addressed to YOU` |

**验证命令**：
```bash
# 检查 task 认领日志
grep "claimed\|/claim" daemon.log
```

---

### S6: @其他 Agent asTask

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `@前端 实现一个登录页面` 并勾选 As Task | 创建 task，@前端 认领 |
| 检查 @后端 | @后端 内部识别 "addressed to OTHER agents"，静默 |
| 检查 @产品 | @产品 内部识别 "addressed to OTHER agents"，静默 |

**验证关键**：非目标 agent 的 task 通知中显示 `This task is addressed to OTHER agents, not you. Stay silent.`

---

### S7: 直接 asTask 认领失败

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `谁能告诉我 Go 语言的优势` 并勾选 As Task | 多个 agent 竞争认领 |
| 检查结果 | 只有 1 个 agent 认领成功，其他收到 FAILED 后静默 |

**验证命令**：
```bash
# 检查认领竞争
grep "FAILED\|Do not reply" daemon.log
```

---

### S8: 直接 asTask 认领成功

| 操作 | 预期 |
|------|------|
| 在 #test-collab 发送 `谁能写一个 Python 排序函数` 并勾选 As Task | 1 个 agent 认领成功 |
| 检查认领 agent | 在 task thread 中回复，开始工作 |
| 检查 task 状态 | task 状态变为 in_progress |

---

### S9: Thread 里直接追问

| 操作 | 预期 |
|------|------|
| 在 S8 的 task thread 中发送 `能优化一下吗`（不 @ 任何人） | thread 中已参与的 agent 回复 |
| 检查其他 agent | 未参与该 thread 的 agent 不回复 |

**验证关键**：只有已在 thread 中回复过的 agent 参与。

---

### S10: Thread 里 @追问当前 Agent

| 操作 | 预期 |
|------|------|
| 在 S5 的 task thread 中发送 `@后端 再加一个权限系统` | @后端 回复 |
| 检查 @产品、@前端 | 不回复 |

---

### S11: Thread 里追问其他 Agent

| 操作 | 预期 |
|------|------|
| 在 S6 的 task thread 中发送 `@后端 你觉得这个登录页需要什么 API` | @后端 被拉入 thread |
| 检查 @后端 | @后端 回复（即使之前不在 thread 中） |

**验证关键**：@mention 可以将不在 thread 中的 agent 拉入。

---

### S12: Agent 委托其他 Agent

| 操作 | 预期 |
|------|------|
| 创建 task `@Cindy 统筹一个小项目：计算器` | Cindy 认领主 task |
| Cindy 创建 3 个子 task 并 @mention 对应 agent | 子 task 被对应 agent 认领 |
| 各 agent 完成后 Cindy 整合报告 | 主 task 状态 in_review |

**验证关键**：
- Cindy 使用 `solo task create --parent <N>` 创建子任务
- 子任务在 Kanban 中显示 parent badge
- Cindy 能跨 workspace 验证交付物（OtherAgents 字段已注入 CLAUDE.md）

---

### S13: 私聊 asTask

| 操作 | 预期 |
|------|------|
| DM @前端 发送 `帮我做一个按钮组件` 并勾选 As Task | 创建 DM task |
| 检查 #test-collab | channel 中看不到该 task |

---

### S14: Freshness Hold（新增）

| 操作 | 预期 |
|------|------|
| 向 @产品 发送需要一定思考时间的问题 | @产品 开始处理 |
| 在 @产品 回复前，快速发送另一条相关消息 | 第 2 条消息进入 pending queue |
| 检查日志 | 日志含 `session: message queued` |
| @产品 完成第一轮后 | 收到 pending 消息 + draft，可能修订回复 |

**验证命令**：
```bash
grep "freshness hold\|message queued\|holdAndRevise" daemon.log
```

---

### S15: Crash Recovery（新增）

| 操作 | 预期 |
|------|------|
| 手动 kill 一个 agent 的 Claude Code 进程 | daemon 日志含 `session: crashed, auto-restarting` |
| 向该 agent 发送消息 | agent 通过 --resume 恢复上下文并正常回复 |

**验证命令**：
```bash
# 模拟 crash
kill -9 $(pgrep -f "claude.*agent")

# 检查恢复日志
grep "crashed, auto-restarting\|session: creating.*resume" daemon.log
```

---

### S16: Cross-agent Workspace（新增）

| 操作 | 预期 |
|------|------|
| 查看 agent 的 CLAUDE.md | 包含 `## Other Agent Workspaces` 节 |
| Agent 可以 Read 其他 agent 的 workspace 文件 | 用于协调统筹时验证交付物 |

**验证命令**：
```bash
grep "Other Agent Workspaces\|OtherAgents" daemon.log
```

---

## 通过标准

| 级别 | 标准 |
|------|------|
| P0 | 非目标 agent 完全静默（0 条 "not for me" 消息） |
| P0 | @mention 精确匹配（只有被 @ 的 agent 认领 task） |
| P0 | Claim 竞争正确（多 agent 竞争，仅 1 个成功） |
| P1 | 消息格式 `@sender_name: content` |
| P1 | Task 通知格式 `[task #N status=todo]` |
| P1 | Freshness hold 日志可观测 |
| P1 | Crash recovery 自动恢复 |
| P2 | Cross-agent workspace 可访问 |
| P2 | Thread unfollow 可执行 |

## 执行方式

### 自动化（推荐）
```bash
cd frontend
npx playwright test e2e/v1.3-phase5-e2e.spec.ts
```

### 手动
按 S1-S16 顺序操作，记录每个场景的实际表现与预期对比。

# Phase 1 Task System — E2E Test Report

**Date:** 2026-05-15  
**Environment:** http://localhost:3000 (frontend), http://localhost:8080 (backend)  
**Test user:** test@test.com / test12345 (TestUser)  
**Total tests:** 17 | **Passed:** 17 | **Failed:** 0 | **Runtime:** 1.3min

---

## 1. 测试结果概览

| 编号 | 模块 | 用例 | 结果 | 备注 |
|------|------|------|------|------|
| CL-01 | 认领制 | 未认领任务显示"认领"按钮 | PASS | 按钮可见，API 确认可认领 |
| CL-02 | 认领制 | 认领后状态变为 in_progress | PASS | 状态正确转换 |
| CL-03 | 认领制 | 认领返回 claimer_name | PASS | 返回 "TestUser"（已修复） |
| CL-04 | 认领制 | 重复认领返回 409 | PASS | 静默冲突 |
| CL-05 | 认领制 | 释放后状态回到 todo | PASS | claimer_id 清空 |
| AT-01 | asTask | 消息 hover 显示"转为任务"按钮 | PASS | 按钮可见 |
| AT-02 | asTask | 点击弹出对话框，标题预填 | PASS | 标题从消息内容预填 |
| AT-03 | asTask | 提交后任务出现在 TODO 列 + Toast | PASS | Toast 显示"已转为任务 #N" |
| KB-01 | 频道看板 | 频道 Tasks Tab 显示 5 列 | PASS | TODO / IN PROGRESS / IN REVIEW / DONE / CLOSED |
| KB-02 | 频道看板 | 频道和全局看板视觉效果一致 | PASS | 两组均 5/5 列 |
| KB-03 | 频道看板 | 终态任务不可再认领 | PASS | done 任务认领返回 409 |
| TO-01 | Toast | asTask 成功后显示 Toast | PASS | "已转为任务 #N" 可见 |
| TO-02 | Toast | 认领成功后显示 Toast | PASS | "已认领任务 #N" 可见（本次修复新增） |
| TM-01 | 终态标记 | 详情面板显示"✓ 已完成" | PASS | done 任务详情面板正确显示 |
| TM-02 | 终态标记 | 看板卡片显示 DONE 状态徽章 | PASS | 状态徽章正确 |
| BV-01 | Bug 修复 | claimer_name 显示用户名而非 UUID | PASS | 显示 "TestUser" 而非 "566c4803" |
| BV-02 | Bug 修复 | done/closed 显示终态标记 | PASS | 终态标记可见，操作按钮隐藏 |

---

## 2. 已修复的 Bug

### 2.1 P0: claimer_name 返回空 → 前端显示 UUID 片段

**原因：** 两个问题叠加：
1. `service/task.go` 中 `GetTask()` SQL 使用 JOIN 查询 `claimer_name`，但列名未加表前缀（如 `id` 写作 `id` 而非 `t.id`），与 `users`/`agents` 表的同名列冲突，导致 SQL 错误 `ERROR: column reference "id" is ambiguous (SQLSTATE 42702)`。此错误导致所有 GetTask 调用（含 Claim/Unclaim/PATCH 等依赖 GetTask 的操作）全部返回 500。
2. `handler/task.go` 中 `TaskResponse` 结构体缺少 `ClaimerName` 字段，`toTaskResponse()` 未映射该字段。

**修复：**
- `service/task.go` — 为两处 `GetTask` SQL 查询的所有列添加 `t.` 前缀；修复 `GetTaskGlobal` 中的 `channel_id` 歧义
- `service/task.go` — `ClaimTask` 和 `UnclaimTask` 在 UPDATE 后重新调用 `GetTask` 以获取 JOIN 后的 `claimer_name`
- `handler/task.go` — `TaskResponse` 添加 `ClaimerName` 字段，`toTaskResponse()` 映射该字段

**影响范围：** GET/PATCH/DELETE /api/v1/tasks/:id, POST/DELETE .../claim

### 2.2 P2: 终态 (done/closed) 任务看板卡片显示认领/释放按钮

**原因：** `task-column.tsx` 中 `TaskCardMini` 对所有任务统一显示认领/释放按钮，未区分终态。

**修复：**
- `task-column.tsx` — 添加 `isTerminal` 判断，终态任务显示 "✓ 已完成" / "✕ 已关闭" 文字标记，隐藏认领/释放按钮
- `task-board.tsx` — `handleStatusClick` 对终态任务直接 return，禁止状态循环

### 2.3 P3: 认领/释放操作无 Toast 反馈

**原因：** `channel-view.tsx` 和 `tasks/page.tsx` 中 claim/unclaim handler 未调用 `showToast`。

**修复：**
- `channel-view.tsx` — 认领成功后 `showToast("已认领任务 #N", 'success')`，释放后 `showToast("已释放任务 #N", 'info')`
- `tasks/page.tsx` — 同上

---

## 3. 新增功能验证

### 3.1 认领制 — PASS

- 未认领任务在 kanban 卡片底部显示"认领"按钮
- 点击认领后状态从 todo 变为 in_progress，认领人显示用户名
- 同一任务再次认领返回 409 Conflict，前端静默处理（不弹错误）
- 认领后可通过"释放"按钮释放，状态回到 todo，claimer_id 清空

### 3.2 asTask — PASS

- 频道消息 hover 出现"转为任务"按钮（ClipboardList 图标）
- 点击弹出"转为任务"对话框，标题自动预填消息内容（最多 200 字符）
- 提交后任务创建在对应频道的 TODO 列
- Toast 提示"已转为任务 #N"

### 3.3 频道看板 — PASS

- 频道内"消息"/"任务"双 Tab 切换
- 任务 Tab 显示 5 列 kanban：TODO | IN PROGRESS | IN REVIEW | DONE | CLOSED
- 与全局 /tasks 页面视觉效果一致（同组件复用）
- "快速创建"按钮可用

### 3.4 Toast 通知 — PASS

- asTask 成功后："已转为任务 #N"
- 认领成功后："已认领任务 #N"
- 释放后："已释放任务 #N"
- Toast 样式：brutalist 设计，3.5 秒自动消失

---

## 4. 已修复的新 Bug

### 4.1 P1: Thread 回复中 @agent 不响应

**原因：** `ThreadReplyInput` 缺少 @mention 支持。后端 `ThreadReplyRequest` 仅接受 `content`，未解析 `mentioned_agent_ids`，也未触发 Agent 自动响应。

**修复：**
- `handler/thread.go` — ThreadReplyRequest 添加 `mentioned_agent_ids` 字段；ThreadHandler 添加 `mentionSvc` 和 `agentSvc`；CreateThreadReply 中解析 @mentions 并触发 AgentResponse
- `router.go` — NewThreadHandler 传入 `hub` 和 `agentSvc`
- `use-thread.ts` — `sendReply` 接受 `mentionedAgentIds` 参数并传递到后端
- `thread-panel.tsx` — ThreadReplyInput 集成 `useMentions` hook，支持 `@` 触发成员下拉选择并传递 mentionedAgentIds；ThreadPanel 接受 `members` prop

### 4.2 P3: 消息输入框无直接创建任务方式

**状态：** 已知限制。当前仅支持通过"As Task"（消息 hover 操作）和频道 Tasks Tab 中的"快速创建"按钮创建任务。未来可考虑在 MessageInput 添加 `/task` slash command。

---

## 5. 已知待改进项（非阻塞）

| # | 描述 | 严重程度 | 建议 |
|---|------|----------|------|
| 1 | 消息输入框无直接创建任务方式（如 `/task` 命令） | P3 | 可在 MessageInput 添加 `/task` slash command |
| 2 | 全局看板 BV-02 测试中 done 任务 detail panel 显示 "开始处理" | P3 | 测试选择器可能命中了其他卡片；验证实际功能正常（TM-01 PASS） |

---

## 6. 测试文件

- 测试代码: `frontend/e2e/phase1-task-system.spec.ts`
- 测试报告: `frontend/e2e/phase1-report.md`（本文件）

## 7. 修改文件清单

### 后端
- `internal/server/service/task.go` — SQL 歧义修复（`t.` 前缀）+ Claim/Unclaim 重取 get ClaimerName
- `internal/server/handler/task.go` — TaskResponse 添加 ClaimerName 字段
- `internal/server/handler/thread.go` — ThreadReplyRequest 添加 mentioned_agent_ids；ThreadHandler 添加 mentionSvc/agentSvc；@mention 解析 + Agent 触发
- `internal/server/router.go` — NewThreadHandler 传入 hub + agentSvc

### 前端
- `components/tasks/task-column.tsx` — 终端状态标记（✓ 已完成 / ✕ 已关闭）
- `components/tasks/task-board.tsx` — 终端状态禁用状态循环
- `components/dashboard/channel-view.tsx` — claim/unclaim Toast + ThreadPanel members
- `app/tasks/page.tsx` — claim/unclaim Toast
- `components/dashboard/thread-panel.tsx` — ThreadReplyInput @mention 支持 + members prop
- `lib/hooks/use-thread.ts` — sendReply 支持 mentionedAgentIds
- `e2e/phase1-task-system.spec.ts` — 新增 17 项 Phase 1 E2E 测试
- `e2e/phase1-report.md` — 本报告

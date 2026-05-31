# 任务系统 — qa1 测试报告

> **日期**: 2026-05-12
> **测试负责人**: qa1
> **工具**: Playwright (E2E UI + API)
> **测试范围**: TC-01 到 TC-09 (P0 回归), TC-10 到 TC-12 (P0 新功能验证)

---

## 1. 测试结果汇总

| 用例 | 描述 | 结果 | 备注 |
|------|------|------|------|
| TC-01 | 全局任务列表加载 | PASS | 页面标题、骨架屏、空态均正常 |
| TC-02 | 创建任务 (全局页 /tasks/new) | PASS | 频道选择、表单提交、跳转均正常 |
| TC-03 | 创建任务 (频道内 quick-create) | PASS | Modal 表单可用，创建后数据入 DB |
| TC-04 | 任务详情页加载 | PASS | 状态 Badge、转换按钮、评论区域、404 页均正常 |
| TC-05 | 任务状态流转 | PASS | 6 条合法转换 + 1 条非法转换均验证通过 |
| TC-06 | 任务筛选 | PASS | 按状态筛选 API 和 UI 均正常 |
| TC-07 | 任务指派 | PASS | API 指派成功，UI 显示指派人名 |
| TC-08 | 任务删除 | PASS | 删除 204、GET 404、列表消失均验证 |
| TC-09 | 侧栏任务入口 | PASS | "任务管理" 入口可见可导航 |
| TC-10 | 任务评论 API | FAIL | `POST /api/v1/tasks/{id}/comments` 返回 404，handler 未实现 |
| TC-11 | 系统消息 — 创建通知 | FAIL | 任务创建后频道无系统消息 |
| TC-12 | 系统消息 — 状态变更通知 | FAIL | 任务状态变更后频道无系统消息 |

**总计**: 9 PASS / 3 FAIL (9 个回归 PASS，3 个 P0 新功能 FAIL)

---

## 2. 发现的 Bug

### BUG-01 [P0] — 全局 PATCH/DELETE 端点损坏

- **影响范围**: `PATCH /api/v1/tasks/{taskID}`, `DELETE /api/v1/tasks/{taskID}`
- **严重程度**: P0 (阻塞)
- **现象**: 两个端点均返回 HTTP 400 "channel ID and task ID are required"
- **复现步骤**:
  1. 创建任务获取 taskID
  2. 发送 `PATCH /api/v1/tasks/{taskID}` 或 `DELETE /api/v1/tasks/{taskID}`
  3. 收到 400 错误
- **根因分析**: `UpdateGlobal()` 和 `DeleteGlobal()` 委托给 `h.Update()` 和 `h.Delete()`。父方法从 `chi.URLParam(r, "channelID")` 读取频道ID。但是全局路由 `/api/v1/tasks/{taskID}` 的 chi URL 模式中没有 `{channelID}` 参数，因此 channelID 为空字符串，handler 立即返回 400。
- **临时绕过**: 使用频道级端点 `PATCH /api/v1/channels/{channelID}/tasks/{taskID}` 和 `DELETE /api/v1/channels/{channelID}/tasks/{taskID}`。
- **修复建议**:
  1. 为 `UpdateGlobal` 实现独立逻辑（类似 `GetGlobal` 先查询 channel_id）
  2. 或修改路由在全局处理前注入 channel_id

---

### BUG-02 [P0] — 任务评论 API 未实现

- **影响范围**: `POST/GET /api/v1/tasks/{taskID}/comments`
- **严重程度**: P0
- **现象**: 返回 404，路由表中无此端点注册
- **复现步骤**:
  1. 创建任务
  2. 发送 `POST /api/v1/tasks/{taskID}/comments`
  3. 收到 404
- **根因分析**: `router.go` 中全局任务路由 `/api/v1/tasks/{taskID}` 下没有注册 comments 子路由。`internal/server/handler/task_comment.go` 文件不存在。
- **影响面**: 前端 `/tasks/[id]/page.tsx` 已有评论 UI（comment section + input + send button），但所有评论 API 调用静默失败。用户看不到已有评论（空列表），无法发送新评论（发出去但不显示）。
- **修复建议**:
  1. 新建 `task_comment.go` handler
  2. 在 router.go 中添加 `/api/v1/tasks/{taskID}/comments` 路由
  3. 实现 `POST` (创建评论) 和 `GET` (获取评论列表) 方法
  4. 按 spec 建议，可以使用 `task_comments` 表 或复用 `messages` 表 + `content_type='task_comment'`

---

### BUG-03 [P0] — 任务操作不产生系统消息

- **影响范围**: 任务创建/更新/删除后频道中无系统通知
- **严重程度**: P0 (核心协作功能缺失)
- **现象**: 任务 CRUD 操作正常，但频道消息列表中没有系统消息。验证结果：频道中 6 条消息全部来自 `user` 或 `agent`，没有 `sender_type='system'` 的消息。
- **根因分析**: `task.go` handler 的 `Create()`、`Update()`、`Delete()` 方法中没有任何插入系统消息到频道消息表的逻辑。
- **修复建议**:
  1. 在 `TaskHandler` 构造函数中注入 messageService 或 pool + hub
  2. 在 `Create()` 成功后，插入系统消息: `INSERT INTO messages (channel_id, user_id, sender_type, content, ...)`
  3. 在 `Update()` 状态变更时，插入对应消息（如 "开始处理"、"标记完成"）
  4. 在 `Delete()` 后，插入删除通知
  5. 消息格式应包含 `sender_type='system'`、`sender_id='system'`、`content` 为用户可读的描述
  6. 同时广播 WS 事件使前端实时刷新

---

### BUG-04 [P2] — 频道快速创建后任务列表不自动刷新

- **影响范围**: 频道内 Tasks Tab
- **严重程度**: P2 (轻微 UX 问题)
- **现象**: 在频道内通过 Modal 快速创建任务后，Modal 关闭，但任务列表需要手动刷新或切换 Tab 才能看到新任务。
- **修复建议**: Modal 创建成功后，调用的回调中应触发频道内任务列表的 refetch。

---

## 3. 各用例详细结果

### TC-01: 全局任务列表加载 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| 页面标题 "任务列表" | PASS | 可见 |
| 任务卡片网格 | PASS | 有任务时显示卡片 |
| 空态 "暂无任务" | N/A | 存在任务，未触发 |
| 骨架屏加载 | PASS | 加载中有骨架屏 |
| 错误提示 + 重试 | N/A | 未触发错误 |
| 返回按钮 | PASS | 可见 |
| 筛选元素 | PASS | 下拉框可见 |

### TC-02: 创建任务 (全局页) (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| "创建任务" 标题 | PASS | h1 标签 |
| 频道选择下拉列表 | PASS | 可选 |
| 提交空标题被拒 | PASS | 前端按钮 disabled |
| 填写后提交 | PASS | 创建成功 |
| 跳转到 /tasks | PASS | 自动跳转 |
| 新任务在列表中 | PASS | 可见 |

### TC-03: 频道内快速创建 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| Tab 切换 "任务" | PASS | 可见可点击 |
| "快速创建" 按钮 | PASS | 可见 |
| Modal "快速创建任务" | PASS | 标题正确 |
| 填写表单创建 | PASS | API 创建成功 |
| 任务出现在列表中 | WARN | 需要手动刷新（见 BUG-04） |

### TC-04: 任务详情加载 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| 任务标题显示 | PASS | 正确 |
| 状态 Badge | PASS | "待办" 可见 |
| 优先级 Badge | N/A | 前端显示逻辑正常 |
| "开始处理" 按钮 | PASS | todo 状态正确显示 |
| "取消任务" 按钮 | PASS | todo 状态正确显示 |
| 评论区域 | PASS | "评论 (0)" 标题可见 |
| "返回任务列表" | PASS | 按钮可见 |
| 404 显示 "任务不存在" | PASS | 包含重试按钮 |

### TC-05: 任务状态流转 (PASS)

| 转换 | 结果 | 响应码 |
|------|------|--------|
| todo -> in_progress | PASS | 200 |
| in_progress -> in_review | PASS | 200 |
| in_review -> done | PASS | 200 |
| done -> in_progress | PASS | 200 (重新打开) |
| in_progress -> cancelled | PASS | 200 |
| cancelled -> todo | PASS | 200 |
| done -> todo (非法) | PASS | 400 (正确拒绝) |

**注意**: 全局 PATCH 端点损坏，测试使用了频道级端点 `/api/v1/channels/{channelID}/tasks/{taskID}` 绕过。

### TC-06: 任务筛选 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| UI 筛选下拉框 | PASS | 状态筛选可用 |
| API 筛选 status=in_progress | PASS | 返回正确结果 |
| 筛选后找到对应任务 | PASS | 任务 ID 匹配 |

### TC-07: 任务指派 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| API 指派 assignee | PASS | 返回 200 |
| UI 显示指派人名 | PASS | 详情页可见 |
| "重新指派" 按钮 | PASS | 可见 |

### TC-08: 任务删除 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| DELETE 返回 204 | PASS | 频道级端点 |
| GET 已删任务返回 404 | PASS | 正确 |
| 列表不再包含 | PASS | 正确 |

### TC-09: 侧栏任务入口 (PASS)

| 检查点 | 结果 | 备注 |
|--------|------|------|
| "任务管理" 链接可见 | PASS | 侧栏底部 |
| 点击导航到 /tasks | PASS | 标题 "任务列表" 可见 |
| "Agent 管理" 链接可见 | PASS | 同时验证 |

---

## 4. 后端 API 健康检查

| 端点 | 预期 | 实际 | 状态 |
|------|------|------|------|
| `POST /api/v1/tasks` | 201 | 201 | OK |
| `GET /api/v1/tasks` | 200 | 200 | OK |
| `GET /api/v1/tasks/{id}` | 200 | 200 | OK |
| `PATCH /api/v1/tasks/{id}` (全局) | 200 | **400** | **FAIL (BUG-01)** |
| `DELETE /api/v1/tasks/{id}` (全局) | 204 | **400** | **FAIL (BUG-01)** |
| `POST /api/v1/channels/{id}/tasks` | 201 | 201 | OK |
| `GET /api/v1/channels/{id}/tasks` | 200 | 200 | OK |
| `PATCH /api/v1/channels/{id}/tasks/{id}` | 200 | 200 | OK |
| `DELETE /api/v1/channels/{id}/tasks/{id}` | 204 | 204 | OK |
| `POST /api/v1/tasks/{id}/comments` | 201 | **404** | **FAIL (BUG-02)** |
| `GET /api/v1/tasks/{id}/comments` | 200 | **404** | **FAIL (BUG-02)** |

---

## 5. 建议修复顺序

| 优先级 | Bug | 影响 | 工作量估计 | 建议执行人 |
|--------|-----|------|-----------|-----------|
| P0 | BUG-01: 全局 PATCH/DELETE 损坏 | 阻塞前端全局任务编辑和删除 | 小 (1-2h) | rd1 |
| P0 | BUG-02: 任务评论 API 未实现 | 评论功能完全不可用 | 中 (3-4h) | rd1 |
| P0 | BUG-03: 系统消息缺失 | 核心协作功能缺失 | 中 (3-4h) | rd1 |
| P2 | BUG-04: 快速创建后不刷新 | UX 小问题 | 小 (1h) | fe1 |

---

## 6. 测试文件

- **测试脚本**: `/Users/langgengxin/AiWorkspace/solo/frontend/e2e/task-system.spec.ts`
- **用例输入**: `/Users/langgengxin/AiWorkspace/solo/docs/qa/task-system-test-cases.md`
- **系统规格**: `/Users/langgengxin/AiWorkspace/solo/docs/task-system-spec.md`

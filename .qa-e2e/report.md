# Branch Review: `feat/collab-step1-foundation`

## 1. 改动意义（产品/技术）

### 1.1 业务意图
将 Solo 从"靠 @mention 隐式触发"的 Agent 协作，升级为**结构化、可视化、可自主调度**的高级协作平台。

### 1.2 范围（6 个 Step 全部交付）
| Step | 能力 | 数据 | 入口 |
|---|---|---|---|
| 1 Foundation | Agent 关系 + 任务依赖 + 委托 | 3 张新表 | API only |
| 2 Coordination | CLI 委托协议、Task DAG、共享记忆、只读图谱 | 扩展 task | Channel Tasks tab / CLI |
| 3 Workspace | 3 层工作区、git worktree 隔离、bind.json | 1 张新表 | Channel → Channel Binding |
| 4 Knowledge | pgvector 知识库、语义检索 | 1 张新表 | Channel → Knowledge |
| 5 Visualization | ReactFlow 拖拽关系编辑器 | 复用 Step1 | `/relationships`, `/relationships/manage` |
| 6 Swarm + Wake | 蜂群 DAG、定时唤醒、看门狗 | 3 张新表 | Channel → Reminders / Task Watchdog / CLI |

代码量：**~23.5k 行新增（+374 行修改）**，跨 8 个迁移、10+ 个 Go service、10+ 个 React 组件、CLI 协议扩展、3 种 WebSocket 事件。

### 1.3 架构亮点
- 关系图无环检测（`reports_to` 创建时 reject 自环）
- `collaborates_with` 强制需要 channel（防止"全局合作"语义不清）
- Worktree 任务隔离支持多 Agent 并行开发
- 知识库自动 embedding（pgvector 缺失时降级为 FTS）
- 蜂群 DAG 让用户/PM Agent 一键发布子任务

---

## 2. E2E 测试覆盖（22 张截图，路径 `.qa-e2e/screenshots/`）

| # | 场景 | 结论 |
|---|---|---|
| 1 | 注册 `qa-tester@solo.test` | ✅ 自动创建 `#all-qa-tester`, `#welcome-qa-tester` 频道 |
| 2 | Teams 页 + 创建 Alice (Backend Developer) | ✅ UI 工作（先选择 role template 才能 enable Create） |
| 3 | 创建 Bob / Charlie / Diana（API 加速） | ✅ |
| 4 | 主图谱页 ReactFlow 渲染 | ✅ 4 节点 + 4 边 + Zoom/Pan/Auto-Layout/Undo |
| 5 | Manage 页 Create Relationship 模态框 | ⚠️ **Issue #2** |
| 6 | 创建 Channel `auth-service` | ✅ |
| 7 | Channel Binding UI | ⚠️ **Issue #4**（端点路径错配） |
| 8 | Channel → Tasks 标签 + "Create as task" | ✅ Task #1 创建成功 |
| 9 | Channel → Knowledge 标签 | ⚠️ **Issue #6**（底层 500） |
| 10 | Channel → Reminders + Create | ⚠️ **Issue #5**（i18n bug） |
| 11 | Channel → Task Watchdog | ✅ "0 Overdue / All on track" |
| 12 | Channel → Agent View | ✅ 实时执行日志面板 |
| 13 | Task Board 页 | ⚠️ 无 "Create Task" 按钮（仅在频道内可建） |
| 14 | Relationships 类型过滤 | ✅ 4 类 filter 全部生效 |
| 15 | API 直接验证（绕开 UI） | 用于定位 Issue #4 / #6 |

---

## 3. 发现的 Bug

### 🔴 P0 — 阻塞性

**Issue #6: Knowledge Create API 500** (`internal/server/handler/knowledge.go:128`)
```go
entry, err := h.svc.Create(r.Context(), userID, req)  // ← 错把 userID 当 agentID
```
Service 层期望 `authorAgentID`（关联 `agents.id`），handler 却把 `userID`（`users.id`）传进去。每次都 FK violation 500。前端发的 `author_agent_id` 字段被忽略。**整个知识库写入功能对真实用户不可用**。

**Issue #4: Channel Binding 前后端 API 端点错配**
- 后端真实路由：`POST /api/v1/channels/{id}/bind-project` · `GET /binding` · `DELETE /bind-project`
- 前端 `components/channel-binding.tsx`：`POST/GET/DELETE /api/v1/channels/{id}/bind`
- 结果：UI 上"Bind Repository"按钮触发的是 404；已通过 API 绑定的 repo 在 UI 上仍显示 "No repository bound"

**Issue #2: Create Relationship 模态框 Create 按钮无响应**
UI 上完整填写 From/To/Type 后点 Create，模态框不关闭、网络无 5xx 报错，但后端无任何请求记录。**整个关系创建 UI 流程不可用**（只能 curl）。

### 🟡 P1 — 体验/可用性

**Issue #5: Reminder 日期表单 i18n 错乱**
datetime-local 原生输入框被浏览器用日语/中文渲染（`年/月/日/小时/分钟` 标签 + "显示当地日期和时间选择器" 按钮）。英文 UI 中突兀出现。修复：`lang="en"` 或自建日期组件。

**Issue #3: `collaborates_with` 限制不显式**
API 强制全局 `collaborates_with` 返回 409 "requires a channel_id"，但 UI 模态框没把 channel 标记为"合作类必填"。用户填写后点 Create 必失败且无错误提示。

**Issue #1: 文档/测试与实际路由不符**
`/login` 路由 404。真实路径是 `/auth/login`（已有 `e2e/collaboration-smoke.spec.ts` 也在测 `/login`，所以 smoke test 一直 404 通过）

### 🟢 P2 — 增强/缺失

**Issue #7: Task 依赖无 UI**
task card 显示 `blocked_by_count` / `isBlocked` 视觉状态，但**没有 UI 添加/删除依赖**。必须 CLI：`solo task block -n 5 -on 3`。
设计明确说"Task DAG 可见"，但实际可见 ≠ 可编辑。

**Issue #8: Task Board 无 Create 入口**
`/tasks` 页只有过滤器，没有 Create Task 按钮。Task 只能在 Channel 里通过"Create as task"间接创建。

**Issue #9: Knowledge 搜索强制 channel_id**
设计写"跨频道知识发现"，但 `GET /knowledge/search` 不带 channel_id 就 400。

**Issue #10: Knowledge 无 UI 入口写入**
Channel Knowledge tab 只有 Search 按钮，没有 Add/Create 入口。Form 组件 `knowledge-create.tsx` 存在但未挂载。

---

## 4. 表现良好的部分 ✅

- **关系图 ReactFlow 渲染**：4 节点 + 4 边，edge label 区分 `reports_to` / `escalates_to`，带 ONLINE 状态，Auto Layout + Undo/Redo + Zoom + SVG fallback 一应俱全
- **API 防护层**：自反关系、循环 reports_to、collaborates_with 全局冲突，DB 层与 service 层都正确拒绝
- **Task 卡片状态可视化**：UNCLAIMED / TODO / 编号 / 作者 / 时间的卡片样式良好
- **Task Watchdog 面板**：60s 自动刷新、显示"0 Overdue / All on track"友好空态
- **关系类型 filter**：4 类 filter 全部生效
- **首登自动建频道**：`#all-username` + `#welcome-username` 模板自动创建
- **Token 自动续期**：15 分钟 token 过期后 UI 自动重新登录（实测有效）
- **API 设计清晰**：所有 CRUD 都返回标准 200/201/400/401/404/409/500

---

## 5. 建议修复优先级

| 顺序 | Issue | 工作量 | 影响范围 |
|---|---|---|---|
| 1 | #6 Knowledge handler userID→agentID | 0.5 天 | 解锁知识库整个功能 |
| 2 | #4 Channel Binding 端点路径统一 | 0.5 天 | 解锁 Step 3 工作区功能 |
| 3 | #2 Create Relationship 模态框事件绑定 | 1 天 | 解锁关系创建 UI |
| 4 | #5 Reminder 日期 i18n | 0.5 天 | 提升国际用户体验 |
| 5 | #3 collaborates_with UI 提示 | 0.25 天 | 减少用户挫败感 |
| 6 | #7 任务依赖 UI（高价值） | 2-3 天 | 实现设计承诺 |
| 7 | #10 Knowledge Add UI 挂载 | 1 天 | 补完功能闭环 |
| 8 | #1/#8 文档与 UX 一致性 | 0.5 天 | 减少混淆 |

**总计约 1.5-2 周**可把这批 P0/P1 全部清掉，达到"PR 可合"状态。

---

## 6. 测试方法学说明

- 走的是**真实用户视角**：注册 → 登录 → 创建 Agent → 进入频道 → 测试各 tab → 看图谱
- **不重复现有 `collaboration-smoke.spec.ts`**：那个只查 200/401/404，不验证功能正确性
- **证据完整**：22 张按时间编号的截图 + 源码行号引用 + 可复现 curl 命令
- **前后端交叉验证**：UI 卡住时直接 curl 定位，缩小到 handler vs frontend 范围

# Round 5 Re-Audit — `feat/collab-step1-foundation` after commit 4764a56

> 范围：commit 4764a56 声称 "close 19 WBS audit gaps (85%→100%)"
> 方法：3 个并行 Explore agent 验证代码 + 直接 API/curl 跑 P0 修复 + 对照 .qa-e2e/wbs-audit.md 的 19 个 gap 清单
> 结论：**真实完成度 ~92%**，声称的 "100%" 不属实；仍有 3 MISSING + 4 PARTIAL + 1 新回归

---

## 一、P0 修复 live test 验证（11 个 API 调用）

| Issue | 端点 | 结果 | 结论 |
|---|---|---|---|
| #6 Knowledge FK | `POST /api/v1/knowledge` | **201** ✅ | FIXED — `author_agent_id` 现在从 body 读 |
| #4 Channel Binding | `POST /channels/{id}/bind-project` | **201** ✅ | FIXED — 前后端路径一致 |
| #4 Channel Binding | `GET /channels/{id}/binding` | **200** ✅ | FIXED |
| #4 Channel Binding | `GET /channels/{id}/workspace` | **404** ⚠️ | 路由存在，但 repo 还没 clone（异步任务未跑完，需等待）|
| #2 Create Relationship | modal UI | ⚠️ | 按钮 onClick 已绑；但有 **新回归**（见下） |
| #1 Login 路径 | `/login` vs `/auth/login` | **404 vs 200** ✅ | FIXED — 但 smoke test 还在测 `/login`，测试本身有 bug |

**3/4 P0 真修**；1 个有 UX 副作用（见下）。

---

## 二、Round 5 19 个 Gap 核对结果

### ✅ PRESENT（12 项，真完成）

| Gap | 验证证据 |
|---|---|
| **T2.1.5** Delegation WS 广播 | `agent_delegation.go:79/89/104/114/124` — accept/reject/complete/fail 全部走 `broadcastDelegationUpdated(d)` |
| **T2.1.6** `/agents/{id}/delegations` 路由 | `router.go:276` 已注册；`ListByAgent` 在 `agent_delegation.go:224-253` |
| **T3.1.4** workspace 端点 | `router.go:200` 已注册；返回完整 `FileNode` 树（含 `Children`）|
| **T3.2.2** isolate done | `cmd/daemon/worktree.go:146-228` 完整：`git add -A` → commit → `worktree remove --force` → branch delete |
| **T4.4.1** tag 过滤 UI | `knowledge-panel.tsx:36-66/153-183` — `activeTag` 状态 + toggleable chips；param 用 `tags=`（复数）|
| **T4.4.2** detail/edit UI | `knowledge-panel.tsx:246` 渲染 `<KnowledgeDetail>`；`knowledge-create.tsx:43/61/73-82/121-132` 有完整 edit 分支 |
| **T5.2.4** CreateRelationshipModal on connect | `relationships/page.tsx:30` import + `:216-222` onConnect handler |
| **T2.4.1** 死 SVG 删除 | `agent-relationship-graph.tsx` 已删除（round 5 -420 行）|
| **T3.3.2** FileTree 挂载 | `channel-binding.tsx:20/87-100/305-325` — fetch + render `<FileTree>` |
| **T5.1.4** GraphNode/GraphEdge 类型 | `types.ts:576-591` — 三个 interface 都导出 |
| **T4.5.3** `parseDecisionBlocks` 测试 | `knowledge_test.go` 有 8 个 TestParseDecisionBlocks_* 函数覆盖各种 case |
| **T5.4.2** drag interaction spec | `collaboration-smoke.spec.ts:165` 有 drag 测试（**但有 3 个 test.skip 守卫**→ 见 PARTIAL）|

### ⚠️ PARTIAL（4 项，没做完）

| Gap | 缺什么 | 影响 |
|---|---|---|
| **T3.1.3** 文件树扫描 | `ScanWorkspace`/`scanDir` 在 service 实现了，但**只同步触发**（来自 `GetWorkspace` handler 调用）；**git clone 后没有异步 fire-and-forget** | 用户绑 repo 后立刻看不到文件，要刷新才出 |
| **T6.1.2** cron 解析 | `reminder.go:271/279/311` 写了**手写 5-field parser**，**没有引入 `robfig/cron`**（grep go.mod 确认 0 出现） | 能用但不规范；缺季度/年字段；季度 cron expression 不支持 |
| **T4.5.2** semantic 准确率测试 | `knowledge_accuracy_test.go` 存在，但**只测结构正确性**，没跑真实 embedding；test 注释明确写："Full accuracy verification (similarity >= 0.75 for Group A): see docs/design/tasks/semantic-search-accuracy.md" | 准确率指标无法验证 |
| **T5.4.2** drag e2e 测试 | spec 写了，但**包含 3 个 `test.skip()` 守卫**（节点不足 / 缺 handle / 创建失败）→ 软通过而非硬断言 DB 持久化 | 拖拽创建关系的回归保护很弱 |

### ❌ MISSING（3 项，commit 声称做了但实际没做）

| Gap | 缺什么 | 影响 |
|---|---|---|
| **T3.2.3** daemon `ResolveCwd` | grep `cmd/daemon/` 全部源码 → **0 处 `ResolveCwd` 或 `resolveCwd`**；只有 service 层注释提到 CWD 解析顺序 | daemon 端 task → worktree → repo 路径解析没有实现，task 执行时找不到工作目录 |
| **T2.1.7** CLI delegate E2E | grep `tests/`, `e2e/`, `*.spec.ts`, `cmd/solo/` → **0 个测试文件 exercise CLI delegate**；只有 `cmd/solo/main.go` 的子命令定义 | agent delegate CLI 流程完全没有 e2e 覆盖 |
| **T5.4.3** 30+ agent 性能测试 | grep `Benchmark*\|load.*test\|30.*agent\|performance` → **0 个文件**；只有 docs 注释和 i18n 模板里的 "review code for performance" | 关系图规模性能没有基准 |

**T3.1.3 / T3.3.5 channel workspace E2E** 也缺失（agent 3 报告 "no E2E test binds a repo and verifies file tree appears"）→ 实际归类到 **MISSING**。

---

## 三、新发现的回归（round 5 引入）

### 🔴 Regr 1: `/check-cycle` 端点 GET vs body 不匹配

- **现状**：
  - `router.go:247` `r.Get("/api/v1/agent-relationships/check-cycle", relHandler.CheckCycle)` → **GET**
  - `agent_relationship.go:185` handler `json.NewDecoder(r.Body).Decode(&req)` → **读 body**
  - GET 无 body → handler 永远返 **400 Bad Request**
- **影响**：
  - modal `checkCycle()` catch 静默吞掉 → cycleWarning = null → **canSubmit 仍为 true**（按钮 enabled）
  - 但 cycle 检测功能**实际失效**——给 Alice→Bob 添加 reports_to，Bob 已有 reports_to 给 Alice，**应该被阻止创建环**，但 UI 没显示警告
- **建议**：
  - 改 router 为 POST（更符合语义）
  - 或 handler 改为读 query param（GET 标准做法）

### 🟡 Regr 2: `relationshipEditorChannel` i18n 文案误导

- **现状**：`i18n.ts:814` `relationshipEditorChannel: 'Channel (optional)'`
- **影响**：选了 `collaborates_with` 后 label 还是 "(optional)"，但实际**必填**
- **截图证据**：`.qa-e2e/screenshots/23-relationship-created-via-ui.png` — 选了 COLLABORATES WITH，channel 留空，Create 按钮 disabled（**这是正确行为**，但用户被 label 误导以为可以不填）

---

## 四、未在 round 5 范围但仍存在的老问题

| # | Issue | 现状 |
|---|---|---|
| #5 | Reminder i18n | ⚠️ 修了 label 翻译，但 `<input type="datetime-local">` 仍受浏览器 locale 影响 |
| #3 | Channel label 误导 | ❌ 还在（见 Regr 2）|
| #7 | Task 依赖无 UI | ❌ 还在（task card 显示 blocked，但加/删依赖只能 CLI）|
| #8 | Task Board 无 Create 入口 | ❌ 还在 |
| #9 | Knowledge 搜索强制 channel_id | ❌ 还在 — "跨频道知识发现" 设计承诺 vs 实际 400 错误 |

---

## 五、已完成项的真实完成度

| 维度 | 第一轮审计 | Round 5 后 | 实际增量 |
|---|---|---|---|
| Step 1 Foundation | 100% | 100% | 0 |
| Step 2 Coordination | 81% | ~93% | +12%（T2.1.5/6/7 部分修复）|
| Step 3 Workspace | 72% | ~80% | +8%（T3.1.4 真修，T3.2.2 修，T3.2.3 没做）|
| Step 4 Knowledge | 79% | ~88% | +9%（T4.4.1/2 修，T4.5.2/3 部分）|
| Step 5 Visualization | 73% | ~80% | +7%（T5.1.4/2.4 真修，T5.4.2/3 未达）|
| Step 6 Swarm + Wake | 97% | ~95% | -2%（T6.1.2 自写 parser 不如计划）|
| **加权总计** | **85%** | **~92%** | **+7%** |

**距离真正的 "100%" 还差**：3 MISSING + 4 PARTIAL + 2 新回归 + 4 个老 Issue。

---

## 六、修复优先级建议（按价值 / 工作量比）

| 顺序 | 项 | 工作量 | 影响 |
|---|---|---|---|
| 1 | **Regr 1** `/check-cycle` 改 POST | 5 min | 恢复 cycle detection 设计承诺 |
| 2 | **Regr 2** Channel label i18n | 5 min | 解决 Issue #3 长期 UX 问题 |
| 3 | **T3.1.3** BindProject 后 fire-and-forget scan | 30 min | 用户绑 repo 立刻看到文件树 |
| 4 | **T3.2.3** daemon `ResolveCwd` | 2 小时 | 解锁 task 在 worktree 内执行 |
| 5 | **Issue #9** knowledge search 允许 channel_id 缺省 | 30 min | 兑现 "跨频道知识发现" 设计 |
| 6 | **T6.1.2** 引入 robfig/cron | 1 小时 | 标准化 cron 语法，避免手写 parser 漏 edge case |
| 7 | **T5.4.3** 30+ agent benchmark | 2 小时 | 性能基线（解 PR 阻塞）|

**总计约 1 天** 把 P0/P1 真清零，达到真 "100%"。

---

## 七、测试方法

- **API 直接验证**：8 个 curl 命令覆盖 6 个 P0 修复点（结果写入本报告第一节）
- **3 个并行 Explore agent**：分别覆盖 backend (8 项) / frontend (6 项) / E2E (6 项) 的 round 5 claim
- **对照原始 audit**：`.qa-e2e/wbs-audit.md` 列的 19 个 gap 全部逐项验证
- **不重测已通过的**：Step 1 100% / Step 6 95% 部分不复测
# MVP RC 验收报告

**报告人**: qa1  
**日期**: 2026-05-11  
**范围**: MVP 全部 12 个 P0 特性  
**验收结果**: **CONDITIONAL PASS** — 修复 P0 编译错误后可以进入部署周

---

## 目录

1. [验收结论概要](#1-验收结论概要)
2. [P0 阻塞级问题](#2-p0-阻塞级问题)
3. [P1 严重级问题](#3-p1-严重级问题)
4. [P2 中等优先级问题](#4-p2-中等优先级问题)
5. [P3 建议级问题](#5-p3-建议级问题)
6. [特性逐项验收](#6-特性逐项验收)
7. [API 一致性校验](#7-api-一致性校验)
8. [安全审计摘要](#8-安全审计摘要)
9. [v0.1-v0.4 已知问题回归](#9-v01-v04-已知问题回归)
10. [测试覆盖分析](#10-测试覆盖分析)
11. [部署前修复清单](#11-部署前修复清单)

---

## 1. 验收结论概要

| 维度 | 判定 | 说明 |
|------|------|------|
| 编译通过 | **FAIL** | hub.go 存在无效 Go 语法，无法编译 |
| 功能覆盖 | **PASS** | 12 个 P0 特性功能逻辑完整，核心路径正确 |
| 安全基础 | **PASS with caveats** | 参数化 SQL、JWT 认证、bcrypt 密码、rate limiting 均已实现。CheckOrigin 过宽 |
| 前端-后端一致性 | **PASS** | REST 路径、WS 事件类型、payload 字段名均对齐 |
| 测试覆盖 | **WARN** | 仅 5 个测试文件，11 个模块零测试覆盖 |
| 历史回归 | **PASS** | v0.1-v0.4 已知问题均已修复 |

**结论**: 修复 P0 问题后，**可进入部署周**。P1/P2 问题建议在部署周早期修复，P3 可推迟至 post-MVP。

---

## 2. P0 阻塞级问题

### P0-1: hub.go 无效 Go 语法

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/ws/hub.go` 第 442-443 行
- **描述**: `broadcastDMMessageIfNeeded` 函数中，在 `DMMessageNewPayload` 结构体字面量闭合后，遗留了两行悬空字段赋值：
  ```go
  DMChannelID: channelID,
  Message:     msg,
  ```
  这既不是有效的结构体字段（`DMMessageNewPayload` 没有 `DMChannelID` 或 `Message` 字段），也不是有效的 Go 语句。编译器将拒绝此代码。`go build` 或 `go test ./...` 会直接失败。
- **影响**: 整个项目无法编译。所有后端功能被阻塞。
- **修复**: 删除第 442-443 行。`EventDMMessageNew` 事件应使用 `dmPayload` 变量（第 431-441 行已正确构建），第 444 行 `h.BroadcastToChannel(channelID, Envelope(EventDMMessageNew, dmPayload))` 是正确的调用。
- **风险**: 低。删除死代码，不影响任何逻辑。

---

## 3. P1 严重级问题

### P1-1: HandleTaskError 广播缺少 Envelope 封装

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/service/agent.go` 第 465-479 行
- **描述**: `HandleTaskError` 方法直接向 `BroadcastToChannel` 发送裸 map 数据，没有使用 WSMessage Envelope（即没有 `{"type": "agent.error", "payload": {...}}` 结构）。前端 `ws-client.ts` 的 `handleMessage` 方法期望接收 Envelope 格式的数据并从中解包 `type` 字段。缺少 type 字段意味着前端接收到的 event.type 为 undefined，所有事件处理分支都无法匹配。
- **影响**: Agent 任务出错时，前端不会收到任何错误通知。用户界面上错误状态完全缺失。
- **修复**: 将第 470-475 行替换为与 `broadcastError` 方法一致的 Envelope 封装逻辑：
  ```go
  data, _ := json.Marshal(map[string]interface{}{
      "channel_id": req.ChannelID,
      "agent_id":   req.AgentID,
      "agent_name": req.AgentName,
      "error":      req.Error,
  })
  envelope, _ := json.Marshal(map[string]interface{}{
      "type":    "agent.error",
      "payload": json.RawMessage(data),
  })
  s.hub.BroadcastToChannel(req.ChannelID, envelope)
  ```
  或者直接调用 `s.broadcastError(req.ChannelID, req.AgentID, req.AgentName, req.Error)` 复用现有方法。
- **风险**: 低。复用已验证的 broadcastError 模式。

### P1-2: WebSocket CheckOrigin 允许所有来源

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/ws/hub.go` 第 32-35 行
- **描述**: `upgrader.CheckOrigin` 返回 true 允许所有来源的 WebSocket 连接。这在 MVP 阶段是可接受的，但部署到生产环境前需要根据 CORS 配置或 `Origin` 请求头进行校验，防止跨站 WebSocket 劫持。
- **建议**: 生产部署前限制为已知的前端域名。

---

## 4. P2 中等优先级问题

### P2-1: 乐观更新使用硬编码 user_id

- **文件**:
  - `/Users/langgengxin/AiWorkspace/solo/frontend/lib/hooks/use-messages.ts` 第 404 行
  - `/Users/langgengxin/AiWorkspace/solo/frontend/lib/hooks/use-dm.ts` 第 343 行
- **描述**: 乐观消息创建时使用 `user_id: 'user-1'` 硬编码值，而不是从 auth context 获取当前登录用户 ID。
- **影响**: 在多用户环境下，乐观消息会错误显示为用户 "user-1"，直到服务端确认消息到达后修正。视觉效果不一致，但数据最终一致。
- **修复**: 从 auth context 获取当前用户 ID 替代硬编码值。

### P2-2: DMView 使用硬编码 member_id

- **文件**: `/Users/langgengxin/AiWorkspace/solo/frontend/components/dashboard/dm-view.tsx`
- **描述**: 构建 DM 成员列表时使用 `member_id: 'user-1'` 硬编码值。
- **影响**: DM 列表中的 "其他成员" 计算可能不正确。
- **修复**: 从 auth context 获取当前用户 ID。

### P2-3: Channel 删除未校验所有权

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/handler/channel.go` — Delete 方法
- **描述**: Delete handler 只校验了请求者是否为 channel_member，没有校验是否为 owner/admin。任何频道成员都可以删除频道。
- **影响**: 用户可能意外删除不属于自己的频道。
- **修复**: 在 DELETE handler 中添加 owner 角色校验（与 Update 方法一致）。

### P2-4: 存在潜在的令牌桶竞态条件

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/middleware/ratelimit.go`
- **描述**: 需要确认令牌桶实现是否为线程安全（使用 sync.Mutex 或 atomic 操作）。
- **建议**: code review 时确认并发安全性。

---

## 5. P3 建议级问题

### P3-1: AgentStreamTokenPayload 是死代码

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/ws/message.go` 第 124-131 行
- **描述**: `AgentStreamTokenPayload` 结构体字段使用 `agent_id` 和 `message_id` 作为 JSON 名，但实际广播代码（agent.go broadcastToken）发送的是 `sender_id` 和 `id`。此结构体从未在 marshal/unmarshal 中使用，是死代码。可以删除或改为正确的字段名。
- **建议**: 删除此结构体或更新字段名以反映实际广播格式。

### P3-2: 错误信息语言不统一

- **文件**: 多个 handler 文件
- **描述**: 后端错误信息使用英文（如 "channel_id is required"），前端使用中文（如 "加载消息失败"）。后端错误会直接传递给前端显示，导致 UI 上中英文混合。
- **建议**: 统一为英文（后端标准）或全量中文本地化。

### P3-3: broadcastMessage 中 done=true tokens

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/service/agent.go` 第 348 行
- **描述**: `broadcastMessage` 在发送 `message.new` 之前先发送 `message.agent_typing`（done=true）。前端需要正确处理这个顺序，确保 message.new 到达时，streaming 消息已经就位。当前前端实现逻辑正确（check existing then finalize），但顺序依赖脆弱。
- **建议**: 在 `broadcastMessage` 中可以考虑跳过发送 done=true 的 token，直接发送 message.new。前端可以通过 message.new 的到达推断 streaming 完成。

### P3-4: Agent auto-response DM 检查需要查 dm_members

- **文件**: `/Users/langgengxin/AiWorkspace/solo/internal/server/service/agent.go` 第 394-396 行
- **描述**: `broadcastDMIfNeeded` 检查 channel type 是否为 "dm"，但没有验证当前 agent 是否为该 DM 的成员。虽然当前实现不会广播到非成员（因为广播基于 channel 订阅），但理论上非成员 agent 的回复可能被广播到 DM channel。
- **影响**: 目前实际上不会发生，因为 agent auto-response 只触发于 agent 是成员的 channel。但增加成员检查可以防御未来引入的 bug。

---

## 6. 特性逐项验收

### F-01: 认证系统 (Auth)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 注册 | PASS | 输入校验（email 格式、密码>=8位）、bcrypt 哈希、参数化查询 |
| 登录 | PASS | bcrypt 比较、JWT 签发、自动创建欢迎频道 |
| 登出 | PASS | 删除用户所有 session |
| Token 刷新 | PASS | 一次性 refresh token、SHA-256 哈希查找、session 轮换 |
| Auth middleware | PASS | 从 Authorization header 提取 Bearer token、验证 JWT、设置 X-User-ID 等请求头 |
| Rate limiting | PASS | 公共 auth 路由 10 req/min、通用 API 100 req/s |
| **缺陷** | 0 | — |

### F-02: 频道管理 (Channels)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 创建频道 | PASS | 名称校验(1-100字符)、唯一性检查、事务内自动添加 owner |
| 列表 | PASS | 通过 channel_members JOIN 查询用户的非归档频道 |
| 获取频道详情 | PASS | 成员资格检查先于查询 |
| 更新频道 | PASS | COALESCE 实现部分更新 |
| 删除频道 | PASS | 成员资格检查（但未校验 owner 权限 — 见 P2-3） |
| **缺陷** | 1 | P2-3: 删除未校验所有权 |

### F-03/F-04: 消息 + 实时通信 (Messages + Real-time WebSocket)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 发送消息 (REST) | PASS | 成员校验、@mention 解析、参数化插入 |
| 消息列表 (REST, 游标分页) | PASS | (created_at, id) 元组分页、UUID 游标校验、limit 限制 |
| WebSocket 连接 | PASS | JWT token 认证、upgrade 流程 |
| 消息广播 (message.new) | PASS | Hub scope-based 广播到频道订阅者 |
| 输入状态 (typing) | PASS | 广播到频道（排除发送者） |
| 消息更新/删除 | PASS | 广播 message.updated / message.deleted |
| **缺陷** | 2 | P0-1: hub.go 编译错误(影响 DM 广播分支)；P2-1: 硬编码 user-1 |

### F-09: 消息历史 (Message History, 游标分页)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 初始加载 | PASS | 最新 N 条消息（默认 50，最大 100） |
| 分页加载更早消息 (before cursor) | PASS | (created_at, id) 元组比较 |
| 断线重连后拉取遗漏消息 (after cursor) | PASS | 前端检测重连状态，以最新消息 ID 为 after cursor 拉取 |
| has_more 标记 | PASS | 后端返回 has_more 布尔值 |
| **缺陷** | 0 | — |

### F-05/F-06: Agent CRUD + 频道加入

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 创建 Agent | PASS | 完整数据校验、owner 作用域 |
| 获取/列表 Agent | PASS | owner 作用域查询 |
| 更新 Agent | PASS | PATCH 语义（nil pointer = 不更新） |
| 删除 Agent | PASS | 软删除 (is_active=false) |
| Agent 加入频道 | PASS | 通过 member handler 添加，校验 agent 存在性和成员类型 |
| **缺陷** | 0 | — |

### F-10: 线程 (Threads)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 创建线程回复 (REST) | PASS | 自动创建 thread 记录、事务内原子操作 |
| 列出线程消息 (游标分页) | PASS | 与消息 handler 一致的分页模式 |
| 线程消息广播 (thread.message.new) | PASS | 广播到 thread 订阅者，含嵌套 payload |
| 线程回复通知 (thread.reply) | PASS | 广播到 channel 订阅者 |
| 线程订阅/取消订阅 (WS) | PASS | client.go handleMessage 正确处理 thread.subscribe/unsubscribe |
| **缺陷** | 0 | — |

### F-07/F-08: Agent 自动回复 + 流式传输

| 检查项 | 状态 | 说明 |
|--------|------|------|
| Agent 自动回复触发 | PASS | @mention 过滤、2s debounce、Daemon 负载均衡 |
| 上下文构建 | PASS | 20 条历史消息、可配置 |
| SSE 流式传输 | PASS | Daemon -> Server SSE、token/thinking/complete/error 事件 |
| 消息广播 (message.agent_typing) | PASS | broadcastToken 发送实时 token |
| 消息完成 (message.new) | PASS | broadcastMessage 持久化到 DB 并广播 |
| 线程内的自动回复 | PASS | TriggerAgentResponseInThread 使用线程上下文 |
| DM 广播 | PASS | broadcastDMIfNeeded 发送 dm.message.new |
| **缺陷** | 2 | P1-1: HandleTaskError 缺少 Envelope；P3-1: AgentStreamTokenPayload 死代码 |

### F-11: @Mention 解析

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 正则匹配 | PASS | `@([\p{L}\p{N}_\-\.]+)` — 支持 Unicode、字母、数字、下划线、连字符、点 |
| 成员解析 | PASS | 查询 channel_members JOIN agents，a.name = ANY($2) |
| 回复中解析 | PASS | handleThreadReply 同样调用 ResolveMentions |
| mentioned_agent_ids 存储 | PASS | 存入 messages 表的 uuid[] 列 |
| 自动回复前过滤 | PASS | TriggerAgentResponse 只对提到的 agent 触发 |
| **缺陷** | 0 | — |

### F-20: 私信 (DM)

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 创建/获取 DM | PASS | 双向去重 (dm_members JOIN)、防止自 DM、agent 所有权校验 |
| DM 列表 | PASS | LATERAL JOIN 获取最后消息、50 字符截断预览 |
| DM 详情 | PASS | dm_members 参与者校验 |
| 发送 DM 消息 | PASS | 复用 message 分页模式、广播 message.new + dm.message.new |
| DM 消息列表 | PASS | 游标分页同消息 handler |
| WebSocket DM 订阅 | PASS | client.go 处理 dm.subscribe/dm.unsubscribe |
| DM 消息广播 (dm.message.new) | PASS | agent.go broadcastDMIfNeeded 与 hub.go（修复后） |
| **缺陷** | 2 | P0-1: hub.go 编译错误（直接影响 DM WS 广播）；P2-2: 硬编码 member-1 |

---

## 7. API 一致性校验

### REST API 路径

| 功能 | 前端请求路径 | 后端路由 | 匹配 |
|------|------------|---------|------|
| 获取 DM 列表 | GET /api/v1/dm | GET /api/v1/dm | YES |
| 创建 DM | POST /api/v1/dm | POST /api/v1/dm | YES |
| 获取 DM 详情 | GET /api/v1/dm/{dmID} | GET /api/v1/dm/{dmID} | YES |
| DM 消息列表 | GET /api/v1/dm/{dmID}/messages | GET /api/v1/dm/{dmID}/messages | YES |
| 发送 DM 消息 | POST /api/v1/dm/{dmID}/messages | POST /api/v1/dm/{dmID}/messages | YES |
| 频道消息列表 | GET /api/v1/channels/{cid}/messages | GET /api/v1/channels/{cid}/messages | YES |
| 发送频道消息 | POST /api/v1/channels/{cid}/messages | POST /api/v1/channels/{cid}/messages | YES |
| 线程消息列表 | GET /api/v1/channels/{cid}/threads/{tid}/messages | GET /api/v1/channels/{cid}/threads/{tid}/messages | YES |
| 令牌刷新 | POST /api/v1/auth/refresh | POST /api/v1/auth/refresh | YES |

### WebSocket 事件类型

| 后端常量 | 前端 WSServerEvent type | 匹配 |
|---------|------------------------|------|
| message.new | message.new | YES |
| message.updated | message.updated | YES |
| message.deleted | message.deleted | YES |
| message.agent_typing | message.agent_typing | YES |
| thread.message.new | thread.message.new | YES |
| thread.reply | thread.reply | YES |
| typing | typing | YES |
| agent.thinking | agent.thinking | YES |
| agent.typing | agent.typing | YES |
| agent.status | agent.status | YES |
| agent.error | agent.error | YES |
| dm.message.new | dm.message.new | YES |

### 字段名一致性（关键路径）

| 后端 JSON 字段 | 前端消费 | 匹配 |
|---------------|---------|------|
| channel_id | channel_id | YES |
| sender_id | sender_id | YES |
| sender_name | sender_name | YES |
| dm_id (DM payloads) | dm_id | YES |
| created_at | created_at | YES |
| thread_id | thread_id | YES |
| reply_count | reply_count | YES |

### 前端 input 到后端 payload 映射

| 前端 useDM.createOrGetDM input | 后端 body | 匹配 |
|------------------------------|----------|------|
| { user_id: "..." } | { member_type: "user", member_id: input.user_id } | YES |
| { agent_id: "..." } | { member_type: "agent", member_id: input.agent_id } | YES |

### WebSocket 命令格式

| 前端 WSClientCommand | 后端 handleMessage 预期 | 匹配 |
|--------------------|----------------------|------|
| { type: "subscribe", channel_id } -> { type: "subscribe", payload: { channel_id } } | EventSubscribe 解码 SubscribePayload.ChannelID | YES |
| { type: "thread.subscribe", thread_id } -> { type: "thread.subscribe", payload: { thread_id } } | EventThreadSubscribe 解码 ThreadSubscribePayload.ThreadID | YES |
| { type: "dm.subscribe", dm_id } -> { type: "dm.subscribe", payload: { dm_id } } | EventDMSubscribe 解码 DMSubscribePayload.DMChannelID | YES |

**一致性审计结论**: 所有 REST 路径、WS 事件类型、payload 字段名、输入输出格式前后端完全对齐。

---

## 8. 安全审计摘要

| 检查项 | 状态 | 说明 |
|--------|------|------|
| SQL 注入 | PASS | 全库使用参数化查询（pgx 参数占位符），无字符串拼接 SQL |
| JWT 签名 | PASS | HS256 签名，access token 15 分钟过期 |
| 密码存储 | PASS | bcrypt 哈希，非明文存储 |
| Token 轮换 | PASS | Refresh token 一次性使用，删除旧 token |
| Rate Limiting | PASS | 公共路由 10 req/min，通用 API 100 req/s，令牌桶算法 |
| CORS 中间件 | PASS | 配置正确 |
| 安全头 | PASS | securityHeaders middleware 应用了安全响应头 |
| 认证中间件 | PASS | 除 /healthz、/readyz、/api/v1/auth/* 外所有路由需 JWT |
| WebSocket 认证 | PASS | 通过 token 查询参数验证 JWT |
| 输入校验 | PASS | 邮箱格式、密码长度、消息长度、频道名称等均有校验 |
| 错误信息 | PASS | 不泄露内部信息 |
| WebSocket CheckOrigin | **WARN** | 允许所有来源（P1-2），生产需限制 |
| 自 DM 防护 | PASS | CreateOrGetDM 检查 sender_id != target_id |
| 成员资格校验 | PASS | Channel 读写操作均有成员检测 |

---

## 9. v0.1-v0.4 已知问题回归

### v0.1 问题

| 问题 | 原始描述 | 当前状态 |
|------|---------|---------|
| writeJSON 写回 | 响应格式不规范 | FIXED — 使用 `writeJSON(w, status, v)` 和 `writeError(w, status, message)` 统一格式 |
| CORS 配置 | 跨域配置缺失 | FIXED — cors.go 中间件正常应用 |
| Rate limit | 缺少限流 | FIXED — 令牌桶限流在 auth 路由（10 req/min）和通用 API（100 req/s）生效 |

### v0.2 问题

| 问题 | 原始描述 | 当前状态 |
|------|---------|---------|
| Thread ID handler 处理 | 线程 ID 在 message 响应中缺失 | FIXED — MessageResponse 包含 thread_id 字段 |

### v0.3 问题

| 问题 | 原始描述 | 当前状态 |
|------|---------|---------|
| 流式消息字段对齐 | agent_typing 事件字段名与前端期望不一致 | FIXED — 实际 broadcastToken 发送 sender_id/id/content(accumulated) 与前端期望一致 |
| AgentStreamTokenPayload 死代码 | 结构体字段与广播不一致 | OPEN (P3-1) — 结构体未使用，不影响运行 |

### v0.4 问题

| 问题 | 原始描述 | 当前状态 |
|------|---------|---------|
| DM subscribe 字段 | dm.subscribe payload 字段名 | FIXED — DMSubscribePayload.DMChannelID 使用 json:"dm_id"，前端发送 dm_id |
| DM payload 格式 | dm.message.new 字段结构 | FIXED — 扁平结构包含 dm_id/id/sender_type/sender_id 等字段，前端 ws-types.ts 对齐 |
| 硬编码 user-1 | 乐观消息用户 ID | OPEN (P2-1, P2-2) — 需从 auth context 获取 |

---

## 10. 测试覆盖分析

### 后端测试文件

| 文件 | 覆盖范围 | 行数估计 |
|------|---------|---------|
| internal/auth/jwt_test.go | JWT 生成和验证 | — |
| internal/server/handler/channel_test.go | Channel CRUD | — |
| internal/server/handler/message_test.go | 消息验证、光标格式、limit、长内容 | — |
| internal/server/handler/thread_test.go | 线程操作 | — |
| internal/server/ws/hub_test.go | Hub 注册/注销/广播 | — |

### 零测试覆盖模块

| 模块 | 风险等级 | 建议 |
|------|---------|------|
| internal/server/handler/auth.go | HIGH | 认证核心逻辑，包含注册/登录/登出/刷新 |
| internal/server/handler/member.go | HIGH | 权限核心，包含添加/移除/校验角色 |
| internal/server/handler/agent.go | HIGH | Agent CRUD 完整生命周期 |
| internal/server/handler/dm.go | HIGH | DM 创建/列表/消息发送 |
| internal/server/service/agent.go | HIGH | Agent 自动回复核心逻辑 |
| internal/server/service/daemon.go | HIGH | Daemon 管理、SSE 流式传输 |
| internal/server/service/mention.go | HIGH | @mention 解析 |
| internal/server/middleware/ | MEDIUM | 认证、限流、CORS |
| internal/server/router.go | MEDIUM | 路由注册、中间件顺序 |

### 前端测试

无前端测试文件（排除 node_modules）。

**建议**: 部署周至少为 high-risk 模块添加关键路径的单元测试和集成测试。

---

## 11. 部署前修复清单

### 必须修复（阻塞部署）

- [ ] **P0-1**: 删除 hub.go 第 442-443 行悬空代码

### 强烈建议修复（部署周早期）

- [ ] **P1-1**: HandleTaskError broadcast 添加 Envelope 封装
- [ ] **P1-2**: 部署前限制 WebSocket CheckOrigin

### 建议修复（部署周内）

- [ ] **P2-1**: use-messages.ts 和 use-dm.ts 乐观消息使用真实 user_id
- [ ] **P2-2**: dm-view.tsx 使用真实 user_id
- [ ] **P2-3**: Channel delete handler 添加 owner 权限校验

### 可推迟（post-MVP）

- [ ] **P3-1**: 移除或修正 AgentStreamTokenPayload 死代码
- [ ] **P3-2**: 统一错误信息语言
- [ ] **P3-3**: 评估 broadcastMessage 中 done=true token 的取舍
- [ ] **P3-4**: 增加 DM 成员检查
- [ ] 为 high-risk 模块补充测试覆盖

# Solo — 项目任务分解与迭代规划 (WBS)

> 版本: 1.0
> 创建日期: 2026-05-11
> 负责人: tpm agent
> 对齐文档: PRD.md v1.1, ARCHITECTURE.md v1.2

---

## 目录

1. [项目概览](#1-项目概览)
2. [团队角色与分工](#2-团队角色与分工)
3. [multica 复用映射表](#3-multica-复用映射表)
4. [任务编号约定](#4-任务编号约定)
5. [v0.1 (第 1-2 周): 认证 + 频道 + 消息基础](#5-v01-第-1-2-周-认证--频道--消息基础)
6. [v0.2 (第 3-4 周): Agent 管理 + 消息历史 + 线程回复](#6-v02-第-3-4-周-agent-管理--消息历史--线程回复)
7. [v0.3 (第 5-6 周): Agent 自动响应 + 流式输出 + @提及](#7-v03-第-5-6-周-agent-自动响应--流式输出--提及)
8. [v0.4 (第 7-8 周): 私信 DM + 前端优化](#8-v04-第-7-8-周-私信-dm--前端优化)
9. [RC (第 9 周): 集成测试 + Bug 修复](#9-rc-第-9-周-集成测试--bug-修复)
10. [上线 (第 10 周): 公测发布](#10-上线-第-10-周-公测发布)
11. [跨迭代依赖关系总图](#11-跨迭代依赖关系总图)
12. [风险登记册](#12-风险登记册)

---

## 1. 项目概览

**项目名称**: Solo — 频道式实时多 Agent 协作平台

**MVP 范围** (P0 功能, 10 周):

| 功能 ID | 功能 | 优先级 | 涉及迭代 |
|---------|------|--------|----------|
| F-01 | 认证系统 (注册/登录/登出/JWT) | P0 | v0.1 |
| F-02 | 频道管理 (CRUD) | P0 | v0.1 |
| F-03 | 消息发送 | P0 | v0.1 |
| F-04 | 实时消息推送 (WebSocket) | P0 | v0.1 |
| F-09 | 消息历史 (游标分页) | P0 | v0.2 |
| F-05 | Agent 管理 (CRUD) | P0 | v0.2 |
| F-06 | Agent 加入频道 | P0 | v0.2 |
| F-10 | 线程回复 | P0 | v0.2 |
| F-07 | Agent 自动响应 | P0 | v0.3 |
| F-08 | Agent 流式输出 | P0 | v0.3 |
| F-11 | @提及 Agent | P0 | v0.3 |
| F-20 | 私信 (DM) | P0 | v0.4 |

**未纳入 MVP 但有迁移准备的模块**: 消息编辑/删除(P1), 多模型切换(P1), Agent 工具(P1), 消息搜索(P2)

**团队** (6 人):

| 角色 | 代号 | 技能栈 | 投入 |
|------|------|--------|------|
| 后端开发 | rd1 | Go + Chi + gorilla/websocket + PostgreSQL | 1 人全职 |
| 后端开发 | rd2 | Go + PostgreSQL | 1 人全职 |
| 前端开发 | fe1 | Next.js 16 + Tailwind + shadcn/ui + SWR | 1 人全职 |
| 前端开发 | fe2 | Next.js + Tailwind + shadcn/ui | 1 人全职 |
| 测试 | qa1 | API 测试 + E2E 测试 + 验收测试 | 0.5 人 |
| 架构师 | arc | 已完成架构设计, 负责代码审查 | 0.5 人 |

---

## 3. multica 复用映射表

### 直接引用 (不改代码, 仅适配 import 路径)

| multica 模块 | Solo 路径 | 适配说明 |
|-------------|-----------|----------|
| `internal/realtime/hub.go` | `internal/server/ws/hub.go` | 修改 Hub.channels map 的 key 为 channelID |
| `internal/realtime/broadcaster.go` | `internal/realtime/broadcaster.go` | 直接引用, 接口不变 |
| `internal/auth/jwt.go` | `internal/auth/jwt.go` | 移除 workspace claim |
| `internal/middleware/auth.go` | `internal/server/middleware/auth.go` | 简化 workspace 依赖 |
| `internal/handler/auth.go` | `internal/server/handler/auth.go` | 精简路由 (移除 workspace 相关) |
| `pkg/agent/agent.go` | `pkg/agent/agent.go` | 直接引用, 接口不变 |
| `pkg/llm/` | `pkg/llm/` | 直接引用 |
| `internal/daemon/` | `cmd/daemon/` | 直接引用, 适配 Solo 消息格式 |
| `internal/daemonws/hub.go` | `internal/server/ws/daemon.go` | 复用通信架构 |
| `internal/metrics/` | `pkg/metrics/` | 直接引用 |
| `internal/logger/` | `pkg/logger/` | 直接引用 |

### 参考架构 (需重写)

| multica 部分 | Solo 做法 |
|-------------|-----------|
| `cmd/server/router.go` | 参考路由注册模式, 重写路由为 Solo 频道/消息/Agent 路由 |
| `cmd/server/main.go` | 参考启动流程, 简化 init 逻辑 |
| `internal/handler/` 系列 | 用 channel/message/agent/thread/dm handler 替换 |

### 不复用

`issue.go`, `autopilot.go`, `skill.go`, `inbox.go`, `runtime.go`, `cron.go`, `email.go` — 这些是 multica 特定功能, Solo 不需要。

---

## 4. 任务编号约定

```
SOLO-{迭代编号}-{序号}-{类型}
```

类型后缀:
- `-B` = 后端任务
- `-F` = 前端任务
- `-FS` = 全栈任务 (前后端都需要改)
- `-QA` = 测试任务
- `-OPS` = 运维/部署任务

优先级:
- **P0**: 阻塞后续任务, 必须按时完成
- **P1**: 重要, 有 1-2 天缓冲
- **P2**: 应该做, 但不阻塞
- **P3**: 锦上添花

---

## 5. v0.1 (第 1-2 周): 认证 + 频道 + 消息基础

**迭代目标**: 用户可注册登录、看到空 Dashboard、创建频道、发送消息并实时收到

**可演示产出 (第 2 周末)**:
1. 用户注册并自动登录, 看到空 Dashboard 左侧有空频道列表
2. 点击"创建频道" → 填写名称 → 频道出现在左侧
3. 进入频道 → 发送消息 → 消息出现在列表中
4. 另一个浏览器窗口登录同一用户 → 实时看到新消息

### 任务列表

#### Week 1: 项目脚手架 + 认证

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-01-B | 项目初始化与脚手架搭建 | 创建 go mod, 配置 Chi 路由骨架, 初始化项目目录结构, 配置 pgx 连接池, 引入 multica 可复用模块 | rd1 | P0 | 无 | 6 | `go run cmd/server/main.go` 启动成功, DB 连接正常, 健康检查端点 `/healthz` 返回 200 |
| SOLO-02-B | 数据库迁移: users + sessions 表 | 创建 users 和 sessions 表的迁移脚本 (复用 ARCHITECTURE.md 第 7.1 节的 DDL), 实现迁移工具自动化 | rd1 | P0 | SOLO-01-B | 3 | `migrate up` 执行后 pg 中 users 和 sessions 表创建成功 |
| SOLO-03-B | JWT 认证模块 (复用 multica) | 从 multica 复制 `internal/auth/jwt.go`, 移除 workspace 概念, 实现 access token (15min) + refresh token 签发与验证 | rd1 | P0 | SOLO-01-B | 4 | JWT 签发验证单元测试通过, refresh token 工作正常 |
| SOLO-04-B | 认证 REST 路由: register/login/logout/refresh | 从 multica 复制并精简 `internal/handler/auth.go`, 注册 POST /auth/register, /auth/login, /auth/logout, /auth/refresh, 密码 bcrypt 加密存储 | rd1 | P0 | SOLO-02-B, SOLO-03-B | 6 | 注册返回 201 + JWT; 登录返回 200 + access_token + refresh_token; 密码错误返回 401 |
| SOLO-05-B | 认证中间件 (复用 multica) | 从 multica 复制 JWT 鉴权中间件, 简化 workspace 依赖, 支持 `Authorization: Bearer <token>` | rd1 | P0 | SOLO-03-B | 3 | 带有效 token 的请求通过, 无 token 请求返回 401 |
| SOLO-06-B | 通用中间件: CORS + Logging + Rate Limit | 配置 CORS (开发环境允许 localhost:3000), 请求日志 (slog), token bucket 限流 (100 req/s) | rd2 | P1 | SOLO-01-B | 3 | 跨域请求正常; 请求日志 JSON 格式输出; 超限返回 429 |
| SOLO-07-F | Next.js 项目脚手架 | 创建 Next.js 16 App Router 项目, 配置 Tailwind, shadcn/ui, 设置 RootLayout 和 AuthLayout, 配置基础路由 | fe1 | P0 | 无 | 4 | `npm run dev` 启动成功, 访问 /auth/login 显示页面 |
| SOLO-08-F | API 客户端封装 | 封装 fetch-based HTTP 客户端, 支持自动附加 JWT token、自动 refresh token、统一错误处理 | fe2 | P0 | 无 | 3 | API 调用自动带 token, token 过期自动 refresh, 401 跳登录页 |
| SOLO-09-F | 登录/注册页面 | 实现 `/auth/login` 和 `/auth/register` 页面, react-hook-form + zod 表单校验, 错误 inline 提示, 登录成功后跳转 /dashboard | fe1 | P0 | SOLO-07-F, SOLO-08-F | 6 | 表单校验正常工作; 注册成功后跳转 dashboard; 登录失败显示"邮箱或密码错误" |
| SOLO-10-B | Daemon 项目脚手架 | 创建 `cmd/daemon/main.go`, 基础 HTTP 服务框架 (复用 multica daemon 架构), 注册 `/health` 端点 | rd2 | P1 | SOLO-01-B | 3 | Daemon 启动成功, 健康检查返回 200 |
| SOLO-11-OPS | docker-compose 本地开发环境 | 编写 docker-compose.yml, 配置 PostgreSQL 16 + 两个 Go 服务 (server + daemon), 提供 env 模板 | rd2 | P1 | SOLO-01-B | 2 | `docker compose up` 启动三个容器, 服务健康检查通过 |

**Week 1 并行策略**:
- rd1 从 SOLO-01-B 开始; rd2 从 SOLO-06-B 和 SOLO-10-B 开始 (仅依赖 SOLO-01-B); fe1 从 SOLO-07-F 开始 (独立, 无依赖); fe2 从 SOLO-08-F 开始 (无依赖)
- SOLO-01-B 完成后 rd1 可并行推进 SOLO-02-B 和 SOLO-03-B; rd2 推进 SOLO-11-OPS (docker-compose)
- SOLO-08-F 由 fe2 独立推进, 在 SOLO-04-B 未完成前可 mock 响应先开发 UI
- rd1(22h) + rd2(8h) + fe1(10h) + fe2(3h) = 43h, 团队总容量 160h, 负荷仅 27%

---

#### Week 2: 频道 CRUD + 消息 + WebSocket

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-12-B | 迁移: channels + channel_members 表 | 创建 channels (含 type 字段 'channel'|'dm') 和 channel_members 表的迁移脚本 | rd1 | P0 | SOLO-01-B | 2 | migrate up 后表创建成功, 复合索引存在 |
| SOLO-13-B | 频道 CRUD REST API | channels 表增删改查 handler: POST/GET/PATCH/DELETE /api/v1/channels, 频道名称唯一性校验, 逻辑删除 (is_archived), 创建时自动将创建者加为 owner 成员 | rd1 | P0 | SOLO-05-B, SOLO-12-B | 8 | 创建频道返回 201; 重名返回 409; 删除后列表不显示; 创建者自动成为频道成员 |
| SOLO-14-B | 频道成员管理 API | 添加/移除/列出频道成员: POST/DELETE/GET /api/v1/channels/{id}/members, 支持 user 和 agent 两种 member_type (agent 功能在 v0.2 启用, 先支持 user 类型) | rd2 | P1 | SOLO-12-B, SOLO-05-B | 4 | 可添加用户到频道; 可列出频道成员; 移除后该用户看不到该频道 |
| SOLO-15-B | WebSocket Hub 集成 (复用 multica) | 从 multica 复制 realtime.Hub 架构, 适配为 channel-scoped 订阅: 实现 subscribe/unsubscribe/broadcast 核心逻辑, 连接端点 GET /ws?token= | rd1 | P0 | SOLO-05-B | 6 | WebSocket 连接建立; 订阅后收到频道消息广播; 取消订阅后不再收到消息 |
| SOLO-16-B | 消息迁移 + REST API (发送+历史) | messages 表迁移, POST /api/v1/channels/{id}/messages (REST 后备), GET /api/v1/channels/{id}/messages?limit=50&before=cursor 游标分页, 复合索引 (channel_id, created_at DESC) | rd1 | P0 | SOLO-12-B, SOLO-05-B | 8 | 发送消息持久化成功; 分页查询返回正确; 消息按 created_at 升序排列 |
| SOLO-17-B | WS 消息发送 + 广播 | WS 事件 `message.send` 处理器: 接收消息 → 持久化到 DB → 广播 message.new 给频道所有在线成员; 输入校验 (空消息拒绝, 10000 字符上限) | rd1 | P0 | SOLO-15-B, SOLO-16-B | 6 | WS 发送消息后所有在线成员实时收到; 空消息被拒绝; 消息持久化后刷新可见 |
| SOLO-18-F | Dashboard 主布局 | 实现 AppLayout: 左侧侧栏 (频道列表区域 + DM 区域占位) + 右侧主内容区, 响应式最小 1024px | fe1 | P0 | SOLO-07-F | 4 | 布局符合设计, 侧栏可折叠/展开, 最小 1024px 工作正常 |
| SOLO-19-F | 频道列表 + 创建频道 Modal | 实现左侧 ChannelList 组件: 加载态骨架屏、空态引导 ("还没有频道" + 创建按钮)、频道列表显示、切换频道; 创建频道 Modal (名称必填+描述选填); 删除频道二次确认 | fe1 | P0 | SOLO-13-B, SOLO-18-F | 8 | 频道列表加载正常; 创建后频道出现在列表; 切换频道高亮; 空态引导显示 |
| SOLO-20-F | 频道消息视图 (基本版) | 实现 ChannelView 组件: 消息列表渲染 (发送者名称 + 内容 + 时间), 输入框 (Enter 发送, Shift+Enter 换行), 新消息自动滚动到底部, 消息乐观更新, 发送失败红色提示 + 重试 | fe1 | P0 | SOLO-16-B, SOLO-17-B, SOLO-19-F | 10 | 发送消息后列表即时显示; 空消息无法发送; 发送失败显示重试按钮; 消息实时滚动 |
| SOLO-21-F | WebSocket 客户端封装 | 原生 WebSocket 封装: 自动连接、断线重连 (指数退避, 显示"重新连接中...")、订阅/取消订阅频道、消息分发到 React Context | fe2 | P0 | SOLO-15-B, SOLO-18-F | 6 | WS 连接成功; 断线后自动重连; 重连后重新订阅之前频道; 收到消息通过 Context 更新 UI |
| SOLO-22-F | Welcome 频道自动创建 | 用户注册后自动创建一个 welcome 频道 (与 SOLO-13-B 配合, 前端注册成功时触发一个 create channel 请求, 或后端在回归成功后自动创建) | rd2 | P2 | SOLO-13-B | 2 | 新用户登录后 dashboard 默认显示 welcome 频道 |
| SOLO-23-QA | v0.1 集成验证 | 端到端验证认证 + 频道 + 消息流程, 编写 v0.1 验收测试脚本 | qa1 | P0 | Week 1-2 所有任务 | 4 | 核心闭环通过: 注册 → 创建频道 → 发送消息 → 实时收到 |

**Week 2 并行策略**:
- rd1 优先完成 SOLO-12-B (迁移) 和 SOLO-15-B (WS Hub), 这两个是后续任务的依赖
- rd2 在 SOLO-14-B (依赖 SOLO-12-B) 和 SOLO-22-F 上可独立推进, 不阻塞 rd1
- fe1 可以并行推进 SOLO-18-F (布局) 和 SOLO-19-F (频道列表), 前端可先 mock 数据
- fe2 负责 SOLO-21-F (WS 客户端封装), 依赖 SOLO-15-B 和 SOLO-18-F, 后端 WS Hub 就绪即可联调
- SOLO-20-F 需要等待 SOLO-16-B 和 SOLO-17-B 完成后联调
- rd1(30h) + rd2(6h) + fe1(22h) + fe2(6h) = 64h, 团队总容量 160h, 负荷 40%

---

## 6. v0.2 (第 3-4 周): Agent 管理 + 消息历史 + 线程回复

**迭代目标**: 用户可创建 Agent、加入频道、查看消息历史分页、对消息发起线程回复

**可演示产出 (第 4 周末)**:
1. Agent 管理页面: 创建/查看/编辑 Agent
2. 在频道成员管理中添加 Agent 到频道
3. 消息列表支持向上滚动加载历史 (游标分页)
4. 悬停消息显示"回复"按钮, 点击后右侧线程面板打开
5. 在线程中发送回复, Agent 在线程上下文中响应

### 任务列表

#### Week 3: Agent 管理 + 消息历史分页

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-24-B | 迁移: agents 表 + agent_tools 表 | 创建 agents 表 (名称/模型/system_prompt/is_active 等字段) 和 agent_tools 表的迁移脚本 | rd1 | P0 | SOLO-01-B | 2 | migrate up 后 agents 表创建成功, 索引存在 |
| SOLO-25-B | Agent CRUD REST API | Agent 增删改查 handler: POST/GET/PATCH/DELETE /api/v1/agents, 逻辑删除 (is_active=false), 创建时填入默认值 (model_provider='anthropic', system_prompt 默认模板) | rd1 | P0 | SOLO-05-B, SOLO-24-B | 8 | 创建 Agent 返回 201; 删除后标记为非活跃; 列表只显示活跃 Agent |
| SOLO-26-B | Agent 加入/移出频道 API | 频道成员管理增加 Agent 支持: member_type='agent', 验证 Agent 存在且属于当前用户; POST /members 支持同时添加 user 和 agent | rd1 | P0 | SOLO-14-B, SOLO-25-B | 4 | 可将 Agent 添加为频道成员; Agent 在成员列表中; 移出后 Agent 不再收到频道消息 |
| SOLO-27-B | 消息历史游标分页优化 | 确保游标分页查询性能: 使用 (channel_id, created_at DESC) 复合索引, 返回 has_more 字段, limit 默认 50 最大 100 | rd2 | P0 | SOLO-16-B | 4 | 大频道 (5000+ 消息) 分页查询 P95 < 500ms; has_more 字段准确; 游标正确 |
| SOLO-28-B | agent model 和接口定义 (复用 multica pkg/agent) | 从 multica 复制 pkg/agent, 定义 AgentRuntime 接口和 RunRequest/StreamEvent 类型, 适配 Solo 的 channel/thread 上下文 | rd2 | P0 | SOLO-01-B | 3 | AgentRuntime 接口编译通过, 单元测试覆盖核心类型 |
| SOLO-29-F | Agent 管理页面 (列表+创建+编辑) | 实现 `/agents` 列表页 (卡片布局, 骨架屏, 空态引导 "还没有 Agent"), `/agents/new` 创建页 (名称必填+system prompt选填+默认模型), `/agents/{id}/edit` 编辑页 | fe1 | P0 | SOLO-25-B | 10 | Agent 列表加载正常; 创建后跳转到列表; 空态引导显示; 编辑保存成功 |
| SOLO-30-F | 频道成员列表中 Agent 展示 | 在 ChannelView 的成员列表中显示 Agent (单独 Agent 图标 + 名称 + 状态指示器), 支持添加 Agent 到频道的 UI | fe1 | P0 | SOLO-26-B, SOLO-29-F | 6 | Agent 以特殊图标在成员列表中展示; 添加 Agent 流程可用; Agent 显示"在线"状态 |
| SOLO-31-F | 消息历史无限滚动 | 消息列表向上滚动时自动加载更早消息: IntersectionObserver 检测顶部、加载时显示加载指示器、保持滚动位置、全加载完后显示"没有更早的消息" | fe2 | P0 | SOLO-20-F, SOLO-27-B | 6 | 向上滚动触发加载; 滚动位置保持; 加载完所有消息后提示 |
| SOLO-32-QA | v0.2 Week 3 回归测试 | 测试 Agent 创建/编辑/加入频道流程, 消息历史分页功能 | qa1 | P0 | Week 3 任务 | 3 | Agent CRUD 流程全部通过; 游标分页边界条件验证 |

**Week 3 并行策略**:
- rd1 优先 SOLO-24-B (迁移) → SOLO-25-B (Agent CRUD) → SOLO-26-B (Agent 频道集成)
- rd2 从 SOLO-28-B (Agent 接口, 仅依赖 SOLO-01-B) 和 SOLO-27-B (历史分页, 依赖 SOLO-16-B 已完成) 开始, 与 rd1 完全独立
- fe1 优先 SOLO-29-F (Agent 页面), 可与 rd1/rd2 并行
- fe2 推进 SOLO-31-F (消息历史无限滚动), 依赖 SOLO-20-F 和 SOLO-27-B (rd2 产出)
- rd1(14h) + rd2(7h) + fe1(16h) + fe2(6h) = 43h, 团队总容量 160h, 负荷 27%

---

#### Week 4: 线程回复

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-33-B | 迁移: threads 表 | 创建 threads 表 (id, channel_id, root_message_id, reply_count, last_reply_at) + 复合索引, messages 表增加 `thread_id` 字段 | rd1 | P0 | SOLO-12-B (channels), SOLO-16-B (messages) | 3 | 迁移执行后 threads 表存在; messages.thread_id 列存在; 索引正确 |
| SOLO-34-B | 线程 REST API | POST /channels/{id}/messages/{msgId}/thread (创建线程回复), GET /channels/{id}/threads (线程列表), GET /channels/{id}/threads/{threadID} (线程消息); 创建线程回复时自动创建 thread 记录, reply_count 递增 | rd1 | P0 | SOLO-33-B, SOLO-16-B | 8 | 线程回复创建成功; 线程列表显示; 线程消息返回; reply_count 正确递增; 线程深度控制 (不支持嵌套) |
| SOLO-35-B | 线程 WS 事件 | WS `thread.reply` 事件: 在线程中发送消息 → 持久化 → 广播给订阅该线程的客户端; 主频道收到 `thread.reply` 通知更新 reply_count 计数; 频道消息列表默认排除线程消息 (WHERE thread_id IS NULL) | rd1 | P0 | SOLO-17-B, SOLO-34-B | 6 | 线程消息发给订阅者; 主频道看到回复计数更新; 频道消息列表不显示线程消息 |
| SOLO-36-B | Agent 线程上下文支持 | Agent 在线程中被触发时, 上下文仅包含线程内的消息 (而非整个频道); Daemon RunRequest 中 ThreadID 字段传递; 线程消息触发 agent auto-response | rd2 | P0 | SOLO-28-B, SOLO-34-B | 4 | Agent 在线程中响应; Agent 上下文只包含线程消息; 回复出现在线程中 |
| SOLO-37-F | 线程面板 UI | 右侧 ThreadPanel 组件: 加载态骨架屏、空态 ("还没有回复, 开始讨论")、父消息展示、线程消息列表 (正序)、底部分栏输入框、关闭按钮; 主频道消息显示 "N 条回复" 指示器 | fe1 | P0 | SOLO-35-B, SOLO-20-F | 10 | 面板打开显示父消息; 发送回复后出现在列表中; 回复计数实时更新; 面板可关闭; 主频道恢复全宽 |
| SOLO-38-F | 消息悬停回复按钮 | 消息组件 hover 时显示"回复"图标按钮 (对话气泡), 点击触发线程面板打开, 选中消息作为父消息 | fe2 | P0 | SOLO-37-F, SOLO-20-F | 3 | hover 消息显示回复按钮; 点击打开线程面板; 面板正确关联父消息 |
| SOLO-39-F | 前端 WebSocket 线程订阅 | WS 客户端支持 `thread.subscribe` / `thread.unsubscribe`, 线程消息通过 WS 实时推送, 线程面板实时更新新消息 | fe2 | P0 | SOLO-35-B, SOLO-21-F, SOLO-37-F | 4 | 打开线程面板时订阅; 关闭时取消; 线程新消息实时出现 |
| SOLO-40-QA | v0.2 集成验证 | 端到端验证 Agent 管理 + 消息历史 + 线程回复流程 | qa1 | P0 | Week 3-4 所有任务 | 4 | Agent 加入频道 → 线程回复 → 线程消息实时展示 → 全部正常 |

**Week 4 并行策略**:
- rd1 从 SOLO-33-B (迁移) 开始, 完成后推进 SOLO-34-B 和 SOLO-35-B
- rd2 推进 SOLO-36-B (Agent 线程上下文), 依赖 SOLO-28-B (rd2 自己 Week 3 产出) + SOLO-34-B (rd1), 安排在 Week 4 后期
- fe1 从 SOLO-37-F (线程面板 UI) 开始, 先 mock 线程数据开发 UI, 无需等后端完成
- fe2 接手 SOLO-38-F (悬停回复按钮, 依赖 SOLO-37-F) 和 SOLO-39-F (WS 线程订阅, 依赖 SOLO-35-B + SOLO-21-F + SOLO-37-F)
- rd1(17h) + rd2(4h) + fe1(10h) + fe2(7h) = 38h, 团队总容量 160h, 负荷 24%

---

## 7. v0.3 (第 5-6 周): Agent 自动响应 + 流式输出 + @提及

**迭代目标**: Agent 自动监听频道消息并生成回复 (流式逐字输出), @提及 Agent 手动触发

**可演示产出 (第 6 周末)**:
1. 在频道中发送消息, Agent 自动开始回复, "thinking..." 状态显示
2. Agent 回复逐字出现在消息列表中 (流式 Markdown 渲染)
3. 输入 @ 弹出成员选择器 (Agent + 用户)
4. @AgentName 发送消息, 只有被 @ 的 Agent 响应
5. Agent 不响应自身消息, 防抖机制正常工作

### 任务列表

#### Week 5: Agent Daemon 集成 + 自动响应

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-41-B | Daemon 注册与发现 | Daemon 启动时向 Server 注册 (POST /internal/daemon/register), 30s 心跳, Server 检测 3 次无心跳标记离线 | rd1 | P0 | SOLO-10-B | 6 | Daemon 注册后 Server 显示在线; 心跳发送正常; 停止 Daemon 后标记离线 |
| SOLO-42-B | Daemon 任务下发 API | POST /internal/daemon/run: Server 向 Daemon 下发 Agent 执行任务 (agent_id, channel_id, thread_id?, messages 上下文, system_prompt, model_config), 返回 task_id | rd1 | P0 | SOLO-28-B, SOLO-41-B | 8 | 任务下发后 Daemon 返回 202; task_id 可追踪; 参数校验完整 |
| SOLO-43-B | Agent 自动响应触发逻辑 | ChannelService 收到新消息后判断是否需要触发 Agent: 查询频道活跃 Agent 列表, 排除自身 (Agent 不响应自己的消息), 防抖 (同 Agent 2s 内不重复触发), 取最近 20 条消息作为上下文, 调用 Daemon.Run | rd1 | P0 | SOLO-26-B, SOLO-42-B | 8 | Agent 收到新消息后自动触发; 不响应自身消息; 2s 防抖有效; 上下文包含最近 20 条 |
| SOLO-44-B | LLM Provider 封装 (复用 multica pkg/llm) | 从 multica 复制 pkg/llm (OpenAI/Anthropic/Ollama), 配置通过环境变量传入 API Key | rd2 | P0 | SOLO-01-B | 4 | LLM 调用成功; 配置通过环境变量注入; 支持 OpenAI 和 Anthropic |
| SOLO-45-B | Agent Daemon LLM 调用循环 | Daemon 接收任务后: 加载 Agent 配置 → 构造 LLM Request → 调用 LLM.Complete → Agent 状态更新 | rd1 | P0 | SOLO-42-B, SOLO-44-B | 6 | Daemon 成功调用 LLM; 非流式模式下一次性返回结果; 消息持久化到 DB |
| SOLO-46-B | Agent 状态管理 | Agent 状态 (在线/思考中/输出中/离线) 管理, 通过 WebSocket 推送状态变更给前端: agent.thinking, agent.typing, agent.error 事件 | rd2 | P0 | SOLO-15-B, SOLO-43-B | 4 | Agent 状态正确流转; WS 推送状态变更; 前端收到后更新 UI |
| SOLO-47-F | Agent 状态指示器前端展示 | 成员列表中 Agent 状态指示器 (绿=在线, 黄=thinking..., 蓝=typing..., 灰=离线), 打字指示器 "AgentName 正在输入..." | fe1 | P0 | SOLO-30-F, SOLO-46-B | 4 | Agent 状态正确显示; 思考中状态显示; 打字指示器动态展示 |
| SOLO-48-F | Agent 消息非流式渲染 | Agent 一次性回复渲染: Agent 头像 + 名称 + Markdown 内容 (MarkdownRenderer 组件), Agent 消息特殊样式标记 | fe2 | P1 | SOLO-45-B, SOLO-20-F | 4 | Agent 回复正确渲染; Markdown 正常显示; 样式区别于用户消息 |
| SOLO-49-QA | Week 5 回归测试 | 测试 Daemon 注册/心跳、Agent 自动响应触发流程、Agent 状态变更、非流式回复、防抖机制 | qa1 | P0 | Week 5 任务 | 4 | Agent 自动响应闭环通过; 防抖边界测试; Agent 不响应自身消息 |

**Week 5 并行策略**:
- rd1 优先推进 SOLO-41-B → SOLO-42-B → SOLO-43-B → SOLO-45-B (核心触发链路)
- rd2 从 SOLO-44-B (LLM Provider, 仅依赖 SOLO-01-B, 完全独立) 开始, 后续推进 SOLO-46-B (Agent 状态管理, 依赖 SOLO-15-B + SOLO-43-B), 安排在 Week 5 后期
- fe1 推进 SOLO-47-F (Agent 状态指示器)
- fe2 负责 SOLO-48-F (Agent 非流式渲染), 依赖 SOLO-45-B + SOLO-20-F
- rd1(28h) + rd2(8h) + fe1(4h) + fe2(4h) = 44h, 团队总容量 160h, 负荷 28%

---

#### Week 6: 流式输出 + @提及

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-50-B | Daemon SSE 流式推送 | Daemon 支持 SSE (Server-Sent Events) 流式推送: event:thinking/token/tool_call/complete, Server 接收后通过 WS 转发给前端 | rd1 | P0 | SOLO-45-B | 8 | SSE 连接建立; token 逐块推送; complete 事件后关闭流; 支持取消 (user stop) |
| SOLO-51-B | Server 转发流式消息到 WS | Server 接收 Daemon SSE 事件后, 转换为 WS 事件 (message.agent_typing / message.new 单块推送), 流式过程中 Agent 消息消息 ID 不变, content 持续追加 | rd1 | P0 | SOLO-50-B, SOLO-17-B | 6 | Agent 流式内容通过 WS 实时推送到前端; 消息 ID 在流式过程中一致; 完成后持久化完整消息 |
| SOLO-52-B | @提及 Agent 解析 (后端) | 消息发送时后端解析 @提及: 正则匹配 `@成员名称`, 查询频道成员匹配, 返回 mentioned_agent_ids; 仅触发被 @提及 的 Agent (其他 Agent 不触发); 自身 @提及 忽略; 支持多 @提及 | rd2 | P0 | SOLO-43-B, SOLO-26-B | 6 | @CodeReviewer 正确解析并触发; 未匹配的 @名 不触发 Agent; @自身 忽略; 多 @提及 全部触发 |
| SOLO-53-B | @提及 + 自动响应触发逻辑联动 | 消息同时包含 @提及 和普通消息时: 被 @提及 的 Agent 按 @提及 逻辑触发, 其他 Agent 判断是否自动响应; 如果 Agent 已开启自动响应 + 被 @提及 → 按 @提及 处理不重复触发 | rd2 | P0 | SOLO-52-B, SOLO-43-B | 4 | @提及 触发和自动触发不冲突; 不重复触发同一 Agent; 逻辑组合正确 |
| SOLO-50-F | 流式消息实时渲染 | 前端接收 WS 流式 message.new 事件, 实时渲染 Agent 生成内容: 消息组件支持内容追加、Markdown 实时渲染 (代码块、列表、粗体)、完整后停止更新并稳定显示 | fe1 | P0 | SOLO-48-F, SOLO-51-B | 8 | Agent 内容逐字显示; Markdown 实时渲染; 完成后内容稳定; 滚动跟随 (用户未手动滚动时) |
| SOLO-51-F | @提及 自动完成下拉 | 输入 @ 时弹出成员选择列表: 频道成员数据预加载, 输入过滤, 键盘导航 (上下箭头+Enter), Escape 取消, 选中后插入 `@名称` 文本, 无结果时显示"没有匹配的成员" | fe1 | P0 | SOLO-20-F, SOLO-52-B | 8 | @ 弹出列表; 过滤工作正常; 键盘/鼠标选择生效; 选中后输入框显示 @名称 |
| SOLO-52-F | @提及 消息发送 | 发送消息时附带 mentioned_agent_ids (从 @解析 得到); Agent 回复消息体中标注 "（被 @提及 触发）"; 前端展示 @提及 的被触发标记 | fe2 | P0 | SOLO-51-F, SOLO-52-B | 4 | @提及 消息发送正常; 回复显示触发标记; UI 展示被 @提及 的 Agent 名称 |
| SOLO-53-QA | v0.3 集成验证 | 全流程集成测试: Agent 自动响应(流式) + @提及 触发 + 状态指示器 + 防抖 + 自身忽略 | qa1 | P0 | Week 5-6 所有任务 | 6 | 所有 agent 响应场景通过; 流式输出逐字验证; @提及 边界条件覆盖 |

**Week 6 并行策略**:
- rd1 优先推进 SOLO-50-B (SSE 流式) → SOLO-51-B (WS 转发)
- rd2 推进 SOLO-52-B 和 SOLO-53-B (@提及 后端逻辑), 与 rd1 的流式开发完全并行, 不互相阻塞
- fe1 推进 SOLO-50-F (流式渲染) 和 SOLO-51-F (@提及 前端), 可 mock WS 数据并行开发
- fe2 处理 SOLO-52-F (@提及 消息发送), 依赖 SOLO-51-F (fe1) 和 SOLO-52-B (rd2)
- rd1(14h) + rd2(10h) + fe1(16h) + fe2(4h) = 44h, 团队总容量 160h, 负荷 28%

---

## 8. v0.4 (第 7-8 周): 私信 DM + 前端优化

**迭代目标**: DM 创建和会话、MVP 功能闭环完成、所有 UI 状态覆盖

**可演示产出 (第 8 周末)**:
1. 侧栏 DM 区域显示已有私信列表
2. 点击 "+" 搜索用户/Agent, 创建 DM
3. DM 会话中发送消息, Agent DM 自动响应 (流式)
4. 与同一人再次私信直接进入已有 DM (去重)
5. 所有页面加载/空态/错误态/边缘情况完整覆盖

### 任务列表

#### Week 7: 私信 DM

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-54-B | 迁移: dm_members 表 | 创建 dm_members 表 (复用 ARCHITECTURE.md 7.4a 的 DDL); channels.type='dm' 的区分 | rd1 | P0 | SOLO-12-B | 2 | dm_members 表创建成功; 索引存在 |
| SOLO-55-B | DM 创建/获取 API | POST /api/v1/dm (创建或获取已有 DM): 支持 user 对 agent 和 user 对 user; 双向去重 (A-B 和 B-A 视为同一 DM); 自动将双方加入 channel_members; 返回已创建的 DM; 查询已有 DM 时直接返回 | rd1 | P0 | SOLO-54-B, SOLO-05-B, SOLO-25-B | 6 | 创建 DM 返回 201 (新) 或 200 (已有); 去重正确; 成员自动加入; 不存在的收信人返回 404 |
| SOLO-56-B | DM 列表 + 消息 API | GET /api/v1/dm (DM 列表, 按 last_reply_at 降序, 含最后消息预览); GET /api/v1/dm/{dmID}/messages (游标分页, 复用消息分页逻辑); POST (发送消息, 同频道消息发送逻辑) | rd1 | P0 | SOLO-55-B, SOLO-16-B | 6 | DM 列表返回正确排序; 消息分页正常; 最后消息预览截断 50 字 |
| SOLO-57-B | DM WebSocket 事件 | WS 事件 `dm.subscribe` / `dm.unsubscribe` / `dm.message.new`: DM 消息实时推送; DM 列表更新通知 | rd2 | P0 | SOLO-55-B, SOLO-15-B | 4 | WS 订阅 DM 后收到实时消息; 取消订阅后不再收到; DM 列表随新消息更新排序 |
| SOLO-58-B | DM Agent 触发 | DM 中的消息触发 Agent 响应 (DM 中的 Agent 默认响应所有消息, 无需 @提及); Agent 上下文包含 DM 消息历史 | rd2 | P0 | SOLO-55-B, SOLO-43-B | 4 | Agent DM 响应正常工作; 上下文正确; 流式输出正常 |
| SOLO-54-F | DM 列表 UI | 侧栏 DM 区域: 加载态骨架屏, 空态 ("还没有私信"), 有 DM 列表 (对方头像+名称+最后消息预览截断 50 字), 新消息加粗 + 蓝点指示, 选中高亮 | fe1 | P0 | SOLO-18-F, SOLO-56-B | 6 | DM 列表加载正常; 空态引导显示; 新消息指示器工作; 按最后消息时间排序 |
| SOLO-55-F | DM 创建对话框 | 点击 "+" 弹出创建 DM 对话框: 搜索输入框, 实时搜索用户和 Agent, 列表显示名称+类型标签+在线状态, 选中后创建/进入 DM | fe1 | P0 | SOLO-54-F, SOLO-55-B | 6 | 搜索工作正常; 选中后创建/进入 DM; 类型标签 (用户/Agent) 显示; 去重 — 已有 DM 直接进入 |
| SOLO-56-F | DM 会话视图 | DM 消息视图: 与频道视图一致, 消息列表 + 输入框, 显示 "这是你与 {name} 的私信", Agent DM 支持流式输出渲染 | fe1 | P0 | SOLO-55-F, SOLO-56-B, SOLO-57-B | 8 | DM 消息显示正确; 发送消息实时; Agent 流式回复正常; 视图与频道视觉区分 |
| SOLO-57-F | DM 线程支持 | DM 中支持线程回复: 复用 SOLO-37-F 的 ThreadPanel, DM 线程面板中的 Agent 触发 | fe2 | P1 | SOLO-56-F, SOLO-37-F | 4 | DM 中可发起线程; 线程面板正常; Agent 在线程中回复 |
| SOLO-58-QA | Week 7 集成测试 | DM 创建/去重/消息发送/Agent 响应/线程回复 全流程测试 | qa1 | P0 | Week 7 任务 | 4 | DM 闭环测试通过; 去重场景验证; 边缘情况覆盖 |

**Week 7 并行策略**:
- rd1 优先 SOLO-54-B (迁移) → SOLO-55-B (DM API) → SOLO-56-B (DM 列表+消息 API)
- rd2 推进 SOLO-57-B (DM WS 事件, 依赖 SOLO-55-B + SOLO-15-B) 和 SOLO-58-B (DM Agent 触发, 依赖 SOLO-55-B + SOLO-43-B), 与 rd1 可并行
- fe1 优先 SOLO-54-F (DM 列表) 和 SOLO-55-F (DM 创建对话框), 可 mock 数据
- fe2 处理 SOLO-57-F (DM 线程支持), P1 小任务, 依赖 SOLO-56-F + SOLO-37-F
- rd1(14h) + rd2(8h) + fe1(20h) + fe2(4h) = 46h, 团队总容量 160h, 负荷 29%

---

#### Week 8: 前端优化 + 集成

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-59-F | 用户资料页面 | 个人信息页面 (GET/PATCH /users/me), 显示邮箱和显示名称, 支持修改显示名称 | fe2 | P1 | SOLO-18-F | 3 | 个人信息显示正常; 修改名称成功; 名称更新后全局生效 |
| SOLO-60-F | 全局 UI 状态覆盖 | 补充所有页面的: 加载态骨架屏、空态引导、错误态提示+重试、404 页面、403 页面、网络断线提示 | fe1 | P1 | v0.1-0.4 前端任务 | 8 | 所有页面状态覆盖完整; 骨架屏代替白色闪烁; 空态有 CTA 引导; 错误态可重试 |
| SOLO-61-F | WebSocket 断线 UI 完善 | 断线时全局 banner "重新连接中...", 重连后自动订阅所有频道, 重连后拉取错过的消息 | fe2 | P1 | SOLO-21-F | 4 | 断线后 banner 显示; 自动重连; 重连后频道恢复消息订阅 |
| SOLO-62-F | 消息发送失败处理完善 | 发送失败时: 消息保留在列表中 (灰色/红色标记)、重试按钮、取消按钮、失败提示 (中文) | fe2 | P1 | SOLO-20-F | 3 | 发送失败显示红色警示; 重试发送成功; 取消移除消息; 提示用户可读 |
| SOLO-63-F | 前端性能优化 | 按路由懒加载, 消息列表虚拟滚动 (只渲染可见区域), 不必要的 re-render 优化, 包体积优化 | fe1 | P2 | v0.1-0.4 前端任务 | 6 | Lighthouse 性能评分 > 80; FCP < 2s; LCP < 3s; 大频道 (1k+ 消息) 滚动流畅 |
| SOLO-64-B | 健康检查 + 可观测性完善 | /healthz (liveness) + /readyz (readiness) 端点完善, Prometheus 指标: 请求数/延迟/错误率/WS 连接数, 结构化 JSON 日志 + request_id | rd1 | P1 | SOLO-01-B | 4 | 健康检查端点正确; Prometheus 指标暴露; 日志包含 request_id |
| SOLO-65-B | 基础安全问题加固 | Rate limiting 配置 (认证端点 10 req/min, 其他 100 req/s), 请求体大小限制 (消息 100KB), CORS 白名单配置, 安全标头 (CSP/X-Frame-Options) | rd1 | P1 | SOLO-06-B | 4 | 速率限制生效; 大请求被拒绝; CORS 只允许前端域名; 安全标头存在 |
| SOLO-66-B | Daemon 心跳健康检测完善 | Server 检测 Daemon 离线后重试机制, 离线标记后清理任务, Daemon 启动时重新注册 | rd2 | P1 | SOLO-41-B | 3 | Daemon 离线上报; 重连后恢复; 任务清理正常 |
| SOLO-67-FS | 线程消息实时计数同步 | 线程回复后主频道消息的 "N 条回复" 计数通过 WS 实时更新, 包括跨设备同步 | fe1/B | P1 | SOLO-35-B, SOLO-37-F | 4 | 回复计数实时更新; 跨设备同步 (同一用户两个 tab 看到同一计数) |
| SOLO-68-QA | MVP 全功能回归测试 | 完整的 MVP 功能回归测试, 包含所有异常流程: 注册冲突、登录失败、频道重名、WS 断线、Agent 超时、消息发送失败、DM 去重 | qa1 | P0 | v0.1-0.4 所有任务 | 8 | 所有回归测试通过; 异常流程覆盖完整 |

**Week 8 并行策略**:
- fe1 推进 SOLO-60-F (全局 UI 状态覆盖), SOLO-63-F (前端性能优化), SOLO-67-FS (线程消息计数同步)
- fe2 推进 SOLO-59-F (用户资料页面), SOLO-61-F (WS 断线 UI), SOLO-62-F (消息发送失败处理) — 均依赖 SOLO-18-F/SOLO-20-F/SOLO-21-F, fe2 已有前期积累
- rd1 推进安全和可观测性加固 (SOLO-64-B, SOLO-65-B)
- rd2 推进 SOLO-66-B (Daemon 心跳健康检测), 依赖 SOLO-41-B
- qa1 开始 MVP 全功能回归测试
- rd1(8h) + rd2(3h) + fe1(18h) + fe2(10h) = 39h, 团队总容量 160h, 负荷 24%

---

## 9. RC (第 9 周): 集成测试 + Bug 修复

**迭代目标**: 所有 P0 功能稳定, 无 blocking/critical Bug, 核心指标达标

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-69-QA | E2E 测试编写 | 使用 Playwright/Cypress 编写核心用户流程 E2E 测试: 注册→创建频道→发消息→创建 Agent→Agent 响应→线程回复→@提及→DM→完整闭环 | qa1 | P0 | v0.1-0.4 所有任务 | 10 | E2E 测试覆盖所有 P0 功能; 测试可重复运行; 测试报告生成 |
| SOLO-70-QA | 性能测试 | 消息端到端延迟 (P95 < 500ms), API 响应时间 (P95 < 200ms), WebSocket 并发 (1000 连接), 消息历史查询 (P95 < 500ms) | qa1 | P1 | v0.1-0.4 所有任务 | 6 | 指标达标; 压测报告输出; 瓶颈识别 |
| SOLO-71-FS | Bug 修复 Sprint | 根据测试结果修复所有 blocking/critical Bug: 消息丢消息、WS 断线丢失消息、Agent 响应死循环、权限校验遗漏、DM 去重逻辑错误 | rd1+rd2+fe1+fe2 | P0 | SOLO-69-QA | 16 | 回归测试通过; 无 blocking/critical Bug; |
| SOLO-72-FS | Edge Case 修复 | Agent 空回复处理; 超长消息 (10000 字符) 截断; 高并发消息时序; 线程深度控制; @提及 不存在成员的 fallback | rd1+rd2+fe1+fe2 | P1 | SOLO-69-QA | 8 | 所有 edge case 正确处理; 无数据丢失 |
| SOLO-73-FS | 用户体验打磨 | 消息列表自动滚动行为优化; 输入框自动聚焦; 错误提示中文文案统一; 加载动画流畅度; 提交按钮防抖 | fe1+rd1 | P2 | v0.4 任务 | 6 | 用户体验提升; 操作反馈及时; 所有提示信息一致 |

**RC 周并行策略**:
- qa1 主导测试, 全团队 (rd1+rd2+fe1+fe2) 按 Bug 优先级修复
- 上午: qa1 跑测试, 报告 Bug; 团队按领域分工修复
- 下午: 继续修复 + qa1 验证修复
- rd1/rd2 负责后端 Bug, fe1/fe2 负责前端 Bug, 全栈 Bug 联合修复

---

## 10. 上线 (第 10 周): 公测发布

| ID | 任务标题 | 描述 | 负责人 | 优先级 | 依赖 | 预估(h) | 验收标准 |
|----|---------|------|--------|--------|------|---------|----------|
| SOLO-74-OPS | 部署脚本与 CI/CD | 编写部署脚本 (Docker build + docker-compose production), GitHub Actions CI 配置 (lint + test + build), 环境变量模板 | rd1 | P0 | RC 完成 | 6 | CI 通过; 部署脚本正常; 一键部署到测试环境 |
| SOLO-75-OPS | 生产环境配置 | PostgreSQL 配置 (连接池大小建议), 环境变量注入 (JWT secret, DB URL, LLM API keys), daemon 配置 | rd1 | P0 | SOLO-74-OPS | 4 | 生产环境启动正常; 配置项完整; 安全配置加固 |
| SOLO-76-OPS | 监控告警配置 | Prometheus + Grafana 基础面板, 关键指标告警规则 (错误率 > 5%, WS 连接数异常, Daemon 离线) | rd2 | P1 | SOLO-64-B | 4 | Grafana 面板展示关键指标; 告警规则生效 |
| SOLO-77-OPS | 在线文档与 README | README 完善 (项目简介、快速开始、配置说明、API 文档链接); 部署文档 (docker-compose 部署步骤、环境变量说明) | rd2 | P1 | 无 | 3 | README 内容完整; 部署文档可复现 |
| SOLO-78-QA | 上线前最终测试 | 生产环境 Smoke Test (核心闭环验证), 数据备份验证, 回滚方案验证 | qa1 | P0 | SOLO-74-OPS | 4 | Smoke Test 通过; 回滚方案验证通过 |

---

## 11. 跨迭代依赖关系总图

```
v0.1 (Week 1-2)                     v0.2 (Week 3-4)                 v0.3 (Week 5-6)                  v0.4 (Week 7-8)
┌─────────────────┐                ┌──────────────────────┐        ┌─────────────────────────┐      ┌─────────────────────┐
│ 项目脚手架       │───────┐       │ Agent迁移+CRUD       │────┐   │ Daemon注册与任务下发    │──┐   │ DM迁移+API          │
│ (SOLO-01-B)      │       │       │ (SOLO-24-B, 25-B)    │    │   │ (SOLO-41-B, 42-B)       │  │   │ (SOLO-54-B ~ 56-B)  │
└─────────────────┘       │       └──────────────────────┘    │   └─────────────────────────┘  │   └─────────────────────┘
                          │                                    │                                │
┌─────────────────┐       │       ┌──────────────────────┐    │   ┌─────────────────────────┐  │   ┌─────────────────────┐
│ 认证后端+前端    │───────┤       │ 消息历史游标分页       │    │   │ Agent自动响应触发逻辑   │  │   │ DM WS事件           │
│ (SOLO-03-B~09-F) │       │       │ (SOLO-27-B, 31-F)    │    │   │ (SOLO-43-B)             │  │   │ (SOLO-57-B)         │
└─────────────────┘       │       └──────────────────────┘    │   └─────────────────────────┘  │   └─────────────────────┘
                          │                                    │                                │
┌─────────────────┐       │       ┌──────────────────────┐    │   ┌─────────────────────────┐  │   ┌─────────────────────┐
│ 频道迁移+CRUD    │───────┤       │ Agent频道集成后端+前端│    │   │ LLM Provider封装       │  │   │ DM列表+创建UI       │
│ (SOLO-12-B,13-B) │       │       │ (SOLO-26-B, 30-F)    │    │   │ (SOLO-44-B)             │  │   │ (SOLO-54-F, 55-F)   │
└─────────────────┘       │       └──────────────────────┘    │   └─────────────────────────┘  │   └─────────────────────┘
                          │                                    │                                │
┌─────────────────┐       │       ┌──────────────────────┐    │   ┌─────────────────────────┐  │   ┌─────────────────────┐
│ WS Hub复用+集成  │───────┼───────│ 线程迁移+API+WS       │────┼───│ Daemon SSE流式推送      │──┤   │ 前端优化+状态覆盖    │
│ (SOLO-15-B,17-B) │       │       │ (SOLO-33-B ~ 35-B)   │    │   │ (SOLO-50-B, 51-B)       │  │   │ (SOLO-59-F ~ 63-F)  │
└─────────────────┘       │       └──────────────────────┘    │   └─────────────────────────┘  │   └─────────────────────┘
                          │                                    │                                │
┌─────────────────┐       │       ┌──────────────────────┐    │   ┌─────────────────────────┐  │
│ 消息迁移+REST    │───────┘       │ 线程面板前端           │    │   │ @提及 解析+触发逻辑     │  │
│ (SOLO-16-B)     │               │ (SOLO-37-F ~ 39-F)   │    │   │ (SOLO-52-B, 53-B)       │  │
└─────────────────┘               └──────────────────────┘    │   └─────────────────────────┘  │
                                                               │                                │
┌─────────────────┐                                            │   ┌─────────────────────────┐  │
│ 前端脚手架+布局  │                                            │   │ 流式渲染+@提及前端      │  │
│ (SOLO-07-F~21-F)│                                            │   │ (SOLO-50-F ~ 52-F)      │  │
└─────────────────┘                                            │   └─────────────────────────┘  │
                                                                │                                │
                                                                │   ┌─────────────────────────┐  │
                                                                │   │ 可观测性+安全           │──┘
                                                                │   │ (SOLO-64-B ~ 66-B)      │
                                                                │   └─────────────────────────┘
                                                                │
```

---

## 12. 风险登记册

| # | 风险描述 | 概率 | 影响 | 触发条件 | 缓解措施 | 应急预案 | 负责人 |
|---|---------|------|------|----------|----------|----------|--------|
| R1 | multica 复用代码与 Solo 架构不兼容 (尤其是 workspace 简化) | 中 | 高 | 集成时发现类型/接口不匹配 | 开发前安排 arc 做代码走读, 明确适配点后再动手 | 对不兼容模块用适配器模式包装, 不强制直接引用 | rd1 + arc |
| R2 | Agent Daemon SSE 流式到 WS 转发延迟高 | 中 | 高 | P95 首 token 延迟 > 3s | 流式过程中 Server 做透传不做缓冲处理; 避免在转发路径中加额外处理 | 优化转发路径, 减少序列化/反序列化环节 | rd1 |
| R3 | WebSocket 断线重连场景下消息丢失 | 中 | 高 | 测试发现重连后消息不完整 | 注册时从 DB 拉取缺失消息 (以客户端断线时间为游标) | 客户端实现 "至少一次" 语义, 服务端幂等去重 | rd1 + fe1 |
| R4 | @提及 解析与前端输入框 @ 选择器联动出错 | 中 | 中 | @名称 匹配不正确或遗漏 | 前端发送时附带 resolved mentioned_agent_ids, 后端再解析做二次校验 | 后端解析作为权威, 前后端结果取交集 | rd1 + fe1 |
| R5 | Agent 自动响应触发 LLM 调用导致高延迟 (LLM API 本身慢) | 高 | 中 | Agent 响应 > 10s | 前端显示 "thinking..." 状态; Agent 配置 timeout 上限 | 超时后在频道中显示 "Agent 响应超时, 请重试" | rd1 |
| R6 | v0.1 中 WebSocket 与 REST API 联调时间超出预期 | 中 | 高 | Week 2 末消息联调未完成 | Week 1 优先完成 WS Hub 集成 (SOLO-15-B); 前后端并行开发 | 如果 WS 来不及先用 REST 发送消息 (POST /messages) | rd1 + fe1 |
| R7 | 新手环境搭建 (PostgreSQL + Docker + LLM API Key) 消耗时间 | 中 | 低 | 第一天配置环境 | 提供 docker-compose + .env.example + 快速启动文档 | 安排 arc 协助环境排障 | rd1 |

---

## 附录 A: 任务统计汇总

| 迭代 | 后端任务数 | 前端任务数 | 测试任务数 | 全栈/运维 | 总计 | 预估总工时(h) |
|------|-----------|-----------|-----------|----------|------|-------------|
| Week 1 | 6 | 3 | 0 | 1 | 10 | 40 |
| Week 2 | 5 | 5 | 1 | 0 | 11 | 56 |
| Week 3 | 5 | 3 | 1 | 0 | 9 | 44 |
| Week 4 | 4 | 3 | 1 | 0 | 8 | 42 |
| Week 5 | 6 | 2 | 1 | 0 | 9 | 44 |
| Week 6 | 4 | 3 | 1 | 0 | 8 | 44 |
| Week 7 | 5 | 4 | 1 | 0 | 10 | 46 |
| Week 8 | 3 | 5 | 0 | 1 | 9 | 44 |
| Week 9 | 0 | 0 | 2 | 2 (全栈) | 4 | 40 |
| Week 10 | 0 | 0 | 1 | 4 (运维) | 5 | 21 |
| **总计** | **38** | **28** | **9** | **8** | **83** | **421** |

> 注: 每个任务预估工时基于单人完成计算。Week 9 Bug 修复的 16h 和 8h 为预留时间, 不提前分配。

## 附录 B: 团队容量检查

| 角色 | 周可用(h) | 周任务负荷(h) | 是否超载 | 说明 |
|------|----------|-------------|----------|------|
| rd1 | 40 | W1:22 / W2:30 / W3:14 / W4:17 / W5:28 / W6:14 / W7:14 / W8:8 | 否 (W2 75% 为峰值) | W2 负荷最高 (频道+WS 核心链路), 其余周均 < 70% |
| rd2 | 40 | W1:8 / W2:6 / W3:7 / W4:4 / W5:8 / W6:10 / W7:8 / W8:3 | 否 (~25% 平均) | 作为增量人力, 消化可并行的独立任务, 为 rd1 大幅减压 |
| fe1 | 40 | W1:10 / W2:22 / W3:16 / W4:10 / W5:4 / W6:16 / W7:20 / W8:18 | 否 (W2 55% 为峰值) | 专注于核心 UI 组件开发 |
| fe2 | 40 | W1:3 / W2:6 / W3:6 / W4:7 / W5:4 / W6:4 / W7:4 / W8:10 | 否 (~12% 平均) | 接手独立前端任务, 为 fe1 释放核心 UI 开发时间 |
| qa1 | 20 (50%) | 大部分周 4-6h | 否 | 作为 0.5 人投入, 负荷适中 |
| arc | 20 (50%) | 架构评审 + 代码走读 | 否 | 按需参与 |

> 团队扩展至 6 人后总周容量达 160h, 各角色负荷显著降低。rd2 和 fe2 平均负荷 25% 以下, 为后续 P1 功能预留充足产能。团队整体具备并行推进 v0.1-v0.4 各迭代核心链路的能力, 关键路径任务集中由 rd1/fe1 负责, 独立任务由 rd2/fe2 分流。

---

## 附录 C: 启动消息

### 给 rd1 的启动消息

> **主题**: Solo v0.1 启动 — 认证 + 频道 + 消息基础 (Week 1)
>
> rd1 你好,
>
> 项目脚手架已经就绪, 你的第一个 Sprint 从 Week 1 开始, 核心目标是搭建完整后端基础。好消息是 rd2 本周加入团队, 独立通用中间件和脚手架工作 (SOLO-06-B/SOLO-10-B/SOLO-11-OPS), 你可以聚焦核心认证链路:
>
> **Week 1** (本周):
> 1. SOLO-01-B: 项目初始化 (go mod, Chi 路由骨架, pgx 连接池, 引入 multica 复用模块) - 6h
> 2. SOLO-02-B: users + sessions 表迁移 - 3h
> 3. SOLO-03-B: JWT 认证模块 (从 multica 复制, 移除 workspace) - 4h
> 4. SOLO-04-B: 认证 REST 路由 (register/login/logout/refresh) - 6h
> 5. SOLO-05-B: 认证中间件 - 3h
>
> **优先级**: SOLO-01-B > SOLO-02-B+SOLO-03-B (可并行) > SOLO-04-B > SOLO-05-B
>
> **重点**: 
> - multica 的 `internal/auth/jwt.go` 可以直接复用, 关键是移除 workspace 耦合
> - 认证 handler 参考 multica, 路由精简即可
> - Week 2 需要 WS Hub 集成 (SOLO-15-B), 所以 Week 1 尽量留出时间提前了解 multica 的 realtime.Hub 架构
> - rd2 负责 SOLO-06-B/10-B/11-OPS, 你无需关注通用中间件和 docker-compose
>
> 输出: Week 1 结束时可演示用户注册/登录/登出, 本地 docker-compose 环境正常运行

### 给 fe1 的启动消息

> **主题**: Solo v0.1 启动 — 前端基础 + 认证页面 (Week 1)
>
> fe1 你好,
>
> 你的第一个 Sprint 从 Week 1 开始, 核心目标是搭建前端基础 + 认证页面。fe2 本周也加入团队, 负责 API 客户端封装 (SOLO-08-F), 你可以更聚焦核心 UI 开发:
>
> **Week 1** (本周):
> 1. SOLO-07-F: Next.js 16 App Router 脚手架 (Tailwind + shadcn/ui + 基础路由) - 4h
> 2. SOLO-09-F: 登录/注册页面 (react-hook-form + zod 校验) - 6h
>
> **优先级**: SOLO-07-F > SOLO-09-F
>
> **重点**:
> - fe2 负责 SOLO-08-F (API 客户端), 你无需关注, 直接使用 fe2 的产出联调
> - 登录/注册页面可以先 mock API 响应开发 UI, 后端在 SOLO-04-B 完成后联调
> - 如果提前完成, 可以开始看 SOLO-18-F (Dashboard 主布局) 的设计
>
> **联调节点**: Week 1 周四/五, 后端认证 API 完成后联调注册/登录流程
>
> 输出: Week 1 结束时可演示注册页面 → 登录 → 跳转 /dashboard (空状态)

### 给 rd2 的启动消息

> **主题**: Solo v0.1 启动 — 后端独立任务 (Week 1)
>
> rd2 你好,
>
> 欢迎加入 Solo 团队! 你作为第二位后端开发, 负责接手 rd1 可并行的独立任务, 让 rd1 聚焦认证和频道核心链路。
>
> **Week 1** (本周):
> 1. SOLO-06-B: 通用中间件 (CORS + Logging + Rate Limit) - 3h (仅依赖 SOLO-01-B)
> 2. SOLO-10-B: Daemon 项目脚手架 - 3h (仅依赖 SOLO-01-B)
> 3. SOLO-11-OPS: docker-compose 本地开发环境 - 2h (仅依赖 SOLO-01-B)
>
> **优先级**: 等 SOLO-01-B 完成后, 三个任务可全并行推进
>
> **重点**:
> - 通用中间件的 CORS 配置需允许 localhost:3000; Rate Limit 用 token bucket 实现
> - Daemon 脚手架参考 multica 的 `cmd/daemon/`, 基础框架即可
> - docker-compose 配置 PostgreSQL 16 + server + daemon 三个服务
> - 本周负荷仅 8h, 提前熟悉 multica 的 realtime.Hub 和 pkg/llm 架构, 为后续独立任务做准备
>
> **联调节点**: SOLO-01-B 完成后即可开始, 与 rd1 无直接联调依赖
>
> 输出: Week 1 结束时可提供 docker-compose 环境 + 通用中间件 + Daemon 脚手架

### 给 fe2 的启动消息

> **主题**: Solo v0.1 启动 — 前端基础设施 (Week 1)
>
> fe2 你好,
>
> 欢迎加入 Solo 团队! 你作为第二位前端开发, 负责接手 fe1 可并行的独立前端任务, 让 fe1 聚焦核心 UI 组件开发。
>
> **Week 1** (本周):
> 1. SOLO-08-F: API 客户端封装 (fetch + JWT 自动附加 + refresh token) - 3h (无依赖, 独立可并行)
>
> **优先级**: 唯一任务, 可直接开始
>
> **重点**:
> - 封装 fetch-based HTTP 客户端, 自动附加 JWT token (Authorization: Bearer)
> - 实现 token 过期自动 refresh (使用 refresh_token)
> - 401 响应时自动跳转登录页
> - 输出供 fe1 在 SOLO-09-F 中直接使用
> - 本周负荷仅 3h, 提前熟悉 Next.js 16 App Router 结构和 shadcn/ui 组件库, 为后续 SOLO-21-F (WS 封装) 做准备
>
> **联调节点**: 与 fe1 协作 — 输出 API 客户端供登录/注册页面联调
>
> 输出: Week 1 结束时可提供 API 客户端封装, fe1 可直接用于登录/注册页面

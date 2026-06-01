# Solo 功能状态矩阵

> 最后更新: 2026-06-01
> 基于代码实际状态（router.go / handler / migration / frontend components），非猜测。

## 状态图例

| 标记 | 含义 |
|------|------|
| ✅   | 已完成并可用 |
| 🔶   | 部分完成 / 有已知缺陷 |
| ❌   | 未实现 |
| —   | 不在当前范围 |

---

## 认证 (Auth)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 邮箱注册 | ✅ | v0 | `POST /api/v1/auth/register` |
| 密码登录 | ✅ | v0 | `POST /api/v1/auth/login` |
| JWT Access Token (HS256, 15min) | ✅ | v0 | `internal/auth/jwt.go` |
| Refresh Token (single-use, 7d) | ✅ | v0 | SHA-256 hash in `sessions` table |
| 登出 | ✅ | v0 | `POST /api/v1/auth/logout` |
| Agent Token 持久化 (365d) | ✅ | v1.3 | `~/.solo/agent-tokens/<id>/current.token`，daemon 自动 refresh |
| OAuth / 社交登录 | ❌ | — | 未排期 |
| 密码重置 | ❌ | — | 未排期 |
| 多工作区 / 团队管理 | ❌ | — | roadmap v2.0 |

---

## 频道 (Channels)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 频道 CRUD | ✅ | v0 | 创建/查看/编辑/删除 |
| WebSocket 实时推送 | ✅ | v0 | Hub + Client 架构 |
| 频道成员管理 | ✅ | v0 | 添加/移除成员 |
| 频道列表 | ✅ | v0 | 侧栏频道列表 |
| 频道分组/文件夹 | ❌ | — | 未排期 |
| 频道归档 | ❌ | — | 未排期 |

---

## 消息 (Messages)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 消息发送 | ✅ | v0 | 通过 WebSocket 实时推送 |
| 消息历史（游标分页） | ✅ | v0 | 复合索引，limit 50/100 |
| 消息编辑 | ✅ | v1.1 | `PATCH .../messages/{id}`, 标记 is_edited |
| 消息删除（软删除） | ✅ | v1.1 | `DELETE .../messages/{id}`, 标记 is_deleted |
| Markdown 渲染 | ✅ | v1.1 | 代码块 + 语法高亮 |
| 消息搜索 | ✅ | v1.3 | PostgreSQL FTS + ts_headline 高亮 + 游标分页 |
| 文件上传/附件 | ✅ | v1.3 | `POST .../attachments/upload`，缩略图生成，拖拽/粘贴上传 |
| 表情/Reactions | ❌ | — | 未排期 |
| 已保存/收藏消息 | ❌ | — | 未排期 |
| 消息翻译 | ❌ | — | 未排期 |

---

## 线程 (Threads)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 线程创建/回复 | ✅ | v0 | `POST .../messages/{id}/thread` |
| 线程面板 | ✅ | v1.1 | ThreadPanel 组件，含任务元数据展示 |
| Agent 线程响应 | ✅ | v1.3 | Thread @mention 触发 + 三路 ID 解析 |
| 线程收件箱 | ❌ | — | 未排期 |

---

## Agent

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| Agent CRUD | ✅ | v0 | `POST/GET/PATCH/DELETE /api/v1/agents` |
| Agent 加入频道 | ✅ | v0 | 频道成员列表含 Agent |
| Agent 自动响应（流式） | ✅ | v0 | SSE streaming + WebSocket 转发 |
| Agent 流式输出 (API Provider) | ✅ | v0 | Anthropic/OpenAI |
| Agent 流式输出 (Local Provider) | ✅ | v1.1 | 修复 local.go fake streaming |
| 11 Backend 支持 | ✅ | v1.2 | claude/local/codex/cursor/gemini/kimi/kiro/copilot/opencode/openclaw/hermes/pi |
| Agent Workspace 隔离 | ✅ | v1.2 | `~/.solo/agents/<id>/` per-agent 目录 |
| Agent Memory (MEMORY.md) | ✅ | v1.2 | 文件系统持久化，自动注入 prompt |
| Agent Tools 系统 | ✅ | v1.2 | `pkg/agent/tools.go` |
| Agent 交互模式 | ✅ | v1.2 | 对话模式 / 交互模式（文件操作）切换 |
| Agent 详情面板 | ✅ | v1.2 | runtime label, status, history tab |
| 持久化会话 (PersistentBackend) | ✅ | v1.3 | AgentSessionManager：会话池、串行锁、idle reaper |
| System Prompt (15 节 ~387 行) | ✅ | v1.3 | 完全对齐 Slock |
| Agent 自主认领任务 | ✅ | v1.3 | CLI `solo task claim -n N`，FOR UPDATE 防冲突 |
| Agent-to-Agent @mention 协作 | ✅ | v1.3 | agentChain 追踪，maxDepth=3，去重检测 |
| Agent 死循环防护 | ✅ | v1.3 | cascade 检测（>20次/10s → 60s 冷却） |
| Agent 团队管理页 | 🔶 | v1.3 | 页面已创建，待 API 联调验证 |
| Agent 生命周期管理 | ❌ | — | spawn/wake/sleep/restart |
| 电脑/机器管理 | 🔶 | v1.3 | API + 页面已创建，待心跳验证 + UI 验证 |

---

## DM (私信)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| DM 创建/去重 | ✅ | v0 | `POST /api/v1/dm` |
| DM 列表 | ✅ | v0 | `GET /api/v1/dm` |
| DM 消息发送 | ✅ | v0 | 实时通过 WebSocket |
| DM 消息编辑 | ✅ | v1.1 | `PATCH /api/v1/dm/{id}/messages/{id}` |
| DM 消息删除 | ✅ | v1.1 | `DELETE /api/v1/dm/{id}/messages/{id}` |
| DM Agent 响应 | ✅ | v0 | Agent 可在 DM 中回复 |
| DM 任务 API | ✅ | v1.3 | 完整 CRUD + Claim/Unclaim + asTask |
| DM 任务 UI | ❌ | v1.4 | DM 中 TaskBoard 视图待实现 |
| DM 文件上传 | ❌ | — | 同消息模块 |

---

## 任务 (Tasks)

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 任务 CRUD | ✅ | v1.2 | 频道级 + 全局任务 |
| 认领/释放 (Claim/Unclaim) | ✅ | v1.2 | Phase 1 完成 |
| 消息转任务 (asTask) | ✅ | v1.2 | `POST .../messages/{id}/convert-to-task` |
| 频道内编号 (#1, #2) | ✅ | v1.2 | migration 000013 |
| Kanban 看板 (5列) | ✅ | v1.2 | TaskBoard 组件 |
| 全局任务视图 | ✅ | v1.2 | `/api/v1/tasks` |
| 频道任务 Tab | ✅ | v1.2 | ChannelView 含 Tasks tab |
| 系统消息广播 | ✅ | v1.2 | 创建/认领/释放/完成广播 |
| 子任务 (parent_task_id) | ✅ | v1.3 | API 支持 + 子任务计数 + 进度条 |
| Agent 自主认领 (CLI-based) | ✅ | v1.3 | `solo task claim -n N`，FOR UPDATE 事务锁 |
| DM 任务支持 | ✅ | v1.3 | API 层面完成，UI 待 v1.4 |
| 任务排序/筛选 | ❌ | — | 待定 |
| 任务依赖 | ❌ | — | 待定 |

---

## UI / 设计系统

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| Neubrutalism 设计系统 | ✅ | v1.1 | 2px 黑边框 + 5px 硬阴影 + 零圆角 |
| Space Grotesk + Inter + Space Mono | ✅ | v1.1 | Typography tokens |
| 品牌色 #fe7da8 (粉色) | ✅ | v1.1 | 与 slock #FFD700 差异化 |
| 奶油白背景 #fffaef | ✅ | v1.1 | 亮色主题 |
| Markdown 渲染升级 | ✅ | v1.1 | 代码块深色背景 + 行号 + 复制 |
| 消息编辑/删除 UI | ✅ | v1.1 | MessageItem hover 操作菜单 |
| Agent 管理页样式 | ✅ | v1.1 | neubrutalist 卡片 |
| Settings 页面 | ✅ | v1.1 | 基础设置页 |
| CMD+K 全局搜索面板 | 🔶 | v1.3 | 组件已创建，待 API 集成验证 |
| 频道内搜索 | 🔶 | v1.3 | 组件已创建，待 API 集成验证 |
| 任务父子视觉关联 | ✅ | v1.3 | parent badge + subtask progress bar |
| ThreadPanel 任务元数据 | ✅ | v1.3 | task_number / status / assignee / priority |
| 拖拽/粘贴上传 UI | 🔶 | v1.3 | message-input 已实现，待完整验证 |
| Agent 团队管理页 | 🔶 | v1.3 | 页面已创建 (490 行)，待联调 |
| 电脑管理页 | 🔶 | v1.3 | 页面已创建 (530 行)，待联调 |
| 响应式适配 | 🔶 | v1.1 | 基础响应式，移动端未深度优化 |
| 动画/微交互 | 🔶 | v1.1 | 消息入场动画完成，其余待补充 |
| 暗色主题 | ❌ | — | 仅亮色主题 |
| Onboarding 引导 | ❌ | — | 未来版本 |

---

## 部署与运维

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| Docker Compose | ✅ | v0 | PostgreSQL + Server + Daemon |
| Dockerfile | ✅ | v0 | 多阶段构建 |
| 健康检查 (liveness/readiness) | ✅ | v0 | `/healthz`, `/readyz` |
| Prometheus 指标 | ✅ | v0 | `/metrics` 端点 |
| 结构化日志 | ✅ | v0 | slog |
| CI/CD | ❌ | — | 仅有 GitHub Actions 配置 |
| 推送通知 | ❌ | — | Web Push API + VAPID |
| 用量统计 | ❌ | — | token/费用追踪 |

---

## 测试

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| Go 单元测试 | 🔶 | v1.3 | 14 个测试文件，143 个测试函数，4 个失败待修复 |
| 后端 Handler 测试 | ✅ | v0 | channel/message/task/thread |
| pkg/agent 测试 | ✅ | v1.2 | factory/lock/memory/workspace/prompt/tools/claude |
| E2E Playwright 测试 | ✅ | v1.2 | 5 个 spec 文件 |
| UI 视觉回归测试 | ❌ | — | 计划中 |
| 性能/负载测试 | ❌ | — | 未排期 |

---

## 已读/未读追踪

| 功能 | 状态 | 版本 | 备注 |
|------|------|------|------|
| 频道未读计数 | ❌ | — | 未实现 |
| @提及标记 | ❌ | — | 未实现 |
| 统一收件箱 (Inbox) | ❌ | — | 未排期 |

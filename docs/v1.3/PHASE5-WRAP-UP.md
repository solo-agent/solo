# v1.3 Phase 5 — 收尾总结

## 完成日期

2026-05-27

## 目标

对标 Slock 协作质量：Agent 自主决策、持久化会话、双通道架构、完整 System Prompt。

## 架构变更

### 双通道架构

```
Agent 思考过程 → 本地（不推送前端）
Agent CLI 消息 → daemon proxy → server API → WS message.new → 前端
```

- SSE 不再流式推送 MessageText（仅累积供日志）
- 消息投递完全走 proxy → API → message.new 路径
- `skip_persist` 已移除，complete event 仅通知 usage

### 持久化会话

- `PersistentBackend` 接口：Start/Send/Close
- `AgentSessionManager`：会话池、turn 串行锁、idle reaper (5min)
- `--system-prompt-file .solo/system-prompt.md` 替代 `--append-system-prompt`

### System Prompt

- 完全对齐 Slock，15 节，~350 行
- 包含：Runtime Context、CRITICAL RULES、Startup sequence、Messaging、Tasks、@Mentions、Communication style、Workspace & Memory、Compaction safety

### 部署架构

```
Docker:  postgres + server
Host:    daemon + solo CLI
```

- `docker compose up -d` 启动 postgres + server
- daemon 在主机运行（需启动 Claude Code 进程）
- `make start` 全栈启动

## 关键文件变更

| 文件 | 变更 |
|------|------|
| `pkg/agent/prompt.go` | System Prompt 完全重写 |
| `pkg/agent/claude.go` | --system-prompt-file + persistent session |
| `pkg/agent/session.go` | AgentSessionManager 新增 |
| `internal/server/handler/message.go` | asTask 先 INSERT 再 ConvertToTask、thread reply 实时广播 |
| `internal/server/handler/task.go` | broadcastSystemMessageWithID showInChannel bool、claim 不广播 channel |
| `internal/server/service/agent.go` | TriggerAgentForTask Slock 格式消息 |
| `internal/server/service/task.go` | creator_name JOIN、UUID WHERE 修复 |
| `cmd/solo/main.go` | Slock 风格输出、-m flag、stdin heredoc、proxy 路由 |
| `cmd/daemon/handler.go` | proxy 路由、SOLO_AUTH_TOKEN 注入 |
| `frontend/components/dashboard/channel-view.tsx` | asTask 走 sendMessage，不再用 createTask |

## CLI 输出格式

- claim 成功: `Claim results (1 claimed): #N (msg:XXXX): claimed` + thread target
- claim 失败: `Claim results (0 claimed, 1 failed): #N: FAILED — already assigned. Do not reply.`
- task update: `#N moved to in_review.`
- message send: `Message sent to thread. Message ID: UUID (to reply in this thread, use target "channel:id")`

## 死代码清理

- `broadcastClaimSystemMessage`、`broadcastUnclaimSystemMessage`
- `buildRuntimeCLAUDE` (workspace.go)
- `BuildCLAUDEBytes`、`BuildAGENTSBytes`、`BuildGEMINIBytes`
- `InjectInstructionFiles`、`WriteSystemPromptFile`
- `claude_template_test.go`

## Phase 5+ 扩展（2026-05-30 ~ 2026-05-31）

### CLI --target 统一化

对标 Slock，solo CLI 的 `message send` 和 `message read` 统一使用 `--target` 参数：
- `'#channel'` / `'dm:@peer'` / `'#channel:shortid'` / `'dm:@peer:shortid'`
- 移除易混淆的 `-C`/`-t` 旧参数
- `resolveTarget()` 居中解析 target 字符串 → UUID

### Agent Token 会话级持久化

- Access Token 从 15 分钟改为 365 天（session 期间永不过期）
- 磁盘持久化：`~/.solo/agent-tokens/<id>/current.token`（纯 JWT，无时间戳行）
- 每次新 session 创建时刷新 token（替代定时刷新循环）
- Daemon 401 自动 refresh + retry，agent 无感
- Daemon 心跳失败自动重注册

### 消息格式完整对齐 Slock

5 种消息格式全部覆盖：
- `[target=#channel msg=...]` — Channel
- `[target=dm:@peer msg=...]` — DM
- `[target=#channel:shortid msg=...] @sender:` — Thread
- `[target=dm:@peer:shortid msg=...] @sender:` — DM Thread  
- Thread @mention 拉入：`[System: You were added to a new thread via @mention...]`

### System Prompt 对齐

- 全部 CLI 示例使用 `--target`
- DM send/thread 示例、Initial role 分离
- CRITICAL RULES 强化、heredoc 警告
- 线程自动创建提示、任务 workflow 示例

### DM 全链路支持

- Daemon proxy DM 路由（`/api/v1/dm/{id}/messages`）
- `server/info` 返回 DM channel（用于 name→UUID 解析）
- `solo message send --target 'dm:@peer'` 端到端可用

### 基础设施修复

- 死循环防护：cascade 检测（>20次/10s → 60s 冷却）
- Thread ID 三路解析：已有 thread UUID / short ID LIKE / message UUID 自动创建
- `ListAllUserTasks` 支持 `channel_id` 过滤

## 已知限制

- Docker compose 需要网络
- DM WebSocket 实时刷新偶有延迟（React state 更新不触发 DOM 重渲染，根因待查）
- E2E 测试中 agent 模型（deepseek-v4-pro）对 CLI 工具调用的跟随度不稳定（文本抑制率高）

# v1.3 Phase 5 — 双通道架构 (Dual-Channel Architecture)

## 架构背景

Solo v1.3 之前，Agent 的消息投递是**单通道**模型：

```
Agent 思考过程 + 最终回复 → SSE streaming → Server → WebSocket → 前端显示
                                                              → DB 持久化
```

问题：
1. 思考过程泄漏到前端（实时 streaming 显示 Claude 的推理文本）
2. `skip_persist` 只在持久化时生效，前端已渲染的思考文本刷新后才消失
3. 思考文本和 CLI 命令输出混在同一个流中

Slock 的**双通道**模型：

| 通道 | 内容 | 去向 |
|------|------|------|
| 本地思考 | 推理过程、读文件、搜索、写代码 | 仅本机，不走 server |
| CLI 消息 | `solo message send` 输出 | daemon proxy → server API → 前端 + DB |

## 架构决策

### 决策 1：SSE 不再流式推送思考文本

**Before:**
```
daemon: MessageText → pushEventJSON("token", ...) → SSE → server → WS message.agent_typing → 前端 StreamingMessage
```

**After:**
```
daemon: MessageText → 仅累积到 fullContent（日志用），不推送 SSE
daemon: MessageThinking → pushEventJSON("thinking", ...) → SSE → server → WS agent.thinking → 前端 typing indicator
```

**实现位置：** `cmd/daemon/handler.go:711-716`

```go
case string(agent.MessageText):
    // v1.3: Slock-aligned — text output is internal thinking.
    // Do NOT stream to frontend.
    fullContent += chunk.Content
```

前端只会看到 "XXX 思考中..." 的打字指示器，看不到思考内容。

### 决策 2：消息投递完全走 proxy → API 路径

**通道图：**

```
Agent 调用 solo message send
  ↓
solo CLI → daemon proxy (POST /internal/daemon/proxy)
  ↓
daemon 生成 JWT → 转发到 server API (POST /api/v1/channels/{id}/messages)
  ↓
server message handler:
  - DB INSERT
  - WebSocket broadcast message.new → 前端显示
```

**SSE streaming 降级为纯通知**：

```
SSE events:
  thinking → 前端 typing indicator（保留）
  complete → 仅通知 server "agent 完成了，用了 X tokens"（无内容，无持久化）
```

**实现位置：**
- `cmd/daemon/handler.go` ProxyRequest handler
- `cmd/solo/main.go` proxyRequest() 函数
- `internal/server/service/agent.go` handleStreamingAgentTask（已简化）

### 决策 3：skip_persist 已移除

双通道架构下，SSE complete 事件不再携带 `content`、`skip_persist`、`message_id` 字段。

**Before (complete event):**
```json
{
  "agent_id": "...",
  "content": "...",
  "skip_persist": true,
  "message_id": "...",
  "usage": {"input_tokens": 100, "output_tokens": 50}
}
```

**After (complete event):**
```json
{
  "agent_id": "...",
  "usage": {"input_tokens": 100, "output_tokens": 50}
}
```

server 端 `handleStreamingAgentTask` 仅：
1. 转发 `thinking` 事件 → typing indicator
2. 接收 `complete` 事件 → 记录 token 用量
3. 接收 `error` 事件 → 广播错误

不再执行任何消息持久化或 `message.new` 广播。

### 决策 4：server 端 `handleStreamingAgentTask` 已大幅简化

**删除的代码路径（全部死代码）：**

| 函数/代码块 | 原因 |
|-------------|------|
| `broadcastAgentToken` + `broadcastToken` | 不再流式推送 token |
| `broadcastAgentMessage` + `broadcastMessage` + `broadcastDMIfNeeded` | 消息由 proxy 路径创建 |
| `parseAndExecuteTaskClaims` | Agent 用 `solo task claim` CLI |
| `parseAndExecuteTaskUpdates` | Agent 用 `solo task update` CLI |
| `isNotParticipating` | 不再解析 "not for me" 文本 |
| `truncateString` | 不再截断 agent 输出 |
| 全部 DB INSERT / message.new 广播代码 | 消息由 proxy 路径处理 |
| `broadcastThreadToken` | 不再流式推送 |
| `skip_persist` 检查 | 始终 true，已移除 |

**保留的代码路径：**

| 函数 | 用途 |
|------|------|
| `broadcastAgentThinking` + `broadcastThinking` + `broadcastThreadThinking` | typing indicator |
| `broadcastAgentError` + `broadcastError` + `broadcastThreadError` | 错误通知 |
| `HandleTaskComplete` + `broadcastThreadMessage` | 回调路径（daemon → server callback）* |
| `HandleTaskError` | 错误回调 |

\* `HandleTaskComplete` 和 `broadcastThreadMessage` 目前仅由 daemon 回调端点调用，
`notifyServerComplete` 在 daemon 中未被调用，该路径标记为待进一步清理。

## 数据流完整图

```
用户在频道发送 "@产品 你好"
  │
  ├─→ server message handler
  │     ├─ DB INSERT
  │     └─ WS broadcast message.new → 前端显示用户消息
  │
  └─→ AgentService.TriggerAgentResponse()
        │
        └─→ daemon StreamTask() [POST /internal/daemon/run]
              │
              └─→ SSE stream
                    │
                    ├─ "thinking" event
                    │     └─→ server broadcastAgentThinking()
                    │           └─→ WS agent.thinking → 前端 "产品 思考中..."
                    │
                    ├─ (text tokens — 不推送，仅 daemon 内部累积)
                    │
                    └─→ Agent 调用 solo message send "你好！我是产品经理"
                          │
                          ├─→ daemon proxy (POST /internal/daemon/proxy)
                          │     └─→ server API (POST /api/v1/channels/{id}/messages)
                          │           ├─ DB INSERT
                          │           └─ WS broadcast message.new → 前端显示回复
                          │
                          └─ Agent 输出完成
                                └─ SSE "complete" event {agent_id, usage}
                                      └─ server: 记录日志，不做任何广播
```

## 关键文件变更

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `cmd/daemon/handler.go` | 修改 | 停止 SSE token 推送，简化 complete event，加 proxy handler |
| `cmd/solo/main.go` | 修改 | 新增 proxyRequest()，消息发送走 daemon proxy |
| `internal/server/service/agent.go` | 大幅简化 | 删除 ~800 行死代码 |
| `pkg/agent/session.go` | 新增 | AgentSessionManager 持久化 session 管理 |
| `pkg/agent/prompt.go` | 重写 | ~350 行 Slock 对齐 prompt |

## 后续迭代建议

1. **清理回调完成路径**：`notifyServerComplete` → `HandleTaskComplete` → `broadcastThreadMessage` 链，确认是否完全不需要，如不需要可删除
2. **Token usage 持久化**：当前 complete event 的 usage 信息只记日志未入库，可考虑写入 agent_usage 表供统计
3. **Thinking 事件可配置**：`agent.thinking` 事件频率高时可能影响前端性能，可加节流

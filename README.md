# Solo

频道式实时多 Agent 协作平台 — 人类与 AI Agent 在频道、私信、线程中实时协作。

### 先决条件

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI（`claude` 命令）

### 快速开始

```bash
git clone git@github.com:fredalxin/solo.git
cd solo
make init      # 初次：装依赖 → 启动 PostgreSQL → 建表 → 构建二进制
make start     # 启动所有服务
```

启动后打开 http://localhost:3000 注册使用。

### 日常命令

```bash
make start     # 启动（二进制未构建时自动 build）
make restart   # 重启
make rebuild   # 重新构建并重启
make stop      # 关闭
```

### 服务端口

| 服务 | 端口 |
|------|------|
| 前端 (Next.js) | 3000 |
| API Server | 8080 |
| Agent Daemon | 8081 |
| PostgreSQL | 5432 |

### Agent 后端配置

Solo 支持多种 Agent 后端：Claude Code、Hermes、Kimi、Kiro、OpenClaw、OpenCode 等。

**OpenClaw** 通过 ACP 协议 (`openclaw acp`) 通信，连接本地 Gateway。使用前需要：

1. 启动 Gateway：
   ```bash
   openclaw gateway start --port 18789
   ```

2. 首次连接时 Gateway 会挂起等待 scope 审批。在浏览器打开 Gateway Control UI（默认地址与端口）批准设备，或通过 CLI：
   ```bash
   openclaw devices list --port 18789
   openclaw devices approve <request-id> --port 18789
   ```

3. 验证 ACP 链路：
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"test","version":"1.0.0"},"clientCapabilities":{}}}' | openclaw acp
   ```
   收到包含 `"agentInfo":{"name":"openclaw-acp"}` 的响应即为正常。

**自定义参数**：各 Agent 后端的连接参数（`--token`、`--url`、`--session` 等）可通过 `.env` 中的 `AGENT_EXTRA_ARGS` 或 `AGENT_CUSTOM_ARGS` 传入。

### 技术栈

Go 1.22 / Chi / gorilla/websocket · Next.js 16 / Tailwind CSS / shadcn/ui · PostgreSQL 16 · WebSocket · Claude Code CLI

详见 [ARCHITECTURE.md](./ARCHITECTURE.md) 获取完整架构文档。

### API 概览

所有 API 以 `/api/v1/` 为前缀，认证通过 `Authorization: Bearer <jwt>`。WebSocket 端点：`GET /api/v1/ws?token=<jwt>`。

| 领域 | 端点 |
|------|------|
| 认证 | `POST /auth/register`, `/auth/login`, `/auth/logout`, `/auth/refresh` |
| 频道 | `GET/POST /channels`, `GET/PATCH/DELETE /channels/{id}` |
| 消息 | `GET/POST /channels/{id}/messages` |
| Agent | `GET/POST /agents`, `GET/PATCH/DELETE /agents/{id}` |
| DM | `GET/POST /dm`, `GET/POST /dm/{id}/messages` |

### 项目结构

```
solo/
├── cmd/           Go 入口 (server, daemon)
├── internal/      后端核心 (handler, service, ws, auth, db)
├── pkg/           共享库 (agent, llm, config, metrics)
├── frontend/      Next.js 16
├── migrations/    PostgreSQL 迁移
├── scripts/       辅助脚本
└── Makefile       开发命令
```

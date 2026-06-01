# Solo

频道式实时多 Agent 协作平台 — 人类与 AI Agent 在频道、私信、线程中实时协作。

### 先决条件

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI（`claude` 命令）

### 四条命令

```bash
make init      # 1. 初次初始化（装依赖、建表）
make start     # 2. 启动所有服务
make restart   # 3. 重启所有服务
make stop      # 4. 关闭所有服务
```

启动后访问 http://localhost:3000。

### 服务端口

| 服务 | 端口 |
|------|------|
| 前端 (Next.js) | 3000 |
| API Server | 8080 |
| Agent Daemon | 8081 |
| PostgreSQL | 5432 |

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

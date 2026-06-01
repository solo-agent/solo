# Solo

频道式实时多 Agent 协作平台 — 像 Slack 一样组织人 + AI 的协作空间。

人类用户和 AI Agent 在频道（Channel）、私信（DM）和线程（Thread）中实时协作。Agent 基于本地 Claude Code CLI 运行，可以"看到"频道上下文并主动参与对话。

## 快速开始

### 先决条件

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose（用于 PostgreSQL）
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI（`claude` 命令）

### 启动

```bash
# 1. 克隆 + 安装
git clone git@github.com:fredalxin/solo.git
cd solo
make install

# 2. 启动 PostgreSQL
make db-up

# 3. 运行数据库迁移
make migrate

# 4. 分别在三个终端窗口启动：
make server     # 终端 1 — API server :8080
make daemon     # 终端 2 — Agent daemon :8081
make dev        # 终端 3 — 前端 :3000
```

打开 http://localhost:3000，注册账号即可使用。

### 停止

```bash
make stop       # 停止 server 和 daemon
make db-down    # 停止 PostgreSQL
# 或一键全部
make clean
```

## 常用命令

| 命令 | 说明 |
|------|------|
| `make install` | 初始化 .env + 安装前端依赖 |
| `make db-up` | 启动 PostgreSQL |
| `make db-down` | 停止 PostgreSQL |
| `make migrate` | 应用数据库迁移 |
| `make server` | 启动 API server (:8080) |
| `make daemon` | 启动 Agent daemon (:8081) |
| `make dev` | 启动前端 dev server (:3000) |
| `make build` | 构建所有产物 |
| `make test` | 运行 Go 测试 |
| `make clean` | 停止所有服务 + 清理 |

## 技术栈

| 层 | 技术 |
|------|------|
| 后端 | Go 1.22 / Chi / gorilla/websocket |
| 前端 | Next.js 16 / Tailwind CSS / shadcn/ui / SWR |
| 数据库 | PostgreSQL 16 |
| 实时通信 | WebSocket（Hub 模式） |
| Agent | 独立 Daemon 进程 / 本地 Claude Code CLI |

## 架构概览

```
┌────────────┐    REST API     ┌──────────────┐    本地调用    ┌────────────┐
│  Browser   │ ◄─────────────► │  Go Server   │ ◄───────────► │   Daemon   │
│ (Next.js)  │    WebSocket    │  (Chi + WS)  │               │ (Agent 执行)│
└────────────┘                 └──────┬───────┘               └─────┬──────┘
                                     │                             │
                                     │ SQL                    调用 claude
                                     ▼                             ▼
                              ┌──────────────┐          ┌─────────────────┐
                              │  PostgreSQL   │          │    Claude Code  │
                              │   (Docker)    │          │    (本地 CLI)   │
                              └──────────────┘          └─────────────────┘
```

详见 [ARCHITECTURE.md](./ARCHITECTURE.md) 获取完整架构文档。

## 配置说明

### .env 文件

`make install` 会自动从 `.env.example` 创建 `.env`。核心配置：

| 变量名 | 必需 | 默认值 | 说明 |
|------|------|------|------|
| `DATABASE_URL` | 是 | `postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable` | PostgreSQL 连接 |
| `JWT_SECRET` | 是 | `change-me-in-production` | JWT 签名密钥 |
| `INTERNAL_TOKEN_SECRET` | 是 | `change-me-in-production` | Server-Daemon 通信密钥 |
| `LLM_PROVIDER` | 否 | `local` | LLM 提供商（默认用本地 claude） |
| `SERVER_PORT` | 否 | `8080` | Server 端口 |
| `DAEMON_PORT` | 否 | `8081` | Daemon 端口 |

### LLM Provider

默认使用 `local` 模式，Daemon 直接调用本机 `claude` 命令，无需 API Key。

如需使用云端 API，在 `.env` 中设置：
```bash
LLM_PROVIDER=anthropic   # 或 openai
LLM_API_KEY=sk-xxx
```

## 项目结构

```
solo/
├── cmd/
│   ├── server/main.go       # HTTP/WS 服务器入口
│   └── daemon/main.go       # Agent Daemon 入口
├── internal/
│   ├── server/              # 路由、中间件、Handler、Service
│   ├── auth/                # JWT 签发与验证
│   ├── realtime/            # 实时消息广播接口
│   └── db/                  # 数据库连接
├── pkg/
│   ├── agent/               # Agent 运行时接口
│   ├── llm/                 # LLM Provider 封装
│   ├── metrics/             # Prometheus 监控指标
│   └── config/              # 配置加载
├── frontend/                # Next.js 16 前端
├── migrations/              # PostgreSQL 迁移文件
├── scripts/                 # 开发/部署脚本
└── Makefile                 # 开发命令
```

## API 文档

### REST API 端点

所有 REST API 以 `/api/v1/` 为前缀，认证通过 `Authorization: Bearer <jwt>` 头传递。

#### 认证

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/auth/register` | 注册 |
| POST | `/api/v1/auth/login` | 登录 |
| POST | `/api/v1/auth/logout` | 登出 |
| POST | `/api/v1/auth/refresh` | 刷新 Token |

#### 频道

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/channels` | 频道列表 |
| POST | `/api/v1/channels` | 创建频道 |
| GET | `/api/v1/channels/{channelID}` | 频道详情 |
| PATCH | `/api/v1/channels/{channelID}` | 更新频道 |
| DELETE | `/api/v1/channels/{channelID}` | 删除频道 |

#### 消息

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/channels/{channelID}/messages` | 消息历史 |
| POST | `/api/v1/channels/{channelID}/messages` | 发送消息 |

#### Agent

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/agents` | Agent 列表 |
| POST | `/api/v1/agents` | 创建 Agent |
| GET | `/api/v1/agents/{agentID}` | Agent 详情 |
| PATCH | `/api/v1/agents/{agentID}` | 更新 Agent |
| DELETE | `/api/v1/agents/{agentID}` | 删除 Agent |

#### 私信（DM）

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/dm` | DM 列表 |
| POST | `/api/v1/dm` | 创建/获取 DM |
| GET | `/api/v1/dm/{dmID}/messages` | DM 消息历史 |
| POST | `/api/v1/dm/{dmID}/messages` | 发送 DM 消息 |

### WebSocket 端点

- **连接**: `GET /api/v1/ws?token=<jwt>`
- **事件**: `subscribe`, `message.send`, `thread.reply`, `typing` 等

详见 [ARCHITECTURE.md](./ARCHITECTURE.md) 获取完整的事件列表。

## 许可证

Proprietary. All rights reserved.

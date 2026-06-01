# Solo

频道式实时多 Agent 协作平台 -- 像 Slack 一样组织人 + AI 的协作空间。

人类用户和 AI Agent 在频道（Channel）、私信（DM）和线程（Thread）中实时协作。Agent 可以"看到"频道上下文并主动参与对话，人类可以编排多个 Agent 协同工作。

## 快速开始

### Docker Compose（推荐）

```bash
# 1. 克隆仓库
git clone <repo-url>
cd solo

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件，设置 LLM_API_KEY

# 3. 启动所有服务（PostgreSQL + Server + Daemon）
docker compose up -d

# 4. 运行数据库迁移
./scripts/migrate.sh up

# 5. 启动前端（另一个终端）
cd frontend
npm install
npm run dev

# 6. 访问 http://localhost:3000
```

启动后你会看到：
- **Frontend**: http://localhost:3000 （用户界面）
- **Server**: http://localhost:8080 （REST API + WebSocket）
- **Daemon**: http://localhost:8081 （Agent 执行引擎）
- **PostgreSQL**: localhost:5432

### 本地开发

```bash
# 后端
go run ./cmd/server/

# Agent Daemon
go run ./cmd/daemon/

# 前端
cd frontend
npm install
npm run dev  # http://localhost:3000
```

### 先决条件

- Go 1.22+
- PostgreSQL 16
- Node.js 20+ （前端开发）
- Docker & Docker Compose （可选，用于容器化部署）
- LLM API Key （Anthropic 或 OpenAI）

## 技术栈

| 层       | 技术                               |
|----------|-----------------------------------|
| 后端     | Go 1.22 / Chi / gorilla/websocket  |
| 前端     | Next.js 16 / Tailwind CSS / shadcn/ui / SWR |
| 数据库   | PostgreSQL 16                       |
| 实时通信 | WebSocket（Hub 模式）              |
| Agent    | 独立 Daemon 进程 / HTTP+SSE 通信   |
| 监控     | Prometheus + Grafana （可选）      |

## 架构概览

```
┌────────────┐    REST API     ┌──────────────┐    HTTP/SSE    ┌────────────┐
│  Browser   │ ◄─────────────► │  Go Server   │ ◄────────────► │   Daemon   │
│ (Next.js)  │    WebSocket    │  (Chi + WS)  │               │ (Agent 执行)│
└────────────┘                 └──────┬───────┘               └─────┬──────┘
                                      │                            │
                                      │ SQL                        │ LLM API
                                      ▼                            ▼
                               ┌──────────────┐          ┌─────────────────┐
                               │  PostgreSQL   │          │  OpenAI/Claude  │
                               └──────────────┘          └─────────────────┘
```

详见 [ARCHITECTURE.md](./ARCHITECTURE.md) 获取完整架构文档。

## 配置说明

### 核心环境变量

| 变量名                   | 必需 | 默认值                     | 说明                            |
|-------------------------|------|---------------------------|--------------------------------|
| `DATABASE_URL`          | 是   | -                         | PostgreSQL 连接字符串           |
| `JWT_SECRET`            | 是   | -                         | JWT 签名密钥                    |
| `INTERNAL_TOKEN_SECRET` | 是   | -                         | Server-Daemon 通信密钥          |
| `LLM_API_KEY`           | 是   | -                         | LLM Provider API Key           |
| `LLM_PROVIDER`          | 否   | `anthropic`               | LLM 提供商（anthropic/openai）  |
| `SERVER_PORT`           | 否   | `8080`                    | Server 监听端口                 |
| `DAEMON_PORT`           | 否   | `8081`                    | Daemon 监听端口                 |
| `DAEMON_ID`             | 否   | `daemon-01`               | Daemon 实例标识                 |
| `APP_ENV`               | 否   | `development`             | 运行环境                        |

### 生产环境配置

复制 `.env.production` 模板并生成密钥：

```bash
cp .env.production .env
./scripts/gen-secret.sh >> .env
# 手动设置 LLM_API_KEY
```

详细的 PostgreSQL 生产配置建议见 `.env.production` 文件注释。

### Secret 生成

```bash
# 生成所有密钥
./scripts/gen-secret.sh

# 生成指定密钥
./scripts/gen-secret.sh jwt       # JWT 签名密钥
./scripts/gen-secret.sh internal  # 内部通信密钥
./scripts/gen-secret.sh db        # 数据库密码
```

## 项目结构

```
solo/
├── cmd/
│   ├── server/main.go       # HTTP/WS 服务器入口
│   └── daemon/main.go       # Agent Daemon 入口
├── internal/
│   ├── server/              # 路由、中间件、Handler、Service
│   │   ├── router.go        # Chi 路由注册
│   │   ├── handler/         # REST API Handler
│   │   ├── middleware/      # 鉴权、CORS、日志、限流
│   │   ├── ws/              # WebSocket Hub + Client
│   │   └── service/         # 业务逻辑层
│   ├── auth/                # JWT 签发与验证
│   ├── realtime/            # 实时消息广播接口
│   ├── db/                  # 数据库连接与迁移
│   └── model/               # 共享领域模型
├── pkg/
│   ├── agent/               # Agent 运行时接口
│   ├── llm/                 # LLM Provider 封装
│   ├── metrics/             # Prometheus 监控指标
│   └── config/              # 配置加载
├── frontend/                # Next.js 16 前端
├── migrations/              # PostgreSQL 迁移文件
├── scripts/                 # 开发/部署脚本
└── monitoring/              # Prometheus + Grafana 配置
```

## API 文档

### REST API 端点

所有 REST API 以 `/api/v1/` 为前缀，认证通过 `Authorization: Bearer <jwt>` 头传递。

#### 认证

| 方法   | 路径                          | 描述     |
|--------|-------------------------------|----------|
| POST   | `/api/v1/auth/register`       | 注册     |
| POST   | `/api/v1/auth/login`          | 登录     |
| POST   | `/api/v1/auth/logout`         | 登出     |
| POST   | `/api/v1/auth/refresh`        | 刷新 Token |

#### 频道

| 方法   | 路径                                      | 描述         |
|--------|-------------------------------------------|--------------|
| GET    | `/api/v1/channels`                        | 频道列表     |
| POST   | `/api/v1/channels`                        | 创建频道     |
| GET    | `/api/v1/channels/{channelID}`            | 频道详情     |
| PATCH  | `/api/v1/channels/{channelID}`            | 更新频道     |
| DELETE | `/api/v1/channels/{channelID}`            | 删除频道     |
| GET    | `/api/v1/channels/{channelID}/members`    | 成员列表     |
| POST   | `/api/v1/channels/{channelID}/members`    | 添加成员     |
| DELETE | `/api/v1/channels/{channelID}/members/{memberID}` | 移除成员 |

#### 消息

| 方法   | 路径                                      | 描述         |
|--------|-------------------------------------------|--------------|
| GET    | `/api/v1/channels/{channelID}/messages`   | 消息历史（游标分页） |
| POST   | `/api/v1/channels/{channelID}/messages`   | 发送消息     |

#### 线程

| 方法   | 路径                                                      | 描述         |
|--------|-----------------------------------------------------------|--------------|
| POST   | `/api/v1/channels/{channelID}/messages/{messageID}/thread` | 创建线程回复 |
| GET    | `/api/v1/channels/{channelID}/threads/{threadID}`          | 线程消息列表 |

#### Agent

| 方法   | 路径                             | 描述       |
|--------|----------------------------------|------------|
| GET    | `/api/v1/agents`                 | Agent 列表 |
| POST   | `/api/v1/agents`                 | 创建 Agent |
| GET    | `/api/v1/agents/{agentID}`       | Agent 详情 |
| PATCH  | `/api/v1/agents/{agentID}`       | 更新 Agent |
| DELETE | `/api/v1/agents/{agentID}`       | 删除 Agent |

#### 私信（DM）

| 方法   | 路径                       | 描述           |
|--------|----------------------------|----------------|
| GET    | `/api/v1/dm`               | DM 列表        |
| POST   | `/api/v1/dm`               | 创建/获取 DM   |
| GET    | `/api/v1/dm/{dmID}`        | DM 详情        |
| GET    | `/api/v1/dm/{dmID}/messages` | DM 消息历史   |
| POST   | `/api/v1/dm/{dmID}/messages` | 发送 DM 消息  |

### WebSocket 端点

- **连接**: `GET /api/v1/ws?token=<jwt>`
- **事件**: 支持 `subscribe`, `message.send`, `thread.reply`, `typing` 等事件
- **推送**: `message.new`, `agent.thinking`, `message.agent_typing` 等

详见 [ARCHITECTURE.md](./ARCHITECTURE.md#62-websocket-事件类型) 获取完整的事件列表。

## 部署

### Docker 部署

```bash
# 1. 配置生产环境
cp .env.production .env
./scripts/gen-secret.sh >> .env
# 编辑 .env，设置 LLM_API_KEY 和 DATABASE_URL

# 2. 运行部署脚本
./scripts/deploy.sh

# 或手动部署
docker compose -f docker-compose.yml up --build -d
./scripts/migrate.sh up
```

### 回滚

部署脚本在健康检查失败时自动回滚。手动回滚：

```bash
# 回滚到上一版本
docker compose down
# 切换回之前的镜像标签或代码版本
git checkout <previous-tag>
docker compose up --build -d
```

## 监控

### Metrics 端点

Server 在 `GET /metrics` 暴露 Prometheus 格式的指标：

- `solo_requests_total` — HTTP 请求总数
- `solo_requests_active` — 当前活跃请求数
- `solo_errors_total` — 服务端错误总数
- `solo_ws_connections` — WebSocket 连接数
- `solo_uptime_seconds` — 运行时长
- `solo_request_duration_avg_ms` — 平均请求耗时

### Prometheus + Grafana（可选）

取消 docker-compose.yml 中 Prometheus 和 Grafana 服务的注释后：

```bash
docker compose up -d prometheus grafana
```

- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 （admin/admin）

告警规则配置在 `monitoring/alerts.yml`，包括：
- 错误率 > 5%
- WebSocket 连接数异常
- Daemon 离线（> 90s）
- Server/Database 不可达

## 开发指南

### 后端开发

```bash
# 运行测试
go test ./... -v

# 运行单个包测试
go test ./internal/auth/ -v

# Lint
golangci-lint run ./...
```

### 前端开发

```bash
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev            # http://localhost:3000

# Lint + 类型检查
npm run lint
npx tsc --noEmit

# 构建
npm run build
```

### 数据库迁移

```bash
# 应用迁移
./scripts/migrate.sh up

# 回滚
./scripts/migrate.sh down

# 创建新迁移
./scripts/migrate.sh create <migration_name>

# 查看版本
./scripts/migrate.sh version
```

## 许可证

Proprietary. All rights reserved.

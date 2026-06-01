.PHONY: init start restart stop

# ── 1. 初次初始化 ────────────────────────────────────────────────────────────
init:
	@echo "=== 初始化 .env ==="
	@cp -n .env.example .env 2>/dev/null || true
	@echo "=== 安装前端依赖 ==="
	@cd frontend && npm install
	@echo "=== 启动 PostgreSQL ==="
	@docker compose up -d postgres
	@echo "=== 等待 PostgreSQL 就绪 ==="
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker exec solo-postgres pg_isready -U solo -d solo > /dev/null 2>&1 && break; \
		sleep 1; \
	done
	@echo "=== 执行数据库迁移 ==="
	@for f in migrations/*.up.sql; do \
		docker exec -i solo-postgres psql -U solo -d solo < "$$f" > /dev/null 2>&1; \
	done
	@echo "=== 初始化完成 ==="

# ── 2. 启动所有服务 ─────────────────────────────────────────────────────────
start:
	@mkdir -p .pids
	@# PostgreSQL
	@if ! docker exec solo-postgres pg_isready -U solo -d solo > /dev/null 2>&1; then \
		docker compose up -d postgres; \
		echo "等待 PostgreSQL..."; \
		for i in 1 2 3 4 5 6 7 8 9 10; do \
			docker exec solo-postgres pg_isready -U solo -d solo > /dev/null 2>&1 && break; \
			sleep 1; \
		done; \
	fi
	@echo "PostgreSQL ✓"
	@# Build
	@echo "Building..."
	@go build -o .pids/server ./cmd/server/
	@go build -o .pids/daemon ./cmd/daemon/
	@# Server
	@if [ -f .pids/server.pid ] && kill -0 $$(cat .pids/server.pid) 2>/dev/null; then \
		echo "Server already running"; \
	else \
		.pids/server > server.log 2>&1 & \
		echo $$! > .pids/server.pid; \
		for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
			curl -sf http://localhost:8080/readyz > /dev/null 2>&1 && break; \
			sleep 0.5; \
		done; \
		echo "Server :8080 ✓"; \
	fi
	@# Daemon
	@if [ -f .pids/daemon.pid ] && kill -0 $$(cat .pids/daemon.pid) 2>/dev/null; then \
		echo "Daemon already running"; \
	else \
		.pids/daemon > daemon.log 2>&1 & \
		echo $$! > .pids/daemon.pid; \
		sleep 2; \
		echo "Daemon :8081 ✓"; \
	fi
	@# Frontend
	@echo "Starting frontend..."
	@if [ -f .pids/frontend.pid ] && kill -0 $$(cat .pids/frontend.pid) 2>/dev/null; then \
		echo "Frontend already running"; \
	else \
		cd frontend && npm run dev > /dev/null 2>&1 & \
		echo $$! > ../.pids/frontend.pid; \
		echo "Frontend :3000 ✓"; \
	fi
	@echo "=== 全部启动完成 ==="
	@echo "  http://localhost:3000"

# ── 3. 重启 ──────────────────────────────────────────────────────────────────
restart: stop start

# ── 4. 关闭所有 ──────────────────────────────────────────────────────────────
stop:
	@echo "=== 关闭所有服务 ==="
	@-lsof -ti :8080 | xargs kill 2>/dev/null && echo "Server stopped" || echo "Server not running"
	@-lsof -ti :8081 | xargs kill 2>/dev/null && echo "Daemon stopped" || echo "Daemon not running"
	@-lsof -ti :3000 | xargs kill 2>/dev/null && echo "Frontend stopped" || echo "Frontend not running"
	@rm -rf .pids/
	@echo "=== 全部关闭 ==="

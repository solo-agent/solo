.PHONY: init start restart rebuild stop pg-ready migrate build

# ── 0. Common: wait for PostgreSQL to be ready (30s timeout) ──────────────
pg-ready:
	@docker compose up -d --remove-orphans postgres >/dev/null
	@for i in $$(seq 1 30); do \
		docker exec solo-postgres pg_isready -U solo -d solo >/dev/null 2>&1 && exit 0; \
		sleep 1; \
	done; \
	echo "ERROR: PostgreSQL not ready after 30s"; exit 1

# ── 0. Common: run migrations (idempotent, any failure exits immediately) ──
migrate: pg-ready
	@echo "=== Running database migrations ==="
	@docker exec -i solo-postgres psql -U solo -d solo -v ON_ERROR_STOP=1 -q -c \
		"CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());" >/dev/null
	@set -e; for f in migrations/*.up.sql; do \
		v=$$(basename $$f .up.sql); \
		applied=$$(docker exec -i solo-postgres psql -U solo -d solo -tAc "SELECT 1 FROM schema_migrations WHERE version='$$v'"); \
		if [ "$$applied" = "1" ]; then \
			echo "  ✓ $$v (already applied, skipped)"; \
		else \
			echo "  → $$v"; \
			docker exec -i solo-postgres psql -U solo -d solo -v ON_ERROR_STOP=1 -q < "$$f"; \
			docker exec -i solo-postgres psql -U solo -d solo -v ON_ERROR_STOP=1 -q -c \
				"INSERT INTO schema_migrations(version) VALUES ('$$v');" >/dev/null; \
		fi; \
	done

# ── 0. Common: build binaries ──────────────────────────────────────────────
build:
	@mkdir -p .pids
	@go build -o .pids/server ./cmd/server/
	@go build -o .pids/daemon ./cmd/daemon/
	@go build -o .pids/solo ./cmd/solo/

# ── 1. First-time setup ────────────────────────────────────────────────────
init:
	@echo "=== Initializing .env ==="
	@cp -n .env.example .env 2>/dev/null || true
	@echo "=== Installing frontend dependencies ==="
	@cd frontend && npm install
	@$(MAKE) migrate
	@echo "=== Building binaries ==="
	@$(MAKE) build
	@echo "=== Initialization complete ==="

# ── 2. Start all services ──────────────────────────────────────────────────
start: pg-ready
	@mkdir -p .pids
	@echo "PostgreSQL ✓"
	@# Server
	@if [ -f .pids/server.pid ] && kill -0 $$(cat .pids/server.pid) 2>/dev/null; then \
		echo "Server already running"; \
	else \
		if [ ! -f .pids/server ]; then \
			echo "Building server..."; \
			go build -o .pids/server ./cmd/server/; \
		fi; \
		.pids/server > server.log 2>&1 & \
		echo $$! > .pids/server.pid; \
		ok=0; \
		for i in $$(seq 1 30); do \
			curl -sf http://localhost:8080/readyz >/dev/null 2>&1 && { ok=1; break; }; \
			sleep 0.5; \
		done; \
		if [ $$ok -ne 1 ]; then \
			echo "ERROR: Server on :8080 did not become ready, recent logs:"; \
			tail -20 server.log; \
			exit 1; \
		fi; \
		echo "Server :8080 ✓"; \
	fi
	@# Daemon
	@if [ -f .pids/daemon.pid ] && kill -0 $$(cat .pids/daemon.pid) 2>/dev/null; then \
		echo "Daemon already running"; \
	else \
		if [ ! -f .pids/daemon ]; then \
			echo "Building daemon..."; \
			go build -o .pids/daemon ./cmd/daemon/; \
			go build -o .pids/solo ./cmd/solo/; \
		fi; \
		.pids/daemon > daemon.log 2>&1 & \
		echo $$! > .pids/daemon.pid; \
		sleep 2; \
		if ! kill -0 $$(cat .pids/daemon.pid) 2>/dev/null; then \
			echo "ERROR: Daemon failed to start, recent logs:"; \
			tail -20 daemon.log; \
			exit 1; \
		fi; \
		echo "Daemon :8081 ✓"; \
	fi
	@# Frontend
	@if [ -f .pids/frontend.pid ] && kill -0 $$(cat .pids/frontend.pid) 2>/dev/null; then \
		echo "Frontend already running"; \
	else \
		cd frontend && npm run dev > /dev/null 2>&1 & \
		echo $$! > ../.pids/frontend.pid; \
		echo "Frontend :3000 ✓"; \
	fi
	@echo "=== All services started ==="
	@echo "  http://localhost:3000"

# ── 3. Restart ─────────────────────────────────────────────────────────────
restart: stop start

# ── 4. Rebuild and restart ─────────────────────────────────────────────────
rebuild: stop build start

# ── 5. Shut down all ───────────────────────────────────────────────────────
stop:
	@echo "=== Stopping all services ==="
	@-lsof -ti :8080 | xargs kill 2>/dev/null && echo "Server stopped" || echo "Server not running"
	@-lsof -ti :8081 | xargs kill 2>/dev/null && echo "Daemon stopped" || echo "Daemon not running"
	@-lsof -ti :3000 | xargs kill 2>/dev/null && echo "Frontend stopped" || echo "Frontend not running"
	@rm -f .pids/*.pid
	@sleep 1
	@echo "=== All services stopped ==="

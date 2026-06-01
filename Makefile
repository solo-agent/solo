.PHONY: install db-up db-down migrate server daemon dev build clean stop

# ── Setup ────────────────────────────────────────────────────────────────────

install:
	@cp -n .env.example .env 2>/dev/null || true
	@echo ".env ready — edit it to set LLM_API_KEY if needed"
	cd frontend && npm install
	@echo "Frontend dependencies installed"

# ── Database ─────────────────────────────────────────────────────────────────

db-up:
	docker compose up -d postgres
	@echo "PostgreSQL running on :5432"

db-down:
	docker compose down

migrate:
	@for f in migrations/*.up.sql; do \
		docker exec -i solo-postgres psql -U solo -d solo < "$$f" > /dev/null 2>&1; \
	done
	@echo "Migrations applied"

# ── Run (start each in a separate terminal) ─────────────────────────────────

server:
	go run ./cmd/server/

daemon:
	go run ./cmd/daemon/

dev:
	cd frontend && npm run dev

# ── Build ────────────────────────────────────────────────────────────────────

build:
	go build -o bin/server ./cmd/server/
	go build -o bin/daemon ./cmd/daemon/
	cd frontend && npm run build
	@echo "Build complete"

# ── Test ─────────────────────────────────────────────────────────────────────

test:
	go test ./... -v

# ── Stop ─────────────────────────────────────────────────────────────────────

stop:
	@lsof -ti :8080 | xargs kill 2>/dev/null || true
	@lsof -ti :8081 | xargs kill 2>/dev/null || true
	@echo "Server and daemon stopped"

# ── Clean ────────────────────────────────────────────────────────────────────

clean: stop db-down
	@rm -rf bin/ .pids/
	@echo "Cleaned"

.PHONY: help dev init start restart rebuild stop clean-pids build migrate db-reset
.DEFAULT_GOAL := help

ENV_FILE ?= .env

ifneq ($(wildcard $(ENV_FILE)),)
include $(ENV_FILE)
export
endif

##@ Quick start

help: ## Show available make targets
	@awk 'BEGIN {FS = ":.*## "; \
		printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nFirst time:\n  \033[36mmake dev\033[0m       Bootstrap and start everything in one shot\n\n"} \
		/^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_.-]+:.*## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Bootstrap from scratch and start all services
	@bash scripts/dev.sh

##@ Lifecycle

init: ## Install deps, set up DB, run migrations, build binaries (no start)
	@cp -n .env.example .env 2>/dev/null || true
	@echo "=== Installing frontend dependencies ==="
	@cd frontend && npm install
	@bash scripts/ensure-postgres.sh
	@$(MAKE) migrate
	@$(MAKE) build
	@echo "=== Initialization complete ==="

start: ## Start all services (ensures DB, runs migrations, auto-builds if needed)
	@bash scripts/ensure-postgres.sh
	@go run ./cmd/migrate up
	@bash scripts/start-services.sh

restart: stop start ## Restart all services

rebuild: stop clean-pids build start ## Rebuild binaries from a clean .pids dir and restart all services

stop: ## Shut down all services
	@echo "=== Stopping all services ==="
	@-for f in .pids/*.pid; do [ -f "$$f" ] || continue; kill "$$(cat "$$f")" 2>/dev/null || true; done
	@-lsof -t "$$(pwd)/.pids/server" "$$(pwd)/.pids/daemon" 2>/dev/null | xargs kill 2>/dev/null && echo "Stale .pids processes stopped" || true
	@-lsof -ti tcp:8080 -sTCP:LISTEN | xargs kill 2>/dev/null && echo "Server stopped" || echo "Server not running"
	@-lsof -ti tcp:8081 -sTCP:LISTEN | xargs kill 2>/dev/null && echo "Daemon stopped" || echo "Daemon not running"
	@-lsof -ti tcp:3000 -sTCP:LISTEN | xargs kill 2>/dev/null && echo "Frontend stopped" || echo "Frontend not running"
	@rm -f .pids/*.pid
	@sleep 1
	@echo "=== All services stopped ==="

clean-pids: ## Remove generated binaries and pid files
	@rm -rf .pids
	@mkdir -p .pids

##@ Build & Database

build: ## Build server, daemon, solo CLI, and migrate binaries
	@mkdir -p .pids
	@go build -o .pids/server ./cmd/server/
	@go build -o .pids/daemon ./cmd/daemon/
	@go build -o .pids/solo ./cmd/solo/
	@go build -o .pids/migrate ./cmd/migrate/

migrate: ## Apply database migrations (idempotent)
	@bash scripts/ensure-postgres.sh
	@go run ./cmd/migrate up

# Drop and recreate the current env's database, then run all migrations.
# Use for a clean slate in local dev. Only affects the DB named in
# DATABASE_URL; the postgres container itself is untouched. Refuses to run
# against a remote host.
db-reset: ## Drop and recreate the local database, then re-run all migrations
	@if [ ! -f "$(ENV_FILE)" ]; then echo "ERROR: $(ENV_FILE) not found. Run 'make dev' first."; exit 1; fi
	@case "$(DATABASE_URL)" in \
		""|*@localhost:*|*@localhost/*|*@127.0.0.1:*|*@127.0.0.1/*|*@\[::1\]:*|*@\[::1\]/*) ;; \
		*) echo "Refusing to reset: DATABASE_URL points at a remote host."; exit 1 ;; \
	esac
	@bash scripts/ensure-postgres.sh
	@DB_USER=$$(printf '%s' "$(DATABASE_URL)" | sed -E 's|^postgres(ql)?://([^:@]+):.*|\2|'); \
	 DB_NAME=$$(printf '%s' "$(DATABASE_URL)" | sed -E 's|^.*/([^/?]+)(\?.*)?$$|\1|'); \
	 CONTAINER=$${SOLO_POSTGRES_CONTAINER:-solo-postgres}; \
	 echo "==> Dropping and recreating database '$$DB_NAME'..."; \
	 docker exec "$$CONTAINER" psql -U "$$DB_USER" -d postgres -v ON_ERROR_STOP=1 \
	   -c "DROP DATABASE IF EXISTS \"$$DB_NAME\" WITH (FORCE);" \
	   -c "CREATE DATABASE \"$$DB_NAME\";"; \
	 echo "==> Running migrations..."; \
	 go run ./cmd/migrate up
	@echo ""
	@echo "✓ Database reset. Run 'make start' to launch the app."

.PHONY: help dev init start restart rebuild stop clean-pids build migrate db-reset test-release-install test-e2e-agent-delivery test-e2e-agent-template-credential test-e2e-automation test-e2e-budget-gate test-e2e-budget-gate-run test-e2e-agent-session-resume test-e2e-agent-idle-resume test-e2e-agent-scope-router test-e2e-send-freshness test-e2e-websocket-recovery test-e2e-m8 test-e2e-m9 test-e2e-remote-server test-e2e-public-remote test-e2e-workspaces
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

test-e2e-agent-delivery: ## Rebuild and verify the real Agent result delivery contract
	@bash scripts/run-local-e2e.sh agent-delivery env CI=1 SOLO_E2E_REAL_AGENT_DELIVERY=1 npx playwright test e2e/agent-result-delivery.spec.ts

test-e2e-agent-template-credential: ## Verify template discovery across persistent Agent Runs
	@bash scripts/run-local-e2e.sh agent-template-credential env CI=1 SOLO_E2E_REAL_AGENT_DELIVERY=1 npx playwright test e2e/agent-result-delivery.spec.ts --grep "lists templates through the current Run" --workers=1

test-e2e-automation: ## Rebuild and verify real Channel automations, Agent runs, UI, and database state
	@bash scripts/run-local-e2e.sh automation env CI=1 SOLO_E2E_REAL_AUTOMATION=1 npx playwright test e2e/automation.spec.ts --workers=1

test-e2e-budget-gate: export INTERNAL_TOKEN_SECRET := solo-budget-gate-e2e-local-internal-token
test-e2e-budget-gate: ## Rebuild and verify the real usage budget gate, UI, API, Agent runtime, and DB ledger
	@bash scripts/run-local-e2e.sh budget-gate env CI=1 SOLO_E2E_REAL_BUDGET_GATE=1 npx playwright test e2e/budget-gate.spec.ts --workers=1

test-e2e-budget-gate-run: ## Verify the budget gate against an already make-managed stack
	@cd frontend && CI=1 SOLO_E2E_REAL_BUDGET_GATE=1 npx playwright test e2e/budget-gate.spec.ts --workers=1

test-e2e-agent-session-resume: ## Rebuild and verify real Channel Session restart continuity
	@bash scripts/run-local-e2e.sh agent-session-resume env CI=1 SOLO_E2E_REAL_AGENT_DELIVERY=1 npx playwright test e2e/agent-result-delivery.spec.ts --grep "resumes the same real Channel provider Session"

test-e2e-agent-idle-resume: export AGENT_SESSION_IDLE_TTL=3s
test-e2e-agent-idle-resume: export SESSION_IDLE_SWEEP_INTERVAL=1s
test-e2e-agent-idle-resume: ## Rebuild and verify real Channel Agent idle sleep and wake
	@bash scripts/run-local-e2e.sh agent-idle-resume env CI=1 SOLO_E2E_REAL_AGENT_DELIVERY=1 SOLO_E2E_EXPECT_AGENT_IDLE_REAPER=1 npx playwright test e2e/agent-result-delivery.spec.ts --grep "sleeps an idle Channel Agent"

test-e2e-agent-scope-router: ## Rebuild and verify the real Coordinator-first Agent router
	@bash scripts/run-local-e2e.sh agent-scope-router env CI=1 SOLO_E2E_REAL_AGENT_DELIVERY=1 npx playwright test e2e/agent-result-delivery.spec.ts --grep "routes an unmentioned Channel message only to the unique Coordinator"

test-e2e-send-freshness: ## Rebuild and verify three real Agents relay through send freshness holds
	@bash scripts/run-local-e2e.sh send-freshness env CI=1 SOLO_E2E_REAL_AGENT_FRESHNESS=1 npx playwright test e2e/send-freshness.spec.ts --workers=1

test-e2e-websocket-recovery: rebuild ## Rebuild and verify browser WebSocket recovery and message backfill
	@cd frontend && CI=1 npx playwright test e2e/websocket-message-send.spec.ts e2e/websocket-recovery.spec.ts --workers=1

test-e2e-m8: export AGENT_SEND_RATE_LIMIT=5
test-e2e-m8: export AGENT_SEND_RATE_WINDOW=3s
test-e2e-m8: export AGENT_CASCADE_THRESHOLD=3
test-e2e-m8: export AGENT_CASCADE_WINDOW=10s
test-e2e-m8: export AGENT_CASCADE_COOLDOWN=5s
test-e2e-m8: ## Rebuild and verify real M8 Agent behavior
	@bash scripts/run-local-e2e.sh m8 env CI=1 SOLO_E2E_REAL_AGENT_M8=1 SOLO_E2E_REAL_AGENT_FRESHNESS=1 npx playwright test e2e/agent-message-flow.spec.ts e2e/send-freshness.spec.ts --workers=1

test-e2e-m9: ## Rebuild and verify runtime metadata and daemon-owned CLI detection
	@bash scripts/run-local-e2e.sh m9 env CI=1 SOLO_E2E_RUNTIME_DETECTION=1 npx playwright test e2e/runtime-detection.spec.ts --workers=1

test-e2e-remote-server: rebuild ## Verify pairing, reverse runtime RPC, durable offline delivery, restart recovery, UI, and DB truth
	@cd frontend && CI=1 SOLO_E2E_REMOTE_SERVER=1 npx playwright test e2e/remote-server.spec.ts --workers=1

test-e2e-workspaces: rebuild ## Verify Workspace isolation, multi-user collaboration, two local Daemons, Guest access, UI, and DB truth
	@cd frontend && CI=1 SOLO_E2E_WORKSPACES=1 npx playwright test e2e/workspace-multi-daemon.spec.ts --workers=1

test-e2e-public-remote: ## Verify real SMTP, registration, recovery, DB state, and clean-machine setup UX
	@bash scripts/test-public-remote-e2e.sh

stop: ## Shut down all services
	@bash scripts/stop-services.sh

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

test-release-install: ## Build a release archive and verify checksum-based clean installation
	@bash scripts/test-release-install.sh

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

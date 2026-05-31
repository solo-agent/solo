.PHONY: start stop restart update status logs solo solo-deploy clean

# ── Main ────────────────────────────────────────────────────────────────────

start:
	@bash scripts/start.sh

stop:
	@bash scripts/stop.sh

restart: stop start

# Rebuild binaries + deploy solo + restart
update:
	@bash scripts/stop.sh 2>/dev/null || true
	@bash scripts/start.sh

# ── Status ──────────────────────────────────────────────────────────────────

status:
	@echo "── PostgreSQL ──"
	@docker ps --filter name=solo-postgres --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' 2>/dev/null || echo "  not running"
	@echo ""
	@echo "── Server (8080) ──"
	@curl -sf http://localhost:8080/healthz && echo "" || echo "  DOWN"
	@echo "── Daemon (8081) ──"
	@lsof -ti:8081 | while read p; do ps -p $$p -o pid,etime,command 2>/dev/null; done || echo "  DOWN"

# ── Logs ────────────────────────────────────────────────────────────────────

logs-server:
	@tail -f .pids/server-run.log

logs-daemon:
	@tail -f .pids/daemon-run.log

# ── Solo CLI ────────────────────────────────────────────────────────────────

solo:
	@go build -o .pids/solo ./cmd/solo/
	@echo "solo built → .pids/solo"

solo-deploy: solo
	@for dir in $$HOME/.solo/agents/*/workspace/; do \
		cp .pids/solo "$$dir/solo" 2>/dev/null && echo "  → $$dir"; \
	done
	@echo "deployed"

# ── Database ────────────────────────────────────────────────────────────────

db-up:
	docker start solo-postgres 2>/dev/null || docker compose up -d postgres

db-down:
	docker compose down

# ── Cleanup ─────────────────────────────────────────────────────────────────

clean:
	-bash scripts/stop.sh
	@rm -rf .pids/
	@echo "Cleaned"

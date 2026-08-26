package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/auth"
)

func TestAuthRemovesClientSuppliedRuntimeIdentityHeaders(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	token, err := auth.GenerateAgentToken("550e8400-e29b-41d4-a716-446655440000", "Agent")
	if err != nil {
		t.Fatal(err)
	}

	var runID, computerID, actorType string
	handler := Auth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runID = r.Header.Get("X-Solo-Run-ID")
		computerID = r.Header.Get("X-Solo-Computer-ID")
		actorType = r.Header.Get("X-Solo-Actor-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Solo-Run-ID", "spoofed-run")
	req.Header.Set("X-Solo-Computer-ID", "spoofed-computer")
	req.Header.Set("X-Solo-Actor-Type", "agent_run")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if runID != "" || computerID != "" || actorType != "agent" {
		t.Fatalf("runtime headers = run %q computer %q actor %q", runID, computerID, actorType)
	}
}

func TestAgentRunCredentialAllowsLocalDaemonAndExpiresWithRun(t *testing.T) {
	pool := middlewareTestPool(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	ctx := context.Background()
	ownerID := uuid.NewString()
	channelID := uuid.NewString()
	agentID := uuid.NewString()
	runID := uuid.NewString()
	daemonID := "local-daemon-" + uuid.NewString()
	email := fmt.Sprintf("local-run-%s@example.test", ownerID)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, 'Local Run Test', 'test')`, ownerID, email); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = $1`, runID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`, channelID, "local-run-"+ownerID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id, name, owner_id, model_name, home_channel_id) VALUES ($1, $2, $3, 'test', $4)`, agentID, "local-agent-"+agentID[:8], ownerID, channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agent_runs (id, agent_id, trigger_type, channel_id, status, daemon_id) VALUES ($1, $2, 'message', $3, 'running', $4)`, runID, agentID, channelID, daemonID); err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateAgentRunToken(agentID, "Local Agent", runID, daemonID)
	if err != nil {
		t.Fatal(err)
	}
	handler := Auth(pool)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Solo-Run-ID") != runID || r.Header.Get("X-Solo-Computer-ID") != daemonID {
			t.Fatalf("runtime headers were not restored from the token")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Code
	}
	if status := request(); status != http.StatusNoContent {
		t.Fatalf("active local run status = %d, want %d", status, http.StatusNoContent)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET finished_at = now() WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if status := request(); status != http.StatusUnauthorized {
		t.Fatalf("finished local run status = %d, want %d", status, http.StatusUnauthorized)
	}
}

func middlewareTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

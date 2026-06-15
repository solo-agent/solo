package handler

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupTestPool creates a test DB pool from DATABASE_URL. Skips the test if
// the DB is not reachable so unit-only test runs (e.g. on CI without a DB)
// still pass cleanly.
func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping: cannot connect to test DB: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping: DB ping failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// createTestAgent inserts a test agent and returns its ID. A per-test UUID
// suffix is appended to the name to make the helper idempotent across re-runs
// (agents have a UNIQUE (owner_id, name) WHERE is_active constraint).
func createTestAgent(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]
	uniqueName := name + "-" + suffix
	ownerName := "test-owner-" + suffix

	var ownerID string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (id, display_name, email, password_hash)
		 VALUES (gen_random_uuid(), $1, $2, 'test-hash')
		 RETURNING id`,
		ownerName, "test-"+suffix+"@example.com",
	).Scan(&ownerID)
	if err != nil {
		t.Fatalf("create placeholder user: %v", err)
	}

	var id string
	err = pool.QueryRow(ctx,
		`INSERT INTO agents (id, name, owner_id, model_provider, model_name, system_prompt, is_active)
		 VALUES (gen_random_uuid(), $1, $2, 'anthropic', 'claude-sonnet-4-5', '', true)
		 RETURNING id`,
		uniqueName, ownerID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create agent %q: %v", uniqueName, err)
	}
	return id
}

// createTestRelationship inserts a relationship row directly. channelID may be nil.
func createTestRelationship(t *testing.T, pool *pgxpool.Pool, fromID, toID, relType string, channelID *string, weight float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, channel_id, weight)
		 VALUES ($1, $2, $3, $4, $5)`,
		fromID, toID, relType, channelID, weight,
	)
	if err != nil {
		t.Fatalf("create relationship %s -> %s (%s): %v", fromID, toID, relType, err)
	}
}

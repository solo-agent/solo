package service

import (
	"context"
	"os"
	"testing"

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
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("skipping: invalid DATABASE_URL: %v", err)
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
	_ = cfg
	return pool
}

// createTestAgent inserts a test agent and returns its ID. The owner_id
// references a placeholder user; if no users exist, a placeholder is created.
func createTestAgent(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	ctx := context.Background()

	// Ensure a placeholder user exists (agents.owner_id is NOT NULL → users).
	var ownerID string
	err := pool.QueryRow(ctx,
		`SELECT id FROM users WHERE display_name = $1 LIMIT 1`, "test-owner-"+name,
	).Scan(&ownerID)
	if err != nil {
		// Insert a placeholder user.
		err = pool.QueryRow(ctx,
			`INSERT INTO users (id, display_name, email, password_hash)
			 VALUES (gen_random_uuid(), $1, $2, 'test-hash')
			 RETURNING id`,
			"test-owner-"+name, "test-"+name+"@example.com",
		).Scan(&ownerID)
		if err != nil {
			t.Fatalf("create placeholder user: %v", err)
		}
	}

	var id string
	err = pool.QueryRow(ctx,
		`INSERT INTO agents (id, name, owner_id, model_provider, model_name, system_prompt, is_active)
		 VALUES (gen_random_uuid(), $1, $2, 'anthropic', 'claude-sonnet-4-5', '', true)
		 RETURNING id`,
		name, ownerID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create agent %q: %v", name, err)
	}
	return id
}

// createTestRelationship inserts a relationship row directly. channelID may be nil.
func createTestRelationship(t *testing.T, pool *pgxpool.Pool, fromID, toID, relType string, channelID *string, weight float64) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, channel_id, weight)
		 VALUES ($1, $2, $3, $4, $5)`,
		fromID, toID, relType, channelID, weight,
	)
	if err != nil {
		t.Fatalf("create relationship %s -> %s (%s): %v", fromID, toID, relType, err)
	}
}

func TestMentionValidator_AllMentionsAllowed(t *testing.T) {
	pool := setupTestPool(t)
	aID := createTestAgent(t, pool, "Alice-MV-All")
	bID := createTestAgent(t, pool, "Bob-MV-All")
	cID := createTestAgent(t, pool, "Carol-MV-All")
	createTestRelationship(t, pool, aID, bID, "assigns_to", nil, 1.0)
	createTestRelationship(t, pool, aID, cID, "assigns_to", nil, 1.0)

	svc := NewMentionValidator(pool)
	result, err := svc.Validate(context.Background(), aID, []string{bID, cID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllAllowed {
		t.Errorf("expected all allowed, got violations: %+v", result.Violations)
	}
	if len(result.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(result.Violations))
	}
}

func TestMentionValidator_OneViolation(t *testing.T) {
	pool := setupTestPool(t)
	aID := createTestAgent(t, pool, "Alice-MV-Viol")
	dID := createTestAgent(t, pool, "Dave-MV-Viol")
	// No assigns_to edge A → D.

	svc := NewMentionValidator(pool)
	result, err := svc.Validate(context.Background(), aID, []string{dID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllAllowed {
		t.Error("expected violation, got AllAllowed=true")
	}
	if len(result.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(result.Violations))
	}
}

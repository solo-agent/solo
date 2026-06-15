package service

import (
	"context"
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

	// Insert a placeholder owner user.
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

// createTestChannel inserts a channel and returns its ID. A unique suffix is
// appended to the name to satisfy the active-name uniqueness constraint.
func createTestChannel(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	// Channels require a created_by user; create a placeholder.
	var ownerID string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (id, display_name, email, password_hash)
		 VALUES (gen_random_uuid(), $1, $2, 'test-hash')
		 RETURNING id`,
		"test-ch-owner-"+suffix, "test-ch-"+suffix+"@example.com",
	).Scan(&ownerID)
	if err != nil {
		t.Fatalf("create channel owner user: %v", err)
	}

	var channelID string
	err = pool.QueryRow(ctx,
		`INSERT INTO channels (name, description, type, created_by, is_archived)
		 VALUES ($1, '', 'channel', $2, false)
		 RETURNING id`,
		"test-ch-"+suffix, ownerID,
	).Scan(&channelID)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return channelID, ownerID
}

// addChannelMember adds an agent or user as a member of a channel.
func addChannelMember(t *testing.T, pool *pgxpool.Pool, channelID, memberID, memberType string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO channel_members (channel_id, member_id, member_type, role)
		 VALUES ($1, $2, $3, 'member')
		 ON CONFLICT DO NOTHING`,
		channelID, memberID, memberType,
	)
	if err != nil {
		t.Fatalf("add channel member: %v", err)
	}
}

// createTestTask inserts a task row directly and returns its ID. parentID may
// be nil for a top-level task. Used by tests that need a known parent_task_id
// without going through the full CreateTask path.
func createTestTask(t *testing.T, pool *pgxpool.Pool, channelID, creatorID, title string, parentID *string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO tasks (id, task_number, channel_id, creator_id, title, status, priority, parent_task_id, created_at, updated_at)
		 VALUES (
		     gen_random_uuid(),
		     COALESCE((SELECT MAX(task_number)+1 FROM tasks WHERE channel_id = $1), 1),
		     $1, $2, $3, 'todo', 'none', $4, now(), now()
		 )
		 RETURNING id`,
		channelID, creatorID, title, parentID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return id
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


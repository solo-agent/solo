// Integration tests for SkillService. These hit a real Postgres instance —
// the tests are skipped if DATABASE_URL is not set or the DB is unreachable.
// Use the same DB the dev server uses (solo-postgres on localhost:5432).
//
// Run with:
//   DATABASE_URL=postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable \
//     go test ./internal/server/service/ -run TestSkill -count=1
//
// Each test uses a unique agent + skill name to avoid clashing with other
// test runs; the cleanup function deletes everything it created.

package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"
)

const testDBURL = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"

// newTestPool opens a pool to the test DB or skips the test.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = testDBURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("DATABASE_URL not reachable, skipping integration test: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("DATABASE_URL not pingable, skipping integration test: %v", err)
	}
	return pool
}

// seedUserAndAgent creates a user + agent row and returns their IDs. The
// agent's owner_id is the user, matching the production schema constraint.
// Returns cleanup func that removes both rows.
func seedUserAndAgent(t *testing.T, pool *pgxpool.Pool) (userID, agentID string, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	userID = uuid.NewString()
	agentID = uuid.NewString()
	now := time.Now()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, 'test-hash', $4, $4)
	`, userID, userID+"@test.local", "Test User "+userID[:8], now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, name, owner_id, model_provider, model_name,
		                   system_prompt, temperature, max_tokens, is_active, auto_join)
		VALUES ($1, $2, $3, 'anthropic', 'claude-sonnet', '', 0.7, 4096, true, false)
	`, agentID, "test-agent-"+agentID[:8], userID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return userID, agentID, func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
}

// seedSkill inserts a row into the skills table. Returns cleanup.
func seedSkill(t *testing.T, pool *pgxpool.Pool, name string) (skillID string, cleanup func()) {
	t.Helper()
	skillID = uuid.NewString()
	now := time.Now()
	srcPath := "/tmp/" + name
	_, err := pool.Exec(context.Background(), `
		INSERT INTO skills (id, name, description, source_path, source_kind, body, body_hash,
		                   discovered_at, updated_at)
		VALUES ($1, $2, '', $3, 'user-test', '', repeat('a', 64), $4, $4)
	`, skillID, name, srcPath, now)
	if err != nil {
		t.Fatalf("seed skill %q: %v", name, err)
	}
	return skillID, func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM skills WHERE id = $1`, skillID)
	}
}

func TestSkillService_GetByID_NotFoundReturnsErrSkillNotFound(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	_, err := svc.GetByID(context.Background(), uuid.NewString())
	if err == nil {
		t.Fatal("expected error for missing skill, got nil")
	}
	if !isErrSkillNotFound(err) {
		t.Fatalf("expected ErrSkillNotFound sentinel, got %v (%T)", err, err)
	}
}

func TestSkillService_GetByID_ExistingSkillReturnsRow(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	want := "test-get-byid-" + uuid.NewString()[:8]
	id, cleanup := seedSkill(t, pool, want)
	defer cleanup()
	got, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Name != want {
		t.Fatalf("expected name %q, got %q", want, got.Name)
	}
	if got.BodyHash == "" {
		t.Fatal("expected non-empty BodyHash")
	}
}

func TestSkillService_SetAgentSkills_RejectsUnknownSkillID(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	_, agentID, agentCleanup := seedUserAndAgent(t, pool)
	defer agentCleanup()

	// Set with a bogus skill UUID: should fail with ErrSkillNotFound
	// and the (non-existent) bindings should be left untouched.
	bogus := uuid.NewString()
	_, err := svc.SetAgentSkills(context.Background(), agentID, []string{bogus})
	if err == nil {
		t.Fatal("expected error for unknown skill id, got nil")
	}
	if !isErrSkillNotFound(err) {
		t.Fatalf("expected ErrSkillNotFound wrapping, got %v", err)
	}
	// Confirm no agent_skills row was inserted (transaction was rolled back).
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_skills WHERE agent_id = $1`, agentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 bindings after rejected set, got %d", count)
	}
}

func TestSkillService_SetAgentSkills_EmptySkillIDsClearsAll(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	_, agentID, agentCleanup := seedUserAndAgent(t, pool)
	defer agentCleanup()
	skillID, skillCleanup := seedSkill(t, pool, "test-empty-clear-"+uuid.NewString()[:8])
	defer skillCleanup()
	// Pre-bind one skill.
	if _, err := svc.SetAgentSkills(context.Background(), agentID, []string{skillID}); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	// Clear via empty list.
	out, err := svc.SetAgentSkills(context.Background(), agentID, []string{})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty after clear, got %d bindings", len(out))
	}
}

func TestSkillService_SetAgentSkills_BatchValidatesAllIDs(t *testing.T) {
	// N valid + 1 bogus: must reject the whole transaction (no partial
	// inserts) — even if the bogus is at the end. This is the "all-or-nothing"
	// semantic the N+1 fix introduced.
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	_, agentID, agentCleanup := seedUserAndAgent(t, pool)
	defer agentCleanup()
	aID, aCleanup := seedSkill(t, pool, "test-batch-a-"+uuid.NewString()[:6])
	bID, bCleanup := seedSkill(t, pool, "test-batch-b-"+uuid.NewString()[:6])
	defer aCleanup()
	defer bCleanup()

	_, err := svc.SetAgentSkills(context.Background(), agentID,
		[]string{aID, bID, uuid.NewString()})
	if err == nil {
		t.Fatal("expected error when 1 of 3 IDs is bogus")
	}
	// Transaction must be rolled back: agent has no bindings.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_skills WHERE agent_id = $1`, agentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("transaction should be atomic; expected 0 bindings, got %d", count)
	}
}

func TestSkillService_SetAgentSkills_HappyPath(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	svc := NewSkillService(pool)
	_, agentID, agentCleanup := seedUserAndAgent(t, pool)
	defer agentCleanup()
	aID, aCleanup := seedSkill(t, pool, "test-happy-a-"+uuid.NewString()[:6])
	bID, bCleanup := seedSkill(t, pool, "test-happy-b-"+uuid.NewString()[:6])
	defer aCleanup()
	defer bCleanup()

	out, err := svc.SetAgentSkills(context.Background(), agentID, []string{aID, bID})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(out))
	}
}

// isErrSkillNotFound is a test-local helper that matches our ErrSkillNotFound
// sentinel (and a wrapped form). Avoids importing errors just for one call.
func isErrSkillNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrSkillNotFound ||
		containsAll(err.Error(), "skill not found")
}

func containsAll(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

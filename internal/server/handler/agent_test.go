package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
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

func setupMentionCandidatesRouter(h *AgentHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/agents/{agentID}/mention-candidates", h.ListMentionCandidates)
	return r
}

func setupCollaboratorsRouter(h *AgentHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/agents/{agentID}/collaborators", h.ListCollaborators)
	return r
}

// TestMentionCandidates_ReturnsAssignsToSortedByWeight verifies that the
// mention-candidates endpoint returns only assigns_to edges from the given
// agent, ordered by weight DESC then name ASC. (T1.4.1)
func TestMentionCandidates_ReturnsAssignsToSortedByWeight(t *testing.T) {
	pool := setupTestPool(t)

	fromID := createTestAgent(t, pool, "Delegator")
	bobID := createTestAgent(t, pool, "Bob")
	carolID := createTestAgent(t, pool, "Carol")
	daveID := createTestAgent(t, pool, "Dave")
	eveID := createTestAgent(t, pool, "Eve")

	// Two assigns_to edges, with different weights. Dave (weight 5) should
	// sort above Bob (weight 1).
	createTestRelationship(t, pool, fromID, bobID, "assigns_to", nil, 1.0)
	createTestRelationship(t, pool, fromID, daveID, "assigns_to", nil, 5.0)
	// One collaborates_with edge that must NOT show up in the result.
	// (rel_type constraint allows only 'assigns_to' | 'collaborates_with'.)
	createTestRelationship(t, pool, fromID, carolID, "collaborates_with", nil, 99.0)
	// An incoming edge from another agent — also must not show up (we only
	// return candidates for the *outgoing* assigns_to direction).
	otherID := createTestAgent(t, pool, "Other")
	createTestRelationship(t, pool, otherID, eveID, "assigns_to", nil, 99.0)

	h := &AgentHandler{pool: pool}
	r := setupMentionCandidatesRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/agents/"+fromID+"/mention-candidates", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var got []MentionCandidate
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rr.Body.String())
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 mention candidates (assigns_to only), got %d: %+v", len(got), got)
	}

	// First entry must be the higher-weight target (Dave, weight 5).
	if got[0].ID != daveID {
		t.Errorf("expected highest-weight candidate first (Dave id=%s), got id=%s (name=%s)", daveID, got[0].ID, got[0].Name)
	}
	if got[0].Weight != 5.0 {
		t.Errorf("expected first weight 5.0, got %v", got[0].Weight)
	}
	// Second entry must be the lower-weight target (Bob, weight 1).
	if got[1].ID != bobID {
		t.Errorf("expected second candidate to be Bob id=%s, got id=%s (name=%s)", bobID, got[1].ID, got[1].Name)
	}
	if got[1].Weight != 1.0 {
		t.Errorf("expected second weight 1.0, got %v", got[1].Weight)
	}
}

// TestMentionCandidates_TieBreakByName verifies the secondary sort: when two
// assigns_to edges have the same weight, candidates are ordered by name ASC.
func TestMentionCandidates_TieBreakByName(t *testing.T) {
	pool := setupTestPool(t)

	fromID := createTestAgent(t, pool, "Delegator")
	zetaID := createTestAgent(t, pool, "Zeta")
	alphaID := createTestAgent(t, pool, "Alpha")

	// Same weight on both — name should break the tie.
	createTestRelationship(t, pool, fromID, zetaID, "assigns_to", nil, 3.0)
	createTestRelationship(t, pool, fromID, alphaID, "assigns_to", nil, 3.0)

	h := &AgentHandler{pool: pool}
	r := setupMentionCandidatesRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/agents/"+fromID+"/mention-candidates", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var got []MentionCandidate
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0].ID != alphaID {
		t.Errorf("expected Alpha first on name tie-break, got id=%s", got[0].ID)
	}
	if got[1].ID != zetaID {
		t.Errorf("expected Zeta second on name tie-break, got id=%s", got[1].ID)
	}
}

// TestMentionCandidates_EmptyWhenNoEdges verifies an agent with no
// assigns_to edges returns an empty list (200 OK, []), not null.
func TestMentionCandidates_EmptyWhenNoEdges(t *testing.T) {
	pool := setupTestPool(t)
	loneID := createTestAgent(t, pool, "Lone")

	h := &AgentHandler{pool: pool}
	r := setupMentionCandidatesRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/agents/"+loneID+"/mention-candidates", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if body != "[]\n" {
		t.Errorf("expected empty JSON array body, got %q", body)
	}
}

// TestCollaborators_ReturnsBidirectional verifies the collaborators endpoint
// returns collaborates_with partners regardless of edge direction, ordered
// by weight DESC then name ASC. (T1.5.1)
//
// Setup:
//   - Alice --collaborates_with (weight 3)--> Bob      (Alice is from)
//   - Carol --collaborates_with (weight 1)--> Alice    (Alice is to)
//
// Querying for Alice must return both Bob and Carol.
func TestCollaborators_ReturnsBidirectional(t *testing.T) {
	pool := setupTestPool(t)

	aliceID := createTestAgent(t, pool, "Alice")
	bobID := createTestAgent(t, pool, "Bob")
	carolID := createTestAgent(t, pool, "Carol")
	daveID := createTestAgent(t, pool, "Dave")

	// Alice -> Bob (Alice is from).
	createTestRelationship(t, pool, aliceID, bobID, "collaborates_with", nil, 3.0)
	// Carol -> Alice (Alice is to).
	createTestRelationship(t, pool, carolID, aliceID, "collaborates_with", nil, 1.0)
	// An unrelated collab edge — must NOT show up when querying for Alice.
	createTestRelationship(t, pool, bobID, daveID, "collaborates_with", nil, 99.0)
	// An assigns_to edge touching Alice — must NOT show up (rel_type filter).
	createTestRelationship(t, pool, aliceID, daveID, "assigns_to", nil, 99.0)

	h := &AgentHandler{pool: pool}
	r := setupCollaboratorsRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/agents/"+aliceID+"/collaborators", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var got []MentionCandidate
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rr.Body.String())
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 collaborators (Bob and Carol), got %d: %+v", len(got), got)
	}

	// Bob has weight 3 → must be first.
	if got[0].ID != bobID {
		t.Errorf("expected Bob first (weight 3), got id=%s (name=%s)", got[0].ID, got[0].Name)
	}
	if got[0].Weight != 3.0 {
		t.Errorf("expected first weight 3.0, got %v", got[0].Weight)
	}
	// Carol has weight 1 → must be second.
	if got[1].ID != carolID {
		t.Errorf("expected Carol second (weight 1), got id=%s (name=%s)", got[1].ID, got[1].Name)
	}
	if got[1].Weight != 1.0 {
		t.Errorf("expected second weight 1.0, got %v", got[1].Weight)
	}
}

// TestCollaborators_EmptyWhenNoEdges verifies an agent with no
// collaborates_with edges returns an empty list (200 OK, []), not null.
func TestCollaborators_EmptyWhenNoEdges(t *testing.T) {
	pool := setupTestPool(t)
	loneID := createTestAgent(t, pool, "Lone")

	h := &AgentHandler{pool: pool}
	r := setupCollaboratorsRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/agents/"+loneID+"/collaborators", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if body != "[]\n" {
		t.Errorf("expected empty JSON array body, got %q", body)
	}
}

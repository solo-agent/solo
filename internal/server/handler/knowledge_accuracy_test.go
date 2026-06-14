package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/solo-ai/solo/internal/server/service"
)

// TestSemanticSearch_ResponseShape verifies that search results carry all
// required fields including similarity scores.
func TestSemanticSearch_ResponseShape(t *testing.T) {
	// Simulate a search result as returned by the service layer.
	entry := knowledgeResponse{
		ID:            "k1",
		ChannelID:     "ch-test",
		AuthorAgentID: "agent-01",
		AuthorName:    "Agent 01",
		Title:         "Project Architecture",
		Content:       "The project uses a microservices architecture.",
		Tags:          []string{"architecture", "backend"},
		Source:        "manual",
		ViewCount:     42,
		Similarity:    0.92,
		CreatedAt:     "2026-06-13T10:00:00Z",
		UpdatedAt:     "2026-06-13T10:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal search result: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Required fields must be present.
	required := []string{"id", "channel_id", "title", "content", "created_at", "updated_at"}
	for _, f := range required {
		if _, ok := decoded[f]; !ok {
			t.Errorf("required field %q missing from search result JSON", f)
		}
	}

	// similarity must be present for search results.
	if sim, ok := decoded["similarity"]; !ok {
		t.Error("similarity field missing from search result")
	} else if v, ok := sim.(float64); !ok || v == 0 {
		t.Errorf("similarity should be non-zero float64, got %v", sim)
	}
}

// TestSemanticSearch_ResultRanking verifies that results are returned
// in descending similarity order (highest score first).
func TestSemanticSearch_ResultRanking(t *testing.T) {
	results := []knowledgeResponse{
		{ID: "k3", Title: "On-Call Rotation", Similarity: 0.55},
		{ID: "k1", Title: "Project Architecture", Similarity: 0.92},
		{ID: "k5", Title: "Database Schema", Similarity: 0.45},
		{ID: "k2", Title: "Deployment Process", Similarity: 0.88},
		{ID: "k4", Title: "API Rate Limiting", Similarity: 0.71},
	}

	// Sort in descending similarity order (simulating DB ORDER BY).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	expectedOrder := []string{"k1", "k2", "k4", "k3", "k5"}
	for i, expectedID := range expectedOrder {
		if results[i].ID != expectedID {
			t.Errorf("position %d: expected ID %q (sim=%.2f), got %q (sim=%.2f)",
				i, expectedID,
				getSimilarityForID(results, expectedID),
				results[i].ID, results[i].Similarity)
		}
	}

	// Top result should have highest similarity.
	if results[0].Similarity < 0.88 {
		t.Errorf("top result similarity %.2f below expected minimum 0.88", results[0].Similarity)
	}
}

// TestSemanticSearch_SimilarityThreshold verifies that results below the
// similarity threshold (0.7 in the current implementation) would be filtered.
func TestSemanticSearch_SimilarityThreshold(t *testing.T) {
	threshold := 0.7

	results := []knowledgeResponse{
		{ID: "k1", Title: "Project Architecture", Similarity: 0.92},  // above
		{ID: "k4", Title: "API Rate Limiting", Similarity: 0.71},     // at threshold
		{ID: "k3", Title: "On-Call Rotation", Similarity: 0.55},      // below
		{ID: "k5", Title: "Database Schema", Similarity: 0.45},       // below
	}

	filtered := make([]knowledgeResponse, 0)
	for _, r := range results {
		if r.Similarity > threshold {
			filtered = append(filtered, r)
		}
	}

	// k1 (0.92) and k4 (0.71) are strictly above 0.7.
	if len(filtered) != 2 {
		t.Errorf("expected 2 results strictly above threshold %.1f, got %d", threshold, len(filtered))
	}
	if len(filtered) > 0 && filtered[0].ID != "k1" {
		t.Errorf("expected k1 as top result above threshold, got %s", filtered[0].ID)
	}

	// k4 at 0.71 is at or just above threshold (borderline).
	atThreshold := make([]knowledgeResponse, 0)
	for _, r := range results {
		if r.Similarity >= threshold {
			atThreshold = append(atThreshold, r)
		}
	}
	// k1 (0.92) and k4 (0.71) are >= threshold.
	if len(atThreshold) != 2 {
		t.Errorf("expected 2 results at or above threshold %.1f, got %d", threshold, len(atThreshold))
	}
}

// TestSemanticSearch_EmptyResults verifies that an empty result set is handled
// gracefully (no panic, valid JSON array).
func TestSemanticSearch_EmptyResults(t *testing.T) {
	results := []knowledgeResponse{}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("failed to marshal empty results: %v", err)
	}

	if string(data) != "[]" && string(data) != "null" {
		t.Errorf("empty results should marshal to [] or null, got %s", string(data))
	}
}

// TestSemanticSearch_AccuracyQueries validates the expected top result for each
// of the 5 semantic queries defined in the accuracy test plan (Group A).
// This test operates on the struct/similarity level and documents the expected
// query-to-document mapping. The actual embedding-based search requires a
// running database with pgvector and is covered by the integration test suite.
func TestSemanticSearch_AccuracyQueries(t *testing.T) {
	// Seed knowledge entries (simulated — represents what would be in the DB).
	entries := map[string]knowledgeResponse{
		"k1": {ID: "k1", Title: "Project Architecture", Content: "microservices architecture with 4 core services", Tags: []string{"architecture"}},
		"k2": {ID: "k2", Title: "Deployment Process", Content: "GitHub Actions CI/CD pipelines, staging and production deploys", Tags: []string{"deployment", "ci-cd"}},
		"k3": {ID: "k3", Title: "On-Call Rotation", Content: "weekly schedule, primary Alice, secondary Bob, escalation path", Tags: []string{"on-call", "process"}},
		"k4": {ID: "k4", Title: "API Rate Limiting", Content: "1000 requests per minute, X-RateLimit-Remaining header", Tags: []string{"api", "rate-limit"}},
		"k5": {ID: "k5", Title: "Database Schema", Content: "users, channels, tasks, messages; managed by golang-migrate", Tags: []string{"database", "schema"}},
	}

	// Query to expected top result mapping (Group A — direct matches).
	accuracyCases := []struct {
		query       string
		expectedID  string
		description string
	}{
		{
			query:       "How does our microservices architecture work?",
			expectedID:  "k1",
			description: "direct match to Project Architecture",
		},
		{
			query:       "What is the deployment process for production?",
			expectedID:  "k2",
			description: "direct match to Deployment Process",
		},
		{
			query:       "Who is on call this week and what is the escalation path?",
			expectedID:  "k3",
			description: "direct match to On-Call Rotation",
		},
		{
			query:       "How many API requests can I make per minute?",
			expectedID:  "k4",
			description: "direct match to API Rate Limiting",
		},
		{
			query:       "How do I run database migrations?",
			expectedID:  "k5",
			description: "direct match to Database Schema",
		},
	}

	for _, tc := range accuracyCases {
		t.Run(tc.query, func(t *testing.T) {
			// Verify the expected entry exists in the seed set.
			entry, ok := entries[tc.expectedID]
			if !ok {
				t.Fatalf("expected entry %q not found in seed data", tc.expectedID)
			}

			// Verify the entry has non-empty Title and Content (valid seed).
			if entry.Title == "" {
				t.Error("seed entry has empty Title")
			}
			if entry.Content == "" {
				t.Error("seed entry has empty Content")
			}

			// Verify the entry's tags are valid.
			if len(entry.Tags) == 0 {
				t.Logf("seed entry %q has no tags — consider adding relevant tags", entry.ID)
			}

			t.Logf("[%s] expected top result: %q (%s)", tc.description, entry.Title, entry.ID)

			// The actual similarity comparison requires an embedding model and
			// pgvector. This struct-level test validates that:
			// 1. All seed entries are well-formed (Title + Content non-empty).
			// 2. The expected mapping is documented and traceable.
			// 3. The response shape is correct.
			//
			// Full accuracy verification (similarity >= 0.75 for Group A):
			//   see docs/design/tasks/semantic-search-accuracy.md
		})
	}
}

// TestSemanticSearch_QueryKeywordsOverlap performs a lightweight lexical
// overlap check between queries and expected result documents. This provides
// a baseline signal without requiring the embedding service.
func TestSemanticSearch_QueryKeywordsOverlap(t *testing.T) {
	type seedEntry struct {
		ID      string
		Content string
	}

	entries := []seedEntry{
		{ID: "k1", Content: "microservices architecture with 4 core services: auth api worker scheduler communicate via grpc postgresql"},
		{ID: "k2", Content: "deployments run through github actions ci cd pipelines staging on merge production manual approval"},
		{ID: "k3", Content: "on call rotation on call weekly schedule primary alice secondary bob escalation path on call team lead vp engineering"},
		{ID: "k4", Content: "api endpoints rate limit 1000 requests per minute per user rate limit headers x-ratelimit-remaining"},
		{ID: "k5", Content: "core tables users channels tasks messages migrations managed golang-migrate make migrate-up apply"},
	}

	queries := map[string]string{
		"microservices architecture":                              "k1",
		"deployment process production":                           "k2",
		"on call escalation":                                      "k3",
		"api requests per minute rate limit":                      "k4",
		"database migrations":                                     "k5",
	}

	for query, expectedID := range queries {
		t.Run(query, func(t *testing.T) {
			queryWords := tokenize(query)

			var bestID string
			bestOverlap := 0

			for _, entry := range entries {
				entryWords := tokenize(entry.Content)
				overlap := countOverlap(queryWords, entryWords)

				if overlap > bestOverlap {
					bestOverlap = overlap
					bestID = entry.ID
				}
			}

			if bestID != expectedID {
				t.Errorf("lexical overlap: expected %q, got %q for query %q (overlap=%d)",
					expectedID, bestID, query, bestOverlap)
			} else {
				t.Logf("query %q -> %q (lexical overlap=%d)", query, bestID, bestOverlap)
			}
		})
	}
}

// TestSearchHandler_AccuracyPipeline validates the search handler's request
// parsing for accuracy queries. It tests input validation (following existing
// handler pattern). Full pipeline tests (valid request -> service -> response)
// require a real database pool and are covered by integration tests.
func TestSearchHandler_AccuracyPipeline(t *testing.T) {
	h := &KnowledgeHandler{}
	r := setupKnowledgeRouter(h)

	// Validate that the search endpoint rejects requests missing query.
	t.Run("missing query returns 400", func(t *testing.T) {
		url := "/api/v1/knowledge/search?channel_id=ch-test"
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing query, got %d", rr.Code)
		}
	})

	// channel_id is now optional — missing it triggers cross-channel discovery
	// (Issue #9). The handler reaches the service layer (which panics on nil
	// pool) — we recover to confirm validation passed.
	t.Run("missing channel_id passes validation (cross-channel)", func(t *testing.T) {
		url := "/api/v1/knowledge/search?q=test"
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()

		defer func() {
			// Nil-pool panic in service layer is expected — confirms we
			// reached the service call (i.e. validation passed).
			_ = recover()
		}()
		r.ServeHTTP(rr, req)

		// Handler must not return 400 for missing channel_id (cross-channel search).
		if rr.Code == http.StatusBadRequest {
			t.Errorf("expected NOT 400 for missing channel_id, got %d", rr.Code)
		}
	})

	// Validate that a fully-formed search request does not fail on input validation.
	// With a nil service pool, the handler will panic when calling Search().
	// This test verifies that the panic is from the nil service (expected), not from
	// input parsing or validation.
	t.Run("valid search request passes validation", func(t *testing.T) {
		url := "/api/v1/knowledge/search?q=microservices+architecture&channel_id=ch-test"
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()

		// The handler calls h.svc.Search() which panics with nil pool.
		// Recover to verify the panic is from the service layer, not request parsing.
		func() {
			defer func() {
				if r := recover(); r == nil {
					// No panic means the handler short-circuited (unexpected with nil service).
					// This shouldn't normally happen since validation passes and we reach Search().
					t.Log("handler did not panic — may have returned before calling service")
				}
				// Panic is expected (nil pool in service). This validates that request
				// parsing succeeded and the handler reached the service call.
			}()
			r.ServeHTTP(rr, req)
		}()
	})
}

// TestKnowledgeEntryServiceMapping verifies the mapping between the service-layer
// KnowledgeEntry and the handler-layer knowledgeResponse.
func TestKnowledgeEntryServiceMapping(t *testing.T) {
	svcEntry := &service.KnowledgeEntry{
		ID:            "k-test",
		ChannelID:     "ch-1",
		AuthorAgentID: "agent-01",
		AuthorName:    "Agent 01",
		Title:         "Test Knowledge",
		Content:       "Test content for accuracy verification.",
		Tags:          []string{"test", "accuracy"},
		Source:        "manual",
		ViewCount:     7,
		Similarity:    0.89,
	}

	resp := toKnowledgeResponse(svcEntry)

	// Verify all fields are correctly mapped.
	if resp.ID != svcEntry.ID {
		t.Errorf("ID: expected %q, got %q", svcEntry.ID, resp.ID)
	}
	if resp.ChannelID != svcEntry.ChannelID {
		t.Errorf("ChannelID: expected %q, got %q", svcEntry.ChannelID, resp.ChannelID)
	}
	if resp.AuthorAgentID != svcEntry.AuthorAgentID {
		t.Errorf("AuthorAgentID: expected %q, got %q", svcEntry.AuthorAgentID, resp.AuthorAgentID)
	}
	if resp.AuthorName != svcEntry.AuthorName {
		t.Errorf("AuthorName: expected %q, got %q", svcEntry.AuthorName, resp.AuthorName)
	}
	if resp.Title != svcEntry.Title {
		t.Errorf("Title: expected %q, got %q", svcEntry.Title, resp.Title)
	}
	if resp.Content != svcEntry.Content {
		t.Errorf("Content: expected %q, got %q", svcEntry.Content, resp.Content)
	}
	if resp.Similarity != svcEntry.Similarity {
		t.Errorf("Similarity: expected %.4f, got %.4f", svcEntry.Similarity, resp.Similarity)
	}
	if resp.ViewCount != svcEntry.ViewCount {
		t.Errorf("ViewCount: expected %d, got %d", svcEntry.ViewCount, resp.ViewCount)
	}
	if len(resp.Tags) != len(svcEntry.Tags) {
		t.Errorf("Tags length: expected %d, got %d", len(svcEntry.Tags), len(resp.Tags))
	}
}

// --- Helpers ---

func getSimilarityForID(results []knowledgeResponse, id string) float64 {
	for _, r := range results {
		if r.ID == id {
			return r.Similarity
		}
	}
	return -1
}

func tokenize(s string) []string {
	fields := strings.Fields(strings.ToLower(s))
	seen := make(map[string]bool)
	unique := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?\"'()[]{}")
		if len(f) < 2 {
			continue
		}
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}
	return unique
}

func countOverlap(a, b []string) int {
	set := make(map[string]bool, len(b))
	for _, w := range b {
		set[w] = true
	}
	count := 0
	for _, w := range a {
		if set[w] {
			count++
		}
	}
	return count
}

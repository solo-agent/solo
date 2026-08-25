package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestAgentHandlerGetReturnsLegacyAgentWithoutHomeChannel(t *testing.T) {
	pool := handlerTestPool(t)
	ctx := context.Background()
	ownerID := uuid.NewString()
	agentID := uuid.NewString()
	email := fmt.Sprintf("legacy-agent-%s@example.test", ownerID)

	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, 'Legacy Agent Owner', 'test')`,
		ownerID, email,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO agents (id, name, owner_id, model_name) VALUES ($1, $2, $3, 'test')`,
		agentID, "legacy-agent-"+agentID[:8], ownerID,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("X-User-ID", ownerID)
	route := chi.NewRouteContext()
	route.URLParams.Add("agentID", agentID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	recorder := httptest.NewRecorder()

	(&AgentHandler{pool: pool}).Get(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response AgentResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != agentID || response.HomeChannelID != "" {
		t.Fatalf("response = %+v, want legacy Agent %s with empty home_channel_id", response, agentID)
	}
}

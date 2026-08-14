package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

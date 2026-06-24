package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

func setupArtifactRouter(h *ArtifactHandler) chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/tasks/{taskID}/artifact", h.GenerateLatest)
	r.Post("/api/v1/tasks/{taskID}/artifact/finalize", h.Finalize)
	r.Post("/api/v1/tasks/{taskID}/artifact/publish", h.Publish)
	r.Get("/api/v1/tasks/{taskID}/artifacts", h.List)
	r.Get("/api/v1/tasks/{taskID}/artifact/latest", h.Latest)
	r.Get("/api/v1/artifacts/{artifactID}", h.Serve)
	return r
}

func TestArtifactHandler_MissingAuth(t *testing.T) {
	h := NewArtifactHandler(service.NewArtifactService(nil, ""))
	r := setupArtifactRouter(h)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/tasks/task-1/artifact"},
		{http.MethodPost, "/api/v1/tasks/task-1/artifact/finalize"},
		{http.MethodPost, "/api/v1/tasks/task-1/artifact/publish"},
		{http.MethodGet, "/api/v1/tasks/task-1/artifacts"},
		{http.MethodGet, "/api/v1/tasks/task-1/artifact/latest"},
		{http.MethodGet, "/api/v1/artifacts/artifact-1"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d", tc.method, tc.path, rr.Code)
		}
	}
}

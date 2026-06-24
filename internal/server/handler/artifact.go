package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/service"
)

type ArtifactHandler struct {
	svc *service.ArtifactService
}

type artifactPublishRequest struct {
	Mode string `json:"mode"`
	HTML string `json:"html"`
}

func NewArtifactHandler(svc *service.ArtifactService) *ArtifactHandler {
	return &ArtifactHandler{svc: svc}
}

func (h *ArtifactHandler) GenerateLatest(w http.ResponseWriter, r *http.Request) {
	h.generate(w, r, false)
}

func (h *ArtifactHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	h.generate(w, r, true)
}

func (h *ArtifactHandler) Publish(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	var req artifactPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mode == "" {
		req.Mode = "latest"
	}
	if req.Mode != "latest" && req.Mode != "final" {
		writeError(w, http.StatusBadRequest, "invalid artifact mode")
		return
	}
	if strings.TrimSpace(req.HTML) == "" {
		writeError(w, http.StatusBadRequest, "artifact html is required")
		return
	}
	artifact, err := h.svc.Publish(r.Context(), taskID, userID, req.Mode, req.HTML)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

func (h *ArtifactHandler) Latest(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "latest"
	}
	if mode != "latest" && mode != "final" {
		writeError(w, http.StatusBadRequest, "invalid artifact mode")
		return
	}
	artifact, err := h.svc.LatestMode(r.Context(), taskID, userID, mode)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (h *ArtifactHandler) Serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	artifactID := chi.URLParam(r, "artifactID")
	if artifactID == "" {
		writeError(w, http.StatusBadRequest, "artifact ID is required")
		return
	}
	artifact, err := h.svc.Get(r.Context(), artifactID, userID)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, artifact.HTMLPath)
}

func (h *ArtifactHandler) generate(w http.ResponseWriter, r *http.Request, final bool) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task ID is required")
		return
	}
	var artifact *service.Artifact
	var err error
	if final {
		artifact, err = h.svc.Finalize(r.Context(), taskID, userID)
	} else {
		artifact, err = h.svc.GenerateLatest(r.Context(), taskID, userID)
	}
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

func writeArtifactError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrTaskNotFound):
		writeError(w, http.StatusNotFound, "artifact not found")
	case errors.Is(err, service.ErrTaskNotChannelMember):
		writeError(w, http.StatusForbidden, "not a channel member")
	case errors.Is(err, service.ErrTaskNotClaimer), errors.Is(err, service.ErrTaskNotCreator):
		writeError(w, http.StatusForbidden, "not allowed to publish artifact")
	default:
		writeError(w, http.StatusInternalServerError, "failed to handle artifact")
	}
}

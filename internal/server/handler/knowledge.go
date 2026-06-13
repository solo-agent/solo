package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

// KnowledgeHandler handles knowledge-related HTTP requests.
type KnowledgeHandler struct {
	pool *pgxpool.Pool
	svc  *service.KnowledgeService
}

// NewKnowledgeHandler creates a new KnowledgeHandler.
func NewKnowledgeHandler(pool *pgxpool.Pool, svc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{pool: pool, svc: svc}
}

// knowledgeResponse is the JSON shape returned by knowledge endpoints.
type knowledgeResponse struct {
	ID            string   `json:"id"`
	ChannelID     string   `json:"channel_id"`
	AuthorAgentID string   `json:"author_agent_id"`
	AuthorName    string   `json:"author_name,omitempty"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	Source        string   `json:"source"`
	SourceRef     string   `json:"source_ref,omitempty"`
	ViewCount     int      `json:"view_count"`
	Similarity    float64  `json:"similarity,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func toKnowledgeResponse(e *service.KnowledgeEntry) knowledgeResponse {
	r := knowledgeResponse{
		ID:            e.ID,
		ChannelID:     e.ChannelID,
		AuthorAgentID: e.AuthorAgentID,
		AuthorName:    e.AuthorName,
		Title:         e.Title,
		Content:       e.Content,
		Tags:          e.Tags,
		Source:        e.Source,
		SourceRef:     e.SourceRef,
		ViewCount:     e.ViewCount,
		Similarity:    e.Similarity,
		CreatedAt:     e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     e.UpdatedAt.Format(time.RFC3339),
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return r
}

// List handles GET /api/v1/knowledge
func (h *KnowledgeHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = userID

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id query parameter is required")
		return
	}

	tag := r.URL.Query().Get("tag")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	entries, err := h.svc.List(r.Context(), channelID, tag, limit, offset)
	if err != nil {
		slog.Error("failed to list knowledge", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list knowledge entries")
		return
	}

	resp := make([]knowledgeResponse, len(entries))
	for i, e := range entries {
		resp[i] = toKnowledgeResponse(&e)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Create handles POST /api/v1/knowledge
func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req service.CreateKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}

	entry, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		slog.Error("failed to create knowledge entry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create knowledge entry")
		return
	}

	writeJSON(w, http.StatusCreated, toKnowledgeResponse(entry))
}

// Get handles GET /api/v1/knowledge/{id}
func (h *KnowledgeHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "knowledge ID is required")
		return
	}

	entry, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if err.Error() == "knowledge entry not found" {
			writeError(w, http.StatusNotFound, "knowledge entry not found")
			return
		}
		slog.Error("failed to get knowledge entry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get knowledge entry")
		return
	}

	writeJSON(w, http.StatusOK, toKnowledgeResponse(entry))
}

// Update handles PATCH /api/v1/knowledge/{id}
func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "knowledge ID is required")
		return
	}

	var req service.UpdateKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entry, err := h.svc.Update(r.Context(), id, userID, req)
	if err != nil {
		switch {
		case err.Error() == "knowledge entry not found":
			writeError(w, http.StatusNotFound, "knowledge entry not found")
		case err.Error() == "only the author can update this entry":
			writeError(w, http.StatusForbidden, "only the author can update this entry")
		default:
			slog.Error("failed to update knowledge entry", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update knowledge entry")
		}
		return
	}

	writeJSON(w, http.StatusOK, toKnowledgeResponse(entry))
}

// Delete handles DELETE /api/v1/knowledge/{id}
func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "knowledge ID is required")
		return
	}

	if err := h.svc.Delete(r.Context(), id, userID); err != nil {
		switch {
		case err.Error() == "knowledge entry not found":
			writeError(w, http.StatusNotFound, "knowledge entry not found")
		case err.Error() == "only the author can delete this entry":
			writeError(w, http.StatusForbidden, "only the author can delete this entry")
		default:
			slog.Error("failed to delete knowledge entry", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete knowledge entry")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Search handles GET /api/v1/knowledge/search?q=...&channel_id=...
func (h *KnowledgeHandler) Search(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "search query 'q' is required")
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id query parameter is required")
		return
	}
	topK, _ := strconv.Atoi(r.URL.Query().Get("top_k"))
	if topK <= 0 {
		topK = 5
	}

	entries, err := h.svc.Search(r.Context(), channelID, query, topK)
	if err != nil {
		slog.Error("failed to search knowledge", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to search knowledge")
		return
	}

	resp := make([]knowledgeResponse, len(entries))
	for i, e := range entries {
		resp[i] = toKnowledgeResponse(&e)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": resp,
		"total":   len(resp),
	})
}

// ImportDecisions handles POST /api/v1/knowledge/import-decisions
func (h *KnowledgeHandler) ImportDecisions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}

	count, err := h.svc.ImportFromDecisions(r.Context(), req.ChannelID, userID)
	if err != nil {
		slog.Error("failed to import decisions", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"imported": count,
		"message":  "decisions imported successfully",
	})
}

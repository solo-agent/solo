package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/internal/server/service"
)

// InboxHandler handles inbox API endpoints.
type InboxHandler struct {
	svc *service.InboxService
}

// NewInboxHandler creates a new InboxHandler.
func NewInboxHandler(svc *service.InboxService) *InboxHandler {
	return &InboxHandler{svc: svc}
}

// List handles GET /api/v1/inbox
func (h *InboxHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	before := time.Now()
	if b := r.URL.Query().Get("before"); b != "" {
		if parsed, err := time.Parse(time.RFC3339, b); err == nil {
			before = parsed
		}
	}

	limit := 30
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	items, hasMore, err := h.svc.List(r.Context(), userID, before, limit)
	if err != nil {
		slog.Error("inbox list: query failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list inbox items")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    items,
		"has_more": hasMore,
	})
}

// UnreadCount handles GET /api/v1/inbox/unread-count
func (h *InboxHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	counts, err := h.svc.UnreadCount(r.Context(), userID)
	if err != nil {
		slog.Error("inbox unread count: query failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get unread count")
		return
	}

	writeJSON(w, http.StatusOK, counts)
}

// Dismiss handles POST /api/v1/inbox/{messageId}/dismiss
func (h *InboxHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	messageID := chi.URLParam(r, "messageId")
	if messageID == "" {
		writeError(w, http.StatusBadRequest, "missing messageId")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	if err := h.svc.Dismiss(r.Context(), userID, messageID); err != nil {
		slog.Error("inbox dismiss: failed", "request_id", reqID, "user_id", userID, "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to dismiss inbox item")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DismissAll handles POST /api/v1/inbox/dismiss-all
func (h *InboxHandler) DismissAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	if err := h.svc.DismissAll(r.Context(), userID); err != nil {
		slog.Error("inbox dismiss-all: failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to dismiss all inbox items")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

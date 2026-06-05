package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
// Returns inbox items for the authenticated user with cursor pagination.
func (h *InboxHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	// Parse before parameter (cursor)
	before := time.Now()
	if b := r.URL.Query().Get("before"); b != "" {
		if parsed, err := time.Parse(time.RFC3339, b); err == nil {
			before = parsed
		}
	}

	// Parse limit parameter
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
// Returns per-category unread counts for the authenticated user.
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

// MarkRead handles POST /api/v1/inbox/mark-read
// Updates the user's last_read_at timestamp to now.
func (h *InboxHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	if err := h.svc.MarkRead(r.Context(), userID); err != nil {
		slog.Error("inbox mark read: query failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to mark inbox as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

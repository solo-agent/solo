package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/internal/server/service"
)

type InboxHandler struct {
	svc *service.InboxService
}

func NewInboxHandler(svc *service.InboxService) *InboxHandler {
	return &InboxHandler{svc: svc}
}

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

	// Parse optional type filter (comma-separated: thread_reply,dm,mention)
	types := []string{}
	if t := r.URL.Query().Get("types"); t != "" {
		for _, s := range strings.Split(t, ",") {
			s = strings.TrimSpace(s)
			if s == "thread_reply" || s == "dm" || s == "mention" {
				types = append(types, s)
			}
		}
	}

	senderFilter := r.URL.Query().Get("sender")

	items, hasMore, err := h.svc.List(r.Context(), userID, before, limit, types, senderFilter)
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

func (h *InboxHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.MarkRead(r.Context(), userID, messageID); err != nil {
		slog.Error("inbox mark read: failed", "request_id", reqID, "user_id", userID, "message_id", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to mark inbox item as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *InboxHandler) ClearAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	if err := h.svc.ClearAll(r.Context(), userID); err != nil {
		slog.Error("inbox clear all: failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to clear inbox")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *InboxHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	reqID := middleware.GetRequestID(r.Context())

	if err := h.svc.MarkAllRead(r.Context(), userID); err != nil {
		slog.Error("inbox mark all read: failed", "request_id", reqID, "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to mark all as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

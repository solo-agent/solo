package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/solo-ai/solo/internal/server/service"
)

// ErrorResponse is the standard API error response body.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: http.StatusText(status), Message: message})
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: http.StatusText(status), Message: message, Code: code})
}

func validateClientMessageID(raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	return id, len(id) <= 128
}

func beginReliableSend(w http.ResponseWriter, r *http.Request, dedupe *service.SendDedupe, senderID, clientMessageID string) (*service.SendDedupeClaim, bool) {
	if dedupe == nil || clientMessageID == "" {
		return nil, true
	}
	claim, cached, err := dedupe.Acquire(r.Context(), senderID+":"+clientMessageID)
	if err != nil {
		return nil, false
	}
	if cached != nil {
		writeJSON(w, cached.Status, markSendDeduplicated(cached.Body))
		return nil, false
	}
	return claim, true
}

func completeReliableSend(w http.ResponseWriter, status int, body any, claim *service.SendDedupeClaim) {
	claim.Complete(status, body)
	writeJSON(w, status, body)
}

func markSendDeduplicated(body any) any {
	switch response := body.(type) {
	case MessageResponse:
		response.Deduplicated = true
		return response
	case ThreadReplyResponse:
		response.Deduplicated = true
		return response
	case TaskResponse:
		response.Deduplicated = true
		return response
	case FreshnessHeldResponse:
		response.Deduplicated = true
		return response
	default:
		return body
	}
}

// requireUserID extracts the X-User-ID header set by the auth middleware.
func requireUserID(r *http.Request) (string, bool) {
	uid := r.Header.Get("X-User-ID")
	if uid == "" {
		return "", false
	}
	return uid, true
}

// isNotFound checks if a pgx error is a "no rows" error.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// isUniqueViolation checks if a pgx error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

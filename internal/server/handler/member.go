package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
	"strings"
)

// MemberHandler handles channel member management requests.
type MemberHandler struct {
	svc      *service.ChannelService
	agentSvc *service.AgentService
}

// NewMemberHandler creates a new MemberHandler.
func NewMemberHandler(pool *pgxpool.Pool, agentSvc *service.AgentService) *MemberHandler {
	return &MemberHandler{
		svc:      service.NewChannelService(pool),
		agentSvc: agentSvc,
	}
}

// --- Request types ---

type AddMemberRequest struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
}

// AddMember handles POST /api/v1/channels/{channelID}/members
func (h *MemberHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	requesterID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate input
	if req.MemberType == "" {
		writeError(w, http.StatusBadRequest, "member_type is required")
		return
	}
	if req.MemberType != "user" && req.MemberType != "agent" {
		writeError(w, http.StatusBadRequest, "member_type must be 'user' or 'agent'")
		return
	}
	if req.MemberID == "" {
		writeError(w, http.StatusBadRequest, "member_id is required")
		return
	}

	err := h.svc.AddMember(r.Context(), channelID, requesterID, req.MemberType, req.MemberID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrChannelNotFound):
			writeError(w, http.StatusNotFound, "channel not found")
		case errors.Is(err, service.ErrNotChannelMember):
			writeError(w, http.StatusForbidden, "you are not a member of this channel")
		case errors.Is(err, service.ErrPermissionDenied):
			writeError(w, http.StatusForbidden, "only channel owners and admins can add members")
		case errors.Is(err, service.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		case errors.Is(err, service.ErrAgentNotFound):
			writeError(w, http.StatusNotFound, "agent not found")
		case errors.Is(err, service.ErrAlreadyMember):
			writeError(w, http.StatusConflict, "member is already in this channel")
		default:
			slog.Error("failed to add member", "channel_id", channelID, "member_type", req.MemberType, "member_id", req.MemberID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to add member")
		}
		return
	}

	slog.Info("member added to channel",
		"channel_id", channelID,
		"member_type", req.MemberType,
		"member_id", req.MemberID,
		"added_by", requesterID,
	)

	// v1.3: When an agent joins a channel, trigger a greeting so it
	// introduces itself — new-agent-in-channel behavior.
	if req.MemberType == "agent" && h.agentSvc != nil {
		go h.agentSvc.TriggerAgentGreeting(context.Background(), channelID, req.MemberID)
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"channel_id":  channelID,
		"member_type": req.MemberType,
		"member_id":   req.MemberID,
		"role":        "member",
	})
}

// RemoveMember handles DELETE /api/v1/channels/{channelID}/members/{memberID}
func (h *MemberHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	memberID := chi.URLParam(r, "memberID")
	requesterID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	err := h.svc.RemoveMember(r.Context(), channelID, requesterID, memberID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMemberNotFound):
			writeError(w, http.StatusNotFound, "member not found")
		case errors.Is(err, service.ErrNotChannelMember):
			writeError(w, http.StatusForbidden, "you are not a member of this channel")
		case errors.Is(err, service.ErrPermissionDenied):
			writeError(w, http.StatusForbidden, "you do not have permission to remove this member")
		default:
			slog.Error("failed to remove member", "channel_id", channelID, "member_id", memberID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to remove member")
		}
		return
	}

	slog.Info("member removed from channel",
		"channel_id", channelID,
		"member_id", memberID,
		"removed_by", requesterID,
	)

	w.WriteHeader(http.StatusNoContent)
}

// ListMembers handles GET /api/v1/channels/{channelID}/members
func (h *MemberHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	// Resolve channel name (e.g. "#test-2" or "test-2") to UUID.
	if !looksLikeUUID(channelID) {
		name := strings.TrimPrefix(channelID, "#")
		if resolved, err := h.resolveChannelName(r, name); err == nil {
			channelID = resolved
		}
	}
	requesterID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	members, err := h.svc.ListMembers(r.Context(), channelID, requesterID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotChannelMember):
			writeError(w, http.StatusForbidden, "you are not a member of this channel")
		default:
			slog.Error("failed to list members", "channel_id", channelID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list members")
		}
		return
	}

	writeJSON(w, http.StatusOK, members)
}

// looksLikeUUID returns true if s looks like a UUID.
func looksLikeUUID(s string) bool {
	return len(s) >= 32 && len(s) <= 36 && (s[8] == '-' || s[12] == '-')
}

func (h *MemberHandler) resolveChannelName(r *http.Request, name string) (string, error) {
	id, ok := h.svc.ResolveChannelName(r.Context(), name)
	if !ok {
		return "", errors.New("channel not found")
	}
	return id, nil
}

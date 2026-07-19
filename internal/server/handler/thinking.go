package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

type ThinkingHandler struct {
	pool     *pgxpool.Pool
	svc      *service.ThinkingService
	agentSvc *service.AgentService
	hub      *ws.Hub
}

func NewThinkingHandler(pool *pgxpool.Pool, hub *ws.Hub, agentSvc *service.AgentService) *ThinkingHandler {
	return &ThinkingHandler{pool: pool, svc: service.NewThinkingService(pool), agentSvc: agentSvc, hub: hub}
}

func (h *ThinkingHandler) requireMember(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	actorID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", "", false
	}
	channelID := chi.URLParam(r, "channelID")
	if _, err := uuid.Parse(channelID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel ID")
		return "", "", false
	}
	var member bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1
			  FROM channel_members member
			  JOIN channels channel ON channel.id = member.channel_id AND channel.is_archived = false
			 WHERE member.channel_id = $1 AND member.member_id = $2
			   AND member.member_type IN ('user', 'agent')
		)`, channelID, actorID).Scan(&member)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check channel membership")
		return "", "", false
	}
	if !member {
		writeError(w, http.StatusNotFound, "channel not found")
		return "", "", false
	}
	return actorID, channelID, true
}

func (h *ThinkingHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	space, err := h.svc.Get(r.Context(), channelID)
	if errors.Is(err, service.ErrThinkingNotFound) {
		writeError(w, http.StatusNotFound, "thinking space not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thinking space")
		return
	}
	writeJSON(w, http.StatusOK, space)
}

func (h *ThinkingHandler) Ensure(w http.ResponseWriter, r *http.Request) {
	actorID, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	space, err := h.svc.Ensure(r.Context(), channelID, actorID)
	if err != nil {
		slog.Error("failed to start thinking mode", "channel_id", channelID, "actor_id", actorID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start thinking mode")
		return
	}
	if h.hub != nil {
		h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{"channel_id": channelID, "space_id": space.ID}))
	}
	writeJSON(w, http.StatusOK, space)
}

type createThinkingChildRequest struct {
	Title string `json:"title"`
}

func (h *ThinkingHandler) CreateChild(w http.ResponseWriter, r *http.Request) {
	actorID, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	nodeID := chi.URLParam(r, "nodeID")
	if _, err := uuid.Parse(nodeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid node ID")
		return
	}
	var req createThinkingChildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "node title is required")
		return
	}
	node, err := h.svc.CreateChild(r.Context(), channelID, nodeID, req.Title, actorID, "manual")
	switch {
	case errors.Is(err, service.ErrThinkingNotFound):
		writeError(w, http.StatusNotFound, "thinking node not found")
	case errors.Is(err, service.ErrThinkingLimit):
		writeError(w, http.StatusConflict, "this node cannot be split further")
	case errors.Is(err, service.ErrThinkingReturned):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrThinkingReturning):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrThinkingPreparing):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrThinkingDuplicate):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		if node.ForkHandoffPending && h.agentSvc != nil {
			if err := h.agentSvc.TriggerThinkingForkHandoff(r.Context(), channelID, node.ParentID, node.ID, node.Title); err != nil {
				slog.Warn("failed to prepare manual fork handoff", "node_id", node.ID, "error", err)
			}
		}
		if h.hub != nil {
			h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{"channel_id": channelID, "node_id": node.ID}))
		}
		writeJSON(w, http.StatusCreated, node)
	}
}

func (h *ThinkingHandler) RetryForkHandoff(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	nodeID := chi.URLParam(r, "nodeID")
	if _, err := uuid.Parse(nodeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid node ID")
		return
	}
	node, err := h.svc.GetNodeForChannel(r.Context(), channelID, nodeID)
	if errors.Is(err, service.ErrThinkingNotFound) {
		writeError(w, http.StatusNotFound, "thinking node not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load thinking node")
		return
	}
	if !node.ForkHandoffPending || node.ParentID == "" {
		writeError(w, http.StatusConflict, "fork handoff is not pending")
		return
	}
	if h.agentSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "Agent runtime is unavailable")
		return
	}
	if err := h.agentSvc.TriggerThinkingForkHandoff(r.Context(), channelID, node.ParentID, node.ID, node.Title); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, node)
}

func (h *ThinkingHandler) RefreshCheckpoint(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	nodeID := chi.URLParam(r, "nodeID")
	if _, err := uuid.Parse(nodeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid node ID")
		return
	}
	node, err := h.svc.PrepareCheckpointRefresh(r.Context(), channelID, nodeID)
	switch {
	case errors.Is(err, service.ErrThinkingNotFound):
		writeError(w, http.StatusNotFound, "thinking node not found")
	case errors.Is(err, service.ErrThinkingReturned),
		errors.Is(err, service.ErrThinkingReturning),
		errors.Is(err, service.ErrThinkingBusy),
		errors.Is(err, service.ErrThinkingPreparing):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusConflict, err.Error())
	default:
		if h.agentSvc == nil {
			writeError(w, http.StatusServiceUnavailable, "Agent runtime is unavailable")
			return
		}
		if err := h.agentSvc.TriggerThinkingCheckpointRefresh(r.Context(), channelID, nodeID); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, node)
	}
}

func (h *ThinkingHandler) ReturnNode(w http.ResponseWriter, r *http.Request) {
	_, channelID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	nodeID := chi.URLParam(r, "nodeID")
	if _, err := uuid.Parse(nodeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid node ID")
		return
	}
	node, err := h.svc.BeginReturn(r.Context(), channelID, nodeID)
	switch {
	case errors.Is(err, service.ErrThinkingNotFound):
		writeError(w, http.StatusNotFound, "thinking node not found")
	case errors.Is(err, service.ErrThinkingReturned),
		errors.Is(err, service.ErrThinkingReturning),
		errors.Is(err, service.ErrThinkingBusy),
		errors.Is(err, service.ErrThinkingChildren),
		errors.Is(err, service.ErrThinkingPreparing):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusConflict, err.Error())
	default:
		if h.agentSvc == nil {
			_, _ = h.svc.CancelReturn(r.Context(), nodeID)
			writeError(w, http.StatusServiceUnavailable, "Agent runtime is unavailable")
			return
		}
		if err := h.agentSvc.TriggerThinkingNodeReturn(r.Context(), channelID, nodeID); err != nil {
			_, _ = h.svc.CancelReturn(r.Context(), nodeID)
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if h.hub != nil {
			h.hub.BroadcastToChannel(channelID, ws.Envelope(ws.EventThinkingUpdated, map[string]string{"channel_id": channelID, "node_id": node.ID}))
		}
		writeJSON(w, http.StatusAccepted, node)
	}
}

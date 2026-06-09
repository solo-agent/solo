package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/onboarding"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/pkg/agent"
)

// OnboardingHandler handles onboarding-related HTTP requests.
type OnboardingHandler struct {
	pool     *pgxpool.Pool
	svc      *service.ChannelService
	agentSvc *service.AgentService
}

// NewOnboardingHandler creates a new OnboardingHandler.
func NewOnboardingHandler(pool *pgxpool.Pool, agentSvc *service.AgentService) *OnboardingHandler {
	return &OnboardingHandler{
		pool:     pool,
		svc:      service.NewChannelService(pool),
		agentSvc: agentSvc,
	}
}

// CreateLucyRequest is the request body for creating the Lucy onboarding agent.
type CreateLucyRequest struct {
	RuntimeType string `json:"runtime_type"`
	ComputerID  string `json:"computer_id,omitempty"`
	ChannelID   string `json:"channel_id"`
}

// CreateLucyResponse is returned after successfully creating Lucy.
type CreateLucyResponse struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	ChannelID string `json:"channel_id"`
}

// CreateLucy handles POST /api/v1/onboarding/create-lucy.
// Creates the Lucy onboarding agent with the user's selected runtime and
// optional computer binding, then triggers her first welcome message.
func (h *OnboardingHandler) CreateLucy(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateLucyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	runtimeType := strings.TrimSpace(req.RuntimeType)
	if runtimeType == "" {
		writeError(w, http.StatusBadRequest, "runtime_type is required")
		return
	}

	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}

	// Validate runtime_type is a registered backend type.
	if !isValidRuntime(runtimeType) {
		writeError(w, http.StatusBadRequest, "unknown runtime_type: "+runtimeType)
		return
	}

	// Verify channel exists and user is a member.
	if !h.userOwnsChannel(r.Context(), channelID, userID) {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	// Get user info for display name / email.
	displayName := r.Header.Get("X-User-Name")
	if displayName == "" {
		displayName = "New User"
	}
	email := r.Header.Get("X-User-Email")
	if email == "" {
		email = ""
	}

	agentID := uuid.New().String()
	agentDesc := "Onboarding lead — helps you set up your Solo workspace."

	// Create Lucy agent via direct SQL with the selected runtime as model_provider.
	// Store computer binding in runtime_id column (dead column from migration 000021, repurposed).
	computerID := strings.TrimSpace(req.ComputerID)
	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO agents (id, name, description, owner_id, model_provider, model_name,
			system_prompt, temperature, max_tokens, runtime_id, custom_env, custom_args)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		agentID, onboarding.LucyName, agentDesc, userID,
		runtimeType, "", // model_provider = selected runtime, model_name = auto
		onboarding.LucySystemPrompt, 0.7, 4096,
		nullIfEmpty(computerID),
		`{}`, `[]`,
	)
	if err != nil {
		slog.Error("onboarding: failed to create Lucy agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create Lucy")
		return
	}

	// Add Lucy to the onboarding channel.
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'agent', $2, 'member')`,
		channelID, agentID,
	)
	if err != nil {
		slog.Warn("onboarding: failed to add Lucy to channel",
			"channel_id", channelID, "agent_id", agentID, "error", err)
	} else if h.agentSvc != nil {
		h.agentSvc.BroadcastMemberEvent(channelID, "member.added", "agent", agentID, onboarding.LucyName)
	}

	// Also add Lucy to the shared #all channel.
	h.ensureAgentInAllChannel(r.Context(), agentID)
	if h.agentSvc != nil {
		allID := allChannelID(r.Context(), h.pool, agentID)
		if allID != "" {
			h.agentSvc.BroadcastMemberEvent(allID, "member.added", "agent", agentID, onboarding.LucyName)
		}
	}

	// Seed knowledge files asynchronously.
	channelName := channelName(r.Context(), h.pool, channelID)
	go onboarding.SeedAgentKnowledge(agentID, displayName, email)

	// Trigger Lucy's first message — the greeting is injected as private
	// agent context (not a visible channel message).
	if h.agentSvc != nil {
		greeting := onboarding.GreetingPrompt(displayName, email, channelName)
		go h.agentSvc.TriggerAgentGreeting(
			context.Background(),
			channelID, agentID, greeting,
		)
	}

	slog.Info("onboarding: Lucy created via wizard",
		"user_id", userID,
		"agent_id", agentID,
		"runtime_type", runtimeType,
		"channel_id", channelID,
	)

	writeJSON(w, http.StatusCreated, CreateLucyResponse{
		AgentID:   agentID,
		AgentName: onboarding.LucyName,
		ChannelID: channelID,
	})
}

// isValidRuntime checks if the runtime type is registered in the global backend registry.
func isValidRuntime(runtimeType string) bool {
	for _, m := range agent.GlobalRegistry().ListMeta() {
		if m.Type == runtimeType {
			return true
		}
	}
	return false
}

// userOwnsChannel checks the user is a member of the given channel.
func (h *OnboardingHandler) userOwnsChannel(ctx context.Context, channelID, userID string) bool {
	var isMember bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = 'user' AND member_id = $2
		)`, channelID, userID,
	).Scan(&isMember)
	return err == nil && isMember
}

// channelName resolves a channel name from its ID.
func channelName(ctx context.Context, pool *pgxpool.Pool, channelID string) string {
	var name string
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(name, id::text) FROM channels WHERE id = $1`, channelID,
	).Scan(&name)
	if name == "" {
		name = channelID
	}
	return name
}

// ensureAgentInAllChannel adds the agent to the shared #all channel.
func (h *OnboardingHandler) ensureAgentInAllChannel(ctx context.Context, agentID string) {
	allID := allChannelID(ctx, h.pool, agentID)
	if allID == "" {
		return
	}
	_, _ = h.pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'agent', $2, 'member')
		 ON CONFLICT DO NOTHING`,
		allID, agentID,
	)
}

// allChannelID resolves the per-user #all-{name} channel ID for the given agent's owner.
func allChannelID(ctx context.Context, pool *pgxpool.Pool, agentID string) string {
	var id string
	_ = pool.QueryRow(ctx,
		`SELECT c.id FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 JOIN agents a ON a.owner_id = cm.member_id
		 WHERE a.id = $1 AND cm.member_type = 'user'
		 AND c.name LIKE 'all-%%' AND c.is_archived = false
		 LIMIT 1`,
		agentID,
	).Scan(&id)
	return id
}

// nullIfEmpty returns a *string that is nil when s is empty.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

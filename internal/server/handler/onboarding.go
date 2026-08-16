package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/onboarding"
	"github.com/solo-ai/solo/internal/server/service"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
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
	ComputerID  string `json:"computer_id"`
	ChannelID   string `json:"channel_id"`
	ModelName   string `json:"model_name"`
}

// CreateLucyResponse is returned after successfully creating Lucy.
type CreateLucyResponse struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	ChannelID string `json:"channel_id"`
}

type onboardingRuntime struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
	Version     string `json:"version,omitempty"`
}

type OnboardingStatusResponse struct {
	Required      bool                `json:"required"`
	Step          int                 `json:"step"`
	ComputerID    string              `json:"computer_id,omitempty"`
	ComputerName  string              `json:"computer_name,omitempty"`
	Runtimes      []onboardingRuntime `json:"runtimes"`
	WorkspaceID   string              `json:"workspace_id,omitempty"`
	WorkspaceName string              `json:"workspace_name,omitempty"`
	LucyChannelID string              `json:"lucy_channel_id,omitempty"`
	LucyAgentID   string              `json:"lucy_agent_id,omitempty"`
	GreetingReady bool                `json:"greeting_ready"`
}

func (h *OnboardingHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	status := OnboardingStatusResponse{Step: 1, Runtimes: []onboardingRuntime{}}
	if err := h.pool.QueryRow(r.Context(), `SELECT onboarding_completed_at IS NULL FROM users WHERE id=$1`, userID).Scan(&status.Required); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load onboarding status")
		return
	}
	if !status.Required {
		writeJSON(w, http.StatusOK, status)
		return
	}

	err := h.pool.QueryRow(r.Context(), `
		SELECT w.id::text,w.name,c.id::text
		  FROM workspaces w
		  JOIN workspace_members wm ON wm.workspace_id=w.id AND wm.user_id=$1
		  JOIN channels c ON c.workspace_id=w.id AND c.type='lucy' AND c.is_archived=false
		 WHERE w.created_by=$1 AND w.is_personal=true AND w.deleted_at IS NULL
		 LIMIT 1`, userID).Scan(&status.WorkspaceID, &status.WorkspaceName, &status.LucyChannelID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load onboarding Workspace")
		return
	}
	if status.WorkspaceID != "" {
		err = h.pool.QueryRow(r.Context(), `
		SELECT id::text FROM agents
		 WHERE owner_id=$1 AND home_channel_id=$2 AND kind='lucy' AND is_active=true
		 LIMIT 1`, userID, status.LucyChannelID).Scan(&status.LucyAgentID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to load Lucy")
			return
		}
		if status.LucyAgentID != "" {
			status.Step = 4
			if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM messages WHERE channel_id=$1 AND sender_type='agent' AND sender_id=$2)`, status.LucyChannelID, status.LucyAgentID).Scan(&status.GreetingReady); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to load Lucy greeting")
				return
			}
			writeJSON(w, http.StatusOK, status)
			return
		}
	}

	var inventory json.RawMessage
	err = h.pool.QueryRow(r.Context(), `
		SELECT id::text,name,COALESCE(runtime_inventory,'[]'::jsonb)
		  FROM computers
		 WHERE owner_id=$1 AND status='online'
		   AND credential_hash IS NOT NULL AND credential_revoked_at IS NULL
		 ORDER BY last_connected_at DESC NULLS LAST,created_at
		 LIMIT 1`, userID).Scan(&status.ComputerID, &status.ComputerName, &inventory)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load onboarding computer")
		return
	}
	if status.ComputerID == "" {
		writeJSON(w, http.StatusOK, status)
		return
	}
	var runtimes []onboardingRuntime
	if json.Unmarshal(inventory, &runtimes) == nil {
		for _, runtime := range runtimes {
			if runtime.Available {
				status.Runtimes = append(status.Runtimes, runtime)
			}
		}
	}
	if status.WorkspaceID == "" {
		status.Step = 2
	} else {
		status.Step = 3
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var channelID string
	err := h.pool.QueryRow(r.Context(), `
		SELECT c.id::text FROM workspaces w
		JOIN channels c ON c.workspace_id=w.id AND c.type='lucy' AND c.is_archived=false
		JOIN agents a ON a.home_channel_id=c.id AND a.owner_id=$1 AND a.kind='lucy' AND a.is_active=true
		JOIN messages m ON m.channel_id=c.id AND m.sender_type='agent' AND m.sender_id=a.id
		WHERE w.created_by=$1 AND w.is_personal=true AND w.deleted_at IS NULL
		LIMIT 1`, userID).Scan(&channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Lucy has not greeted you yet")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish onboarding")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `UPDATE users SET onboarding_completed_at=COALESCE(onboarding_completed_at,now()),updated_at=now() WHERE id=$1`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish onboarding")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"channel_id": channelID})
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
	modelName := strings.TrimSpace(req.ModelName)
	if len(modelName) > 200 {
		writeError(w, http.StatusBadRequest, "model_name is too long")
		return
	}

	// Verify channel exists and user is a member.
	if !h.userOwnsChannel(r.Context(), channelID, userID, serverworkspace.ID(r)) {
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
	if computerID == "" {
		writeError(w, http.StatusBadRequest, "computer_id is required")
		return
	}
	{
		var usable bool
		if err := h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(
				SELECT 1 FROM computers c
				LEFT JOIN computer_members cm ON cm.computer_id = c.id AND cm.user_id = $2
				WHERE c.id = $1
				  AND (c.owner_id = $2 OR cm.user_id IS NOT NULL)
				  AND ((c.credential_hash IS NOT NULL AND c.credential_revoked_at IS NULL)
				    OR (c.daemon_id IS NOT NULL AND c.status = 'online'))
			)`,
			computerID, userID,
		).Scan(&usable); err != nil {
			slog.Error("onboarding: check computer access failed", "computer_id", computerID, "user_id", userID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to verify computer access")
			return
		}
		if !usable {
			writeError(w, http.StatusBadRequest, "computer is unavailable")
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create Lucy")
		return
	}
	defer tx.Rollback(r.Context())
	var existingAgentID string
	if err := tx.QueryRow(r.Context(), `
		SELECT id::text FROM agents
		 WHERE owner_id=$1 AND home_channel_id=$2 AND kind='lucy' AND is_active=true
		 LIMIT 1 FOR UPDATE`, userID, channelID).Scan(&existingAgentID); err == nil {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load Lucy")
			return
		}
		h.triggerGreetingIfMissing(channelID, existingAgentID, displayName, email)
		writeJSON(w, http.StatusOK, CreateLucyResponse{AgentID: existingAgentID, AgentName: onboarding.LucyName, ChannelID: channelID})
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load Lucy")
		return
	}

	_, err = tx.Exec(r.Context(),
		`INSERT INTO agents (id, name, description, owner_id, model_provider, model_name,
			system_prompt, runtime_id, custom_env, custom_args, avatar_url, home_channel_id, kind)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'dicebear:pixel-art:lucy', $11, 'lucy')`,
		agentID, onboarding.LucyName, agentDesc, userID,
		runtimeType, modelName,
		onboarding.LucySystemPrompt,
		nullIfEmpty(computerID),
		`{}`, `[]`, channelID,
	)
	if err != nil {
		slog.Error("onboarding: failed to create Lucy agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create Lucy")
		return
	}

	// Add Lucy to the onboarding channel.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'agent', $2, 'member')`,
		channelID, agentID,
	)
	if err != nil {
		slog.Error("onboarding: failed to add Lucy to channel",
			"channel_id", channelID, "agent_id", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create Lucy")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("onboarding: failed to commit Lucy", "agent_id", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create Lucy")
		return
	}
	if h.agentSvc != nil {
		h.agentSvc.BroadcastMemberEvent(channelID, "member.added", "agent", agentID, onboarding.LucyName)
	}

	// Seed knowledge files asynchronously.
	go onboarding.SeedAgentKnowledge(agentID, displayName, email)

	// Trigger Lucy's first message — the greeting is injected as private
	// agent context (not a visible channel message).
	h.triggerGreetingIfMissing(channelID, agentID, displayName, email)

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

func (h *OnboardingHandler) triggerGreetingIfMissing(channelID, agentID, displayName, email string) {
	if h.agentSvc == nil {
		return
	}
	var exists bool
	if h.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM messages WHERE channel_id=$1 AND sender_type='agent' AND sender_id=$2)`, channelID, agentID).Scan(&exists) != nil || exists {
		return
	}
	greeting := onboarding.GreetingPrompt(displayName, email, channelName(context.Background(), h.pool, channelID))
	go h.agentSvc.TriggerAgentGreeting(context.Background(), channelID, agentID, greeting)
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
func (h *OnboardingHandler) userOwnsChannel(ctx context.Context, channelID, userID, workspaceID string) bool {
	var isMember bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1
			  FROM channels c
			  JOIN channel_members cm ON cm.channel_id = c.id
			 WHERE c.id = $1
			   AND c.workspace_id = $3
			   AND c.type = 'lucy'
			   AND c.is_archived = false
			   AND cm.member_type = 'user'
			   AND cm.member_id = $2
		)`, channelID, userID, workspaceID,
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

// nullIfEmpty returns a *string that is nil when s is empty.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

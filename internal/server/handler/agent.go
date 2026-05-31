package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentHandler handles agent-related HTTP requests.
type AgentHandler struct {
	pool *pgxpool.Pool
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(pool *pgxpool.Pool) *AgentHandler {
	return &AgentHandler{pool: pool}
}

// --- Request/Response types ---

type CreateAgentRequest struct {
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	SystemPrompt  string  `json:"system_prompt,omitempty"`
	ModelProvider string  `json:"model_provider,omitempty"`
	ModelName     string  `json:"model_name,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	MaxTokens     *int    `json:"max_tokens,omitempty"`
	AutoJoin      *bool   `json:"auto_join,omitempty"`
}

type UpdateAgentRequest struct {
	Name          *string  `json:"name,omitempty"`
	Description   *string  `json:"description,omitempty"`
	SystemPrompt  *string  `json:"system_prompt,omitempty"`
	ModelProvider *string  `json:"model_provider,omitempty"`
	ModelName     *string  `json:"model_name,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	MaxTokens     *int     `json:"max_tokens,omitempty"`
	AutoJoin      *bool    `json:"auto_join,omitempty"`
}

type AgentResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	OwnerID       string  `json:"owner_id"`
	ModelProvider string  `json:"model_provider"`
	ModelName     string  `json:"model_name"`
	SystemPrompt  string  `json:"system_prompt"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	IsActive      bool    `json:"is_active"`
	AutoJoin      bool    `json:"auto_join"`
	AvatarURL     string  `json:"avatar_url,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// Create handles POST /api/v1/agents
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "agent name is required")
		return
	}
	if len(name) > 100 {
		writeError(w, http.StatusBadRequest, "agent name must be 100 characters or less")
		return
	}

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant."
	}

	modelProvider := req.ModelProvider
	if modelProvider == "" {
		modelProvider = "anthropic"
	}

	modelName := req.ModelName
	if modelName == "" {
		modelName = "claude-sonnet-4-20250514"
	}

	temperature := 0.7
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	maxTokens := 4096
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	autoJoin := false
	if req.AutoJoin != nil {
		autoJoin = *req.AutoJoin
	}

	var agentID string
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO agents (name, description, owner_id, model_provider, model_name, system_prompt, temperature, max_tokens, auto_join)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at, updated_at`,
		name, req.Description, userID, modelProvider, modelName, systemPrompt, temperature, maxTokens, autoJoin,
	).Scan(&agentID, &createdAt, &updatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "agent name conflict")
			return
		}
		slog.Error("failed to create agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	slog.Info("agent created", "agent_id", agentID, "name", name, "owner_id", userID)

	writeJSON(w, http.StatusCreated, AgentResponse{
		ID:            agentID,
		Name:          name,
		Description:   req.Description,
		OwnerID:       userID,
		ModelProvider: modelProvider,
		ModelName:     modelName,
		SystemPrompt:  systemPrompt,
		Temperature:   temperature,
		MaxTokens:     maxTokens,
		IsActive:      true,
		AutoJoin:      autoJoin,
		CreatedAt:     createdAt.Format(time.RFC3339),
		UpdatedAt:     updatedAt.Format(time.RFC3339),
	})
}

// List handles GET /api/v1/agents
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, COALESCE(description, ''), owner_id, model_provider, model_name,
		        system_prompt, temperature, max_tokens, is_active, auto_join, COALESCE(avatar_url, ''),
		        created_at, updated_at
		 FROM agents
		 WHERE owner_id = $1 AND is_active = true
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("failed to query agents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	defer rows.Close()

	agents := make([]AgentResponse, 0)
	for rows.Next() {
		var a AgentResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
			&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
			&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
			&createdAt, &updatedAt)
		if err != nil {
			slog.Error("failed to scan agent row", "error", err)
			continue
		}
		a.CreatedAt = createdAt.Format(time.RFC3339)
		a.UpdatedAt = updatedAt.Format(time.RFC3339)
		agents = append(agents, a)
	}

	writeJSON(w, http.StatusOK, agents)
}

// Get handles GET /api/v1/agents/{id}
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required")
		return
	}

	var a AgentResponse
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description, ''), owner_id, model_provider, model_name,
		        system_prompt, temperature, max_tokens, is_active, auto_join, COALESCE(avatar_url, ''),
		        created_at, updated_at
		 FROM agents
		 WHERE id = $1 AND owner_id = $2 AND is_active = true`,
		agentID, userID,
	).Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
		&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
		&createdAt, &updatedAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		slog.Error("failed to query agent", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)

	writeJSON(w, http.StatusOK, a)
}

// Update handles PATCH /api/v1/agents/{id}
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required")
		return
	}

	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate name if provided
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "agent name cannot be empty")
			return
		}
		if len(name) > 100 {
			writeError(w, http.StatusBadRequest, "agent name must be 100 characters or less")
			return
		}
	}

	var a AgentResponse
	var createdAt, updatedAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`UPDATE agents SET
			name = COALESCE($1, name),
			description = COALESCE($2, description),
			system_prompt = COALESCE($3, system_prompt),
			model_provider = COALESCE($4, model_provider),
			model_name = COALESCE($5, model_name),
			temperature = COALESCE($6, temperature),
			max_tokens = COALESCE($7, max_tokens),
			auto_join = COALESCE($8, auto_join),
			updated_at = now()
		 WHERE id = $9 AND owner_id = $10 AND is_active = true
		 RETURNING id, name, COALESCE(description, ''), owner_id, model_provider, model_name,
		           system_prompt, temperature, max_tokens, is_active, auto_join, COALESCE(avatar_url, ''),
		           created_at, updated_at`,
		req.Name, req.Description, req.SystemPrompt,
		req.ModelProvider, req.ModelName, req.Temperature, req.MaxTokens, req.AutoJoin,
		agentID, userID,
	).Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
		&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
		&createdAt, &updatedAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		slog.Error("failed to update agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)

	writeJSON(w, http.StatusOK, a)
}

// Delete handles DELETE /api/v1/agents/{id} (soft delete: sets is_active=false)
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID is required")
		return
	}

	result, err := h.pool.Exec(r.Context(),
		`UPDATE agents SET is_active = false, updated_at = now()
		 WHERE id = $1 AND owner_id = $2 AND is_active = true`,
		agentID, userID,
	)
	if err != nil {
		slog.Error("failed to deactivate agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	slog.Info("agent deactivated", "agent_id", agentID, "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "agent deleted"})
}

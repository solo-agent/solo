package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/workspace"
	"github.com/solo-ai/solo/pkg/agent"
)

const (
	// maxWorkspaceFileSize limits the size of files returned when content=true.
	maxWorkspaceFileSize = 1 * 1024 * 1024 // 1 MB
	// maxWorkspaceDepth limits directory traversal depth.
	maxWorkspaceDepth = 20
)

// AgentHandler handles agent-related HTTP requests.
type AgentHandler struct {
	pool          *pgxpool.Pool
	workspaceRoot string             // base path for agent workspaces, defaults to ~/.solo/agents
	proxy         workspace.Proxy    // optional proxy for workspace requests (nil = local FS only)
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(pool *pgxpool.Pool, proxy workspace.Proxy) *AgentHandler {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	workspaceRoot := filepath.Join(home, ".solo", "agents")
	if proxy == nil {
		slog.Warn("agent handler: no workspace proxy configured, falling back to local filesystem only")
	}
	return &AgentHandler{pool: pool, workspaceRoot: workspaceRoot, proxy: proxy}
}

// --- Request/Response types ---

type CreateAgentRequest struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	SystemPrompt  string            `json:"system_prompt,omitempty"`
	ModelProvider string            `json:"model_provider,omitempty"`
	ModelName     string            `json:"model_name,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	MaxTokens     *int              `json:"max_tokens,omitempty"`
	AutoJoin      *bool             `json:"auto_join,omitempty"`
	CustomEnv     map[string]string `json:"custom_env,omitempty"`
	CustomArgs    []string          `json:"custom_args,omitempty"`
}

type UpdateAgentRequest struct {
	Name          *string            `json:"name,omitempty"`
	Description   *string            `json:"description,omitempty"`
	SystemPrompt  *string            `json:"system_prompt,omitempty"`
	ModelProvider *string            `json:"model_provider,omitempty"`
	ModelName     *string            `json:"model_name,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	MaxTokens     *int               `json:"max_tokens,omitempty"`
	AutoJoin      *bool              `json:"auto_join,omitempty"`
	CustomEnv     *map[string]string `json:"custom_env,omitempty"`
	CustomArgs    *[]string          `json:"custom_args,omitempty"`
}

type AgentResponse struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	OwnerID       string            `json:"owner_id"`
	ModelProvider string            `json:"model_provider"`
	ModelName     string            `json:"model_name"`
	SystemPrompt  string            `json:"system_prompt"`
	Temperature   float64           `json:"temperature"`
	MaxTokens     int               `json:"max_tokens"`
	IsActive      bool              `json:"is_active"`
	AutoJoin      bool              `json:"auto_join"`
	AvatarURL     string            `json:"avatar_url,omitempty"`
	CustomEnv     map[string]string `json:"custom_env,omitempty"`
	CustomArgs    []string          `json:"custom_args,omitempty"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
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

	modelName := req.ModelName

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

	customEnv := req.CustomEnv
	if customEnv == nil {
		customEnv = map[string]string{}
	}
	customEnvBytes, err := json.Marshal(customEnv)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid custom_env")
		return
	}

	customArgs := req.CustomArgs
	if customArgs == nil {
		customArgs = []string{}
	}
	customArgsBytes, err := json.Marshal(customArgs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid custom_args")
		return
	}

	// Auto-bind to the first online computer if available.
	var computerID *string
	var cid string
	compErr := h.pool.QueryRow(r.Context(),
		`SELECT id FROM computers WHERE status = 'online' ORDER BY created_at ASC LIMIT 1`,
	).Scan(&cid)
	if compErr == nil && cid != "" {
		computerID = &cid
	}

	var agentID string
	var createdAt, updatedAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO agents (name, description, owner_id, model_provider, model_name, system_prompt, temperature, max_tokens, auto_join, runtime_id, custom_env, custom_args)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id, created_at, updated_at`,
		name, req.Description, userID, modelProvider, modelName, systemPrompt, temperature, maxTokens, autoJoin,
		computerID,
		customEnvBytes, customArgsBytes,
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

	// Auto-join the user's #all-{name} channel so @mentions work immediately.
	var allID string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT c.id FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user' AND cm.member_id = $1
		 AND c.name LIKE 'all-%%' AND c.is_archived = false
		 LIMIT 1`,
		userID,
	).Scan(&allID)
	if allID != "" {
		_, _ = h.pool.Exec(r.Context(),
			`INSERT INTO channel_members (channel_id, member_type, member_id, role)
			 VALUES ($1, 'agent', $2, 'member')
			 ON CONFLICT DO NOTHING`,
			allID, agentID,
		)
	}

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
		CustomEnv:     customEnv,
		CustomArgs:    customArgs,
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
		        custom_env, custom_args,
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
		var customEnvBytes, customArgsBytes []byte
		err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
			&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
			&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
			&customEnvBytes, &customArgsBytes,
			&createdAt, &updatedAt)
		if err != nil {
			slog.Error("failed to scan agent row", "error", err)
			continue
		}
		a.CustomEnv = unmarshalStringMap(customEnvBytes)
		a.CustomArgs = unmarshalStringSlice(customArgsBytes)
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
	var customEnvBytes, customArgsBytes []byte
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description, ''), owner_id, model_provider, model_name,
		        system_prompt, temperature, max_tokens, is_active, auto_join, COALESCE(avatar_url, ''),
		        custom_env, custom_args,
		        created_at, updated_at
		 FROM agents
		 WHERE id = $1 AND owner_id = $2 AND is_active = true`,
		agentID, userID,
	).Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
		&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
		&customEnvBytes, &customArgsBytes,
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

	a.CustomEnv = unmarshalStringMap(customEnvBytes)
	a.CustomArgs = unmarshalStringSlice(customArgsBytes)
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

	// Marshal custom_env if provided; nil bytes means "don't update".
	var customEnvBytes []byte
	if req.CustomEnv != nil {
		var err error
		customEnvBytes, err = json.Marshal(*req.CustomEnv)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid custom_env")
			return
		}
	}

	// Marshal custom_args if provided; nil bytes means "don't update".
	var customArgsBytes []byte
	if req.CustomArgs != nil {
		var err error
		customArgsBytes, err = json.Marshal(*req.CustomArgs)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid custom_args")
			return
		}
	}

	var a AgentResponse
	var createdAt, updatedAt time.Time
	var retCustomEnvBytes, retCustomArgsBytes []byte
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
			custom_env = COALESCE($9, custom_env),
			custom_args = COALESCE($10, custom_args),
			updated_at = now()
		 WHERE id = $11 AND owner_id = $12 AND is_active = true
		 RETURNING id, name, COALESCE(description, ''), owner_id, model_provider, model_name,
		           system_prompt, temperature, max_tokens, is_active, auto_join, COALESCE(avatar_url, ''),
		           custom_env, custom_args,
		           created_at, updated_at`,
		req.Name, req.Description, req.SystemPrompt,
		req.ModelProvider, req.ModelName, req.Temperature, req.MaxTokens, req.AutoJoin,
		customEnvBytes, customArgsBytes,
		agentID, userID,
	).Scan(&a.ID, &a.Name, &a.Description, &a.OwnerID,
		&a.ModelProvider, &a.ModelName, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.IsActive, &a.AutoJoin, &a.AvatarURL,
		&retCustomEnvBytes, &retCustomArgsBytes,
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

	a.CustomEnv = unmarshalStringMap(retCustomEnvBytes)
	a.CustomArgs = unmarshalStringSlice(retCustomArgsBytes)
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

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	defer tx.Rollback(r.Context())

	result, err := tx.Exec(r.Context(),
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

	// Remove agent from non-DM channel members (DM channel memberships are kept
	// so the DM list still shows the conversation, with a "deleted" indicator)
	_, err = tx.Exec(r.Context(),
		`DELETE FROM channel_members
		 WHERE member_type = 'agent' AND member_id = $1
		 AND channel_id NOT IN (
			 SELECT id FROM channels WHERE type = 'dm'
		 )`,
		agentID,
	)
	if err != nil {
		slog.Error("failed to remove agent from channels", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	slog.Info("agent deactivated", "agent_id", agentID, "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "agent deleted"})
}

// AgentSkills handles GET /api/v1/agents/{agentID}/skills — proxies to daemon.
func (h *AgentHandler) AgentSkills(w http.ResponseWriter, r *http.Request) {
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

	// Verify agent ownership.
	var ownerID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM agents WHERE id = $1 AND is_active = true`, agentID,
	).Scan(&ownerID)
	if err != nil || ownerID != userID {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	if h.proxy == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"skills": []interface{}{}})
		return
	}

	d, ok := h.proxy.FindDaemonForAgent(r.Context(), agentID)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{"skills": []interface{}{}})
		return
	}

	data, err := h.proxy.ProxySkillList(r.Context(), d, agentID)
	if err != nil {
		slog.Warn("agent skills: daemon proxy failed", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{"skills": []interface{}{}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// AgentBackends handles GET /api/v1/agent-backends
func (h *AgentHandler) AgentBackends(w http.ResponseWriter, r *http.Request) {
	metas := agent.GlobalRegistry().ListMeta()
	if metas == nil {
		metas = []agent.AdapterMeta{}
	}
	writeJSON(w, http.StatusOK, metas)
}

// AgentBackendsDetect handles GET /api/v1/agent-backends/detect
func (h *AgentHandler) AgentBackendsDetect(w http.ResponseWriter, r *http.Request) {
	results := agent.GlobalRegistry().Detect()
	if results == nil {
		results = []agent.BackendStatus{}
	}
	writeJSON(w, http.StatusOK, results)
}

// --- JSONB helpers ---

// unmarshalStringMap deserializes JSON bytes into a map[string]string.
// Returns an empty map when b is nil or empty.
func unmarshalStringMap(b []byte) map[string]string {
	if len(b) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		slog.Warn("failed to unmarshal string map", "error", err)
		return map[string]string{}
	}
	if m == nil {
		return map[string]string{}
	}
	return m
}

// unmarshalStringSlice deserializes JSON bytes into a []string.
// Returns an empty slice when b is nil or empty.
func unmarshalStringSlice(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	var s []string
	if err := json.Unmarshal(b, &s); err != nil {
		slog.Warn("failed to unmarshal string slice", "error", err)
		return []string{}
	}
	if s == nil {
		return []string{}
	}
	return s
}

// workspaceNode represents a file or directory in the agent workspace tree.
type workspaceNode struct {
	Type     string           `json:"type"` // "file" or "directory"
	Name     string           `json:"name"`
	Path     string           `json:"path,omitempty"`
	Content  string           `json:"content,omitempty"`
	Size     int64            `json:"size,omitempty"`
	Children []workspaceNode  `json:"children,omitempty"`
}

// Workspace handles GET /api/v1/agents/{agentID}/workspace
//
// Returns a file tree for the agent's workspace directory. Supports query params:
//   - path: subdirectory or file path relative to workspace (default: "workspace")
//   - content: "true" to include file content (default: false)
//
// This is a read-only API. No write operations are supported.
func (h *AgentHandler) Workspace(w http.ResponseWriter, r *http.Request) {
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

	// Verify the agent exists and belongs to the user.
	var ownerID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM agents WHERE id = $1 AND is_active = true`,
		agentID,
	).Scan(&ownerID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		slog.Error("workspace: failed to query agent", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// Resolve the workspace base directory: ~/.solo/agents/<agentID>/workspace
	workspaceDir := filepath.Join(h.workspaceRoot, agentID, "workspace")

	// Parse query params.
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		relPath = "."
	}
	includeContent := r.URL.Query().Get("content") == "true"

	// Try proxy to daemon first
	if h.proxy != nil {
		if d, ok := h.proxy.FindDaemonForAgent(r.Context(), agentID); ok {
			if includeContent {
				data, err := h.proxy.ProxyWorkspaceRead(r.Context(), d, agentID, relPath)
				if err == nil {
					w.Header().Set("Content-Type", "application/json")
					w.Write(data)
					return
				}
				slog.Warn("workspace: daemon proxy read failed, falling back to local filesystem", "error", err)
			} else {
				data, err := h.proxy.ProxyWorkspaceList(r.Context(), d, agentID, relPath)
				if err == nil {
					w.Header().Set("Content-Type", "application/json")
					w.Write(data)
					return
				}
				slog.Warn("workspace: daemon proxy list failed, falling back to local filesystem", "error", err)
			}
		}
	}

	// Fallback: local filesystem reading
	// Prevent path traversal: resolve and verify it stays within workspaceDir.
	fullPath := filepath.Clean(filepath.Join(workspaceDir, relPath))
	if !strings.HasPrefix(fullPath, workspaceDir) {
		writeError(w, http.StatusBadRequest, "path traversal not allowed")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "path not found in workspace")
			return
		}
		slog.Error("workspace: failed to stat path", "path", fullPath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var node workspaceNode
	if info.IsDir() {
		node, err = h.buildFileTree(fullPath, workspaceDir, 0, includeContent)
		if err != nil {
			slog.Error("workspace: failed to build file tree", "path", fullPath, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to read workspace")
			return
		}
	} else {
		node, err = h.buildFileNode(fullPath, workspaceDir, includeContent)
		if err != nil {
			slog.Error("workspace: failed to read file", "path", fullPath, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
	}

	writeJSON(w, http.StatusOK, node)
}

// buildFileTree recursively builds a workspaceNode tree for a directory.
func (h *AgentHandler) buildFileTree(dirPath, basePath string, depth int, includeContent bool) (workspaceNode, error) {
	if depth > maxWorkspaceDepth {
		return workspaceNode{}, nil
	}

	name := filepath.Base(dirPath)
	if dirPath == basePath {
		name = "."
	}

	node := workspaceNode{
		Type:     "directory",
		Name:     name,
		Path:     dirPath,
		Children: []workspaceNode{},
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return node, err
	}

	for _, entry := range entries {
		// Skip hidden files and directories.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		if entry.IsDir() {
			child, err := h.buildFileTree(fullPath, basePath, depth+1, includeContent)
			if err != nil {
				slog.Warn("workspace: failed to read subdirectory", "path", fullPath, "error", err)
				continue
			}
			node.Children = append(node.Children, child)
		} else {
			child, err := h.buildFileNode(fullPath, basePath, includeContent)
			if err != nil {
				slog.Warn("workspace: failed to read file", "path", fullPath, "error", err)
				continue
			}
			node.Children = append(node.Children, child)
		}
	}

	return node, nil
}

// buildFileNode creates a workspaceNode for a single file.
func (h *AgentHandler) buildFileNode(filePath, basePath string, includeContent bool) (workspaceNode, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return workspaceNode{}, err
	}

	node := workspaceNode{
		Type: "file",
		Name: info.Name(),
		Path: filePath,
		Size: info.Size(),
	}

	if includeContent {
		if info.Size() > maxWorkspaceFileSize {
			node.Content = "[file too large to preview]"
		} else {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return node, err
			}
			// Only include text content; binary files are skipped.
			if isTextFile(data) {
				node.Content = string(data)
			} else {
				node.Content = "[binary file]"
			}
		}
	}

	return node, nil
}

// isTextFile detects whether byte data represents text content by checking
// for null bytes (common in binary files).
func isTextFile(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Check first 8KB for null bytes.
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for _, b := range data[:checkLen] {
		if b == 0 {
			return false
		}
	}
	return true
}

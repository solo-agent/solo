package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/pkg/llm"
)

// agentTokenState holds a cached JWT for an agent plus its expiry.
type agentTokenState struct {
	accessToken string
	expiresAt   time.Time
}

// daemonHandler holds the daemon-side HTTP handlers.
type daemonHandler struct {
	taskManager      *taskManager
	providers        map[string]llm.Provider
	pool             *pgxpool.Pool
	serverURL        string
	internalToken    string
	httpClient       *http.Client
	mu               sync.Mutex
	workspaceManager *agent.WorkspaceManager
	memoryManager    *agent.MemoryManager
	sessionManagers  map[string]*agent.AgentSessionManager // v1.4: per-provider persistent sessions

	// v1.4: per-agent token store for persistent session token auto-refresh (SOLO-254-B).
	agentTokens   map[string]*agentTokenState // agentID -> cached token + expiry
	agentTokensMu sync.RWMutex
}

// newDaemonHandler creates a new daemon HTTP handler.
// provider is the default LLM provider (used when no per-task override exists).
func newDaemonHandler(pool *pgxpool.Pool, tm *taskManager, provider llm.Provider, serverURL, internalToken string) *daemonHandler {
	return &daemonHandler{
		taskManager: tm,
		providers: map[string]llm.Provider{
			"": provider, // default provider key
		},
		pool:             pool,
		serverURL:        serverURL,
		internalToken:    internalToken,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		workspaceManager: agent.NewWorkspaceManager(""),
		memoryManager:    agent.NewMemoryManager(""),
		agentTokens:      make(map[string]*agentTokenState),
	}
}

// SetSessionManager registers a session manager for a provider type.
func (h *daemonHandler) SetSessionManager(providerType string, sm *agent.AgentSessionManager) {
	if h.sessionManagers == nil {
		h.sessionManagers = make(map[string]*agent.AgentSessionManager)
	}
	h.sessionManagers[providerType] = sm
}

// getSessionManager returns the session manager for the given provider type.
func (h *daemonHandler) getSessionManager(providerType string) *agent.AgentSessionManager {
	if h.sessionManagers == nil {
		return nil
	}
	return h.sessionManagers[providerType]
}

// ── Token store (SOLO-254-B: persistent session token auto-refresh) ────────────

// getOrGenerateToken returns a cached valid token for the agent, or generates a
// new one. This is the primary entry point — proxy requests and task dispatches
// both use it so the daemon can track token expiry and refresh proactively.
func (h *daemonHandler) getOrGenerateToken(ctx context.Context, agentID string) (string, error) {
	h.agentTokensMu.RLock()
	st, ok := h.agentTokens[agentID]
	h.agentTokensMu.RUnlock()

	if ok && time.Now().Before(st.expiresAt) {
		return st.accessToken, nil
	}

	// Cache miss — try disk (survives daemon restarts).
	if diskSt := h.loadTokenFromDisk(agentID); diskSt != nil {
		h.agentTokensMu.Lock()
		h.agentTokens[agentID] = diskSt
		h.agentTokensMu.Unlock()
		return diskSt.accessToken, nil
	}

	return h.generateAndStoreToken(ctx, agentID)
}

// generateAndStoreToken creates a fresh access token for the agent, stores it in
// the cache, and returns it. Caller must NOT hold agentTokensMu.
func (h *daemonHandler) generateAndStoreToken(ctx context.Context, agentID string) (string, error) {
	var agentName string
	_ = h.pool.QueryRow(ctx, `SELECT COALESCE(name, $1) FROM agents WHERE id = $1`, agentID).Scan(&agentName)
	if agentName == "" {
		agentName = agentID
	}

	token, err := auth.GenerateAgentToken(agentID, agentName)
	if err != nil {
		return "", err
	}

	st := &agentTokenState{
		accessToken: token,
		expiresAt:   time.Now().Add(auth.AgentAccessTokenDuration),
	}

	h.agentTokensMu.Lock()
	h.agentTokens[agentID] = st
	h.agentTokensMu.Unlock()

	// Persist to disk so agent sessions survive daemon restarts.
	h.saveTokenToDisk(agentID, token, st.expiresAt)

	slog.Debug("daemon: generated new agent token", "agent_id", agentID, "expires_at", st.expiresAt.Format(time.RFC3339))
	return token, nil
}

// tokenDir returns the directory for persistent agent token storage.
func tokenDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".solo", "agent-tokens")
}

// saveTokenToDisk persists an agent token to disk (JWT only — expiry in claims).
func (h *daemonHandler) saveTokenToDisk(agentID, token string, _ time.Time) {
	dir := filepath.Join(tokenDir(), agentID)
	os.MkdirAll(dir, 0700)
	if err := os.WriteFile(filepath.Join(dir, "current.token"), []byte(token), 0600); err != nil { slog.Warn("daemon: failed to persist agent token", "agent_id", agentID, "error", err) }
}

// loadTokenFromDisk reads a persisted token. Returns nil if not found.
func (h *daemonHandler) loadTokenFromDisk(agentID string) *agentTokenState {
	file := filepath.Join(tokenDir(), agentID, "current.token")
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return nil
	}
	return &agentTokenState{
		accessToken: token,
		expiresAt:   time.Now().Add(auth.AgentAccessTokenDuration),
	}
}

// refreshToken regenerates the agent token unconditionally (used on 401 retry).
func (h *daemonHandler) refreshToken(ctx context.Context, agentID string) (string, error) {
	h.agentTokensMu.Lock()
	delete(h.agentTokens, agentID)
	h.agentTokensMu.Unlock()

	slog.Info("daemon: token refreshed (401 retry)", "agent_id", agentID)
	return h.generateAndStoreToken(ctx, agentID)
}

// storeTokenFromTask records a token generated during task dispatch so the
// background goroutine knows about it. Unlike getOrGenerateToken, this doesn't
// query the agent name from DB (we already have it).
func (h *daemonHandler) storeTokenFromTask(agentID, token string) {
	st := &agentTokenState{
		accessToken: token,
		expiresAt:   time.Now().Add(auth.AgentAccessTokenDuration),
	}
	h.agentTokensMu.Lock()
	h.agentTokens[agentID] = st
	h.agentTokensMu.Unlock()
}

// fetchChannelAgentWorkspaces returns (name, workspace) pairs for other active
// agents in the given channel, excluding the requesting agent itself.
func (h *daemonHandler) fetchChannelAgentWorkspaces(ctx context.Context, channelID, excludeAgentID string) ([]agent.AgentWorkspace, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT a.name, a.workspace_path FROM agents a
		 INNER JOIN channel_members cm ON cm.member_id = a.id AND cm.member_type = 'agent'
		 WHERE cm.channel_id = $1 AND a.is_active = true AND a.id != $2`,
		channelID, excludeAgentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []agent.AgentWorkspace
	for rows.Next() {
		var aw agent.AgentWorkspace
		if err := rows.Scan(&aw.Name, &aw.Workspace); err != nil {
			return nil, err
		}
		result = append(result, aw)
	}
	return result, rows.Err()
}


// holdAndRevise injects pending messages into the agent session so it can
// revise its draft based on the latest context. Returns the revised content.
func (h *daemonHandler) holdAndRevise(ctx context.Context, req runTaskRequest, draftContent string, pending []agent.Message) (string, bool) {
	if len(pending) == 0 {
		return draftContent, false
	}

	var revBuilder strings.Builder
	revBuilder.WriteString("Newer messages arrived while you were composing. Your draft may be stale.\n\n")
	revBuilder.WriteString("## New Messages\n")
	for _, m := range pending {
		revBuilder.WriteString(m.Content)
		revBuilder.WriteString("\n")
	}
	revBuilder.WriteString("\n## Your Draft\n")
	revBuilder.WriteString(draftContent)
	revBuilder.WriteString("\n\nIf your draft is still accurate, reply with the same text. If the new messages change anything, revise. Reply NOW.")

	pendingMsgs := []agent.Message{
		{Role: agent.RoleUser, Content: revBuilder.String()},
	}

	sm := h.getSessionManager(req.ModelConfig.Provider); if sm == nil { slog.Warn("task: holdAndRevise: no session manager", "provider", req.ModelConfig.Provider); return draftContent, false }; ps, err := sm.DeliverMessage(ctx, req.AgentID, pendingMsgs)
	if err != nil {
		slog.Warn("task: holdAndRevise failed to deliver", "agent_id", req.AgentID, "error", err)
		return draftContent, false
	}

	// Collect the revised response.
	var revised strings.Builder
	for chunk := range ps.Messages {
		if chunk.Type == string(agent.MessageText) {
			revised.WriteString(chunk.Content)
		}
	}
	result := <-ps.Result

	finalText := revised.String()
	if result.Output != "" {
		finalText = result.Output
	}
	if strings.TrimSpace(finalText) == "" {
		return draftContent, false
	}
	return finalText, true
}


// ProxyRequest handles POST /internal/daemon/proxy
// Agents call this local endpoint instead of hitting the server API directly.
// The daemon adds auth and forwards the request to the server. This keeps
// local thinking separate from channel communication (Slock-aligned).
func (h *daemonHandler) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	// Only accept from localhost — this is an internal agent proxy.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "proxy only accessible from localhost"})
		return
	}

	var req struct {
		AgentID   string `json:"agent_id"`
		Action    string `json:"action"`
		ChannelID string `json:"channel_id"`
		Content   string `json:"content,omitempty"`
		ThreadID  string `json:"thread_id,omitempty"`
		TaskNumber int    `json:"task_number,omitempty"`
			TaskID    string `json:"task_id,omitempty"`
		Status    string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Generate or reuse auth token for this agent (SOLO-254-B: token store).
	token, err := h.getOrGenerateToken(r.Context(), req.AgentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate auth token"})
		return
	}

	// Build and forward the request to the server
	var serverPath string
	var serverBody []byte
	switch req.Action {
	case "message_send":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/messages", req.ChannelID)
		bodyMap := map[string]string{"content": req.Content}
		if req.ThreadID != "" { bodyMap["thread_id"] = req.ThreadID }
		serverBody, _ = json.Marshal(bodyMap)
	case "task_claim":
		if req.TaskID != "" {
			serverPath = fmt.Sprintf("/api/v1/channels/%s/tasks/%s/claim", req.ChannelID, req.TaskID)
		} else {
			serverPath = fmt.Sprintf("/api/v1/channels/%s/tasks/%d/claim", req.ChannelID, req.TaskNumber)
		}
	case "task_update":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/tasks/%d", req.ChannelID, req.TaskNumber)
		serverBody, _ = json.Marshal(map[string]string{"status": req.Status})
	case "task_unclaim":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/tasks/%d/claim", req.ChannelID, req.TaskNumber)
	case "channel_members":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/members", req.ChannelID)
	case "server_info":
		serverPath = "/api/v1/server/info"
	case "message_read":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/messages", req.ChannelID)
	case "message_check":
		serverPath = fmt.Sprintf("/api/v1/messages/check?channel_id=%s", req.ChannelID)
	case "channel_join":
		serverPath = "/api/v1/channels/join"
		serverBody, _ = json.Marshal(map[string]string{"target": req.Content})
	case "thread_unfollow":
		serverPath = "/api/v1/threads/unfollow"
		serverBody, _ = json.Marshal(map[string]string{"target": req.Content})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action: " + req.Action})
		return
	}

	// Helper to forward the request with a given token.
	forwardRequest := func(tok string) (*http.Response, []byte, error) {
		serverURL := h.serverURL + serverPath
		method := "GET"
		var fwdBody io.Reader
		switch req.Action {
		case "task_claim":
			method = "POST"
		case "task_update":
			method = "PATCH"
			fwdBody = bytes.NewReader(serverBody)
		case "task_unclaim":
			method = "DELETE"
		default:
			if serverBody != nil {
				method = "POST"
				fwdBody = bytes.NewReader(serverBody)
			}
		}
		httpReq, err := http.NewRequestWithContext(r.Context(), method, serverURL, fwdBody)
		if err != nil {
			return nil, nil, err
		}
		if serverBody != nil || req.Action == "task_update" {
			httpReq.Header.Set("Content-Type", "application/json")
		}
		httpReq.Header.Set("Authorization", "Bearer "+tok)

		resp, fwdErr := h.httpClient.Do(httpReq)
		if fwdErr != nil {
			return nil, nil, fwdErr
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, respBody, nil
	}

	resp, body, err := forwardRequest(token)
	if err != nil {
		slog.Error("proxy: server request failed", "action", req.Action, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "server unreachable"})
		return
	}

	// SOLO-254-B: On 401, refresh the token and retry once.
	if resp.StatusCode == http.StatusUnauthorized {
		slog.Info("proxy: got 401, refreshing token and retrying", "agent_id", req.AgentID, "action", req.Action)
		newToken, refreshErr := h.refreshToken(r.Context(), req.AgentID)
		if refreshErr != nil {
			slog.Error("proxy: token refresh failed after 401", "agent_id", req.AgentID, "error", refreshErr)
		} else {
			resp, body, err = forwardRequest(newToken)
			if err != nil {
				slog.Error("proxy: retry request failed", "action", req.Action, "error", err)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "server unreachable"})
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// getProvider returns the LLM provider for the given provider type.
// If no specific provider is registered for that type, it falls back
// to the default provider. Providers are lazily created on first use.
func (h *daemonHandler) getProvider(providerType string) llm.Provider {
	if providerType == "" {
		return h.providers[""]
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.providers[providerType]; ok {
		return p
	}

	// Lazy-create provider based on type
	var p llm.Provider
	apiKey := os.Getenv("LLM_API_KEY")
	switch providerType {
	case "anthropic":
		p = llm.NewAnthropicProvider(apiKey)
	case "openai":
		p = llm.NewOpenAIProvider(apiKey)
	case "local":
		p = llm.NewLocalProvider("")
	default:
		p = h.providers[""]
	}

	h.providers[providerType] = p
	slog.Info("lazy-created LLM provider", "type", providerType)
	return p
}

// --- Request types (server -> daemon) ---

// runTaskRequest is the payload sent by the server to the daemon.
type runTaskRequest struct {
	TaskID       string             `json:"task_id"`
	AgentID      string             `json:"agent_id"`
	ChannelID    string             `json:"channel_id"`
	ThreadID     string             `json:"thread_id,omitempty"`
	Messages     []llmMessage       `json:"messages"`
	SystemPrompt string             `json:"system_prompt"`
	ModelConfig  modelConfigPayload `json:"model_config"`
	TaskContext  string             `json:"task_context,omitempty"`   // SOLO-221-B: summary of pending tasks in channel
	MentionedNames []string         `json:"mentioned_names,omitempty"` // v1.3: names of @mentioned agents
}

type llmMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	SenderID string `json:"sender_id,omitempty"`
}

type modelConfigPayload struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

// runTaskResponse is returned after accepting a task.
type runTaskResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// Run handles POST /internal/daemon/run
// Server sends an agent execution task here.
func (h *daemonHandler) Run(w http.ResponseWriter, r *http.Request) {
	var req runTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields
	if req.TaskID == "" {
		req.TaskID = uuid.New().String()
	}
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}
	if req.ChannelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id is required"})
		return
	}
	if req.ModelConfig.MaxTokens <= 0 {
		req.ModelConfig.MaxTokens = 4096
	}
	if req.ModelConfig.Temperature <= 0 {
		req.ModelConfig.Temperature = 0.7
	}

	// Register the task
	h.taskManager.AddTask(req.TaskID, &taskState{
		TaskID:     req.TaskID,
		AgentID:    req.AgentID,
		ChannelID:  req.ChannelID,
		ThreadID:   req.ThreadID,
		Status:     taskStatusRunning,
		ReceivedAt: time.Now(),
	})

	slog.Info("task received",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"channel_id", req.ChannelID,
		"model", req.ModelConfig.Model,
		"provider", req.ModelConfig.Provider,
	)

	// Return 202 Accepted immediately
	writeJSON(w, http.StatusAccepted, runTaskResponse{
		TaskID: req.TaskID,
		Status: "running",
	})

	// Process the task asynchronously with streaming
	ctx, cancel := context.WithCancel(context.Background())
	h.taskManager.SetCancelFunc(req.TaskID, cancel)
	go h.processTaskStreaming(ctx, req)
}

// processTaskStreaming executes the LLM call in streaming mode and pushes events.
// It tries the Backend interface first (for all registered CLI backends);
// falls back to the old LLM provider path for API-based providers.
func (h *daemonHandler) processTaskStreaming(ctx context.Context, req runTaskRequest) {
	if backend, err := agent.NewBackend(req.ModelConfig.Provider, os.Getenv("LLM_API_KEY")); err == nil {
		h.processTaskWithBackend(ctx, req, backend)
		return
	}

	// Fallback: use old LLM provider path.
	h.processTaskWithProvider(ctx, req)
}

// processTaskWithProvider runs a task using the old LLM provider interface.
// This is the fallback path for providers not supported by the Backend
// interface (e.g. openai, anthropic).
func (h *daemonHandler) processTaskWithProvider(ctx context.Context, req runTaskRequest) {
	h.taskManager.UpdateStatus(req.TaskID, taskStatusThinking)
	slog.Info("task processing started (streaming)", "task_id", req.TaskID, "agent_id", req.AgentID)

	// Push thinking event
	h.pushEventJSON(req.TaskID, "thinking", map[string]string{
		"agent_id": req.AgentID,
		"thought":  "Processing request...",
	})

	// Build LLM request
	llmMsgs := make([]llm.Message, len(req.Messages))
	for i, m := range req.Messages {
		llmMsgs[i] = llm.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}


		// SOLO-221-B: Append task context to the system prompt so agents
		// can see pending tasks in the channel.
		systemPrompt := req.SystemPrompt
		if req.TaskContext != "" {
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += req.TaskContext
		}

	llmReq := &llm.CompletionRequest{
		Model:        req.ModelConfig.Model,
		Messages:     llmMsgs,
		SystemPrompt: systemPrompt,
		Temperature:  req.ModelConfig.Temperature,
		MaxTokens:    req.ModelConfig.MaxTokens,
	}

	// Select the provider matching the agent type and stream
	provider := h.getProvider(req.ModelConfig.Provider)
	streamCh, err := provider.CompleteStream(ctx, llmReq)
	if err != nil {
		slog.Error("task: streaming LLM call failed to start",
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
			"error", err,
		)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
		h.notifyServerError(req, err.Error())

		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id":  req.AgentID,
			"error":     err.Error(),
			"retryable": true,
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	// Collect full content from stream
	var fullContent string
	var usage llm.Usage

	for chunk := range streamCh {
		if chunk.Error != nil {
			slog.Error("task: streaming LLM call error",
				"task_id", req.TaskID,
				"error", chunk.Error,
			)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
			h.notifyServerError(req, chunk.Error.Error())

			h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
				"agent_id":  req.AgentID,
				"error":     chunk.Error.Error(),
				"retryable": true,
			})
			h.taskManager.CloseAllSubscribers(req.TaskID)
			return
		}

		if chunk.Done {
			// Final chunk with usage info
			usage = chunk.Usage
			break
		}

		// Accumulate content
		fullContent += chunk.Content

		// Push token event
		h.pushEventJSON(req.TaskID, "token", map[string]interface{}{
			"agent_id": req.AgentID,
			"content":  chunk.Content,
		})
	}

	// Check if context was cancelled (user stop)
	if ctx.Err() != nil {
		slog.Info("task cancelled via context", "task_id", req.TaskID, "agent_id", req.AgentID)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusCancelled)
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	slog.Info("task streaming completed",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"content_length", len(fullContent),
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
	)

	// The server-side handler receives the "complete" SSE event and persists
	// the message to the database. The daemon only streams content via SSE.
	h.taskManager.UpdateStatus(req.TaskID, taskStatusCompleted)
	slog.Info("task completed", "task_id", req.TaskID)

	// Push complete event -- server will persist from this
	h.pushEventJSON(req.TaskID, "complete", map[string]interface{}{
		"agent_id": req.AgentID,
		"content":  fullContent,
		"usage": map[string]int{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		},
	})

	// Also notify the server via direct HTTP callback as a fallback,
	// in case the SSE stream has timing issues.
	_ = uuid.New().String() // messageID kept for symmetry with SSE path

	// Push a done sentinel event before closing, so SSE subscribers consume
	// events in order without needing a delay. The "done" event signals the
	// end of the stream; the SSE handler will exit cleanly after receiving it.
	h.pushEventJSON(req.TaskID, "done", map[string]interface{}{})
	h.taskManager.CloseAllSubscribers(req.TaskID)
}

// processTaskWithBackend runs a task using the new agent.Backend interface.
// It prepares the workspace, loads memory, builds the system prompt, and
// executes the agent, streaming output chunks as SSE events.
func (h *daemonHandler) processTaskWithBackend(ctx context.Context, req runTaskRequest, backend agent.Backend) {
	h.taskManager.UpdateStatus(req.TaskID, taskStatusThinking)
	slog.Info("task processing started (backend)",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"backend", backend.Name(),
		"model", req.ModelConfig.Model,
	)

	// Push initial thinking event
	h.pushEventJSON(req.TaskID, "thinking", map[string]string{
		"agent_id": req.AgentID,
		"thought":  "Preparing workspace...",
	})

	// Fetch agent info from DB for name and system prompt
	agentInfo, err := h.fetchAgentInfo(ctx, req.AgentID)
	if err != nil {
		slog.Error("task: failed to fetch agent info", "task_id", req.TaskID, "agent_id", req.AgentID, "error", err)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
		h.notifyServerError(req, "failed to load agent configuration: "+err.Error())
		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id": req.AgentID, "error": "failed to load agent configuration", "retryable": false,
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	// Prepare workspace (idempotent — safe to call every time)
	ws, err := h.workspaceManager.Prepare(req.AgentID, &agent.AgentConfig{
		AgentID:      req.AgentID,
		Name:         agentInfo.Name,
		SystemPrompt: req.SystemPrompt,
		Model:        req.ModelConfig.Model,
		Provider:     req.ModelConfig.Provider,
		MaxTokens:    req.ModelConfig.MaxTokens,
		Temperature:  req.ModelConfig.Temperature,
	})
	if err != nil {
		slog.Error("task: workspace preparation failed", "task_id", req.TaskID, "error", err)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
		h.notifyServerError(req, "workspace preparation failed: "+err.Error())
		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id": req.AgentID, "error": "workspace preparation failed", "retryable": true,
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	// Load memory content
	memoryContent, _ := h.memoryManager.Load(req.AgentID)

	// Fetch channel name for context
	channelName, _ := h.fetchChannelName(ctx, req.ChannelID)

	// Determine trigger type.
	triggerType := agent.TriggerChat
	if req.ThreadID != "" {
		triggerType = agent.TriggerThread
	}
	for _, name := range req.MentionedNames {
		if strings.EqualFold(name, agentInfo.Name) {
			triggerType = agent.TriggerMention
			break
		}
	}

	// Build channel context
	channelCtx := agent.ChannelContext{
		ChannelID:   req.ChannelID,
		ChannelName: channelName,
		TriggerType: triggerType,
	}

	// SOLO-254-B: Obtain a JWT via the token store so the background refresh
	// goroutine keeps it fresh for the lifetime of the persistent session.
	// The agent authenticates as itself — it's a channel member.
	// Must be generated before agentCfg so it's available for persistent session creation.
	agentToken, err := h.getOrGenerateToken(ctx, req.AgentID)
	if err != nil {
		slog.Warn("task: failed to generate agent token — agent cannot call APIs", "agent_id", req.AgentID, "error", err)
	}
	agentEnv := map[string]string{
		"SOLO_AGENT_ID":   req.AgentID,
		"SOLO_AGENT_NAME": agentInfo.Name,
	}
	if agentToken != "" {
		agentEnv["SOLO_AUTH_TOKEN"] = agentToken
	}
	// Merge agent-level custom_env over base agentEnv (agent wins).
	for k, v := range agentInfo.CustomEnv {
		agentEnv[k] = v
	}

	// Build system prompt using PromptBuilder
	hostname, _ := os.Hostname()
	agentCfg := agent.AgentConfig{
		AgentID:       req.AgentID,
		Name:          agentInfo.Name,
		SystemPrompt:  req.SystemPrompt,
		Model:         req.ModelConfig.Model,
		Provider:      req.ModelConfig.Provider,
		MaxTokens:     req.ModelConfig.MaxTokens,
		Temperature:   req.ModelConfig.Temperature,
		CustomArgs:    agentInfo.CustomArgs,
		Env:           agentEnv,
		WorkspacePath: ws.WorkDir,
		ServerID:      h.serverURL,
		Hostname:      hostname,
		OS:            runtime.GOOS + " " + runtime.GOARCH,
	}
	systemPrompt := agent.BuildSystemPrompt(agentCfg, channelCtx, memoryContent, req.MentionedNames)

		// SOLO-221-B: Include task context (pending channel tasks) in the prompt
		// so agents can decide whether to claim tasks.
		if req.TaskContext != "" {
			systemPrompt += "\n\n" + req.TaskContext
		}


	// Inject runtime configuration into workspace
	if err := h.workspaceManager.InjectConfig(ctx, req.AgentID, &channelCtx); err != nil {
		slog.Warn("task: InjectConfig failed (non-fatal)", "task_id", req.TaskID, "error", err)
	}



	// Convert messages to agent.Message format
	msgs := make([]agent.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = agent.Message{
			Role:     agent.Role(m.Role),
			Content:  m.Content,
			SenderID: m.SenderID,
		}
	}

	h.pushEventJSON(req.TaskID, "thinking", map[string]string{
		"agent_id": req.AgentID,
		"thought":  "Processing...",
	})

	// Execute via Backend
	executeReq := &agent.ExecuteRequest{
		AgentID:  req.AgentID,
		Messages: msgs,
	}
	// Inject solo binary into workspace so agents can run solo CLI commands.
	// The workspace is Claude Code's CWD, so ./solo is immediately accessible.
	// Try PATH first, then fall back to the daemon binary's directory.
	soloPath, _ := exec.LookPath("solo")
	if soloPath == "" {
		if exe, err := os.Executable(); err == nil {
			soloPath = filepath.Join(filepath.Dir(exe), "solo")
		}
	}
	if soloPath != "" {
		soloDest := filepath.Join(ws.WorkDir, "solo")
		if copyErr := copyFile(soloPath, soloDest, 0755); copyErr != nil {
			slog.Warn("task: failed to copy solo binary to workspace", "solo_path", soloPath, "error", copyErr)
		}
	} else {
		slog.Warn("task: solo binary not found — agents cannot use solo CLI")
	}

	executeOpts := &agent.ExecuteOptions{
		SystemPrompt: systemPrompt,
		WorkspaceDir: ws.WorkDir,
		Model:        req.ModelConfig.Model,
		MaxTokens:    req.ModelConfig.MaxTokens,
		Env:          agentEnv,
		Temperature:  req.ModelConfig.Temperature,
		CustomArgs:   agentInfo.CustomArgs,
		// ExtraArgs: daemonConfig.ExtraArgs[backend.Name()], // P1 reserved
	}

	// v1.3: Session-aware dispatch (Slock-aligned).
	// For Claude backend, use persistent sessions via AgentSessionManager.
	// Falls back to backend.Execute() for non-persistent backends.
	var session *agent.Session
	if _, isPersistent := backend.(agent.PersistentBackend); isPersistent && h.getSessionManager(req.ModelConfig.Provider) != nil {
		if h.getSessionManager(req.ModelConfig.Provider).IsActive(req.AgentID) {
			slog.Info("task: reusing persistent session", "agent_id", req.AgentID)
			ps, psErr := h.getSessionManager(req.ModelConfig.Provider).DeliverMessage(ctx, req.AgentID, msgs)
			if psErr == nil {
				session = &agent.Session{Messages: ps.Messages, Result: ps.Result, Stop: ps.Stop}
			} else {
				slog.Warn("task: session delivery failed", "agent_id", req.AgentID, "error", psErr)
			}
		} else {
			_, _ = h.refreshToken(ctx, req.AgentID)
			slog.Info("task: creating persistent session", "agent_id", req.AgentID)
			ps, psErr := h.getSessionManager(req.ModelConfig.Provider).GetOrCreateSession(ctx, req.AgentID, agentCfg, channelCtx, msgs, req.MentionedNames)
			if psErr == nil {
				session = &agent.Session{Messages: ps.Messages, Result: ps.Result, Stop: ps.Stop}
			} else {
				slog.Warn("task: session creation failed, falling back to Execute", "agent_id", req.AgentID, "error", psErr)
			}
		}
	}

	if session == nil {
		var execErr error
		session, execErr = backend.Execute(ctx, executeReq, executeOpts)
		if execErr != nil {
			slog.Error("task: Backend.Execute failed", "task_id", req.TaskID, "error", execErr)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
			h.notifyServerError(req, execErr.Error())
			h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
				"agent_id": req.AgentID, "error": execErr.Error(), "retryable": true,
			})
			h.taskManager.CloseAllSubscribers(req.TaskID)
			return
		}
	}

	// Stream output chunks
	var fullContent string
	var messageSentViaCLI bool

	for chunk := range session.Messages {
		switch chunk.Type {
		case string(agent.MessageText):
			// v1.3: Slock-aligned — text output is internal thinking.
			// Forward as SSE for agent view. Chat messages via solo message send (proxy→API)
			// delivers visible messages via message.new WebSocket events.
			fullContent += chunk.Content
			h.pushEventJSON(req.TaskID, "text", map[string]interface{}{
				"agent_id":   req.AgentID,
				"agent_name": agentInfo.Name,
				"content":    chunk.Content,
			})

		case string(agent.MessageThinking):
			h.pushEventJSON(req.TaskID, "thinking", map[string]interface{}{
				"agent_id": req.AgentID,
				"thought":  chunk.Content,
			})

		case string(agent.MessageError):
			slog.Error("task: backend stream error", "task_id", req.TaskID, "error", chunk.Content)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
			h.notifyServerError(req, chunk.Content)
			h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
				"agent_id": req.AgentID, "error": chunk.Content, "retryable": true,
			})
			h.taskManager.CloseAllSubscribers(req.TaskID)
			return

		case string(agent.MessageToolUse):
			if chunk.Tool != nil {
				// Detect solo message send for Slock-aligned routing
				if chunk.Tool.Name == "Bash" {
					if input, ok := chunk.Tool.Input["command"].(string); ok {
						if strings.Contains(input, "solo message send") {
							messageSentViaCLI = true
						}
					}
				}
				// Forward tool_use as SSE for agent view
				inputJSON, _ := json.Marshal(chunk.Tool.Input)
				h.pushEventJSON(req.TaskID, "tool_use", map[string]interface{}{
					"agent_id":   req.AgentID,
					"agent_name": agentInfo.Name,
					"tool_name":  chunk.Tool.Name,
					"tool_input": string(inputJSON),
					"call_id":    chunk.Tool.CallID,
				})
			}

		case string(agent.MessageToolResult):
			if chunk.Tool != nil {
				h.pushEventJSON(req.TaskID, "tool_result", map[string]interface{}{
					"agent_id":   req.AgentID,
					"agent_name": agentInfo.Name,
					"tool_name":  chunk.Tool.Name,
					"output":     chunk.Tool.Output,
					"call_id":    chunk.Tool.CallID,
					"is_error":   chunk.Tool.IsError,
				})
			}
		}
	}

	// v1.3: Slock-aligned - only CLI-sent messages appear in channel.
	// If solo message send was called, API already created the message.
	// Direct text output is internal thinking, not channel messages.
	

	// Check context cancellation
	if ctx.Err() != nil {
		slog.Info("task cancelled via context", "task_id", req.TaskID, "agent_id", req.AgentID)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusCancelled)
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	// Get final result
	result := <-session.Result

	// v1.3: Slock-aligned — NEVER persist text output as channel messages.
	// Only solo message send API (via proxy) creates visible messages.
	// All text output is internal thinking. Always skip persist.
	if !messageSentViaCLI && strings.TrimSpace(fullContent) != "" {
		slog.Info("task: suppressing text output (not sent via CLI)", "agent_id", req.AgentID, "length", len(fullContent))
	}

	slog.Info("task backend completed",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"status", result.Status,
		"content_length", len(result.Output),
		"duration_ms", result.DurationMs,
		"message_sent_via_cli", messageSentViaCLI,
	)

	// Extract usage from result
	var inputTokens, outputTokens int
	if result.Usage != nil {
		for _, u := range result.Usage {
			inputTokens += int(u.InputTokens)
			outputTokens += int(u.OutputTokens)
		}
	}

	h.taskManager.UpdateStatus(req.TaskID, taskStatusCompleted)

	// Push complete event — notification only (no content, no persist).
	// Real messages arrive via solo message send → daemon proxy → server API → message.new.
	h.pushEventJSON(req.TaskID, "complete", map[string]interface{}{
		"agent_id": req.AgentID,
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})

	// Push done sentinel and close. The done event is consumed by SSE
	// subscribers in order, eliminating the need for a delay.
	h.pushEventJSON(req.TaskID, "done", map[string]interface{}{})
	h.taskManager.CloseAllSubscribers(req.TaskID)
}

// agentInfo holds agent metadata fetched from the database.
type agentInfo struct {
	Name       string
	CustomEnv  map[string]string // agent-level env overrides (from custom_env JSONB)
	CustomArgs []string          // agent-level CLI args (from custom_args JSONB)
}

// fetchAgentInfo queries agent metadata by ID.
func (h *daemonHandler) fetchAgentInfo(ctx context.Context, agentID string) (*agentInfo, error) {
	var info agentInfo
	var customEnvBytes, customArgsBytes []byte
	err := h.pool.QueryRow(ctx,
		`SELECT name, custom_env, custom_args FROM agents WHERE id = $1 AND is_active = true`, agentID,
	).Scan(&info.Name, &customEnvBytes, &customArgsBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch agent %s: %w", agentID, err)
	}
	if len(customEnvBytes) > 0 {
		json.Unmarshal(customEnvBytes, &info.CustomEnv)
	}
	if info.CustomEnv == nil {
		info.CustomEnv = make(map[string]string)
	}
	if len(customArgsBytes) > 0 {
		json.Unmarshal(customArgsBytes, &info.CustomArgs)
	}
	if info.CustomArgs == nil {
		info.CustomArgs = make([]string, 0)
	}
	return &info, nil
}

// fetchChannelName queries a channel's name by ID.
func (h *daemonHandler) fetchChannelName(ctx context.Context, channelID string) (string, error) {
	var name string
	err := h.pool.QueryRow(ctx,
		`SELECT name FROM channels WHERE id = $1`, channelID,
	).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("fetch channel %s: %w", channelID, err)
	}
	return name, nil
}

// pushEventJSON marshals data as JSON and pushes an SSE event to all subscribers.
func (h *daemonHandler) pushEventJSON(taskID, event string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to marshal SSE event data", "task_id", taskID, "event", event, "error", err)
		return
	}
	h.taskManager.PushSSEEvent(taskID, sseEvent{
		Event: event,
		Data:  string(raw),
	})
}

// --- SSE endpoint ---

// TaskEvents handles GET /internal/daemon/tasks/{taskID}/events
// This is an SSE endpoint that streams task execution events.
func (h *daemonHandler) TaskEvents(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Verify task exists
	_, ok := h.taskManager.GetTask(taskID)
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to task events
	sub := h.taskManager.SubscribeSSE(taskID)
	defer h.taskManager.UnsubscribeSSE(taskID, sub)

	// Send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case evt, ok := <-sub.events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, evt.Data)
			flusher.Flush()
			// "done" event signals stream end - exit cleanly
			// so the client sees events in order.
			if evt.Event == "done" {
				return
			}

		case <-sub.done:
			// Fallback: task completed/cancelled unexpectedly.
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return

		case <-r.Context().Done():
			return
		}
	}
}

// CancelTask handles POST /internal/daemon/tasks/{taskID}/cancel
func (h *daemonHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task_id is required"})
		return
	}

	cancelled := h.taskManager.CancelTask(taskID)
	if !cancelled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found or already completed"})
		return
	}

	slog.Info("task cancel requested", "task_id", taskID)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "cancelled",
		"task_id": taskID,
	})
}

// --- Callbacks ---

// notifyServerComplete sends a task completion callback to the server.
// NOTE: This is only used for non-streaming (callback-based) task paths.
// The SSE streaming path handles completion via the "complete" SSE event.
func (h *daemonHandler) notifyServerComplete(req runTaskRequest, content, messageID string, usage llm.Usage) {
	if h.serverURL == "" {
		slog.Warn("no server URL configured, skipping task complete notification",
			"task_id", req.TaskID,
		)
		return
	}

	cbReq := taskCompleteCallback{
		TaskID:    req.TaskID,
		AgentID:   req.AgentID,
		ChannelID: req.ChannelID,
		ThreadID:  req.ThreadID,
		Content:   content,
		MessageID: messageID,
	}
	cbReq.Usage.InputTokens = usage.InputTokens
	cbReq.Usage.OutputTokens = usage.OutputTokens

	payload, err := json.Marshal(cbReq)
	if err != nil {
		slog.Error("failed to marshal task complete callback", "error", err)
		return
	}

	url := h.serverURL + "/internal/daemon/tasks/" + req.TaskID + "/complete"
	if err := h.sendInternalRequest(url, payload); err != nil {
		slog.Error("failed to notify server of task completion",
			"task_id", req.TaskID, "error", err,
		)
		return
	}

	slog.Info("task completion notified to server",
		"task_id", req.TaskID,
	)
}

// notifyServerError sends a task error callback to the server.
func (h *daemonHandler) notifyServerError(req runTaskRequest, errMsg string) {
	if h.serverURL == "" {
		return
	}

	cbReq := taskErrorCallback{
		TaskID:    req.TaskID,
		AgentID:   req.AgentID,
		ChannelID: req.ChannelID,
		Error:     errMsg,
	}

	payload, err := json.Marshal(cbReq)
	if err != nil {
		slog.Error("failed to marshal task error callback", "error", err)
		return
	}

	url := h.serverURL + "/internal/daemon/tasks/" + req.TaskID + "/error"
	if err := h.sendInternalRequest(url, payload); err != nil {
		slog.Error("failed to notify server of task error",
			"task_id", req.TaskID, "error", err,
		)
	}
}

// sendInternalRequest sends a POST request to the server with the internal auth header.
func (h *daemonHandler) sendInternalRequest(url string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.internalToken != "" {
		req.Header.Set("Authorization", "Internal-Token "+h.internalToken)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// --- Callback types ---

// taskCompleteCallback is sent to the server when a task completes.
type taskCompleteCallback struct {
	TaskID    string `json:"task_id"`
	AgentID   string `json:"agent_id"`
	ChannelID string `json:"channel_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	Content   string `json:"content"`
	MessageID string `json:"message_id"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

// taskErrorCallback is sent to the server when a task encounters an error.
type taskErrorCallback struct {
	TaskID    string `json:"task_id"`
	AgentID   string `json:"agent_id"`
	ChannelID string `json:"channel_id"`
	Error     string `json:"error"`
}

// copyFile copies a file from src to dst with the given permissions mode.
func copyFile(src, dst string, mode os.FileMode) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = io.Copy(d, s)
	return err
}

// ── Workspace file browsing endpoints ──────────────────────────────────────────

// workspaceNode represents a file or directory in the workspace tree.
type workspaceNode struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Path     string          `json:"path,omitempty"`
	Content  string          `json:"content,omitempty"`
	Size     int64           `json:"size,omitempty"`
	Children []workspaceNode `json:"children,omitempty"`
}

// HandleWorkspaceList returns a file tree for the given agent's workspace.
func (h *daemonHandler) HandleWorkspaceList(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		relPath = "."
	}

	workspaceDir := h.workspaceManager.WorkspaceDir(agentID)
	fullPath := filepath.Clean(filepath.Join(workspaceDir, relPath))
	if !strings.HasPrefix(fullPath, workspaceDir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "path not found"})
			return
		}
		slog.Error("workspace list: stat failed", "path", fullPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Resolve symlinks and re-verify containment to prevent
	// symlink-based path traversal that would bypass the string prefix check.
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path resolution failed"})
		return
	}
	if !strings.HasPrefix(resolvedPath, workspaceDir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
		return
	}

	var node workspaceNode
	if info.IsDir() {
		node, err = buildFileTree(resolvedPath, workspaceDir, 0)
	} else {
		node, err = buildFileNode(resolvedPath, workspaceDir)
	}
	if err != nil {
		slog.Error("workspace list: build failed", "path", resolvedPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read workspace"})
		return
	}

	writeJSON(w, http.StatusOK, node)
}

// HandleWorkspaceRead returns the content of a single file in the workspace.
func (h *daemonHandler) HandleWorkspaceRead(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}

	workspaceDir := h.workspaceManager.WorkspaceDir(agentID)
	fullPath := filepath.Clean(filepath.Join(workspaceDir, relPath))
	if !strings.HasPrefix(fullPath, workspaceDir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		slog.Error("workspace read: stat failed", "path", fullPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Resolve symlinks and re-verify containment to prevent
	// symlink-based path traversal that would bypass the string prefix check.
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path resolution failed"})
		return
	}
	if !strings.HasPrefix(resolvedPath, workspaceDir) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
		return
	}

	if info.Size() > 1*1024*1024 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"content": "[file too large to preview]",
			"name":    info.Name(),
			"size":    info.Size(),
		})
		return
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		slog.Error("workspace read: read failed", "path", resolvedPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
		return
	}

	content := string(data)
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	for _, b := range data[:checkLen] {
		if b == 0 {
			content = "[binary file]"
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": content,
		"name":    info.Name(),
		"size":    info.Size(),
	})
}

const maxWorkspaceDepth = 20

// buildFileTree recursively builds a workspaceNode tree for a directory.
func buildFileTree(dirPath, basePath string, depth int) (workspaceNode, error) {
	if depth > maxWorkspaceDepth {
		return workspaceNode{
			Type: "directory",
			Name: filepath.Base(dirPath),
		}, nil
	}

	name := filepath.Base(dirPath)
	if dirPath == basePath {
		name = "."
	}

	relPath, _ := filepath.Rel(basePath, dirPath)
	node := workspaceNode{
		Type:     "directory",
		Name:     name,
		Path:     relPath,
		Children: []workspaceNode{},
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return node, err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Skip symlinks to prevent recursion into directories outside
		// the workspace via symlink-based traversal.
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		if entry.IsDir() {
			child, err := buildFileTree(fullPath, basePath, depth+1)
			if err != nil {
				slog.Warn("workspace: failed to read subdirectory", "path", fullPath, "error", err)
				continue
			}
			node.Children = append(node.Children, child)
		} else {
			child, err := buildFileNode(fullPath, basePath)
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
func buildFileNode(filePath, basePath string) (workspaceNode, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return workspaceNode{}, err
	}

	relPath, _ := filepath.Rel(basePath, filePath)
	node := workspaceNode{
		Type: "file",
		Name: info.Name(),
		Path: relPath,
		Size: info.Size(),
	}

	return node, nil
}

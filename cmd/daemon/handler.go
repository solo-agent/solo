package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	serverservice "github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/llm"
	"github.com/solo-ai/solo/pkg/skillloader"
)

const backendFinalResultWaitAfter = 5 * time.Second
const maxControlRPCResponse = 900 * 1024

const (
	defaultAgentSessionIdleTTL      = 30 * time.Minute
	defaultThinkingSessionIdleTTL   = 30 * time.Minute
	defaultSessionIdleSweepInterval = time.Minute
)

// daemonHandler holds the daemon-side HTTP handlers.
type daemonHandler struct {
	taskManager      *taskManager
	providers        map[string]llm.Provider
	serverURL        string
	internalToken    string
	httpClient       *http.Client
	mu               sync.Mutex
	workspaceManager *agent.WorkspaceManager
	memoryManager    *agent.MemoryManager
	sessionManagers  map[string]*agent.AgentSessionManager // v1.4: per-provider persistent sessions
	agentIdleTTL     time.Duration
	thinkingIdleTTL  time.Duration
	sessionIdleSweep time.Duration
	shuttingDown     atomic.Bool
}

// newDaemonHandler creates a new daemon HTTP handler.
// provider is the default LLM provider (used when no per-task override exists).
func newDaemonHandler(tm *taskManager, provider llm.Provider, serverURL, internalToken string) *daemonHandler {
	return &daemonHandler{
		taskManager: tm,
		providers: map[string]llm.Provider{
			"": provider, // default provider key
		},
		serverURL:        serverURL,
		internalToken:    internalToken,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		workspaceManager: agent.NewWorkspaceManager(""),
		memoryManager:    agent.NewMemoryManager(""),
		agentIdleTTL:     durationFromEnv("AGENT_SESSION_IDLE_TTL", defaultAgentSessionIdleTTL),
		thinkingIdleTTL:  durationFromEnv("THINKING_SESSION_IDLE_TTL", defaultThinkingSessionIdleTTL),
		sessionIdleSweep: durationFromEnv("SESSION_IDLE_SWEEP_INTERVAL", durationFromEnv("THINKING_SESSION_SWEEP_INTERVAL", defaultSessionIdleSweepInterval)),
	}
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		slog.Warn("daemon: invalid duration setting, using default", "name", name, "value", value, "default", fallback)
		return fallback
	}
	return duration
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

// DetectBackends reports CLI availability on the machine that runs agents.
func (h *daemonHandler) DetectBackends(w http.ResponseWriter, _ *http.Request) {
	results := agent.GlobalRegistry().Detect()
	if results == nil {
		results = []agent.BackendStatus{}
	}
	writeJSON(w, http.StatusOK, results)
}

// handleControlRPC reuses the existing local handlers while the transport is
// reversed: Server asks through the authenticated control socket, Daemon reads
// the local resource, and only the bounded JSON response crosses the network.
func (h *daemonHandler) handleControlRPC(ctx context.Context, method string, raw json.RawMessage) ([]byte, error) {
	var params map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("invalid RPC params: %w", err)
		}
	}
	query := make(url.Values)
	for _, key := range []string{"agent_id", "path", "provider"} {
		if value, ok := params[key].(string); ok && value != "" {
			query.Set(key, value)
		}
	}

	var handler http.HandlerFunc
	switch method {
	case "backend.detect":
		handler = h.DetectBackends
	case "transcript.read":
		return h.readTranscriptRPC(raw)
	case "workspace.list":
		handler = h.HandleWorkspaceList
	case "workspace.read":
		handler = h.HandleWorkspaceRead
	case "skills.list":
		handler = h.HandleSkillsList
	case "task.cancel":
		taskID, _ := params["task_id"].(string)
		if taskID == "" || !h.taskManager.CancelTask(taskID) {
			return nil, errors.New("task not found")
		}
		return json.Marshal(map[string]bool{"cancelled": true})
	case "agent.cleanup":
		agentID, _ := params["agent_id"].(string)
		if agentID == "" {
			return nil, errors.New("agent_id is required")
		}
		h.cleanupAgent(agentID)
		return json.Marshal(map[string]bool{"cleaned": true})
	case "thinking.cleanup":
		nodeValues, _ := params["node_ids"].([]any)
		for _, value := range nodeValues {
			nodeID, _ := value.(string)
			for _, manager := range h.sessionManagers {
				_ = manager.ForceCloseThinkingSession(nodeID)
			}
		}
		return json.Marshal(map[string]bool{"cleaned": true})
	default:
		return nil, fmt.Errorf("unsupported RPC method %q", method)
	}
	req := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	handler(recorder, req)
	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxControlRPCResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxControlRPCResponse {
		return nil, fmt.Errorf("local RPC %s response exceeds limit", method)
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("local RPC %s returned %d: %s", method, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("local RPC %s returned invalid JSON", method)
	}
	return body, nil
}

func (h *daemonHandler) readTranscriptRPC(raw json.RawMessage) ([]byte, error) {
	var req struct {
		AgentID           string `json:"agent_id"`
		Provider          string `json:"provider"`
		ExternalSessionID string `json:"external_session_id"`
		Start             string `json:"start"`
		End               string `json:"end"`
		Limit             int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("invalid transcript request: %w", err)
	}
	if req.AgentID == "" || req.Provider == "" {
		return nil, errors.New("agent_id and provider are required")
	}
	if req.ExternalSessionID == "" {
		return []byte("[]"), nil
	}
	if req.Limit <= 0 || req.Limit > 2000 {
		req.Limit = 2000
	}
	var start, end time.Time
	var err error
	if req.Start != "" {
		start, err = time.Parse(time.RFC3339Nano, req.Start)
		if err != nil {
			return nil, errors.New("invalid transcript start")
		}
	}
	if req.End != "" {
		end, err = time.Parse(time.RFC3339Nano, req.End)
		if err != nil {
			return nil, errors.New("invalid transcript end")
		}
	}
	var entries []serverservice.AgentTranscriptEntry
	if req.Provider == "hermes" {
		entries, err = serverservice.ReadHermesTranscriptWindow(req.ExternalSessionID, start, end, req.Limit)
	} else {
		if h.workspaceManager == nil {
			return nil, errors.New("workspace manager unavailable")
		}
		path := transcriptPathForProvider(req.Provider, h.workspaceManager.WorkspaceDir(req.AgentID), req.ExternalSessionID)
		entries, err = serverservice.ReadAgentTranscriptWindow(path, start, end, req.Limit)
	}
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Raw = nil
	}
	result, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	if len(result) > maxControlRPCResponse {
		return nil, errors.New("transcript response exceeds limit")
	}
	return result, nil
}

// cachedSessionAgentIDs returns all Agent IDs with a cached persistent session,
// including idle sessions whose provider process is asleep. A resumable Agent
// remains online and routable while its local process is released.
func (h *daemonHandler) cachedSessionAgentIDs() []string {
	if h.sessionManagers == nil {
		return nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, sm := range h.sessionManagers {
		for _, id := range sm.CachedAgentIDs() {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (h *daemonHandler) activeThinkingNodeID(agentID string) (string, error) {
	var nodeID string
	for provider, sm := range h.sessionManagers {
		candidate, ok := sm.ActiveThinkingNodeID(agentID)
		if !ok {
			continue
		}
		if nodeID != "" && nodeID != candidate {
			return "", fmt.Errorf("agent %s has conflicting Thinking runtime scopes (%s, provider %s)", agentID, candidate, provider)
		}
		nodeID = candidate
	}
	return nodeID, nil
}

// runSessionReaper releases idle provider processes while retaining their
// session IDs so the next tracked turn resumes the same conversation.
func (h *daemonHandler) runSessionReaper(ctx context.Context) {
	ticker := time.NewTicker(h.sessionIdleSweep)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			agentIdleBefore := now.Add(-h.agentIdleTTL)
			thinkingIdleBefore := now.Add(-h.thinkingIdleTTL)
			for provider, sm := range h.sessionManagers {
				slept, err := sm.SleepIdleAgentSessions(agentIdleBefore)
				if err != nil {
					slog.Warn("daemon: failed to sleep idle Agent sessions", "provider", provider, "error", err)
				}
				if slept > 0 {
					slog.Info("daemon: slept idle Agent sessions", "provider", provider, "count", slept)
				}

				slept, err = sm.SleepIdleThinkingSessions(thinkingIdleBefore)
				if err != nil {
					slog.Warn("daemon: failed to sleep idle Thinking sessions", "provider", provider, "error", err)
				}
				if slept > 0 {
					slog.Info("daemon: slept idle Thinking sessions", "provider", provider, "count", slept)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// ProxyRequest handles POST /internal/daemon/proxy
// Agents call this local endpoint instead of hitting the server API directly.
// The daemon adds auth and forwards the request to the server. This keeps
// local thinking separate from channel communication.
func (h *daemonHandler) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	// Only accept from localhost — this is an internal agent proxy.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "proxy only accessible from localhost"})
		return
	}

	var req struct {
		AgentID     string `json:"agent_id"`
		Action      string `json:"action"`
		ChannelID   string `json:"channel_id"`
		Content     string `json:"content,omitempty"`
		ThreadID    string `json:"thread_id,omitempty"`
		NodeID      string `json:"thinking_node_id,omitempty"`
		RunID       string `json:"run_id,omitempty"`
		TaskNumber  int    `json:"task_number,omitempty"`
		TaskID      string `json:"task_id,omitempty"`
		Status      string `json:"status,omitempty"`
		ClientMsgID string `json:"client_msg_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Action == "message_send" || req.Action == "message_read" || req.Action == "message_check" {
		runtimeNodeID, err := h.activeThinkingNodeID(req.AgentID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "ambiguous Thinking runtime route"})
			return
		}
		if runtimeNodeID != "" {
			if req.NodeID != "" && req.NodeID != runtimeNodeID {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Thinking node route conflicts with active runtime"})
				return
			}
			req.NodeID = runtimeNodeID
		}
	}
	if req.Action == "message_send" {
		if runID, _ := h.taskManager.ExecutingCredential(req.AgentID, req.ChannelID, req.NodeID); runID != "" {
			req.RunID = runID
		}
	}

	_, token := h.taskManager.ExecutingCredential(req.AgentID, req.ChannelID, req.NodeID)
	if token == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no executing Run credential for this Agent scope"})
		return
	}

	// Build and forward the request to the server
	var serverPath string
	var serverBody []byte
	switch req.Action {
	case "message_send":
		serverPath = fmt.Sprintf("/api/v1/channels/%s/messages", req.ChannelID)
		bodyMap := map[string]string{"content": req.Content}
		if req.ThreadID != "" {
			bodyMap["thread_id"] = req.ThreadID
		}
		if req.NodeID != "" {
			bodyMap["thinking_node_id"] = req.NodeID
		}
		if req.RunID != "" {
			bodyMap["run_id"] = req.RunID
		}
		if req.ClientMsgID != "" {
			bodyMap["client_msg_id"] = req.ClientMsgID
		}
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
		if req.NodeID != "" {
			serverPath += "?thinking_node_id=" + url.QueryEscape(req.NodeID)
		}
	case "message_check":
		serverPath = fmt.Sprintf("/api/v1/messages/check?channel_id=%s", req.ChannelID)
		if req.NodeID != "" {
			serverPath += "&thinking_node_id=" + url.QueryEscape(req.NodeID)
		}
	case "channel_join":
		serverPath = "/api/v1/channels/join"
		serverBody, _ = json.Marshal(map[string]string{"target": req.Content})
	case "thread_unfollow":
		serverPath = "/api/v1/threads/unfollow"
		serverBody, _ = json.Marshal(map[string]string{"target": req.Content})
	case "team_form":
		serverPath = "/api/v1/team-formations"
		serverBody = []byte(req.Content)
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

		client := h.httpClient
		if req.Action == "team_form" {
			client = cloneHTTPClientWithTimeout(h.httpClient, 55*time.Second)
		}
		resp, fwdErr := client.Do(httpReq)
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

	w.Header().Set("Content-Type", "application/json")
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func cloneHTTPClientWithTimeout(client *http.Client, timeout time.Duration) *http.Client {
	clone := *client
	clone.Timeout = timeout
	return &clone
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
	TaskID                string             `json:"task_id"`
	RunID                 string             `json:"run_id,omitempty"`
	AgentID               string             `json:"agent_id"`
	ChannelID             string             `json:"channel_id"`
	ThreadID              string             `json:"thread_id,omitempty"`
	NodeID                string             `json:"thinking_node_id,omitempty"`
	ResumeSessionID       string             `json:"resume_session_id,omitempty"`
	ReturnHandoff         bool               `json:"return_handoff,omitempty"`
	Messages              []llmMessage       `json:"messages"`
	ColdStartMessages     []llmMessage       `json:"cold_start_messages,omitempty"`
	SystemPrompt          string             `json:"system_prompt"`
	ThinkingRuntimePrompt string             `json:"thinking_runtime_prompt,omitempty"`
	ModelConfig           modelConfigPayload `json:"model_config"`
	TaskContext           string             `json:"task_context,omitempty"`    // SOLO-221-B: summary of pending tasks in channel
	MentionedNames        []string           `json:"mentioned_names,omitempty"` // v1.3: names of @mentioned agents
	AgentToken            string             `json:"agent_token,omitempty"`
	AgentName             string             `json:"agent_name,omitempty"`
	ChannelName           string             `json:"channel_name,omitempty"`
	CustomEnv             map[string]string  `json:"custom_env,omitempty"`
	CustomArgs            []string           `json:"custom_args,omitempty"`
}

type llmMessage struct {
	Role        string             `json:"role"`
	Content     string             `json:"content"`
	SenderID    string             `json:"sender_id,omitempty"`
	Attachments []agent.Attachment `json:"attachments,omitempty"`
}

type modelConfigPayload struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
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
	if req.TaskID == "" {
		req.TaskID = uuid.NewString()
	}

	if err := h.startTask(req, ""); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, runTaskResponse{TaskID: req.TaskID, Status: taskStatusQueued})
}

func (h *daemonHandler) startTask(req runTaskRequest, attemptID string) error {
	if req.TaskID == "" {
		req.TaskID = uuid.New().String()
	}
	if req.AgentID == "" || req.ChannelID == "" {
		return errors.New("agent_id and channel_id are required")
	}
	// Register the task
	if !h.taskManager.AddTask(req.TaskID, &taskState{
		TaskID:     req.TaskID,
		RunID:      req.RunID,
		AgentID:    req.AgentID,
		ChannelID:  req.ChannelID,
		ThreadID:   req.ThreadID,
		NodeID:     req.NodeID,
		Status:     taskStatusQueued,
		ReceivedAt: time.Now(),
		AttemptID:  attemptID,
		AgentToken: req.AgentToken,
	}) {
		return nil
	}

	slog.Info("task received",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"channel_id", req.ChannelID,
		"model", req.ModelConfig.Model,
		"provider", req.ModelConfig.Provider,
	)

	// Process the task asynchronously with streaming
	ctx, cancel := context.WithCancel(context.Background())
	h.taskManager.SetCancelFunc(req.TaskID, cancel)
	go func() {
		defer h.taskManager.ClearCancelFunc(req.TaskID)
		h.processTaskStreaming(ctx, req)
	}()
	return nil
}

// processTaskStreaming executes the LLM call in streaming mode and pushes events.
// It tries the Backend interface first (for all registered CLI backends);
// falls back to the old LLM provider path for API-based providers.
func (h *daemonHandler) processTaskStreaming(ctx context.Context, req runTaskRequest) {
	release, err := h.taskManager.acquireAgentTurn(ctx, req.AgentID)
	if err != nil {
		h.finishCancelledTask(req)
		return
	}
	defer release()

	if backend, err := agent.NewBackend(req.ModelConfig.Provider, os.Getenv("LLM_API_KEY")); err == nil {
		h.processTaskWithBackend(ctx, req, backend)
		return
	}

	// Fallback: use old LLM provider path.
	h.processTaskWithProvider(ctx, req)
}

func (h *daemonHandler) finishCancelledTask(req runTaskRequest) {
	if h.shuttingDown.Load() {
		return
	}
	h.taskManager.UpdateStatus(req.TaskID, taskStatusCancelled)
	h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
		"agent_id":  req.AgentID,
		"error":     "execution cancelled",
		"status":    "cancelled",
		"retryable": false,
	})
	h.pushEventJSON(req.TaskID, "done", map[string]interface{}{})
	h.taskManager.CloseAllSubscribers(req.TaskID)
}

// processTaskWithProvider runs a task using the old LLM provider interface.
// This is the fallback path for providers not supported by the Backend
// interface (e.g. openai, anthropic).
func (h *daemonHandler) processTaskWithProvider(ctx context.Context, req runTaskRequest) {
	slog.Info("task processing started (streaming)", "task_id", req.TaskID, "agent_id", req.AgentID)

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
	if req.ThinkingRuntimePrompt != "" {
		systemPrompt += "\n\n## Thinking Runtime\n\n" + req.ThinkingRuntimePrompt
	}
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

		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id":  req.AgentID,
			"error":     err.Error(),
			"retryable": true,
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}
	h.pushEventJSON(req.TaskID, "backend_started", map[string]string{
		"agent_id": req.AgentID,
		"run_id":   req.RunID,
	})
	h.taskManager.UpdateStatus(req.TaskID, taskStatusThinking)
	h.pushEventJSON(req.TaskID, "thinking", map[string]string{
		"agent_id": req.AgentID,
		"thought":  "Processing request...",
	})

	// Collect full content from stream
	var fullContent string
	var usage llm.Usage
	var sawDone bool

	for chunk := range streamCh {
		if chunk.Error != nil {
			slog.Error("task: streaming LLM call error",
				"task_id", req.TaskID,
				"error", chunk.Error,
			)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)

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
			sawDone = true
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
		h.finishCancelledTask(req)
		return
	}
	if !sawDone {
		errMsg := "provider stream closed without completion"
		slog.Error("task: streaming LLM call ended without done",
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
		)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id":  req.AgentID,
			"error":     errMsg,
			"retryable": true,
		})
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
	if req.ReturnHandoff && req.NodeID != "" {
		defer func() {
			for provider, sm := range h.sessionManagers {
				if err := sm.CloseThinkingSession(req.NodeID); err != nil {
					slog.Warn("task: failed to close returned Thinking session", "node_id", req.NodeID, "provider", provider, "error", err)
				}
			}
		}()
	}
	slog.Info("task processing started (backend)",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"backend", backend.Name(),
		"model", req.ModelConfig.Model,
	)

	agentInfo := &agentInfo{Name: req.AgentName, CustomEnv: req.CustomEnv, CustomArgs: req.CustomArgs}
	if agentInfo.Name == "" {
		agentInfo.Name = req.AgentID
	}
	if agentInfo.CustomEnv == nil {
		agentInfo.CustomEnv = map[string]string{}
	}

	// Prepare workspace (idempotent — safe to call every time)
	ws, err := h.workspaceManager.Prepare(req.AgentID, &agent.AgentConfig{
		AgentID:      req.AgentID,
		Name:         agentInfo.Name,
		SystemPrompt: req.SystemPrompt,
		Model:        req.ModelConfig.Model,
		Provider:     req.ModelConfig.Provider,
	})
	if err != nil {
		slog.Error("task: workspace preparation failed", "task_id", req.TaskID, "error", err)
		h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id": req.AgentID, "error": "workspace preparation failed", "retryable": true,
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	// Load memory content
	memoryContent, _ := h.memoryManager.Load(req.AgentID)

	channelName := req.ChannelName

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

	localDaemonPort := strings.TrimSpace(os.Getenv("DAEMON_PORT"))
	if localDaemonPort == "" {
		localDaemonPort = "8081"
	}
	agentEnv := map[string]string{
		"SOLO_AGENT_ID":   req.AgentID,
		"SOLO_AGENT_NAME": agentInfo.Name,
		"SOLO_API_URL":    h.serverURL,
		"SOLO_DAEMON_URL": "http://127.0.0.1:" + localDaemonPort,
	}
	if req.NodeID != "" {
		agentEnv["SOLO_NODE_ID"] = req.NodeID
	}
	if req.AgentToken != "" {
		agentEnv["SOLO_AUTH_TOKEN"] = req.AgentToken
	}
	// Merge agent-level custom_env over base agentEnv (agent wins).
	for k, v := range agentInfo.CustomEnv {
		agentEnv[k] = v
	}
	// SOLO_NODE_ID is runtime-owned routing state. Agent custom environment
	// must not redirect a normal or node-scoped session into another node.
	if req.NodeID != "" {
		agentEnv["SOLO_NODE_ID"] = req.NodeID
	} else {
		delete(agentEnv, "SOLO_NODE_ID")
	}
	if req.RunID != "" {
		agentEnv["SOLO_RUN_ID"] = req.RunID
	} else {
		delete(agentEnv, "SOLO_RUN_ID")
	}

	// Build system prompt using PromptBuilder
	hostname, _ := os.Hostname()
	agentCfg := agent.AgentConfig{
		AgentID:               req.AgentID,
		Name:                  agentInfo.Name,
		SystemPrompt:          req.SystemPrompt,
		ThinkingRuntimePrompt: req.ThinkingRuntimePrompt,
		Model:                 req.ModelConfig.Model,
		Provider:              req.ModelConfig.Provider,
		CustomArgs:            agentInfo.CustomArgs,
		Env:                   agentEnv,
		WorkspacePath:         ws.WorkDir,
		ServerID:              h.serverURL,
		Hostname:              hostname,
		OS:                    runtime.GOOS + " " + runtime.GOARCH,
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
	if err := agent.SyncSoloSkillsForProvider(soloSkillsRoot(), ws.WorkDir, req.ModelConfig.Provider); err != nil {
		slog.Warn("task: sync solo skills failed (non-fatal)", "task_id", req.TaskID, "error", err)
	}

	materializedMessages := h.materializeMessageAttachments(ctx, req.AgentToken, req.Messages, ws.WorkDir)
	materializedColdStartMessages := h.materializeMessageAttachments(ctx, req.AgentToken, req.ColdStartMessages, ws.WorkDir)

	// Convert messages to agent.Message format
	msgs := make([]agent.Message, len(materializedMessages))
	for i, m := range materializedMessages {
		msgs[i] = agent.Message{
			Role:        agent.Role(m.Role),
			Content:     m.Content,
			SenderID:    m.SenderID,
			Attachments: m.Attachments,
		}
	}
	coldStartMsgs := make([]agent.Message, len(materializedColdStartMessages))
	for i, m := range materializedColdStartMessages {
		coldStartMsgs[i] = agent.Message{
			Role:        agent.Role(m.Role),
			Content:     m.Content,
			SenderID:    m.SenderID,
			Attachments: m.Attachments,
		}
	}
	if len(coldStartMsgs) == 0 {
		coldStartMsgs = msgs
	}

	// Execute via Backend
	executeReq := &agent.ExecuteRequest{
		AgentID:  req.AgentID,
		Messages: coldStartMsgs,
	}
	// Inject the companion solo CLI into the workspace so agents can send
	// visible messages through the daemon proxy. Development uses .pids/solo;
	// packaged installs keep it beside the daemon.
	soloPath := resolveSoloBinary()
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
		Env:          agentEnv,
		CustomArgs:   agentInfo.CustomArgs,
		// ExtraArgs: daemonConfig.ExtraArgs[backend.Name()], // P1 reserved
	}

	// v1.3: Session-aware dispatch. Thinking nodes use a node-scoped pool key
	// while retaining the real Agent identity, workspace, and configuration.
	var session *agent.Session
	var providerSessionID string
	var transcriptPath string
	if _, isPersistent := backend.(agent.PersistentBackend); isPersistent && h.getSessionManager(req.ModelConfig.Provider) != nil {
		sm := h.getSessionManager(req.ModelConfig.Provider)
		sessionKey := agent.ChannelSessionKey(req.AgentID, req.ChannelID)
		if req.NodeID != "" {
			sessionKey = agent.ThinkingSessionKey(req.NodeID)
		}
		slog.Info("task: getting persistent session", "agent_id", req.AgentID, "session_key", sessionKey, "resume", req.ResumeSessionID)
		ps, psErr := sm.GetOrCreateScopedSession(ctx, sessionKey, req.AgentID, agentCfg, channelCtx, msgs, coldStartMsgs, req.ResumeSessionID, req.MentionedNames)
		if psErr == nil {
			providerSessionID = ps.SessionID
			transcriptPath = transcriptPathForProvider(req.ModelConfig.Provider, ws.WorkDir, providerSessionID)
			session = &agent.Session{Messages: ps.Messages, Result: ps.Result, Stop: ps.Stop, SessionID: providerSessionID}
		} else {
			slog.Warn("task: persistent session failed, falling back to Execute", "agent_id", req.AgentID, "session_key", sessionKey, "error", psErr)
		}
	}

	if ctx.Err() != nil {
		if session != nil && session.Stop != nil {
			if stopErr := session.Stop(); stopErr != nil {
				slog.Warn("task: backend stop failed after cancelled start", "task_id", req.TaskID, "agent_id", req.AgentID, "error", stopErr)
			}
		}
		slog.Info("task cancelled while waiting for backend turn", "task_id", req.TaskID, "agent_id", req.AgentID)
		h.finishCancelledTask(req)
		return
	}
	if session == nil {
		var execErr error
		session, execErr = backend.Execute(ctx, executeReq, executeOpts)
		if execErr != nil {
			if ctx.Err() != nil {
				slog.Info("task cancelled during backend start", "task_id", req.TaskID, "agent_id", req.AgentID)
				h.finishCancelledTask(req)
				return
			}
			slog.Error("task: Backend.Execute failed", "task_id", req.TaskID, "error", execErr)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
			h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
				"agent_id": req.AgentID, "error": execErr.Error(), "retryable": true,
			})
			h.taskManager.CloseAllSubscribers(req.TaskID)
			return
		}
	}
	if providerSessionID == "" && session.SessionID != "" {
		providerSessionID = session.SessionID
		transcriptPath = transcriptPathForProvider(req.ModelConfig.Provider, ws.WorkDir, providerSessionID)
	}
	h.pushEventJSON(req.TaskID, "backend_started", map[string]string{
		"agent_id": req.AgentID,
		"run_id":   req.RunID,
	})
	h.taskManager.UpdateStatus(req.TaskID, taskStatusThinking)
	h.pushEventJSON(req.TaskID, "thinking", map[string]string{
		"agent_id": req.AgentID,
		"thought":  "Processing...",
	})
	if providerSessionID != "" || transcriptPath != "" {
		h.pushEventJSON(req.TaskID, "session", map[string]interface{}{
			"agent_id":            req.AgentID,
			"external_session_id": providerSessionID,
			"transcript_path":     transcriptPath,
		})
	}

	// Stream output chunks
	var fullContent string

	streamOpen := true
	for streamOpen {
		var chunk agent.OutputChunk
		select {
		case <-ctx.Done():
			if session.Stop != nil {
				if stopErr := session.Stop(); stopErr != nil {
					slog.Warn("task: backend stop failed", "task_id", req.TaskID, "agent_id", req.AgentID, "error", stopErr)
				}
			}
			slog.Info("task cancelled via context", "task_id", req.TaskID, "agent_id", req.AgentID)
			h.finishCancelledTask(req)
			return
		case next, ok := <-session.Messages:
			if !ok {
				streamOpen = false
				continue
			}
			chunk = next
		}
		if chunk.SessionID != "" && chunk.SessionID != providerSessionID {
			providerSessionID = chunk.SessionID
			transcriptPath = transcriptPathForProvider(req.ModelConfig.Provider, ws.WorkDir, providerSessionID)
			h.pushEventJSON(req.TaskID, "session", map[string]interface{}{
				"agent_id":            req.AgentID,
				"external_session_id": providerSessionID,
				"transcript_path":     transcriptPath,
			})
		}
		if providerSessionID != "" && transcriptPath == "" {
			if path := transcriptPathForProvider(req.ModelConfig.Provider, ws.WorkDir, providerSessionID); path != "" {
				transcriptPath = path
				h.pushEventJSON(req.TaskID, "session", map[string]interface{}{
					"agent_id":            req.AgentID,
					"external_session_id": providerSessionID,
					"transcript_path":     transcriptPath,
				})
			}
		}

		// Emit a run update for every chunk the UI cares about.
		h.pushAgentActivity(req, agentInfo.Name, req.ModelConfig.Provider, chunk)

		switch chunk.Type {
		case string(agent.MessageText):
			// v1.3: — text output is internal thinking.
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
			if h.shuttingDown.Load() {
				return
			}
			slog.Error("task: backend stream error", "task_id", req.TaskID, "error", chunk.Content)
			h.taskManager.UpdateStatus(req.TaskID, taskStatusFailed)
			h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
				"agent_id": req.AgentID, "error": chunk.Content, "retryable": true,
			})
			h.taskManager.CloseAllSubscribers(req.TaskID)
			return

		case string(agent.MessageToolUse):
			if chunk.Tool != nil {
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

	if h.shuttingDown.Load() {
		return
	}

	// v1.3: - only CLI-sent messages appear in channel.
	// If solo message send was called, API already created the message.
	// Direct text output is internal thinking, not channel messages.

	// Check context cancellation
	if ctx.Err() != nil {
		slog.Info("task cancelled via context", "task_id", req.TaskID, "agent_id", req.AgentID)
		h.finishCancelledTask(req)
		return
	}

	// Get final result
	result, ok := readBackendFinalResult(ctx, session.Result, backendFinalResultWaitAfter)
	if !ok || result == nil {
		result = &agent.Result{Status: "failed", Error: "backend finished without a final result"}
	}
	finalStatus := backendFinalStatus(result)

	// v1.3: — NEVER persist text output as channel messages.
	// Only solo message send API (via proxy) creates visible messages.
	// All text output is internal thinking. Always skip persist.
	if strings.TrimSpace(fullContent) != "" {
		slog.Info("task: suppressing text output (not sent via CLI)", "agent_id", req.AgentID, "length", len(fullContent))
	}

	slog.Info("task backend completed",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"status", result.Status,
		"content_length", len(result.Output),
		"duration_ms", result.DurationMs,
	)

	// Extract usage from result
	var inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int
	if result.Usage != nil {
		for _, u := range result.Usage {
			inputTokens += int(u.InputTokens)
			outputTokens += int(u.OutputTokens)
			cacheReadTokens += int(u.CacheReadTokens)
			cacheWriteTokens += int(u.CacheWriteTokens)
		}
	}

	if finalStatus != "completed" {
		errMsg := backendErrorMessage(result)
		h.taskManager.UpdateStatus(req.TaskID, backendTaskStatus(finalStatus))
		h.pushEventJSON(req.TaskID, "error", map[string]interface{}{
			"agent_id":  req.AgentID,
			"error":     errMsg,
			"status":    finalStatus,
			"retryable": finalStatus != "cancelled",
			"usage": map[string]int{
				"input_tokens":       inputTokens,
				"output_tokens":      outputTokens,
				"cache_read_tokens":  cacheReadTokens,
				"cache_write_tokens": cacheWriteTokens,
			},
		})
		h.taskManager.CloseAllSubscribers(req.TaskID)
		return
	}

	h.taskManager.UpdateStatus(req.TaskID, backendTaskStatus(finalStatus))
	transcriptPath = refreshTranscriptPathForProvider(req.ModelConfig.Provider, ws.WorkDir, providerSessionID, transcriptPath)

	// Push complete event — notification only (no content, no persist).
	// Real messages arrive via solo message send → daemon proxy → server API → message.new.
	h.pushEventJSON(req.TaskID, "complete", map[string]interface{}{
		"agent_id":            req.AgentID,
		"external_session_id": providerSessionID,
		"transcript_path":     transcriptPath,
		"usage": map[string]int{
			"input_tokens":       inputTokens,
			"output_tokens":      outputTokens,
			"cache_read_tokens":  cacheReadTokens,
			"cache_write_tokens": cacheWriteTokens,
		},
	})

	// Push done sentinel and close. The done event is consumed by SSE
	// subscribers in order, eliminating the need for a delay.
	h.pushEventJSON(req.TaskID, "done", map[string]interface{}{})
	h.taskManager.CloseAllSubscribers(req.TaskID)
}

func (h *daemonHandler) materializeMessageAttachments(ctx context.Context, token string, messages []llmMessage, workDir string) []llmMessage {
	if len(messages) == 0 || workDir == "" {
		return messages
	}

	out := make([]llmMessage, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if len(msg.Attachments) == 0 {
			continue
		}

		attachments := make([]agent.Attachment, len(msg.Attachments))
		copy(attachments, msg.Attachments)
		for j := range attachments {
			localPath, err := h.materializeAttachment(ctx, workDir, token, &attachments[j])
			if err != nil {
				slog.Warn("task: failed to materialize attachment", "attachment_id", attachments[j].ID, "filename", attachments[j].Filename, "error", err)
				continue
			}
			attachments[j].LocalPath = localPath
		}
		out[i].Attachments = attachments
		out[i].Content = appendMaterializedAttachmentPaths(out[i].Content, attachments)
	}
	return out
}

func (h *daemonHandler) materializeAttachment(ctx context.Context, workDir, token string, attachment *agent.Attachment) (string, error) {
	localPath := attachment.LocalPath
	if localPath == "" {
		localPath = agent.AttachmentLocalPath(attachment.ID, attachment.Filename)
	}
	if filepath.IsAbs(localPath) {
		return "", fmt.Errorf("attachment local path must be relative")
	}

	dst := filepath.Join(workDir, filepath.FromSlash(localPath))
	rel, err := filepath.Rel(workDir, dst)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("attachment local path escapes workspace")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	if attachment.StoragePath != "" {
		if err := copyAttachmentFromStorage(attachment.StoragePath, dst); err == nil {
			return filepath.ToSlash(localPath), nil
		} else {
			slog.Warn("task: failed to copy attachment from storage, trying URL", "attachment_id", attachment.ID, "error", err)
		}
	}

	if attachment.URL == "" {
		return "", fmt.Errorf("attachment has neither storage_path nor url")
	}
	if err := h.downloadAttachment(ctx, attachment.URL, dst, token); err != nil {
		return "", err
	}
	return filepath.ToSlash(localPath), nil
}

func copyAttachmentFromStorage(storagePath, dst string) error {
	src, err := resolveDaemonAttachmentPath(storagePath)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func resolveDaemonAttachmentPath(storagePath string) (string, error) {
	if filepath.IsAbs(storagePath) {
		return "", fmt.Errorf("invalid attachment storage path")
	}
	root := daemonAttachmentsRoot()
	fullPath := filepath.Join(root, filepath.Clean(storagePath))
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid attachment storage path")
	}
	return fullPath, nil
}

func daemonAttachmentsRoot() string {
	if dir := os.Getenv("ATTACHMENTS_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".solo", "attachments")
	}
	return filepath.Join(".", "attachments")
}

func (h *daemonHandler) downloadAttachment(ctx context.Context, rawURL, dst, token string) error {
	downloadURL, err := h.resolveAttachmentURL(rawURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download attachment: status %d", resp.StatusCode)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	return out.Close()
}

func (h *daemonHandler) resolveAttachmentURL(rawURL string) (string, error) {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL, nil
	}
	if !strings.HasPrefix(rawURL, "/") {
		return "", fmt.Errorf("attachment url must be absolute or server-relative")
	}
	if h.serverURL == "" {
		return "", fmt.Errorf("server url is not configured")
	}
	return strings.TrimRight(h.serverURL, "/") + rawURL, nil
}

func appendMaterializedAttachmentPaths(content string, attachments []agent.Attachment) string {
	paths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.LocalPath == "" {
			continue
		}
		paths = append(paths, fmt.Sprintf("- %s -> %s", attachment.Filename, attachment.LocalPath))
	}
	if len(paths) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString(content)
	b.WriteString("\n\nMaterialized attachment files in this workspace:\n")
	for _, path := range paths {
		b.WriteString(path)
		b.WriteString("\n")
	}
	return b.String()
}

func backendFinalStatus(result *agent.Result) string {
	if result == nil {
		return "failed"
	}
	switch result.Status {
	case "completed", "timeout", "cancelled":
		return result.Status
	case "aborted":
		return "cancelled"
	default:
		return "failed"
	}
}

func readBackendFinalResult(ctx context.Context, resultCh <-chan *agent.Result, wait time.Duration) (*agent.Result, bool) {
	if resultCh == nil {
		return nil, false
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case result, ok := <-resultCh:
		return result, ok
	case <-ctx.Done():
		return &agent.Result{Status: "cancelled", Error: ctx.Err().Error()}, true
	case <-timer.C:
		return nil, false
	}
}

func backendTaskStatus(finalStatus string) string {
	switch finalStatus {
	case "completed":
		return taskStatusCompleted
	case "cancelled":
		return taskStatusCancelled
	default:
		return taskStatusFailed
	}
}

func backendErrorMessage(result *agent.Result) string {
	if result != nil && strings.TrimSpace(result.Error) != "" {
		return result.Error
	}
	switch backendFinalStatus(result) {
	case "timeout":
		return "agent execution timed out"
	case "cancelled":
		return "agent execution cancelled"
	default:
		return "agent execution failed"
	}
}

func transcriptPathForProvider(provider, workspaceDir, sessionID string) string {
	return agent.TranscriptPath(provider, workspaceDir, sessionID)
}

func refreshTranscriptPathForProvider(provider, workspaceDir, sessionID, current string) string {
	if current != "" || sessionID == "" {
		return current
	}
	return transcriptPathForProvider(provider, workspaceDir, sessionID)
}

// agentInfo holds agent metadata fetched from the database.
type agentInfo struct {
	Name       string
	CustomEnv  map[string]string // agent-level env overrides (from custom_env JSONB)
	CustomArgs []string          // agent-level CLI args (from custom_args JSONB)
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

// pushAgentActivity translates a backend OutputChunk into a run status update.
// Skips push when the chunk produces no activity text or no run status.
//
// Per-CLI adaptation (SOLO-island PR-fix): uses
// InferActivityTextForBackend so ACP backends (Kimi/Kiro/Hermes)
// get normalised tool names, the stream-json family gets
// namespace-stripped names, and all surfaces share the same Chinese
// pill text.
func (h *daemonHandler) pushAgentActivity(req runTaskRequest, agentName, provider string, chunk agent.OutputChunk) {
	if req.ChannelID == "" {
		return
	}
	activityText := agent.InferActivityTextForBackend(provider, chunk)
	if activityText == "" {
		return
	}
	status, ok := agent.InferRunStatusFromChunk(chunk)
	if !ok {
		return
	}

	var toolName, toolInputSummary string
	if chunk.Tool != nil {
		// Display name goes through NormalizeToolName so the island
		// pill shows the canonical form. The raw name is preserved
		// in tool_name for the AgentViewPanel to render exactly
		// what the backend said.
		toolName = agent.NormalizeToolName(provider, chunk.Tool.Name)
		toolInputSummary = agent.SummarizeToolInput(toolName, chunk.Tool.Input)
	}

	payload := map[string]interface{}{
		"channel_id":    req.ChannelID,
		"agent_id":      req.AgentID,
		"agent_name":    agentName,
		"status":        string(status),
		"activity_text": activityText,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	if toolName != "" {
		payload["tool_name"] = toolName
	}
	if toolInputSummary != "" {
		payload["tool_input_summary"] = toolInputSummary
	}
	if provider != "" {
		payload["source"] = provider
	}

	slog.Debug("daemon: pushing agent.run.updated", "task_id", req.TaskID, "channel_id", req.ChannelID, "agent_id", req.AgentID, "status", status, "activity_text", activityText)
	h.pushEventJSON(req.TaskID, "agent.run.updated", payload)
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

func resolveSoloBinary() string {
	candidates := make([]string, 0, 5)
	if configured := strings.TrimSpace(os.Getenv("SOLO_CLI_BIN")); configured != "" {
		candidates = append(candidates, configured)
	}
	if path, err := exec.LookPath("solo"); err == nil {
		candidates = append(candidates, path)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "solo"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, ".pids", "solo"),
			filepath.Join(wd, "bin", "solo"),
		)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

func soloSkillsRoot() string {
	if dir := os.Getenv("SOLO_SKILLS_DIR"); dir != "" {
		return dir
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "skills")
	}
	return "skills"
}

// ── Skill listing endpoints ──────────────────────────────────────────────────

// skillListItem is the JSON shape returned by the skill list endpoint.
type skillListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SourceKind  string `json:"source_kind"`
	SourcePath  string `json:"source_path"`
}

// HandleSkillsList returns discovered skills for an agent (global + workspace).
func (h *daemonHandler) HandleSkillsList(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"skills": []skillListItem{}})
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("skills list: no home dir", "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{"skills": []skillListItem{}})
		return
	}

	wsDir := ""
	if h.workspaceManager != nil {
		wsDir = h.workspaceManager.WorkspaceDir(agentID)
	}

	globalRoots := agentGlobalRoots(provider, home)
	var wsRoots []skillloader.SkillRoot
	if h.workspaceManager != nil {
		wsRoots = agentWorkspaceRoots(provider, wsDir)
	}

	allRoots := append([]skillloader.SkillRoot{}, globalRoots...)
	allRoots = append(allRoots, wsRoots...)

	discovered, err := skillloader.ScanRoots(home, allRoots)
	if err != nil {
		slog.Warn("skills list: scan failed", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan skills"})
		return
	}

	var skills []skillListItem
	for _, ds := range discovered {
		skills = append(skills, skillListItem{
			Name:        ds.Name,
			Description: ds.Description,
			SourceKind:  ds.SourceKind,
			SourcePath:  ds.SourcePath,
		})
	}

	globalPaths := uniquePaths(globalRoots, home, "")
	wsPaths := uniquePaths(wsRoots, home, wsDir)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"skills":          skills,
		"global_paths":    globalPaths,
		"workspace_paths": wsPaths,
	})
}

// uniquePaths returns unique Path strings from a slice of SkillRoot, preserving
// insertion order. home prefix → ~, strip prefix → relative (used for workspace).
func uniquePaths(roots []skillloader.SkillRoot, home, strip string) []string {
	seen := make(map[string]bool, len(roots))
	var out []string
	for _, r := range roots {
		if !seen[r.Path] {
			seen[r.Path] = true
			p := r.Path
			if strip != "" && strings.HasPrefix(p, strip) {
				p = p[len(strip)+1:] // +1 to skip the path separator after strip prefix
			} else if home != "" && strings.HasPrefix(p, home) {
				p = "~" + p[len(home):]
			}
			out = append(out, p)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
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
	rel, err := filepath.Rel(workspaceDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
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
	resolvedRel, err := filepath.Rel(workspaceDir, resolvedPath)
	if err != nil || strings.HasPrefix(resolvedRel, "..") {
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
	rel, err := filepath.Rel(workspaceDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
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
	resolvedRel, err := filepath.Rel(workspaceDir, resolvedPath)
	if err != nil || strings.HasPrefix(resolvedRel, "..") {
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

	relPath, err := filepath.Rel(basePath, dirPath)
	if err != nil {
		relPath = filepath.Base(dirPath)
	}
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

	relPath, err := filepath.Rel(basePath, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	node := workspaceNode{
		Type: "file",
		Name: info.Name(),
		Path: relPath,
		Size: info.Size(),
	}

	return node, nil
}

// ── Agent cleanup (hard-delete side effects) ────────────────────────────────
//
// CleanupAgent handles POST /internal/daemon/agents/{agentID}/cleanup.
// Called by the server after an agent is soft-deleted to release local
// resources: kills any running session subprocess, deletes the workspace
// directory, and deletes the memory file. Idempotent — missing resources
// are not errors.

// CleanupAgent handles POST /internal/daemon/agents/{agentID}/cleanup.
func (h *daemonHandler) CleanupAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	if agentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	h.cleanupAgent(agentID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *daemonHandler) cleanupAgent(agentID string) {
	slog.Info("daemon: cleanup agent requested", "agent_id", agentID)
	for provider, sm := range h.sessionManagers {
		if err := sm.ForceCloseSession(agentID); err != nil {
			slog.Warn("daemon: force-close session failed",
				"agent_id", agentID, "provider", provider, "error", err)
		}
	}

	if h.workspaceManager != nil {
		if err := h.workspaceManager.Cleanup(agentID); err != nil {
			slog.Warn("daemon: workspace cleanup failed",
				"agent_id", agentID, "error", err)
		}
	}

	if h.memoryManager != nil {
		if err := h.memoryManager.Delete(agentID); err != nil {
			slog.Warn("daemon: memory delete failed",
				"agent_id", agentID, "error", err)
		}
	}

}

type cleanupThinkingSessionsRequest struct {
	NodeIDs []string `json:"node_ids"`
}

// CleanupThinkingSessions handles POST /internal/daemon/thinking/cleanup.
// The server broadcasts this idempotent request when a channel is archived,
// because node-to-daemon affinity is intentionally not persisted yet.
func (h *daemonHandler) CleanupThinkingSessions(w http.ResponseWriter, r *http.Request) {
	var req cleanupThinkingSessionsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.NodeIDs) > 1000 {
		http.Error(w, "too many node_ids", http.StatusBadRequest)
		return
	}

	nodeIDs := make([]string, 0, len(req.NodeIDs))
	seen := make(map[string]struct{}, len(req.NodeIDs))
	for _, nodeID := range req.NodeIDs {
		if _, err := uuid.Parse(nodeID); err != nil {
			http.Error(w, "invalid node_id", http.StatusBadRequest)
			return
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		nodeIDs = append(nodeIDs, nodeID)
	}

	for provider, sm := range h.sessionManagers {
		for _, nodeID := range nodeIDs {
			if err := sm.ForceCloseThinkingSession(nodeID); err != nil {
				slog.Warn("daemon: force-close Thinking session failed", "node_id", nodeID, "provider", provider, "error", err)
			}
		}
	}
	slog.Info("daemon: Thinking session cleanup completed", "node_count", len(nodeIDs))
	w.WriteHeader(http.StatusNoContent)
}

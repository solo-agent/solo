package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

// Default agent auto-response settings.
const (
	defaultContextMessageCount = 1  // v1.3: Slock-aligned — deliver only the triggering message
	defaultDebounceDuration   = 2 * time.Second
)

// maxAgentChainDepth limits the depth of agent-to-agent trigger chains
// to prevent infinite loops. A chain of [A, B, C] means A triggered B,
// B triggered C; C cannot trigger another agent because depth == 3.
const maxAgentChainDepth = 3

	// cascadeProtection detects runaway dispatch loops per channel.
	const cascadeWindow = 10 * time.Second
	const cascadeThreshold = 20
	const cascadeCooldown = 60 * time.Second

	type cascadeCount struct {
		count         int
		windowStart   time.Time
		cooldownUntil time.Time
	}

// AgentService handles agent lifecycle, auto-response triggering, and status management.
type AgentService struct {
	pool *pgxpool.Pool
	dm   *DaemonManager
	hub  realtime.Broadcaster

	// debounceMap tracks the last trigger time per (channelID, agentID) pair
	// to prevent rapid re-triggering.
	debounceMap sync.Map
		// cascadeMap tracks dispatch velocity per channel for loop detection.
		cascadeMap sync.Map

	// httpClient for daemon communication
	httpClient *http.Client

	// claimWindow manages @mention priority claim windows for tasks.
	claimWindow *TaskClaimWindowManager

	// mentionSvc is used to resolve @mentions in agent responses
	// for agent-to-agent trigger chains.
	mentionSvc *MentionService
}

// NewAgentService creates a new AgentService.
func NewAgentService(pool *pgxpool.Pool, dm *DaemonManager, hub realtime.Broadcaster, mentionSvc *MentionService) *AgentService {
	return &AgentService{
		pool:        pool,
		dm:          dm,
		hub:         hub,
		httpClient:  &http.Client{Timeout: 120 * time.Second},
		claimWindow: NewTaskClaimWindowManager(),
		mentionSvc:  mentionSvc,
	}
}

// debounceKey generates a unique key for debounce tracking.
func debounceKey(channelID, agentID string) string {
	return channelID + ":" + agentID
}

// checkDebounce returns true if the agent was triggered within the debounce window.
func (s *AgentService) checkDebounce(channelID, agentID string) bool {
	key := debounceKey(channelID, agentID)
	lastTrigger, ok := s.debounceMap.Load(key)
	if !ok {
		return false
	}
	return time.Since(lastTrigger.(time.Time)) < defaultDebounceDuration
}

// updateDebounce records the last trigger time for debounce.
func (s *AgentService) updateDebounce(channelID, agentID string) {
	key := debounceKey(channelID, agentID)
	s.debounceMap.Store(key, time.Now())
}

// checkThreadDebounce returns true if the agent was triggered in this thread
// within the debounce window. Uses a thread-scoped key separate from channel-level
// debounce so thread follow-ups are not blocked by channel-level triggers.
func (s *AgentService) checkThreadDebounce(channelID, threadID, agentID string) bool {
	key := channelID + ":" + threadID + ":" + agentID
	lastTrigger, ok := s.debounceMap.Load(key)
	if !ok {
		return false
	}
	return time.Since(lastTrigger.(time.Time)) < defaultDebounceDuration
}

// updateThreadDebounce records the last trigger time for thread-level debounce.
func (s *AgentService) updateThreadDebounce(channelID, threadID, agentID string) {
	key := channelID + ":" + threadID + ":" + agentID
	s.debounceMap.Store(key, time.Now())
}

// TriggerAgentResponse checks whether an Agent should respond to a new message,
// gathers context, and dispatches the task to a Daemon via streaming SSE.
//
// mentionedAgentIDs: if non-empty, only the mentioned agents are triggered.
// If empty but hasMentions is true, @patterns existed but none resolved — suppress
// all agent responses. If empty and hasMentions is false, trigger all active
// agents (auto-response).
//
// Called after a message is persisted and broadcast.
func (s *AgentService) TriggerAgentResponse(ctx context.Context, channelID, messageID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string) {
	// If the message was sent by an agent, only proceed if it @mentions other agents.
	// Agents don't respond to themselves or to non-mentioned agent messages.
	if senderType == "agent" {
		if len(mentionedAgentIDs) == 0 {
			return // agent talking — no @mentions to other agents, skip
		}
		// Has @mentions to other agents — proceed to trigger only those agents
	}

	// Find active agent members of this channel
	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		slog.Error("failed to get channel active agents", "channel_id", channelID, "error", err)
		return
	}

	// Resolve @mentioned agent names for context awareness in the system prompt.
	// This tells each agent WHO was mentioned, enabling "if it's for someone else, stay out."
	mentionedNames := s.resolveMentionedNames(ctx, mentionedAgentIDs)

	// v1.3: ALL agents receive ALL messages. No filtering by @mention.
	// Each agent decides autonomously whether to participate based on the
	// CRITICAL RULES in the system prompt (aligned with Slock's model).
	// The mentionedNames list provides context — agents know if they were
	// the target or if the message was for someone else.
	targetAgents := agents

	if len(targetAgents) == 0 {
		return
	}

	// Get last N messages for context
	contextMessages, err := s.getRecentMessages(ctx, channelID, defaultContextMessageCount)
	if err != nil {
		slog.Error("failed to get context messages", "channel_id", channelID, "error", err)
		return
	}

	// Query for open tasks in the channel to provide task context to agents
	taskContext := s.getChannelOpenTasksSummary(ctx, channelID)

	// For each target agent, check debounce and dispatch via streaming
	for _, ag := range targetAgents {
		// Skip if agent was triggered recently
		if s.checkDebounce(channelID, ag.ID) {
			slog.Debug("agent debounced", "agent_id", ag.ID, "channel_id", channelID)
			continue
		}
		// Set debounce BEFORE spawning the goroutine to prevent
		// concurrent duplicate triggers from REST and WS handlers.
		s.updateDebounce(channelID, ag.ID)

		// SOLO-228-B: Validate agent trigger chain to prevent infinite loops.
		if agentChain != nil {
			if len(agentChain) >= maxAgentChainDepth {
				slog.Debug("agent chain depth limit reached", "agent_id", ag.ID, "channel_id", channelID, "depth", len(agentChain))
				continue
			}
			if containsStr(agentChain, ag.ID) {
				slog.Debug("agent already in trigger chain, skipping", "agent_id", ag.ID, "channel_id", channelID)
				continue
			}
		}
		newChain := append([]string(nil), agentChain...)
		newChain = append(newChain, ag.ID)


		// Select a daemon that supports this agent's model provider
		daemon := s.dm.SelectDaemon("llm")
		if daemon == nil {
			slog.Warn("no available daemon for agent task", "agent_id", ag.ID)
			continue
		}

		slog.Debug("triggering agent response",
			"agent_id", ag.ID,
			"agent_name", ag.Name,
			"channel_id", channelID,
			"message_id", messageID,
			"daemon_id", daemon.ID,
		)

		// Build the task request
		taskReq := daemonTaskRequest{
			TaskID:       uuid.New().String(),
			AgentID:      ag.ID,
			ChannelID:    channelID,
			Messages:     contextMessages,
			SystemPrompt: ag.SystemPrompt,
			ModelConfig: agent.ModelConfig{
				Provider:    ag.ModelProvider,
				Model:       ag.ModelName,
				Temperature: ag.Temperature,
				MaxTokens:   ag.MaxTokens,
			},
			TaskContext:    taskContext,
				AgentChain:     newChain,
				MentionedNames: mentionedNames,
		}

		// Dispatch via streaming SSE and handle events
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
	}
}

// --- Broadcast dispatch helpers ---
// These route to thread-scoped or channel-scoped broadcasts based on whether
// a ThreadID is present (P25-05-B).

func (s *AgentService) broadcastAgentThinking(threadID, channelID, agentID, agentName, thought string) {
	if threadID != "" {
		s.broadcastThreadThinking(threadID, channelID, agentID, agentName, thought)
	} else {
		s.broadcastThinking(channelID, agentID, agentName, thought)
	}
}


func (s *AgentService) broadcastAgentError(threadID, channelID, agentID, agentName, errMsg string) {
	if threadID != "" {
		s.broadcastThreadError(threadID, channelID, agentID, agentName, errMsg)
	} else {
		s.broadcastError(channelID, agentID, agentName, errMsg)
	}
}


// handleStreamingAgentTask dispatches a task to a daemon via SSE streaming
// and forwards events to WebSocket subscribers.
func (s *AgentService) handleStreamingAgentTask(ctx context.Context, daemon *DaemonInfo, taskReq daemonTaskRequest, ag agentChannelInfo) {
	// Get agent display name
	agentName := ag.Name
	if agentName == "" {
		agentName = "Agent"
	}

	// Track this task for daemon offline cleanup (cleanup on return)
	s.dm.TrackTask(taskReq.TaskID, daemon.ID, ag.ID)
	defer s.dm.RemoveTask(taskReq.TaskID)

	// Use a timeout context so tasks don't hang indefinitely (e.g., LLM API hang)
	// 5 minutes should be sufficient for any agent response.
	streamCtx, streamCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer streamCancel()

	// Pre-generate a message ID for the streaming message
	messageID := uuid.New().String()

	slog.Debug("dispatching agent streaming task",
		"task_id", taskReq.TaskID,
		"agent_id", ag.ID,
		"daemon_id", daemon.ID,
		"channel_id", taskReq.ChannelID,
	)

	// Broadcast thinking event immediately
	s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "Processing request...")

	// Broadcast user trigger message as context for agent view
	if len(taskReq.Messages) > 0 {
		lastMsg := taskReq.Messages[len(taskReq.Messages)-1]
		s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "context", lastMsg.Content, nil)
	}

	// Send via SSE streaming
	eventCh, err := s.dm.StreamTask(streamCtx, daemon, taskReq)
	if err != nil {
		slog.Error("failed to stream agent task",
			"task_id", taskReq.TaskID,
			"agent_id", ag.ID,
			"daemon_id", daemon.ID,
			"error", err,
		)
		s.broadcastAgentError(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "Failed to start task: "+err.Error())
		return
	}

	// Track usage for logging
	var inputTokens, outputTokens int
	taskCompleted := false

	for event := range eventCh {
		switch event.Event {
		case "thinking":
			var data struct {
				AgentID string `json:"agent_id"`
				Thought string `json:"thought"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, data.Thought)
			}


		case "text":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				Content   string `json:"content"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "text", data.Content, nil)
			}

		case "tool_use":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				ToolName  string `json:"tool_name"`
				ToolInput string `json:"tool_input"`
				CallID    string `json:"call_id"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "tool_use", "", map[string]interface{}{
					"name":    data.ToolName,
					"input":   data.ToolInput,
					"call_id": data.CallID,
				})
			}

		case "tool_result":
			var data struct {
				AgentID   string `json:"agent_id"`
				AgentName string `json:"agent_name"`
				ToolName  string `json:"tool_name"`
				Output    string `json:"output"`
				CallID    string `json:"call_id"`
				IsError   bool   `json:"is_error"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				s.broadcastAgentChunk(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "tool_result", data.Output, map[string]interface{}{
					"name":   data.ToolName,
					"output": data.Output,
					"call_id": data.CallID,
				})
			}

		case "complete":
			var data struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				inputTokens = data.Usage.InputTokens
				outputTokens = data.Usage.OutputTokens
				taskCompleted = true
			}

		case "error":
			var data struct {
				AgentID string `json:"agent_id"`
				Error   string `json:"error"`
			}
			if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
				slog.Error("agent task stream error",
					"agent_id", ag.ID,
					"error", data.Error,
				)
				s.broadcastAgentError(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, data.Error)
			}
			return
		}
	}

	// If task was not completed, skip saving
	if !taskCompleted {
		slog.Warn("agent task stream ended without complete event",
			"agent_id", ag.ID,
			"channel_id", taskReq.ChannelID,
		)
		s.broadcastAgentThinking(taskReq.ThreadID, taskReq.ChannelID, ag.ID, agentName, "Response ended unexpectedly.")
		return
	}

	// v1.3: Slock-aligned dual-channel architecture.
	// Agent text output is internal thinking (streamed nowhere).
	// Real messages arrive via solo message send → daemon proxy → server API → message.new.
	// The complete event here is purely for usage tracking and status notification.
	slog.Info("agent streaming task completed",
		"agent_id", ag.ID,
		"channel_id", taskReq.ChannelID,
		"message_id", messageID,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
	)
}

// --- Internal broadcast helpers (use realtime.Broadcaster directly) ---

func (s *AgentService) broadcastThinking(channelID, agentID, agentName, thought string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"thought":    thought,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.thinking",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(channelID, envelope)
}


func (s *AgentService) broadcastError(channelID, agentID, agentName, errMsg string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"error":      errMsg,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.error",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(channelID, envelope)
}


func (s *AgentService) broadcastTaskClaimed(task *Task, channelID string) {
	if task == nil || s.hub == nil {
		return
	}

	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format(time.RFC3339)
	}

	// Broadcast task.updated
	taskPayload := map[string]interface{}{
		"id":          task.ID,
		"task_number": task.TaskNumber,
		"channel_id":  task.ChannelID,
		"title":       task.Title,
		"description": task.Description,
		"status":      task.Status,
		"claimer_id":  task.ClaimerID,
		"priority":    task.Priority,
		"due_date":    dueDate,
		"message_id":  task.MessageID,
		"updated_at":  task.UpdatedAt.Format(time.RFC3339),
	}
	taskData, _ := json.Marshal(taskPayload)
	taskEnvelope, _ := json.Marshal(map[string]interface{}{
		"type":    "task.updated",
		"payload": json.RawMessage(taskData),
	})
	s.hub.BroadcastToChannel(channelID, taskEnvelope)

	// Broadcast message.updated (for TaskBadge real-time update)
	if task.MessageID != "" {
		msgPayload := map[string]interface{}{
			"id":                task.MessageID,
			"channel_id":        channelID,
			"task_number":       task.TaskNumber,
			"task_status":       task.Status,
			"task_claimer_name": task.ClaimerName,
		}
		msgData, _ := json.Marshal(msgPayload)
		msgEnvelope, _ := json.Marshal(map[string]interface{}{
			"type":    "message.updated",
			"payload": json.RawMessage(msgData),
		})
		s.hub.BroadcastToChannel(channelID, msgEnvelope)
	}

	slog.Debug("broadcast task auto-claim",
		"task_id", task.ID,
		"channel_id", channelID,
		"claimer_id", task.ClaimerID,
	)
}


// parseAndExecuteTaskClaims scans the agent's accumulated output text for
// inline task claim directives and executes them. It returns the list of
// task numbers that were successfully claimed.
//
// Claim syntax: /claim #N or /认领 #N (case-insensitive)
//
// For each match:
//   - Resolves task_number → UUID via the tasks table (scoped to channel)
//   - Calls TaskService.ClaimTask
//   - On success: broadcasts task.updated + message.updated to the channel
//   - On already-claimed (409): logs and continues silently
//   - On not-found: logs and continues


func (s *AgentService) broadcastThreadThinking(threadID, channelID, agentID, agentName, thought string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"thought":    thought,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.thinking",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
}


func (s *AgentService) broadcastThreadError(threadID, channelID, agentID, agentName, errMsg string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"error":      errMsg,
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.error",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
}


// broadcastThreadMessage broadcasts final message to thread scope plus
// thread.reply notification to channel subscribers.
func (s *AgentService) broadcastThreadMessage(threadID, channelID, agentID, agentName, messageID, content string) {
	payload := map[string]interface{}{
		"id":           messageID,
		"channel_id":   channelID,
		"thread_id":    threadID,
		"sender_type":  "agent",
		"sender_id":    agentID,
		"sender_name":  agentName,
		"content":      content,
		"content_type": "text",
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "thread.message.new",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)

	// Also broadcast thread.reply to channel scope
	replyPayload := map[string]interface{}{
		"thread_id":   threadID,
		"channel_id":  channelID,
		"message_id":  messageID,
		"sender_id":   agentID,
		"sender_name": agentName,
	}
	replyData, _ := json.Marshal(replyPayload)
	replyEnvelope, _ := json.Marshal(map[string]interface{}{
		"type":    "thread.reply",
		"payload": json.RawMessage(replyData),
	})
	s.hub.BroadcastToChannel(channelID, replyEnvelope)
}

func (s *AgentService) broadcastAgentChunk(threadID, channelID, agentID, agentName, chunkType, content string, tool map[string]interface{}) {
	if threadID != "" {
		payload := map[string]interface{}{
			"channel_id": channelID,
			"agent_id":   agentID,
			"agent_name": agentName,
			"chunk_type": chunkType,
			"content":    content,
		}
		if tool != nil {
			payload["tool"] = tool
		}
		data, _ := json.Marshal(payload)
		envelope, _ := json.Marshal(map[string]interface{}{
			"type":    "agent.chunk",
			"payload": json.RawMessage(data),
		})
		s.hub.BroadcastToScope(realtime.ScopeThread, threadID, envelope)
	} else {
		payload := map[string]interface{}{
			"channel_id": channelID,
			"agent_id":   agentID,
			"agent_name": agentName,
			"chunk_type": chunkType,
			"content":    content,
		}
		if tool != nil {
			payload["tool"] = tool
		}
		data, _ := json.Marshal(payload)
		envelope, _ := json.Marshal(map[string]interface{}{
			"type":    "agent.chunk",
			"payload": json.RawMessage(data),
		})
		s.hub.BroadcastToChannel(channelID, envelope)
	}
}


func (s *AgentService) HandleTaskComplete(ctx context.Context, req *TaskCompleteRequest) error {
	// Remove from pending task tracking
	s.dm.RemoveTask(req.TaskID)

	var senderName string
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id = $1`, req.AgentID,
	).Scan(&senderName)
	if err != nil {
		senderName = "Agent"
	}

	// Persist the message to DB (callback path may not go through SSE streaming)
	if req.MessageID != "" && req.Content != "" {
		_, _ = s.pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, created_at, updated_at)
			 VALUES ($1, $2, 'agent', $3, $4, $5, $6, $6)
			 ON CONFLICT (id) DO UPDATE SET content = $4, updated_at = $6`,
			req.MessageID, req.ChannelID, req.AgentID, req.Content, nullableStr(req.ThreadID), time.Now(),
		)
	}

	// P25-05-B: Route broadcast based on thread ID.
	// If thread ID is present, broadcast to thread scope + thread.reply to channel.
	// If thread ID is empty, log error and skip (no fallback to channel).
	if req.ThreadID != "" {
		s.broadcastThreadMessage(req.ThreadID, req.ChannelID, req.AgentID, senderName, req.MessageID, req.Content)
	} else {
		slog.Error("HandleTaskComplete: no thread_id for task response, skipping broadcast",
			"task_id", req.TaskID,
			"agent_id", req.AgentID,
			"channel_id", req.ChannelID,
		)
	}

	slog.Info("agent response broadcast (callback)",
		"task_id", req.TaskID,
		"agent_id", req.AgentID,
		"channel_id", req.ChannelID,
		"message_id", req.MessageID,
	)

	return nil
}

// HandleTaskError processes a task error callback from the daemon.
func (s *AgentService) HandleTaskError(ctx context.Context, req *TaskErrorRequest) error {
	// Remove from pending task tracking
	s.dm.RemoveTask(req.TaskID)

	// Look up agent name for the error broadcast so the frontend can display
	// a meaningful sender name instead of "?".
	agentName := "Agent"
	var dbName string
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id = $1`, req.AgentID,
	).Scan(&dbName)
	if err == nil && dbName != "" {
		agentName = dbName
	}

	data, _ := json.Marshal(map[string]interface{}{
		"channel_id": req.ChannelID,
		"agent_id":   req.AgentID,
		"agent_name": agentName,
		"error":      req.Error,
	})
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    "agent.error",
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(req.ChannelID, envelope)

	slog.Error("agent task error", "task_id", req.TaskID, "agent_id", req.AgentID, "error", req.Error)
	return nil
}

// --- Thread trigger ---

// TriggerAgentResponseInThread triggers agent auto-response for a thread message.
func (s *AgentService) TriggerAgentResponseInThread(ctx context.Context, channelID, threadID, senderType, senderID string, mentionedAgentIDs []string, hasMentions bool, agentChain []string) {
	if senderType == "agent" {
		if len(mentionedAgentIDs) == 0 {
			return // agent talking — no @mentions to other agents, skip
		}
		// Has @mentions to other agents — proceed to trigger only those agents
	}

	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		slog.Error("failed to get channel active agents for thread", "channel_id", channelID, "error", err)
		return
	}

	// Determine target agents for this thread follow-up:
	// - If @mentions provided: only trigger the mentioned agents.
	// - If no @mentions: only trigger agents that have already replied in this thread.
	var targetAgents []agentChannelInfo
	if len(mentionedAgentIDs) > 0 {
		mentionedSet := make(map[string]bool, len(mentionedAgentIDs))
		for _, id := range mentionedAgentIDs {
			mentionedSet[id] = true
		}
		for _, ag := range agents {
			if mentionedSet[ag.ID] {
				targetAgents = append(targetAgents, ag)
			}
		}
	} else if hasMentions {
		// Mentions were present but none resolved to active agents — do not fall back.
		slog.Info("mentions found but none resolved to active agents in thread, skipping", "channel_id", channelID, "thread_id", threadID)
		return
	} else {
		// No @mentions — only trigger agents that have already replied in this thread.
		threadAgentIDs, err := s.getThreadParticipantAgents(ctx, threadID)
		if err != nil {
			slog.Error("failed to get thread participant agents for follow-up", "thread_id", threadID, "error", err)
			return
		}
		if len(threadAgentIDs) == 0 {
			return
		}
		threadAgentSet := make(map[string]bool, len(threadAgentIDs))
		for _, id := range threadAgentIDs {
			threadAgentSet[id] = true
		}
		for _, ag := range agents {
			if threadAgentSet[ag.ID] {
				targetAgents = append(targetAgents, ag)
			}
		}
	}

	if len(targetAgents) == 0 {
		return
	}

	// Get thread context messages
	threadSvc := NewThreadService(s.pool)
	threadMsgs, err := threadSvc.GetThreadContextMessages(ctx, threadID)
	if err != nil {
		slog.Error("failed to get thread context messages", "thread_id", threadID, "error", err)
		return
	}

	// Resolve channel name/type for Slock-aligned target headers in thread context.
	var threadChannelName, threadChannelType string
	_ = s.pool.QueryRow(ctx, `SELECT c.name, c.type FROM channels c
		JOIN threads t ON t.channel_id = c.id WHERE t.id = $1`, threadID,
	).Scan(&threadChannelName, &threadChannelType)
	if threadChannelName == "" { slog.Warn("thread trigger: failed to resolve channel name", "thread_id", threadID) }

	contextMsgs := make([]agent.Message, len(threadMsgs))
	for i, tm := range threadMsgs {
		role := agent.RoleUser
		if tm.SenderType == "agent" {
			role = agent.RoleAssistant
		}
		target := "#" + threadChannelName
		if threadChannelType == "dm" {
			target = "dm:@" + threadChannelName
		}
		shortID := tm.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		// Slock-aligned header with thread :shortid suffix.
		senderName := tm.SenderName
		if senderName == "" {
			senderName = tm.SenderID
		}
		header := fmt.Sprintf("[target=%s:%s msg=%s time=%s type=%s] @%s:",
			target, shortID, shortID,
			tm.CreatedAt.UTC().Format(time.RFC3339), tm.SenderType, senderName)
		contextMsgs[i] = agent.Message{
			Role:     role,
			Content:  header + " " + tm.Content,
			SenderID: tm.SenderID,
		}
	}
	// Prepend a system header when the agent is @mentioned into a new thread.
		if len(mentionedAgentIDs) > 0 {
			threadTargetPrefix := "#" + threadChannelName
			if threadChannelType == "dm" {
				threadTargetPrefix = "dm:@" + threadChannelName
			}
			threadShortID := threadID
			if len(threadShortID) > 8 {
				threadShortID = threadShortID[:8]
			}
			systemHeader := fmt.Sprintf("[System: You were added to a new thread via @mention. Reply in this thread.]\n"+
				"target: %s:%s\n"+
				"read thread: solo message read --target '%s:%s'",
				threadTargetPrefix, threadShortID, threadTargetPrefix, threadShortID)
			contextMsgs = append([]agent.Message{{
				Role:     agent.RoleSystem,
				Content:  systemHeader,
				SenderID: "system",
			}}, contextMsgs...)
		}
	// Query for open tasks in the channel to provide task context to agents
	taskContext := s.getChannelOpenTasksSummary(ctx, channelID)

		// Resolve @mentioned agent names for context awareness.
		threadMentionedNames := s.resolveMentionedNames(ctx, mentionedAgentIDs)

	for _, ag := range targetAgents {
		// Use thread-scoped debounce so thread follow-ups are not blocked
		// by channel-level triggers for the same agent.
		if s.checkThreadDebounce(channelID, threadID, ag.ID) {
			slog.Debug("agent debounced for thread", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID)
			continue
		}
		s.updateThreadDebounce(channelID, threadID, ag.ID)

			// SOLO-228-B: Validate agent trigger chain to prevent infinite loops.
			if agentChain != nil {
				if len(agentChain) >= maxAgentChainDepth {
					slog.Debug("agent chain depth limit reached (thread)", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID, "depth", len(agentChain))
					continue
				}
				if containsStr(agentChain, ag.ID) {
					slog.Debug("agent already in trigger chain (thread), skipping", "agent_id", ag.ID, "channel_id", channelID, "thread_id", threadID)
					continue
				}
			}
			newChain := append([]string(nil), agentChain...)
			newChain = append(newChain, ag.ID)


		daemon := s.dm.SelectDaemon("llm")
		if daemon == nil {
			slog.Warn("no available daemon for thread agent task", "agent_id", ag.ID)
			continue
		}

		taskReq := daemonTaskRequest{
			TaskID:       uuid.New().String(),
			AgentID:      ag.ID,
			ChannelID:    channelID,
			ThreadID:     threadID,
			Messages:     contextMsgs,
			SystemPrompt: ag.SystemPrompt,
			ModelConfig: agent.ModelConfig{
				Provider:    ag.ModelProvider,
				Model:       ag.ModelName,
				Temperature: ag.Temperature,
				MaxTokens:   ag.MaxTokens,
			},
			TaskContext:    taskContext,
				AgentChain:     newChain,
				MentionedNames: threadMentionedNames,
		}

		// Use streaming for thread as well
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
	}
}

// CheckClaimWindow checks whether the given claimerID is allowed to claim the
// task at this time. Returns (allowed bool, reason string).
func (s *AgentService) CheckClaimWindow(taskID, claimerID string) (bool, string) {
	return s.claimWindow.CheckClaimAllowed(taskID, claimerID)
}

// CloseClaimWindow explicitly closes the claim window for a task.
func (s *AgentService) CloseClaimWindow(taskID string) {
	s.claimWindow.CloseWindow(taskID)
}

// TriggerAgentForTask triggers an agent to respond to a task assignment.
// SOLO-123-B: Agent execution triggered by task assignment.
// The agent's response is routed to the task's thread (not the main channel),
// so replies stay organized under the task.

// TriggerAllAgentsForTask triggers active agents in a channel to respond
// to a newly created task. Agent replies are routed to the task's thread.
//
// If mentionedAgentIDs is non-empty, only the @mentioned agents are triggered
// immediately and a 30-second priority claim window is opened. After the window
// expires, remaining agents are triggered if the task is still unclaimed.
// Without @mentions, all agents are triggered at once (existing behavior).
func (s *AgentService) TriggerAllAgentsForTask(ctx context.Context, channelID, taskID string, taskNumber int, taskTitle string, mentionedAgentIDs []string, agentChain []string) {
	agents, err := s.getChannelActiveAgents(ctx, channelID)
	if err != nil || len(agents) == 0 {
		return
	}

	// If @mentions found, still trigger all agents immediately and autonomously.
	// Each agent receives the task context and decides whether to claim via /claim #N.
	// The mention set and claim window are preserved for priority notification.
	if len(mentionedAgentIDs) > 0 {
		// Open the claim priority window for mentioned agents
		s.claimWindow.OpenWindow(taskID, mentionedAgentIDs)

		// Trigger ALL agents immediately — not just @mentioned ones.
		// In v1.3, every agent evaluates autonomously and decides whether to claim.
		for _, ag := range agents {
			go s.TriggerAgentForTask(ctx, channelID, taskID, ag.ID, taskNumber, taskTitle, "", agentChain, mentionedAgentIDs)
		}
		return
	}

	// No @mentions — trigger all agents (existing behavior)
	for _, ag := range agents {
		go s.TriggerAgentForTask(ctx, channelID, taskID, ag.ID, taskNumber, taskTitle, "", agentChain, mentionedAgentIDs)
	}
}

func (s *AgentService) TriggerAgentForTask(ctx context.Context, channelID, taskID, agentID string, taskNumber int, taskTitle, taskDescription string, agentChain, mentionedAgentIDs []string) {
	// Get agent info
	var ag agentChannelInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, model_provider, model_name, system_prompt, temperature, max_tokens
		 FROM agents WHERE id = $1 AND is_active = true`,
		agentID,
	).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName,
		&ag.SystemPrompt, &ag.Temperature, &ag.MaxTokens)
	if err != nil {
		slog.Error("failed to get agent info for task trigger",
			"agent_id", agentID, "task_id", taskID, "error", err,
		)
		return
	}

	// Look up the task's message_id so we can route the agent response to the
	// task's thread rather than the main channel.
	var messageID string
	var taskMsgChannelID string
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(message_id::text, ''), channel_id FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&messageID, &taskMsgChannelID)
	if err != nil {
		slog.Error("failed to look up task for thread routing",
			"task_id", taskID, "error", err,
		)
		return
	}

	// Resolve thread ID from the task's message. If the task was created from
	// a message (ConvertMessageToTask) or if CreateTask already created a thread,
	// the thread exists. Otherwise we create one so the agent response goes to
	// a dedicated thread.
	var threadID string
	if messageID != "" {
		cid := channelID
		if taskMsgChannelID != "" {
			cid = taskMsgChannelID
		}
		threadSvc := NewThreadService(s.pool)
		tid, _, err := threadSvc.GetOrCreateThread(ctx, cid, messageID)
		if err != nil {
			slog.Warn("failed to get-or-create thread for task agent response",
				"task_id", taskID, "message_id", messageID, "error", err,
			)
		} else {
			threadID = tid
		}
	}

	// P25-05-B: If no thread could be resolved for the task, do not fall back
	// to channel-level broadcast. The agent response MUST go to a thread.
	if threadID == "" {
		slog.Error("TriggerAgentForTask: no thread_id resolved, skipping agent trigger",
			"task_id", taskID,
			"agent_id", agentID,
			"channel_id", channelID,
		)
		return
	}

	// Select daemon
	daemon := s.dm.SelectDaemon("llm")
	if daemon == nil {
		slog.Warn("no available daemon for task agent trigger", "agent_id", ag.ID, "task_id", taskID)
		return


	}

		// SOLO-228-B: Validate agent trigger chain for task dispatching.
		if agentChain != nil {
			if len(agentChain) >= maxAgentChainDepth {
				slog.Debug("agent chain depth limit reached (task)", "agent_id", agentID, "channel_id", channelID, "depth", len(agentChain))
				return
			}
			if containsStr(agentChain, agentID) {
				slog.Debug("agent already in trigger chain (task), skipping", "agent_id", agentID, "channel_id", channelID)
				return
			}
		}
		newChain := append([]string(nil), agentChain...)
		newChain = append(newChain, agentID)


	// Slock-aligned: deliver the task as a regular channel message with
	// [task #N status=todo] suffix, preserving the original sender and content.
	// The agent sees the same format as any other message — @mention context intact.
	var channelName, channelType string
	_ = s.pool.QueryRow(ctx, `SELECT name, type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType)

	target := "#" + channelName
	if channelType == "dm" {
		target = "dm:@" + channelName
	}

	shortMsgID := ""
	if len(messageID) >= 8 {
		shortMsgID = messageID[:8]
		target += ":" + shortMsgID
	}

	// Fetch sender info from the task's linked message
	var senderType, senderID, senderName, msgContent, msgCreatedAt string
	if messageID != "" {
		_ = s.pool.QueryRow(ctx,
			`SELECT m.sender_type, m.sender_id,
			        COALESCE(u.display_name, a.name, m.sender_id::text),
			        COALESCE(m.content, ''),
			        COALESCE(to_char(m.created_at, 'YYYY-MM-DD HH24:MI:SS'), '')
			 FROM messages m
			 LEFT JOIN users u ON m.sender_id = u.id
			 LEFT JOIN agents a ON m.sender_id = a.id
			 WHERE m.id = $1`,
			messageID,
		).Scan(&senderType, &senderID, &senderName, &msgContent, &msgCreatedAt)
	}

	if senderName == "" || senderName == messageID {
		senderName = s.resolveSenderName(ctx, senderType, senderID)
	}

	taskContent := fmt.Sprintf("New message received:\n\n[target=%s msg=%s time=%s type=%s] @%s: %s",
		target, shortMsgID, msgCreatedAt, senderType, senderName, msgContent)
	taskContent += fmt.Sprintf(" [task #%d status=todo]", taskNumber)
	taskContent += "\n\nRespond as appropriate. Complete all your work before stopping."
	taskContent += "\nReply in the channel or create/reply in a thread as appropriate; use each message's `target` and `msg` fields to choose the exact target."

	contextMsgs := []agent.Message{
		{Role: agent.RoleUser, Content: taskContent, SenderID: ""},
	}

	taskReq := daemonTaskRequest{
		TaskID:       uuid.New().String(),
		AgentID:      ag.ID,
		ChannelID:    channelID,
		ThreadID:     threadID,
		Messages:     contextMsgs,
		SystemPrompt: ag.SystemPrompt,
		ModelConfig: agent.ModelConfig{
			Provider:    ag.ModelProvider,
			Model:       ag.ModelName,
			Temperature: ag.Temperature,
			MaxTokens:   ag.MaxTokens,
		},
		// v1.3: No OriginTaskID — agents complete tasks via /done #N directives.
	}

	slog.Info("triggering agent for task",
		"agent_id", ag.ID,
		"task_id", taskID,
		"channel_id", channelID,
		"thread_id", threadID,
	)

		// v1.3: Agent decides whether to claim autonomously after reading task context.
	// The daemon task request includes task context. The agent evaluates the task,
	// and if it decides to claim, outputs /claim #N which is parsed by
		go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
}

// agentChannelInfo holds agent data needed for triggering.
type agentChannelInfo struct {
	ID            string
	Name          string
	ModelProvider string
	ModelName     string
	SystemPrompt  string
	Temperature   float64
	MaxTokens     int
}

// getChannelActiveAgents queries all active agent members of a channel.
func (s *AgentService) getChannelActiveAgents(ctx context.Context, channelID string) ([]agentChannelInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.name, a.model_provider, a.model_name,
		        a.system_prompt, a.temperature, a.max_tokens
		 FROM channel_members cm
		 JOIN agents a ON a.id = cm.member_id
		 WHERE cm.channel_id = $1
		   AND cm.member_type = 'agent'
		   AND a.is_active = true`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []agentChannelInfo
	for rows.Next() {
		var a agentChannelInfo
		if err := rows.Scan(&a.ID, &a.Name, &a.ModelProvider, &a.ModelName,
			&a.SystemPrompt, &a.Temperature, &a.MaxTokens); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if agents == nil {
		agents = []agentChannelInfo{}
	}

	return agents, nil
}

// getThreadParticipantAgents returns the distinct agent sender IDs that have
// already replied in the given thread. Used to scope thread follow-up triggers.
func (s *AgentService) getThreadParticipantAgents(ctx context.Context, threadID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT sender_id FROM messages
		 WHERE thread_id = $1 AND sender_type = 'agent'`,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		agentIDs = append(agentIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agentIDs, nil
}

// getRecentMessages returns the most recent N messages in a channel as agent messages.
// v1.3: Slock-aligned format — each message gets a structured header plus
// "Respond as appropriate" routing instruction on the last message.
func (s *AgentService) getRecentMessages(ctx context.Context, channelID string, limit int) ([]agent.Message, error) {
	// Get channel name for the target header.
	var channelName, channelType string
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(name, id::text), type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType)

	msgTarget := "#" + channelName
	if channelType == "dm" {
		msgTarget = "dm:@" + channelName
	}

	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.sender_type, m.sender_id, m.content, m.created_at
		 FROM messages m
		 WHERE m.channel_id = $1 AND m.thread_id IS NULL
		 ORDER BY m.created_at DESC, m.id DESC
		 LIMIT $2`,
		channelID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type msgRow struct {
		id         string
		senderType string
		senderID   string
		content    string
		createdAt  string
	}
	var rows_ []msgRow
	for rows.Next() {
		var r msgRow
		var t time.Time
		if err := rows.Scan(&r.id, &r.senderType, &r.senderID, &r.content, &t); err != nil {
			return nil, err
		}
		r.createdAt = t.Format(time.RFC3339)
		rows_ = append(rows_, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(rows_)-1; i < j; i, j = i+1, j-1 {
		rows_[i], rows_[j] = rows_[j], rows_[i]
	}

	msgs := make([]agent.Message, 0, len(rows_))
	for i, row := range rows_ {
		senderName := s.resolveSenderName(ctx, row.senderType, row.senderID)
		shortID := row.id
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		role := agent.RoleUser
		if row.senderType == "agent" {
			role = agent.RoleAssistant
		}

		// Slock-aligned: [target=#channel msg=shortid time=isotime type=human|agent] @sender: content
		content := fmt.Sprintf("New message received:\n\n[target=%s msg=%s time=%s type=%s] @%s: %s",
			msgTarget, shortID, row.createdAt, row.senderType, senderName, row.content)

		// On the LAST (most recent) message, append routing instruction.
		if i == len(rows_)-1 {
			content += "\n\nRespond as appropriate. Complete all your work before stopping.\nReply in the channel or create/reply in a thread as appropriate; use each message's `target` and `msg` fields to choose the exact target."
		}

		msgs = append(msgs, agent.Message{
			Role:     role,
			Content:  content,
			SenderID: row.senderID,
		})
	}

	if msgs == nil {
		msgs = []agent.Message{}
	}

	return msgs, nil
}

// resolveSenderName resolves a sender ID to a human-readable name for
// message formatting. Returns the ID as fallback.
func (s *AgentService) resolveSenderName(ctx context.Context, senderType, senderID string) string {
	if senderType == "system" {
		return "Solo"
	}
	var name string
	var err error
	if senderType == "agent" {
		err = s.pool.QueryRow(ctx, `SELECT COALESCE(name, $1) FROM agents WHERE id = $2`, senderID, senderID).Scan(&name)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT COALESCE(display_name, email, $1) FROM users WHERE id = $2`, senderID, senderID).Scan(&name)
	}
	if err != nil || name == "" {
		return senderID
	}
	return name
}

// --- Agent status management ---

// AgentStatus represents the runtime status of an agent.
type AgentStatus string

const (
	AgentStatusOnline   AgentStatus = "online"
	AgentStatusThinking AgentStatus = "thinking"
	AgentStatusTyping   AgentStatus = "typing"
	AgentStatusOffline  AgentStatus = "offline"
	AgentStatusError    AgentStatus = "error"
)

// BroadcastAgentStatus sends an agent status update to channel subscribers.
func (s *AgentService) BroadcastAgentStatus(channelID, agentID string, status AgentStatus, detail string) {
	payload := map[string]interface{}{
		"channel_id": channelID,
		"agent_id":   agentID,
		"status":     string(status),
	}
	if detail != "" {
		payload["detail"] = detail
	}

	var eventType string
	switch status {
	case AgentStatusThinking:
		eventType = "agent.thinking"
	case AgentStatusTyping:
		eventType = "agent.typing"
	case AgentStatusError:
		eventType = "agent.error"
	default:
		eventType = "agent.status"
	}

	data, _ := json.Marshal(payload)
	envelope, _ := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": json.RawMessage(data),
	})
	s.hub.BroadcastToChannel(channelID, envelope)
}

// nullableStr returns a *string for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}


func (s *AgentService) getChannelOpenTasksSummary(ctx context.Context, channelID string) string {
	rows, err := s.pool.Query(ctx,
		`SELECT task_number, title, priority
		 FROM tasks
		 WHERE channel_id = $1 AND status = $2
		 ORDER BY created_at DESC
		 LIMIT 10`,
		channelID, TaskStatusTodo,
	)
	if err != nil {
		slog.Warn("failed to query open tasks for agent context", "channel_id", channelID, "error", err)
		return ""
	}
	defer rows.Close()

	var b strings.Builder
	hasTasks := false
	for rows.Next() {
		var number int
		var title, priority string
		if err := rows.Scan(&number, &title, &priority); err != nil {
			continue
		}
		if !hasTasks {
			b.WriteString("Available tasks in this channel:\n")
			hasTasks = true
		}
		prio := ""
		if priority != "" && priority != "none" {
			prio = fmt.Sprintf(" [%s]", priority)
		}
		fmt.Fprintf(&b, "#%d %s%s [todo]\n", number, title, prio)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("error iterating open tasks for agent context", "channel_id", channelID, "error", err)
		return ""
	}
	return b.String()
}

// daemonTaskRequest is the format for tasks sent from server to daemon.
type daemonTaskRequest struct {
	TaskID       string            `json:"task_id"`
	AgentID      string            `json:"agent_id"`
	ChannelID    string            `json:"channel_id"`
	ThreadID     string            `json:"thread_id,omitempty"`
	Messages     []agent.Message   `json:"messages"`
	SystemPrompt string            `json:"system_prompt"`
	ModelConfig  agent.ModelConfig `json:"model_config"`
	OriginTaskID string            `json:"origin_task_id,omitempty"` // SOLO-123-B: task ID for status update
	TaskContext  string            `json:"task_context,omitempty"`   // SOLO-221-B: summary of pending tasks in channel
	AgentChain   []string          `json:"agent_chain,omitempty"`    // SOLO-228-B: agent trigger chain for loop prevention
	MentionedNames []string        `json:"mentioned_names,omitempty"` // v1.3: names of @mentioned agents for context awareness
}


// containsStr returns true if the slice s contains value v.
func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// isNotParticipating detects "not for me" / "not participating" messages
// that agents output despite being told to output NOTHING. These should be
// suppressed so they don't clutter the channel/thread.


// inCascadeCooldown returns true if the channel is in cascade cooldown.
func (s *AgentService) inCascadeCooldown(channelID string) bool {
	v, ok := s.cascadeMap.Load(channelID)
	if !ok {
		return false
	}
	cc := v.(*cascadeCount)
	return time.Now().Before(cc.cooldownUntil)
}

// recordCascadeDispatch records a dispatch for cascade detection.
func (s *AgentService) recordCascadeDispatch(channelID string) {
	now := time.Now()
	v, _ := s.cascadeMap.LoadOrStore(channelID, &cascadeCount{windowStart: now})
	cc := v.(*cascadeCount)
	if now.Sub(cc.windowStart) > cascadeWindow {
		cc.count = 0
		cc.windowStart = now
	}
	cc.count++
	if cc.count >= cascadeThreshold {
		cc.cooldownUntil = now.Add(cascadeCooldown)
		slog.Warn("cascade detected — pausing agent triggers",
			"channel_id", channelID, "count", cc.count, "cooldown", cascadeCooldown)
	}
}

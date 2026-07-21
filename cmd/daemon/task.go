package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/pkg/llm"
)

// Task status constants.
const (
	taskStatusRunning   = "running"
	taskStatusThinking  = "thinking"
	taskStatusCompleted = "completed"
	taskStatusFailed    = "failed"
	taskStatusCancelled = "cancelled"
)

// taskState holds the runtime state of a task being processed by the daemon.
type taskState struct {
	TaskID      string
	AgentID     string
	ChannelID   string
	ThreadID    string
	Status      string
	Result      string
	Error       string
	ReceivedAt  time.Time
	CompletedAt time.Time
}

// sseEvent represents an SSE event to be streamed to subscribers.
type sseEvent struct {
	Event string
	Data  string
}

// sseSubscriber receives SSE events for a task.
type sseSubscriber struct {
	events chan sseEvent
	done   chan struct{}
}

type actionScope struct {
	RunID     string
	AgentID   string
	ChannelID string
	ThreadID  string
	NodeID    string
	TaskID    string
}

// taskManager manages task states and SSE subscribers in the daemon.
type taskManager struct {
	mu           sync.RWMutex
	tasks        map[string]*taskState
	subscribers  map[string][]*sseSubscriber   // taskID -> subscribers
	eventHistory map[string][]sseEvent         // taskID -> replayable SSE control events
	cancelFuncs  map[string]context.CancelFunc // taskID -> cancel func
	actionScopes map[string]actionScope        // agentID -> current provider turn
}

// newTaskManager creates a new task manager.
func newTaskManager() *taskManager {
	return &taskManager{
		tasks:        make(map[string]*taskState),
		subscribers:  make(map[string][]*sseSubscriber),
		eventHistory: make(map[string][]sseEvent),
		cancelFuncs:  make(map[string]context.CancelFunc),
		actionScopes: make(map[string]actionScope),
	}
}

func (tm *taskManager) SetActionScope(scope actionScope) bool {
	if scope.AgentID == "" || scope.RunID == "" {
		return false
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, exists := tm.actionScopes[scope.AgentID]; exists {
		return false
	}
	tm.actionScopes[scope.AgentID] = scope
	return true
}

func (tm *taskManager) GetActionScope(agentID string) (actionScope, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	scope, ok := tm.actionScopes[agentID]
	return scope, ok
}

func (tm *taskManager) ClearActionScope(agentID, runID string) {
	tm.mu.Lock()
	if scope, ok := tm.actionScopes[agentID]; ok && scope.RunID == runID {
		delete(tm.actionScopes, agentID)
	}
	tm.mu.Unlock()
}

// AddTask registers a new task.
func (tm *taskManager) AddTask(taskID string, state *taskState) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tasks[taskID] = state
}

// GetTask returns a task by ID.
func (tm *taskManager) GetTask(taskID string) (*taskState, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tasks[taskID]
	return t, ok
}

// UpdateStatus updates the status of a task.
func (tm *taskManager) UpdateStatus(taskID, status string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[taskID]; ok {
		t.Status = status
		if status == taskStatusCompleted || status == taskStatusFailed || status == taskStatusCancelled {
			t.CompletedAt = time.Now()
		}
	}
}

// ListActiveTasks returns all tasks that are not in a terminal state.
func (tm *taskManager) ListActiveTasks() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var active []string
	for id, t := range tm.tasks {
		if t.Status == taskStatusRunning || t.Status == taskStatusThinking {
			active = append(active, id)
		}
	}
	return active
}

// ActiveTaskCount returns the number of currently running tasks.
func (tm *taskManager) ActiveTaskCount() int {
	return len(tm.ListActiveTasks())
}

// ActiveAgentIDs returns unique agent IDs for all running/thinking tasks.
func (tm *taskManager) ActiveAgentIDs() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	seen := make(map[string]bool)
	var ids []string
	for _, t := range tm.tasks {
		if (t.Status == taskStatusRunning || t.Status == taskStatusThinking) && !seen[t.AgentID] {
			seen[t.AgentID] = true
			ids = append(ids, t.AgentID)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids
}

// --- SSE subscriber management ---

// SubscribeSSE adds a subscriber for a task's SSE events.
// The subscriber's events channel will receive events until unsubscribed or the task completes.
func (tm *taskManager) SubscribeSSE(taskID string) *sseSubscriber {
	sub := &sseSubscriber{
		events: make(chan sseEvent, 64),
		done:   make(chan struct{}),
	}

	tm.mu.Lock()
	for _, evt := range tm.eventHistory[taskID] {
		sub.events <- evt
	}
	tm.subscribers[taskID] = append(tm.subscribers[taskID], sub)
	tm.mu.Unlock()

	return sub
}

// UnsubscribeSSE removes a subscriber from a task.
func (tm *taskManager) UnsubscribeSSE(taskID string, sub *sseSubscriber) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	subs := tm.subscribers[taskID]
	for i, s := range subs {
		if s == sub {
			tm.subscribers[taskID] = append(subs[:i], subs[i+1:]...)
			close(sub.done)
			break
		}
	}
}

// PushSSEEvent sends an SSE event to all subscribers of a task.
// This is non-blocking: if a subscriber's buffer is full, the event is dropped.
func (tm *taskManager) PushSSEEvent(taskID string, evt sseEvent) {
	tm.mu.Lock()
	if isReplayableSSEEvent(evt.Event) {
		tm.eventHistory[taskID] = append(tm.eventHistory[taskID], evt)
	}
	subs := tm.subscribers[taskID]

	for _, sub := range subs {
		select {
		case sub.events <- evt:
		default:
			slog.Debug("dropping SSE event for slow subscriber", "task_id", taskID, "event", evt.Event)
		}
	}
	tm.mu.Unlock()
}

// CloseAllSubscribers closes all subscribers for a task and cleans up.
func (tm *taskManager) CloseAllSubscribers(taskID string) {
	tm.mu.Lock()
	subs := tm.subscribers[taskID]
	delete(tm.subscribers, taskID)
	for _, sub := range subs {
		close(sub.events)
	}
	tm.mu.Unlock()
}

func isReplayableSSEEvent(event string) bool {
	switch event {
	case "session", "complete", "error", "done":
		return true
	default:
		return false
	}
}

// --- Cancel support ---

// SetCancelFunc stores a cancel function for a task.
func (tm *taskManager) SetCancelFunc(taskID string, cancel context.CancelFunc) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cancelFuncs[taskID] = cancel
}

// CancelTask cancels a running task using its stored cancel function.
// Returns true if the task was found and cancelled.
func (tm *taskManager) CancelTask(taskID string) bool {
	tm.mu.Lock()
	cancel, ok := tm.cancelFuncs[taskID]
	if ok {
		delete(tm.cancelFuncs, taskID)
	}
	tm.mu.Unlock()

	if !ok {
		return false
	}

	cancel()

	tm.UpdateStatus(taskID, taskStatusCancelled)
	tm.CloseAllSubscribers(taskID)

	slog.Info("task cancelled", "task_id", taskID)
	return true
}

// persistAgentMessage saves an agent's response message to the database.
// The daemon writes directly to the messages table since it has DB access.
func persistAgentMessage(ctx context.Context, pool *pgxpool.Pool, req runTaskRequest, content string, usage llm.Usage) (string, error) {
	messageID := uuid.New().String()
	now := time.Now()

	// Get agent's display name
	var agentName string
	err := pool.QueryRow(ctx,
		`SELECT name FROM agents WHERE id = $1`, req.AgentID,
	).Scan(&agentName)
	if err != nil {
		agentName = "Agent"
		slog.Warn("failed to get agent name for message", "agent_id", req.AgentID, "error", err)
	}

	// Insert the agent's response message
	_, err = pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, created_at, updated_at)
		 VALUES ($1, $2, 'agent', $3, $4, $5, $6, $6)`,
		messageID, req.ChannelID, req.AgentID, content, nullableStr(req.ThreadID), now,
	)
	if err != nil {
		return "", err
	}

	slog.Info("agent message persisted",
		"message_id", messageID,
		"channel_id", req.ChannelID,
		"agent_id", req.AgentID,
		"thread_id", req.ThreadID,
	)

	return messageID, nil
}

// nullableStr returns a *string for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const maxTaskEventHistoryBytes = 16 << 20

// Task status constants.
const (
	taskStatusQueued    = "queued"
	taskStatusRunning   = "running"
	taskStatusThinking  = "thinking"
	taskStatusCompleted = "completed"
	taskStatusFailed    = "failed"
	taskStatusCancelled = "cancelled"
)

// taskState holds the runtime state of a task being processed by the daemon.
type taskState struct {
	TaskID      string
	RunID       string
	AgentID     string
	ChannelID   string
	ThreadID    string
	NodeID      string
	Status      string
	Result      string
	Error       string
	ReceivedAt  time.Time
	CompletedAt time.Time
	AttemptID   string
	AgentToken  string
	Forwarding  bool
}

type activeRunAttempt struct {
	RunID     string `json:"run_id"`
	AttemptID string `json:"execution_attempt_id"`
}

func (tm *taskManager) ActiveAttempts() []activeRunAttempt {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	attempts := make([]activeRunAttempt, 0)
	for _, task := range tm.tasks {
		if task.AttemptID != "" {
			attempts = append(attempts, activeRunAttempt{RunID: task.RunID, AttemptID: task.AttemptID})
		}
	}
	return attempts
}

func (tm *taskManager) BeginForward(taskID, attemptID string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	task := tm.tasks[taskID]
	if task == nil || task.AttemptID != attemptID || task.Forwarding {
		return false
	}
	task.Forwarding = true
	return true
}

func (tm *taskManager) EndForward(taskID, attemptID string, delivered bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	task := tm.tasks[taskID]
	if task == nil || task.AttemptID != attemptID {
		return
	}
	task.Forwarding = false
	if delivered {
		delete(tm.tasks, taskID)
		delete(tm.eventHistory, taskID)
		delete(tm.historyBytes, taskID)
		delete(tm.nextEventSeq, taskID)
	}
}

// sseEvent represents an SSE event to be streamed to subscribers.
type sseEvent struct {
	Event string
	Data  string
	Seq   int64
}

// sseSubscriber receives SSE events for a task.
type sseSubscriber struct {
	events chan sseEvent
	done   chan struct{}
}

// taskManager manages task states and SSE subscribers in the daemon.
type taskManager struct {
	mu           sync.RWMutex
	tasks        map[string]*taskState
	subscribers  map[string][]*sseSubscriber // taskID -> subscribers
	eventHistory map[string][]sseEvent       // taskID -> replayable SSE control events
	historyBytes map[string]int
	nextEventSeq map[string]int64
	cancelFuncs  map[string]context.CancelFunc // taskID -> cancel func
	agentTurns   map[string]chan struct{}      // agentID -> one executing Run
}

// newTaskManager creates a new task manager.
func newTaskManager() *taskManager {
	return &taskManager{
		tasks:        make(map[string]*taskState),
		subscribers:  make(map[string][]*sseSubscriber),
		eventHistory: make(map[string][]sseEvent),
		historyBytes: make(map[string]int),
		nextEventSeq: make(map[string]int64),
		cancelFuncs:  make(map[string]context.CancelFunc),
		agentTurns:   make(map[string]chan struct{}),
	}
}

// acquireAgentTurn serializes every runtime path before Backend selection.
func (tm *taskManager) acquireAgentTurn(ctx context.Context, agentID string) (func(), error) {
	tm.mu.Lock()
	turn, ok := tm.agentTurns[agentID]
	if !ok {
		turn = make(chan struct{}, 1)
		turn <- struct{}{}
		tm.agentTurns[agentID] = turn
	}
	tm.mu.Unlock()

	select {
	case <-turn:
		return func() { turn <- struct{}{} }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AddTask registers a new task. It returns false when the same delivery
// attempt is already known, so duplicate wakeups cannot start the model twice.
func (tm *taskManager) AddTask(taskID string, state *taskState) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if existing := tm.tasks[taskID]; existing != nil && existing.AttemptID == state.AttemptID {
		return false
	}
	tm.tasks[taskID] = state
	return true
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
		if t.Status == taskStatusQueued || t.Status == taskStatusRunning || t.Status == taskStatusThinking {
			active = append(active, id)
		}
	}
	return active
}

// ListTaskIDs returns the daemon's durable-recovery evidence. Terminal
// tasks are included because their replayable completion events may not yet
// have reached a restarted server.
func (tm *taskManager) ListTaskIDs() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]string, 0, len(tm.tasks))
	for id := range tm.tasks {
		tasks = append(tasks, id)
	}
	return tasks
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
		if (t.Status == taskStatusQueued || t.Status == taskStatusRunning || t.Status == taskStatusThinking) && !seen[t.AgentID] {
			seen[t.AgentID] = true
			ids = append(ids, t.AgentID)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids
}

// ExecutingRunID returns the database Run currently owning an agent's runtime scope.
func (tm *taskManager) ExecutingRunID(agentID, channelID, nodeID string) string {
	runID, _ := tm.ExecutingCredential(agentID, channelID, nodeID)
	return runID
}

// ExecutingCredential resolves both identity and credential from the task
// that currently owns this runtime scope. An empty channel is allowed only
// when one active Run is unambiguous, which lets the CLI resolve a channel
// name before it knows the target channel ID.
func (tm *taskManager) ExecutingCredential(agentID, channelID, nodeID string) (string, string) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var runID, token string
	for id, t := range tm.tasks {
		if t.AgentID != agentID || (channelID != "" && t.ChannelID != channelID) || t.NodeID != nodeID ||
			(t.Status != taskStatusRunning && t.Status != taskStatusThinking) {
			continue
		}
		if runID != "" {
			return "", ""
		}
		runID = t.RunID
		if runID == "" {
			runID = id
		}
		token = t.AgentToken
	}
	return runID, token
}

// --- SSE subscriber management ---

// SubscribeSSE adds a subscriber for a task's SSE events.
// The subscriber's events channel will receive events until unsubscribed or the task completes.
func (tm *taskManager) SubscribeSSE(taskID string) *sseSubscriber {
	tm.mu.RLock()
	capacity := len(tm.eventHistory[taskID]) + 64
	tm.mu.RUnlock()
	sub := &sseSubscriber{
		events: make(chan sseEvent, capacity),
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
	if evt.Seq == 0 {
		tm.nextEventSeq[taskID]++
		evt.Seq = tm.nextEventSeq[taskID]
	}
	tm.eventHistory[taskID] = append(tm.eventHistory[taskID], evt)
	tm.historyBytes[taskID] += len(evt.Event) + len(evt.Data) + 16
	for tm.historyBytes[taskID] > maxTaskEventHistoryBytes && len(tm.eventHistory[taskID]) > 1 {
		dropped := tm.eventHistory[taskID][0]
		tm.eventHistory[taskID] = tm.eventHistory[taskID][1:]
		tm.historyBytes[taskID] -= len(dropped.Event) + len(dropped.Data) + 16
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

// EventsAfter returns a stable copy for the authenticated remote forwarder.
// Polling the bounded history avoids losing terminal events when the Server is
// temporarily unavailable and the legacy live subscriber buffer fills.
func (tm *taskManager) EventsAfter(taskID string, sourceSeq int64) []sseEvent {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	events := tm.eventHistory[taskID]
	result := make([]sseEvent, 0, len(events))
	for _, event := range events {
		if event.Seq > sourceSeq {
			result = append(result, event)
		}
	}
	return result
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

// --- Cancel support ---

// SetCancelFunc stores a cancel function for a task.
func (tm *taskManager) SetCancelFunc(taskID string, cancel context.CancelFunc) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cancelFuncs[taskID] = cancel
}

// ClearCancelFunc removes task-scoped cancellation after processing ends. A
// completed task must never retain authority over a reused persistent process.
func (tm *taskManager) ClearCancelFunc(taskID string) {
	tm.mu.Lock()
	delete(tm.cancelFuncs, taskID)
	tm.mu.Unlock()
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

	slog.Info("task cancelled", "task_id", taskID)
	return true
}

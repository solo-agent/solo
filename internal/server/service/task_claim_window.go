package service

import (
	"log/slog"
	"sync"
	"time"
)

// claimWindowDuration is how long @mentioned agents have exclusive claim access.
const claimWindowDuration = 30 * time.Second

// claimWindowEntry holds the priority claim window state for a task.
type claimWindowEntry struct {
	mentionedAgentIDs map[string]bool
	expiresAt         time.Time
}

// TaskClaimWindowManager manages the @mention priority claim windows for tasks.
// It is an in-memory store — windows are lost on server restart, which is
// acceptable for the 30-second window.
type TaskClaimWindowManager struct {
	mu      sync.RWMutex
	windows map[string]*claimWindowEntry // taskID -> entry
}

// NewTaskClaimWindowManager creates a new TaskClaimWindowManager.
func NewTaskClaimWindowManager() *TaskClaimWindowManager {
	return &TaskClaimWindowManager{
		windows: make(map[string]*claimWindowEntry),
	}
}

// OpenWindow creates a 30-second priority claim window for the given task.
// mentionedAgentIDs are the agent IDs that have exclusive claim rights.
// Returns true if a window was opened, false if one already exists.
func (m *TaskClaimWindowManager) OpenWindow(taskID string, mentionedAgentIDs []string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.windows[taskID]; exists {
		return false
	}

	set := make(map[string]bool, len(mentionedAgentIDs))
	for _, id := range mentionedAgentIDs {
		set[id] = true
	}

	m.windows[taskID] = &claimWindowEntry{
		mentionedAgentIDs: set,
		expiresAt:         time.Now().Add(claimWindowDuration),
	}

	slog.Info("task claim priority window opened",
		"task_id", taskID,
		"mentioned_agent_ids", mentionedAgentIDs,
		"expires_at", m.windows[taskID].expiresAt.Format(time.RFC3339),
	)
	return true
}

// CheckClaimAllowed checks whether the given claimerID is allowed to claim
// the task at this time. Returns (allowed bool, reason string).
// allowed=false means the claim should be rejected; reason explains why.
func (m *TaskClaimWindowManager) CheckClaimAllowed(taskID, claimerID string) (bool, string) {
	m.mu.RLock()
	entry, exists := m.windows[taskID]
	m.mu.RUnlock()

	if !exists {
		// No priority window — anyone can claim (standard behavior)
		return true, ""
	}

	if time.Now().After(entry.expiresAt) {
		// Window expired — remove it and allow anyone
		m.mu.Lock()
		delete(m.windows, taskID)
		m.mu.Unlock()
		slog.Debug("task claim priority window expired", "task_id", taskID)
		return true, ""
	}

	if entry.mentionedAgentIDs[claimerID] {
		return true, ""
	}

	return false, "task is reserved for @mentioned agents during the priority window"
}

// CloseWindow explicitly removes the claim window for a task.
// Called when a task is claimed during the priority window.
func (m *TaskClaimWindowManager) CloseWindow(taskID string) {
	m.mu.Lock()
	delete(m.windows, taskID)
	m.mu.Unlock()
}

// HasWindow returns true if a priority claim window is active for the task.
func (m *TaskClaimWindowManager) HasWindow(taskID string) bool {
	m.mu.RLock()
	entry, exists := m.windows[taskID]
	m.mu.RUnlock()
	if !exists {
		return false
	}
	return time.Now().Before(entry.expiresAt)
}

// GetMentionedAgentIDs returns the mentioned agent IDs if a window is active.
func (m *TaskClaimWindowManager) GetMentionedAgentIDs(taskID string) []string {
	m.mu.RLock()
	entry, exists := m.windows[taskID]
	m.mu.RUnlock()
	if !exists {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	ids := make([]string, 0, len(entry.mentionedAgentIDs))
	for id := range entry.mentionedAgentIDs {
		ids = append(ids, id)
	}
	return ids
}

// ScheduleExpiry starts a goroutine that fires after the claim window expires.
// When the window expires, the callback is invoked with the taskID.
// If the window is closed before expiry, the goroutine is a no-op.
func (m *TaskClaimWindowManager) ScheduleExpiry(taskID string, onExpiry func(taskID string)) {
	go func() {
		time.Sleep(claimWindowDuration)

		m.mu.RLock()
		_, exists := m.windows[taskID]
		m.mu.RUnlock()

		if !exists {
			// Window was already closed (task was claimed within window)
			return
		}

		m.mu.Lock()
		delete(m.windows, taskID)
		m.mu.Unlock()

		slog.Info("task claim priority window expired, opening to all agents",
			"task_id", taskID,
		)

		onExpiry(taskID)
	}()
}

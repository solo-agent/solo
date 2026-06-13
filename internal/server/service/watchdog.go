package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// TaskWatchdog represents a timeout monitor for a claimed task.
type TaskWatchdog struct {
	TaskID        string    `json:"task_id"`
	ClaimerID     string    `json:"claimer_id"`
	ClaimedAt     time.Time `json:"claimed_at"`
	Deadline      time.Time `json:"deadline"`
	LastActivity  time.Time `json:"last_activity"`
	TimeoutAction string    `json:"timeout_action"`
	EscalateTo    *string   `json:"escalate_to,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// WatchdogService monitors claimed tasks for inactivity and escalates.
type WatchdogService struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

// NewWatchdogService creates a new WatchdogService.
func NewWatchdogService(pool *pgxpool.Pool, hub realtime.Broadcaster) *WatchdogService {
	return &WatchdogService{pool: pool, hub: hub}
}

// CreateWatchdog creates a watchdog entry for a claimed task. Called after
// a task is claimed.
func (s *WatchdogService) CreateWatchdog(ctx context.Context, taskID, claimerID string, deadline time.Time, timeoutAction, escalateTo string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO task_watchdog (task_id, claimer_id, claimed_at, deadline, last_activity, timeout_action, escalate_to, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (task_id) DO UPDATE SET
		   claimer_id = EXCLUDED.claimer_id,
		   claimed_at = EXCLUDED.claimed_at,
		   deadline = EXCLUDED.deadline,
		   timeout_action = EXCLUDED.timeout_action,
		   escalate_to = EXCLUDED.escalate_to`,
		taskID, claimerID, now, deadline, now, timeoutAction, nullableStr(escalateTo), now,
	)
	if err != nil {
		return fmt.Errorf("create watchdog: %w", err)
	}

	slog.Info("watchdog created", "task_id", taskID, "claimer_id", claimerID, "deadline", deadline)
	return nil
}

// TouchActivity updates the last_activity timestamp for a task.
func (s *WatchdogService) TouchActivity(ctx context.Context, taskID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE task_watchdog SET last_activity = now() WHERE task_id = $1`, taskID)
	return err
}

// RemoveWatchdog deletes the watchdog for a completed or unclaimed task.
func (s *WatchdogService) RemoveWatchdog(ctx context.Context, taskID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM task_watchdog WHERE task_id = $1`, taskID)
	if err != nil {
		return fmt.Errorf("remove watchdog: %w", err)
	}
	return nil
}

// CheckOverdueTasks queries tasks whose deadline has passed and that are still
// in an active (non-terminal) state.
func (s *WatchdogService) CheckOverdueTasks(ctx context.Context) ([]TaskWatchdog, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT tw.task_id, tw.claimer_id, tw.claimed_at, tw.deadline, tw.last_activity,
		        tw.timeout_action, tw.escalate_to::text, tw.created_at
		 FROM task_watchdog tw
		 JOIN tasks t ON tw.task_id = t.id
		 WHERE tw.deadline < now() AND t.status NOT IN ('done', 'closed')`,
	)
	if err != nil {
		return nil, fmt.Errorf("check overdue tasks: %w", err)
	}
	defer rows.Close()

	var watchdogs []TaskWatchdog
	for rows.Next() {
		var w TaskWatchdog
		var escalateTo *string
		if err := rows.Scan(&w.TaskID, &w.ClaimerID, &w.ClaimedAt, &w.Deadline,
			&w.LastActivity, &w.TimeoutAction, &escalateTo, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan overdue task: %w", err)
		}
		w.EscalateTo = escalateTo
		watchdogs = append(watchdogs, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return watchdogs, nil
}

// HandleOverdueTask processes a single overdue task based on its timeout_action.
func (s *WatchdogService) HandleOverdueTask(ctx context.Context, w TaskWatchdog) {
	// T6.4.3: Look up the task's channel for proper broadcasting.
	taskChannelID := s.taskChannel(ctx, w.TaskID)

	switch w.TimeoutAction {
	case "remind":
		slog.Info("watchdog: reminding claimer",
			"task_id", w.TaskID,
			"claimer_id", w.ClaimerID,
		)
		if s.hub != nil {
			s.hub.SendToUser(w.ClaimerID, jsonEnvelope("reminder_fired", map[string]interface{}{
				"reminder_id": "",
				"agent_id":    w.ClaimerID,
				"task_id":     w.TaskID,
				"message":     fmt.Sprintf("Task %s is overdue (deadline was %s)", w.TaskID, w.Deadline.Format(time.RFC3339)),
			}))
			if taskChannelID != "" {
				s.hub.BroadcastToChannel(taskChannelID, jsonEnvelope("reminder_fired", map[string]interface{}{
					"task_id":    w.TaskID,
					"channel_id": taskChannelID,
					"message":    fmt.Sprintf("Task %s is overdue", w.TaskID),
				}))
			}
		}

		// Update deadline to re-check later (push back 24h to avoid spam).
		newDeadline := time.Now().Add(24 * time.Hour)
		s.pool.Exec(ctx, `UPDATE task_watchdog SET deadline = $1 WHERE task_id = $2`, newDeadline, w.TaskID)

	case "escalate":
		slog.Info("watchdog: escalating",
			"task_id", w.TaskID,
			"claimer_id", w.ClaimerID,
			"escalate_to", w.EscalateTo,
		)
		if w.EscalateTo != nil && *w.EscalateTo != "" && s.hub != nil {
			s.hub.SendToUser(*w.EscalateTo, jsonEnvelope("task_escalated", map[string]interface{}{
				"task_id":     w.TaskID,
				"claimer_id":  w.ClaimerID,
				"escalate_to": *w.EscalateTo,
			}))
		}
		// Notify the claimer too.
		if s.hub != nil {
			s.hub.SendToUser(w.ClaimerID, jsonEnvelope("task_escalated", map[string]interface{}{
				"task_id":    w.TaskID,
				"claimer_id": w.ClaimerID,
				"message":    "Your task has been escalated due to timeout.",
			}))
			if taskChannelID != "" {
				s.hub.BroadcastToChannel(taskChannelID, jsonEnvelope("task_escalated", map[string]interface{}{
					"task_id":     w.TaskID,
					"channel_id":  taskChannelID,
					"claimer_id":  w.ClaimerID,
					"escalate_to": w.EscalateTo,
				}))
			}
		}
		// Escalate to the next level.
		_, err := s.pool.Exec(ctx,
			`UPDATE task_watchdog SET timeout_action = 'unclaim', deadline = $1 WHERE task_id = $2`,
			time.Now().Add(12*time.Hour), w.TaskID)
		if err != nil {
			slog.Warn("watchdog: failed to escalate", "task_id", w.TaskID, "error", err)
		}

	case "unclaim":
		slog.Info("watchdog: auto-unclaiming",
			"task_id", w.TaskID,
			"claimer_id", w.ClaimerID,
		)
		_, err := s.pool.Exec(ctx,
			`UPDATE tasks SET claimer_id = NULL, status = 'todo', updated_at = now() WHERE id = $1`,
			w.TaskID,
		)
		if err != nil {
			slog.Warn("watchdog: failed to unclaim task", "task_id", w.TaskID, "error", err)
		}
		// Remove watchdog record.
		s.pool.Exec(ctx, `DELETE FROM task_watchdog WHERE task_id = $1`, w.TaskID)

		if s.hub != nil {
			// T6.4.3: Broadcast to the correct channel instead of empty string.
			s.hub.BroadcastToChannel(taskChannelID, jsonEnvelope("task_unclaimed_auto", map[string]interface{}{
				"task_id":    w.TaskID,
				"channel_id": taskChannelID,
				"reason":     "timeout",
			}))
		}
	}
}

// taskChannel looks up the channel ID for a task.
func (s *WatchdogService) taskChannel(ctx context.Context, taskID string) string {
	var channelID string
	_ = s.pool.QueryRow(ctx, `SELECT channel_id FROM tasks WHERE id = $1`, taskID).Scan(&channelID)
	return channelID
}

// VerifyEscalationChain checks whether an escalates_to relationship exists
// between the claimer and the escalate target (T6.4.5).
func (s *WatchdogService) VerifyEscalationChain(ctx context.Context, claimerID, escalateToID string) error {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM agent_relationships
			WHERE from_agent_id = $1 AND to_agent_id = $2 AND rel_type = 'escalates_to'
		)`, claimerID, escalateToID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify escalation chain: %w", err)
	}
	if !exists {
		return fmt.Errorf("no escalates_to relationship from %s to %s", claimerID, escalateToID)
	}
	return nil
}

// ListStaleTasks returns tasks that are past their claim deadline or
// have no watchdog but are claimed (T6.2.3).
func (s *WatchdogService) ListStaleTasks(ctx context.Context, channelID string) ([]TaskWatchdog, error) {
	query := `SELECT tw.task_id, tw.claimer_id, tw.claimed_at, tw.deadline, tw.last_activity,
		          tw.timeout_action, tw.escalate_to::text, tw.created_at
		   FROM task_watchdog tw
		   JOIN tasks t ON tw.task_id = t.id
		   WHERE t.status NOT IN ('done', 'closed')`
	args := []any{}
	argIdx := 1

	if channelID != "" {
		query += fmt.Sprintf(" AND t.channel_id = $%d", argIdx)
		args = append(args, channelID)
		argIdx++
	}

	query += " ORDER BY tw.deadline ASC LIMIT 50"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stale tasks: %w", err)
	}
	defer rows.Close()

	var watchdogs []TaskWatchdog
	for rows.Next() {
		var w TaskWatchdog
		var escalateTo *string
		if err := rows.Scan(&w.TaskID, &w.ClaimerID, &w.ClaimedAt, &w.Deadline,
			&w.LastActivity, &w.TimeoutAction, &escalateTo, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan stale task: %w", err)
		}
		w.EscalateTo = escalateTo
		watchdogs = append(watchdogs, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if watchdogs == nil {
		watchdogs = []TaskWatchdog{}
	}
	return watchdogs, nil
}

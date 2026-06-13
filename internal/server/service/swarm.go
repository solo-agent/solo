package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// SwarmCoordinator handles multi-agent task decomposition and coordination.
type SwarmCoordinator struct {
	pool     *pgxpool.Pool
	taskSvc  *TaskService
	hub      realtime.Broadcaster
}

// NewSwarmCoordinator creates a new SwarmCoordinator.
func NewSwarmCoordinator(pool *pgxpool.Pool, taskSvc *TaskService, hub realtime.Broadcaster) *SwarmCoordinator {
	return &SwarmCoordinator{pool: pool, taskSvc: taskSvc, hub: hub}
}

// SwarmSubtaskDef represents a subtask produced by the decomposition step.
type SwarmSubtaskDef struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DependsOn   []int  `json:"depends_on_indices"`
}

// SwarmPlan is stored in tasks.swarm_plan as JSONB.
type SwarmPlan struct {
	Breakdown []SwarmSubtaskDef `json:"breakdown"`
	Strategy  string            `json:"strategy"` // "parallel" or "sequential"
}

// DecomposeTask uses the provided subtask definitions to split a parent task
// into a DAG of subtasks. The subtasks parameter comes from the request body
// (the frontend or CLI provides the breakdown).
func (s *SwarmCoordinator) DecomposeTask(ctx context.Context, parentTaskID, channelID, userID string, subtasks []SwarmSubtaskDef) ([]Task, error) {
	parent, err := s.taskSvc.GetTask(ctx, channelID, parentTaskID, userID)
	if err != nil {
		return nil, fmt.Errorf("get parent task: %w", err)
	}

	var created []Task
	for i, st := range subtasks {
		req := TaskCreateRequest{
			Title:        st.Title,
			Description:  st.Description,
			ParentTaskID: parentTaskID,
		}
		subtask, err := s.taskSvc.CreateTask(ctx, channelID, userID, req)
		if err != nil {
			slog.Warn("swarm: failed to create subtask", "index", i, "error", err)
			continue
		}

		// Add dependencies from DependsOn indices.
		for _, depIdx := range st.DependsOn {
			if depIdx >= 0 && depIdx < len(created) {
				_, depErr := s.taskSvc.AddDependency(ctx, created[depIdx].ID, subtask.ID)
				if depErr != nil {
					slog.Warn("swarm: failed to add dependency",
						"blocker", created[depIdx].ID,
						"blocked", subtask.ID,
						"error", depErr,
					)
				}
			}
		}
		created = append(created, *subtask)
	}

	// Mark parent task as swarm.
	plan := SwarmPlan{
		Breakdown: subtasks,
		Strategy:  "parallel",
	}
	planJSON, _ := json.Marshal(plan)
	_, err = s.pool.Exec(ctx,
		`UPDATE tasks SET is_swarm = true, swarm_plan = $1, status = 'in_progress', updated_at = now() WHERE id = $2`,
		planJSON, parent.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("mark swarm task: %w", err)
	}

	// Update parent in-memory after the status change.
	parent.Status = TaskStatusInProgress

	slog.Info("swarm: task decomposed",
		"parent_task_id", parentTaskID,
		"subtask_count", len(created),
	)

	// T6.4.3: Broadcast swarm_decomposed event.
	if s.hub != nil {
		s.hub.BroadcastToChannel(channelID, jsonEnvelope("swarm_decomposed", map[string]interface{}{
			"parent_task_id": parentTaskID,
			"channel_id":     channelID,
			"subtask_count":  len(created),
		}))
	}

	return created, nil
}

// ListClaimableSwarmTasks returns subtasks of swarm parents that have no
// blockers and are not yet claimed.
func (s *SwarmCoordinator) ListClaimableSwarmTasks(ctx context.Context, channelID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.task_number, t.channel_id, t.creator_id, t.title,
		        COALESCE(t.description, ''), t.status, COALESCE(t.claimer_id::text, ''),
		        t.priority, t.due_date, COALESCE(t.message_id::text, ''),
		        t.parent_task_id, t.created_at, t.updated_at
		 FROM tasks t
		 JOIN tasks parent ON t.parent_task_id = parent.id
		 WHERE parent.is_swarm = true
		   AND t.channel_id = $1
		   AND t.status = 'todo'
		   AND t.claimer_id IS NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM task_dependencies td
		     JOIN tasks blocker ON td.blocker_task_id = blocker.id
		     WHERE td.blocked_task_id = t.id AND blocker.status NOT IN ('done', 'closed')
		   )
		 ORDER BY t.created_at ASC`,
		channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("list claimable swarm tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var parentTaskID *string
		err := rows.Scan(&t.ID, &t.TaskNumber, &t.ChannelID, &t.CreatorID, &t.Title,
			&t.Description, &t.Status, &t.ClaimerID,
			&t.Priority, &t.DueDate, &t.MessageID,
			&parentTaskID, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan claimable task: %w", err)
		}
		t.ParentTaskID = parentTaskID
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []Task{}
	}
	return tasks, nil
}

// CheckSwarmProgress evaluates whether all subtasks of a swarm parent are
// done and marks the parent as done if so.
func (s *SwarmCoordinator) CheckSwarmProgress(ctx context.Context, parentTaskID, channelID string) error {
	var total, done int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END)
		 FROM tasks WHERE parent_task_id = $1`,
		parentTaskID,
	).Scan(&total, &done)
	if err != nil {
		return fmt.Errorf("check swarm progress: %w", err)
	}

	if total > 0 && total == done {
		_, err = s.pool.Exec(ctx,
			`UPDATE tasks SET status = 'done', updated_at = now() WHERE id = $1`,
			parentTaskID,
		)
		if err != nil {
			return fmt.Errorf("mark swarm parent done: %w", err)
		}

		if s.hub != nil {
			s.hub.BroadcastToChannel(channelID, jsonEnvelope("swarm_all_done", map[string]interface{}{
				"parent_task_id": parentTaskID,
				"channel_id":     channelID,
			}))
		}

		slog.Info("swarm: all subtasks complete",
			"parent_task_id", parentTaskID,
			"subtask_count", total,
		)
	}

	return nil
}

// GetSwarmStatus returns the execution progress of a swarm task.
func (s *SwarmCoordinator) GetSwarmStatus(ctx context.Context, parentTaskID, channelID string) (map[string]interface{}, error) {
	var total, done, inProgress int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*),
		        SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status = 'in_progress' THEN 1 ELSE 0 END)
		 FROM tasks WHERE parent_task_id = $1`,
		parentTaskID,
	).Scan(&total, &done, &inProgress)
	if err != nil {
		return nil, fmt.Errorf("get swarm status: %w", err)
	}

	progress := 0.0
	if total > 0 {
		progress = float64(done) / float64(total) * 100
	}

	return map[string]interface{}{
		"total":       total,
		"done":        done,
		"in_progress": inProgress,
		"progress":    progress,
	}, nil
}

// AfterTaskCompleted is called when a task transitions to 'done'. It handles
// swarm parent progress checking.
func (s *SwarmCoordinator) AfterTaskCompleted(ctx context.Context, task *Task) {
	if task.ParentTaskID != nil && *task.ParentTaskID != "" {
		// Check if parent is a swarm task.
		var isSwarm bool
		err := s.pool.QueryRow(ctx,
			`SELECT is_swarm FROM tasks WHERE id = $1`, *task.ParentTaskID,
		).Scan(&isSwarm)
		if err == nil && isSwarm {
			if checkErr := s.CheckSwarmProgress(ctx, *task.ParentTaskID, task.ChannelID); checkErr != nil {
				slog.Warn("swarm: failed to check parent progress",
					"parent_task_id", *task.ParentTaskID,
					"error", checkErr,
				)
			}
		}
	}
}


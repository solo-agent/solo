package service

import (
	"context"
	"fmt"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Task status constants.
const (
	TaskStatusTodo       = "todo"
	TaskStatusInProgress = "in_progress"
	TaskStatusInReview   = "in_review"
	TaskStatusDone       = "done"
	TaskStatusClosed     = "closed"
)

// TerminalStatuses contains statuses that represent end states.
var TerminalStatuses = map[string]bool{
	TaskStatusDone:   true,
	TaskStatusClosed: true,
}

// ValidTaskStatuses contains all valid task status values.
var ValidTaskStatuses = []string{
	TaskStatusTodo,
	TaskStatusInProgress,
	TaskStatusInReview,
	TaskStatusDone,
	TaskStatusClosed,
}

// allowedTransitions maps a current status to the set of allowed next statuses.
var allowedTransitions = map[string]map[string]bool{
	TaskStatusTodo: {
		TaskStatusInProgress: true,
		TaskStatusClosed:     true,
	},
	TaskStatusInProgress: {
		TaskStatusInReview: true,
		TaskStatusClosed:   true,
	},
	TaskStatusInReview: {
		TaskStatusDone:       true,
		TaskStatusInProgress: true,
		TaskStatusClosed:     true,
	},
	// done is a terminal state per PRD v1.3 §3.2 Q5.
	// To follow up on a completed task, create a new subtask instead.
	TaskStatusDone: {
		TaskStatusClosed: true,
	},
	TaskStatusClosed: {
		TaskStatusTodo: true,
	},
}

var (
	ErrTaskNotFound          = errors.New("task not found")
	ErrTaskInvalidStatus     = errors.New("invalid task status")
	ErrTaskInvalidTransition = errors.New("invalid task status transition")
	ErrTaskNotChannelMember  = errors.New("user is not a channel member")
	ErrTaskAlreadyClaimed    = errors.New("task is already claimed by another agent")
	ErrTaskInTerminalState   = errors.New("task is in a terminal state and cannot be claimed")
	ErrTaskNotClaimable      = errors.New("task status does not allow claiming")
	ErrTaskNotClaimer        = errors.New("you are not the claimer of this task")
)

// Task represents a task in a channel.
type Task struct {
	ID             string     `json:"id"`
	TaskNumber     int        `json:"task_number"`
	ChannelID      string     `json:"channel_id"`
	CreatorID      string     `json:"creator_id"`
	CreatorName    string     `json:"creator_name,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Status         string     `json:"status"`
	ClaimerID      string     `json:"claimer_id,omitempty"`
	ClaimerName    string     `json:"claimer_name,omitempty"`
	Priority       string     `json:"priority"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	MessageID      string     `json:"message_id,omitempty"`
	ParentTaskID   *string    `json:"parent_task_id,omitempty"`
	SubtaskCount   int        `json:"subtask_count,omitempty"`
	DoneSubtaskCount int      `json:"done_subtask_count,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TaskCreateRequest contains the fields needed to create a task.
type TaskCreateRequest struct {
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Priority      string     `json:"priority,omitempty"`
	DueDate       *time.Time `json:"due_date,omitempty"`
	MessageID     string     `json:"message_id,omitempty"`
	ParentTaskID  string     `json:"parent_task_id,omitempty"`
}

// TaskUpdateRequest contains the fields that can be updated on a task.
type TaskUpdateRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Status      *string    `json:"status,omitempty"`
	Priority    *string    `json:"priority,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

// TaskFilter contains optional filters for listing tasks.
type TaskFilter struct {
	Status       string
	ClaimerID    string
	ParentTaskID string
}

// TaskService handles task business logic.
type TaskService struct {
	pool *pgxpool.Pool
}

// NewTaskService creates a new TaskService.
func NewTaskService(pool *pgxpool.Pool) *TaskService {
	return &TaskService{pool: pool}
}

// CreateTask creates a new task in the channel with per-channel task numbering.
func (s *TaskService) CreateTask(ctx context.Context, channelID, creatorID string, req TaskCreateRequest) (*Task, error) {
	// Verify user is a channel member (skip if no channel specified)
	if channelID != "" {
		if err := s.requireChannelMember(ctx, channelID, creatorID); err != nil {
			return nil, err
		}
	}

	// Validate title
	if req.Title == "" {
		return nil, errors.New("task title is required")
	}

	if req.Priority == "" {
		req.Priority = "none"
	}

	// Validate parent_task_id if provided: must be a valid UUID pointing to an
	// existing task in the same channel.
	var parentTaskID interface{}
	if req.ParentTaskID != "" {
		if _, err := uuid.Parse(req.ParentTaskID); err != nil {
			return nil, fmt.Errorf("invalid parent_task_id: %w", err)
		}
		var parentChannelID string
		err := s.pool.QueryRow(ctx,
			`SELECT channel_id FROM tasks WHERE id = $1`, req.ParentTaskID,
		).Scan(&parentChannelID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("parent task not found")
			}
			return nil, fmt.Errorf("lookup parent task: %w", err)
		}
		if parentChannelID != channelID {
			return nil, fmt.Errorf("parent task is not in the same channel")
		}
		parentUUID, _ := uuid.Parse(req.ParentTaskID)
		parentTaskID = parentUUID
	} else {
		parentTaskID = nil
	}

	id := uuid.New().String()
	now := time.Now()

	// Compute per-channel task number without the SERIAL default.
	nextNumber, err := s.nextTaskNumber(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("compute next task number: %w", err)
	}

	// NULL-safe channel_id, message_id
	chanID := interface{}(nil)
	if channelID != "" {
		chanID = channelID
	}
	msgID := interface{}(nil)
	if req.MessageID != "" {
		msgID = req.MessageID
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO tasks (id, task_number, channel_id, creator_id, title, description, status, priority, due_date, message_id, parent_task_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		id, nextNumber, chanID, creatorID, req.Title, nullableStr(req.Description),
		TaskStatusTodo, req.Priority, req.DueDate, msgID, parentTaskID, now, now,
	)
	if err != nil {
		// If unique constraint on (channel_id, task_number) is violated, retry once.
		if isPgUniqueViolation(err) {
			nextNumber2, err2 := s.nextTaskNumber(ctx, channelID)
			if err2 != nil {
				return nil, fmt.Errorf("retry next task number: %w", err2)
			}
			_, err = s.pool.Exec(ctx,
				`INSERT INTO tasks (id, task_number, channel_id, creator_id, title, description, status, priority, due_date, message_id, parent_task_id, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
				id, nextNumber2, chanID, creatorID, req.Title, nullableStr(req.Description),
				TaskStatusTodo, req.Priority, req.DueDate, msgID, parentTaskID, now, now,
			)
			if err != nil {
				return nil, err
			}
			nextNumber = nextNumber2
		} else {
			return nil, err
		}
	}

	var pti *string
	if req.ParentTaskID != "" {
		pti = &req.ParentTaskID
	}
	task := &Task{
		ID:           id,
		TaskNumber:   nextNumber,
		ChannelID:    channelID,
		CreatorID:    creatorID,
		Title:        req.Title,
		Description:  req.Description,
		Status:       TaskStatusTodo,
		Priority:     req.Priority,
		DueDate:      req.DueDate,
		MessageID:    req.MessageID,
		ParentTaskID: pti,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Resolve creator name for WS broadcast
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(
			(SELECT display_name FROM users WHERE id = $1),
			(SELECT name FROM agents WHERE id = $1),
			''
		)`, creatorID,
	).Scan(&task.CreatorName)

	slog.Info("task created",
		"task_id", id,
		"task_number", task.TaskNumber,
		"channel_id", channelID,
		"creator_id", creatorID,
		"title", req.Title,
	)

	return task, nil
}

// nextTaskNumber computes the next per-channel task number.
func (s *TaskService) nextTaskNumber(ctx context.Context, channelID string) (int, error) {
	var num int
	if channelID != "" {
		err := s.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = $1`,
			channelID,
		).Scan(&num)
		return num, err
	}
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id IS NULL`,
	).Scan(&num)
	return num, err
}

// ClaimTask claims a task for the current user. The task must exist, be in a
// claimable state (todo or in_progress), and either unclaimed or already claimed
// by the same caller (idempotent re-claim). On success, claimer_id is set and
// status transitions from todo to in_progress.
//
// Uses SELECT ... FOR UPDATE within a transaction to prevent concurrent claim races.
func (s *TaskService) ClaimTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	// Begin transaction for atomic check-and-claim.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the task row for update to prevent concurrent claims.
	var currentStatus, currentClaimerID string
	err = tx.QueryRow(ctx,
		`SELECT status, COALESCE(claimer_id::text, '')
		 FROM tasks
		 WHERE id = $1 AND channel_id = $2
		 FOR UPDATE`,
		taskID, channelID,
	).Scan(&currentStatus, &currentClaimerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task: %w", err)
	}

	// State validation: only todo and in_progress are claimable.
	if TerminalStatuses[currentStatus] {
		return nil, ErrTaskInTerminalState
	}
	if currentStatus == TaskStatusInReview {
		return nil, ErrTaskNotClaimable
	}
	// (TaskStatusTodo and TaskStatusInProgress fall through)

	// Claimer validation: prevent stealing from another claimer.
	if currentClaimerID != "" && currentClaimerID != userID {
		return nil, ErrTaskAlreadyClaimed
	}

	// Determine new status.
	newStatus := currentStatus
	if currentStatus == TaskStatusTodo {
		newStatus = TaskStatusInProgress
	}

	// Update the task — idempotent when the same claimer re-claims.
	now := time.Now()
	_, err = tx.Exec(ctx,
		`UPDATE tasks SET claimer_id = $1, status = $2, updated_at = $3
		 WHERE id = $4 AND channel_id = $5`,
		userID, newStatus, now, taskID, channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("update claim: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Re-fetch to get ClaimerName from the JOIN with users/agents tables.
	refetched, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}

	slog.Info("task claimed",
		"task_id", taskID,
		"task_number", refetched.TaskNumber,
		"channel_id", channelID,
		"claimer_id", userID,
		"new_status", newStatus,
	)

	return refetched, nil
}

// UnclaimTask releases a claim on a task. Only the current claimer can unclaim.
// On success, claimer_id is cleared and status reverts to todo.
func (s *TaskService) UnclaimTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	current, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}

	if current.ClaimerID == "" {
		return nil, ErrTaskNotClaimer
	}
	if current.ClaimerID != userID {
		return nil, ErrTaskNotClaimer
	}

	if TerminalStatuses[current.Status] {
		return nil, ErrTaskInTerminalState
	}

	now := time.Now()
	_, err = s.pool.Exec(ctx,
		`UPDATE tasks SET claimer_id = NULL, status = $1, updated_at = $2
		 WHERE id = $3 AND channel_id = $4`,
		TaskStatusTodo, now, taskID, channelID,
	)
	if err != nil {
		return nil, err
	}

	// Re-fetch to get accurate state after UPDATE
	refetched, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}

	slog.Info("task unclaimed",
		"task_id", taskID,
		"task_number", refetched.TaskNumber,
		"channel_id", channelID,
		"previous_claimer", userID,
	)

	return refetched, nil
}

// ConvertMessageToTask creates a task from an existing message (asTask).
// The message content becomes the task title, and the task is linked via message_id.
func (s *TaskService) ConvertMessageToTask(ctx context.Context, channelID, messageID, userID string) (*Task, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	// Get message content for the task title
	var content string
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM messages WHERE id = $1 AND channel_id = $2 AND COALESCE(is_deleted, false) = false`,
		messageID, channelID,
	).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("message not found")
		}
		return nil, err
	}

	// Use truncated message content as default title
	title := truncateForTitle(content, 500)

	req := TaskCreateRequest{
		Title:       title,
		Description: content, // Full message content as description
		MessageID:   messageID,
	}

	return s.CreateTask(ctx, channelID, userID, req)
}

// GetTask retrieves a single task by ID.
func (s *TaskService) GetTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	var task Task
	var description string
	var dueDate *time.Time

	// Reject tasks in archived channels.
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	// Try UUID first
	err := s.pool.QueryRow(ctx,
		`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
		 COALESCE(t.claimer_id::text, ''),
		 COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''),
		 t.parent_task_id,
		 (SELECT COUNT(*) FROM tasks WHERE parent_task_id = t.id) AS subtask_count,
		 (SELECT COUNT(*) FROM tasks WHERE parent_task_id = t.id AND status = 'done') AS done_subtask_count,
		 t.created_at, t.updated_at
		 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.id = $1 AND t.channel_id = $2`,
		taskID, channelID,
	).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
		&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID,
		&task.ParentTaskID, &task.SubtaskCount, &task.DoneSubtaskCount, &task.CreatedAt, &task.UpdatedAt)

	if err != nil {
		// Try by task_number
		err2 := s.pool.QueryRow(ctx,
			`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
			 COALESCE(t.claimer_id::text, ''),
		 COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at
			 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.task_number::text = $1 AND t.channel_id = $2`,
			taskID, channelID,
		).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
			&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID, &task.CreatedAt, &task.UpdatedAt)
		if err2 != nil {
			// Try by message_id (Slock-aligned: agent uses msg= header short ID)
			err3 := s.pool.QueryRow(ctx,
				`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
				 COALESCE(t.claimer_id::text, ''),
				COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at
				 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.message_id::text = $1 AND t.channel_id = $2`,
				taskID, channelID,
			).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
				&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID, &task.CreatedAt, &task.UpdatedAt)
			if err3 != nil {
				if errors.Is(err3, pgx.ErrNoRows) {
					return nil, ErrTaskNotFound
				}
				return nil, err3
			}
		}
	}

	task.Description = description
	if dueDate != nil {
		task.DueDate = dueDate
	}
	return &task, nil
}
func (s *TaskService) ListTasks(ctx context.Context, channelID, userID string, filter TaskFilter) ([]Task, error) {
	// Verify channel member
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	query := `SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
			                  COALESCE(t.claimer_id::text, ''), t.priority,
			                  t.due_date, COALESCE(t.message_id::text, ''), COALESCE(t.parent_task_id::text, ''),
			                  t.created_at, t.updated_at,
			                  COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name
		           FROM tasks t
		           LEFT JOIN users u_creator ON t.creator_id = u_creator.id
		           LEFT JOIN agents a_creator ON t.creator_id = a_creator.id
		           LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id
		           LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id
		           WHERE t.channel_id = $1`
	args := []any{channelID}
	argIdx := 2

	if filter.Status != "" {
		query += ` AND t.status = $` + strconv.Itoa(argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.ClaimerID != "" {
		query += ` AND t.claimer_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ClaimerID)
		argIdx++
	}
	if filter.ParentTaskID != "" {
		query += ` AND t.parent_task_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.ParentTaskID)
		argIdx++
	}

	query += ` ORDER BY t.created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var dueDate *time.Time
		var parentTaskID *string
		err := rows.Scan(&t.ID, &t.TaskNumber, &t.ChannelID, &t.CreatorID, &t.CreatorName, &t.Title, &t.Description,
			&t.Status, &t.ClaimerID, &t.Priority,
			&dueDate, &t.MessageID, &parentTaskID, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName)
		if err != nil {
			return nil, err
		}
		if dueDate != nil {
			t.DueDate = dueDate
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

// UpdateTask updates a task. Validates status transitions.
func (s *TaskService) UpdateTask(ctx context.Context, channelID, taskID, userID string, req TaskUpdateRequest) (*Task, error) {
	// Verify channel member
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}

	// Get current task
	currentTask, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}

	// Validate status transition if status is being changed
	if req.Status != nil && *req.Status != "" {
		if err := validateStatusTransition(currentTask.Status, *req.Status); err != nil {
			return nil, err
		}
	}

	// Build dynamic update
	newStatus := currentTask.Status
	if req.Status != nil && *req.Status != "" {
		newStatus = *req.Status
	}

	newTitle := currentTask.Title
	if req.Title != nil {
		newTitle = *req.Title
	}

	newDescription := currentTask.Description
	if req.Description != nil {
		newDescription = *req.Description
	}

	newPriority := currentTask.Priority
	if req.Priority != nil {
		newPriority = *req.Priority
	}

	var newDueDate *time.Time
	if req.DueDate != nil {
		newDueDate = req.DueDate
	} else {
		newDueDate = currentTask.DueDate
	}

	now := time.Now()

	result, err := s.pool.Exec(ctx,
		`UPDATE tasks SET
			title = $1, description = $2, status = $3,
			priority = $4, due_date = $5, updated_at = $6
		 WHERE id = $7 AND channel_id = $8`,
		newTitle, nullableStr(newDescription), newStatus,
		newPriority, newDueDate, now, currentTask.ID, channelID,
	)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, ErrTaskNotFound
	}

	updatedTask := &Task{
		ID:          taskID,
		TaskNumber:  currentTask.TaskNumber,
		ChannelID:   channelID,
		CreatorID:   currentTask.CreatorID,
		Title:       newTitle,
		Description: newDescription,
		Status:      newStatus,
		ClaimerID:   currentTask.ClaimerID,
		Priority:    newPriority,
		DueDate:     newDueDate,
		MessageID:   currentTask.MessageID,
		CreatedAt:   currentTask.CreatedAt,
		UpdatedAt:   now,
	}

	slog.Info("task updated",
		"task_id", taskID,
		"channel_id", channelID,
		"user_id", userID,
		"new_status", newStatus,
	)

	return updatedTask, nil
}

// DeleteTask deletes a task.
func (s *TaskService) DeleteTask(ctx context.Context, channelID, taskID, userID string) error {
	// Verify channel member
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return err
	}

	result, err := s.pool.Exec(ctx,
		`DELETE FROM tasks WHERE id = $1 AND channel_id = $2`,
		taskID, channelID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrTaskNotFound
	}

	slog.Info("task deleted",
		"task_id", taskID,
		"channel_id", channelID,
		"user_id", userID,
	)

	return nil
}

// GetTasksForAgent returns all tasks claimed by a specific agent that are
// in the in_progress state.
func (s *TaskService) GetTasksForAgent(ctx context.Context, agentID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.task_number, t.channel_id, t.creator_id, t.title, COALESCE(t.description, ''), t.status,
		        COALESCE(t.claimer_id::text, ''), t.priority,
		        t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at
		 FROM tasks t
		 LEFT JOIN channels c ON t.channel_id = c.id
		 WHERE t.claimer_id = $1 AND t.status = $2 AND (t.channel_id IS NULL OR c.is_archived = false)
		 ORDER BY t.created_at DESC`,
		agentID, TaskStatusInProgress,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var dueDate *time.Time
		var parentTaskID *string
		err := rows.Scan(&t.ID, &t.TaskNumber, &t.ChannelID, &t.CreatorID, &t.CreatorName, &t.Title, &t.Description,
			&t.Status, &t.ClaimerID, &t.Priority,
			&dueDate, &t.MessageID, &parentTaskID, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if dueDate != nil {
			t.DueDate = dueDate
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

// CompleteTaskForAgent marks a task claimed by an agent as in_review after
// the agent has completed its execution. Accepts both todo and in_progress
// because an agent may be assigned a task that hasn't been explicitly claimed.
func (s *TaskService) CompleteTaskForAgent(ctx context.Context, taskID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status IN ($3, $4)`,
		TaskStatusInReview, taskID, TaskStatusInProgress, TaskStatusTodo,
	)
	return err
}

// requireChannelMember checks that the user is a member of the channel and the channel is not archived.
func (s *TaskService) requireChannelMember(ctx context.Context, channelID, userID string) error {
	var archived bool
	err := s.pool.QueryRow(ctx,
		`SELECT is_archived FROM channels WHERE id = $1`,
		channelID,
	).Scan(&archived)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTaskNotChannelMember
		}
		return err
	}
	if archived {
		return ErrTaskNotChannelMember
	}

	var exists bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_id = $2
		)`, channelID, userID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrTaskNotChannelMember
	}
	return nil
}

// validateStatusTransition checks if a transition from currentStatus to
// newStatus is allowed per the status flow rules.
func validateStatusTransition(currentStatus, newStatus string) error {
	// Allow staying in the same status (no-op)
	if currentStatus == newStatus {
		return nil
	}

	// Validate new status is known
	valid := false
	for _, s := range ValidTaskStatuses {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return ErrTaskInvalidStatus
	}

	// Check transition is allowed
	allowed, ok := allowedTransitions[currentStatus]
	if !ok || !allowed[newStatus] {
		return ErrTaskInvalidTransition
	}

	return nil
}

// GetTaskGlobal retrieves a task by ID without requiring channelID in the URL.
func (s *TaskService) GetTaskGlobal(ctx context.Context, taskID, userID string) (*Task, error) {
	var channelID string
	err := s.pool.QueryRow(ctx, `SELECT channel_id FROM tasks WHERE id = $1`, taskID).Scan(&channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return s.GetTask(ctx, channelID, taskID, userID)
}

// ListAllUserTasks returns all tasks across channels the user is a member of.
func (s *TaskService) ListAllUserTasks(ctx context.Context, userID string, channelID string, status string, claimerID string) ([]Task, error) {
	query := `SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''),
		t.status, COALESCE(t.claimer_id::text, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at,
		COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name
		FROM tasks t
		LEFT JOIN users u_creator ON t.creator_id = u_creator.id
		LEFT JOIN agents a_creator ON t.creator_id = a_creator.id
		LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id
		LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id
		JOIN channel_members cm ON cm.channel_id = t.channel_id
		JOIN channels c ON t.channel_id = c.id AND c.is_archived = false
		WHERE cm.member_type = 'user' AND cm.member_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if channelID != "" {
		query += fmt.Sprintf(" AND t.channel_id = $%d", argIdx)
		args = append(args, channelID)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if claimerID != "" {
		query += fmt.Sprintf(" AND t.claimer_id = $%d", argIdx)
		args = append(args, claimerID)
		argIdx++
	}
	query += " ORDER BY t.created_at DESC LIMIT 100"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var dueDate *time.Time
		err := rows.Scan(&t.ID, &t.TaskNumber, &t.ChannelID, &t.CreatorID, &t.CreatorName, &t.Title, &t.Description,
			&t.Status, &t.ClaimerID, &t.Priority, &dueDate, &t.MessageID, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName)
		if err != nil {
			return nil, err
		}
		if dueDate != nil {
			t.DueDate = dueDate
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// isPgUniqueViolation checks if an error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// truncateForTitle truncates a string to maxLen runes for use as a task title.
func truncateForTitle(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}


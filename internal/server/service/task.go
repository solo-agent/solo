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
	ErrTaskBlocked           = errors.New("task is blocked by incomplete dependencies")
)

// Task represents a task in a channel.
type Task struct {
	ID               string     `json:"id"`
	TaskNumber       int        `json:"task_number"`
	ChannelID        string     `json:"channel_id"`
	CreatorID        string     `json:"creator_id"`
	CreatorName      string     `json:"creator_name,omitempty"`
	Title            string     `json:"title"`
	Description      string     `json:"description,omitempty"`
	Status           string     `json:"status"`
	ClaimerID        string     `json:"claimer_id,omitempty"`
	ClaimerName      string     `json:"claimer_name,omitempty"`
	ClaimerDeleted   bool       `json:"claimer_deleted"`
	Priority         string     `json:"priority"`
	DueDate          *time.Time `json:"due_date,omitempty"`
	MessageID        string     `json:"message_id,omitempty"`
	ParentTaskID     *string    `json:"parent_task_id,omitempty"`
	SubtaskCount     int        `json:"subtask_count,omitempty"`
	DoneSubtaskCount int        `json:"done_subtask_count,omitempty"`
	BlockerIDs       []string   `json:"blocker_ids,omitempty"`
	BlockedByCount   int        `json:"blocked_by_count,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TaskCreateRequest contains the fields needed to create a task.
type TaskCreateRequest struct {
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Priority      string     `json:"priority,omitempty"`
	DueDate       *time.Time `json:"due_date,omitempty"`
	MessageID     string     `json:"message_id,omitempty"`
	ParentTaskID  string     `json:"parent_task_id,omitempty"`
	DependsOn     []string   `json:"depends_on,omitempty"`
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
	CreatorID    string
	ChannelID    string
	ParentTaskID string
}

// TaskService handles task business logic.
type TaskService struct {
	pool          *pgxpool.Pool
	agentNotifier *AgentNotifier
}

// NewTaskService creates a new TaskService.
func NewTaskService(pool *pgxpool.Pool) *TaskService {
	return &TaskService{pool: pool}
}

// SetAgentNotifier injects the agent DM notification service.
func (s *TaskService) SetAgentNotifier(n *AgentNotifier) {
	s.agentNotifier = n
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

	// Create dependency relationships if DependsOn was specified.
	if len(req.DependsOn) > 0 {
		for _, blockerID := range req.DependsOn {
			_, depErr := s.AddDependency(ctx, blockerID, id)
			if depErr != nil {
				slog.Warn("failed to create dependency", "blocked", id, "blocker", blockerID, "error", depErr)
			}
		}
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

	// T1.2.4: Check if task is blocked by incomplete dependencies.
	blocked, blockErr := s.IsTaskBlocked(ctx, taskID)
	if blockErr != nil {
		return nil, fmt.Errorf("check blocked: %w", blockErr)
	}
	if blocked {
		return nil, ErrTaskBlocked
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

	// 1.2: Notify sub-task creator of the claim (only for sub-tasks with reverse assigns_to edge).
	if s.agentNotifier != nil {
		if err := s.agentNotifier.NotifyClaim(ctx, taskID, userID); err != nil {
			slog.Warn("subtask notify claim failed", "task_id", taskID, "err", err)
		}
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
		 (SELECT COUNT(*) FROM task_dependencies td
		    JOIN tasks t2 ON td.blocker_task_id = t2.id
		    WHERE td.blocked_task_id = t.id AND t2.status NOT IN ('done','closed')
		 ) AS blocked_by_count,
		 t.created_at, t.updated_at,
		 (NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted
		 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.id = $1 AND t.channel_id = $2`,
		taskID, channelID,
	).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
		&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID,
		&task.ParentTaskID, &task.SubtaskCount, &task.DoneSubtaskCount, &task.BlockedByCount, &task.CreatedAt, &task.UpdatedAt, &task.ClaimerDeleted)

	if err != nil {
		// Try by task_number
		err2 := s.pool.QueryRow(ctx,
			`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
			 COALESCE(t.claimer_id::text, ''),
		 COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at,
		 (NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted
			 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.task_number::text = $1 AND t.channel_id = $2`,
			taskID, channelID,
		).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
			&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID, &task.CreatedAt, &task.UpdatedAt, &task.ClaimerDeleted)
		if err2 != nil {
			// Try by message_id( agent uses msg= header short ID)
			err3 := s.pool.QueryRow(ctx,
				`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
				 COALESCE(t.claimer_id::text, ''),
				COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at,
				(NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted
				 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.message_id::text = $1 AND t.channel_id = $2`,
				taskID, channelID,
			).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
				&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID, &task.CreatedAt, &task.UpdatedAt, &task.ClaimerDeleted)
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

	// T1.2.4: Populate BlockerIDs array.
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(array_agg(td.blocker_task_id) FILTER (WHERE td.blocker_task_id IS NOT NULL), '{}')
		FROM task_dependencies td
		JOIN tasks t2 ON td.blocker_task_id = t2.id
		WHERE td.blocked_task_id = $1 AND t2.status NOT IN ('done', 'closed')
	`, task.ID).Scan(&task.BlockerIDs)

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
			                  COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name,
			                  (NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted
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
	if filter.CreatorID != "" {
		query += ` AND t.creator_id = $` + strconv.Itoa(argIdx)
		args = append(args, filter.CreatorID)
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
			&dueDate, &t.MessageID, &parentTaskID, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName, &t.ClaimerDeleted)
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
		ID:               taskID,
		TaskNumber:       currentTask.TaskNumber,
		ChannelID:        channelID,
		CreatorID:        currentTask.CreatorID,
		CreatorName:      currentTask.CreatorName,
		Title:            newTitle,
		Description:      newDescription,
		Status:           newStatus,
		ClaimerID:        currentTask.ClaimerID,
		ClaimerName:      currentTask.ClaimerName,
		Priority:         newPriority,
		DueDate:          newDueDate,
		MessageID:        currentTask.MessageID,
		ParentTaskID:     currentTask.ParentTaskID,
		SubtaskCount:     currentTask.SubtaskCount,
		DoneSubtaskCount: currentTask.DoneSubtaskCount,
		BlockerIDs:       currentTask.BlockerIDs,
		BlockedByCount:   currentTask.BlockedByCount,
		CreatedAt:        currentTask.CreatedAt,
		UpdatedAt:        now,
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
	if err != nil {
		return err
	}

	// 1.2: Notify sub-task creator of the completion (only for sub-tasks with reverse assigns_to edge).
	if s.agentNotifier != nil {
		// Look up the claimer_id to pass to the notifier.
		var claimerID string
		if err := s.pool.QueryRow(ctx, `SELECT COALESCE(claimer_id::text, '') FROM tasks WHERE id = $1`, taskID).Scan(&claimerID); err == nil && claimerID != "" {
			if err := s.agentNotifier.NotifyComplete(ctx, taskID, claimerID); err != nil {
				slog.Warn("subtask notify complete failed", "task_id", taskID, "err", err)
			}
		}
	}
	return nil
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
func (s *TaskService) ListAllUserTasks(ctx context.Context, userID string, channelID string, status string, claimerID string, creatorID string) ([]Task, error) {
	query := `SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''),
		t.status, COALESCE(t.claimer_id::text, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), t.created_at, t.updated_at,
		COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name,
		(NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted
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
	if creatorID != "" {
		query += fmt.Sprintf(" AND t.creator_id = $%d", argIdx)
		args = append(args, creatorID)
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
			&t.Status, &t.ClaimerID, &t.Priority, &dueDate, &t.MessageID, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName, &t.ClaimerDeleted)
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

// ---- Task Dependencies ----

type TaskDependency struct {
	BlockerTaskID string `json:"blocker_task_id"`
	BlockedTaskID string `json:"blocked_task_id"`
	CreatedAt     string `json:"created_at"`
}

// AddDependency creates a dependency: blocked_task waits for blocker_task.
// Detects cycles by walking the dependency chain.
func (s *TaskService) AddDependency(ctx context.Context, blockerTaskID, blockedTaskID string) (*TaskDependency, error) {
	if blockerTaskID == blockedTaskID {
		return nil, fmt.Errorf("a task cannot depend on itself")
	}

	// BUG-012: Validate that both tasks exist before INSERT.
	for _, tid := range []string{blockerTaskID, blockedTaskID} {
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1)`, tid).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check task existence: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("task not found: %s", tid)
		}
	}

	if err := s.checkDependencyCycle(ctx, blockerTaskID, blockedTaskID); err != nil {
		return nil, err
	}

	var dep TaskDependency
	err := s.pool.QueryRow(ctx, `
		INSERT INTO task_dependencies (blocker_task_id, blocked_task_id)
		VALUES ($1, $2)
		RETURNING blocker_task_id, blocked_task_id, created_at::text
	`, blockerTaskID, blockedTaskID).Scan(&dep.BlockerTaskID, &dep.BlockedTaskID, &dep.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add dependency: %w", err)
	}
	return &dep, nil
}

func (s *TaskService) checkDependencyCycle(ctx context.Context, blockerID, blockedID string) error {
	visited := map[string]bool{blockedID: true}
	queue := []string{blockerID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == blockedID {
			return fmt.Errorf("this dependency would create a cycle")
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := s.pool.Query(ctx, `
			SELECT blocker_task_id FROM task_dependencies
			WHERE blocked_task_id = $1
		`, current)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		var blockers []string
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				rows.Close()
				return err
			}
			blockers = append(blockers, b)
		}
		rows.Close()
		queue = append(queue, blockers...)
	}
	return nil
}

// ListBlockers returns tasks that block the given task.
func (s *TaskService) ListBlockers(ctx context.Context, taskID string) ([]TaskDependency, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT blocker_task_id, blocked_task_id, created_at::text
		FROM task_dependencies WHERE blocked_task_id = $1
		ORDER BY created_at
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list blockers: %w", err)
	}
	defer rows.Close()
	return scanDependencies(rows)
}

// ListBlocked returns tasks that the given task blocks.
func (s *TaskService) ListBlocked(ctx context.Context, taskID string) ([]TaskDependency, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT blocker_task_id, blocked_task_id, created_at::text
		FROM task_dependencies WHERE blocker_task_id = $1
		ORDER BY created_at
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list blocked: %w", err)
	}
	defer rows.Close()
	return scanDependencies(rows)
}

// IsTaskBlocked checks whether a task has unfinished blockers.
func (s *TaskService) IsTaskBlocked(ctx context.Context, taskID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_dependencies td
		INNER JOIN tasks t ON t.id = td.blocker_task_id
		WHERE td.blocked_task_id = $1 AND t.status != 'done' AND t.status != 'closed'
	`, taskID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check blocked: %w", err)
	}
	return count > 0, nil
}

// RemoveDependency deletes a dependency between two tasks.
func (s *TaskService) RemoveDependency(ctx context.Context, blockerTaskID, blockedTaskID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM task_dependencies WHERE blocker_task_id = $1 AND blocked_task_id = $2
	`, blockerTaskID, blockedTaskID)
	if err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dependency not found")
	}
	return nil
}

func scanDependencies(rows pgx.Rows) ([]TaskDependency, error) {
	var deps []TaskDependency
	for rows.Next() {
		var d TaskDependency
		if err := rows.Scan(&d.BlockerTaskID, &d.BlockedTaskID, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		deps = append(deps, d)
	}
	return deps, nil
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


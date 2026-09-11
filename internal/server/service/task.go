package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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
	ErrTaskNotFound           = errors.New("task not found")
	ErrTaskReferenceAmbiguous = errors.New("message prefix matches more than one message; use a longer ID")
	ErrTaskSourceExists       = errors.New("this message already has a task")
	ErrTaskInvalidStatus      = errors.New("invalid task status")
	ErrTaskInvalidTransition  = errors.New("invalid task status transition")
	ErrTaskNotChannelMember   = errors.New("user is not a channel member")
	ErrTaskAlreadyClaimed     = errors.New("task is already claimed by another agent")
	ErrTaskInTerminalState    = errors.New("task is in a terminal state and cannot be claimed")
	ErrTaskNotClaimable       = errors.New("task status does not allow claiming")
	ErrTaskNotClaimer         = errors.New("you are not the claimer of this task")
	ErrTaskAssigneeNotFound   = errors.New("task assignee not found in channel")
	ErrTaskAssigneeAmbiguous  = errors.New("task assignee is ambiguous")
	ErrTaskNotCreator         = errors.New("you are not the creator of this task")
	ErrTaskHumanOnly          = errors.New("this task action is human-only")
	ErrTaskNotSubmittable     = errors.New("task is not ready to submit")
	ErrTaskNotReviewable      = errors.New("task is not in review")
	ErrTaskHasOpenSubtasks    = errors.New("task has unfinished subtasks")
	ErrTaskReasonRequired     = errors.New("reject reason is required")
	ErrTaskLifecyclePatch     = errors.New("task lifecycle status changes must use lifecycle endpoints")
)

// Task represents a task in a channel.
type Task struct {
	Waiting             bool          `json:"waiting"`
	CanReview           bool          `json:"can_review"`
	Version             int64         `json:"version"`
	Contract            *TaskContract `json:"contract,omitempty"`
	CurrentSubmissionID string        `json:"current_submission_id,omitempty"`
	ID                  string        `json:"id"`
	TaskNumber          int           `json:"task_number"`
	ChannelID           string        `json:"channel_id"`
	CreatorID           string        `json:"creator_id"`
	CreatorName         string        `json:"creator_name,omitempty"`
	Title               string        `json:"title"`
	Description         string        `json:"description,omitempty"`
	Status              string        `json:"status"`
	ClaimerID           string        `json:"claimer_id,omitempty"`
	ClaimerName         string        `json:"claimer_name,omitempty"`
	ClaimerDeleted      bool          `json:"claimer_deleted"`
	Priority            string        `json:"priority"`
	DueDate             *time.Time    `json:"due_date,omitempty"`
	MessageID           string        `json:"message_id,omitempty"`
	ParentTaskID        *string       `json:"parent_task_id,omitempty"`
	SubtaskCount        int           `json:"subtask_count,omitempty"`
	DoneSubtaskCount    int           `json:"done_subtask_count,omitempty"`
	ArtifactStatus      string        `json:"artifact_status,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

// TaskCreateRequest contains the fields needed to create a task.
type TaskCreateRequest struct {
	Contract     *TaskContract `json:"contract,omitempty"`
	Title        string        `json:"title"`
	Description  string        `json:"description,omitempty"`
	Priority     string        `json:"priority,omitempty"`
	DueDate      *time.Time    `json:"due_date,omitempty"`
	MessageID    string        `json:"message_id,omitempty"`
	ParentTaskID string        `json:"parent_task_id,omitempty"`
	Assignee     string        `json:"assignee,omitempty"`
}

// TaskUpdateRequest contains the fields that can be updated on a task.
type TaskUpdateRequest struct {
	Contract            *TaskContract `json:"contract,omitempty"`
	ExpectedTaskVersion int64         `json:"expected_task_version,omitempty"`
	Title               *string       `json:"title,omitempty"`
	Description         *string       `json:"description,omitempty"`
	Status              *string       `json:"status,omitempty"`
	Priority            *string       `json:"priority,omitempty"`
	DueDate             *time.Time    `json:"due_date,omitempty"`
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
	pool              *pgxpool.Pool
	agentNotifier     *AgentNotifier
	artifactGenerator func(context.Context, string, string) (string, error)
}

// NewTaskService creates a new TaskService.
func NewTaskService(pool *pgxpool.Pool) *TaskService {
	return &TaskService{pool: pool}
}

func (s *TaskService) SetAgentNotifier(n *AgentNotifier) {
	s.agentNotifier = n
}

func (s *TaskService) SetArtifactGenerator(generator func(context.Context, string, string) (string, error)) {
	s.artifactGenerator = generator
}

// CreateTask creates a new task in the channel with per-channel task numbering.
func (s *TaskService) CreateTask(ctx context.Context, channelID, creatorID string, req TaskCreateRequest) (*Task, error) {
	// Verify user is a channel member (skip if no channel specified)
	if channelID != "" {
		if err := s.requireChannelMember(ctx, channelID, creatorID); err != nil {
			return nil, err
		}
	}

	if err := s.ValidateContract(ctx, channelID, creatorID, req.Contract); err != nil {
		return nil, err
	}
	// Validate title
	if req.Title == "" {
		return nil, errors.New("task title is required")
	}

	if req.Priority == "" {
		req.Priority = "none"
	}

	assigneeID, assigneeName, err := s.resolveTaskAssignee(ctx, channelID, req.Assignee)
	if err != nil {
		return nil, err
	}
	taskStatus := TaskStatusTodo
	var claimerID interface{}
	if assigneeID != "" {
		taskStatus = TaskStatusInProgress
		claimerID = assigneeID
	}

	// Validate parent_task_id if provided: must be a valid UUID pointing to an
	// existing task in the same channel.
	var parentTaskID interface{}
	if req.ParentTaskID != "" {
		if _, err := uuid.Parse(req.ParentTaskID); err != nil {
			return nil, fmt.Errorf("invalid parent_task_id: %w", err)
		}
		parentUUID, _ := uuid.Parse(req.ParentTaskID)
		parentTaskID = parentUUID
	} else {
		parentTaskID = nil
	}

	id := uuid.New().String()
	now := time.Now()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	nextNumber, err := nextTaskNumberTx(ctx, tx, channelID)
	if err != nil {
		return nil, err
	}
	if req.MessageID != "" {
		req.MessageID, err = resolveTaskMessageID(ctx, tx, channelID, req.MessageID)
		if err != nil {
			return nil, err
		}
		var sourceID string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM messages WHERE id=$1 FOR UPDATE`, req.MessageID).Scan(&sourceID); err != nil {
			return nil, err
		}
	}
	if req.ParentTaskID != "" {
		var parentChannel, parentStatus string
		err = tx.QueryRow(ctx, `SELECT channel_id::text,status FROM tasks WHERE id=$1 FOR UPDATE`, req.ParentTaskID).Scan(&parentChannel, &parentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		if err != nil {
			return nil, err
		}
		if parentChannel != channelID {
			return nil, errors.New("parent task is not in the same channel")
		}
		if parentStatus == TaskStatusInReview || TerminalStatuses[parentStatus] {
			return nil, ErrTaskInvalidTransition
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO tasks(id,task_number,channel_id,creator_id,title,description,status,claimer_id,priority,due_date,message_id,parent_task_id,created_at,updated_at,contract)
      VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13,$14)`, id, nextNumber, nullableStr(channelID), creatorID, req.Title, nullableStr(req.Description), taskStatus, claimerID, req.Priority, req.DueDate, nullableStr(req.MessageID), parentTaskID, now, req.Contract)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "unique_task_source_message" {
			return nil, ErrTaskSourceExists
		}
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}

	var pti *string
	if req.ParentTaskID != "" {
		pti = &req.ParentTaskID
	}
	task := &Task{
		Version: 1, Contract: req.Contract,
		ID:           id,
		TaskNumber:   nextNumber,
		ChannelID:    channelID,
		CreatorID:    creatorID,
		Title:        req.Title,
		Description:  req.Description,
		Status:       taskStatus,
		ClaimerID:    assigneeID,
		ClaimerName:  assigneeName,
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

func (s *TaskService) resolveTaskAssignee(ctx context.Context, channelID, assignee string) (string, string, error) {
	assignee = strings.TrimPrefix(strings.TrimSpace(assignee), "@")
	if assignee == "" {
		return "", "", nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.id::text, a.name
		  FROM agents a
		  JOIN channel_members cm ON cm.member_type = 'agent' AND cm.member_id = a.id
		 WHERE cm.channel_id = $1
		   AND a.is_active = true
		   AND (a.id::text = $2 OR lower(a.name) = lower($2))
		 LIMIT 2`, channelID, assignee)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	type match struct{ id, name string }
	var matches []match
	for rows.Next() {
		var candidate match
		if err := rows.Scan(&candidate.id, &candidate.name); err != nil {
			return "", "", err
		}
		matches = append(matches, candidate)
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", ErrTaskAssigneeNotFound
	}
	if len(matches) > 1 {
		return "", "", ErrTaskAssigneeAmbiguous
	}
	return matches[0].id, matches[0].name, nil
}

func nextTaskNumberTx(ctx context.Context, tx pgx.Tx, channelID string) (int, error) {
	// Share Automation's per-channel creation lock; numbering is part of the insert transaction.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, channelID); err != nil {
		return 0, err
	}
	var num int
	if channelID != "" {
		err := tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = $1`,
			channelID,
		).Scan(&num)
		return num, err
	}
	err := tx.QueryRow(ctx,
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

	if err := claimTaskTx(ctx, tx, channelID, taskID, userID); err != nil {
		return nil, err
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
		"new_status", refetched.Status,
	)

	return refetched, nil
}

func claimTaskTx(ctx context.Context, tx pgx.Tx, channelID, taskID, userID string) error {
	// Lock the task row for update to prevent concurrent claims.
	var currentStatus, currentClaimerID string
	err := tx.QueryRow(ctx,
		`SELECT status, COALESCE(claimer_id::text, '')
		 FROM tasks
		 WHERE id = $1 AND channel_id = $2
		 FOR UPDATE`,
		taskID, channelID,
	).Scan(&currentStatus, &currentClaimerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("lock task: %w", err)
	}

	// State validation: only todo and in_progress are claimable.
	if TerminalStatuses[currentStatus] {
		return ErrTaskInTerminalState
	}
	if currentStatus == TaskStatusInReview {
		return ErrTaskNotClaimable
	}
	// (TaskStatusTodo and TaskStatusInProgress fall through)

	// Claimer validation: prevent stealing from another claimer.
	if currentClaimerID != "" && currentClaimerID != userID {
		return ErrTaskAlreadyClaimed
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
		return fmt.Errorf("update claim: %w", err)
	}
	if currentClaimerID == "" {
		if _, err = tx.Exec(ctx, `INSERT INTO task_responsibility_events(task_id,task_version,actor_id,assignee_id,kind) SELECT id,version,$2,$2,'claimed' FROM tasks WHERE id=$1`, taskID, userID); err != nil {
			return err
		}
	}

	return nil
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

func (s *TaskService) SubmitTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	task, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status, claimerID string
	var contract *TaskContract
	err = tx.QueryRow(ctx,
		`SELECT status, COALESCE(claimer_id::text, ''), contract FROM tasks WHERE id = $1 AND channel_id = $2 FOR UPDATE`,
		task.ID, channelID,
	).Scan(&status, &claimerID, &contract)
	if err == nil && contract != nil {
		return nil, ErrTaskContractRequired
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if claimerID != userID {
		return nil, ErrTaskNotClaimer
	}
	if status != TaskStatusInProgress {
		return nil, ErrTaskNotSubmittable
	}
	var openSubtasks int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE parent_task_id = $1 AND status NOT IN ('done', 'closed')`,
		task.ID,
	).Scan(&openSubtasks); err != nil {
		return nil, err
	}
	if openSubtasks > 0 {
		return nil, ErrTaskHasOpenSubtasks
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2`, TaskStatusInReview, task.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	updated, err := s.GetTask(ctx, channelID, task.ID, userID)
	if err == nil && s.agentNotifier != nil {
		if notifyErr := s.agentNotifier.NotifyReviewReady(ctx, updated.ID, userID); notifyErr != nil {
			slog.Warn("notify review ready failed", "task_id", updated.ID, "err", notifyErr)
		}
	}
	if err == nil && updated.ParentTaskID == nil && s.artifactGenerator != nil {
		if artifactStatus, artifactErr := s.artifactGenerator(ctx, updated.ID, userID); artifactErr != nil {
			slog.Warn("artifact auto-generate failed", "task_id", updated.ID, "err", artifactErr)
		} else if artifactStatus != "" {
			updated.ArtifactStatus = artifactStatus
		}
	}
	return updated, err
}

func (s *TaskService) AcceptTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	updated, err := s.reviewTask(ctx, channelID, taskID, userID, TaskStatusDone, "accepted", "")
	if err == nil && s.agentNotifier != nil {
		if notifyErr := s.agentNotifier.NotifyAccepted(ctx, updated.ID, userID); notifyErr != nil {
			slog.Warn("notify accepted failed", "task_id", updated.ID, "err", notifyErr)
		}
	}
	return updated, err
}

func (s *TaskService) RejectTask(ctx context.Context, channelID, taskID, userID, reason string) (*Task, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrTaskReasonRequired
	}
	updated, err := s.reviewTask(ctx, channelID, taskID, userID, TaskStatusInProgress, "rejected", reason)
	if err == nil && s.agentNotifier != nil {
		if notifyErr := s.agentNotifier.NotifyRejected(ctx, updated.ID, userID, reason); notifyErr != nil {
			slog.Warn("notify rejected failed", "task_id", updated.ID, "err", notifyErr)
		}
	}
	return updated, err
}

func (s *TaskService) reviewTask(ctx context.Context, channelID, taskID, userID, status, decision, reason string) (*Task, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var creatorID, currentStatus, claimerID string
	var contract *TaskContract
	if err := tx.QueryRow(ctx,
		`SELECT creator_id::text, status, COALESCE(claimer_id::text, ''), contract
		 FROM tasks WHERE id = $1 AND channel_id = $2 FOR UPDATE`,
		taskID, channelID,
	).Scan(&creatorID, &currentStatus, &claimerID, &contract); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if contract != nil {
		return nil, ErrTaskContractRequired
	}
	if creatorID != userID {
		return nil, ErrTaskNotCreator
	}
	if currentStatus != TaskStatusInReview {
		return nil, ErrTaskNotReviewable
	}

	var artifactID string
	artifactErr := tx.QueryRow(ctx,
		`SELECT id::text FROM artifacts
		 WHERE task_id = $1 AND COALESCE(summary, '') <> 'pending'
		 ORDER BY updated_at DESC LIMIT 1`,
		taskID,
	).Scan(&artifactID)
	if artifactErr != nil && !errors.Is(artifactErr, pgx.ErrNoRows) {
		return nil, artifactErr
	}
	var artifactValue, nextOwnerValue any
	if artifactID != "" {
		artifactValue = artifactID
	}
	if decision == "rejected" && claimerID != "" {
		nextOwnerValue = claimerID
	}

	if _, err := tx.Exec(ctx,
		`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND channel_id = $3`,
		status, taskID, channelID,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO task_reviews (task_id, reviewer_id, decision, reason, artifact_id, next_owner_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		taskID, userID, decision, reason, artifactValue, nextOwnerValue,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetTask(ctx, channelID, taskID, userID)
}

func (s *TaskService) CloseTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	task, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}
	isAgent, err := s.isAgentActor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if isAgent {
		return nil, ErrTaskHumanOnly
	}
	if task.Status == TaskStatusClosed {
		return nil, ErrTaskInTerminalState
	}
	updated, err := s.setTaskStatus(ctx, channelID, task.ID, userID, TaskStatusClosed)
	if err == nil && s.agentNotifier != nil {
		if notifyErr := s.agentNotifier.NotifyClosed(ctx, updated.ID, userID); notifyErr != nil {
			slog.Warn("notify closed failed", "task_id", updated.ID, "err", notifyErr)
		}
	}
	return updated, err
}

func (s *TaskService) ReopenTask(ctx context.Context, channelID, taskID, userID string) (*Task, error) {
	task, err := s.GetTask(ctx, channelID, taskID, userID)
	if err != nil {
		return nil, err
	}
	isAgent, err := s.isAgentActor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if isAgent {
		return nil, ErrTaskHumanOnly
	}
	if task.CreatorID != userID {
		return nil, ErrTaskNotCreator
	}
	if task.Status != TaskStatusClosed && task.Status != TaskStatusDone {
		return nil, ErrTaskInvalidTransition
	}
	updated, err := s.setTaskStatus(ctx, channelID, task.ID, userID, TaskStatusTodo)
	if err == nil && s.agentNotifier != nil {
		if notifyErr := s.agentNotifier.NotifyReopened(ctx, updated.ID, userID); notifyErr != nil {
			slog.Warn("notify reopened failed", "task_id", updated.ID, "err", notifyErr)
		}
	}
	return updated, err
}

func (s *TaskService) setTaskStatus(ctx context.Context, channelID, taskID, userID, status string) (*Task, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND channel_id = $3`,
		status, taskID, channelID,
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrTaskNotFound
	}
	return s.GetTask(ctx, channelID, taskID, userID)
}

// ConvertMessageToTask creates a task from an existing message (asTask).
// The message content becomes the task title, and the task is linked via message_id.
func (s *TaskService) ConvertMessageToTask(ctx context.Context, channelID, messageID, userID string) (*Task, error) {
	task, _, err := s.messageTask(ctx, channelID, messageID, userID, false, nil, nil)
	return task, err
}

// ClaimMessageTask converts an ordinary message and claims it in one transaction.
// beforeClaim preserves the existing mention-priority window even if conversion raced.
func (s *TaskService) ClaimMessageTask(ctx context.Context, channelID, messageID, userID string, beforeClaim func(string) error, contract *TaskContract) (*Task, bool, error) {
	return s.messageTask(ctx, channelID, messageID, userID, true, beforeClaim, contract)
}

func resolveTaskMessageID(ctx context.Context, db agentRunRowQuerier, channelID, prefix string) (string, error) {
	if !messageIDPattern.MatchString(prefix) {
		return "", ErrTaskNotFound
	}
	var ids []string
	err := db.QueryRow(ctx, `SELECT COALESCE(array_agg(id::text),'{}') FROM (SELECT id FROM messages WHERE channel_id=$1 AND id::text LIKE lower($2)||'%' AND NOT COALESCE(is_deleted,false) AND thinking_node_id IS NULL ORDER BY id LIMIT 2) candidates`, channelID, prefix).Scan(&ids)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", ErrTaskNotFound
	}
	if len(ids) > 1 {
		return "", ErrTaskReferenceAmbiguous
	}
	return ids[0], nil
}

// A legacy message may still link to multiple Tasks. Never choose one implicitly.
func resolveMessageTaskID(ctx context.Context, db agentRunRowQuerier, channelID, messageID string) (string, error) {
	var ids []string
	err := db.QueryRow(ctx, `SELECT COALESCE(array_agg(id::text),'{}') FROM (SELECT id FROM tasks WHERE channel_id=$1 AND message_id=$2 ORDER BY id LIMIT 2) candidates`, channelID, messageID).Scan(&ids)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", pgx.ErrNoRows
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("source message has multiple historical tasks; use a task number or UUID: %w", ErrTaskReferenceAmbiguous)
	}
	return ids[0], nil
}

func (s *TaskService) messageTask(ctx context.Context, channelID, messageID, userID string, claim bool, beforeClaim func(string) error, contract *TaskContract) (*Task, bool, error) {
	if err := s.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	number, err := nextTaskNumberTx(ctx, tx, channelID)
	if err != nil {
		return nil, false, err
	}
	messageID, err = resolveTaskMessageID(ctx, tx, channelID, messageID)
	if err != nil {
		return nil, false, err
	}
	var content, senderType, senderID string
	if err = tx.QueryRow(ctx, `SELECT content,sender_type,sender_id::text FROM messages WHERE id=$1 AND NOT COALESCE(is_deleted,false) FOR UPDATE`, messageID).Scan(&content, &senderType, &senderID); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, false, invalidDelivery("an empty source message needs an explicit task title")
	}
	var taskID string
	taskID, err = resolveMessageTaskID(ctx, tx, channelID, messageID)
	created := errors.Is(err, pgx.ErrNoRows)
	if created {
		taskID = uuid.NewString()
		creator := userID
		if claim && (senderType == "user" || senderType == "agent") {
			creator = senderID
		}
		if contract != nil {
			if err = validateTaskContract(ctx, tx, channelID, creator, contract); err != nil {
				return nil, false, err
			}
			// Claiming a user's request never delegates their acceptance authority.
			if userID != creator && (senderType != "user" || contract.Gate.Kind != "human" || contract.Gate.ReviewerID != creator || contract.Gate.HumanReviewMode != "decision") {
				return nil, false, invalidDelivery("initial claim requirements must keep result confirmation with the source user")
			}
			if contract.Gate.ReviewerID == userID {
				return nil, false, invalidDelivery("the executing Agent cannot review its own delivery")
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO tasks(id,task_number,channel_id,creator_id,title,description,status,priority,message_id,contract) VALUES($1,$2,$3,$4,$5,$6,'todo','none',$7,$8)`, taskID, number, channelID, creator, truncateForTitle(content, 500), content, messageID, contract)
	} else if err == nil && contract != nil {
		var existing *TaskContract
		var creator string
		if err = tx.QueryRow(ctx, `SELECT contract,creator_id::text FROM tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&existing, &creator); err != nil {
			return nil, false, err
		}
		if err = validateTaskContract(ctx, tx, channelID, creator, contract); err != nil {
			return nil, false, err
		}
		if existing == nil || deliveryRequestHash(existing) != deliveryRequestHash(contract) {
			return nil, false, ErrTaskVersionConflict
		}
	}
	if err != nil {
		return nil, false, err
	}
	if claim {
		if beforeClaim != nil {
			if err = beforeClaim(taskID); err != nil {
				return nil, false, err
			}
		}
		if err = claimTaskTx(ctx, tx, channelID, taskID, userID); err != nil {
			return nil, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	task, err := s.GetTask(ctx, channelID, taskID, userID)
	return task, created, err
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

	var resolvedID string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM tasks WHERE channel_id=$1 AND (id::text=lower($2) OR task_number::text=$2) ORDER BY (id::text=lower($2)) DESC LIMIT 1`, channelID, taskID).Scan(&resolvedID)
	if errors.Is(err, pgx.ErrNoRows) {
		messageID, resolveErr := resolveTaskMessageID(ctx, s.pool, channelID, taskID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		resolvedID, err = resolveMessageTaskID(ctx, s.pool, channelID, messageID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	taskID = resolvedID
	// The same projection applies to UUIDs, task numbers and message prefixes.
	err = s.pool.QueryRow(ctx,
		`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''), t.status,
		 COALESCE(t.claimer_id::text, ''),
		 COALESCE(u_claimer.display_name, a_claimer.name, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''),
		 t.parent_task_id,
		 (SELECT COUNT(*) FROM tasks WHERE parent_task_id = t.id) AS subtask_count,
		 (SELECT COUNT(*) FROM tasks WHERE parent_task_id = t.id AND status = 'done') AS done_subtask_count,
		 t.created_at, t.updated_at,
		 `+taskArtifactStatusSQL("t")+` AS artifact_status,
		 (NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted, t.version, t.contract, COALESCE(t.current_submission_id::text,''), EXISTS(SELECT 1 FROM task_waits w WHERE w.task_id=t.id AND w.status='waiting')
		 FROM tasks t LEFT JOIN users u_creator ON t.creator_id = u_creator.id LEFT JOIN agents a_creator ON t.creator_id = a_creator.id LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id WHERE t.id = $1 AND t.channel_id = $2`,
		taskID, channelID,
	).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.Title, &description,
		&task.Status, &task.ClaimerID, &task.ClaimerName, &task.Priority, &dueDate, &task.MessageID,
		&task.ParentTaskID, &task.SubtaskCount, &task.DoneSubtaskCount, &task.CreatedAt, &task.UpdatedAt, &task.ArtifactStatus, &task.ClaimerDeleted, &task.Version, &task.Contract, &task.CurrentSubmissionID, &task.Waiting)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}

	task.Description = description
	if dueDate != nil {
		task.DueDate = dueDate
	}
	if task.Contract != nil && task.Status == TaskStatusInReview && task.ClaimerID != userID {
		task.CanReview = task.Contract.Gate.ReviewerID == userID
		if !task.CanReview && task.CreatorID == userID {
			if err := s.pool.QueryRow(ctx, `SELECT (EXISTS(SELECT 1 FROM task_reviews WHERE submission_id=NULLIF($1,'')::uuid AND decision='needs_human') OR EXISTS(SELECT 1 FROM task_review_deliveries d LEFT JOIN agent_runs r ON r.id=d.run_id WHERE d.submission_id=NULLIF($1,'')::uuid AND d.attempts>=3 AND (r.id IS NULL OR r.finished_at IS NOT NULL))) AND EXISTS(SELECT 1 FROM users WHERE id=$2)`, task.CurrentSubmissionID, userID).Scan(&task.CanReview); err != nil {
				return nil, err
			}
		}
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
			                  COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name,
			                  ` + taskArtifactStatusSQL("t") + ` AS artifact_status,
			                  (NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted, t.version, t.contract, COALESCE(t.current_submission_id::text,''), EXISTS(SELECT 1 FROM task_waits w WHERE w.task_id=t.id AND w.status='waiting')
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
			&dueDate, &t.MessageID, &parentTaskID, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName, &t.ArtifactStatus, &t.ClaimerDeleted, &t.Version, &t.Contract, &t.CurrentSubmissionID, &t.Waiting)
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

	if currentTask.Contract != nil || req.Contract != nil {
		if TerminalStatuses[currentTask.Status] {
			return nil, invalidDelivery("reopen a finished task before changing its requirements or scope")
		}
		if currentTask.CreatorID != userID {
			return nil, ErrTaskNotCreator
		}
		if req.ExpectedTaskVersion != currentTask.Version {
			return nil, ErrTaskVersionConflict
		}
	}
	contract := currentTask.Contract
	if req.Contract != nil {
		if err := s.ValidateContract(ctx, channelID, userID, req.Contract); err != nil {
			return nil, err
		}
		contract = req.Contract
	}
	// Validate status transition if status is being changed
	if req.Status != nil && *req.Status != "" {
		return nil, ErrTaskLifecyclePatch
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
			priority = $4, due_date = $5, updated_at = $6, contract = $9
		 WHERE id = $7 AND channel_id = $8 AND version = $10`,
		newTitle, nullableStr(newDescription), newStatus,
		newPriority, newDueDate, now, currentTask.ID, channelID, contract, currentTask.Version,
	)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, ErrTaskVersionConflict
	}

	slog.Info("task updated",
		"task_id", taskID,
		"channel_id", channelID,
		"user_id", userID,
		"new_status", newStatus,
	)

	return s.GetTask(ctx, channelID, currentTask.ID, userID)
}

func (s *TaskService) validateStatusActor(ctx context.Context, task *Task, userID, newStatus string) error {
	isClose := newStatus == TaskStatusClosed
	isReopen := task.Status == TaskStatusClosed && newStatus != TaskStatusClosed
	if isClose || isReopen {
		isAgent, err := s.isAgentActor(ctx, userID)
		if err != nil {
			return err
		}
		if isAgent {
			return ErrTaskHumanOnly
		}
	}
	if newStatus == TaskStatusDone && task.CreatorID != userID {
		return ErrTaskNotCreator
	}
	return nil
}

func (s *TaskService) isAgentActor(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, userID).Scan(&exists)
	return exists, err
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
		`SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE((SELECT display_name FROM users WHERE id=t.creator_id),(SELECT name FROM agents WHERE id=t.creator_id),''), t.title, COALESCE(t.description, ''), t.status,
		        COALESCE(t.claimer_id::text, ''), t.priority,
		        t.due_date, COALESCE(t.message_id::text, ''), t.parent_task_id, t.created_at, t.updated_at, t.version, t.contract, COALESCE(t.current_submission_id::text,''), EXISTS(SELECT 1 FROM task_waits w WHERE w.task_id=t.id AND w.status='waiting')
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
			&dueDate, &t.MessageID, &parentTaskID, &t.CreatedAt, &t.UpdatedAt, &t.Version, &t.Contract, &t.CurrentSubmissionID, &t.Waiting)
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
		`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status IN ($3, $4) AND contract IS NULL`,
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
func (s *TaskService) ListAllUserTasks(ctx context.Context, userID string, channelID string, status string, claimerID string, creatorID string, workspaceIDs ...string) ([]Task, error) {
	workspaceID := ""
	if len(workspaceIDs) > 0 {
		workspaceID = workspaceIDs[0]
	}
	query, args := buildListAllUserTasksQuery(userID, channelID, status, claimerID, creatorID, workspaceID)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var dueDate *time.Time
		var parentTaskID string
		err := rows.Scan(&t.ID, &t.TaskNumber, &t.ChannelID, &t.CreatorID, &t.CreatorName, &t.Title, &t.Description,
			&t.Status, &t.ClaimerID, &t.Priority, &dueDate, &t.MessageID, &parentTaskID,
			&t.SubtaskCount, &t.DoneSubtaskCount, &t.CreatedAt, &t.UpdatedAt, &t.ClaimerName, &t.ArtifactStatus, &t.ClaimerDeleted, &t.Version, &t.Contract, &t.CurrentSubmissionID, &t.Waiting)
		if err != nil {
			return nil, err
		}
		if dueDate != nil {
			t.DueDate = dueDate
		}
		if parentTaskID != "" {
			t.ParentTaskID = &parentTaskID
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func buildListAllUserTasksQuery(userID string, channelID string, status string, claimerID string, creatorID string, workspaceIDs ...string) (string, []interface{}) {
	workspaceID := ""
	if len(workspaceIDs) > 0 {
		workspaceID = workspaceIDs[0]
	}
	query := `SELECT t.id, t.task_number, t.channel_id, t.creator_id, COALESCE(u_creator.display_name, a_creator.name, '') as creator_name, t.title, COALESCE(t.description, ''),
		t.status, COALESCE(t.claimer_id::text, ''), t.priority, t.due_date, COALESCE(t.message_id::text, ''), COALESCE(t.parent_task_id::text, ''),
		(SELECT COUNT(*) FROM tasks child WHERE child.parent_task_id = t.id) AS subtask_count,
		(SELECT COUNT(*) FROM tasks child WHERE child.parent_task_id = t.id AND child.status = 'done') AS done_subtask_count,
		t.created_at, t.updated_at,
		COALESCE(u_claimer.display_name, a_claimer.name, '') AS claimer_name,
		` + taskArtifactStatusSQL("t") + ` AS artifact_status,
		(NOT COALESCE(a_claimer.is_active, true)) AS claimer_deleted, t.version, t.contract, COALESCE(t.current_submission_id::text,''), EXISTS(SELECT 1 FROM task_waits w WHERE w.task_id=t.id AND w.status='waiting')
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
	if workspaceID != "" {
		query += fmt.Sprintf(" AND c.workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
		argIdx++
	}

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
	if channelID == "" {
		query += " AND NOT EXISTS(SELECT 1 FROM agent_selection_trials internal_trial JOIN agent_selections internal_selection ON internal_selection.id=internal_trial.selection_id WHERE internal_trial.channel_id=c.id AND COALESCE(internal_selection.plan->>'reviewer_agent_id','')<>'')"
	}
	query += " ORDER BY t.created_at DESC LIMIT 100"

	return query, args
}

func taskArtifactStatusSQL(taskAlias string) string {
	return fmt.Sprintf(`CASE
		WHEN EXISTS (
			SELECT 1 FROM artifacts ar
			WHERE ar.task_id = %s.id
			  AND ar.kind = 'task_snapshot'
			  AND COALESCE(ar.summary, '') = 'pending'
			  AND ar.updated_at > now() - interval '5 minutes'
		) THEN 'pending'
		WHEN EXISTS (
			SELECT 1 FROM artifacts ar
			WHERE ar.task_id = %s.id
			  AND ar.kind = 'task_snapshot'
			  AND COALESCE(ar.summary, '') <> 'pending'
		) THEN 'available'
		ELSE 'none'
	END`, taskAlias, taskAlias)
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

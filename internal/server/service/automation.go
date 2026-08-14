package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/i18n"
	"github.com/solo-ai/solo/internal/realtime"
)

const (
	AutomationScheduleDaily    = "daily"
	AutomationScheduleWeekdays = "weekdays"
	AutomationScheduleWeekly   = "weekly"
	AutomationCompletionAuto   = "auto_complete"
	AutomationCompletionReview = "review_required"

	AutomationRunRunning   = "running"
	AutomationRunCompleted = "completed"
	AutomationRunSkipped   = "skipped"
	AutomationRunFailed    = "failed"

	automationPollInterval = 15 * time.Second
)

var (
	ErrAutomationNotFound      = errors.New("automation not found")
	ErrAutomationAlreadyActive = errors.New("automation already has an active run")
	ErrAutomationNotDue        = errors.New("automation is not due")
	ErrAutomationTargetMissing = errors.New("automation target agent is unavailable")
	ErrAutomationInvalidInput  = errors.New("invalid automation input")
)

// Automation describes one recurring task definition in a Channel.
type Automation struct {
	ID               string         `json:"id"`
	ChannelID        string         `json:"channel_id"`
	CreatorID        string         `json:"creator_id"`
	CreatorName      string         `json:"creator_name,omitempty"`
	Name             string         `json:"name"`
	TaskTitle        string         `json:"task_title"`
	TaskDescription  string         `json:"task_description"`
	TargetAgentID    string         `json:"target_agent_id,omitempty"`
	TargetAgentName  string         `json:"target_agent_name,omitempty"`
	ScheduleType     string         `json:"schedule_type"`
	ScheduleHour     int            `json:"schedule_hour"`
	ScheduleMinute   int            `json:"schedule_minute"`
	ScheduleWeekday  *int           `json:"schedule_weekday,omitempty"`
	Timezone         string         `json:"timezone"`
	CompletionPolicy string         `json:"completion_policy"`
	Enabled          bool           `json:"enabled"`
	NextRunAt        *time.Time     `json:"next_run_at,omitempty"`
	LastRunAt        *time.Time     `json:"last_run_at,omitempty"`
	LastRun          *AutomationRun `json:"last_run,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// AutomationRun records every scheduled or manual trigger, including skips.
type AutomationRun struct {
	ID                 string     `json:"id"`
	AutomationID       string     `json:"automation_id"`
	Source             string     `json:"source"`
	Status             string     `json:"status"`
	ScheduledFor       time.Time  `json:"scheduled_for"`
	TaskID             string     `json:"task_id,omitempty"`
	TaskNumber         *int       `json:"task_number,omitempty"`
	TaskTitle          string     `json:"task_title,omitempty"`
	CoalescedIntoRunID string     `json:"coalesced_into_run_id,omitempty"`
	FailureReason      string     `json:"failure_reason,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

// AutomationInput is the user-editable portion of an Automation.
type AutomationInput struct {
	Name             string `json:"name"`
	TaskTitle        string `json:"task_title"`
	TaskDescription  string `json:"task_description"`
	TargetAgentID    string `json:"target_agent_id"`
	ScheduleType     string `json:"schedule_type"`
	ScheduleHour     int    `json:"schedule_hour"`
	ScheduleMinute   int    `json:"schedule_minute"`
	ScheduleWeekday  *int   `json:"schedule_weekday"`
	Timezone         string `json:"timezone"`
	CompletionPolicy string `json:"completion_policy"`
	Enabled          bool   `json:"enabled"`
}

// AutomationService owns persistence, scheduling and task materialisation.
type AutomationService struct {
	pool      *pgxpool.Pool
	taskSvc   *TaskService
	hub       realtime.Broadcaster
	agentSvc  *AgentService
	pollEvery time.Duration
}

func NewAutomationService(pool *pgxpool.Pool, taskSvc *TaskService, hub realtime.Broadcaster, agentSvc *AgentService) *AutomationService {
	pollEvery := automationPollInterval
	if raw := strings.TrimSpace(os.Getenv("AUTOMATION_POLL_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed >= time.Second {
			pollEvery = parsed
		}
	}
	return &AutomationService{pool: pool, taskSvc: taskSvc, hub: hub, agentSvc: agentSvc, pollEvery: pollEvery}
}

// Start runs the database-backed scheduler until ctx is cancelled.
func (s *AutomationService) Start(ctx context.Context) {
	s.Tick(ctx)
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick reconciles finished work and dispatches each currently-due definition once.
// Missed occurrences intentionally collapse to one run; next_run_at is moved directly
// into the future before any task is created.
func (s *AutomationService) Tick(ctx context.Context) {
	if err := s.reconcileRuns(ctx); err != nil {
		slog.Warn("automation scheduler: reconcile failed", "error", err)
	}
	if err := s.pauseRepeatedFailures(ctx); err != nil {
		slog.Warn("automation scheduler: auto-pause failed", "error", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text
		  FROM automations
		 WHERE enabled = true AND next_run_at IS NOT NULL AND next_run_at <= now()
		 ORDER BY next_run_at ASC
		 LIMIT 50`)
	if err != nil {
		slog.Warn("automation scheduler: list due definitions failed", "error", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.dispatch(ctx, id, "scheduled"); err != nil && !errors.Is(err, ErrAutomationAlreadyActive) && !errors.Is(err, ErrAutomationNotDue) {
			slog.Warn("automation scheduler: dispatch failed", "automation_id", id, "error", err)
		}
	}
}

func (s *AutomationService) List(ctx context.Context, channelID, userID string) ([]Automation, error) {
	if err := s.taskSvc.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, automationSelect+`
		 WHERE a.channel_id = $1
		 ORDER BY a.created_at DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Automation, 0)
	for rows.Next() {
		item, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		last, err := s.latestRun(ctx, item.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		item.LastRun = last
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *AutomationService) Get(ctx context.Context, channelID, automationID, userID string) (*Automation, error) {
	if err := s.taskSvc.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	item, err := scanAutomation(s.pool.QueryRow(ctx, automationSelect+`
		 WHERE a.id = $1 AND a.channel_id = $2`, automationID, channelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAutomationNotFound
	}
	if err != nil {
		return nil, err
	}
	last, err := s.latestRun(ctx, item.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	item.LastRun = last
	return item, nil
}

func (s *AutomationService) Create(ctx context.Context, channelID, userID string, input AutomationInput) (*Automation, error) {
	if err := s.taskSvc.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	if input.CompletionPolicy == "" {
		input.CompletionPolicy = AutomationCompletionAuto
	}
	if err := s.validateInput(ctx, channelID, &input); err != nil {
		return nil, err
	}
	var nextRun any
	if input.Enabled {
		next, err := nextScheduledAt(input, time.Now())
		if err != nil {
			return nil, err
		}
		nextRun = next
	}
	id := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO automations (
			id, channel_id, creator_id, name, task_title, task_description,
			target_agent_id, schedule_type, schedule_hour, schedule_minute,
			schedule_weekday, timezone, completion_policy, enabled, next_run_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		id, channelID, userID, input.Name, input.TaskTitle, input.TaskDescription,
		input.TargetAgentID, input.ScheduleType, input.ScheduleHour, input.ScheduleMinute,
		input.ScheduleWeekday, input.Timezone, input.CompletionPolicy, input.Enabled, nextRun)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, channelID, id, userID)
}

func (s *AutomationService) Update(ctx context.Context, channelID, automationID, userID string, input AutomationInput) (*Automation, error) {
	if err := s.taskSvc.requireChannelMember(ctx, channelID, userID); err != nil {
		return nil, err
	}
	if input.CompletionPolicy == "" {
		if err := s.pool.QueryRow(ctx, `SELECT completion_policy FROM automations WHERE id=$1 AND channel_id=$2`, automationID, channelID).Scan(&input.CompletionPolicy); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAutomationNotFound
		} else if err != nil {
			return nil, err
		}
	}
	if err := s.validateInput(ctx, channelID, &input); err != nil {
		return nil, err
	}
	var nextRun any
	if input.Enabled {
		next, err := nextScheduledAt(input, time.Now())
		if err != nil {
			return nil, err
		}
		nextRun = next
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE automations SET
			name=$1, task_title=$2, task_description=$3, target_agent_id=$4,
			schedule_type=$5, schedule_hour=$6, schedule_minute=$7,
			schedule_weekday=$8, timezone=$9, completion_policy=$10, enabled=$11, next_run_at=$12, updated_at=now()
		 WHERE id=$13 AND channel_id=$14`,
		input.Name, input.TaskTitle, input.TaskDescription, input.TargetAgentID,
		input.ScheduleType, input.ScheduleHour, input.ScheduleMinute,
		input.ScheduleWeekday, input.Timezone, input.CompletionPolicy, input.Enabled, nextRun, automationID, channelID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrAutomationNotFound
	}
	return s.Get(ctx, channelID, automationID, userID)
}

func (s *AutomationService) Delete(ctx context.Context, channelID, automationID, userID string) error {
	if err := s.taskSvc.requireChannelMember(ctx, channelID, userID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM automations WHERE id=$1 AND channel_id=$2`, automationID, channelID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAutomationNotFound
	}
	return nil
}

func (s *AutomationService) ListRuns(ctx context.Context, channelID, automationID, userID string, limit int) ([]AutomationRun, error) {
	if _, err := s.Get(ctx, channelID, automationID, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, automationRunSelect+`
		 WHERE r.automation_id=$1
		 ORDER BY r.created_at DESC
		 LIMIT $2`, automationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AutomationRun, 0)
	for rows.Next() {
		item, err := scanAutomationRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *AutomationService) RunNow(ctx context.Context, channelID, automationID, userID string) (*AutomationRun, error) {
	if _, err := s.Get(ctx, channelID, automationID, userID); err != nil {
		return nil, err
	}
	return s.dispatch(ctx, automationID, "manual")
}

func (s *AutomationService) dispatch(ctx context.Context, automationID, source string) (*AutomationRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	automation, err := scanAutomation(tx.QueryRow(ctx, automationSelect+` WHERE a.id=$1 FOR UPDATE OF a`, automationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAutomationNotFound
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scheduledFor := now
	if source == "scheduled" {
		if !automation.Enabled || automation.NextRunAt == nil || automation.NextRunAt.After(now) {
			return nil, ErrAutomationNotDue
		}
		scheduledFor = *automation.NextRunAt
		next, err := nextScheduledAt(AutomationInput{
			ScheduleType: automation.ScheduleType, ScheduleHour: automation.ScheduleHour,
			ScheduleMinute: automation.ScheduleMinute, ScheduleWeekday: automation.ScheduleWeekday,
			Timezone: automation.Timezone,
		}, now)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE automations SET next_run_at=$1, updated_at=now() WHERE id=$2`, next, automation.ID); err != nil {
			return nil, err
		}
	}

	completedTaskIDs, err := reconcileAutomationRunTx(ctx, tx, automation.ID)
	if err != nil {
		return nil, err
	}
	active, err := scanAutomationRun(tx.QueryRow(ctx, automationRunSelect+`
		 WHERE r.automation_id=$1 AND r.status='running'
		 ORDER BY r.created_at DESC LIMIT 1`, automation.ID))
	if err == nil {
		skipped, createErr := insertSkippedRun(ctx, tx, automation.ID, source, scheduledFor, active.ID, "already_running")
		if createErr != nil {
			return nil, createErr
		}
		if _, createErr = tx.Exec(ctx, `UPDATE automations SET last_run_at=now(), updated_at=now() WHERE id=$1`, automation.ID); createErr != nil {
			return nil, createErr
		}
		if createErr = tx.Commit(ctx); createErr != nil {
			return nil, createErr
		}
		for _, taskID := range completedTaskIDs {
			s.broadcastTaskStatus(ctx, taskID)
		}
		return skipped, ErrAutomationAlreadyActive
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if automation.TargetAgentID == "" || !s.targetAgentAvailable(ctx, tx, automation.ChannelID, automation.TargetAgentID) {
		skipped, createErr := insertSkippedRun(ctx, tx, automation.ID, source, scheduledFor, "", "target_unavailable")
		if createErr != nil {
			return nil, createErr
		}
		if _, createErr = tx.Exec(ctx, `UPDATE automations SET last_run_at=now(), updated_at=now() WHERE id=$1`, automation.ID); createErr != nil {
			return nil, createErr
		}
		if createErr = tx.Commit(ctx); createErr != nil {
			return nil, createErr
		}
		for _, taskID := range completedTaskIDs {
			s.broadcastTaskStatus(ctx, taskID)
		}
		if createErr = s.pauseRepeatedFailures(ctx); createErr != nil {
			slog.Warn("automation: could not evaluate auto-pause", "automation_id", automation.ID, "error", createErr)
		}
		return skipped, nil
	}

	runID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO automation_runs (id, automation_id, source, status, scheduled_for)
		VALUES ($1,$2,$3,'running',$4)`, runID, automation.ID, source, scheduledFor); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, automation.ChannelID); err != nil {
		return nil, err
	}
	var taskNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id=$1`, automation.ChannelID).Scan(&taskNumber); err != nil {
		return nil, err
	}
	taskID := uuid.NewString()
	messageID := uuid.NewString()
	threadID := uuid.NewString()
	content := fmt.Sprintf("📋 Task #%d %s: %s", taskNumber, i18n.Active.SysTaskCreated, automation.TaskTitle)
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		VALUES ($1,$2,'system','00000000-0000-0000-0000-000000000000',$3,'system',$4,$4)`,
		messageID, automation.ChannelID, content, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO threads (id, channel_id, root_message_id, last_reply_at, created_at)
		VALUES ($1,$2,$3,$4,$4)`, threadID, automation.ChannelID, messageID, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tasks (
			id, task_number, channel_id, creator_id, title, description, status,
			claimer_id, priority, message_id, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'in_progress',$7,'normal',$8,$9,$9)`,
		taskID, taskNumber, automation.ChannelID, automation.CreatorID, automation.TaskTitle,
		nullableStr(automation.TaskDescription), automation.TargetAgentID, messageID, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE automation_runs SET task_id=$1 WHERE id=$2`, taskID, runID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE automations SET last_run_at=$1, updated_at=$1 WHERE id=$2`, now, automation.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, completedTaskID := range completedTaskIDs {
		s.broadcastTaskStatus(ctx, completedTaskID)
	}

	s.broadcastCreatedTask(automation, taskID, taskNumber, messageID, content, now)
	if s.agentSvc == nil || !s.agentSvc.TriggerAgentForTask(context.Background(), automation.ChannelID, taskID, automation.TargetAgentID, taskNumber, automation.TaskTitle, automation.TaskDescription, nil, nil) {
		if err := s.failDispatchedTask(context.Background(), runID, taskID, automation, taskNumber, messageID, now); err != nil {
			slog.Warn("automation: failed to record dispatch failure", "run_id", runID, "error", err)
		}
	}
	return s.getRun(ctx, runID)
}

func (s *AutomationService) targetAgentAvailable(ctx context.Context, tx pgx.Tx, channelID, agentID string) bool {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agents a
			JOIN channel_members cm ON cm.member_type='agent' AND cm.member_id=a.id
			WHERE a.id=$1 AND a.is_active=true AND cm.channel_id=$2
		)`, agentID, channelID).Scan(&exists)
	if err != nil || !exists || s.agentSvc == nil || s.agentSvc.dm == nil {
		return false
	}
	_, err = s.agentSvc.dm.ResolveDaemonForAgent(ctx, agentID, "llm")
	return err == nil
}

func (s *AutomationService) broadcastCreatedTask(automation *Automation, taskID string, taskNumber int, messageID, content string, now time.Time) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastToChannel(automation.ChannelID, realtime.Envelope("task.created", map[string]any{
		"id": taskID, "task_number": taskNumber, "channel_id": automation.ChannelID,
		"creator_id": automation.CreatorID, "creator_name": automation.CreatorName,
		"title": automation.TaskTitle, "description": automation.TaskDescription,
		"status": TaskStatusInProgress, "claimer_id": automation.TargetAgentID,
		"claimer_name": automation.TargetAgentName, "priority": "normal", "message_id": messageID,
		"created_at": now.UTC().Format(time.RFC3339), "updated_at": now.UTC().Format(time.RFC3339),
	}))
	s.hub.BroadcastToChannel(automation.ChannelID, realtime.Envelope("message.new", map[string]any{
		"id": messageID, "channel_id": automation.ChannelID, "sender_type": "system",
		"sender_id": "system", "sender_name": "Solo", "content": content,
		"content_type": "system", "task_number": taskNumber, "task_status": "",
		"created_at": now.UTC().Format(time.RFC3339),
	}))
}

func (s *AutomationService) failDispatchedTask(ctx context.Context, runID, taskID string, automation *Automation, taskNumber int, messageID string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE automation_runs SET status='failed', failure_reason='dispatch_failed', completed_at=now() WHERE id=$1 AND status='running'`, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status='closed', updated_at=now() WHERE id=$1 AND status='in_progress'`, taskID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.pauseRepeatedFailures(ctx); err != nil {
		slog.Warn("automation: could not evaluate auto-pause", "automation_id", automation.ID, "error", err)
	}
	if s.hub != nil {
		s.hub.BroadcastToChannel(automation.ChannelID, realtime.Envelope("task.updated", map[string]any{
			"id": taskID, "task_number": taskNumber, "channel_id": automation.ChannelID,
			"title": automation.TaskTitle, "description": automation.TaskDescription,
			"status": TaskStatusClosed, "claimer_id": automation.TargetAgentID,
			"claimer_name": automation.TargetAgentName, "priority": "normal", "message_id": messageID,
			"updated_at": now.UTC().Format(time.RFC3339),
		}))
	}
	return nil
}

func (s *AutomationService) reconcileRuns(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	completedRows, err := tx.Query(ctx, `
		UPDATE automation_runs r
		   SET status='completed', completed_at=now()
		 WHERE r.status='running'
		   AND r.task_id IS NOT NULL
		   AND COALESCE((
			SELECT ar.status
			  FROM agent_run_task_links link
			  JOIN agent_runs ar ON ar.id=link.run_id
			 WHERE link.task_id=r.task_id
			   AND link.role=$1
			 ORDER BY ar.started_at DESC, ar.id DESC
			 LIMIT 1
		   ), '')=$2
		   AND NOT EXISTS (
			SELECT 1
			  FROM agent_run_task_links link
			  JOIN agent_runs ar ON ar.id=link.run_id
			 WHERE link.task_id=r.task_id
			   AND link.role=$1
			   AND ar.status=ANY($3)
		   )
		 RETURNING r.task_id::text`, AgentRunTaskRolePrimary, AgentRunStatusCompleted, activeAgentRunStatuses())
	if err != nil {
		return err
	}
	completedTaskIDs, err := collectTaskIDs(completedRows)
	if err != nil {
		return err
	}
	for _, taskID := range completedTaskIDs {
		if err := updateCompletedAutomationTask(ctx, tx, taskID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE automation_runs r
		   SET status='failed', failure_reason='task_returned_to_human', completed_at=now()
		  FROM tasks t
		 WHERE r.status='running' AND r.task_id=t.id AND t.status='todo' AND t.claimer_id IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE automation_runs
		   SET status='failed', failure_reason='task_missing', completed_at=now()
		 WHERE status='running' AND task_id IS NULL AND created_at < now() - interval '1 minute'`); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, taskID := range completedTaskIDs {
		s.broadcastTaskStatus(ctx, taskID)
	}
	return nil
}

// pauseRepeatedFailures disables a definition after three consecutive relevant
// runs fail or cannot start. "Already running" skips are normal coalescing and
// are deliberately ignored when evaluating the streak.
func (s *AutomationService) pauseRepeatedFailures(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE automations a
		   SET enabled=false, next_run_at=NULL, updated_at=now()
		 WHERE a.enabled=true
		   AND EXISTS (
			SELECT 1
			  FROM (
				SELECT r.status, r.failure_reason
				  FROM automation_runs r
				 WHERE r.automation_id=a.id
				   AND NOT (r.status='skipped' AND r.failure_reason='already_running')
				 ORDER BY r.created_at DESC
				 LIMIT 3
			  ) recent
			HAVING count(*)=3
			   AND bool_and(recent.status IN ('failed','skipped'))
		   )`)
	return err
}

func reconcileAutomationRunTx(ctx context.Context, tx pgx.Tx, automationID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		UPDATE automation_runs r SET status='completed', completed_at=now()
		WHERE r.automation_id=$1 AND r.status='running'
		  AND r.task_id IS NOT NULL
		  AND COALESCE((
			SELECT ar.status
			  FROM agent_run_task_links link
			  JOIN agent_runs ar ON ar.id=link.run_id
			 WHERE link.task_id=r.task_id
			   AND link.role=$2
			 ORDER BY ar.started_at DESC, ar.id DESC
			 LIMIT 1
		  ), '')=$3
		  AND NOT EXISTS (
			SELECT 1
			  FROM agent_run_task_links link
			  JOIN agent_runs ar ON ar.id=link.run_id
			 WHERE link.task_id=r.task_id
			   AND link.role=$2
			   AND ar.status=ANY($4)
		  )
		RETURNING r.task_id::text`, automationID, AgentRunTaskRolePrimary, AgentRunStatusCompleted, activeAgentRunStatuses())
	if err != nil {
		return nil, err
	}
	completedTaskIDs, err := collectTaskIDs(rows)
	if err != nil {
		return nil, err
	}
	for _, taskID := range completedTaskIDs {
		if err := updateCompletedAutomationTask(ctx, tx, taskID); err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE automation_runs r SET status='failed', failure_reason='task_returned_to_human', completed_at=now()
		FROM tasks t
		WHERE r.automation_id=$1 AND r.status='running' AND r.task_id=t.id AND t.status='todo' AND t.claimer_id IS NULL`, automationID)
	return completedTaskIDs, err
}

func updateCompletedAutomationTask(ctx context.Context, tx pgx.Tx, taskID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE tasks t
		   SET status=CASE a.completion_policy
		                WHEN $2 THEN $3
		                ELSE $4
		              END,
		       updated_at=now()
		  FROM automation_runs r
		  JOIN automations a ON a.id=r.automation_id
		 WHERE t.id=$1
		   AND r.task_id=t.id
		   AND r.status='completed'
		   AND t.status IN ($5,$6)`,
		taskID, AutomationCompletionAuto, TaskStatusDone, TaskStatusInReview,
		TaskStatusInProgress, TaskStatusTodo)
	return err
}

func collectTaskIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var taskIDs []string
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	return taskIDs, rows.Err()
}

func (s *AutomationService) broadcastTaskStatus(ctx context.Context, taskID string) {
	if s.hub == nil {
		return
	}
	var task struct {
		ID, ChannelID, CreatorID, CreatorName, Title, Description, Status string
		ClaimerID, ClaimerName, Priority, MessageID                       string
		TaskNumber                                                        int
		UpdatedAt                                                         time.Time
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT t.id::text, t.channel_id::text, t.creator_id::text, COALESCE(u.display_name,''),
		       t.task_number, t.title, COALESCE(t.description,''), t.status,
		       COALESCE(t.claimer_id::text,''), COALESCE(a.name,''), t.priority,
		       COALESCE(t.message_id::text,''), t.updated_at
		  FROM tasks t
		  LEFT JOIN users u ON u.id=t.creator_id
		  LEFT JOIN agents a ON a.id=t.claimer_id
		 WHERE t.id=$1`, taskID).Scan(
		&task.ID, &task.ChannelID, &task.CreatorID, &task.CreatorName, &task.TaskNumber,
		&task.Title, &task.Description, &task.Status, &task.ClaimerID, &task.ClaimerName,
		&task.Priority, &task.MessageID, &task.UpdatedAt,
	); err != nil {
		slog.Warn("automation: could not reload task for status broadcast", "task_id", taskID, "error", err)
		return
	}
	s.hub.BroadcastToChannel(task.ChannelID, realtime.Envelope("task.updated", map[string]any{
		"id": task.ID, "task_number": task.TaskNumber, "channel_id": task.ChannelID,
		"creator_id": task.CreatorID, "creator_name": task.CreatorName,
		"title": task.Title, "description": task.Description, "status": task.Status,
		"claimer_id": task.ClaimerID, "claimer_name": task.ClaimerName,
		"priority": task.Priority, "message_id": task.MessageID,
		"updated_at": task.UpdatedAt.UTC().Format(time.RFC3339),
	}))
}

func insertSkippedRun(ctx context.Context, tx pgx.Tx, automationID, source string, scheduledFor time.Time, activeRunID, reason string) (*AutomationRun, error) {
	id := uuid.NewString()
	var active any
	if activeRunID != "" {
		active = activeRunID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO automation_runs (
			id, automation_id, source, status, scheduled_for, coalesced_into_run_id,
			failure_reason, completed_at
		) VALUES ($1,$2,$3,'skipped',$4,$5,$6,now())`,
		id, automationID, source, scheduledFor, active, reason)
	if err != nil {
		return nil, err
	}
	return &AutomationRun{ID: id, AutomationID: automationID, Source: source, Status: AutomationRunSkipped, ScheduledFor: scheduledFor, CoalescedIntoRunID: activeRunID, FailureReason: reason, CreatedAt: time.Now()}, nil
}

func (s *AutomationService) validateInput(ctx context.Context, channelID string, input *AutomationInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.TaskTitle = strings.TrimSpace(input.TaskTitle)
	input.TaskDescription = strings.TrimSpace(input.TaskDescription)
	input.TargetAgentID = strings.TrimSpace(input.TargetAgentID)
	input.ScheduleType = strings.TrimSpace(input.ScheduleType)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.CompletionPolicy = strings.TrimSpace(input.CompletionPolicy)
	if input.Name == "" || len(input.Name) > 120 {
		return fmt.Errorf("%w: automation name is required and must be at most 120 characters", ErrAutomationInvalidInput)
	}
	if input.TaskTitle == "" || len(input.TaskTitle) > 500 {
		return fmt.Errorf("%w: task title is required and must be at most 500 characters", ErrAutomationInvalidInput)
	}
	if len(input.TaskDescription) > 10000 {
		return fmt.Errorf("%w: task description must be at most 10000 characters", ErrAutomationInvalidInput)
	}
	if input.ScheduleHour < 0 || input.ScheduleHour > 23 || input.ScheduleMinute < 0 || input.ScheduleMinute > 59 {
		return fmt.Errorf("%w: invalid schedule time", ErrAutomationInvalidInput)
	}
	switch input.ScheduleType {
	case AutomationScheduleDaily, AutomationScheduleWeekdays:
		input.ScheduleWeekday = nil
	case AutomationScheduleWeekly:
		if input.ScheduleWeekday == nil || *input.ScheduleWeekday < 0 || *input.ScheduleWeekday > 6 {
			return fmt.Errorf("%w: weekly schedule requires a weekday", ErrAutomationInvalidInput)
		}
	default:
		return fmt.Errorf("%w: invalid schedule type", ErrAutomationInvalidInput)
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return fmt.Errorf("%w: invalid timezone", ErrAutomationInvalidInput)
	}
	if input.CompletionPolicy != AutomationCompletionAuto && input.CompletionPolicy != AutomationCompletionReview {
		return fmt.Errorf("%w: invalid completion policy", ErrAutomationInvalidInput)
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agents a
			JOIN channel_members cm ON cm.member_type='agent' AND cm.member_id=a.id
			WHERE a.id=$1 AND a.is_active=true AND cm.channel_id=$2
		)`, input.TargetAgentID, channelID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrAutomationTargetMissing
	}
	return nil
}

func nextScheduledAt(input AutomationInput, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return time.Time{}, errors.New("invalid timezone")
	}
	localAfter := after.In(loc)
	for days := 0; days <= 8; days++ {
		day := localAfter.AddDate(0, 0, days)
		candidate := time.Date(day.Year(), day.Month(), day.Day(), input.ScheduleHour, input.ScheduleMinute, 0, 0, loc)
		if !candidate.After(localAfter) {
			continue
		}
		matches := false
		switch input.ScheduleType {
		case AutomationScheduleDaily:
			matches = true
		case AutomationScheduleWeekdays:
			matches = candidate.Weekday() >= time.Monday && candidate.Weekday() <= time.Friday
		case AutomationScheduleWeekly:
			matches = input.ScheduleWeekday != nil && int(candidate.Weekday()) == *input.ScheduleWeekday
		default:
			return time.Time{}, errors.New("invalid schedule type")
		}
		if matches {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, errors.New("could not compute next schedule")
}

func (s *AutomationService) latestRun(ctx context.Context, automationID string) (*AutomationRun, error) {
	return scanAutomationRun(s.pool.QueryRow(ctx, automationRunSelect+`
		 WHERE r.automation_id=$1 ORDER BY r.created_at DESC LIMIT 1`, automationID))
}

func (s *AutomationService) getRun(ctx context.Context, runID string) (*AutomationRun, error) {
	return scanAutomationRun(s.pool.QueryRow(ctx, automationRunSelect+` WHERE r.id=$1`, runID))
}

const automationSelect = `
	SELECT a.id::text, a.channel_id::text, a.creator_id::text,
	       COALESCE(u.display_name,''), a.name, a.task_title, a.task_description,
	       COALESCE(a.target_agent_id::text,''), COALESCE(ag.name,''),
	       a.schedule_type, a.schedule_hour, a.schedule_minute, a.schedule_weekday,
	       a.timezone, a.completion_policy, a.enabled, a.next_run_at, a.last_run_at, a.created_at, a.updated_at
	  FROM automations a
	  LEFT JOIN users u ON u.id=a.creator_id
	  LEFT JOIN agents ag ON ag.id=a.target_agent_id`

const automationRunSelect = `
	SELECT r.id::text, r.automation_id::text, r.source, r.status, r.scheduled_for,
	       COALESCE(r.task_id::text,''), t.task_number, COALESCE(t.title,''),
	       COALESCE(r.coalesced_into_run_id::text,''), COALESCE(r.failure_reason,''),
	       r.created_at, r.completed_at
	  FROM automation_runs r
	  LEFT JOIN tasks t ON t.id=r.task_id`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAutomation(row rowScanner) (*Automation, error) {
	var item Automation
	if err := row.Scan(
		&item.ID, &item.ChannelID, &item.CreatorID, &item.CreatorName,
		&item.Name, &item.TaskTitle, &item.TaskDescription,
		&item.TargetAgentID, &item.TargetAgentName, &item.ScheduleType,
		&item.ScheduleHour, &item.ScheduleMinute, &item.ScheduleWeekday,
		&item.Timezone, &item.CompletionPolicy, &item.Enabled, &item.NextRunAt, &item.LastRunAt,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanAutomationRun(row rowScanner) (*AutomationRun, error) {
	var item AutomationRun
	if err := row.Scan(
		&item.ID, &item.AutomationID, &item.Source, &item.Status, &item.ScheduledFor,
		&item.TaskID, &item.TaskNumber, &item.TaskTitle, &item.CoalescedIntoRunID,
		&item.FailureReason, &item.CreatedAt, &item.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

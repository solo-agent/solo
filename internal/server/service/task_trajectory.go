package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TaskTrajectorySchemaVersion = "solo.task_trajectory.v1"
	maxTrajectoryTasks          = 500
	maxTrajectoryRunLinkRows    = 2000
	maxTrajectoryEvents         = 20000
	maxTrajectoryArtifacts      = 1000
	maxTrajectoryShortTextRunes = 500
	maxTrajectoryBodyTextRunes  = 4000
)

type TaskTrajectoryCoverage struct {
	Completeness           string `json:"completeness"`
	TaskTree               string `json:"task_tree"`
	RunMetadata            string `json:"run_metadata"`
	RunTaskLinks           string `json:"run_task_links"`
	RunEvents              string `json:"run_events"`
	TranscriptAvailability string `json:"transcript_availability"`
	TaskArtifacts          string `json:"task_artifacts"`
	DispatchDecisions      string `json:"dispatch_decisions"`
	ParentRunLinks         string `json:"parent_run_links"`
	OutboundMessageRuns    string `json:"outbound_message_runs"`
	TaskLifecycleEvents    string `json:"task_lifecycle_events"`
	RelationshipSnapshots  string `json:"relationship_snapshots"`
}

type TaskTrajectoryWarning struct {
	Code    string `json:"code"`
	RunID   string `json:"run_id,omitempty"`
	Message string `json:"message"`
}

type TaskTrajectoryTask struct {
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
	Priority         string     `json:"priority"`
	DueDate          *time.Time `json:"due_date,omitempty"`
	MessageID        string     `json:"message_id,omitempty"`
	ParentTaskID     string     `json:"parent_task_id,omitempty"`
	Depth            int        `json:"depth"`
	ContentTruncated bool       `json:"content_truncated,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type TaskTrajectoryLink struct {
	TaskID            string    `json:"task_id"`
	Role              string    `json:"role"`
	Confidence        float64   `json:"confidence"`
	MetadataTruncated bool      `json:"metadata_truncated,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type TaskTrajectoryTranscript struct {
	Status       string    `json:"status"`
	Association  string    `json:"association"`
	WindowStart  time.Time `json:"window_start"`
	WindowEnd    time.Time `json:"window_end"`
	TextExported bool      `json:"text_exported"`
}

type TaskTrajectoryEvent struct {
	ID             string         `json:"id"`
	RunID          string         `json:"run_id"`
	Seq            int            `json:"seq"`
	Type           string         `json:"type"`
	ToolName       string         `json:"tool_name,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ContentOmitted bool           `json:"content_omitted,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type TaskTrajectoryArtifact struct {
	ID               string    `json:"id"`
	TaskID           string    `json:"task_id"`
	Kind             string    `json:"kind"`
	Title            string    `json:"title"`
	Summary          string    `json:"summary,omitempty"`
	ContentTruncated bool      `json:"content_truncated,omitempty"`
	URL              string    `json:"url"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TaskTrajectoryRun struct {
	ID                string                   `json:"id"`
	AgentID           string                   `json:"agent_id"`
	AgentName         string                   `json:"agent_name,omitempty"`
	SessionID         string                   `json:"session_id,omitempty"`
	TriggerType       string                   `json:"trigger_type"`
	TriggerMessageID  string                   `json:"trigger_message_id,omitempty"`
	ChannelID         string                   `json:"channel_id,omitempty"`
	ThreadID          string                   `json:"thread_id,omitempty"`
	ThinkingNodeID    string                   `json:"thinking_node_id,omitempty"`
	Status            AgentRunStatus           `json:"status"`
	ToolName          string                   `json:"tool_name,omitempty"`
	Source            string                   `json:"source,omitempty"`
	Usage             map[string]any           `json:"usage,omitempty"`
	MetadataTruncated bool                     `json:"metadata_truncated,omitempty"`
	StartedAt         time.Time                `json:"started_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
	FinishedAt        *time.Time               `json:"finished_at,omitempty"`
	TaskLinks         []TaskTrajectoryLink     `json:"task_links"`
	Events            []TaskTrajectoryEvent    `json:"events"`
	Transcript        TaskTrajectoryTranscript `json:"transcript"`
}

type TaskTrajectorySnapshot struct {
	SchemaVersion      string                   `json:"schema_version"`
	DatabaseCapturedAt time.Time                `json:"database_captured_at"`
	RootTaskID         string                   `json:"root_task_id"`
	Coverage           TaskTrajectoryCoverage   `json:"coverage"`
	Tasks              []TaskTrajectoryTask     `json:"tasks"`
	Runs               []TaskTrajectoryRun      `json:"runs"`
	Artifacts          []TaskTrajectoryArtifact `json:"artifacts"`
	Warnings           []TaskTrajectoryWarning  `json:"warnings"`
}

type TaskTrajectoryService struct {
	pool *pgxpool.Pool
}

func NewTaskTrajectoryService(pool *pgxpool.Pool) *TaskTrajectoryService {
	return &TaskTrajectoryService{pool: pool}
}

type taskTrajectoryRunSource struct {
	referenceRecorded bool
}

type taskTrajectoryTruncation struct {
	tasks     bool
	runLinks  bool
	events    bool
	artifacts bool
}

func (s *TaskTrajectoryService) Export(ctx context.Context, taskID, userID string) (*TaskTrajectorySnapshot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var capturedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&capturedAt); err != nil {
		return nil, err
	}
	rootChannelID, err := authorizeTaskTrajectory(ctx, tx, taskID, userID)
	if err != nil {
		return nil, err
	}

	tasks, tasksTruncated, err := loadTaskTrajectoryTasks(ctx, tx, taskID, rootChannelID)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	runs, sources, runLinksTruncated, err := loadTaskTrajectoryRuns(ctx, tx, taskIDs, rootChannelID)
	if err != nil {
		return nil, err
	}
	eventsTruncated, err := loadTaskTrajectoryEvents(ctx, tx, runs)
	if err != nil {
		return nil, err
	}
	artifacts, artifactsTruncated, err := loadTaskTrajectoryArtifacts(ctx, tx, taskIDs, rootChannelID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	warnings := []TaskTrajectoryWarning{}
	for i := range runs {
		runWarnings := attachTaskTrajectoryTranscriptReference(&runs[i], sources[runs[i].ID], capturedAt)
		warnings = append(warnings, runWarnings...)
	}
	truncation := taskTrajectoryTruncation{
		tasks:     tasksTruncated,
		runLinks:  runLinksTruncated,
		events:    eventsTruncated,
		artifacts: artifactsTruncated,
	}
	warnings = append(warnings, trajectoryTruncationWarnings(truncation)...)
	completeness := "partial_stored_records"
	if tasksTruncated || runLinksTruncated || eventsTruncated || artifactsTruncated {
		completeness = "partial_truncated_stored_records"
	}

	return &TaskTrajectorySnapshot{
		SchemaVersion:      TaskTrajectorySchemaVersion,
		DatabaseCapturedAt: capturedAt,
		RootTaskID:         taskID,
		Coverage: TaskTrajectoryCoverage{
			Completeness:           completeness,
			TaskTree:               "stored_records",
			RunMetadata:            "stored_linked_selected_metadata",
			RunTaskLinks:           "stored_records",
			RunEvents:              "stored_metadata_only",
			TranscriptAvailability: "reference_recorded_only_unverified_time_window",
			TaskArtifacts:          "stored_selected_metadata",
			DispatchDecisions:      "unavailable",
			ParentRunLinks:         "unavailable",
			OutboundMessageRuns:    "unavailable",
			TaskLifecycleEvents:    "unavailable",
			RelationshipSnapshots:  "unavailable",
		},
		Tasks:     tasks,
		Runs:      runs,
		Artifacts: artifacts,
		Warnings:  warnings,
	}, nil
}

func authorizeTaskTrajectory(ctx context.Context, tx pgx.Tx, taskID, userID string) (string, error) {
	var channelID string
	if err := tx.QueryRow(ctx, `SELECT channel_id::text FROM tasks WHERE id = $1`, taskID).Scan(&channelID); err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrTaskNotFound
		}
		return "", err
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM channels c
			  JOIN channel_members cm ON cm.channel_id = c.id
			 WHERE c.id = $1 AND c.is_archived = false
			   AND cm.member_type = 'user' AND cm.member_id = $2
		)`, channelID, userID).Scan(&allowed); err != nil {
		return "", err
	}
	if !allowed {
		return "", ErrTaskNotChannelMember
	}
	return channelID, nil
}

func loadTaskTrajectoryTasks(ctx context.Context, tx pgx.Tx, rootTaskID, rootChannelID string) ([]TaskTrajectoryTask, bool, error) {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE task_tree(id, depth) AS (
			SELECT t.id, 0 FROM tasks t WHERE t.id = $1
			UNION ALL
			SELECT child.id, parent.depth + 1
			  FROM tasks child
			  JOIN task_tree parent ON child.parent_task_id = parent.id
			 WHERE child.channel_id = $2
		), bounded_tree AS MATERIALIZED (
			SELECT id, depth FROM task_tree LIMIT $3
		)
		SELECT t.id::text, COALESCE(t.task_number, 0), COALESCE(t.channel_id::text, ''),
		       t.creator_id::text, COALESCE(u_creator.display_name, a_creator.name, ''),
		       left(t.title, $4), char_length(t.title) > $4,
		       left(COALESCE(t.description, ''), $5), char_length(COALESCE(t.description, '')) > $5,
		       left(t.status, $4), char_length(t.status) > $4,
		       COALESCE(t.claimer_id::text, ''), COALESCE(u_claimer.display_name, a_claimer.name, ''),
		       left(COALESCE(t.priority, ''), $4), char_length(COALESCE(t.priority, '')) > $4,
		       t.due_date, COALESCE(t.message_id::text, ''),
		       COALESCE(t.parent_task_id::text, ''), tree.depth, t.created_at, t.updated_at
		  FROM bounded_tree tree
		  JOIN tasks t ON t.id = tree.id
		  LEFT JOIN users u_creator ON t.creator_id = u_creator.id
		  LEFT JOIN agents a_creator ON t.creator_id = a_creator.id
		  LEFT JOIN users u_claimer ON t.claimer_id = u_claimer.id
		  LEFT JOIN agents a_claimer ON t.claimer_id = a_claimer.id
		 ORDER BY tree.depth ASC, t.created_at ASC, t.id ASC`,
		rootTaskID, rootChannelID, maxTrajectoryTasks+1, maxTrajectoryShortTextRunes, maxTrajectoryBodyTextRunes)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	tasks := []TaskTrajectoryTask{}
	for rows.Next() {
		var task TaskTrajectoryTask
		var dueDate sql.NullTime
		var titleTruncated bool
		var descriptionTruncated bool
		var statusTruncated bool
		var priorityTruncated bool
		if err := rows.Scan(
			&task.ID, &task.TaskNumber, &task.ChannelID, &task.CreatorID, &task.CreatorName,
			&task.Title, &titleTruncated, &task.Description, &descriptionTruncated,
			&task.Status, &statusTruncated, &task.ClaimerID, &task.ClaimerName,
			&task.Priority, &priorityTruncated, &dueDate, &task.MessageID, &task.ParentTaskID, &task.Depth,
			&task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, false, err
		}
		if dueDate.Valid {
			task.DueDate = &dueDate.Time
		}
		task.ContentTruncated = titleTruncated || descriptionTruncated || statusTruncated || priorityTruncated
		if !safeTaskTrajectoryIdentifier(task.Status, 128) {
			task.Status = "unknown"
			task.ContentTruncated = true
		}
		if task.Priority != "" && !safeTaskTrajectoryIdentifier(task.Priority, 128) {
			task.Priority = "unknown"
			task.ContentTruncated = true
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(tasks) > maxTrajectoryTasks
	if truncated {
		tasks = tasks[:maxTrajectoryTasks]
	}
	return tasks, truncated, nil
}

func loadTaskTrajectoryRuns(ctx context.Context, tx pgx.Tx, taskIDs []string, rootChannelID string) ([]TaskTrajectoryRun, map[string]taskTrajectoryRunSource, bool, error) {
	if len(taskIDs) == 0 {
		return []TaskTrajectoryRun{}, map[string]taskTrajectoryRunSource{}, false, nil
	}
	rows, err := tx.Query(ctx, `
		WITH bounded_links AS MATERIALIZED (
			SELECT l.run_id, l.task_id, l.role, l.confidence, l.created_at
			  FROM agent_run_task_links l
			  JOIN tasks linked_task ON linked_task.id = l.task_id
			  JOIN agent_runs linked_run ON linked_run.id = l.run_id
			 WHERE l.task_id = ANY($1::uuid[]) AND linked_task.channel_id = $2
			   AND (linked_run.channel_id = $2 OR linked_run.channel_id IS NULL)
			 LIMIT $3
		)
		SELECT r.id::text, r.agent_id::text,
		       left(COALESCE(a.name, ''), $4), char_length(COALESCE(a.name, '')) > $4,
		       COALESCE(r.session_id::text, ''),
		       left(r.trigger_type, $4), char_length(r.trigger_type) > $4,
		       COALESCE(r.trigger_message_id::text, ''), COALESCE(r.channel_id::text, ''),
		       COALESCE(r.thread_id::text, ''), COALESCE(r.thinking_node_id::text, ''),
		       left(r.status, $4), char_length(r.status) > $4,
		       left(COALESCE(r.tool_name, ''), $4), char_length(COALESCE(r.tool_name, '')) > $4,
		       left(COALESCE(r.source, ''), $4), char_length(COALESCE(r.source, '')) > $4,
		       CASE WHEN COALESCE(r.usage_json->>'input_tokens', '') ~ '^[0-9]{1,18}$' THEN (r.usage_json->>'input_tokens')::bigint END,
		       CASE WHEN COALESCE(r.usage_json->>'output_tokens', '') ~ '^[0-9]{1,18}$' THEN (r.usage_json->>'output_tokens')::bigint END,
		       CASE WHEN COALESCE(r.usage_json->>'cache_creation_input_tokens', '') ~ '^[0-9]{1,18}$' THEN (r.usage_json->>'cache_creation_input_tokens')::bigint END,
		       CASE WHEN COALESCE(r.usage_json->>'cache_read_input_tokens', '') ~ '^[0-9]{1,18}$' THEN (r.usage_json->>'cache_read_input_tokens')::bigint END,
		       r.started_at, r.updated_at, r.finished_at,
		       l.task_id::text, left(l.role, $4), char_length(l.role) > $4, l.confidence, l.created_at,
		       (COALESCE(r.transcript_path, sess.transcript_path, '') <> '' OR COALESCE(sess.external_session_id, '') <> '')
		  FROM bounded_links l
		  JOIN agent_runs r ON r.id = l.run_id
		  LEFT JOIN agents a ON a.id = r.agent_id
		  LEFT JOIN agent_sessions sess ON sess.id = r.session_id
		 ORDER BY r.started_at ASC, r.id ASC, l.created_at ASC, l.task_id ASC
		`, taskIDs, rootChannelID, maxTrajectoryRunLinkRows+1, maxTrajectoryShortTextRunes)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()

	runs := []TaskTrajectoryRun{}
	indexByID := map[string]int{}
	sources := map[string]taskTrajectoryRunSource{}
	rowCount := 0
	for rows.Next() {
		rowCount++
		if rowCount > maxTrajectoryRunLinkRows {
			break
		}
		var run TaskTrajectoryRun
		var link TaskTrajectoryLink
		var status string
		var agentNameTruncated bool
		var triggerTypeTruncated bool
		var statusTruncated bool
		var toolNameTruncated bool
		var sourceTruncated bool
		var roleTruncated bool
		var finished sql.NullTime
		var inputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var cacheCreationTokens sql.NullInt64
		var cacheReadTokens sql.NullInt64
		var source taskTrajectoryRunSource
		if err := rows.Scan(
			&run.ID, &run.AgentID, &run.AgentName, &agentNameTruncated, &run.SessionID,
			&run.TriggerType, &triggerTypeTruncated,
			&run.TriggerMessageID, &run.ChannelID, &run.ThreadID, &run.ThinkingNodeID,
			&status, &statusTruncated, &run.ToolName, &toolNameTruncated, &run.Source, &sourceTruncated,
			&inputTokens, &outputTokens, &cacheCreationTokens, &cacheReadTokens,
			&run.StartedAt, &run.UpdatedAt, &finished,
			&link.TaskID, &link.Role, &roleTruncated, &link.Confidence, &link.CreatedAt,
			&source.referenceRecorded,
		); err != nil {
			return nil, nil, false, err
		}
		run.Status = AgentRunStatus(status)
		run.MetadataTruncated = agentNameTruncated || triggerTypeTruncated || statusTruncated || toolNameTruncated || sourceTruncated
		link.MetadataTruncated = roleTruncated
		if !safeTaskTrajectoryIdentifier(run.TriggerType, 128) {
			run.TriggerType = "unknown"
			run.MetadataTruncated = true
		}
		if !safeTaskTrajectoryIdentifier(string(run.Status), 128) {
			run.Status = AgentRunStatus("unknown")
			run.MetadataTruncated = true
		}
		if run.ToolName != "" && !safeTaskTrajectoryIdentifier(run.ToolName, 128) {
			run.ToolName = ""
			run.MetadataTruncated = true
		}
		if run.Source != "" && !safeTaskTrajectoryIdentifier(run.Source, 128) {
			run.Source = ""
			run.MetadataTruncated = true
		}
		if !safeTaskTrajectoryIdentifier(link.Role, 128) {
			link.Role = "unknown"
			link.MetadataTruncated = true
		}
		run.Usage = taskTrajectoryUsageFromColumns(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens)
		if finished.Valid {
			run.FinishedAt = &finished.Time
		}
		if idx, ok := indexByID[run.ID]; ok {
			runs[idx].TaskLinks = append(runs[idx].TaskLinks, link)
			continue
		}
		run.TaskLinks = []TaskTrajectoryLink{link}
		run.Events = []TaskTrajectoryEvent{}
		indexByID[run.ID] = len(runs)
		runs = append(runs, run)
		sources[run.ID] = source
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}
	return runs, sources, rowCount > maxTrajectoryRunLinkRows, nil
}

func loadTaskTrajectoryEvents(ctx context.Context, tx pgx.Tx, runs []TaskTrajectoryRun) (bool, error) {
	if len(runs) == 0 {
		return false, nil
	}
	runIDs := make([]string, 0, len(runs))
	indexByID := make(map[string]int, len(runs))
	for i := range runs {
		runIDs = append(runIDs, runs[i].ID)
		indexByID[runs[i].ID] = i
	}
	rows, err := tx.Query(ctx, `
		WITH bounded_events AS MATERIALIZED (
			SELECT id FROM agent_run_events WHERE run_id = ANY($1::uuid[]) LIMIT $2
		)
		SELECT e.id::text, e.run_id::text, e.seq,
		       left(e.type, $3), char_length(e.type) > $3,
		       left(COALESCE(e.tool_name, ''), $3), char_length(COALESCE(e.tool_name, '')) > $3,
		       jsonb_strip_nulls(jsonb_build_object(
		         'agent_id', left(e.payload->>'agent_id', 128),
		         'task_id', left(e.payload->>'task_id', 128),
		         'call_id', left(e.payload->>'call_id', 128),
		         'role', left(e.payload->>'role', 128),
		         'status', left(e.payload->>'status', 128),
		         'is_error', CASE WHEN jsonb_typeof(e.payload->'is_error') = 'boolean' THEN (e.payload->>'is_error')::boolean END,
		         'exit_code', CASE WHEN COALESCE(e.payload->>'exit_code', '') ~ '^-?[0-9]{1,18}$' THEN (e.payload->>'exit_code')::bigint END,
		         'duration_ms', CASE WHEN COALESCE(e.payload->>'duration_ms', '') ~ '^[0-9]{1,18}$' THEN (e.payload->>'duration_ms')::bigint END,
		         'input_tokens', CASE WHEN COALESCE(e.payload->>'input_tokens', '') ~ '^[0-9]{1,18}$' THEN (e.payload->>'input_tokens')::bigint END,
		         'output_tokens', CASE WHEN COALESCE(e.payload->>'output_tokens', '') ~ '^[0-9]{1,18}$' THEN (e.payload->>'output_tokens')::bigint END,
		         'cache_creation_input_tokens', CASE WHEN COALESCE(e.payload->>'cache_creation_input_tokens', '') ~ '^[0-9]{1,18}$' THEN (e.payload->>'cache_creation_input_tokens')::bigint END,
		         'cache_read_input_tokens', CASE WHEN COALESCE(e.payload->>'cache_read_input_tokens', '') ~ '^[0-9]{1,18}$' THEN (e.payload->>'cache_read_input_tokens')::bigint END
		       )),
		       (e.message <> '' OR e.payload <> '{}'::jsonb), e.created_at
		  FROM bounded_events bounded
		  JOIN agent_run_events e ON e.id = bounded.id
		  JOIN agent_runs r ON r.id = e.run_id
		 ORDER BY r.started_at ASC, e.run_id ASC, e.seq ASC
		`, runIDs, maxTrajectoryEvents+1, maxTrajectoryShortTextRunes)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	rowCount := 0
	for rows.Next() {
		rowCount++
		if rowCount > maxTrajectoryEvents {
			break
		}
		var event TaskTrajectoryEvent
		var eventTypeTruncated bool
		var toolNameTruncated bool
		var metadataJSON json.RawMessage
		var sourceContentPresent bool
		if err := rows.Scan(
			&event.ID, &event.RunID, &event.Seq, &event.Type, &eventTypeTruncated,
			&event.ToolName, &toolNameTruncated, &metadataJSON, &sourceContentPresent, &event.CreatedAt,
		); err != nil {
			return false, err
		}
		if idx, ok := indexByID[event.RunID]; ok {
			metadata, payloadOmitted := taskTrajectoryEventMetadata(metadataJSON)
			event.Metadata = metadata
			event.ContentOmitted = sourceContentPresent || payloadOmitted || eventTypeTruncated || toolNameTruncated
			if !safeTaskTrajectoryIdentifier(event.Type, 128) {
				event.Type = "unknown"
				event.ContentOmitted = true
			}
			if event.ToolName != "" && !safeTaskTrajectoryIdentifier(event.ToolName, 128) {
				event.ToolName = ""
				event.ContentOmitted = true
			}
			runs[idx].Events = append(runs[idx].Events, event)
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return rowCount > maxTrajectoryEvents, nil
}

func taskTrajectoryEventMetadata(payload json.RawMessage) (map[string]any, bool) {
	var values map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &values) != nil || len(values) == 0 {
		return nil, len(payload) > 0 && string(payload) != "{}"
	}
	allowed := map[string]bool{
		"agent_id": true, "task_id": true, "call_id": true, "role": true,
		"status": true, "is_error": true, "exit_code": true, "duration_ms": true,
		"input_tokens": true, "output_tokens": true, "cache_creation_input_tokens": true,
		"cache_read_input_tokens": true,
	}
	metadata := map[string]any{}
	omitted := false
	for key, value := range values {
		if !allowed[key] || !safeTaskTrajectoryEventMetadataValue(key, value) {
			omitted = true
			continue
		}
		metadata[key] = value
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return metadata, omitted
}

func safeTaskTrajectoryEventMetadataValue(key string, value any) bool {
	switch typed := value.(type) {
	case bool:
		return key == "is_error"
	case float64:
		return true
	case string:
		switch key {
		case "agent_id", "task_id":
			_, err := uuid.Parse(typed)
			return err == nil
		case "call_id", "role", "status":
			return safeTaskTrajectoryIdentifier(typed, 128)
		}
	}
	return false
}

func safeTaskTrajectoryIdentifier(value string, maxRunes int) bool {
	runes := []rune(value)
	if len(runes) == 0 || len(runes) > maxRunes {
		return false
	}
	for _, r := range runes {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func taskTrajectoryUsageFromColumns(input, output, cacheCreation, cacheRead sql.NullInt64) map[string]any {
	metadata := map[string]any{}
	for key, value := range map[string]sql.NullInt64{
		"input_tokens": input, "output_tokens": output,
		"cache_creation_input_tokens": cacheCreation, "cache_read_input_tokens": cacheRead,
	} {
		if value.Valid {
			metadata[key] = value.Int64
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func loadTaskTrajectoryArtifacts(ctx context.Context, tx pgx.Tx, taskIDs []string, rootChannelID string) ([]TaskTrajectoryArtifact, bool, error) {
	if len(taskIDs) == 0 {
		return []TaskTrajectoryArtifact{}, false, nil
	}
	rows, err := tx.Query(ctx, `
		WITH bounded_artifacts AS MATERIALIZED (
			SELECT id FROM artifacts
			 WHERE task_id = ANY($1::uuid[]) AND channel_id = $2
			 LIMIT $3
		)
		SELECT artifact.id::text, artifact.task_id::text,
		       left(artifact.kind, $4), char_length(artifact.kind) > $4,
		       left(artifact.title, $4), char_length(artifact.title) > $4,
		       left(COALESCE(artifact.summary, ''), $5), char_length(COALESCE(artifact.summary, '')) > $5,
		       artifact.created_by::text, artifact.created_at, artifact.updated_at
		  FROM bounded_artifacts bounded
		  JOIN artifacts artifact ON artifact.id = bounded.id
		 ORDER BY artifact.created_at ASC, artifact.id ASC`,
		taskIDs, rootChannelID, maxTrajectoryArtifacts+1, maxTrajectoryShortTextRunes, maxTrajectoryBodyTextRunes)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	artifacts := []TaskTrajectoryArtifact{}
	rowCount := 0
	for rows.Next() {
		rowCount++
		if rowCount > maxTrajectoryArtifacts {
			break
		}
		var artifact TaskTrajectoryArtifact
		var kindTruncated bool
		var titleTruncated bool
		var summaryTruncated bool
		if err := rows.Scan(
			&artifact.ID, &artifact.TaskID, &artifact.Kind, &kindTruncated,
			&artifact.Title, &titleTruncated, &artifact.Summary, &summaryTruncated,
			&artifact.CreatedBy, &artifact.CreatedAt, &artifact.UpdatedAt,
		); err != nil {
			return nil, false, err
		}
		artifact.URL = "/api/v1/artifacts/" + artifact.ID
		artifact.ContentTruncated = kindTruncated || titleTruncated || summaryTruncated
		if !safeTaskTrajectoryIdentifier(artifact.Kind, 128) {
			artifact.Kind = "unknown"
			artifact.ContentTruncated = true
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return artifacts, rowCount > maxTrajectoryArtifacts, nil
}

func trajectoryTruncationWarnings(truncation taskTrajectoryTruncation) []TaskTrajectoryWarning {
	warnings := []TaskTrajectoryWarning{}
	if truncation.tasks {
		warnings = append(warnings, TaskTrajectoryWarning{Code: "task_limit_reached", Message: fmt.Sprintf("task tree exceeded the %d-task export limit", maxTrajectoryTasks)})
	}
	if truncation.runLinks {
		warnings = append(warnings, TaskTrajectoryWarning{Code: "run_link_limit_reached", Message: fmt.Sprintf("run-to-task links exceeded the %d-row export limit; a boundary run may have partial links and some runs may be omitted", maxTrajectoryRunLinkRows)})
	}
	if truncation.events {
		warnings = append(warnings, TaskTrajectoryWarning{Code: "event_limit_reached", Message: fmt.Sprintf("run events exceeded the %d-event export limit; a boundary run may have partial events and some events may be omitted", maxTrajectoryEvents)})
	}
	if truncation.artifacts {
		warnings = append(warnings, TaskTrajectoryWarning{Code: "artifact_limit_reached", Message: fmt.Sprintf("artifact metadata exceeded the %d-record export limit", maxTrajectoryArtifacts)})
	}
	return warnings
}

func attachTaskTrajectoryTranscriptReference(run *TaskTrajectoryRun, source taskTrajectoryRunSource, capturedAt time.Time) []TaskTrajectoryWarning {
	warnings := []TaskTrajectoryWarning{}
	windowEnd := capturedAt
	if run.FinishedAt != nil {
		windowEnd = *run.FinishedAt
	}
	run.Transcript = TaskTrajectoryTranscript{
		Status:       "reference_recorded",
		Association:  "unverified_time_window",
		WindowStart:  run.StartedAt.Add(-2 * time.Second),
		WindowEnd:    windowEnd.Add(2 * time.Second),
		TextExported: false,
	}
	if run.FinishedAt == nil {
		warnings = append(warnings, TaskTrajectoryWarning{
			Code:    "run_in_progress",
			RunID:   run.ID,
			Message: "run was not finished at the database snapshot time",
		})
	}
	if !source.referenceRecorded {
		run.Transcript.Status = "missing"
		warnings = append(warnings, TaskTrajectoryWarning{
			Code:    "transcript_reference_missing",
			RunID:   run.ID,
			Message: "no transcript source reference is stored for this run",
		})
	}
	return warnings
}

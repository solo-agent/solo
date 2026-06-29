package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	agentruntime "github.com/solo-ai/solo/pkg/agent"
)

type AgentRunStatus string

const (
	AgentRunStatusQueued          AgentRunStatus = "queued"
	AgentRunStatusThinking        AgentRunStatus = "thinking"
	AgentRunStatusRunning         AgentRunStatus = "running"
	AgentRunStatusStreaming       AgentRunStatus = "streaming"
	AgentRunStatusWaitingInput    AgentRunStatus = "waiting_input"
	AgentRunStatusWaitingApproval AgentRunStatus = "waiting_approval"
	AgentRunStatusCompleted       AgentRunStatus = "completed"
	AgentRunStatusFailed          AgentRunStatus = "failed"
	AgentRunStatusCancelled       AgentRunStatus = "cancelled"
	AgentRunStatusTimeout         AgentRunStatus = "timeout"
)

const (
	AgentRunTriggerMessage  = "message"
	AgentRunTriggerTask     = "task"
	AgentRunTriggerManual   = "manual"
	AgentRunTriggerSchedule = "schedule"
)

const (
	AgentRunTaskRolePrimary = "primary"
	AgentRunTaskRoleRelated = "related"
)

const (
	AgentRunEventUserMessageReceived = "user_message_received"
	AgentRunEventRunStarted          = "run_started"
	AgentRunEventThinking            = "thinking"
	AgentRunEventActivity            = "activity"
	AgentRunEventToolStarted         = "tool_started"
	AgentRunEventToolFinished        = "tool_finished"
	AgentRunEventAssistantMessage    = "assistant_message"
	AgentRunEventTaskLinked          = "task_linked"
	AgentRunEventUsage               = "usage"
	AgentRunEventDone                = "done"
	AgentRunEventError               = "error"
)

const agentRunEventTextLimit = 2048

var nonPrimaryTaskRunStatuses = []string{
	string(AgentRunStatusFailed),
	string(AgentRunStatusTimeout),
	string(AgentRunStatusCancelled),
}

type AgentSession struct {
	ID                string    `json:"id"`
	AgentID           string    `json:"agent_id"`
	Provider          string    `json:"provider"`
	ExternalSessionID string    `json:"external_session_id,omitempty"`
	TranscriptPath    string    `json:"transcript_path,omitempty"`
	Title             string    `json:"title,omitempty"`
	Status            string    `json:"status"`
	StartedAt         time.Time `json:"started_at"`
	LastActiveAt      time.Time `json:"last_active_at"`
}

type AgentRun struct {
	ID               string          `json:"id"`
	AgentID          string          `json:"agent_id"`
	AgentName        string          `json:"agent_name,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	TriggerType      string          `json:"trigger_type"`
	TriggerMessageID string          `json:"trigger_message_id,omitempty"`
	ChannelID        string          `json:"channel_id,omitempty"`
	ThreadID         string          `json:"thread_id,omitempty"`
	Status           AgentRunStatus  `json:"status"`
	ActivityText     string          `json:"activity_text"`
	ToolName         string          `json:"tool_name,omitempty"`
	ToolInputSummary string          `json:"tool_input_summary,omitempty"`
	Source           string          `json:"source,omitempty"`
	TranscriptPath   string          `json:"transcript_path,omitempty"`
	UsageJSON        json.RawMessage `json:"usage_json"`
	StartedAt        time.Time       `json:"started_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
}

type AgentRunEvent struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Seq       int             `json:"seq"`
	Type      string          `json:"type"`
	Message   string          `json:"message"`
	ToolName  string          `json:"tool_name,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type AgentTaskSummary struct {
	ID             string     `json:"id"`
	TaskNumber     int        `json:"task_number"`
	ChannelID      string     `json:"channel_id,omitempty"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	LastRunID      string     `json:"last_run_id"`
	LastRunStatus  string     `json:"last_run_status"`
	LastActivity   string     `json:"last_activity"`
	LastRunAt      time.Time  `json:"last_run_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	LinkedRunCount int        `json:"linked_run_count"`
}

type UpsertSessionInput struct {
	AgentID           string
	Provider          string
	ExternalSessionID string
	TranscriptPath    string
	Title             string
}

type CreateOrResumeSessionInput = UpsertSessionInput

type UpdateSessionMetadataInput struct {
	SessionID         string
	ExternalSessionID string
	TranscriptPath    string
	Title             string
}

type StartRunInput struct {
	AgentID          string
	SessionID        string
	TriggerType      string
	TriggerMessageID string
	ChannelID        string
	ThreadID         string
	Status           AgentRunStatus
	ActivityText     string
	ToolName         string
	ToolInputSummary string
	Source           string
	Usage            any
}

type AppendRunEventInput struct {
	RunID    string
	Type     string
	Message  string
	ToolName string
	Payload  any
}

type UpdateRunStatusInput struct {
	RunID            string
	Status           AgentRunStatus
	ActivityText     string
	ToolName         string
	ToolInputSummary string
	Source           string
	Usage            any
}

type UpdateRunTranscriptInput struct {
	RunID          string
	TranscriptPath string
}

type BindRunSessionInput struct {
	RunID     string
	SessionID string
}

type LinkRunTaskInput struct {
	RunID      string
	TaskID     string
	Role       string
	Confidence float64
}

type FinishRunInput struct {
	RunID        string
	Status       AgentRunStatus
	ActivityText string
	Usage        any
}

type AgentRunService struct {
	pool *pgxpool.Pool
}

func NewAgentRunService(pool *pgxpool.Pool) *AgentRunService {
	return &AgentRunService{pool: pool}
}

func (s *AgentRunService) UpsertSession(ctx context.Context, input UpsertSessionInput) (*AgentSession, error) {
	if input.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if input.Provider == "" {
		return nil, fmt.Errorf("provider is required")
	}

	externalSessionID := strings.TrimSpace(input.ExternalSessionID)
	transcriptPath := strings.TrimSpace(input.TranscriptPath)
	if externalSessionID == "" && transcriptPath != "" {
		externalSessionID = stableTranscriptSessionID(transcriptPath)
	}
	if externalSessionID == "" {
		return nil, fmt.Errorf("external_session_id or transcript_path is required")
	}

	return scanAgentSession(s.pool.QueryRow(ctx,
		`INSERT INTO agent_sessions (id, agent_id, provider, external_session_id, transcript_path, title, last_active_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 ON CONFLICT (agent_id, provider, external_session_id)
		 WHERE external_session_id IS NOT NULL
		 DO UPDATE SET
		   transcript_path = COALESCE(EXCLUDED.transcript_path, agent_sessions.transcript_path),
		   title = COALESCE(EXCLUDED.title, agent_sessions.title),
		   status = 'active',
		   last_active_at = now()
		 RETURNING id::text, agent_id::text, provider, COALESCE(external_session_id, ''),
		       COALESCE(transcript_path, ''), COALESCE(title, ''), status, started_at, last_active_at`,
		uuid.NewString(), input.AgentID, input.Provider, externalSessionID,
		nullableStr(transcriptPath), nullableStr(input.Title),
	))
}

func (s *AgentRunService) CreateOrResumeSession(ctx context.Context, input CreateOrResumeSessionInput) (*AgentSession, error) {
	return s.UpsertSession(ctx, input)
}

func (s *AgentRunService) UpdateSessionMetadata(ctx context.Context, input UpdateSessionMetadataInput) (*AgentSession, error) {
	if input.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	return scanAgentSession(s.pool.QueryRow(ctx,
		`UPDATE agent_sessions
		    SET external_session_id = COALESCE($2, external_session_id),
		        transcript_path = COALESCE($3, transcript_path),
		        title = COALESCE($4, title),
		        last_active_at = now()
		  WHERE id = $1
		  RETURNING id::text, agent_id::text, provider, COALESCE(external_session_id, ''),
		        COALESCE(transcript_path, ''), COALESCE(title, ''), status, started_at, last_active_at`,
		input.SessionID, nullableStr(input.ExternalSessionID), nullableStr(input.TranscriptPath), nullableStr(input.Title),
	))
}

func stableTranscriptSessionID(transcriptPath string) string {
	clean := filepath.Clean(strings.TrimSpace(transcriptPath))
	if clean == "" || clean == "." {
		return ""
	}
	sum := sha256.Sum256([]byte(clean))
	base := filepath.Base(clean)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." {
		base = "transcript"
	}
	return "path:" + base + ":" + hex.EncodeToString(sum[:])[:16]
}

func (s *AgentRunService) StartRun(ctx context.Context, input StartRunInput) (*AgentRun, error) {
	if input.Status == "" {
		input.Status = AgentRunStatusQueued
	}
	usage, err := marshalJSON(input.Usage)
	if err != nil {
		return nil, err
	}
	return scanAgentRun(s.pool.QueryRow(ctx,
		`INSERT INTO agent_runs (
		   id, agent_id, session_id, trigger_type, trigger_message_id, channel_id, thread_id,
		   status, activity_text, tool_name, tool_input_summary, source, usage_json
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id::text, agent_id::text,
		       COALESCE((SELECT name FROM agents WHERE id = agent_runs.agent_id), ''),
		       COALESCE(session_id::text, ''), trigger_type,
		       COALESCE(trigger_message_id::text, ''), COALESCE(channel_id::text, ''),
		       COALESCE(thread_id::text, ''), status, activity_text, COALESCE(tool_name, ''),
		       COALESCE(tool_input_summary, ''), COALESCE(source, ''), COALESCE(transcript_path, ''), usage_json,
		       started_at, updated_at, finished_at`,
		uuid.NewString(), input.AgentID, nullableUUID(input.SessionID), input.TriggerType,
		nullableUUID(input.TriggerMessageID), nullableUUID(input.ChannelID), nullableUUID(input.ThreadID),
		string(input.Status), input.ActivityText, nullableStr(input.ToolName),
		nullableStr(input.ToolInputSummary), nullableStr(input.Source), usage,
	))
}

func (s *AgentRunService) BindRunSession(ctx context.Context, input BindRunSessionInput) (*AgentRun, error) {
	if input.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if input.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	return scanAgentRun(s.pool.QueryRow(ctx,
		`UPDATE agent_runs
		    SET session_id = $2,
		        status = CASE WHEN status = 'queued' THEN 'running' ELSE status END,
		        activity_text = CASE WHEN status = 'queued' THEN '执行中' ELSE activity_text END,
		        updated_at = now()
		  WHERE id = $1
		  RETURNING id::text, agent_id::text,
		        COALESCE((SELECT name FROM agents WHERE id = agent_runs.agent_id), ''),
		        COALESCE(session_id::text, ''), trigger_type,
		        COALESCE(trigger_message_id::text, ''), COALESCE(channel_id::text, ''),
		        COALESCE(thread_id::text, ''), status, activity_text, COALESCE(tool_name, ''),
		        COALESCE(tool_input_summary, ''), COALESCE(source, ''), COALESCE(transcript_path, ''),
		        usage_json, started_at, updated_at, finished_at`,
		input.RunID, input.SessionID,
	))
}

func (s *AgentRunService) AppendEvent(ctx context.Context, input AppendRunEventInput) (*AgentRunEvent, error) {
	payload, err := marshalJSON(slimRunEventPayload(input.Payload))
	if err != nil {
		return nil, err
	}
	return scanAgentRunEvent(s.pool.QueryRow(ctx,
		`INSERT INTO agent_run_events (id, run_id, seq, type, message, tool_name, payload)
		 SELECT $1, $2, COALESCE(MAX(seq), 0) + 1, $3, $4, $5, $6
		   FROM agent_run_events
		  WHERE run_id = $2
		 RETURNING id::text, run_id::text, seq, type, message, COALESCE(tool_name, ''), payload, created_at`,
		uuid.NewString(), input.RunID, input.Type, slimRunEventText(input.Message), nullableStr(input.ToolName), payload,
	))
}

func (s *AgentRunService) UpdateStatus(ctx context.Context, input UpdateRunStatusInput) (*AgentRun, error) {
	usage, err := marshalJSON(input.Usage)
	if err != nil {
		return nil, err
	}
	return scanAgentRun(s.pool.QueryRow(ctx,
		`UPDATE agent_runs
		    SET status = $2,
		        activity_text = $3,
		        tool_name = $4,
		        tool_input_summary = $5,
		        source = COALESCE($6, source),
		        usage_json = CASE WHEN $7::jsonb = '{}'::jsonb THEN usage_json ELSE $7::jsonb END,
		        updated_at = now()
		  WHERE id = $1
		  RETURNING id::text, agent_id::text,
		        COALESCE((SELECT name FROM agents WHERE id = agent_runs.agent_id), ''),
		        COALESCE(session_id::text, ''), trigger_type,
		        COALESCE(trigger_message_id::text, ''), COALESCE(channel_id::text, ''),
		        COALESCE(thread_id::text, ''), status, activity_text, COALESCE(tool_name, ''),
		        COALESCE(tool_input_summary, ''), COALESCE(source, ''), COALESCE(transcript_path, ''), usage_json,
		        started_at, updated_at, finished_at`,
		input.RunID, string(input.Status), input.ActivityText, nullableStr(input.ToolName),
		nullableStr(input.ToolInputSummary), nullableStr(input.Source), usage,
	))
}

func (s *AgentRunService) UpdateRunTranscript(ctx context.Context, input UpdateRunTranscriptInput) (*AgentRun, error) {
	if input.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	return scanAgentRun(s.pool.QueryRow(ctx,
		`UPDATE agent_runs
		    SET transcript_path = COALESCE($2, transcript_path),
		        updated_at = now()
		  WHERE id = $1
		  RETURNING id::text, agent_id::text,
		        COALESCE((SELECT name FROM agents WHERE id = agent_runs.agent_id), ''),
		        COALESCE(session_id::text, ''), trigger_type,
		        COALESCE(trigger_message_id::text, ''), COALESCE(channel_id::text, ''),
		        COALESCE(thread_id::text, ''), status, activity_text, COALESCE(tool_name, ''),
		        COALESCE(tool_input_summary, ''), COALESCE(source, ''), COALESCE(transcript_path, ''),
		        usage_json, started_at, updated_at, finished_at`,
		input.RunID, nullableStr(input.TranscriptPath),
	))
}

func (s *AgentRunService) LinkTask(ctx context.Context, input LinkRunTaskInput) error {
	if input.Role == "" {
		input.Role = AgentRunTaskRolePrimary
	}
	if input.Confidence == 0 {
		input.Confidence = 1
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_run_task_links (run_id, task_id, role, confidence)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (run_id, task_id)
		 DO UPDATE SET role = EXCLUDED.role, confidence = EXCLUDED.confidence`,
		input.RunID, input.TaskID, input.Role, input.Confidence,
	)
	return err
}

func (s *AgentRunService) FinishRun(ctx context.Context, input FinishRunInput) (*AgentRun, error) {
	usage, err := marshalJSON(input.Usage)
	if err != nil {
		return nil, err
	}
	return scanAgentRun(s.pool.QueryRow(ctx,
		`UPDATE agent_runs
		    SET status = $2,
		        activity_text = $3,
		        usage_json = CASE WHEN $4::jsonb = '{}'::jsonb THEN usage_json ELSE $4::jsonb END,
		        updated_at = now(),
		        finished_at = now()
		  WHERE id = $1
		  RETURNING id::text, agent_id::text,
		        COALESCE((SELECT name FROM agents WHERE id = agent_runs.agent_id), ''),
		        COALESCE(session_id::text, ''), trigger_type,
		        COALESCE(trigger_message_id::text, ''), COALESCE(channel_id::text, ''),
		        COALESCE(thread_id::text, ''), status, activity_text, COALESCE(tool_name, ''),
		        COALESCE(tool_input_summary, ''), COALESCE(source, ''), COALESCE(transcript_path, ''), usage_json,
		        started_at, updated_at, finished_at`,
		input.RunID, string(input.Status), input.ActivityText, usage,
	))
}

func (s *AgentRunService) GetRun(ctx context.Context, runID string) (*AgentRun, error) {
	return scanAgentRun(s.pool.QueryRow(ctx, baseAgentRunSelect()+` WHERE r.id = $1`, runID))
}

func (s *AgentRunService) ListActiveRuns(ctx context.Context) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.status = ANY($1)
		 ORDER BY r.updated_at DESC
		 LIMIT 100`,
		[]string{
			string(AgentRunStatusQueued),
			string(AgentRunStatusThinking),
			string(AgentRunStatusRunning),
			string(AgentRunStatusStreaming),
			string(AgentRunStatusWaitingInput),
			string(AgentRunStatusWaitingApproval),
		},
	))
}

func (s *AgentRunService) ListRecentRuns(ctx context.Context) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 ORDER BY r.updated_at DESC
		 LIMIT 100`))
}

func (s *AgentRunService) ListRunsByAgent(ctx context.Context, agentID string) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.agent_id = $1
		 ORDER BY r.started_at DESC
		 LIMIT 100`, agentID))
}

func (s *AgentRunService) ListRunsByTask(ctx context.Context, taskID string) ([]AgentRun, error) {
	return scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE r.id IN (SELECT run_id FROM agent_run_task_links WHERE task_id = $1)
		   AND (
		     NOT (r.status = ANY($2))
		     OR NOT EXISTS (
		       SELECT 1
		         FROM agent_runs r2
		        WHERE r2.id IN (SELECT run_id FROM agent_run_task_links WHERE task_id = $1)
		          AND NOT (r2.status = ANY($2))
		     )
		   )
		 ORDER BY r.started_at DESC
		 LIMIT 100`, taskID, nonPrimaryTaskRunStatuses))
}

func (s *AgentRunService) ListSessionsByAgent(ctx context.Context, agentID string) ([]AgentSession, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, agent_id::text, provider, COALESCE(external_session_id, ''),
		        COALESCE(transcript_path, ''), COALESCE(title, ''), status, started_at, last_active_at
		   FROM agent_sessions
		  WHERE agent_id = $1
		  ORDER BY last_active_at DESC
		  LIMIT 100`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []AgentSession
	for rows.Next() {
		var session AgentSession
		if err := rows.Scan(&session.ID, &session.AgentID, &session.Provider, &session.ExternalSessionID, &session.TranscriptPath, &session.Title, &session.Status, &session.StartedAt, &session.LastActiveAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *AgentRunService) ListEvents(ctx context.Context, runID string) ([]AgentRunEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, run_id::text, seq, type, message, COALESCE(tool_name, ''), payload, created_at
		   FROM agent_run_events
		  WHERE run_id = $1
		  ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AgentRunEvent
	for rows.Next() {
		event, err := scanAgentRunEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

func (s *AgentRunService) GetRunTranscript(ctx context.Context, runID string, limit int) ([]AgentTranscriptEntry, error) {
	var path string
	var agentID string
	var provider string
	var externalSessionID string
	var startedAt time.Time
	var finished sql.NullTime
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(r.transcript_path, sess.transcript_path, ''),
		        r.agent_id::text, COALESCE(sess.provider, r.source, ''),
		        COALESCE(sess.external_session_id, ''),
		        r.started_at, r.finished_at
		   FROM agent_runs r
		   LEFT JOIN agent_sessions sess ON sess.id = r.session_id
		  WHERE r.id = $1`,
		runID,
	).Scan(&path, &agentID, &provider, &externalSessionID, &startedAt, &finished)
	if err != nil {
		return nil, err
	}
	if livePath := liveTranscriptPath(provider, agentID, externalSessionID); livePath != "" {
		path = livePath
	}
	end := time.Now().UTC()
	if finished.Valid {
		end = finished.Time
	}
	if provider == "hermes" && externalSessionID != "" {
		return ReadHermesTranscriptWindow(externalSessionID, startedAt.Add(-2*time.Second), end.Add(2*time.Second), limit)
	}
	return ReadAgentTranscriptWindow(path, startedAt.Add(-2*time.Second), end.Add(2*time.Second), limit)
}

func liveTranscriptPath(provider, agentID, externalSessionID string) string {
	if provider == "" || externalSessionID == "" {
		return ""
	}
	workspaceDir := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" && agentID != "" {
		workspaceDir = filepath.Join(home, ".solo", "agents", agentID, "workspace")
	}
	path := agentruntime.TranscriptPath(provider, workspaceDir, externalSessionID)
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func (s *AgentRunService) ListAgentTasks(ctx context.Context, agentID string) ([]AgentTaskSummary, error) {
	rows, err := s.pool.Query(ctx,
		`WITH ranked AS (
		   SELECT t.id::text AS task_id, COALESCE(t.task_number, 0) AS task_number,
		          COALESCE(t.channel_id::text, '') AS channel_id,
		          t.title, t.status AS task_status, r.id::text AS run_id,
		          r.status AS run_status, r.activity_text,
		          r.updated_at, r.finished_at,
		          COUNT(*) OVER (PARTITION BY t.id) AS linked_run_count,
		          COUNT(*) FILTER (WHERE NOT (r.status = ANY($2))) OVER (PARTITION BY t.id) AS effective_run_count,
		          ROW_NUMBER() OVER (
		            PARTITION BY t.id
		            ORDER BY CASE WHEN r.status = ANY($2) THEN 1 ELSE 0 END, r.updated_at DESC
		          ) AS rn
		     FROM tasks t
		     JOIN agent_run_task_links l ON l.task_id = t.id
		     JOIN agent_runs r ON r.id = l.run_id
		    WHERE r.agent_id = $1
		 )
		 SELECT task_id, task_number, channel_id, title, task_status, run_id, run_status,
		        activity_text, updated_at, finished_at,
		        CASE WHEN effective_run_count > 0 THEN effective_run_count ELSE linked_run_count END
		   FROM ranked
		  WHERE rn = 1
		  ORDER BY updated_at DESC
		  LIMIT 100`, agentID, nonPrimaryTaskRunStatuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []AgentTaskSummary
	for rows.Next() {
		var item AgentTaskSummary
		var finished sql.NullTime
		if err := rows.Scan(&item.ID, &item.TaskNumber, &item.ChannelID, &item.Title, &item.Status, &item.LastRunID, &item.LastRunStatus, &item.LastActivity, &item.LastRunAt, &finished, &item.LinkedRunCount); err != nil {
			return nil, err
		}
		if finished.Valid {
			item.CompletedAt = &finished.Time
		}
		tasks = append(tasks, item)
	}
	return tasks, rows.Err()
}

func baseAgentRunSelect() string {
	return `SELECT r.id::text, r.agent_id::text, COALESCE(a.name, ''), COALESCE(r.session_id::text, ''), r.trigger_type,
	        COALESCE(r.trigger_message_id::text, ''), COALESCE(r.channel_id::text, ''),
	        COALESCE(r.thread_id::text, ''), r.status, r.activity_text, COALESCE(r.tool_name, ''),
	        COALESCE(r.tool_input_summary, ''), COALESCE(r.source, ''), COALESCE(r.transcript_path, ''), r.usage_json,
	        r.started_at, r.updated_at, r.finished_at
	   FROM agent_runs r
	   LEFT JOIN agents a ON a.id = r.agent_id`
}

func scanAgentRuns(rows pgx.Rows, err error) ([]AgentRun, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []AgentRun
	for rows.Next() {
		run, err := scanAgentRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

func scanAgentSession(row interface {
	Scan(dest ...any) error
}) (*AgentSession, error) {
	var s AgentSession
	if err := row.Scan(&s.ID, &s.AgentID, &s.Provider, &s.ExternalSessionID, &s.TranscriptPath, &s.Title, &s.Status, &s.StartedAt, &s.LastActiveAt); err != nil {
		return nil, err
	}
	return &s, nil
}

func scanAgentRun(row interface {
	Scan(dest ...any) error
}) (*AgentRun, error) {
	var run AgentRun
	var status string
	var finished sql.NullTime
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.AgentName, &run.SessionID, &run.TriggerType, &run.TriggerMessageID,
		&run.ChannelID, &run.ThreadID, &status, &run.ActivityText, &run.ToolName,
		&run.ToolInputSummary, &run.Source, &run.TranscriptPath, &run.UsageJSON,
		&run.StartedAt, &run.UpdatedAt, &finished,
	); err != nil {
		return nil, err
	}
	run.Status = AgentRunStatus(status)
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	return &run, nil
}

func scanAgentRunEvent(row interface {
	Scan(dest ...any) error
}) (*AgentRunEvent, error) {
	var event AgentRunEvent
	if err := row.Scan(&event.ID, &event.RunID, &event.Seq, &event.Type, &event.Message, &event.ToolName, &event.Payload, &event.CreatedAt); err != nil {
		return nil, err
	}
	return &event, nil
}

func nullableUUID(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func marshalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(v)
}

func slimRunEventPayload(v any) any {
	switch value := v.(type) {
	case map[string]any:
		next := make(map[string]any, len(value))
		for key, item := range value {
			next[key] = slimRunEventPayload(item)
		}
		return next
	case []any:
		next := make([]any, len(value))
		for i, item := range value {
			next[i] = slimRunEventPayload(item)
		}
		return next
	case string:
		return slimRunEventText(value)
	default:
		return value
	}
}

func slimRunEventText(value string) string {
	runes := []rune(value)
	if len(runes) <= agentRunEventTextLimit {
		return value
	}
	return string(runes[:agentRunEventTextLimit]) + fmt.Sprintf("\n[truncated %d chars]", len(runes)-agentRunEventTextLimit)
}

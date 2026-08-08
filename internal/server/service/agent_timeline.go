package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

type AgentTimeline struct {
	Scope   string                 `json:"scope"`
	ID      string                 `json:"id"`
	Session *AgentSession          `json:"session,omitempty"`
	Task    *AgentTimelineTask     `json:"task,omitempty"`
	Runs    []AgentTimelineRun     `json:"runs"`
	Entries []AgentTranscriptEntry `json:"entries"`
}

type AgentTimelineTask struct {
	ID         string `json:"id"`
	TaskNumber int    `json:"task_number"`
	ChannelID  string `json:"channel_id,omitempty"`
	Title      string `json:"title"`
	Status     string `json:"status"`
}

type AgentTimelineRun struct {
	ID             string         `json:"id"`
	AgentID        string         `json:"agent_id"`
	SessionID      string         `json:"session_id,omitempty"`
	Status         AgentRunStatus `json:"status"`
	ActivityText   string         `json:"activity_text"`
	Source         string         `json:"source,omitempty"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	EntryStartSeq  int            `json:"entry_start_seq,omitempty"`
	EntryEndSeq    int            `json:"entry_end_seq,omitempty"`
	TranscriptPath string         `json:"transcript_path,omitempty"`
}

func (s *AgentRunService) GetSessionTimeline(ctx context.Context, sessionID string, limit int) (*AgentTimeline, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	session, err := scanAgentSession(s.pool.QueryRow(ctx,
		`SELECT id::text, agent_id::text, provider, COALESCE(external_session_id, ''),
		        COALESCE(transcript_path, ''), COALESCE(title, ''), status, started_at, last_active_at
		   FROM agent_sessions
		  WHERE id = $1`, sessionID))
	if err != nil {
		return nil, err
	}
	runs, err := scanAgentRuns(s.pool.Query(ctx, baseAgentRunSelect()+`
		 WHERE session_id = $1
		 ORDER BY started_at ASC, id ASC`, sessionID))
	if err != nil {
		return nil, err
	}
	transcriptPath := session.TranscriptPath
	for _, run := range runs {
		transcriptPath = firstNonEmptyString(transcriptPath, run.TranscriptPath)
		if transcriptPath != "" {
			break
		}
	}
	computerID := ""
	if s.dm != nil {
		err = s.pool.QueryRow(ctx, `SELECT COALESCE(computer_id::text, '') FROM agent_runs WHERE session_id = $1 AND computer_id IS NOT NULL ORDER BY started_at LIMIT 1`, sessionID).Scan(&computerID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if computerID == "" {
		if livePath := liveTranscriptPath(session.Provider, session.AgentID, session.ExternalSessionID); livePath != "" {
			transcriptPath = livePath
		}
	}
	session.TranscriptPath = transcriptPath
	var entries []AgentTranscriptEntry
	if computerID != "" {
		entries, err = s.readRemoteTranscript(ctx, computerID, session.AgentID, session.Provider, session.ExternalSessionID, time.Time{}, time.Time{}, limit)
	} else if session.Provider == "hermes" && session.ExternalSessionID != "" {
		entries, err = ReadHermesTranscript(session.ExternalSessionID, limit)
	} else {
		entries, err = ReadAgentTranscript(transcriptPath, limit)
	}
	if err != nil {
		return nil, err
	}
	timelineRuns := make([]AgentTimelineRun, 0, len(runs))
	for _, run := range runs {
		item := timelineRunFromAgentRun(run, firstNonEmptyString(run.TranscriptPath, transcriptPath))
		item.EntryStartSeq, item.EntryEndSeq = transcriptSeqWindow(entries, run)
		timelineRuns = append(timelineRuns, item)
	}
	return &AgentTimeline{
		Scope:   "session",
		ID:      session.ID,
		Session: session,
		Runs:    timelineRuns,
		Entries: entries,
	}, nil
}

func (s *AgentRunService) GetTaskTimeline(ctx context.Context, taskID, agentID string, limit int) (*AgentTimeline, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if limit <= 0 {
		limit = 1000
	}
	task, err := s.getTimelineTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	runRows, err := s.listTimelineTaskRuns(ctx, taskID, agentID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	entries := []AgentTranscriptEntry{}
	timelineRuns := make([]AgentTimelineRun, 0, len(runRows))
	for _, row := range runRows {
		transcriptPath := row.transcriptPath
		if row.computerID == "" {
			if livePath := liveTranscriptPath(row.provider, row.run.AgentID, row.externalSessionID); livePath != "" {
				transcriptPath = livePath
			}
		}
		item := timelineRunFromAgentRun(row.run, transcriptPath)
		windowStart := row.windowStart
		if windowStart.IsZero() {
			windowStart = row.run.StartedAt
		}
		var windowEntries []AgentTranscriptEntry
		if row.computerID != "" {
			windowEntries, err = s.readRemoteTranscript(ctx, row.computerID, row.run.AgentID, row.provider, row.externalSessionID, windowStart, runWindowEnd(row.run).Add(2*time.Second), limit)
		} else if row.provider == "hermes" && row.externalSessionID != "" {
			windowEntries, err = ReadHermesTranscriptWindow(row.externalSessionID, windowStart, runWindowEnd(row.run).Add(2*time.Second), limit)
		} else {
			var resolvedPath string
			resolvedPath, windowEntries, err = transcriptWindowEntries(transcriptPath, windowStart, runWindowEnd(row.run).Add(2*time.Second), limit)
			item.TranscriptPath = resolvedPath
		}
		if err != nil {
			return nil, err
		}
		var startSeq, endSeq int
		entries, startSeq, endSeq = appendTimelineEntries(entries, windowEntries, seen, limit)
		item.EntryStartSeq = startSeq
		item.EntryEndSeq = endSeq
		timelineRuns = append(timelineRuns, item)
	}
	return &AgentTimeline{
		Scope:   "task",
		ID:      task.ID,
		Task:    task,
		Runs:    timelineRuns,
		Entries: entries,
	}, nil
}

func (s *AgentRunService) getTimelineTask(ctx context.Context, taskID string) (*AgentTimelineTask, error) {
	var task AgentTimelineTask
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, COALESCE(task_number, 0), COALESCE(channel_id::text, ''), title, status
		   FROM tasks
		  WHERE id = $1`, taskID,
	).Scan(&task.ID, &task.TaskNumber, &task.ChannelID, &task.Title, &task.Status)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

type timelineTaskRunRow struct {
	run               AgentRun
	transcriptPath    string
	provider          string
	externalSessionID string
	windowStart       time.Time
	computerID        string
}

func (s *AgentRunService) listTimelineTaskRuns(ctx context.Context, taskID, agentID string) ([]timelineTaskRunRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.id::text, r.agent_id::text, COALESCE(r.session_id::text, ''), r.trigger_type,
		        COALESCE(r.trigger_message_id::text, ''), COALESCE(r.channel_id::text, ''),
		        COALESCE(r.thread_id::text, ''), r.status, r.activity_text, COALESCE(r.tool_name, ''),
		        COALESCE(r.tool_input_summary, ''), COALESCE(r.source, ''),
		        COALESCE(r.transcript_path, ''), r.usage_json, r.started_at, r.updated_at, r.finished_at,
		        COALESCE(r.transcript_path, s.transcript_path, ''), COALESCE(s.provider, r.source, ''),
		        COALESCE(s.external_session_id, ''), COALESCE(m.created_at, r.started_at), COALESCE(r.computer_id::text, '')
		   FROM agent_runs r
		   JOIN agent_run_task_links l ON l.run_id = r.id
		   LEFT JOIN agent_sessions s ON s.id = r.session_id
		   LEFT JOIN messages m ON m.id = r.trigger_message_id
		  WHERE l.task_id = $1
		    AND ($2 = '' OR r.agent_id = $2::uuid)
		  ORDER BY r.started_at ASC, r.id ASC`, taskID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []timelineTaskRunRow
	for rows.Next() {
		var item timelineTaskRunRow
		var status string
		var finished sql.NullTime
		if err := rows.Scan(
			&item.run.ID, &item.run.AgentID, &item.run.SessionID, &item.run.TriggerType,
			&item.run.TriggerMessageID, &item.run.ChannelID, &item.run.ThreadID, &status,
			&item.run.ActivityText, &item.run.ToolName, &item.run.ToolInputSummary,
			&item.run.Source, &item.run.TranscriptPath, &item.run.UsageJSON,
			&item.run.StartedAt, &item.run.UpdatedAt, &finished, &item.transcriptPath,
			&item.provider, &item.externalSessionID, &item.windowStart, &item.computerID,
		); err != nil {
			return nil, err
		}
		item.run.Status = AgentRunStatus(status)
		if finished.Valid {
			item.run.FinishedAt = &finished.Time
		}
		if item.transcriptPath == "" {
			item.transcriptPath = item.run.TranscriptPath
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func timelineRunFromAgentRun(run AgentRun, transcriptPath string) AgentTimelineRun {
	return AgentTimelineRun{
		ID:             run.ID,
		AgentID:        run.AgentID,
		SessionID:      run.SessionID,
		Status:         run.Status,
		ActivityText:   run.ActivityText,
		Source:         run.Source,
		StartedAt:      run.StartedAt,
		FinishedAt:     run.FinishedAt,
		TranscriptPath: transcriptPath,
	}
}

func transcriptWindowEntries(path string, anchor, end time.Time, limit int) (string, []AgentTranscriptEntry, error) {
	if path == "" {
		return path, []AgentTranscriptEntry{}, nil
	}
	entries, err := cachedTranscriptEntries(path)
	if err != nil {
		return path, nil, err
	}
	start := boundedTranscriptPromptWindowStart(entries, anchor)
	window := filterTranscriptEntries(entries, start.Add(-2*time.Second), end, limit)
	if len(window) > 0 || path == "" {
		return path, window, nil
	}
	return siblingTranscriptWindow(path, anchor, end, limit)
}

func siblingTranscriptWindow(path string, anchor, end time.Time, limit int) (string, []AgentTranscriptEntry, error) {
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.jsonl"))
	if err != nil {
		return path, nil, err
	}
	for _, candidate := range matches {
		if candidate == path {
			continue
		}
		entries, err := cachedTranscriptEntries(candidate)
		if err != nil {
			return path, nil, err
		}
		start := boundedTranscriptPromptWindowStart(entries, anchor)
		window := filterTranscriptEntries(entries, start.Add(-2*time.Second), end, limit)
		if len(window) > 0 {
			return candidate, window, nil
		}
	}
	return path, []AgentTranscriptEntry{}, nil
}

func boundedTranscriptPromptWindowStart(entries []AgentTranscriptEntry, anchor time.Time) time.Time {
	start := transcriptPromptWindowStart(entries, anchor)
	if start.Before(anchor.Add(-30 * time.Minute)) {
		return anchor
	}
	return start
}

func transcriptPromptWindowStart(entries []AgentTranscriptEntry, anchor time.Time) time.Time {
	start := anchor
	for _, entry := range entries {
		if entry.Role != "user" || entry.Type != "text" || entry.Text == "" || entry.Timestamp == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil || ts.After(anchor) {
			continue
		}
		start = ts
	}
	return start
}

func transcriptSeqWindow(entries []AgentTranscriptEntry, run AgentRun) (int, int) {
	start := run.StartedAt.Add(-2 * time.Second)
	end := runWindowEnd(run).Add(2 * time.Second)
	var startSeq, endSeq int
	for _, entry := range entries {
		if !entryInWindow(entry, start, end) {
			continue
		}
		if startSeq == 0 {
			startSeq = entry.Seq
		}
		endSeq = entry.Seq
	}
	return startSeq, endSeq
}

func runWindowEnd(run AgentRun) time.Time {
	if run.FinishedAt != nil {
		return *run.FinishedAt
	}
	return time.Now().UTC()
}

func appendTimelineEntries(dst []AgentTranscriptEntry, entries []AgentTranscriptEntry, seen map[string]bool, limit int) ([]AgentTranscriptEntry, int, int) {
	var startSeq, endSeq int
	for _, entry := range entries {
		if len(dst) >= limit {
			break
		}
		key := transcriptEntryKey(entry)
		if seen[key] {
			continue
		}
		seen[key] = true
		entry.Seq = len(dst) + 1
		if startSeq == 0 {
			startSeq = entry.Seq
		}
		endSeq = entry.Seq
		dst = append(dst, entry)
	}
	return dst, startSeq, endSeq
}

func transcriptEntryKey(entry AgentTranscriptEntry) string {
	if len(entry.Raw) > 0 {
		return entry.Timestamp + "|" + entry.Role + "|" + entry.Type + "|" + string(entry.Raw)
	}
	raw, _ := json.Marshal(entry)
	return string(raw)
}

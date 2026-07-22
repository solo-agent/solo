package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"
)

type DashboardLive struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Groups      []DashboardLiveGroup `json:"groups"`
	Totals      DashboardLiveTotals  `json:"totals"`
}

type DashboardLiveTotals struct {
	Agents         int `json:"agents"`
	Working        int `json:"working"`
	NeedsAttention int `json:"needs_attention"`
	IdleRecent     int `json:"idle_recent"`
}

type DashboardLiveGroup struct {
	Key   string               `json:"key"`
	Label string               `json:"label"`
	Count int                  `json:"count"`
	Items []DashboardLiveAgent `json:"items"`
}

type DashboardLiveAgent struct {
	AgentID          string         `json:"agent_id"`
	AgentName        string         `json:"agent_name"`
	AvatarURL        string         `json:"avatar_url,omitempty"`
	IsActive         bool           `json:"is_active"`
	Group            string         `json:"group"`
	RunID            string         `json:"run_id,omitempty"`
	SessionID        string         `json:"session_id,omitempty"`
	TaskID           string         `json:"task_id,omitempty"`
	Status           AgentRunStatus `json:"status,omitempty"`
	ActivityText     string         `json:"activity_text,omitempty"`
	ToolName         string         `json:"tool_name,omitempty"`
	ToolInputSummary string         `json:"tool_input_summary,omitempty"`
	Source           string         `json:"source,omitempty"`
	UpdatedAt        *time.Time     `json:"updated_at,omitempty"`
	ActiveCount      int            `json:"active_count"`
	AttentionCount   int            `json:"attention_count"`
	RunCount         int            `json:"run_count"`
}

type DashboardInsight struct {
	Since       time.Time              `json:"since"`
	GeneratedAt time.Time              `json:"generated_at"`
	Messages    int                    `json:"messages"`
	AgentRuns   int                    `json:"agent_runs"`
	Tasks       int                    `json:"tasks"`
	Tokens      DashboardTokenUsage    `json:"tokens"`
	RunStatus   []DashboardCount       `json:"run_status"`
	TaskStatus  []DashboardCount       `json:"task_status"`
	AgentUsage  []DashboardUsageCount  `json:"agent_usage"`
	TaskUsage   []DashboardTaskUsage   `json:"task_usage"`
	ToolUsage   []DashboardCount       `json:"tool_usage"`
	Terms       []DashboardCount       `json:"terms"`
	Series      []DashboardSeriesPoint `json:"series"`
}

type DashboardTokenUsage struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Total  int64 `json:"total"`
}

type DashboardCount struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type DashboardUsageCount struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Count  int        `json:"count"`
	LastAt *time.Time `json:"last_at,omitempty"`
}

type DashboardTaskUsage struct {
	ID         string     `json:"id"`
	TaskNumber int        `json:"task_number"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Count      int        `json:"count"`
	LastAt     *time.Time `json:"last_at,omitempty"`
}

type DashboardSeriesPoint struct {
	Date      string `json:"date"`
	Messages  int    `json:"messages"`
	AgentRuns int    `json:"agent_runs"`
	Tasks     int    `json:"tasks"`
	Tokens    int64  `json:"tokens"`
}

func (s *AgentRunService) GetDashboardLive(ctx context.Context, ownerID string) (*DashboardLive, error) {
	rows, err := s.pool.Query(ctx,
		`WITH latest_runs AS (
		   SELECT DISTINCT ON (r.agent_id)
		          r.agent_id, r.id::text AS run_id, COALESCE(r.session_id::text, '') AS session_id,
		          r.status, r.activity_text, COALESCE(r.tool_name, '') AS tool_name,
		          COALESCE(r.tool_input_summary, '') AS tool_input_summary,
		          COALESCE(r.source, '') AS source, r.updated_at,
		          COALESCE((
		            SELECT l.task_id::text
		              FROM agent_run_task_links l
		             WHERE l.run_id = r.id
		             ORDER BY l.created_at DESC
		             LIMIT 1
		          ), '') AS task_id
		     FROM agent_runs r
		    ORDER BY r.agent_id,
		             CASE
		               WHEN r.status IN ('thinking', 'running', 'streaming', 'waiting_input', 'waiting_approval') THEN 0
		               WHEN r.status = 'queued' THEN 1
		               ELSE 2
		             END,
		             r.started_at DESC, r.updated_at DESC
		 ),
		 counts AS (
		   SELECT agent_id,
		          COUNT(*) AS run_count,
		          COUNT(*) FILTER (WHERE status IN ('queued', 'thinking', 'running', 'streaming')) AS active_count,
		          COUNT(*) FILTER (WHERE status IN ('waiting_input', 'waiting_approval', 'failed', 'timeout')) AS attention_count
		     FROM agent_runs
		    GROUP BY agent_id
		 )
		 SELECT a.id::text, a.name, COALESCE(a.avatar_url, ''), a.is_active,
		        COALESCE(lr.run_id, ''), COALESCE(lr.session_id, ''), COALESCE(lr.task_id, ''),
		        COALESCE(lr.status, ''), COALESCE(lr.activity_text, ''), COALESCE(lr.tool_name, ''),
		        COALESCE(lr.tool_input_summary, ''), COALESCE(lr.source, ''), lr.updated_at,
		        COALESCE(c.active_count, 0), COALESCE(c.attention_count, 0), COALESCE(c.run_count, 0)
		   FROM agents a
		   LEFT JOIN latest_runs lr ON lr.agent_id = a.id
		   LEFT JOIN counts c ON c.agent_id = a.id
		  WHERE a.owner_id = $1 AND a.is_active = true
		  ORDER BY lr.updated_at DESC NULLS LAST, a.name ASC
		  LIMIT 200`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := map[string][]DashboardLiveAgent{
		"working":         {},
		"needs_attention": {},
		"idle_recent":     {},
	}
	for rows.Next() {
		var item DashboardLiveAgent
		var status string
		var updated sql.NullTime
		if err := rows.Scan(
			&item.AgentID, &item.AgentName, &item.AvatarURL, &item.IsActive,
			&item.RunID, &item.SessionID, &item.TaskID, &status, &item.ActivityText,
			&item.ToolName, &item.ToolInputSummary, &item.Source, &updated,
			&item.ActiveCount, &item.AttentionCount, &item.RunCount,
		); err != nil {
			return nil, err
		}
		if status != "" {
			item.Status = AgentRunStatus(status)
		}
		if updated.Valid {
			item.UpdatedAt = &updated.Time
		}
		item.Group = dashboardLiveGroup(item)
		groups[item.Group] = append(groups[item.Group], item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &DashboardLive{
		GeneratedAt: time.Now().UTC(),
		Groups: []DashboardLiveGroup{
			{Key: "working", Label: "Working", Items: groups["working"]},
			{Key: "needs_attention", Label: "Needs Attention", Items: groups["needs_attention"]},
			{Key: "idle_recent", Label: "Idle / Recent", Items: groups["idle_recent"]},
		},
	}
	for i := range result.Groups {
		result.Groups[i].Count = len(result.Groups[i].Items)
	}
	result.Totals = DashboardLiveTotals{
		Agents:         result.Groups[0].Count + result.Groups[1].Count + result.Groups[2].Count,
		Working:        result.Groups[0].Count,
		NeedsAttention: result.Groups[1].Count,
		IdleRecent:     result.Groups[2].Count,
	}
	return result, nil
}

func dashboardLiveGroup(item DashboardLiveAgent) string {
	switch item.Status {
	case AgentRunStatusQueued, AgentRunStatusThinking, AgentRunStatusRunning, AgentRunStatusStreaming:
		return "working"
	case AgentRunStatusWaitingInput, AgentRunStatusWaitingApproval, AgentRunStatusFailed, AgentRunStatusTimeout:
		return "needs_attention"
	default:
		return "idle_recent"
	}
}

func (s *AgentRunService) GetDashboardInsight(ctx context.Context, ownerID string, since time.Time) (*DashboardInsight, error) {
	result := &DashboardInsight{
		Since:       since,
		GeneratedAt: time.Now().UTC(),
		Terms:       []DashboardCount{},
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE created_at >= $1`, since).Scan(&result.Messages); err != nil {
		return nil, err
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM agent_runs r
		   JOIN agents a ON a.id = r.agent_id
		  WHERE r.updated_at >= $1 AND a.owner_id = $2 AND a.is_active = true`,
		since, ownerID,
	).Scan(&result.AgentRuns); err != nil {
		return nil, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE updated_at >= $1`, since).Scan(&result.Tasks); err != nil {
		return nil, err
	}
	tokens, err := s.dashboardTokenUsage(ctx, ownerID, since)
	if err != nil {
		return nil, err
	}
	result.Tokens = tokens

	result.RunStatus, err = s.dashboardCounts(ctx,
		`SELECT r.status, r.status, COUNT(*)
		   FROM agent_runs r
		   JOIN agents a ON a.id = r.agent_id
		  WHERE r.updated_at >= $1 AND a.owner_id = $2 AND a.is_active = true
		  GROUP BY r.status
		  ORDER BY COUNT(*) DESC`, since, ownerID)
	if err != nil {
		return nil, err
	}
	result.TaskStatus, err = s.dashboardCounts(ctx, `SELECT status, status, COUNT(*) FROM tasks WHERE updated_at >= $1 GROUP BY status ORDER BY COUNT(*) DESC`, since)
	if err != nil {
		return nil, err
	}
	result.ToolUsage, err = s.dashboardCounts(ctx,
		`SELECT e.tool_name, e.tool_name, COUNT(*)
		   FROM agent_run_events e
		   JOIN agent_runs r ON r.id = e.run_id
		   JOIN agents a ON a.id = r.agent_id
		  WHERE e.created_at >= $1 AND a.owner_id = $2 AND a.is_active = true AND COALESCE(e.tool_name, '') <> ''
		  GROUP BY e.tool_name
		  ORDER BY COUNT(*) DESC
		  LIMIT 20`, since, ownerID)
	if err != nil {
		return nil, err
	}
	result.AgentUsage, err = s.dashboardAgentUsage(ctx, ownerID, since)
	if err != nil {
		return nil, err
	}
	result.TaskUsage, err = s.dashboardTaskUsage(ctx, ownerID, since)
	if err != nil {
		return nil, err
	}
	result.Series, err = s.dashboardSeries(ctx, ownerID, since)
	if err != nil {
		return nil, err
	}
	result.Terms, err = s.dashboardTerms(ctx, since)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *AgentRunService) dashboardTokenUsage(ctx context.Context, ownerID string, since time.Time) (DashboardTokenUsage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.usage_json, COALESCE(r.transcript_path, sess.transcript_path, ''), r.started_at, r.finished_at
		   FROM agent_runs r
		   JOIN agents a ON a.id = r.agent_id
		   LEFT JOIN agent_sessions sess ON sess.id = r.session_id
		  WHERE r.updated_at >= $1 AND a.owner_id = $2 AND a.is_active = true`,
		since, ownerID)
	if err != nil {
		return DashboardTokenUsage{}, err
	}
	defer rows.Close()

	var total DashboardTokenUsage
	for rows.Next() {
		var usageJSON json.RawMessage
		var transcriptPath string
		var startedAt time.Time
		var finished sql.NullTime
		if err := rows.Scan(&usageJSON, &transcriptPath, &startedAt, &finished); err != nil {
			return DashboardTokenUsage{}, err
		}
		usage, err := dashboardRunUsage(usageJSON, transcriptPath, startedAt, finished)
		if err != nil {
			return DashboardTokenUsage{}, err
		}
		total.Input += usage.Input
		total.Output += usage.Output
	}
	if err := rows.Err(); err != nil {
		return DashboardTokenUsage{}, err
	}
	total.Total = total.Input + total.Output
	return total, nil
}

func dashboardRunUsage(usageJSON json.RawMessage, transcriptPath string, startedAt time.Time, finished sql.NullTime) (DashboardTokenUsage, error) {
	usage := dashboardUsageFromJSON(usageJSON)
	if usage.Input != 0 || usage.Output != 0 {
		usage.Total = usage.Input + usage.Output
		return usage, nil
	}
	end := time.Now().UTC()
	if finished.Valid {
		end = finished.Time
	}
	entries, err := ReadAgentTranscriptWindow(transcriptPath, startedAt.Add(-2*time.Second), end.Add(2*time.Second), 20000)
	if err != nil {
		return DashboardTokenUsage{}, nil
	}
	return dashboardUsageFromTranscript(entries), nil
}

func dashboardUsageFromJSON(raw json.RawMessage) DashboardTokenUsage {
	var data struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &data) != nil {
		return DashboardTokenUsage{}
	}
	return DashboardTokenUsage{Input: data.InputTokens, Output: data.OutputTokens}
}

func dashboardUsageFromTranscript(entries []AgentTranscriptEntry) DashboardTokenUsage {
	var usage DashboardTokenUsage
	for _, entry := range entries {
		if entry.Usage == nil {
			continue
		}
		usage.Input += entry.Usage.InputTokens
		usage.Output += entry.Usage.OutputTokens
	}
	usage.Total = usage.Input + usage.Output
	return usage
}

func (s *AgentRunService) dashboardCounts(ctx context.Context, query string, args ...any) ([]DashboardCount, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var counts []DashboardCount
	for rows.Next() {
		var item DashboardCount
		if err := rows.Scan(&item.Key, &item.Label, &item.Count); err != nil {
			return nil, err
		}
		counts = append(counts, item)
	}
	return counts, rows.Err()
}

func (s *AgentRunService) dashboardAgentUsage(ctx context.Context, ownerID string, since time.Time) ([]DashboardUsageCount, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id::text, a.name, COUNT(r.id), MAX(r.updated_at)
		   FROM agents a
		   JOIN agent_runs r ON r.agent_id = a.id AND r.updated_at >= $1
		  WHERE a.owner_id = $2 AND a.is_active = true
		  GROUP BY a.id, a.name
		  ORDER BY COUNT(r.id) DESC, MAX(r.updated_at) DESC NULLS LAST
		  LIMIT 20`, since, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DashboardUsageCount
	for rows.Next() {
		var item DashboardUsageCount
		var last sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Count, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			item.LastAt = &last.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *AgentRunService) dashboardSeries(ctx context.Context, ownerID string, since time.Time) ([]DashboardSeriesPoint, error) {
	start := dayStart(since)
	end := dayStart(time.Now().UTC()).AddDate(0, 0, 1)
	byDate := map[string]*DashboardSeriesPoint{}
	for t := start; t.Before(end); t = t.AddDate(0, 0, 1) {
		key := t.Format("2006-01-02")
		byDate[key] = &DashboardSeriesPoint{Date: key}
	}
	if err := s.fillDailyCounts(ctx, byDate, "messages",
		`SELECT date_trunc('day', m.created_at)::date::text, COUNT(*)
		   FROM messages m
		  WHERE m.created_at >= $1
		  GROUP BY 1`, since); err != nil {
		return nil, err
	}
	if err := s.fillDailyCounts(ctx, byDate, "tasks",
		`SELECT date_trunc('day', t.updated_at)::date::text, COUNT(*)
		   FROM tasks t
		  WHERE t.updated_at >= $1
		  GROUP BY 1`, since); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT date_trunc('day', r.updated_at)::date::text,
		        r.usage_json, COALESCE(r.transcript_path, sess.transcript_path, ''), r.started_at, r.finished_at
		   FROM agent_runs r
		   JOIN agents a ON a.id = r.agent_id
		   LEFT JOIN agent_sessions sess ON sess.id = r.session_id
		  WHERE r.updated_at >= $1 AND a.owner_id = $2 AND a.is_active = true`,
		since, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var usageJSON json.RawMessage
		var transcriptPath string
		var startedAt time.Time
		var finished sql.NullTime
		if err := rows.Scan(&key, &usageJSON, &transcriptPath, &startedAt, &finished); err != nil {
			return nil, err
		}
		point := byDate[key]
		if point == nil {
			continue
		}
		point.AgentRuns++
		usage, err := dashboardRunUsage(usageJSON, transcriptPath, startedAt, finished)
		if err != nil {
			return nil, err
		}
		point.Tokens += usage.Total
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]DashboardSeriesPoint, 0, len(byDate))
	for t := start; t.Before(end); t = t.AddDate(0, 0, 1) {
		result = append(result, *byDate[t.Format("2006-01-02")])
	}
	return result, nil
}

func (s *AgentRunService) fillDailyCounts(ctx context.Context, byDate map[string]*DashboardSeriesPoint, field, query string, args ...any) error {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		point := byDate[key]
		if point == nil {
			continue
		}
		switch field {
		case "messages":
			point.Messages = count
		case "tasks":
			point.Tasks = count
		}
	}
	return rows.Err()
}

func (s *AgentRunService) dashboardTerms(ctx context.Context, since time.Time) ([]DashboardCount, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.content
		   FROM messages m
		  WHERE m.created_at >= $1 AND m.sender_type = 'user'
		  ORDER BY m.created_at DESC
		  LIMIT 5000`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		for _, term := range dashboardTermsFromText(content) {
			counts[term]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]DashboardCount, 0, len(counts))
	for term, count := range counts {
		items = append(items, DashboardCount{Key: term, Label: term, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > 80 {
		items = items[:80]
	}
	return items, nil
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dashboardTermsFromText(text string) []string {
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "you": true, "with": true, "this": true,
		"that": true, "from": true, "are": true, "user": true, "assistant": true,
		"target": true, "msg": true, "time": true, "type": true, "channel": true,
	}
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "_-")
		if len([]rune(field)) < 2 || stop[field] {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

func (s *AgentRunService) dashboardTaskUsage(ctx context.Context, ownerID string, since time.Time) ([]DashboardTaskUsage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id::text, COALESCE(t.task_number, 0), t.title, t.status, COUNT(r.id), MAX(r.updated_at)
		   FROM tasks t
		   JOIN agent_run_task_links l ON l.task_id = t.id
		   JOIN agent_runs r ON r.id = l.run_id
		   JOIN agents a ON a.id = r.agent_id
		  WHERE r.updated_at >= $1 AND a.owner_id = $2 AND a.is_active = true
		  GROUP BY t.id, t.task_number, t.title, t.status
		  ORDER BY COUNT(r.id) DESC, MAX(r.updated_at) DESC
		  LIMIT 20`, since, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DashboardTaskUsage
	for rows.Next() {
		var item DashboardTaskUsage
		var last sql.NullTime
		if err := rows.Scan(&item.ID, &item.TaskNumber, &item.Title, &item.Status, &item.Count, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			item.LastAt = &last.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

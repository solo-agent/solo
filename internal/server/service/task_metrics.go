package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

type TaskObservationRequest struct {
	Kind               string   `json:"kind"`
	Note               string   `json:"note"`
	Minutes            *float64 `json:"minutes,omitempty"`
	Category           string   `json:"category,omitempty"`
	RunID              string   `json:"run_id,omitempty"`
	ReviewID           string   `json:"review_id,omitempty"`
	FirstActionCorrect *bool    `json:"first_action_correct,omitempty"`
	IdempotencyKey     string   `json:"idempotency_key"`
}

type TaskObservation struct {
	TaskObservationRequest
	ID         string    `json:"id"`
	ObserverID string    `json:"observer_id"`
	CreatedAt  time.Time `json:"created_at"`
}
type DeliveryRunMetric struct {
	BudgetOwnerID    string          `json:"budget_owner_id,omitempty"`
	ID               string          `json:"id"`
	AgentID          string          `json:"agent_id"`
	Model            string          `json:"model"`
	Budget           json.RawMessage `json:"budget"`
	Status           string          `json:"status"`
	ActualTokens     *int64          `json:"actual_tokens"`
	AccountedTokens  int64           `json:"accounted_tokens"`
	Active           bool            `json:"active"`
	Shared           bool            `json:"shared"`
	ExecutionSeconds *float64        `json:"execution_seconds"`
}
type DeliveryRework struct {
	ID       string `json:"id"`
	Reason   string `json:"reason"`
	Category string `json:"category"`
}
type DeliveryRecovery struct {
	RunID              string   `json:"run_id"`
	FirstToolSeconds   *float64 `json:"first_tool_seconds"`
	FirstActionCorrect *bool    `json:"first_action_correct"`
}
type TaskDeliveryMetric struct {
	TaskID         string              `json:"task_id"`
	TaskNumber     int                 `json:"task_number"`
	Title          string              `json:"title"`
	Status         string              `json:"status"`
	Qualified      bool                `json:"qualified"`
	ElapsedSeconds *float64            `json:"elapsed_seconds"`
	Cohort         string              `json:"cohort"`
	Observations   []TaskObservation   `json:"observations"`
	Reworks        []DeliveryRework    `json:"reworks"`
	Runs           []DeliveryRunMetric `json:"runs"`
	Recoveries     []DeliveryRecovery  `json:"recoveries"`
}

func (s *TaskService) TaskMetrics(ctx context.Context, channelID, taskID, actorID string) (*TaskDeliveryMetric, error) {
	if _, err := s.GetTask(ctx, channelID, taskID, actorID); err != nil {
		return nil, err
	}
	var metric TaskDeliveryMetric
	err := s.pool.QueryRow(ctx, `SELECT metrics FROM task_delivery_measurements WHERE id=$1 AND channel_id=$2`, taskID, channelID).Scan(&metric)
	redactDeliveryBudgets(&metric, actorID)
	return &metric, err
}

func (s *TaskService) RecordTaskObservation(ctx context.Context, channelID, taskID, actorID string, req TaskObservationRequest) (string, error) {
	if _, err := s.GetTask(ctx, channelID, taskID, actorID); err != nil {
		return "", err
	}
	req.Note = strings.TrimSpace(req.Note)
	req.Category = strings.TrimSpace(req.Category)
	if !deliveryID.MatchString(req.IdempotencyKey) || req.Note == "" || len(req.Note) > 4000 || len(req.Category) > 160 {
		return "", invalidDelivery("observation needs a note, idempotency key and bounded category")
	}
	if req.Minutes != nil && (math.IsNaN(*req.Minutes) || math.IsInf(*req.Minutes, 0) || *req.Minutes < 0 || *req.Minutes > 1440) {
		return "", invalidDelivery("record 0–1440 actual minutes per entry")
	}
	if (req.Kind == "intervention") != (req.Minutes != nil) || (req.Kind == "recovery") != (req.RunID != "" && req.FirstActionCorrect != nil) || (req.Kind == "rework") != (req.ReviewID != "") {
		return "", invalidDelivery("observation fields do not match its kind")
	}
	if req.Kind != "recovery" && (req.RunID != "" || req.FirstActionCorrect != nil) {
		return "", invalidDelivery("recovery fields require a recovery observation")
	}
	switch req.Kind {
	case "intervention", "duplicate_work", "reexplanation", "handoff":
		if req.Category != "" {
			return "", invalidDelivery("this observation does not have a category")
		}
	case "comparison":
		if req.Category == "" {
			return "", invalidDelivery("specify task type and difficulty for comparison")
		}
	case "rework":
		if _, err := uuid.Parse(req.ReviewID); err != nil {
			return "", invalidDelivery("review_id must be a UUID")
		}
		switch req.Category {
		case "requirements", "context", "implementation", "evidence", "handoff", "environment", "other":
		default:
			return "", invalidDelivery("unknown rework category")
		}
	case "recovery":
		if _, err := uuid.Parse(req.RunID); err != nil || req.Category != "" {
			return "", invalidDelivery("recovery requires its Run UUID")
		}
	default:
		return "", invalidDelivery("unknown observation kind")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	// Serialize append and authorization with Task edits; an observation never changes the Task version.
	var canClassify bool
	err = tx.QueryRow(ctx, `SELECT t.creator_id=$3 OR COALESCE(a.owner_id=$3,false) FROM tasks t LEFT JOIN agents a ON a.id=t.claimer_id JOIN channels c ON c.id=t.channel_id AND NOT c.is_archived JOIN channel_members m ON m.channel_id=t.channel_id AND m.member_id=$3 AND m.member_type='user' JOIN users u ON u.id=m.member_id AND u.is_active WHERE t.id=$1 AND t.channel_id=$2 FOR UPDATE OF t FOR SHARE OF m,u`, taskID, channelID, actorID).Scan(&canClassify)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrTaskNotReviewer
	}
	if err != nil {
		return "", err
	}
	if req.Kind == "comparison" && !canClassify {
		return "", ErrTaskNotReviewer
	}
	var id, hash string
	err = tx.QueryRow(ctx, `SELECT id::text,request_hash FROM task_observations WHERE task_id=$1 AND observer_id=$2 AND idempotency_key=$3`, taskID, actorID, req.IdempotencyKey).Scan(&id, &hash)
	if err == nil {
		if hash != deliveryRequestHash(req) {
			return "", ErrTaskVersionConflict
		}
		return id, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if req.Kind == "rework" || req.Kind == "recovery" {
		var valid, recorded bool
		if req.Kind == "rework" {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_reviews WHERE id=$1 AND task_id=$2 AND decision='rejected'),EXISTS(SELECT 1 FROM task_observations WHERE review_id=$1)`, req.ReviewID, taskID).Scan(&valid, &recorded)
		} else {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_waits WHERE run_id=$1 AND task_id=$2 AND status='resumed' AND EXISTS(SELECT 1 FROM agent_run_events WHERE run_id=$1 AND type='tool_started')),EXISTS(SELECT 1 FROM task_observations WHERE run_id=$1)`, req.RunID, taskID).Scan(&valid, &recorded)
		}
		if err != nil {
			return "", err
		}
		if !valid || recorded {
			return "", invalidDelivery("reference is unavailable or already classified")
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO task_observations(task_id,observer_id,kind,note,minutes,category,run_id,review_id,first_action_correct,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,$9,$10,$11) RETURNING id::text`, taskID, actorID, req.Kind, req.Note, req.Minutes, req.Category, req.RunID, req.ReviewID, req.FirstActionCorrect, req.IdempotencyKey, deliveryRequestHash(req)).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

type DeliveryCohort struct {
	Cohort                  string            `json:"cohort"`
	Models                  []string          `json:"models"`
	Budgets                 []json.RawMessage `json:"budgets"`
	Tasks                   int               `json:"tasks"`
	Qualified               int               `json:"qualified"`
	ComparableTasks         int               `json:"comparable_tasks"`
	ComparableQualified     int               `json:"comparable_qualified"`
	ComparableTokens        int64             `json:"comparable_tokens"`
	ActualTokens            int64             `json:"actual_tokens"`
	AccountedTokens         int64             `json:"accounted_tokens"`
	UnknownRuns             int               `json:"unknown_runs"`
	SharedRuns              int               `json:"shared_runs"`
	ActiveRuns              int               `json:"active_runs"`
	FailedRuns              int               `json:"failed_runs"`
	ExecutionSeconds        float64           `json:"execution_seconds"`
	QualifiedElapsedSeconds float64           `json:"qualified_elapsed_seconds"`
	HumanMinutes            float64           `json:"human_minutes"`
	HumanRecordedTasks      int               `json:"human_recorded_tasks"`
	Reworks                 int               `json:"reworks"`
	ReworkReasons           map[string]int    `json:"rework_reasons"`
	Observations            map[string]int    `json:"observations"`
	RecoverySamples         int               `json:"recovery_samples"`
	RecoverySeconds         float64           `json:"recovery_seconds"`
	FirstActionsRecorded    int               `json:"first_actions_recorded"`
	FirstActionsCorrect     int               `json:"first_actions_correct"`
}

// All Run costs are kept, including failed and unaccepted work. A shared Run is
// counted once inside a cohort; cohorts cannot be summed when sharing is present.
func deliveryCohorts(metrics []TaskDeliveryMetric) []DeliveryCohort {
	groups := map[string]*DeliveryCohort{}
	runsByGroup := map[string]map[string]bool{}
	for _, m := range metrics {
		models := map[string]bool{}
		budgets := map[string]bool{}
		comparable := m.Cohort != "" && len(m.Runs) > 0
		for _, r := range m.Runs {
			models[r.Model] = true
			var canonical any
			_ = json.Unmarshal(r.Budget, &canonical)
			data, _ := json.Marshal(canonical)
			budgets[string(data)] = true
			if r.Model == "unknown/unknown" || strings.Contains(r.Model, "/unknown") || canonical == nil || r.ActualTokens == nil || r.Active || r.Shared {
				comparable = false
			}
		}
		modelKeys := sortedMetricKeys(models)
		budgetKeys := sortedMetricKeys(budgets)
		keyBytes, _ := json.Marshal([]any{m.Cohort, modelKeys, budgetKeys})
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			g = &DeliveryCohort{Cohort: m.Cohort, Models: modelKeys, Budgets: []json.RawMessage{}, ReworkReasons: map[string]int{}, Observations: map[string]int{}}
			for _, b := range budgetKeys {
				g.Budgets = append(g.Budgets, json.RawMessage(b))
			}
			groups[key] = g
			runsByGroup[key] = map[string]bool{}
		}
		g.Tasks++
		if m.Qualified {
			g.Qualified++
			if m.ElapsedSeconds != nil {
				g.QualifiedElapsedSeconds += *m.ElapsedSeconds
			}
		}
		if comparable {
			g.ComparableTasks++
			if m.Qualified {
				g.ComparableQualified++
			}
		}
		for _, r := range m.Runs {
			if runsByGroup[key][r.ID] {
				continue
			}
			runsByGroup[key][r.ID] = true
			if r.ActualTokens != nil {
				g.ActualTokens += *r.ActualTokens
				if comparable {
					g.ComparableTokens += *r.ActualTokens
				}
			} else {
				g.UnknownRuns++
			}
			g.AccountedTokens += r.AccountedTokens
			if r.Active {
				g.ActiveRuns++
			}
			if r.Shared {
				g.SharedRuns++
			}
			if r.Status == "failed" || r.Status == "timeout" || r.Status == "cancelled" {
				g.FailedRuns++
			}
			if r.ExecutionSeconds != nil {
				g.ExecutionSeconds += *r.ExecutionSeconds
			}
		}
		humanRecorded := false
		for _, o := range m.Observations {
			g.Observations[o.Kind]++
			if o.Minutes != nil {
				g.HumanMinutes += *o.Minutes
				humanRecorded = true
			}
		}
		if humanRecorded {
			g.HumanRecordedTasks++
		}
		for _, r := range m.Reworks {
			g.Reworks++
			g.ReworkReasons[r.Category]++
		}
		for _, r := range m.Recoveries {
			if r.FirstToolSeconds != nil {
				g.RecoverySamples++
				g.RecoverySeconds += *r.FirstToolSeconds
			}
			if r.FirstActionCorrect != nil {
				g.FirstActionsRecorded++
				if *r.FirstActionCorrect {
					g.FirstActionsCorrect++
				}
			}
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]DeliveryCohort, 0, len(keys))
	for _, k := range keys {
		result = append(result, *groups[k])
	}
	return result
}
func sortedMetricKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func (s *AgentRunService) dashboardDelivery(ctx context.Context, actorID string, since time.Time) ([]DeliveryCohort, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.metrics FROM task_delivery_measurements d JOIN channels c ON c.id=d.channel_id JOIN channel_members m ON m.channel_id=c.id AND m.member_id=$1 AND m.member_type='user' WHERE d.created_at >= $2 AND ($3='' OR c.workspace_id::text=$3) ORDER BY d.created_at,d.id`, actorID, since, serverworkspace.FilterID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metrics := []TaskDeliveryMetric{}
	for rows.Next() {
		var m TaskDeliveryMetric
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		redactDeliveryBudgets(&m, actorID)
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deliveryCohorts(metrics), nil
}

// User budget policy limits remain private even when Task results are shared.
func redactDeliveryBudgets(metric *TaskDeliveryMetric, actorID string) {
	for i := range metric.Runs {
		if metric.Runs[i].BudgetOwnerID != actorID {
			metric.Runs[i].Budget = json.RawMessage("null")
		}
		metric.Runs[i].BudgetOwnerID = ""
	}
}

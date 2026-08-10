package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BudgetPolicy struct {
	ID                 string `json:"id,omitempty"`
	ScopeType          string `json:"scope_type"`
	AgentID            string `json:"agent_id,omitempty"`
	Enabled            bool   `json:"enabled"`
	MonthlyLimitTokens int64  `json:"monthly_limit_tokens"`
}

type BudgetPeriodSummary struct {
	LimitTokens     int64 `json:"limit_tokens"`
	UsedTokens      int64 `json:"used_tokens"`
	ReservedTokens  int64 `json:"reserved_tokens"`
	RemainingTokens int64 `json:"remaining_tokens"`
	Blocked         bool  `json:"blocked"`
}

type BudgetView struct {
	Policy         BudgetPolicy        `json:"policy"`
	Month          BudgetPeriodSummary `json:"month"`
	Unit           string              `json:"unit"`
	PeriodTimezone string              `json:"period_timezone"`
	PauseReason    string              `json:"pause_reason,omitempty"`
}

type SaveBudgetInput struct {
	Enabled            bool  `json:"enabled"`
	MonthlyLimitTokens int64 `json:"monthly_limit_tokens"`
}

type SaveAgentBudgetInput = SaveBudgetInput

type BudgetStartError struct {
	Scope  string
	Period string
	Reason string
}

func (e *BudgetStartError) Error() string { return e.Reason }

func (e *BudgetStartError) Code() string {
	if e.Scope == "agent" {
		return "agent.error.agent_monthly_token_budget_exhausted"
	}
	return "agent.error.user_monthly_token_budget_exhausted"
}

type BudgetInputError struct {
	Code    string
	Message string
}

func (e *BudgetInputError) Error() string { return e.Message }

const systemRunReserveTokens int64 = 100_000
const maxSafeJSONInteger int64 = 9_007_199_254_740_991

type BudgetService struct {
	pool *pgxpool.Pool
}

func NewBudgetService(pool *pgxpool.Pool) *BudgetService {
	return &BudgetService{pool: pool}
}

func validateBudgetInput(input SaveBudgetInput) error {
	if input.MonthlyLimitTokens < 0 || input.MonthlyLimitTokens > maxSafeJSONInteger {
		return &BudgetInputError{Code: "budget.error.monthly_limit_invalid", Message: "monthly Token limit is outside the supported range"}
	}
	if !input.Enabled {
		return nil
	}
	if input.MonthlyLimitTokens == 0 {
		return &BudgetInputError{Code: "budget.error.monthly_limit_required", Message: "monthly Token limit must be greater than zero when enabled"}
	}
	return nil
}

func (s *BudgetService) GetUserBudget(ctx context.Context, ownerID string) (*BudgetView, error) {
	policy, err := s.loadPolicy(ctx, "user", ownerID, "")
	if err != nil {
		return nil, err
	}
	return s.buildView(ctx, policy, ownerID, "")
}

func (s *BudgetService) SaveUserBudget(ctx context.Context, ownerID string, input SaveBudgetInput) (*BudgetView, error) {
	if err := validateBudgetInput(input); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockBudgetKey(ctx, tx, "user:"+ownerID); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO budget_policies (
		  id, owner_id, scope_type, enabled, monthly_limit_tokens
		) VALUES ($1, $2, 'user', $3, $4)
		ON CONFLICT (owner_id) WHERE scope_type = 'user'
		DO UPDATE SET enabled = EXCLUDED.enabled,
		              monthly_limit_tokens = EXCLUDED.monthly_limit_tokens,
		              updated_at = now()`,
		uuid.NewString(), ownerID, input.Enabled, input.MonthlyLimitTokens)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetUserBudget(ctx, ownerID)
}

func (s *BudgetService) GetAgentBudget(ctx context.Context, ownerID, agentID string) (*BudgetView, error) {
	if err := s.ensureOwnedAgent(ctx, ownerID, agentID); err != nil {
		return nil, err
	}
	policy, err := s.loadPolicy(ctx, "agent", ownerID, agentID)
	if err != nil {
		return nil, err
	}
	return s.buildView(ctx, policy, ownerID, agentID)
}

func (s *BudgetService) SaveAgentBudget(ctx context.Context, ownerID, agentID string, input SaveAgentBudgetInput) (*BudgetView, error) {
	if err := validateBudgetInput(input); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockBudgetKey(ctx, tx, "user:"+ownerID); err != nil {
		return nil, err
	}
	if err := lockBudgetKey(ctx, tx, "agent:"+agentID); err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT true
		  FROM agents
		 WHERE id = $1 AND owner_id = $2 AND is_active = true
		 FOR SHARE`, agentID, ownerID).Scan(&exists); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO budget_policies (
		  id, owner_id, scope_type, agent_id, enabled, monthly_limit_tokens
		) VALUES ($1, $2, 'agent', $3, $4, $5)
		ON CONFLICT (agent_id) WHERE scope_type = 'agent'
		DO UPDATE SET owner_id = EXCLUDED.owner_id,
		              enabled = EXCLUDED.enabled,
		              monthly_limit_tokens = EXCLUDED.monthly_limit_tokens,
		              updated_at = now()`,
		uuid.NewString(), ownerID, agentID, input.Enabled, input.MonthlyLimitTokens)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetAgentBudget(ctx, ownerID, agentID)
}

func (s *BudgetService) ensureOwnedAgent(ctx context.Context, ownerID, agentID string) error {
	var exists bool
	return s.pool.QueryRow(ctx, `
		SELECT true
		  FROM agents
		 WHERE id = $1 AND owner_id = $2 AND is_active = true`, agentID, ownerID).Scan(&exists)
}

func (s *BudgetService) loadPolicy(ctx context.Context, scope, ownerID, agentID string) (BudgetPolicy, error) {
	policy := BudgetPolicy{ScopeType: scope, AgentID: agentID}
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, enabled, monthly_limit_tokens
		  FROM budget_policies
		 WHERE owner_id = $1 AND scope_type = $2
		   AND (($2 = 'user' AND agent_id IS NULL) OR ($2 = 'agent' AND agent_id = $3))`,
		ownerID, scope, nullableUUID(agentID)).Scan(&policy.ID, &policy.Enabled, &policy.MonthlyLimitTokens)
	if errors.Is(err, pgx.ErrNoRows) {
		return policy, nil
	}
	return policy, err
}

func (s *BudgetService) buildView(ctx context.Context, policy BudgetPolicy, ownerID, agentID string) (*BudgetView, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	month, err := s.periodSummary(ctx, policy, ownerID, agentID, monthStart, policy.MonthlyLimitTokens)
	if err != nil {
		return nil, err
	}
	view := &BudgetView{Policy: policy, Month: month, Unit: "token", PeriodTimezone: "UTC"}
	if policy.Enabled && month.Blocked {
		view.PauseReason = "budget.error.monthly_limit_exhausted"
	}
	return view, nil
}

func (s *BudgetService) periodSummary(ctx context.Context, policy BudgetPolicy, ownerID, agentID string, start time.Time, limit int64) (BudgetPeriodSummary, error) {
	used, reserved, err := s.scopeUsage(ctx, nil, policy.ID, ownerID, agentID, start)
	if err != nil {
		return BudgetPeriodSummary{}, err
	}
	result := BudgetPeriodSummary{LimitTokens: limit, UsedTokens: used, ReservedTokens: reserved}
	if !policy.Enabled {
		return result, nil
	}
	result.RemainingTokens = limit - used - reserved
	if result.RemainingTokens < 0 {
		result.RemainingTokens = 0
	}
	result.Blocked = limit <= 0 || used+reserved+runReserveTokens(policy) > limit
	return result, nil
}

func runReserveTokens(policy BudgetPolicy) int64 {
	if !policy.Enabled || policy.MonthlyLimitTokens <= 0 {
		return 0
	}
	if policy.MonthlyLimitTokens < systemRunReserveTokens {
		return policy.MonthlyLimitTokens
	}
	return systemRunReserveTokens
}

type budgetRunPlan struct {
	OwnerID  string
	AgentID  string
	Policies []BudgetPolicy
}

func (s *BudgetService) ReserveRunTx(ctx context.Context, tx pgx.Tx, runID, agentID string) error {
	plan, err := s.buildRunPlanTx(ctx, tx, agentID)
	if err != nil {
		return err
	}
	var reserved int64
	for _, policy := range plan.Policies {
		if candidate := runReserveTokens(policy); candidate > reserved {
			reserved = candidate
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO agent_run_token_usage (run_id, owner_id, agent_id, state, reserved_tokens)
		VALUES ($1, $2, $3, 'pending', $4)`, runID, plan.OwnerID, agentID, reserved)
	if err != nil {
		return err
	}
	for _, policy := range plan.Policies {
		reserve := runReserveTokens(policy)
		if _, err := tx.Exec(ctx, `
			INSERT INTO budget_reservations (run_id, policy_id, reserved_tokens)
			VALUES ($1, $2, $3)`, runID, policy.ID, reserve); err != nil {
			return err
		}
	}
	return nil
}

func (s *BudgetService) buildRunPlanTx(ctx context.Context, tx pgx.Tx, agentID string) (*budgetRunPlan, error) {
	var ownerID string
	if err := tx.QueryRow(ctx, `
		SELECT owner_id::text
		  FROM agents
		 WHERE id = $1 AND is_active = true
		 FOR SHARE`, agentID).Scan(&ownerID); err != nil {
		return nil, err
	}
	if err := lockBudgetKey(ctx, tx, "user:"+ownerID); err != nil {
		return nil, err
	}
	if err := lockBudgetKey(ctx, tx, "agent:"+agentID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, scope_type, COALESCE(agent_id::text, ''), enabled,
		       monthly_limit_tokens
		  FROM budget_policies
		 WHERE owner_id = $1
		   AND ((scope_type = 'user' AND agent_id IS NULL) OR (scope_type = 'agent' AND agent_id = $2))
		 ORDER BY scope_type DESC`, ownerID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BudgetPolicy
	for rows.Next() {
		var policy BudgetPolicy
		if err := rows.Scan(&policy.ID, &policy.ScopeType, &policy.AgentID, &policy.Enabled,
			&policy.MonthlyLimitTokens); err != nil {
			return nil, err
		}
		if policy.Enabled {
			policies = append(policies, policy)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, policy := range policies {
		if policy.MonthlyLimitTokens <= 0 {
			return nil, budgetLimitError(policy.ScopeType)
		}
		scopeAgentID := ""
		if policy.ScopeType == "agent" {
			scopeAgentID = agentID
		}
		usedMonth, active, err := s.scopeUsage(ctx, tx, policy.ID, ownerID, scopeAgentID, monthStart)
		if err != nil {
			return nil, err
		}
		if usedMonth+active+runReserveTokens(policy) > policy.MonthlyLimitTokens {
			return nil, budgetLimitError(policy.ScopeType)
		}
	}
	return &budgetRunPlan{OwnerID: ownerID, AgentID: agentID, Policies: policies}, nil
}

func budgetLimitError(scope string) error {
	scopeName := "user"
	if scope == "agent" {
		scopeName = "Agent"
	}
	return &BudgetStartError{Scope: scope, Period: "month", Reason: fmt.Sprintf("%s monthly Token budget is exhausted", scopeName)}
}

type budgetQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *BudgetService) scopeUsage(ctx context.Context, tx pgx.Tx, policyID, ownerID, agentID string, start time.Time) (int64, int64, error) {
	var q budgetQuerier = s.pool
	if tx != nil {
		q = tx
	}
	var actual int64
	query := `
		SELECT COALESCE(SUM(actual_tokens), 0)
		  FROM agent_run_token_usage
		 WHERE state = 'settled' AND created_at >= $1 AND owner_id = $2`
	args := []any{start, ownerID}
	if agentID != "" {
		query += ` AND agent_id = $3`
		args = append(args, agentID)
	}
	if err := q.QueryRow(ctx, query, args...).Scan(&actual); err != nil {
		return 0, 0, err
	}
	var unknown, active int64
	if policyID != "" {
		if err := q.QueryRow(ctx, `
			SELECT COALESCE(SUM(CASE WHEN state = 'settled' AND created_at >= $2 THEN accounted_tokens ELSE 0 END), 0),
			       COALESCE(SUM(CASE WHEN state = 'active' THEN reserved_tokens ELSE 0 END), 0)
			  FROM budget_reservations
			 WHERE policy_id = $1`, policyID, start).Scan(&unknown, &active); err != nil {
			return 0, 0, err
		}
	}
	return actual + unknown, active, nil
}

func (s *BudgetService) SettleRunTx(ctx context.Context, tx pgx.Tx, runID string, backendStarted bool, usageJSON []byte) error {
	var state string
	var reserved int64
	err := tx.QueryRow(ctx, `
		SELECT state, reserved_tokens
		  FROM agent_run_token_usage WHERE run_id = $1 FOR UPDATE`, runID).Scan(&state, &reserved)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	usage, err := budgetUsageFromJSON(usageJSON)
	if err != nil {
		return err
	}
	total, err := usage.total()
	if err != nil {
		return err
	}

	newState := "settled"
	actual := &total
	overrun := false
	reservationState := "settled"
	accountReserved := false
	if !backendStarted {
		newState = "released"
		zero := int64(0)
		actual = &zero
		reservationState = "released"
	} else if total == 0 {
		newState = "usage_unknown"
		actual = nil
		accountReserved = true
	} else {
		overrun = reserved > 0 && total > reserved
	}

	_, err = tx.Exec(ctx, `
		UPDATE agent_run_token_usage
		   SET state = $2, actual_tokens = $3, input_tokens = $4, output_tokens = $5,
		       cache_read_tokens = $6, cache_write_tokens = $7, overrun = $8, settled_at = now()
		 WHERE run_id = $1`, runID, newState, actual, usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite, overrun)
	if err != nil {
		return err
	}
	if accountReserved {
		_, err = tx.Exec(ctx, `
			UPDATE budget_reservations
			   SET state = $2, accounted_tokens = reserved_tokens, settled_at = now()
			 WHERE run_id = $1 AND state = 'active'`, runID, reservationState)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE budget_reservations
			   SET state = $2, accounted_tokens = 0, settled_at = now()
			 WHERE run_id = $1 AND state = 'active'`, runID, reservationState)
	}
	return err
}

// SettleTerminalRunsTx closes token reservations for terminal runs that were
// cancelled by a wider operation such as deleting an Agent or Channel.
func (s *BudgetService) SettleTerminalRunsTx(ctx context.Context, tx pgx.Tx, runIDs []string) error {
	for _, runID := range runIDs {
		var backendStarted bool
		var usageJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT backend_started_at IS NOT NULL, usage_json
			  FROM agent_runs WHERE id = $1`, runID).Scan(&backendStarted, &usageJSON); err != nil {
			return err
		}
		if err := s.SettleRunTx(ctx, tx, runID, backendStarted, usageJSON); err != nil {
			return err
		}
	}
	return nil
}

type budgetTokenUsage struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

func (u budgetTokenUsage) total() (int64, error) {
	const maxInt64 = int64(1<<63 - 1)
	total := int64(0)
	for _, value := range []int64{u.Input, u.Output, u.CacheRead, u.CacheWrite} {
		if value > maxInt64-total {
			return 0, fmt.Errorf("run token usage is too large")
		}
		total += value
	}
	return total, nil
}

func budgetUsageFromJSON(raw []byte) (budgetTokenUsage, error) {
	if len(raw) == 0 {
		return budgetTokenUsage{}, nil
	}
	var data struct {
		Input      int64 `json:"input_tokens"`
		Output     int64 `json:"output_tokens"`
		CacheRead  int64 `json:"cache_read_tokens"`
		CacheWrite int64 `json:"cache_write_tokens"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return budgetTokenUsage{}, fmt.Errorf("decode run usage: %w", err)
	}
	if data.Input < 0 || data.Output < 0 || data.CacheRead < 0 || data.CacheWrite < 0 {
		return budgetTokenUsage{}, fmt.Errorf("run usage cannot be negative")
	}
	return budgetTokenUsage{Input: data.Input, Output: data.Output, CacheRead: data.CacheRead, CacheWrite: data.CacheWrite}, nil
}

func lockBudgetKey(ctx context.Context, tx pgx.Tx, key string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, strings.TrimSpace(key))
	return err
}

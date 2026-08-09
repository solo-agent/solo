package service

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestValidateBudgetInput(t *testing.T) {
	tests := []struct {
		name string
		in   SaveBudgetInput
		code string
	}{
		{name: "disabled zero", in: SaveBudgetInput{}},
		{name: "enabled positive", in: SaveBudgetInput{Enabled: true, MonthlyLimitTokens: 1}},
		{name: "negative", in: SaveBudgetInput{MonthlyLimitTokens: -1}, code: "budget.error.monthly_limit_invalid"},
		{name: "too large for JSON", in: SaveBudgetInput{MonthlyLimitTokens: maxSafeJSONInteger + 1}, code: "budget.error.monthly_limit_invalid"},
		{name: "enabled zero", in: SaveBudgetInput{Enabled: true}, code: "budget.error.monthly_limit_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateBudgetInput(test.in)
			if test.code == "" {
				if err != nil {
					t.Fatalf("validateBudgetInput: %v", err)
				}
				return
			}
			var inputErr *BudgetInputError
			if !errors.As(err, &inputErr) || inputErr.Code != test.code {
				t.Fatalf("validateBudgetInput error = %#v, want code %q", err, test.code)
			}
		})
	}
}

func TestRunReserveTokensIsInternalAndBoundedByMonthlyLimit(t *testing.T) {
	if got := runReserveTokens(BudgetPolicy{}); got != 0 {
		t.Fatalf("disabled reserve = %d, want 0", got)
	}
	if got := runReserveTokens(BudgetPolicy{Enabled: true, MonthlyLimitTokens: 40_000}); got != 40_000 {
		t.Fatalf("small monthly reserve = %d, want 40000", got)
	}
	if got := runReserveTokens(BudgetPolicy{Enabled: true, MonthlyLimitTokens: 2_000_000}); got != systemRunReserveTokens {
		t.Fatalf("normal monthly reserve = %d, want %d", got, systemRunReserveTokens)
	}
}

func TestBudgetGateSettlesTokenTotalOnce(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	budgetSvc := NewBudgetService(pool)
	_, err := budgetSvc.SaveAgentBudget(ctx, ownerID, agentID, SaveAgentBudgetInput{
		Enabled: true, MonthlyLimitTokens: 10_000_000,
	})
	if err != nil {
		t.Fatalf("SaveAgentBudget: %v", err)
	}

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, Status: AgentRunStatusQueued, ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.BudgetState != "pending" || run.ReservedTokens != systemRunReserveTokens {
		t.Fatalf("start token usage = %+v", run)
	}
	if _, err := runSvc.MarkBackendStarted(ctx, run.ID); err != nil {
		t.Fatalf("MarkBackendStarted: %v", err)
	}
	finished, err := runSvc.FinishRun(ctx, FinishRunInput{
		RunID: run.ID, Status: AgentRunStatusCompleted, ActivityText: "已完成",
		Usage: map[string]int64{
			"input_tokens": 500_000, "output_tokens": 200_000,
			"cache_read_tokens": 100_000, "cache_write_tokens": 50_000,
		},
	})
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if finished.BudgetState != "settled" || finished.ActualTokens == nil || *finished.ActualTokens != 850_000 {
		t.Fatalf("settled token usage = %+v, want 850000", finished)
	}
	late, err := runSvc.FinishRun(ctx, FinishRunInput{RunID: run.ID, Status: AgentRunStatusFailed, ActivityText: "late"})
	if !errors.Is(err, ErrAgentRunAlreadyFinished) || late.ActualTokens == nil || *late.ActualTokens != 850_000 {
		t.Fatalf("late finish = %+v, %v", late, err)
	}
}

func TestBudgetGateConcurrentStartsDoNotExceedLimit(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	_, err := NewBudgetService(pool).SaveAgentBudget(ctx, ownerID, agentID, SaveAgentBudgetInput{
		Enabled: true, MonthlyLimitTokens: systemRunReserveTokens,
	})
	if err != nil {
		t.Fatalf("SaveAgentBudget: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
				AgentID: agentID, TriggerType: AgentRunTriggerMessage, Status: AgentRunStatusQueued, ActivityText: "等待执行",
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded, blocked := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		var budgetErr *BudgetStartError
		if errors.As(err, &budgetErr) {
			blocked++
			continue
		}
		t.Fatalf("unexpected start error: %v", err)
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("starts: succeeded=%d blocked=%d, want 1/1", succeeded, blocked)
	}
}

func TestBudgetGateFailureAccounting(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	budgetSvc := NewBudgetService(pool)
	_, err := budgetSvc.SaveAgentBudget(ctx, ownerID, agentID, SaveAgentBudgetInput{
		Enabled: true, MonthlyLimitTokens: 4_000_000,
	})
	if err != nil {
		t.Fatalf("SaveAgentBudget: %v", err)
	}
	runSvc := NewAgentRunService(pool)

	notStarted, err := runSvc.StartRun(ctx, StartRunInput{AgentID: agentID, TriggerType: AgentRunTriggerMessage, ActivityText: "等待执行"})
	if err != nil {
		t.Fatalf("start not-started run: %v", err)
	}
	notStarted, err = runSvc.FinishRun(ctx, FinishRunInput{RunID: notStarted.ID, Status: AgentRunStatusCancelled, ActivityText: "已取消"})
	if err != nil || notStarted.BudgetState != "released" || notStarted.ActualTokens == nil || *notStarted.ActualTokens != 0 {
		t.Fatalf("not-started settlement = %+v, %v", notStarted, err)
	}

	unknown, err := runSvc.StartRun(ctx, StartRunInput{AgentID: agentID, TriggerType: AgentRunTriggerMessage, ActivityText: "等待执行"})
	if err != nil {
		t.Fatalf("start unknown run: %v", err)
	}
	if _, err := runSvc.MarkBackendStarted(ctx, unknown.ID); err != nil {
		t.Fatalf("mark unknown run started: %v", err)
	}
	unknown, err = runSvc.FinishRun(ctx, FinishRunInput{RunID: unknown.ID, Status: AgentRunStatusFailed, ActivityText: "失败"})
	if err != nil || unknown.BudgetState != "usage_unknown" || unknown.ActualTokens != nil {
		t.Fatalf("unknown settlement = %+v, %v", unknown, err)
	}
	view, err := budgetSvc.GetAgentBudget(ctx, ownerID, agentID)
	if err != nil {
		t.Fatalf("GetAgentBudget: %v", err)
	}
	if view.Month.UsedTokens != systemRunReserveTokens || view.Month.ReservedTokens != 0 {
		t.Fatalf("month = %+v, want unknown run to account for its reservation", view.Month)
	}
}

func TestBudgetGateRecordsTokensWhileDisabled(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{AgentID: agentID, TriggerType: AgentRunTriggerMessage, ActivityText: "等待执行"})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := runSvc.MarkBackendStarted(ctx, run.ID); err != nil {
		t.Fatalf("MarkBackendStarted: %v", err)
	}
	run, err = runSvc.FinishRun(ctx, FinishRunInput{
		RunID: run.ID, Status: AgentRunStatusCompleted, ActivityText: "已完成",
		Usage: map[string]int64{"input_tokens": 10, "output_tokens": 20, "cache_read_tokens": 3, "cache_write_tokens": 4},
	})
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if run.ActualTokens == nil || *run.ActualTokens != 37 {
		t.Fatalf("actual tokens = %+v, want 37", run.ActualTokens)
	}
	view, err := NewBudgetService(pool).GetAgentBudget(ctx, ownerID, agentID)
	if err != nil {
		t.Fatalf("GetAgentBudget: %v", err)
	}
	if view.Policy.Enabled || view.Month.UsedTokens != 37 {
		t.Fatalf("disabled view = %+v, want 37 used tokens", view)
	}
}

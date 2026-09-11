package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestTaskMetricsPostgresEvidenceCostsAndObservations(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	tasks := NewTaskService(pool)
	runs := NewAgentRunService(pool)
	task, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "measured delivery", Assignee: author, Contract: &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "verified 42"}}, Gate: TaskGate{Kind: "human", ReviewerID: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []AgentRunStatus{AgentRunStatusFailed, AgentRunStatusCompleted} {
		run, e := runs.StartRun(ctx, StartRunInput{AgentID: author, ChannelID: channel, TriggerType: "task", Source: "claude"})
		if e != nil {
			t.Fatal(e)
		}
		if e = runs.LinkTask(ctx, LinkRunTaskInput{RunID: run.ID, TaskID: task.ID}); e != nil {
			t.Fatal(e)
		}
		if _, e = runs.MarkBackendStarted(ctx, run.ID); e != nil {
			t.Fatal(e)
		}
		if _, e = runs.FinishRun(ctx, FinishRunInput{RunID: run.ID, Status: status, Usage: map[string]int{"input_tokens": 100, "output_tokens": 20}}); e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			if _, e = pool.Exec(ctx, `UPDATE agent_run_token_usage SET created_at=now()-interval '1 year' WHERE run_id=$1`, run.ID); e != nil {
				t.Fatal(e)
			}
		}
		task, e = tasks.SubmitDelivery(ctx, channel, task.ID, author, run.ID, TaskSubmitRequest{ExpectedTaskVersion: task.Version, IdempotencyKey: string(status), ArtifactVersion: string(status), Handoff: TaskHandoff{Summary: "42"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "42"}}})
		if e != nil {
			t.Fatal(e)
		}
		decision := "rejected"
		if i == 1 {
			decision = "accepted"
		}
		task, e = tasks.ReviewDelivery(ctx, channel, task.ID, owner, TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: string(status), Decision: decision, Reason: "actual check", IdempotencyKey: decision, Checks: []TaskCheck{{RequirementID: "R1", Passed: i == 1, EvidenceIDs: []string{"E1"}, Reason: "checked"}}})
		if e != nil {
			t.Fatal(e)
		}
	}
	minutes := 2.5
	observation := TaskObservationRequest{Kind: "intervention", Minutes: &minutes, Note: "read actual evidence", IdempotencyKey: "minutes"}
	id, err := tasks.RecordTaskObservation(ctx, channel, task.ID, owner, observation)
	if err != nil {
		t.Fatal(err)
	}
	again, err := tasks.RecordTaskObservation(ctx, channel, task.ID, owner, observation)
	if err != nil || id != again {
		t.Fatal("idempotence", again, err)
	}
	observation.Note = "changed"
	if _, err = tasks.RecordTaskObservation(ctx, channel, task.ID, owner, observation); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatal("changed retry", err)
	}
	observation.IdempotencyKey = "agent-minutes"
	if _, err = tasks.RecordTaskObservation(ctx, channel, task.ID, author, observation); !errors.Is(err, ErrTaskNotReviewer) {
		t.Fatal("agent invented human minutes", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE task_observations SET note='rewrite' WHERE id=$1`, id); err == nil {
		t.Fatal("rewrote observation")
	}
	if _, err = tasks.RecordTaskObservation(ctx, channel, task.ID, owner, TaskObservationRequest{Kind: "comparison", Category: "arithmetic / simple", Note: "fixed task class", IdempotencyKey: "group"}); err != nil {
		t.Fatal(err)
	}
	metric, err := tasks.TaskMetrics(ctx, channel, task.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(metric.Reworks) != 1 {
		t.Fatalf("reworks %+v", metric)
	}
	classify := TaskObservationRequest{Kind: "rework", Category: "evidence", ReviewID: metric.Reworks[0].ID, Note: "missing proof", IdempotencyKey: "classify"}
	if _, err = tasks.RecordTaskObservation(ctx, channel, task.ID, owner, classify); err != nil {
		t.Fatal(err)
	}
	classify.IdempotencyKey = "duplicate"
	if _, err = tasks.RecordTaskObservation(ctx, channel, task.ID, owner, classify); err == nil {
		t.Fatal("double-counted classification")
	}
	metric, err = tasks.TaskMetrics(ctx, channel, task.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	work, e := GetAgentWork(ctx, pool, author, author)
	if e != nil {
		t.Fatal(e)
	}
	var workState struct {
		Feedback []struct {
			Kind string `json:"kind"`
			Note string `json:"note"`
		} `json:"feedback"`
	}
	if e = json.Unmarshal(work, &workState); e != nil || len(workState.Feedback) != 3 {
		t.Fatal("Agent lost actual review/observation feedback", string(work), e)
	}
	groups := deliveryCohorts([]TaskDeliveryMetric{*metric})
	g := groups[0]
	if !metric.Qualified || len(metric.Runs) != 2 || g.ActualTokens != 240 || g.FailedRuns != 1 || g.HumanMinutes != 2.5 || g.HumanRecordedTasks != 1 || g.Reworks != 1 || g.ReworkReasons["evidence"] != 1 || g.ComparableTasks != 1 {
		t.Fatalf("lost lifetime costs or classification: %+v / %+v", metric, g)
	}
	peer := taskSubmitUser(t, pool)
	taskSubmitMember(t, pool, channel, "user", peer)
	peerMetric, e := tasks.TaskMetrics(ctx, channel, task.ID, peer)
	if e != nil {
		t.Fatal(e)
	}
	if string(peerMetric.Runs[0].Budget) != "null" {
		t.Fatal("shared Task leaked private budget")
	}
	if deliveryCohorts([]TaskDeliveryMetric{*peerMetric})[0].ComparableTasks != 0 {
		t.Fatal("unknown budget counted as comparable")
	}
	other := taskSubmitUser(t, pool)
	if _, err = tasks.TaskMetrics(ctx, channel, task.ID, other); err == nil {
		t.Fatal("nonmember read task metrics")
	}
	report, err := runs.dashboardDelivery(ctx, other, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 0 {
		t.Fatal("dashboard leaked another channel")
	}
	// A shared Run stays visible, but cannot become an independent comparable sample.
	task2, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "shared work", Assignee: author})
	if err != nil {
		t.Fatal(err)
	}
	if err = runs.LinkTask(ctx, LinkRunTaskInput{RunID: metric.Runs[0].ID, TaskID: task2.ID, Role: "related"}); err != nil {
		t.Fatal(err)
	}
	metric, err = tasks.TaskMetrics(ctx, channel, task.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	shared := deliveryCohorts([]TaskDeliveryMetric{*metric, *metric})[0]
	if shared.ActualTokens != 240 || shared.SharedRuns != 1 || shared.ComparableTasks != 0 {
		t.Fatalf("shared accounting %+v", shared)
	}
}

func TestDeliveryCohortsKeepUnknownAndFailedSamples(t *testing.T) {
	tokens := int64(90)
	metrics := []TaskDeliveryMetric{
		{Cohort: "same", Qualified: true, Runs: []DeliveryRunMetric{{ID: "good", Model: "claude/haiku", Budget: []byte(`{}`), ActualTokens: &tokens, AccountedTokens: 90}}},
		{Cohort: "same", Qualified: false, Runs: []DeliveryRunMetric{{ID: "bad", Model: "claude/haiku", Budget: []byte(`{}`), ActualTokens: &tokens, AccountedTokens: 90, Status: "failed"}}},
		{Cohort: "same", Runs: []DeliveryRunMetric{{ID: "unknown", Model: "claude/haiku", Budget: []byte(`null`), AccountedTokens: 100}}},
	}
	groups := deliveryCohorts(metrics)
	if len(groups) != 2 {
		t.Fatalf("mixed known and unknown budgets %+v", groups)
	}
	for _, g := range groups {
		if g.UnknownRuns == 1 {
			if g.ComparableTasks != 0 || g.AccountedTokens != 100 {
				t.Fatal(g)
			}
		} else if g.ComparableTokens != 180 || g.ComparableQualified != 1 || g.Tasks != 2 || g.FailedRuns != 1 {
			t.Fatalf("hid failed sample %+v", g)
		}
	}
}

func TestCodeGateDeliveryMetricsAreKnownZeroTokens(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	tasks := NewTaskService(pool)
	task, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "code gate cost", Assignee: a})
	if err != nil {
		t.Fatal(err)
	}
	runs := NewAgentRunService(pool)
	run, err := runs.StartRun(ctx, StartRunInput{AgentID: a, ChannelID: channel, TriggerType: AgentRunTriggerTask, Source: "code_gate"})
	if err != nil {
		t.Fatal(err)
	}
	if err = runs.LinkTask(ctx, LinkRunTaskInput{RunID: run.ID, TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.MarkBackendStarted(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: run.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	metric, err := tasks.TaskMetrics(ctx, channel, task.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(metric.Runs) != 1 || metric.Runs[0].Model != "code_gate/process" || metric.Runs[0].ActualTokens == nil || *metric.Runs[0].ActualTokens != 0 || metric.Runs[0].ExecutionSeconds == nil || string(metric.Runs[0].Budget) != "{\"code_gate_tokens\": 0}" {
		t.Fatal("Code Gate was treated as unknown LLM usage", metric)
	}
}

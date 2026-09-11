package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Uses real PostgreSQL/services. Runtime execution is covered separately by E2E.
func TestSelectionAgentInitiationAndIndependentReview(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	original := taskSubmitAgent(t, pool, owner)
	receiver := taskSubmitAgent(t, pool, owner)
	reviewer := taskSubmitAgent(t, pool, owner)
	home := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, home, "user", owner)
	taskSubmitMember(t, pool, home, "agent", original)
	taskSubmitMember(t, pool, home, "agent", receiver)
	computer := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers(id,name,owner_id,status,daemon_id) VALUES($1,'Selection review test',$2,'online',$3)`, computer, owner, "selection-review-"+computer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id=$2,model_provider='claude',model_name='haiku' WHERE owner_id=$1`, owner, computer); err != nil {
		t.Fatal(err)
	}
	candidate, err := CreateAgentRevision(ctx, pool, original, original, AgentRevisionRequest{Summary: "measured improvement", Config: AgentRevisionConfig{SystemPrompt: "return structured output", ModelProvider: "claude", ModelName: "haiku"}})
	if err != nil {
		t.Fatal(err)
	}
	req := AgentSelectionRequest{ChannelID: home, CandidateRevisionID: candidate, ReceiverAgentID: receiver, Problem: "output cannot be consumed", Change: "return structured output", Cases: []SelectionCase{{Title: "parse output", Input: "calculate 20+22", Requirements: []TaskRequirement{{ID: "R1", Text: "output contains 42"}}}}, TeamCheck: "parse exact candidate output", TokenBudget: 1000, IdempotencyKey: "agent-independent"}
	if _, err = CreateAgentSelection(ctx, pool, original, original, req); err == nil {
		t.Fatal("Agent created trials requiring manual per-case approval")
	}
	req.ReviewerAgentID = original
	if _, err = CreateAgentSelection(ctx, pool, original, original, req); err == nil {
		t.Fatal("original selected itself as reviewer")
	}
	req.ReviewerAgentID = reviewer
	if _, err = CreateAgentSelection(ctx, pool, original, receiver, req); err != ErrAgentWorkForbidden {
		t.Fatal("unrelated Agent acted for the original", err)
	}
	id, err := CreateAgentSelection(ctx, pool, original, original, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE channels SET is_archived=true WHERE id IN (SELECT channel_id FROM agent_selection_trials WHERE selection_id=$1)`, id)
	})
	if replay, err := CreateAgentSelection(ctx, pool, original, original, req); err != nil || replay != id {
		t.Fatal("lost Agent creation idempotency", replay, err)
	}
	var clone string
	if err = pool.QueryRow(ctx, `SELECT agent_id::text FROM agent_selection_trials WHERE selection_id=$1 AND arm='reviewer'`, id).Scan(&clone); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err = pool.QueryRow(ctx, `SELECT count(*)=3 AND bool_and(t.contract->'gate'->>'kind'='agent' AND t.contract->'gate'->>'reviewer_id'=$2 AND t.claimer_id::text<>$2) FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=$1`, id, clone).Scan(&valid); err != nil || !valid {
		t.Fatal("not all three arms use the fixed independent reviewer", valid, err)
	}
	var taskID, channelID, worker string
	if err = pool.QueryRow(ctx, `SELECT t.id::text,t.channel_id::text,t.claimer_id::text FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=$1 AND st.arm='candidate'`, id).Scan(&taskID, &channelID, &worker); err != nil {
		t.Fatal(err)
	}
	runs := NewAgentRunService(pool)
	if _, err = runs.StartRun(ctx, StartRunInput{AgentID: clone, ChannelID: channelID, TriggerType: AgentRunTriggerMessage}); err == nil {
		t.Fatal("reviewer started arbitrary work")
	}
	tasks := NewTaskService(pool)
	submitted, err := tasks.SubmitDelivery(ctx, channelID, taskID, worker, "", TaskSubmitRequest{ExpectedTaskVersion: 1, IdempotencyKey: "sub", ArtifactVersion: "output-v1", Handoff: TaskHandoff{Summary: "42"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "42"}}})
	if err != nil {
		t.Fatal(err)
	}
	input := StartRunInput{AgentID: clone, ChannelID: channelID, TriggerType: AgentRunTriggerTask, Source: "claude", ReviewSubmissionID: uuid.NewString()}
	if _, err = runs.StartRun(ctx, input); err == nil {
		t.Fatal("reviewer started with unrelated submission")
	}
	input.ReviewSubmissionID = submitted.CurrentSubmissionID
	run, err := runs.StartRun(ctx, input)
	if err != nil {
		t.Fatal("queued current review could not start", err)
	}
	if _, err = runs.MarkBackendStarted(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: run.ID, Status: AgentRunStatusCompleted, Usage: map[string]int{"input_tokens": 800, "output_tokens": 200}}); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.StartRun(ctx, input); err == nil {
		t.Fatal("reviewer exceeded its lifetime budget")
	}
	if data, err := ListAgentSelections(ctx, pool, original, original); err != nil || len(data) == 0 {
		t.Fatal("Agent cannot read independent review progress", err)
	}
	if err = DecideAgentSelection(ctx, pool, original, original, id, "accepted", "self-adopt"); err != ErrAgentWorkForbidden {
		t.Fatal("Agent acquired its owner's adoption authority", err)
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "accepted", "incomplete"); err == nil {
		t.Fatal("accepted unfinished independent comparison")
	}
}

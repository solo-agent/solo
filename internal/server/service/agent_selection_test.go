package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

func TestSelectionPostgresPlanBudgetAndDecision(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	original := taskSubmitAgent(t, pool, owner)
	receiver := taskSubmitAgent(t, pool, owner)
	home := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, home, "user", owner)
	computer := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers(id,name,owner_id,status,daemon_id) VALUES($1,'Selection test',$2,'online',$3)`, computer, owner, "selection-test-"+computer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id=$2,model_provider='claude',model_name='haiku' WHERE owner_id=$1`, owner, computer); err != nil {
		t.Fatal(err)
	}
	candidate, err := CreateAgentRevision(ctx, pool, original, owner, AgentRevisionRequest{Summary: "paired improvement", Config: AgentRevisionConfig{SystemPrompt: "improved", ModelProvider: "claude", ModelName: "haiku"}})
	if err != nil {
		t.Fatal(err)
	}
	req := AgentSelectionRequest{CandidateRevisionID: candidate, ReceiverAgentID: receiver, Problem: "Task showed incompatible output", Change: "produce usable JSON", Cases: []SelectionCase{{Title: "parse the result", Input: "return 42", Requirements: []TaskRequirement{{ID: "R1", Text: "actual output is usable"}}}}, TeamCheck: "parse exact candidate evidence", TokenBudget: 1000, IdempotencyKey: "paired"}
	sharedWorkspace, sharedChannel := uuid.NewString(), uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO workspaces(id,name,created_by) VALUES($1,'Selection shared space',$2)`, sharedWorkspace, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'owner')`, sharedWorkspace, owner); err != nil {
		t.Fatal(err)
	}
	ctx = serverworkspace.WithScope(ctx, serverworkspace.Scope{ID: sharedWorkspace, Role: "owner"})
	if _, err = CreateAgentSelection(ctx, pool, original, owner, req); err != ErrAgentWorkForbidden {
		t.Fatal("created comparison where original does not participate", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO channels(id,workspace_id,name,created_by) VALUES($1,$2,'shared-selection',$3)`, sharedChannel, sharedWorkspace, owner); err != nil {
		t.Fatal(err)
	}
	taskSubmitMember(t, pool, sharedChannel, "user", owner)
	taskSubmitMember(t, pool, sharedChannel, "agent", original)
	taskSubmitMember(t, pool, sharedChannel, "agent", receiver)
	req.ChannelID = sharedChannel
	id, err := CreateAgentSelection(ctx, pool, original, owner, req)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := CreateAgentSelection(ctx, pool, original, owner, req)
	if err != nil || replay != id {
		t.Fatal("lost creation idempotency", err)
	}
	var correctScope bool
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*)=3 FROM agent_selection_trials tr JOIN channels c ON c.id=tr.channel_id WHERE tr.selection_id=$1 AND c.workspace_id=$2) AND (SELECT home_channel_id=$3 FROM agents WHERE id=$4)`, id, sharedWorkspace, home, original).Scan(&correctScope); err != nil || !correctScope {
		t.Fatal("comparison did not follow active Workspace or moved original home", correctScope, err)
	}
	homeContext := serverworkspace.WithScope(ctx, serverworkspace.Scope{ID: serverworkspace.PublicID})
	if _, err = CreateAgentSelection(homeContext, pool, original, owner, req); err != ErrTaskVersionConflict {
		t.Fatal("replayed comparison into a different Workspace", err)
	}
	if data, err := ListAgentSelections(homeContext, pool, original, owner); err != nil || string(data) != "[]" {
		t.Fatal("listed another Workspace's comparison", string(data), err)
	}
	req.TeamCheck = "different"
	if _, err = CreateAgentSelection(ctx, pool, original, owner, req); err == nil {
		t.Fatal("reused key changed plan")
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "accepted", "not run"); err == nil {
		t.Fatal("accepted empty evaluation")
	}
	if err = DecideAgentSelection(ctx, pool, original, receiver, id, "observe", "self-select"); err == nil {
		t.Fatal("Agent selected itself")
	}
	if _, err = pool.Exec(ctx, `UPDATE agent_selections SET plan='{}' WHERE id=$1`, id); err == nil {
		t.Fatal("changed frozen plan")
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "observe", "continue measurement"); err != nil {
		t.Fatal(err)
	}
	var candidateSub string
	for _, arm := range []string{"baseline", "candidate", "receiver"} {
		var agentID, channelID, taskID string
		if err = pool.QueryRow(ctx, `SELECT tr.agent_id::text,tr.channel_id::text,st.task_id::text FROM agent_selection_trials tr JOIN agent_selection_tasks st USING(selection_id,arm) WHERE tr.selection_id=$1 AND tr.arm=$2`, id, arm).Scan(&agentID, &channelID, &taskID); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE tasks SET title='change input' WHERE id=$1`, taskID); err == nil {
			t.Fatal("changed fixed input")
		}
		runs := NewAgentRunService(pool)
		if arm == "baseline" {
			failed, err := runs.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: "task", Source: "claude", SelectionTaskID: taskID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = runs.MarkBackendStarted(ctx, failed.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: failed.ID, Status: AgentRunStatusFailed, Usage: map[string]int{"input_tokens": 80, "output_tokens": 20}}); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE agent_run_token_usage SET created_at=now()-interval '1 year' WHERE run_id=$1`, failed.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = runs.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: "message"}); err == nil {
			t.Fatal("uncontrolled message entered comparison")
		}
		run, err := runs.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: "task", Source: "claude", SelectionTaskID: taskID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE agent_runs SET backend_started_at=now() WHERE id=$1`, run.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE agent_selection_tasks SET run_id=$2,attempts=1,dispatched_version=1 WHERE task_id=$1`, taskID, run.ID); err != nil {
			t.Fatal(err)
		}
		if arm == "receiver" {
			if _, err = pool.Exec(ctx, `UPDATE agent_selection_tasks SET input_submission_id=$2 WHERE task_id=$1`, taskID, candidateSub); err != nil {
				t.Fatal(err)
			}
		}
		// A concurrent Run cannot reserve the same remaining lifetime budget.
		if _, err = runs.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: "task", SelectionTaskID: taskID}); err == nil {
			t.Fatal("oversubscribed Selection allowance")
		}
		tasks := NewTaskService(pool)
		task, err := tasks.SubmitDelivery(ctx, channelID, taskID, agentID, run.ID, TaskSubmitRequest{ExpectedTaskVersion: 1, IdempotencyKey: "submit", ArtifactVersion: "v1", Handoff: TaskHandoff{Summary: "42"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "42"}}})
		if err != nil {
			t.Fatal(err)
		}
		requirement := "R1"
		if arm == "receiver" {
			requirement = "compatibility"
		}
		if _, err = tasks.ReviewDelivery(ctx, channelID, taskID, owner, TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: "v1", IdempotencyKey: "accept", Decision: "accepted", Reason: "verified actual output", Checks: []TaskCheck{{RequirementID: requirement, Passed: true, Reason: "parsed", EvidenceIDs: []string{"E1"}}}}); err != nil {
			t.Fatal(err)
		}
		if arm == "candidate" {
			candidateSub = task.CurrentSubmissionID
		}
		if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: run.ID, Status: AgentRunStatusCompleted, Usage: map[string]int{"input_tokens": 100, "output_tokens": 20}}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewAgentService(pool, nil, nil, nil)
	if found, err := service.notifySelectionOutcome(ctx); err != nil || !found {
		t.Fatal("completion did not queue original Agent report", found, err)
	}
	if found, err := service.notifySelectionOutcome(ctx); err != nil || found {
		t.Fatal("completion notification repeated", found, err)
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "accepted", "paired output and consumer verified"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE agent_selection_decisions SET reason='rewrite' WHERE selection_id=$1`, id); err == nil {
		t.Fatal("rewrote decision history")
	}
	// One-team adoption includes other owners' approval, idempotency and pinned Runs.
	peer := taskSubmitUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE channels SET is_archived=true WHERE id=$1`, sharedChannel)
	})
	peerAgent := taskSubmitAgent(t, pool, peer)
	if _, err = pool.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'member')`, sharedWorkspace, peer); err != nil {
		t.Fatal(err)
	}
	taskSubmitMember(t, pool, sharedChannel, "user", peer)
	taskSubmitMember(t, pool, sharedChannel, "agent", peerAgent)
	var scopedBaseline string
	if err = pool.QueryRow(ctx, `SELECT baseline_revision_id::text FROM agent_selections WHERE id=$1`, id).Scan(&scopedBaseline); err != nil {
		t.Fatal(err)
	}
	queued, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: original, ChannelID: sharedChannel, TriggerType: AgentRunTriggerTask})
	if err != nil {
		t.Fatal(err)
	}
	live := ""
	application := AgentRevisionApplicationRequest{ChannelID: sharedChannel, SelectionID: id, ExpectedTeamVersionID: &live, IdempotencyKey: "apply-to-team", Reason: "Use this accepted result only in the named team"}
	if _, err = ApplyAgentRevision(ctx, pool, original, original, candidate, application); err != ErrAgentWorkForbidden {
		t.Fatal("Agent adopted without its owner", err)
	}
	version, err := ApplyAgentRevision(ctx, pool, original, owner, candidate, application)
	if err != nil {
		t.Fatal(err)
	}
	var applied, globalUnchanged, queueUnchanged bool
	if err = pool.QueryRow(ctx, `SELECT team_version_id IS NOT NULL FROM channels WHERE id=$1`, sharedChannel).Scan(&applied); err != nil || applied {
		t.Fatal("applied before the other owner approved", err)
	}
	if replay, err := ApplyAgentRevision(ctx, pool, original, owner, candidate, application); err != nil || replay != version {
		t.Fatal("lost application idempotency", replay, err)
	}
	if _, err = PublishTeamVersion(ctx, pool, sharedChannel, peer, version, "Approve my member's exact version"); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT team_version_id=$2 FROM channels WHERE id=$1),(SELECT solo_agent_config(a)=v.config FROM agents a JOIN agent_revisions v ON v.id=$4 WHERE a.id=$3),(SELECT agent_revision_id=$4 AND team_version_id IS NULL FROM agent_runs WHERE id=$5)`, sharedChannel, version, original, scopedBaseline, queued.ID).Scan(&applied, &globalUnchanged, &queueUnchanged); err != nil || !applied || !globalUnchanged || !queueUnchanged {
		t.Fatal("scope or queued snapshot changed", applied, globalUnchanged, queueUnchanged, err)
	}
	if _, err = NewAgentRunService(pool).FinishRun(ctx, FinishRunInput{RunID: queued.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	after, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: original, ChannelID: sharedChannel, TriggerType: AgentRunTriggerTask})
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT agent_revision_id=$2 AND team_version_id=$3 FROM agent_runs WHERE id=$1`, after.ID, candidate, version).Scan(&applied); err != nil || !applied {
		t.Fatal("subsequent team Run did not pin candidate", err)
	}
	if _, err = NewAgentRunService(pool).FinishRun(ctx, FinishRunInput{RunID: after.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	restore := AgentRevisionApplicationRequest{ChannelID: sharedChannel, Restore: true, ExpectedTeamVersionID: &version, IdempotencyKey: "restore-team", Reason: "Restore the prior team behavior"}
	restored, err := ApplyAgentRevision(ctx, pool, original, owner, scopedBaseline, restore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = PublishTeamVersion(ctx, pool, sharedChannel, peer, restored, "Approve restoring the previous team combination"); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT c.team_version_id=$2 AND EXISTS(SELECT 1 FROM channel_team_versions tv CROSS JOIN LATERAL jsonb_array_elements(tv.lockfile->'members') member WHERE tv.id=c.team_version_id AND member->>'agent_id'=$3 AND member->>'revision_id'=$4) FROM channels c WHERE c.id=$1`, sharedChannel, restored, original, scopedBaseline).Scan(&applied); err != nil || !applied {
		t.Fatal("restore did not preserve the original member revision", err)
	}
	application.IdempotencyKey = "stale-team-application"
	if _, err = ApplyAgentRevision(ctx, pool, original, owner, candidate, application); err != ErrTaskVersionConflict {
		t.Fatal("stale team application was accepted", err)
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, candidate, "", "use accepted comparison"); err != nil {
		t.Fatal(err)
	}
	data, err := ListAgentSelections(ctx, pool, original, owner)
	if err != nil || !json.Valid(data) || string(data) == "[]" {
		t.Fatal("cannot read results", err)
	}
	var baselineCost int64
	if err = pool.QueryRow(ctx, `SELECT COALESCE(sum(actual_tokens),0) FROM agent_run_token_usage u JOIN agent_selection_trials tr ON tr.agent_id=u.agent_id WHERE tr.selection_id=$1 AND tr.arm='baseline'`, id).Scan(&baselineCost); err != nil || baselineCost != 220 {
		t.Fatal("lost failed Run or old-period cost", baselineCost, err)
	}
	var baseline string
	if err = pool.QueryRow(ctx, `SELECT baseline_revision_id::text FROM agent_selections WHERE id=$1`, id).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, baseline, "", "restore baseline"); err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err = pool.QueryRow(ctx, `SELECT task_id::text FROM agent_selection_tasks WHERE selection_id=$1 AND arm='candidate'`, id).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE tasks SET status='in_progress' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM agent_selections WHERE id=$1`, id).Scan(&status); err != nil || status != "observe" {
		t.Fatal("changed evidence retained acceptance", status, err)
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "rejected", "reopened sample needs new comparison"); err != nil {
		t.Fatal(err)
	}
	if err = DecideAgentSelection(ctx, pool, original, owner, id, "accepted", "stale result"); err == nil {
		t.Fatal("accepted reopened sample")
	}
}

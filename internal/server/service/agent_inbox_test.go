package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/pkg/agent"
)

func TestAgentInboxPostgresOrderSingleClaimAndOfflineRecovery(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	other := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO channels(id,name,created_by) VALUES($1,$2,$3)`, other, "inbox-"+other, owner); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{channel, other} {
		taskSubmitMember(t, pool, c, "user", owner)
		taskSubmitMember(t, pool, c, "agent", a)
	}
	svc := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	runs := NewAgentRunService(pool)
	current, err := runs.StartRun(ctx, StartRunInput{AgentID: a, ChannelID: channel, TriggerType: AgentRunTriggerMessage})
	if err != nil {
		t.Fatal(err)
	}
	request := daemonTaskRequest{AgentID: a, ChannelID: channel, Messages: []agent.Message{{Role: agent.RoleUser, Content: "greet once"}}, SystemPrompt: "private config", CustomEnv: map[string]string{"SECRET": "not queued"}, AgentToken: "not queued"}
	if err = svc.enqueueAgentWork(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err = svc.enqueueAgentWork(ctx, request); err != nil {
		t.Fatal(err)
	}
	task, err := NewTaskService(pool).CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "explicit task", Assignee: a})
	if err != nil {
		t.Fatal(err)
	}
	request.OriginTaskID = task.ID
	request.ResultContract = agentResultContractVisibleMessage
	request.Messages[0].Content = "perform task"
	if err = svc.enqueueAgentWork(ctx, request); err != nil {
		t.Fatal(err)
	}
	var id string
	var payload json.RawMessage
	if err = pool.QueryRow(ctx, `SELECT id::text,payload FROM agent_pending_work WHERE agent_id=$1 AND kind='task'`, a).Scan(&id, &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "not queued") || strings.Contains(string(payload), "private config") {
		t.Fatal("persisted private runtime configuration")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = upsertPendingMessageWakeTx(ctx, tx, pendingMessageWake{AgentID: a, ChannelID: other, ScopeKey: "channel", FirstMessageSeq: 1, LatestMessageSeq: 1, RequiresVisibleReply: true}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_work WHERE agent_id=$1`, a).Scan(&count); err != nil || count != 2 {
		t.Fatal("pending idempotence", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_inbox_heads WHERE agent_id=$1`, a).Scan(&count); err != nil || count != 0 {
		t.Fatal("busy Agent had claimable work", count, err)
	}
	if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: current.ID, Status: AgentRunStatusCompleted, Usage: map[string]int{"input_tokens": 0, "output_tokens": 0}}); err != nil {
		t.Fatal(err)
	}
	// This is the same SQL order and lock used by all production dispatchers.
	var kind string
	if err = pool.QueryRow(ctx, `SELECT kind FROM agent_inbox_heads WHERE agent_id=$1`, a).Scan(&kind); err != nil || kind != "task" {
		t.Fatal("explicit work lost to older greeting", kind, err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Error(e)
				return
			}
			defer tx.Rollback(ctx)
			if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "agent-work:"+a); e != nil {
				t.Error(e)
				return
			}
			ok, e := agentInboxTurnTx(ctx, tx, a, "task", id)
			if e != nil {
				t.Error(e)
				return
			}
			if !ok {
				return
			}
			run, e := runs.startRunTx(ctx, tx, StartRunInput{AgentID: a, ChannelID: channel, TriggerType: AgentRunTriggerTask})
			if e != nil {
				t.Error(e)
				return
			}
			if _, e = tx.Exec(ctx, `UPDATE agent_pending_work SET status='dispatched',run_id=$2 WHERE id=$1`, id, run.ID); e != nil {
				t.Error(e)
				return
			}
			if e = tx.Commit(ctx); e != nil {
				t.Error(e)
				return
			}
			winners.Add(1)
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("concurrent claims", winners.Load())
	}
	var active string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM agent_runs WHERE agent_id=$1 AND finished_at IS NULL`, a).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: active, Status: AgentRunStatusCompleted, Usage: map[string]int{"input_tokens": 0, "output_tokens": 0}}); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT kind FROM agent_inbox_heads WHERE agent_id=$1`, a).Scan(&kind); err != nil || kind != "message" {
		t.Fatal("message did not follow older explicit Task", kind, err)
	}
	// Retire the synthetic pending source, then a new service instance recovers
	// the persisted greeting without any in-memory queue or available Computer.
	if _, err = pool.Exec(ctx, `DELETE FROM agent_pending_message_wakes WHERE agent_id=$1`, a); err != nil {
		t.Fatal(err)
	}
	restarted := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	if found, e := restarted.dispatchPendingAgentWork(ctx); e != nil || !found {
		t.Fatal("restart lost pending work", found, e)
	}
	var retained bool
	if err = pool.QueryRow(ctx, `SELECT status='pending' AND run_id IS NULL AND last_error<>'' AND next_attempt_at>now() FROM agent_pending_work WHERE agent_id=$1 AND kind='greeting'`, a).Scan(&retained); err != nil || !retained {
		t.Fatal("offline source was lost", retained, err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND member_type='agent' AND member_id=$2`, channel, a); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.dispatchPendingAgentWork(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status='cancelled' FROM agent_pending_work WHERE agent_id=$1 AND kind='greeting'`, a).Scan(&retained); err != nil || !retained {
		t.Fatal("withdrawn membership left runnable work", retained, err)
	}
}

func TestAgentInboxHeadIncludesActualReview(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	reviewer := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	for _, a := range []string{author, reviewer} {
		taskSubmitMember(t, pool, channel, "agent", a)
	}
	taskSubmitMember(t, pool, channel, "user", owner)
	tasks := NewTaskService(pool)
	task, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "actual review", Assignee: author, Contract: &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "42"}}, Gate: TaskGate{Kind: "agent", ReviewerID: reviewer}}})
	if err != nil {
		t.Fatal(err)
	}
	task, err = tasks.SubmitDelivery(ctx, channel, task.ID, author, "", TaskSubmitRequest{ExpectedTaskVersion: task.Version, IdempotencyKey: "review-inbox", ArtifactVersion: "v1", Handoff: TaskHandoff{Summary: "42"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "42"}}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	ok, err := agentInboxTurnTx(ctx, tx, reviewer, "review", task.CurrentSubmissionID)
	if err != nil || !ok {
		t.Fatal("review missing from shared Inbox", ok, err)
	}
	if _, err = tx.Exec(ctx, `UPDATE task_review_deliveries SET next_attempt_at=now()+interval '1 minute' WHERE submission_id=$1`, task.CurrentSubmissionID); err != nil {
		t.Fatal(err)
	}
	ok, err = agentInboxTurnTx(ctx, tx, reviewer, "review", task.CurrentSubmissionID)
	if err != nil || ok {
		t.Fatal("backoff did not release the head", ok, err)
	}
}

func TestAgentInboxMuteDoesNotRequeueMessagesOrEraseResponsibility(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	message := agentRunMessage(t, pool, channel, owner)
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id=$1`, message).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	task, err := NewTaskService(pool).CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "mute retains responsibility", Assignee: a})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	if err = svc.enqueueAgentWork(ctx, daemonTaskRequest{AgentID: a, ChannelID: channel, OriginTaskID: task.ID, ResultContract: agentResultContractVisibleMessage}); err != nil {
		t.Fatal(err)
	}
	wake := pendingMessageWake{AgentID: a, ChannelID: channel, ScopeKey: "channel", FirstMessageSeq: seq, LatestMessageSeq: seq, RequiresVisibleReply: true}
	queue := func() {
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if e = upsertPendingMessageWakeTx(ctx, tx, wake); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	queue()
	if err = SetAgentAttention(ctx, pool, a, owner, "", "nothing"); err != nil {
		t.Fatal(err)
	}
	queue() // An old routing result/recovery must respect the newly persisted mute.
	var messages, work int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id=$1),(SELECT count(*) FROM agent_pending_work WHERE agent_id=$1 AND status='pending')`, a).Scan(&messages, &work); err != nil || messages != 0 || work != 1 {
		t.Fatal("mute lost work or queued muted input", messages, work, err)
	}
	var retained bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id=$1 AND NOT is_deleted) AND EXISTS(SELECT 1 FROM tasks WHERE id=$2 AND claimer_id=$3)`, message, task.ID, a).Scan(&retained); err != nil || !retained {
		t.Fatal("mute erased original message or responsibility", err)
	}
}

func TestAgentInboxCancelledThinkingReturnReleasesItsOriginalNode(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	thinking := NewThinkingService(pool)
	space, err := thinking.Ensure(ctx, channel, owner)
	if err != nil {
		t.Fatal(err)
	}
	child, err := thinking.CreateChild(ctx, channel, space.Nodes[0].ID, "queued return", owner, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO messages(id,channel_id,thinking_node_id,sender_type,sender_id,content) VALUES($1,$2,$3,'user',$4,'finished the branch discussion')`, uuid.NewString(), channel, child.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = thinking.BeginReturn(ctx, channel, child.ID); err != nil {
		t.Fatal(err)
	}
	worker := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	req := daemonTaskRequest{AgentID: a, ChannelID: channel, NodeID: child.ID, ReturnHandoff: true, ResultContract: agentResultContractHandoff, Messages: []agent.Message{{Role: agent.RoleUser, SenderID: "system", Content: thinkingReturnPrompt}}}
	if err = worker.enqueueAgentWork(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND member_id=$2 AND member_type='agent'`, channel, a); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.dispatchPendingAgentWork(ctx); err != nil {
		t.Fatal(err)
	}
	var released bool
	if err = pool.QueryRow(ctx, `SELECT returning_at IS NULL AND returned_at IS NULL FROM thinking_nodes WHERE id=$1`, child.ID).Scan(&released); err != nil || !released {
		t.Fatal("cancelled pending Return left the branch locked", released, err)
	}
	taskSubmitMember(t, pool, channel, "agent", a)
	if _, err = thinking.BeginReturn(ctx, channel, child.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.enqueueAgentWork(ctx, req); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_work WHERE agent_id=$1 AND status='pending' AND thinking_node_id=$2`, a, child.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("new explicit Return was swallowed by its older request", count, err)
	}
}

func TestAgentInboxGreetingDeduplicatesOneMembershipOnly(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	svc := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	req := daemonTaskRequest{AgentID: a, ChannelID: channel, Messages: []agent.Message{{Role: agent.RoleUser, Content: "greet"}}}
	for i := 0; i < 2; i++ {
		if err := svc.enqueueAgentWork(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_work WHERE agent_id=$1`, a).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND member_id=$2 AND member_type='agent'`, channel, a); err != nil {
		t.Fatal(err)
	}
	taskSubmitMember(t, pool, channel, "agent", a)
	if err := svc.enqueueAgentWork(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_work WHERE agent_id=$1`, a).Scan(&count); err != nil || count != 2 {
		t.Fatal("rejoin was swallowed", count, err)
	}
}

func TestAgentInboxUnclaimCancelsAssignmentButPreservesArtifactLeader(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	svc := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	tasks := NewTaskService(pool)
	assigned, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "pending assignment", Assignee: a})
	if err != nil {
		t.Fatal(err)
	}
	req := daemonTaskRequest{AgentID: a, ChannelID: channel, OriginTaskID: assigned.ID, ResultContract: agentResultContractVisibleMessage}
	if err = svc.enqueueAgentWork(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.UnclaimTask(ctx, channel, assigned.ID, a); err != nil {
		t.Fatal(err)
	}
	if found, e := svc.dispatchPendingAgentWork(ctx); e != nil || !found {
		t.Fatal(found, e)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM agent_pending_work WHERE task_id=$1`, assigned.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatal("cancelled assignment restarted", status, err)
	}
	artifact, err := tasks.CreateTask(ctx, channel, a, TaskCreateRequest{Title: "human-owned display"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.ClaimTask(ctx, channel, artifact.ID, owner); err != nil {
		t.Fatal(err)
	}
	req.OriginTaskID = artifact.ID
	req.ResultContract = agentResultContractNone
	if err = svc.enqueueAgentWork(ctx, req); err != nil {
		t.Fatal(err)
	}
	if found, e := svc.dispatchPendingAgentWork(ctx); e != nil || !found {
		t.Fatal(found, e)
	}
	var retained bool
	if err = pool.QueryRow(ctx, `SELECT status='pending' AND last_error<>'' AND run_id IS NULL FROM agent_pending_work WHERE task_id=$1`, artifact.ID).Scan(&retained); err != nil || !retained {
		t.Fatal("valid artifact request was cancelled instead of waiting for Computer", retained, err)
	}
}

func TestRejectedTaskReturnsToOriginalTeamInbox(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	agents := NewAgentService(pool, NewDaemonManager(pool, nil), nil, nil)
	tasks := NewTaskService(pool)
	notifier := NewAgentNotifier(pool, nil, agents)
	tasks.SetAgentNotifier(notifier)
	task, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "return to same team", Assignee: author, Contract: &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "requested result"}}, Gate: TaskGate{Kind: "human", ReviewerID: owner, HumanReviewMode: "decision"}}})
	if err != nil {
		t.Fatal(err)
	}
	task, err = tasks.SubmitDelivery(ctx, channel, task.ID, author, "", TaskSubmitRequest{ExpectedTaskVersion: task.Version, IdempotencyKey: "first", ArtifactVersion: "v1", Handoff: TaskHandoff{Summary: "needs correction"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "40"}}})
	if err != nil {
		t.Fatal(err)
	}
	version, err := PublishTeamVersion(ctx, pool, channel, owner, "", "approved team")
	if err != nil {
		t.Fatal(err)
	}
	review := TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: "v1", Decision: "rejected", Reason: "Use the approved team and recompute", IdempotencyKey: "rework"}
	if _, err = tasks.ReviewDelivery(ctx, channel, task.ID, owner, review); err != nil {
		t.Fatal(err)
	}
	if err = notifier.NotifyRejected(ctx, task.ID, owner, review.Reason); err != nil {
		t.Fatal(err)
	}
	var count int
	var payload daemonTaskRequest
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_work WHERE task_id=$1`, task.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate or missing rework", count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT payload FROM agent_pending_work WHERE task_id=$1`, task.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChannelID != channel || payload.ThreadID == "" || payload.OriginTaskID != task.ID || payload.AgentID != author || !strings.Contains(payload.Messages[0].Content, review.Reason) {
		t.Fatalf("lost task context: %+v", payload)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dm_members WHERE member_type='agent' AND member_id=$1`, author).Scan(&count); err != nil || count != 0 {
		t.Fatal("created unrelated DM", count, err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: author, ChannelID: payload.ChannelID, ThreadID: payload.ThreadID, TriggerType: AgentRunTriggerTask})
	if err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = pool.QueryRow(ctx, `SELECT team_version_id::text FROM agent_runs WHERE id=$1`, run.ID).Scan(&actual); err != nil || actual != version {
		t.Fatal("rework lost approved team snapshot", actual, version, err)
	}
}

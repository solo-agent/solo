package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAgentAttentionAndWorkMarksPostgres(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	outsider := taskSubmitUser(t, pool)
	id := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", id)
	svc := NewAgentService(pool, nil, nil, nil)
	req := wakeRouteRequest{Scope: wakeScopeChannel, ChannelID: channel}
	if err := SetAgentAttention(ctx, pool, id, outsider, "", "nothing"); !errors.Is(err, ErrAgentWorkForbidden) {
		t.Fatalf("foreign policy change: %v", err)
	}
	if err := SetAgentAttention(ctx, pool, id, owner, "", "mentions"); err != nil {
		t.Fatal(err)
	}
	ids, err := svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 0 {
		t.Fatalf("ordinary message received in mentions policy: %v %v", ids, err)
	}
	req.MentionedAgentIDs = []string{id}
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 1 {
		t.Fatalf("explicit mention lost: %v %v", ids, err)
	}
	if err := SetAgentAttention(ctx, pool, id, owner, channel, "nothing"); err != nil {
		t.Fatal(err)
	}
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 0 {
		t.Fatalf("nothing policy ignored: %v %v", ids, err)
	}
	if err := SetAgentAttention(ctx, pool, id, owner, channel, "all"); err != nil {
		t.Fatal(err)
	}
	root := agentRunMessage(t, pool, channel, owner)
	thread, _, err := NewThreadService(pool).GetOrCreateThread(ctx, channel, root)
	if err != nil {
		t.Fatal(err)
	}
	wake := pendingMessageWake{AgentID: id, ChannelID: channel, ThreadID: thread, ScopeKey: "thread:" + thread, FirstMessageSeq: 1, LatestMessageSeq: 1}
	queue := func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = upsertPendingMessageWakeTx(ctx, tx, wake); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	queue()
	if err := NewThreadService(pool).Unfollow(ctx, id, thread); err != nil {
		t.Fatal(err)
	}
	queue() // a delayed routing result must not restore an exited subscription
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id=$1 AND thread_id=$2`, id, thread).Scan(&queued); err != nil || queued != 0 {
		t.Fatal("unfollow restored ordinary work", queued, err)
	}
	wake.RequiresVisibleReply = true
	queue()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id=$1 AND thread_id=$2`, id, thread).Scan(&queued); err != nil || queued != 1 {
		t.Fatal("unfollow discarded explicit mention", queued, err)
	}
	req.Scope = wakeScopeThread
	req.ThreadID = thread
	req.MentionedAgentIDs = nil
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 0 {
		t.Fatalf("unfollow ignored: %v %v", ids, err)
	}
	req.MentionedAgentIDs = []string{id}
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 1 {
		t.Fatalf("explicit mention cannot wake unfollowed agent: %v %v", ids, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,thread_id,sender_type,sender_id,content) VALUES($1,$2,$3,'agent',$4,'reply')`, uuid.NewString(), channel, thread, id); err != nil {
		t.Fatal(err)
	}
	req.MentionedAgentIDs = nil
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 1 {
		t.Fatalf("reply did not restore follow: %v %v", ids, err)
	}
	if err = SetAgentAttention(ctx, pool, id, owner, channel, "mentions"); err != nil {
		t.Fatal(err)
	}
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 1 {
		t.Fatal("followed Thread lost under mentions policy", ids, err)
	}
	if err = SetAgentAttention(ctx, pool, id, owner, channel, "nothing"); err != nil {
		t.Fatal(err)
	}
	ids, err = svc.filterAttention(ctx, []string{id}, req)
	if err != nil || len(ids) != 0 {
		t.Fatal("nothing policy received followed Thread", ids, err)
	}
	mark := AgentWorkMarkRequest{ChannelID: channel, SourceMessageID: root, Description: "follow up after test", NextAction: "check outcome", IdempotencyKey: "promise-1"}
	markID, err := SaveAgentWorkMark(ctx, pool, id, id, mark)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := SaveAgentWorkMark(ctx, pool, id, owner, mark)
	if err != nil || duplicate != markID {
		t.Fatalf("idempotent mark: %v", err)
	}
	if _, err := GetAgentWork(ctx, pool, id, outsider); !errors.Is(err, ErrAgentWorkForbidden) {
		t.Fatalf("private inbox leaked: %v", err)
	}
	work, err := GetAgentWork(ctx, pool, id, owner)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Marks []json.RawMessage `json:"marks"`
	}
	if err := json.Unmarshal(work, &state); err != nil || len(state.Marks) != 1 {
		t.Fatalf("missing obligation: %s %v", work, err)
	}
	if _, err := SaveAgentWorkMark(ctx, pool, id, owner, AgentWorkMarkRequest{ID: markID, Status: "resolved", Resolution: "verified persistent result"}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_work_marks WHERE id=$1`, markID).Scan(&status); err != nil || status != "resolved" {
		t.Fatalf("resolution not persisted: %s %v", status, err)
	}
}

func TestHumanCorrectionOnlyRoutesToItsObligatedAgent(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	b := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "agent", a)
	taskSubmitMember(t, pool, channel, "agent", b)
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: a, ChannelID: channel, TriggerType: AgentRunTriggerMessage})
	if err != nil {
		t.Fatal(err)
	}
	message := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content,metadata) VALUES($1,$2,'user',$3,'correction',jsonb_build_object('correction_of_run_id',$4::text))`, message, channel, owner, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = SetAgentAttention(ctx, pool, a, owner, "", "nothing"); err != nil {
		t.Fatal(err)
	}
	svc := NewAgentService(pool, nil, nil, nil)
	targets, _ := svc.routeWakeTargets(ctx, []agentChannelInfo{{ID: a}, {ID: b}}, wakeRouteRequest{ChannelID: channel, TriggerMessageID: message, Scope: wakeScopeChannel})
	if len(targets) != 1 || targets[0].ID != a {
		t.Fatalf("correction woke wrong Agents: %+v", targets)
	}
}

func TestHumanCorrectionFreshnessPagesAndPersistsPostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	owner := agentRunUser(t, pool)
	channel := agentRunChannel(t, pool, owner)
	id := agentRunAgent(t, pool, owner)
	trigger := agentRunMessage(t, pool, channel, owner)
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id=$1`, trigger).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: id, ChannelID: channel, TriggerType: AgentRunTriggerMessage, Status: AgentRunStatusRunning, FreshnessSeenSeq: seq})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < FreshnessHeldMessageLimit+2; i++ {
		_, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content,metadata) VALUES($1,$2,'user',$3,$4,jsonb_build_object('correction_of_run_id',$5::text))`, uuid.NewString(), channel, owner, fmt.Sprintf("correction-%d", i), run.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	input := AgentSendFreshnessInput{RunID: run.ID, AgentID: id, ChannelID: channel, Draft: "old draft"}
	first := checkFreshnessInTransaction(t, pool, input)
	if first == nil || len(first.Messages) != FreshnessHeldMessageLimit || first.Messages[0].Content != "correction-0" {
		t.Fatalf("first page skipped messages: %+v", first)
	}
	second := checkFreshnessInTransaction(t, pool, input)
	if second == nil || len(second.Messages) != 2 {
		t.Fatalf("second page missing: %+v", second)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = LockMessageScope(ctx, tx, channel, "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = CheckAndHoldAgentSend(ctx, tx, input)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrFreshnessDraftUnchanged) {
		t.Fatalf("old draft escaped after consuming corrections: %v", err)
	}
	// Confirmation names the exact observed sequence, and newer corrections win.
	input.KeepAfterSeq = second.SeenUpToSeq
	tx, _ = pool.Begin(ctx)
	_, err = CheckAndHoldAgentSend(ctx, tx, input)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrFreshnessDraftUnchanged) {
		t.Fatalf("retained draft without reason: %v", err)
	}
	input.FreshnessReason = "Corrections confirm the original answer is still appropriate"
	if hold := checkFreshnessInTransaction(t, pool, input); hold != nil {
		t.Fatalf("explicit retention held: %+v", hold)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content,metadata) VALUES($1,$2,'user',$3,'later correction',jsonb_build_object('correction_of_run_id',$4::text))`, uuid.NewString(), channel, owner, run.ID); err != nil {
		t.Fatal(err)
	}
	latest := checkFreshnessInTransaction(t, pool, input)
	if latest == nil || len(latest.Messages) != 1 {
		t.Fatalf("stale confirmation bypassed new message: %+v", latest)
	}
	tx, _ = pool.Begin(ctx)
	_, err = CheckAndHoldAgentSend(ctx, tx, input)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrFreshnessDraftUnchanged) {
		t.Fatalf("old sequence retained changed context: %v", err)
	}
	input.KeepAfterSeq = latest.SeenUpToSeq
	if hold := checkFreshnessInTransaction(t, pool, input); hold != nil {
		t.Fatalf("latest confirmation held: %+v", hold)
	}
	input.KeepAfterSeq = 0
	input.FreshnessReason = ""
	input.Draft = "revised using all corrections"
	if hold := checkFreshnessInTransaction(t, pool, input); hold != nil {
		t.Fatalf("correction consumed twice: %+v", hold)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_message_consumptions WHERE run_id=$1`, run.ID).Scan(&count); err != nil || count != FreshnessHeldMessageLimit+3 {
		t.Fatalf("consumed count %d: %v", count, err)
	}
	var draft string
	if err := pool.QueryRow(ctx, `SELECT content FROM agent_held_drafts WHERE run_id=$1`, run.ID).Scan(&draft); err != nil || draft != "old draft" {
		t.Fatalf("draft lost: %q %v", draft, err)
	}
}

func TestDiscardAgentDraftPostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	owner := agentRunUser(t, pool)
	other := agentRunUser(t, pool)
	channel := agentRunChannel(t, pool, owner)
	id := agentRunAgent(t, pool, owner)
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: id, ChannelID: channel, TriggerType: AgentRunTriggerMessage, Status: AgentRunStatusRunning})
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	if err = pool.QueryRow(ctx, `INSERT INTO agent_held_drafts(run_id,channel_id,content,based_on_seq) VALUES($1,$2,'original draft',0) RETURNING encode(sha256(convert_to(content,'UTF8')),'hex')`, run.ID, channel).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	req := DiscardAgentDraftRequest{RunID: run.ID, SHA256: digest, Reason: "No reply is needed"}
	if err = DiscardAgentDraft(ctx, pool, id, other, req); !errors.Is(err, ErrAgentWorkForbidden) {
		t.Fatalf("foreign owner discarded draft: %v", err)
	}
	stale := req
	stale.SHA256 = strings.Repeat("a", 64)
	if err = DiscardAgentDraft(ctx, pool, id, owner, stale); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("stale draft discarded: %v", err)
	}
	if err = DiscardAgentDraft(ctx, pool, id, owner, req); err != nil {
		t.Fatal(err)
	}
	if err = DiscardAgentDraft(ctx, pool, id, owner, req); err != nil {
		t.Fatalf("lost idempotence: %v", err)
	}
	var remaining, events int
	var status string
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_held_drafts WHERE run_id=$1),(SELECT count(*) FROM agent_run_events WHERE run_id=$1 AND type='visible_message_draft_discarded'),status FROM agent_runs WHERE id=$1`, run.ID).Scan(&remaining, &events, &status); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 || events != 1 || status != string(AgentRunStatusRunning) {
		t.Fatalf("discard mutated work: %d %d %s", remaining, events, status)
	}
	req.Reason = "different decision"
	if err = DiscardAgentDraft(ctx, pool, id, owner, req); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("idempotency ignored decision: %v", err)
	}
}

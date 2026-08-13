package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/pkg/agent"
)

func TestMessageWakeCoalescesEveryPersistedMessageAndClaimsNextRun(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageIDs := []string{
		agentRunMessage(t, pool, channelID, ownerID),
		agentRunMessage(t, pool, channelID, ownerID),
		agentRunMessage(t, pool, channelID, ownerID),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_message_wake_slots WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	var seqs [3]int64
	for i, messageID := range messageIDs {
		if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, messageID).Scan(&seqs[i]); err != nil {
			t.Fatalf("load message %d seq: %v", i, err)
		}
	}
	active, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, TriggerMessageID: messageIDs[0],
		ChannelID: channelID, Status: AgentRunStatusRunning, ActivityText: "running",
	})
	if err != nil {
		t.Fatalf("start active Run: %v", err)
	}

	svc := NewAgentService(pool, NewDaemonManager(pool, noopBroadcaster{}), noopBroadcaster{}, nil)
	dmn := &DaemonInfo{ID: "test-daemon", Status: DaemonStatusOnline}
	ag := agentChannelInfo{ID: agentID, Name: "Wake Test", ModelProvider: "claude", ModelName: "test"}
	for i := 1; i < len(messageIDs); i++ {
		_, queued, err := svc.claimMessageWake(ctx, dmn, daemonTaskRequest{
			AgentID: agentID, ChannelID: channelID, TriggerMessageID: messageIDs[i],
			Messages:       []agent.Message{{Role: agent.RoleUser, Content: "message", Seq: seqs[i]}},
			ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
			ResultContract: agentResultContractVisibleMessage,
		}, ag, seqs[i], seqs[i])
		if err != nil || !queued {
			t.Fatalf("claim message %d = queued %v, err %v", i, queued, err)
		}
	}

	var first, latest int64
	var visible bool
	if err := pool.QueryRow(ctx, `
		SELECT first_message_seq, latest_message_seq, requires_visible_result
		  FROM agent_pending_message_wakes
		 WHERE agent_id = $1 AND channel_id = $2 AND scope_key = 'channel'`, agentID, channelID,
	).Scan(&first, &latest, &visible); err != nil {
		t.Fatalf("load pending wake: %v", err)
	}
	if first != seqs[1] || latest != seqs[2] || !visible {
		t.Fatalf("pending range = (%d,%d,%v), want (%d,%d,true)", first, latest, visible, seqs[1], seqs[2])
	}
	merged, err := svc.getChannelMessagesInSeqRange(ctx, channelID, first, latest)
	if err != nil {
		t.Fatalf("load merged messages: %v", err)
	}
	if len(merged) != 2 || merged[0].Seq != seqs[1] || merged[1].Seq != seqs[2] {
		t.Fatalf("merged seqs = %#v, want [%d %d]", merged, seqs[1], seqs[2])
	}

	if _, err := NewAgentRunService(pool).FinishRun(ctx, FinishRunInput{
		RunID: active.ID, Status: AgentRunStatusCompleted, ActivityText: "done",
	}); err != nil {
		t.Fatalf("finish active Run: %v", err)
	}
	// No daemon is registered in this transaction-focused test. Claim remains
	// pending and the real watchdog will retry when the Agent's Computer is back.
	if err := svc.advancePendingMessageWake(ctx, agentID, channelID); err == nil {
		t.Fatal("advance unexpectedly succeeded without a Daemon")
	}
	var pendingCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_pending_message_wakes WHERE agent_id = $1 AND channel_id = $2`, agentID, channelID).Scan(&pendingCount); err != nil {
		t.Fatalf("count pending after unavailable Daemon: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending count after unavailable Daemon = %d, want 1", pendingCount)
	}
}

func TestPublicChannelSuppressesOnlyUnmentionedOrdinaryWake(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	if _, err := pool.Exec(ctx, `UPDATE channels SET workspace_id = $2 WHERE id = $1`,
		channelID, "00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("move test Channel to public: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id)
		VALUES ($1, 'agent', $2) ON CONFLICT DO NOTHING`, channelID, agentID); err != nil {
		t.Fatalf("add test Agent to public Channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	recorder := newRecordingBroadcaster()
	svc := NewAgentService(pool, NewDaemonManager(pool, recorder), recorder, nil)
	svc.TriggerAgentResponse(ctx, channelID, messageID, "user", ownerID, nil, false, nil)
	var runs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_runs WHERE agent_id = $1 AND trigger_message_id = $2`, agentID, messageID).Scan(&runs); err != nil {
		t.Fatalf("count public Runs: %v", err)
	}
	if runs != 0 {
		t.Fatalf("unmentioned public message created %d Runs", runs)
	}

	svc.TriggerAgentResponse(ctx, channelID, messageID, "user", ownerID, []string{agentID}, true, nil)
	if !recorder.hasChannelEvent(channelID, "agent.error", agentErrorNoAvailableDaemon) {
		t.Fatal("explicit public mention was suppressed instead of reaching Daemon resolution")
	}
}

func TestMessageWakeScopeKey(t *testing.T) {
	if messageWakeScopeKey("") != "channel" {
		t.Fatal("channel scope key changed")
	}
	threadID := uuid.NewString()
	if got := messageWakeScopeKey(threadID); got != "thread:"+threadID {
		t.Fatalf("thread scope key = %q", got)
	}
}

func TestMessageWakePersistsRemoteDispatchBeforeCommit(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	computerID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers (id, name, owner_id, status) VALUES ($1, 'wake-remote', $2, 'online')`, computerID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, messageID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	svc := NewAgentService(pool, NewDaemonManager(pool, noopBroadcaster{}), noopBroadcaster{}, nil)
	taskReq := daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID, TriggerMessageID: messageID,
		Messages:       []agent.Message{{Role: agent.RoleUser, Content: "persist me", Seq: seq}},
		ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
		ResultContract: agentResultContractVisibleMessage,
	}
	run, queued, err := svc.claimMessageWake(ctx,
		&DaemonInfo{ID: computerID, ComputerID: computerID, Status: DaemonStatusOnline},
		taskReq, agentChannelInfo{ID: agentID, Name: "Wake Test", ModelProvider: "claude", ModelName: "test"}, seq, seq)
	if err != nil || queued {
		t.Fatalf("claim remote wake = run %+v, queued %v, err %v", run, queued, err)
	}
	var first, latest int64
	var visible bool
	var payload json.RawMessage
	if err := pool.QueryRow(ctx, `
		SELECT wake_first_message_seq, wake_latest_message_seq, wake_requires_visible_result, dispatch_payload
		  FROM agent_runs WHERE id = $1`, run.ID,
	).Scan(&first, &latest, &visible, &payload); err != nil {
		t.Fatal(err)
	}
	var saved daemonTaskRequest
	if err := json.Unmarshal(payload, &saved); err != nil {
		t.Fatal(err)
	}
	if first != seq || latest != seq || !visible || saved.RunID != run.ID || saved.TaskID != run.ID || len(saved.Messages) != 1 {
		t.Fatalf("persisted wake = range(%d,%d,%v), payload %+v", first, latest, visible, saved)
	}
}

func TestPersistedRemoteWakeUsesRunAsTaskIdentity(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	computerID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers (id, name, owner_id, status) VALUES ($1, 'wake-prequeued', $2, 'online')`, computerID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, messageID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	dm := NewDaemonManager(pool, noopBroadcaster{})
	svc := NewAgentService(pool, dm, noopBroadcaster{}, nil)
	taskReq := daemonTaskRequest{
		AgentID: agentID, ChannelID: channelID, TriggerMessageID: messageID,
		Messages:       []agent.Message{{Role: agent.RoleUser, Content: "persist me", Seq: seq}},
		ModelConfig:    agent.ModelConfig{Provider: "claude", Model: "test"},
		ResultContract: agentResultContractVisibleMessage,
	}
	run, queued, err := svc.claimMessageWake(ctx,
		&DaemonInfo{ID: computerID, ComputerID: computerID, Status: DaemonStatusOnline},
		taskReq, agentChannelInfo{ID: agentID, Name: "Wake Test", ModelProvider: "claude", ModelName: "test"}, seq, seq)
	if err != nil || queued {
		t.Fatalf("claim remote wake = run %+v, queued %v, err %v", run, queued, err)
	}
	var payload json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT dispatch_payload FROM agent_runs WHERE id = $1`, run.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var saved daemonTaskRequest
	if err := json.Unmarshal(payload, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.TaskID != run.ID {
		t.Fatalf("persisted TaskID = %q, want %q", saved.TaskID, run.ID)
	}
}

func TestDaemonLostRunRequeuesItsPersistedRange(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	var seq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, messageID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, TriggerMessageID: messageID,
		ChannelID: channelID, Status: AgentRunStatusQueued, WakeFirstSeq: seq,
		WakeLatestSeq: seq, WakeVisible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	svc := NewAgentService(pool, NewDaemonManager(pool, noopBroadcaster{}), noopBroadcaster{}, nil)
	if err := svc.finishDaemonLostRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	var first, latest int64
	var visible bool
	if err := pool.QueryRow(ctx, `
		SELECT first_message_seq, latest_message_seq, requires_visible_result
		  FROM agent_pending_message_wakes
		 WHERE agent_id = $1 AND channel_id = $2 AND scope_key = 'channel'`, agentID, channelID,
	).Scan(&first, &latest, &visible); err != nil {
		t.Fatal(err)
	}
	if first != seq || latest != seq || !visible {
		t.Fatalf("requeued range = (%d,%d,%v), want (%d,%d,true)", first, latest, visible, seq, seq)
	}
}

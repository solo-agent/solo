package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAgentSendFreshnessHoldsAndAdvancesRunCursor(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	channelID := agentRunChannel(t, pool, ownerID)
	agentA := agentRunAgent(t, pool, ownerID)
	agentB := agentRunAgent(t, pool, ownerID)
	triggerID := agentRunMessage(t, pool, channelID, ownerID)

	var triggerSeq int64
	if err := pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, triggerID).Scan(&triggerSeq); err != nil {
		t.Fatalf("load trigger seq: %v", err)
	}
	runSvc := NewAgentRunService(pool)
	runA, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentA, TriggerType: AgentRunTriggerMessage, TriggerMessageID: triggerID,
		ChannelID: channelID, Status: AgentRunStatusRunning, FreshnessSeenSeq: triggerSeq,
	})
	if err != nil {
		t.Fatalf("start run A: %v", err)
	}
	runB, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentB, TriggerType: AgentRunTriggerMessage, TriggerMessageID: triggerID,
		ChannelID: channelID, Status: AgentRunStatusRunning, FreshnessSeenSeq: triggerSeq,
	})
	if err != nil {
		t.Fatalf("start run B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = ANY($1::uuid[])`, []string{runA.ID, runB.ID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1::uuid[])`, []string{agentA, agentB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	messageA := uuid.NewString()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first send: %v", err)
	}
	if err := LockMessageScope(ctx, tx, channelID, "", ""); err != nil {
		t.Fatalf("lock first send: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, metadata)
		VALUES ($1, $2, 'agent', $3, '1', jsonb_build_object('agent_run_id', $4::text))`,
		messageA, channelID, agentA, runA.ID,
	); err != nil {
		t.Fatalf("insert first send: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit first send: %v", err)
	}

	hold := checkFreshnessInTransaction(t, pool, AgentSendFreshnessInput{
		RunID: runB.ID, AgentID: agentB, ChannelID: channelID,
	})
	if hold == nil {
		t.Fatal("second run was not held")
	}
	if hold.NewMessageCount != 1 || len(hold.Messages) != 1 || hold.Messages[0].Content != "1" {
		t.Fatalf("hold = %+v, want the first Agent message", hold)
	}
	held, err := runSvc.HasFreshnessHold(ctx, runB.ID)
	if err != nil || !held {
		t.Fatalf("HasFreshnessHold = %v, %v; want true", held, err)
	}
	if next := checkFreshnessInTransaction(t, pool, AgentSendFreshnessInput{
		RunID: runB.ID, AgentID: agentB, ChannelID: channelID,
	}); next != nil {
		t.Fatalf("same newer context held twice: %+v", next)
	}

	// A successful message from the same Run must not make its next send stale.
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, metadata)
		VALUES ($1, $2, 'agent', $3, '2', jsonb_build_object('agent_run_id', $4::text))`,
		uuid.NewString(), channelID, agentB, runB.ID,
	); err != nil {
		t.Fatalf("insert same-run message: %v", err)
	}
	if next := checkFreshnessInTransaction(t, pool, AgentSendFreshnessInput{
		RunID: runB.ID, AgentID: agentB, ChannelID: channelID,
	}); next != nil {
		t.Fatalf("same-run message caused hold: %+v", next)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, is_deleted)
		VALUES ($1, $2, 'agent', $3, 'deleted', true)`,
		uuid.NewString(), channelID, agentA,
	); err != nil {
		t.Fatalf("insert deleted message: %v", err)
	}
	if next := checkFreshnessInTransaction(t, pool, AgentSendFreshnessInput{
		RunID: runB.ID, AgentID: agentB, ChannelID: channelID,
	}); next != nil {
		t.Fatalf("deleted message caused hold: %+v", next)
	}
}

func checkFreshnessInTransaction(t *testing.T, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, input AgentSendFreshnessInput) *AgentSendFreshnessHold {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin freshness check: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := LockMessageScope(ctx, tx, input.ChannelID, input.ThreadID, ""); err != nil {
		t.Fatalf("lock freshness check: %v", err)
	}
	hold, err := CheckAndHoldAgentSend(ctx, tx, input)
	if err != nil {
		t.Fatalf("check freshness: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit freshness check: %v", err)
	}
	return hold
}

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestResultReminderUsesRunScopeFromPostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	owner := agentRunUser(t, pool)
	channel := agentRunChannel(t, pool, owner)
	root := agentRunMessage(t, pool, channel, owner)
	thread, _, err := NewThreadService(pool).GetOrCreateThread(ctx, channel, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM channels WHERE id=$1`, channel)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	})
	svc := &AgentService{pool: pool}
	run := &AgentRun{ChannelID: channel, ThreadID: thread}
	message, err := svc.resultReminderMessage(ctx, run)
	if err != nil || !strings.Contains(message.Content, "[target="+channel+":"+root+" msg="+root+" type=system]") || !strings.Contains(message.Content, "solo message send --target '"+channel+":"+root+"'") {
		t.Fatalf("reminder lost the current thread: %+v %v", message, err)
	}
	run.ChannelID = uuid.NewString()
	if _, err := svc.resultReminderMessage(ctx, run); err == nil {
		t.Fatal("accepted a thread from a different channel")
	}
	run.ChannelID, run.ThreadID = channel, ""
	message, err = svc.resultReminderMessage(ctx, run)
	if err != nil || !strings.Contains(message.Content, "solo message send --target '"+channel+"'") {
		t.Fatalf("channel/DM reminder changed scope: %+v %v", message, err)
	}
	run.ThinkingNodeID = uuid.NewString()
	message, err = svc.resultReminderMessage(ctx, run)
	if err != nil || !strings.Contains(message.Content, "current Thinking node "+run.ThinkingNodeID) {
		t.Fatalf("reminder lost Thinking context: %+v %v", message, err)
	}
	run.ChannelID = ""
	if _, err := svc.resultReminderMessage(ctx, run); err == nil {
		t.Fatal("invented a target without a Run channel")
	}
}

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/pkg/agent"
)

func TestRemoteStreamClosesOnTerminalResultWithoutDone(t *testing.T) {
	for _, terminal := range []string{"error", "complete", "done"} {
		for _, replay := range []bool{false, true} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			stream := &remoteRunStream{events: make(chan SSEDaemonEvent, 4), incoming: make(chan remoteDeliveryEvent, 4), done: make(chan struct{})}
			events := []remoteDeliveryEvent{
				{attemptID: "attempt", sourceSeq: 1, event: SSEDaemonEvent{Event: "thinking"}},
				{attemptID: "attempt", sourceSeq: 1, event: SSEDaemonEvent{Event: "thinking"}},
				{attemptID: "attempt", sourceSeq: 2, event: SSEDaemonEvent{Event: terminal}},
			}
			if replay {
				go stream.pump(ctx, events)
			} else {
				go stream.pump(ctx, nil)
				for _, event := range events {
					stream.enqueue(event)
				}
			}
			var got []string
			for event := range stream.events {
				got = append(got, event.Event)
			}
			if ctx.Err() != nil || strings.Join(got, ",") != "thinking,"+terminal {
				t.Fatalf("terminal=%s replay=%v: events=%v context=%v", terminal, replay, got, ctx.Err())
			}
			cancel()
		}
	}
}

func TestMissingSessionRecoveryKeepsHistoryAndUsesColdContinuity(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	taskID := agentRunTask(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE channel_id=$1`, channelID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_runs WHERE agent_id=$1`, agentID)
		_, _ = pool.Exec(ctx, `DELETE FROM agent_sessions WHERE agent_id=$1`, agentID)
		_, _ = pool.Exec(ctx, `DELETE FROM agents WHERE id=$1`, agentID)
		_, _ = pool.Exec(ctx, `DELETE FROM channels WHERE id=$1`, channelID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, ownerID)
	})
	svc := NewAgentRunService(pool)
	oldRun, err := svc.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: AgentRunTriggerMessage})
	if err != nil {
		t.Fatal(err)
	}
	missingID := uuid.NewString()
	old, err := svc.BindProviderSession(ctx, BindProviderSessionInput{RunID: oldRun.ID, AgentID: agentID, Provider: "claude", ExternalSessionID: missingID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.FinishRun(ctx, FinishRunInput{RunID: oldRun.ID, Status: AgentRunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{AgentID: agentID, ChannelID: channelID, TriggerType: AgentRunTriggerMessage})
	if err != nil {
		t.Fatal(err)
	}
	req := daemonTaskRequest{AgentID: agentID, ChannelID: channelID, OriginTaskID: taskID, ResumeSessionID: missingID, ModelConfig: agent.ModelConfig{Provider: "claude"}, Messages: []agent.Message{{Role: agent.RoleUser, Content: "continue"}}}
	message := "exit status 1 (stderr: No conversation found with session ID: " + missingID + ")"
	if !canRecoverMissingSession(req, run, message) {
		t.Fatal("exact missing-session failure did not qualify")
	}
	for _, change := range []func(*daemonTaskRequest){
		func(r *daemonTaskRequest) { r.SessionRecoveryAttempt = true },
		func(r *daemonTaskRequest) { r.ForceFreshSession = true },
		func(r *daemonTaskRequest) { r.ResumeSessionID = uuid.NewString() },
		func(r *daemonTaskRequest) { r.ResumeSessionID = "" },
		func(r *daemonTaskRequest) { r.ModelConfig.Provider = "codex" },
		func(r *daemonTaskRequest) { r.NodeID = uuid.NewString() },
	} {
		other := req
		change(&other)
		if canRecoverMissingSession(other, run, message) {
			t.Fatalf("unsafe recovery qualified: %+v", other)
		}
	}
	if canRecoverMissingSession(req, run, "agent execution failed") {
		t.Fatal("generic failure must not retire a session")
	}
	req.SessionRecoveryAttempt = true
	dispatch, err := svc.ResolveSessionDispatch(ctx, ResolveSessionDispatchInput{RunID: run.ID, AgentID: agentID, ChannelID: channelID, Provider: "claude", ResumeSessionID: missingID, ForceFreshSession: true, SupportsContextRollover: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dispatch.ColdStart || !dispatch.ForceFreshSession || dispatch.RetireSessionID != missingID || dispatch.RolloverFromSessionID != old.Session.ID {
		t.Fatalf("wrong recovery dispatch: %+v", dispatch)
	}
	if err = (&AgentService{pool: pool}).applySessionDispatch(ctx, pool, &req, dispatch); err != nil {
		t.Fatal(err)
	}
	if len(req.ColdStartMessages) < 2 || !strings.Contains(req.ColdStartMessages[0].Content, "agent-run-test") {
		t.Fatal("task continuity missing")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var replay daemonTaskRequest
	if err = json.Unmarshal(raw, &replay); err != nil || !replay.SessionRecoveryAttempt || !replay.ForceFreshSession {
		t.Fatalf("recovery retry guard lost on replay: %v", err)
	}
	replacement, err := svc.BindProviderSession(ctx, BindProviderSessionInput{RunID: run.ID, AgentID: agentID, Provider: "claude", ExternalSessionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	var status, previousRunSession string
	if err = pool.QueryRow(ctx, `SELECT status FROM agent_sessions WHERE id=$1`, old.Session.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT session_id::text FROM agent_runs WHERE id=$1`, oldRun.ID).Scan(&previousRunSession); err != nil {
		t.Fatal(err)
	}
	if status != AgentSessionStatusClosed || previousRunSession != old.Session.ID || replacement.Session.ID == old.Session.ID {
		t.Fatal("history or rollover invariant failed")
	}
}

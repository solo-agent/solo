package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/internal/auth"
)

func TestRemoteRunDeliveryIsDurableScopedAndIdempotent(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, ownerID)
	computerID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO computers (id, name, owner_id, status) VALUES ($1, 'remote-test', $2, 'offline')`, computerID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id = $2 WHERE id = $1`, agentID, computerID); err != nil {
		t.Fatal(err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{
		AgentID: agentID, DaemonID: computerID, TriggerType: AgentRunTriggerMessage,
		ChannelID: taskSubmitChannel(t, pool, ownerID), Status: AgentRunStatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = $1`, run.ID)
		_, _ = pool.Exec(context.Background(), `UPDATE agents SET runtime_id = NULL WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = $1`, computerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	dm := NewDaemonManager(pool, nil)
	taskID := run.ID
	request := daemonTaskRequest{
		RunID: run.ID, TaskID: taskID, ResultContract: agentResultContractNone, ResultReminderAttempt: true,
	}
	if _, err := dm.QueueRemoteRun(ctx, &DaemonInfo{ID: computerID, ComputerID: computerID}, request); err != nil {
		t.Fatalf("QueueRemoteRun: %v", err)
	}
	var saved daemonTaskRequest
	var savedPayload json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT dispatch_payload FROM agent_runs WHERE id = $1`, run.ID).Scan(&savedPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(savedPayload, &saved); err != nil || saved.ResultContract != agentResultContractNone || !saved.ResultReminderAttempt {
		t.Fatalf("saved orchestration state = %+v, %v", saved, err)
	}
	pending, err := dm.PendingRemoteRuns(ctx, computerID)
	if err != nil || len(pending) != 1 || pending[0] != run.ID {
		t.Fatalf("PendingRemoteRuns = %#v, %v", pending, err)
	}

	connectionID := uuid.NewString()
	control := &DaemonControlConnection{ID: connectionID, ComputerID: computerID}
	dm.controlConnections[computerID] = control
	delivery, err := dm.AcceptRemoteRun(ctx, computerID, run.ID, connectionID)
	if err != nil {
		t.Fatalf("AcceptRemoteRun: %v", err)
	}
	claims, err := auth.ValidateToken(delivery.AgentToken)
	if err != nil || claims.RunID != run.ID || claims.ComputerID != computerID || claims.Subject != agentID {
		t.Fatalf("delivery claims = %+v, %v", claims, err)
	}
	replayed, err := dm.AcceptRemoteRun(ctx, computerID, run.ID, connectionID)
	if err != nil || replayed.AttemptID != delivery.AttemptID {
		t.Fatalf("idempotent accept = %+v, %v", replayed, err)
	}
	dm.unregisterControlConnection(control)
	replayed, err = dm.AcceptRemoteRun(ctx, computerID, run.ID, connectionID)
	if err != nil || replayed.AttemptID != delivery.AttemptID {
		t.Fatalf("accept during disconnect grace = %+v, %v", replayed, err)
	}

	event := RemoteRunEventInput{
		TaskID: taskID, AttemptID: delivery.AttemptID, ConnectionID: connectionID,
		SourceSeq: 1, Event: "thinking", Data: json.RawMessage(`{"thought":"ready"}`),
	}
	if err := dm.AppendRemoteRunEvent(ctx, computerID, run.ID, event); err != nil {
		t.Fatalf("AppendRemoteRunEvent: %v", err)
	}
	if err := dm.AppendRemoteRunEvent(ctx, computerID, run.ID, event); err != nil {
		t.Fatalf("duplicate AppendRemoteRunEvent: %v", err)
	}
	wrongTask := event
	wrongTask.TaskID = uuid.NewString()
	wrongTask.SourceSeq = 2
	if err := dm.AppendRemoteRunEvent(ctx, computerID, run.ID, wrongTask); !errors.Is(err, ErrRemoteRunAttempt) {
		t.Fatalf("wrong task identity error = %v, want ErrRemoteRunAttempt", err)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_run_delivery_events WHERE run_id = $1`, run.ID).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("persisted delivery event count = %d, %v", eventCount, err)
	}
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT delivery_expires_at FROM agent_runs WHERE id = $1`, run.ID).Scan(&expiresAt); err != nil || time.Until(expiresAt) < 23*time.Hour {
		t.Fatalf("delivery expiry = %v, %v", expiresAt, err)
	}
	dm.registerControlConnection(&DaemonControlConnection{ID: uuid.NewString(), ComputerID: computerID})
	if _, err := dm.AcceptRemoteRun(ctx, computerID, run.ID, connectionID); !errors.Is(err, ErrStaleControlLease) {
		t.Fatalf("accept from replaced connection error = %v, want ErrStaleControlLease", err)
	}
}

func TestControlRPCTimeoutMatchesOperationRisk(t *testing.T) {
	if got := controlRPCTimeout("workspace.read"); got != 10*time.Second {
		t.Fatalf("workspace timeout = %s", got)
	}
	if got := controlRPCTimeout("agent.cleanup"); got != 30*time.Second {
		t.Fatalf("cleanup timeout = %s", got)
	}
}

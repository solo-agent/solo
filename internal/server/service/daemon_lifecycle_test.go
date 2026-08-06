package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProxyBackendDetectRequiresOneOnlineDaemon(t *testing.T) {
	dm := NewDaemonManager(nil, nil)
	if _, err := dm.ProxyBackendDetect(context.Background()); err == nil || !strings.Contains(err.Error(), "no online daemon") {
		t.Fatalf("no-daemon error = %v", err)
	}

	dm.Register(&DaemonInfo{ID: "daemon-a", Status: DaemonStatusOnline})
	dm.Register(&DaemonInfo{ID: "daemon-b", Status: DaemonStatusOnline})
	if _, err := dm.ProxyBackendDetect(context.Background()); err == nil || !strings.Contains(err.Error(), "multiple online daemons") {
		t.Fatalf("multi-daemon error = %v", err)
	}
}

func TestPendingTaskTimeoutsUseCurrentLifecyclePhase(t *testing.T) {
	now := time.Now().UTC()
	backendStarted := now.Add(-2 * time.Minute)
	dm := NewDaemonManager(nil, nil)
	dm.queueTimeout = 10 * time.Minute
	dm.executionTimeout = time.Minute
	dm.pendingTasks = map[string]PendingTaskInfo{
		"fresh-queue": {
			TaskID:    "fresh-queue",
			CreatedAt: now.Add(-5 * time.Minute),
		},
		"stale-queue": {
			TaskID:    "stale-queue",
			CreatedAt: now.Add(-11 * time.Minute),
		},
		"stale-execution": {
			TaskID:           "stale-execution",
			CreatedAt:        now.Add(-30 * time.Minute),
			BackendStartedAt: &backendStarted,
		},
	}

	stale := dm.removeStaleTasks(now)
	if len(stale) != 2 {
		t.Fatalf("stale tasks = %+v, want queue and execution timeout", stale)
	}
	phases := map[string]string{}
	for _, task := range stale {
		phases[task.TaskID] = task.TimeoutPhase
	}
	if phases["stale-queue"] != "queue" || phases["stale-execution"] != "execution" {
		t.Fatalf("timeout phases = %+v", phases)
	}
	if _, ok := dm.pendingTasks["fresh-queue"]; !ok {
		t.Fatal("fresh queued task was removed by execution timeout")
	}
}

func TestResolveDaemonForAgentUsesComputerBinding(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	computerA, computerB := uuid.NewString(), uuid.NewString()
	var homeChannelID string
	if err := pool.QueryRow(ctx, `SELECT home_channel_id::text FROM agents WHERE id = $1`, agentID).Scan(&homeChannelID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM computers WHERE id = ANY($1::uuid[])`, []string{computerA, computerB})
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, homeChannelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO computers (id, name, owner_id, daemon_id, status)
		VALUES ($1, 'computer-a', $3, 'daemon-a', 'online'),
		       ($2, 'computer-b', $3, 'daemon-b', 'online')`, computerA, computerB, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id = $2 WHERE id = $1`, agentID, computerA); err != nil {
		t.Fatal(err)
	}

	dm := NewDaemonManager(pool, nil)
	dm.Register(&DaemonInfo{ID: "daemon-a", Capabilities: []string{"llm"}, MaxConcurrent: 1})
	dm.Register(&DaemonInfo{ID: "daemon-b", Capabilities: []string{"llm"}, MaxConcurrent: 1})
	daemon, err := dm.ResolveDaemonForAgent(ctx, agentID, "llm")
	if err != nil || daemon.ID != "daemon-a" {
		t.Fatalf("bound daemon = %#v, %v; want daemon-a", daemon, err)
	}

	dm.Unregister("daemon-a")
	if _, err := dm.ResolveDaemonForAgent(ctx, agentID, "llm"); err == nil {
		t.Fatal("offline bound daemon fell back to another computer")
	}
	dm.Register(&DaemonInfo{ID: "daemon-a", Capabilities: []string{"llm"}, MaxConcurrent: 1})
	if _, err := pool.Exec(ctx, `UPDATE agents SET runtime_id = NULL WHERE id = $1`, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := dm.ResolveDaemonForAgent(ctx, agentID, "llm"); err == nil {
		t.Fatal("unbound agent selected randomly from multiple daemons")
	}
	dm.Unregister("daemon-b")
	daemon, err = dm.ResolveDaemonForAgent(ctx, agentID, "llm")
	if err != nil || daemon.ID != "daemon-a" {
		t.Fatalf("single-daemon legacy fallback = %#v, %v; want daemon-a", daemon, err)
	}
	var runtimeID string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(runtime_id, '') FROM agents WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatal(err)
	}
	if runtimeID != computerA {
		t.Fatalf("persisted runtime_id = %q, want %q", runtimeID, computerA)
	}
}

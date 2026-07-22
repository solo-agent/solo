package service

import (
	"testing"
	"time"
)

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

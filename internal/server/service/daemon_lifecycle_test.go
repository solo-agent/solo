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

func TestSlidingProgressTimeoutResetsExecutionDeadline(t *testing.T) {
	now := time.Now().UTC()
	backendStarted := now.Add(-11 * time.Minute)
	lastProgress := now.Add(-3 * time.Minute)

	dm := NewDaemonManager(nil, nil)
	dm.queueTimeout = 10 * time.Minute
	dm.executionTimeout = 6 * time.Minute
	dm.pendingTasks = map[string]PendingTaskInfo{
		"stale-fixed": {
			TaskID:           "stale-fixed",
			CreatedAt:        now.Add(-30 * time.Minute),
			BackendStartedAt: &backendStarted,
			// No LastProgressAt — falls back to BackendStartedAt (11 min ago > 6 min → stale)
		},
		"fresh-sliding": {
			TaskID:           "fresh-sliding",
			CreatedAt:        now.Add(-30 * time.Minute),
			BackendStartedAt: &backendStarted,
			LastProgressAt:   lastProgress, // 3 min ago < 6 min → still fresh
		},
	}

	stale := dm.removeStaleTasks(now)
	staleIDs := make(map[string]bool)
	for _, s := range stale {
		staleIDs[s.TaskID] = true
	}
	if !staleIDs["stale-fixed"] {
		t.Fatal("stale-fixed was not caught by timeout")
	}
	if staleIDs["fresh-sliding"] {
		t.Fatal("fresh-sliding was incorrectly timed out despite recent LastProgressAt")
	}
}

func TestMarkTaskProgressUpdatesPendingTask(t *testing.T) {
	dm := NewDaemonManager(nil, nil)
	dm.TrackTask("prog-task", "d1", "a1")
	now := time.Now()
	dm.MarkTaskBackendStarted("prog-task", &now)

	time.Sleep(1 * time.Millisecond)
	dm.MarkTaskProgress("prog-task")

	dm.mu.RLock()
	task := dm.pendingTasks["prog-task"]
	dm.mu.RUnlock()

	if !task.LastProgressAt.After(now) {
		t.Fatal("LastProgressAt was not updated by MarkTaskProgress")
	}
}

func TestMarkTaskProgressNoOp(t *testing.T) {
	dm := NewDaemonManager(nil, nil)

	// Non-existent task should not panic
	dm.MarkTaskProgress("nonexistent")

	// Task without BackendStartedAt should not update LastProgressAt
	dm.TrackTask("queued-only", "d1", "a1")
	dm.MarkTaskProgress("queued-only")

	dm.mu.RLock()
	task := dm.pendingTasks["queued-only"]
	dm.mu.RUnlock()

	if !task.LastProgressAt.IsZero() {
		t.Fatal("LastProgressAt should remain zero for queue-phase task")
	}
}

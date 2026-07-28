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
	dm.TrackTask("prog-task", "d1", "a1", 0)
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

func TestCustomExecutionTimeout(t *testing.T) {
	now := time.Now().UTC()
	dm := NewDaemonManager(nil, nil)
	dm.queueTimeout = 10 * time.Minute
	dm.executionTimeout = 6 * time.Minute
	dm.maxExecutionTimeout = 120 * time.Minute

	// Task with 60 min custom timeout and recent progress — NOT stale at 30 min.
	backend30min := now.Add(-30 * time.Minute)
	dm.pendingTasks = map[string]PendingTaskInfo{
		"custom-60-active": {
			TaskID:                  "custom-60-active",
			CreatedAt:               now.Add(-35 * time.Minute),
			BackendStartedAt:        &backend30min,
			LastProgressAt:          now.Add(-3 * time.Minute),
			ExpectedDurationMinutes: 60,
		},
		// Task with 60 min custom timeout but no progress for 70 min — IS stale.
		"custom-60-stale": {
			TaskID:                  "custom-60-stale",
			CreatedAt:               now.Add(-80 * time.Minute),
			BackendStartedAt:        &backend30min,
			LastProgressAt:          now.Add(-70 * time.Minute),
			ExpectedDurationMinutes: 60,
		},
		// Task with 0 expected duration — uses default 6 min.
		"default-timeout-stale": {
			TaskID:                  "default-timeout-stale",
			CreatedAt:               now.Add(-30 * time.Minute),
			BackendStartedAt:        &backend30min,
			LastProgressAt:          now.Add(-10 * time.Minute),
			ExpectedDurationMinutes: 0,
		},
		// Task with 200 min expected — capped at 120 min max.
		"capped-max": {
			TaskID:                  "capped-max",
			CreatedAt:               now.Add(-130 * time.Minute),
			BackendStartedAt:        &backend30min,
			LastProgressAt:          now.Add(-30 * time.Minute),
			ExpectedDurationMinutes: 200,
		},
	}

	stale := dm.removeStaleTasks(now)
	staleIDs := make(map[string]bool)
	for _, s := range stale {
		staleIDs[s.TaskID] = true
	}

	if staleIDs["custom-60-active"] {
		t.Fatal("custom-60-active should NOT be stale with recent progress")
	}
	if !staleIDs["custom-60-stale"] {
		t.Fatal("custom-60-stale should be stale after 70 min without progress")
	}
	if !staleIDs["default-timeout-stale"] {
		t.Fatal("default-timeout-stale should be stale after 10 min without progress")
	}
	if staleIDs["capped-max"] {
		t.Fatal("capped-max should NOT be stale at 30 min (200 capped to 120)")
	}
}

func TestMarkTaskProgressNoOp(t *testing.T) {
	dm := NewDaemonManager(nil, nil)

	// Non-existent task should not panic
	dm.MarkTaskProgress("nonexistent")

	// Task without BackendStartedAt should not update LastProgressAt
	dm.TrackTask("queued-only", "d1", "a1", 0)
	dm.MarkTaskProgress("queued-only")

	dm.mu.RLock()
	task := dm.pendingTasks["queued-only"]
	dm.mu.RUnlock()

	if !task.LastProgressAt.IsZero() {
		t.Fatal("LastProgressAt should remain zero for queue-phase task")
	}
}

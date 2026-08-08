package main

import (
	"context"
	"testing"
	"time"
)

func TestTaskManagerReplaysBackendStartSessionAndTerminalEvents(t *testing.T) {
	tm := newTaskManager()
	taskID := "task-1"
	tm.AddTask(taskID, &taskState{TaskID: taskID})

	tm.PushSSEEvent(taskID, sseEvent{Event: "backend_started", Data: `{"run_id":"run-1"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "session", Data: `{"external_session_id":"s1"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "text", Data: `{"content":"replayed"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "complete", Data: `{"status":"ok"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "done", Data: `{}`})

	sub := tm.SubscribeSSE(taskID)
	got := drainEvents(sub.events)
	want := []string{"backend_started", "session", "text", "complete", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTaskManagerIDsIncludeActiveAndTerminalTasks(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("active", &taskState{TaskID: "active", Status: taskStatusRunning})
	tm.AddTask("done", &taskState{TaskID: "done", Status: taskStatusCompleted})

	got := map[string]bool{}
	for _, taskID := range tm.ListTaskIDs() {
		got[taskID] = true
	}
	if !got["active"] || !got["done"] {
		t.Fatalf("snapshots = %#v", got)
	}
}

func TestTaskManagerExecutingRunIDUsesRuntimeScope(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("old", &taskState{
		AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusCompleted,
	})
	tm.AddTask("current", &taskState{
		RunID: "run-current", AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusThinking, AgentToken: "run-token",
	})
	tm.AddTask("other-channel", &taskState{
		AgentID: "agent-1", ChannelID: "channel-2", Status: taskStatusThinking,
	})

	if got := tm.ExecutingRunID("agent-1", "channel-1", ""); got != "run-current" {
		t.Fatalf("ExecutingRunID = %q, want run-current", got)
	}
	if runID, token := tm.ExecutingCredential("agent-1", "channel-1", ""); runID != "run-current" || token != "run-token" {
		t.Fatalf("ExecutingCredential = %q, %q", runID, token)
	}
}

func TestTaskManagerExecutingCredentialAllowsUnambiguousChannelResolution(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("current", &taskState{
		RunID: "run-current", AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusThinking, AgentToken: "run-token",
	})

	if runID, token := tm.ExecutingCredential("agent-1", "", ""); runID != "run-current" || token != "run-token" {
		t.Fatalf("ExecutingCredential = %q, %q", runID, token)
	}
	tm.AddTask("ambiguous", &taskState{
		RunID: "run-other", AgentID: "agent-1", ChannelID: "channel-2", Status: taskStatusThinking, AgentToken: "other-token",
	})
	if runID, token := tm.ExecutingCredential("agent-1", "", ""); runID != "" || token != "" {
		t.Fatalf("ambiguous ExecutingCredential = %q, %q", runID, token)
	}
}

func TestTaskManagerAgentTurnSerializesOneAgentOnly(t *testing.T) {
	tm := newTaskManager()
	releaseFirst, err := tm.acquireAgentTurn(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("acquire first turn: %v", err)
	}

	secondAcquired := make(chan func(), 1)
	go func() {
		release, acquireErr := tm.acquireAgentTurn(context.Background(), "agent-1")
		if acquireErr == nil {
			secondAcquired <- release
		}
	}()
	select {
	case <-secondAcquired:
		t.Fatal("second turn for the same Agent acquired before release")
	case <-time.After(20 * time.Millisecond):
	}

	releaseOther, err := tm.acquireAgentTurn(context.Background(), "agent-2")
	if err != nil {
		t.Fatalf("other Agent was blocked: %v", err)
	}
	releaseOther()

	releaseFirst()
	select {
	case releaseSecond := <-secondAcquired:
		releaseSecond()
	case <-time.After(time.Second):
		t.Fatal("second turn did not acquire after release")
	}
}

func TestTaskManagerClearsCompletedTaskCancellation(t *testing.T) {
	tm := newTaskManager()
	called := false
	tm.SetCancelFunc("task-1", func() { called = true })
	tm.ClearCancelFunc("task-1")
	if tm.CancelTask("task-1") {
		t.Fatal("completed task retained cancellation authority")
	}
	if called {
		t.Fatal("cleared cancellation function was invoked")
	}
}

func TestTaskManagerCancellationLeavesStreamForTerminalEvent(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("task-1", &taskState{TaskID: "task-1"})
	sub := tm.SubscribeSSE("task-1")
	tm.SetCancelFunc("task-1", func() {})
	if !tm.CancelTask("task-1") {
		t.Fatal("CancelTask = false, want true")
	}
	select {
	case _, ok := <-sub.events:
		if !ok {
			t.Fatal("cancellation closed stream before processor terminal event")
		}
	default:
	}

	h := &daemonHandler{taskManager: tm}
	h.finishCancelledTask(runTaskRequest{TaskID: "task-1", AgentID: "agent-1"})
	got := drainEvents(sub.events)
	want := []string{"error", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTaskManagerCloseDrainsQueuedEvents(t *testing.T) {
	tm := newTaskManager()
	taskID := "task-1"
	tm.AddTask(taskID, &taskState{TaskID: taskID})
	sub := tm.SubscribeSSE(taskID)

	tm.PushSSEEvent(taskID, sseEvent{Event: "complete", Data: `{"status":"ok"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "done", Data: `{}`})
	tm.CloseAllSubscribers(taskID)

	got := drainEvents(sub.events)
	want := []string{"complete", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTaskManagerDeduplicatesDeliveryAttempt(t *testing.T) {
	tm := newTaskManager()
	if !tm.AddTask("task-1", &taskState{TaskID: "task-1", AttemptID: "attempt-1"}) {
		t.Fatal("first delivery was rejected")
	}
	if tm.AddTask("task-1", &taskState{TaskID: "task-1", AttemptID: "attempt-1"}) {
		t.Fatal("duplicate delivery attempt was accepted")
	}
	if !tm.AddTask("task-1", &taskState{TaskID: "task-1", AttemptID: "attempt-2"}) {
		t.Fatal("new delivery attempt was rejected")
	}
}

func TestTaskManagerRemoteHistorySurvivesSubscriberCapacityAndCleansUp(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("task-1", &taskState{TaskID: "task-1", AttemptID: "attempt-1", Forwarding: true})
	for i := 0; i < 100; i++ {
		tm.PushSSEEvent("task-1", sseEvent{Event: "text", Data: `{"content":"chunk"}`})
	}
	tm.PushSSEEvent("task-1", sseEvent{Event: "done", Data: `{}`})
	events := tm.EventsAfter("task-1", 0)
	if len(events) != 101 || events[len(events)-1].Event != "done" {
		t.Fatalf("remote history = %d events, last = %q", len(events), events[len(events)-1].Event)
	}
	tm.EndForward("task-1", "attempt-1", true)
	if _, ok := tm.GetTask("task-1"); ok || len(tm.EventsAfter("task-1", 0)) != 0 {
		t.Fatal("delivered remote task history was retained")
	}
}

func TestTaskManagerBoundsRemoteHistory(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("task-1", &taskState{TaskID: "task-1", AttemptID: "attempt-1"})
	payload := string(make([]byte, 512<<10))
	for i := 0; i < 40; i++ {
		tm.PushSSEEvent("task-1", sseEvent{Event: "text", Data: payload})
	}
	tm.PushSSEEvent("task-1", sseEvent{Event: "done", Data: `{}`})
	events := tm.eventHistory["task-1"]
	if tm.historyBytes["task-1"] > maxTaskEventHistoryBytes || events[len(events)-1].Event != "done" {
		t.Fatalf("history bytes = %d, events = %d", tm.historyBytes["task-1"], len(events))
	}
}

func drainEvents(ch <-chan sseEvent) []string {
	var events []string
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt.Event)
		default:
			return events
		}
	}
}

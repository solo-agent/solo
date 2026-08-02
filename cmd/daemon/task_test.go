package main

import "testing"

func TestTaskManagerReplaysBackendStartSessionAndTerminalEvents(t *testing.T) {
	tm := newTaskManager()
	taskID := "task-1"
	tm.AddTask(taskID, &taskState{TaskID: taskID})

	tm.PushSSEEvent(taskID, sseEvent{Event: "backend_started", Data: `{"run_id":"run-1"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "session", Data: `{"external_session_id":"s1"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "text", Data: `{"content":"not replayed"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "complete", Data: `{"status":"ok"}`})
	tm.PushSSEEvent(taskID, sseEvent{Event: "done", Data: `{}`})

	sub := tm.SubscribeSSE(taskID)
	got := drainEvents(sub.events)
	want := []string{"backend_started", "session", "complete", "done"}
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

func TestTaskManagerExecutingTaskIDUsesRuntimeScope(t *testing.T) {
	tm := newTaskManager()
	tm.AddTask("old", &taskState{
		AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusCompleted,
	})
	tm.AddTask("current", &taskState{
		AgentID: "agent-1", ChannelID: "channel-1", Status: taskStatusThinking,
	})
	tm.AddTask("other-channel", &taskState{
		AgentID: "agent-1", ChannelID: "channel-2", Status: taskStatusThinking,
	})

	if got := tm.ExecutingTaskID("agent-1", "channel-1", ""); got != "current" {
		t.Fatalf("ExecutingTaskID = %q, want current", got)
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

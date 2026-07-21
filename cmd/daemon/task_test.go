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

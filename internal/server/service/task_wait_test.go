package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTaskWaitPostgres(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	other := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	for _, member := range []struct{ kind, id string }{{"user", owner}, {"user", other}, {"agent", a}} {
		taskSubmitMember(t, pool, channel, member.kind, member.id)
	}
	svc := NewTaskService(pool)
	create := func(title string) *Task {
		task, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: title, Assignee: a})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	first, second := create("Waiting"), create("Dependency")
	req := TaskWaitRequest{ExpectedTaskVersion: first.Version, IdempotencyKey: "first-wait", Condition: TaskWaitCondition{Kind: "task_done", TaskID: second.ID}, Handoff: TaskHandoff{Summary: "analysis preserved"}, NextAction: "continue the same task"}
	if _, err := svc.WaitTask(ctx, channel, first.ID, other, req); !errors.Is(err, ErrTaskNotClaimer) {
		t.Fatalf("foreign wait: %v", err)
	}
	id, err := svc.WaitTask(ctx, channel, first.ID, a, req)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := svc.WaitTask(ctx, channel, first.ID, a, req); err != nil || again != id {
		t.Fatalf("replay %s %v", again, err)
	}
	req.NextAction = "different"
	if _, err = svc.WaitTask(ctx, channel, first.ID, a, req); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatal("changed same request", err)
	}
	req.NextAction = "continue the same task"
	for _, kind := range []string{"get", "list", "agent", "all"} {
		var tasks []Task
		switch kind {
		case "get":
			task, e := svc.GetTask(ctx, channel, first.ID, owner)
			err = e
			if task != nil {
				tasks = []Task{*task}
			}
		case "list":
			tasks, err = svc.ListTasks(ctx, channel, owner, TaskFilter{})
		case "agent":
			tasks, err = svc.GetTasksForAgent(ctx, a)
		case "all":
			tasks, err = svc.ListAllUserTasks(ctx, owner, channel, "", "", "")
		}
		if err != nil {
			t.Fatal(kind, err)
		}
		found := false
		for _, task := range tasks {
			if task.ID == first.ID {
				found = task.Waiting
			}
		}
		if !found {
			t.Fatal(kind, "lost waiting projection")
		}
	}
	req.ExpectedTaskVersion = second.Version
	req.Condition.TaskID = first.ID
	if _, err = svc.WaitTask(ctx, channel, second.ID, a, req); err == nil {
		t.Fatal("accepted cycle")
	}
	packet, err := (&AgentService{pool: pool}).getAgentContinuityPacket(ctx, pool, a, channel, second.ID)
	if err != nil || !strings.Contains(packet, "is WAITING") || !strings.Contains(packet, "analysis preserved") {
		t.Fatalf("continuity: %s %v", packet, err)
	}
	// Recreating the service models a Server restart; no in-memory condition cache.
	worker := &AgentService{pool: pool}
	for i := 0; i < 3; i++ {
		if found, err := worker.dispatchTaskWait(ctx); err != nil || found {
			t.Fatalf("unsatisfied wait dispatched: %t %v", found, err)
		}
	}
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, owner, TaskWaitResolution{WaitID: id, Action: "cancel", Reason: "dependency changed"}); err != nil {
		t.Fatal(err)
	}
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, owner, TaskWaitResolution{WaitID: id, Action: "cancel", Reason: "dependency changed"}); err != nil {
		t.Fatal("cancel replay", err)
	}
	// The task remains owned; waiting does not finish it or change its version.
	first, err = svc.GetTask(ctx, channel, first.ID, owner)
	if err != nil || first.Status != "in_progress" || first.ClaimerID != a || first.Waiting {
		t.Fatal(first, err)
	}
	req.ExpectedTaskVersion = first.Version
	req.IdempotencyKey = "signal"
	req.Condition = TaskWaitCondition{Kind: "signal", Description: "owner confirms deployment v3"}
	id, err = svc.WaitTask(ctx, channel, first.ID, a, req)
	if err != nil {
		t.Fatal(err)
	}
	resolution := TaskWaitResolution{WaitID: id, Action: "signal", Reason: "Observed deployment v3 and its real health check"}
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, a, resolution); !errors.Is(err, ErrTaskHumanOnly) {
		t.Fatal("agent forged external condition", err)
	}
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, owner, resolution); err != nil {
		t.Fatal(err)
	}
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, owner, resolution); err != nil {
		t.Fatal("signal replay", err)
	}
	resolution.Reason = "different evidence"
	if err = svc.ResolveTaskWait(ctx, channel, first.ID, owner, resolution); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatal("overwrote condition evidence", err)
	}
	// Changes invalidate the old obligation across every Task API, via the DB trigger.
	title := "Revised goal"
	if _, err = svc.UpdateTask(ctx, channel, first.ID, owner, TaskUpdateRequest{Title: &title}); err != nil {
		t.Fatal(err)
	}
	data, err := svc.ListTaskWaits(ctx, channel, first.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ Waits []struct{ ID, Status string } }
	if err = json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	for _, wait := range state.Waits {
		if wait.Status != "cancelled" {
			t.Fatal("old wait survived Task change", string(data))
		}
	}
	if _, err = svc.WaitTask(ctx, channel, first.ID, a, req); err != nil {
		t.Fatal("original request replay should remain stable", err)
	}
	req.IdempotencyKey = "stale"
	if _, err = svc.WaitTask(ctx, channel, first.ID, a, req); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatal("accepted stale version", err)
	}
	// Parent completion implicitly depends on its unfinished child.
	child, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "child", ParentTaskID: second.ID, Assignee: a})
	if err != nil {
		t.Fatal(err)
	}
	req.ExpectedTaskVersion = child.Version
	req.Condition = TaskWaitCondition{Kind: "task_done", TaskID: second.ID}
	req.IdempotencyKey = "parent-cycle"
	if _, err = svc.WaitTask(ctx, channel, child.ID, a, req); err == nil {
		t.Fatal("child waited for its own parent's completion")
	}
	// Supported future times stay dormant; arbitrary pseudo-conditions are rejected.
	future := time.Now().Add(time.Hour)
	req.Condition = TaskWaitCondition{Kind: "at_time", At: &future}
	req.IdempotencyKey = "time"
	if _, err = svc.WaitTask(ctx, channel, child.ID, a, req); err != nil {
		t.Fatal(err)
	}
	if found, err := worker.dispatchTaskWait(ctx); err != nil || found {
		t.Fatalf("future wait dispatched: %t %v", found, err)
	}
	req.Condition = TaskWaitCondition{Kind: "arbitrary_shell_poll"}
	if _, err = svc.WaitTask(ctx, channel, child.ID, a, req); err == nil {
		t.Fatal("accepted unsupported condition")
	}
	// A satisfied dependency is evaluated with the real service and Daemon manager.
	// With no Computer bound, it stays durable and records the actual dispatch error.
	ready := create("Ready dependency")
	if _, err = svc.SubmitTask(ctx, channel, second.ID, a); !errors.Is(err, ErrTaskHasOpenSubtasks) {
		t.Fatal("parent should retain child gate", err)
	}
	dependency := create("Independent dependency")
	if _, err = svc.SubmitTask(ctx, channel, dependency.ID, a); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AcceptTask(ctx, channel, dependency.ID, owner); err != nil {
		t.Fatal(err)
	}
	req.ExpectedTaskVersion = ready.Version
	req.IdempotencyKey = "ready-dependency"
	req.Condition = TaskWaitCondition{Kind: "task_done", TaskID: dependency.ID}
	readyID, err := svc.WaitTask(ctx, channel, ready.ID, a, req)
	if err != nil {
		t.Fatal(err)
	}
	worker.dm = NewDaemonManager(pool, nil)
	if found, err := worker.dispatchTaskWait(ctx); err != nil || !found {
		t.Fatalf("ready dependency not evaluated: %t %v", found, err)
	}
	var durable bool
	if err = pool.QueryRow(ctx, `SELECT status='waiting' AND run_id IS NULL AND last_error<>'' AND next_attempt_at>now() FROM task_waits WHERE id=$1`, readyID).Scan(&durable); err != nil || !durable {
		t.Fatalf("offline work lost: %t %v", durable, err)
	}
	if err = svc.ResolveTaskWait(ctx, channel, ready.ID, owner, TaskWaitResolution{WaitID: readyID, Action: "cancel", Reason: "test next condition"}); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	req.Condition = TaskWaitCondition{Kind: "at_time", At: &past}
	req.IdempotencyKey = "time-ready"
	if _, err = svc.WaitTask(ctx, channel, ready.ID, a, req); err != nil {
		t.Fatal(err)
	}
	if found, err := worker.dispatchTaskWait(ctx); err != nil || !found {
		t.Fatalf("due time not evaluated: %t %v", found, err)
	}
}

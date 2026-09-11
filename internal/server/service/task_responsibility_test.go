package service

import (
	"context"
	"testing"
)

func TestTaskResponsibilityEventsAreActualAndIdempotent(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	svc := NewTaskService(pool)
	assigned, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "delegated", Assignee: author})
	if err != nil {
		t.Fatal(err)
	}
	var kind, actor, assignee string
	if err = pool.QueryRow(ctx, `SELECT kind,actor_id::text,assignee_id::text FROM task_responsibility_events WHERE task_id=$1`, assigned.ID).Scan(&kind, &actor, &assignee); err != nil {
		t.Fatal(err)
	}
	if kind != "assigned" || actor != owner || assignee != author {
		t.Fatal("incorrect delegation evidence", kind, actor, assignee)
	}
	task, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "self claimed"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = svc.ClaimTask(ctx, channel, task.ID, author); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM task_responsibility_events WHERE task_id=$1 AND kind='claimed' AND actor_id=$2 AND assignee_id=$2`, task.ID, author).Scan(&count); err != nil || count != 1 {
		t.Fatal("claim retries changed history", count, err)
	}
	// Releasing the current assignment must not erase the original claim evidence.
	if _, err = pool.Exec(ctx, `UPDATE tasks SET claimer_id=NULL,status='todo' WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM task_responsibility_events WHERE task_id=$1`, task.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("lost original claim", count, err)
	}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestMessageTaskClaimPostgres(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	channel := taskSubmitChannel(t, pool, owner)
	a := taskSubmitAgent(t, pool, owner)
	b := taskSubmitAgent(t, pool, owner)
	for _, actor := range []struct{ kind, id string }{{"user", owner}, {"agent", a}, {"agent", b}} {
		taskSubmitMember(t, pool, channel, actor.kind, actor.id)
	}
	svc := NewTaskService(pool)
	message := func(id string) string {
		if id == "" {
			id = uuid.NewString()
		}
		if _, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'Please verify this ordinary request')`, id, channel, owner); err != nil {
			t.Fatal(err)
		}
		return id
	}
	source := message("")
	var wg sync.WaitGroup
	results := make(chan *Task, 2)
	failures := make(chan error, 2)
	for _, actor := range []string{a, b} {
		wg.Add(1)
		go func(actor string) {
			defer wg.Done()
			task, _, err := svc.ClaimMessageTask(ctx, channel, source[:8], actor, nil, nil)
			if err != nil {
				failures <- err
			} else {
				results <- task
			}
		}(actor)
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("claim winners=%d failures=%d", len(results), len(failures))
	}
	winner := <-results
	if err := <-failures; !errors.Is(err, ErrTaskAlreadyClaimed) {
		t.Fatal(err)
	}
	if winner.MessageID != source || winner.CreatorID != owner || winner.Status != TaskStatusInProgress {
		t.Fatalf("wrong task: %+v", winner)
	}
	replay, created, err := svc.ClaimMessageTask(ctx, channel, source, winner.ClaimerID, nil, nil)
	if err != nil || created || replay.ID != winner.ID {
		t.Fatalf("claim replay: %+v %t %v", replay, created, err)
	}
	for _, ref := range []string{winner.ID, fmt.Sprint(winner.TaskNumber), source, source[:8]} {
		found, err := svc.GetTask(ctx, channel, ref, owner)
		if err != nil || found.ID != winner.ID {
			t.Fatalf("lookup %s: %+v %v", ref, found, err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE message_id=$1`, source).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate task count=%d: %v", count, err)
	}
	if _, err = svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "duplicate", MessageID: source}); !errors.Is(err, ErrTaskSourceExists) {
		t.Fatalf("Create bypassed source uniqueness: %v", err)
	}
	otherChannel := agentRunChannel(t, pool, owner)
	taskSubmitMember(t, pool, otherChannel, "user", owner)
	if _, _, err = svc.ClaimMessageTask(ctx, otherChannel, source, owner, nil, nil); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-channel message claimed: %v", err)
	}
	prefix := uuid.NewString()[:8]
	first := message(prefix + "-1111-4111-8111-111111111111")
	message(prefix + "-2222-4222-8222-222222222222")
	if _, _, err = svc.ClaimMessageTask(ctx, channel, prefix, a, nil, nil); !errors.Is(err, ErrTaskReferenceAmbiguous) {
		t.Fatalf("ambiguous prefix guessed: %v", err)
	}
	// Denied priority windows leave neither a new task nor a partial claim.
	if _, _, err = svc.ClaimMessageTask(ctx, channel, first, a, func(string) error { return ErrTaskAlreadyClaimed }, nil); !errors.Is(err, ErrTaskAlreadyClaimed) {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE message_id=$1`, first).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed claim left a Task: %d %v", count, err)
	}
}

func TestMessageTaskConvertAndNumberLookupPostgres(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	svc := NewTaskService(pool)
	source := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'Convert only once')`, source, channel, owner); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := svc.ConvertMessageToTask(ctx, channel, source[:8], owner)
			if err != nil {
				failures <- err
			} else {
				ids <- task.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var canonical string
	for id := range ids {
		if canonical != "" && id != canonical {
			t.Fatalf("two tasks: %s %s", canonical, id)
		}
		canonical = id
	}
	if canonical == "" {
		t.Fatal("no task")
	}
	child, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "child", ParentTaskID: canonical})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{canonical, source[:8]} {
		parent, err := svc.GetTask(ctx, channel, ref, owner)
		if err != nil || parent.SubtaskCount != 1 {
			t.Fatalf("lookup dropped child count: %+v %v", parent, err)
		}
	}
	found, err := svc.GetTask(ctx, channel, fmt.Sprint(child.TaskNumber), owner)
	if err != nil || found.ParentTaskID == nil || *found.ParentTaskID != canonical {
		t.Fatalf("number lookup lost parent: %+v %v", found, err)
	}
}

func TestMessageClaimInitialContractKeepsUserDecision(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	ch := taskSubmitChannel(t, pool, owner)
	worker := taskSubmitAgent(t, pool, owner)
	reviewer := taskSubmitAgent(t, pool, owner)
	for _, member := range []struct{ kind, id string }{{"user", owner}, {"agent", worker}, {"agent", reviewer}} {
		taskSubmitMember(t, pool, ch, member.kind, member.id)
	}
	message := func() string {
		id := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'Deliver the requested result')`, id, ch, owner); err != nil {
			t.Fatal(err)
		}
		return id
	}
	svc := NewTaskService(pool)
	source := message()
	contract := &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "Deliver the requested result"}}, Gate: TaskGate{Kind: "human", HumanReviewMode: "decision"}}
	task, created, err := svc.ClaimMessageTask(ctx, ch, source, worker, nil, contract)
	if err != nil || !created {
		t.Fatalf("claim: %+v %v", task, err)
	}
	if task.CreatorID != owner || task.MessageID != source || task.ClaimerID != worker || task.Contract.Gate.ReviewerID != owner {
		t.Fatalf("ownership changed: %+v", task)
	}
	replay := &TaskContract{Requirements: contract.Requirements, Gate: TaskGate{Kind: "human", HumanReviewMode: "decision"}}
	same, created, err := svc.ClaimMessageTask(ctx, ch, source, worker, nil, replay)
	if err != nil || created || same.ID != task.ID {
		t.Fatalf("replay: %+v %v", same, err)
	}
	changed := &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "Silently weaken the goal"}}, Gate: contract.Gate}
	if _, _, err = svc.ClaimMessageTask(ctx, ch, source, worker, nil, changed); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("rewrote existing contract: %v", err)
	}
	deniedSource := message()
	delegated := &TaskContract{Requirements: contract.Requirements, Gate: TaskGate{Kind: "agent", ReviewerID: reviewer}}
	if _, _, err = svc.ClaimMessageTask(ctx, ch, deniedSource, worker, nil, delegated); !errors.Is(err, ErrTaskDeliveryInvalid) {
		t.Fatalf("delegated user acceptance: %v", err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE message_id=$1`, deniedSource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed claim left task: %d %v", count, err)
	}
	legacySource := message()
	if _, _, err = svc.ClaimMessageTask(ctx, ch, legacySource, worker, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.ClaimMessageTask(ctx, ch, legacySource, worker, nil, contract); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("modified legacy task via claim: %v", err)
	}
}

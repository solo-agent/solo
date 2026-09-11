package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestTaskThreadFollowupProtectsDraftWithoutSelectingRun(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	other := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	taskSubmitMember(t, pool, channel, "agent", other)
	task, err := NewTaskService(pool).CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "follow-up target", Assignee: author})
	if err != nil {
		t.Fatal(err)
	}
	task.MessageID = agentRunMessage(t, pool, channel, owner)
	if _, err = pool.Exec(ctx, `UPDATE tasks SET message_id=$2 WHERE id=$1`, task.ID, task.MessageID); err != nil {
		t.Fatal(err)
	}
	thread, _, err := NewThreadService(pool).GetOrCreateThread(ctx, channel, task.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	var seq int64
	if err = pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id=$1`, task.MessageID).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	runs := NewAgentRunService(pool)
	run, err := runs.StartRun(ctx, StartRunInput{AgentID: author, ChannelID: channel, ThreadID: thread, TriggerType: AgentRunTriggerTask, FreshnessSeenSeq: seq})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO agent_run_task_links(run_id,task_id,role,confidence) VALUES($1,$2,'primary',1)`, run.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	infer := func(scope string) string {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = LockMessageScope(ctx, tx, channel, scope, ""); err != nil {
			t.Fatal(err)
		}
		id, err := TaskCorrectionRun(ctx, tx, channel, scope)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if infer("") != "" || infer(uuid.NewString()) != "" || infer(thread) != run.ID {
		t.Fatal("associated unrelated conversation or missed original task thread")
	}
	message := uuid.NewString()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = LockMessageScope(ctx, tx, channel, thread, ""); err != nil {
		t.Fatal(err)
	}
	target, err := TaskCorrectionRun(ctx, tx, channel, thread)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO messages(id,channel_id,thread_id,sender_type,sender_id,content,metadata) VALUES($1,$2,$3,'user',$4,'Use the revised requirement',jsonb_build_object('correction_of_run_id',$5::text))`, message, channel, thread, owner, target); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	hold := checkFreshnessInTransaction(t, pool, AgentSendFreshnessInput{RunID: run.ID, AgentID: author, ChannelID: channel, ThreadID: thread, Draft: "outdated result"})
	if hold == nil || len(hold.Messages) != 1 || hold.Messages[0].ID != message {
		t.Fatal("ordinary task-thread follow-up did not hold the outdated draft", hold)
	}
	second, err := runs.StartRun(ctx, StartRunInput{AgentID: other, ChannelID: channel, ThreadID: thread, TriggerType: AgentRunTriggerTask})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO agent_run_task_links(run_id,task_id,role,confidence) VALUES($1,$2,'related',1)`, second.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if infer(thread) != "" {
		t.Fatal("guessed a target among multiple active workers")
	}
	for _, id := range []string{run.ID, second.ID} {
		if _, err = runs.FinishRun(ctx, FinishRunInput{RunID: id, Status: AgentRunStatusCompleted}); err != nil {
			t.Fatal(err)
		}
	}
	if infer(thread) != "" {
		t.Fatal("associated a completed Run")
	}
}

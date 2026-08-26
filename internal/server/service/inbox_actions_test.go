package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestInboxActionsPersistReviewDecisions(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	agentID := taskSubmitAgent(t, pool, creatorID)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskSubmitMember(t, pool, channelID, "agent", agentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	acceptedID := taskSubmitTask(t, pool, channelID, creatorID, agentID, TaskStatusInReview, nil)
	rejectedID := taskSubmitTask(t, pool, channelID, creatorID, agentID, TaskStatusInReview, nil)
	for _, taskID := range []string{acceptedID, rejectedID} {
		_, err := pool.Exec(ctx,
			`INSERT INTO artifacts (id, task_id, channel_id, title, html_path, created_by)
			 VALUES ($1, $2, $3, 'Review result', $4, $5)`,
			uuid.NewString(), taskID, channelID, "/tmp/"+taskID+".html", agentID,
		)
		if err != nil {
			t.Fatalf("create artifact: %v", err)
		}
	}

	inbox := NewInboxService(pool)
	pending, err := inbox.ListActions(ctx, creatorID, "pending")
	if err != nil {
		t.Fatalf("ListActions pending: %v", err)
	}
	if len(pending) != 2 || pending[0].Type != "review" || pending[1].Type != "review" {
		t.Fatalf("pending actions = %+v, want two reviews", pending)
	}

	tasks := NewTaskService(pool)
	if _, err := tasks.AcceptTask(ctx, channelID, acceptedID, creatorID); err != nil {
		t.Fatalf("AcceptTask: %v", err)
	}
	if _, err := tasks.RejectTask(ctx, channelID, rejectedID, creatorID, "Please add evidence"); err != nil {
		t.Fatalf("RejectTask: %v", err)
	}

	handled, err := inbox.ListActions(ctx, creatorID, "handled")
	if err != nil {
		t.Fatalf("ListActions handled: %v", err)
	}
	if len(handled) != 2 {
		t.Fatalf("handled actions = %+v, want two decisions", handled)
	}
	decisions := map[string]string{}
	for _, action := range handled {
		decisions[action.TaskID] = *action.Decision
		if action.TaskID == acceptedID && action.NextOwnerName != nil {
			t.Fatalf("accepted next owner = %v, want nil", *action.NextOwnerName)
		}
		if action.TaskID == rejectedID && (action.Reason == nil || *action.Reason != "Please add evidence") {
			t.Fatalf("rejection reason = %v", action.Reason)
		}
	}
	if decisions[acceptedID] != "accepted" || decisions[rejectedID] != "rejected" {
		t.Fatalf("decisions = %#v", decisions)
	}
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRenderPendingArtifactHTML_DoesNotCloneThread(t *testing.T) {
	data := artifactRenderData{
		Task: ArtifactTask{
			ID:        "task-1",
			ChannelID: "channel-1",
			Number:    7,
			Title:     "<script>alert(1)</script>",
			Status:    TaskStatusInReview,
		},
		RootMessage: ArtifactMessage{
			SenderName: "Alice",
			Content:    `<img src=x onerror=alert(1)>`,
			CreatedAt:  time.Date(2026, 6, 23, 10, 5, 0, 0, time.UTC),
		},
		Thread: []ArtifactMessage{
			{SenderName: "Bob", Content: "This should not be copied into the pending artifact."},
		},
		GeneratedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
		Mode:        "latest",
	}

	html := renderPendingArtifactHTML(data)
	for _, want := range []string{"Generating artifact", "&lt;script&gt;alert(1)&lt;/script&gt;", "Task task-1", "oklch(0.975 0.008 80)", "#dbe5ed"} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected pending HTML to contain %q, got:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<img src=x") || strings.Contains(html, "This should not be copied") {
		t.Fatalf("pending artifact should not clone raw thread content, got:\n%s", html)
	}
}

func TestArtifactFilenameForMode(t *testing.T) {
	if got := artifactFilename("latest"); got != "latest.html" {
		t.Fatalf("latest filename = %q", got)
	}
	if got := artifactFilename("final"); got != "final.html" {
		t.Fatalf("final filename = %q", got)
	}
}

func TestArtifactServiceList_EmptyReturnsJSONArray(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskID := taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusInReview, nil)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE task_id = $1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	artifacts, err := NewArtifactService(pool, "").List(ctx, taskID, creatorID)
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if artifacts == nil {
		t.Fatal("expected empty artifact list to be a JSON array, got nil slice")
	}
	body, err := json.Marshal(artifacts)
	if err != nil {
		t.Fatalf("marshal artifacts: %v", err)
	}
	if string(body) != "[]" {
		t.Fatalf("empty artifact list JSON = %s, want []", body)
	}
}

func TestArtifactServiceRegenerateLatest_ForcesRequester(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	taskID := taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusInReview, nil)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE task_id = $1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	requests := 0
	svc := NewArtifactService(pool, t.TempDir())
	svc.SetAgentArtifactRequester(func(context.Context, *Task, artifactRenderData, string, string) error {
		requests++
		return nil
	})

	if _, err := svc.GenerateLatest(ctx, taskID, creatorID); err != nil {
		t.Fatalf("GenerateLatest error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("GenerateLatest requests = %d, want 1", requests)
	}
	if _, err := svc.RegenerateLatest(ctx, taskID, creatorID); err != nil {
		t.Fatalf("RegenerateLatest error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("RegenerateLatest requests = %d, want 2", requests)
	}
}

func TestArtifactServiceGenerateLatest_RejectsChildTask(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creatorID := taskSubmitUser(t, pool)
	channelID := taskSubmitChannel(t, pool, creatorID)
	taskSubmitMember(t, pool, channelID, "user", creatorID)
	parentID := taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusInProgress, nil)
	childID := taskSubmitTask(t, pool, channelID, creatorID, "", TaskStatusInReview, &parentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE task_id IN ($1, $2)`, parentID, childID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, creatorID)
	})

	_, err := NewArtifactService(pool, t.TempDir()).GenerateLatest(ctx, childID, creatorID)
	if !errors.Is(err, ErrArtifactChildTaskUnsupported) {
		t.Fatalf("GenerateLatest child error = %v, want ErrArtifactChildTaskUnsupported", err)
	}
}

func TestArtifactRenderDataFromTask_AllowsTaskWithoutSourceMessage(t *testing.T) {
	task := &Task{
		ID:          "task-1",
		ChannelID:   "channel-1",
		Title:       "Direct task",
		Description: "Created from the task board, not a message.",
		Status:      TaskStatusTodo,
		Priority:    "normal",
		TaskNumber:  9,
	}

	data := artifactRenderDataFromTask(task)
	if data.Task.ID != "task-1" || data.Task.Title != "Direct task" {
		t.Fatalf("expected task metadata in render data, got %#v", data.Task)
	}
	if data.RootMessage.Content != "Created from the task board, not a message." {
		t.Fatalf("expected task description as fallback root content, got %q", data.RootMessage.Content)
	}
}

func TestRenderArtifactAgentPrompt_InstructsPublishWithContext(t *testing.T) {
	data := artifactRenderData{
		Task: ArtifactTask{
			ID:          "task-1",
			ChannelID:   "channel-1",
			Number:      7,
			Title:       "Review decision",
			Description: "Decide whether to ship.",
			Status:      TaskStatusInReview,
			Priority:    "p1",
			CreatorName: "Ada",
			ClaimerName: "Grace",
		},
		RootMessage: ArtifactMessage{SenderName: "Alice", Content: "Need a visual review decision."},
		Thread: []ArtifactMessage{
			{SenderName: "Grace", Content: "Option A is safer; option B is faster."},
		},
		Subtasks: []ArtifactTask{
			{ID: "task-2", MessageID: "msg-child", Number: 8, Title: "E2E coverage", Status: TaskStatusDone, ClaimerName: "Grace"},
		},
		Mode: "latest",
	}

	prompt := renderArtifactAgentPrompt(data, "latest")
	for _, want := range []string{
		"solo-artifacts",
		"Deliverable: ./path/to/result.html",
		"Do not guess from old workspace files",
		"publish that file directly instead of summarizing",
		"prefer the product/final deliverable over review/report/panel files",
		"Do not switch to review-decision just because the task is in_review",
		"solo artifact publish --task task-1 --mode latest --file",
		"Need a visual review decision.",
		"Option A is safer; option B is faster.",
		"E2E coverage",
		"solo task list -c channel-1 --output json",
		"solo message read",
		"default Warm Archive skin",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected artifact agent prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

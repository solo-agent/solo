package service

import (
	"strings"
	"testing"
	"time"

	"github.com/solo-ai/solo/pkg/agent"
)

func TestTaskReviewPromptBindsCurrentTaskAndImmutableEvidence(t *testing.T) {
	prompt := taskReviewPrompt(7, "channel-id", "task-message-id", "submission-id", []byte(`{"artifact_version":"actual-digest"}`))
	for _, required := range []string{"[target=channel-id:task-message-id msg=task-message-id type=system]", "solo message send --target 'channel-id:task-message-id'", "solo task evidence -n 7 -c channel-id --submission submission-id", "reproducible input", `"artifact_version":"actual-digest"`, agent.TaskVerificationGuidance} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("review lost current context: %s", required)
		}
	}
	if strings.Contains(taskReviewPrompt(1, "channel-id", "", "s", nil), "channel-id:") {
		t.Fatal("invented a thread for a legacy task without a message")
	}
}

func TestTaskDueDateContextPreservesUTCAndAbsentDates(t *testing.T) {
	if taskDueDateContext(nil) != "" {
		t.Fatal("invented a deadline for an undated task")
	}
	due := time.Date(2026, 9, 12, 20, 30, 40, 123000000, time.FixedZone("UTC+8", 8*3600))
	if got := taskDueDateContext(&due); got != "\nTask due date (UTC): 2026-09-12T12:30:40.123Z." {
		t.Fatalf("wrong deadline: %q", got)
	}
}

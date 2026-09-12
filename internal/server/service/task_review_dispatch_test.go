package service

import (
	"strings"
	"testing"

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

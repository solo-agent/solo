package service

import (
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
	for _, want := range []string{"Generating artifact", "&lt;script&gt;alert(1)&lt;/script&gt;", "Task task-1"} {
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
		Mode: "latest",
	}

	prompt := renderArtifactAgentPrompt(data, "latest")
	for _, want := range []string{
		"solo-artifacts",
		"solo artifact publish --task task-1 --mode latest --file",
		"Need a visual review decision.",
		"Option A is safer; option B is faster.",
		"Solo-brutal",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected artifact agent prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

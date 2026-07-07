package handler

import "testing"

func TestThoughtNodeTitlesFromContextUsesAgendaExploreChildren(t *testing.T) {
	got := thoughtNodeTitlesFromContext("复制 Solo MVP", `[
		{"id":"explore","title":"探索你的想法","children":[
			{"id":"path","title":"产品路径"},
			{"id":"risk","title":"风险判断"}
		]},
		{"id":"work","title":"开始行动","children":[{"id":"task","title":"拆分任务"}]}
	]`)
	if len(got) < 2 || got[0] != "产品路径" || got[1] != "风险判断" {
		t.Fatalf("expected agenda explore nodes first, got %#v", got)
	}
}

func TestThoughtNodeTitlesFromContextAvoidsOldHardcodedNodes(t *testing.T) {
	got := thoughtNodeTitlesFromContext("我想复制一个 Solo MVP", `[]`)
	banned := map[string]bool{"Setup": true, "Product": true, "Architecture": true}
	for _, title := range got {
		if banned[title] {
			t.Fatalf("old hardcoded node leaked into thought nodes: %#v", got)
		}
	}
	if len(got) != 3 || got[0] != "产品路径" {
		t.Fatalf("expected Solo-specific fallback nodes, got %#v", got)
	}
}

func TestThoughtTitleFromContextUsesTarget(t *testing.T) {
	if got := thoughtTitleFromContext("", "复制 Solo MVP"); got != "复制 Solo MVP 探索" {
		t.Fatalf("expected target-based thought title, got %q", got)
	}
}

func TestThoughtReviewCardPayload(t *testing.T) {
	got := thoughtReviewCardPayload("thought-1", "产品路径探索")
	if got["card_type"] != "thought_review" {
		t.Fatalf("expected thought_review card, got %#v", got)
	}
	if got["thought_id"] != "thought-1" || got["title"] != "产品路径探索" {
		t.Fatalf("unexpected thought review identity: %#v", got)
	}
	if got["status"] != "open" || got["summary"] == "" {
		t.Fatalf("expected open review card with summary, got %#v", got)
	}
}

package service

import (
	"reflect"
	"testing"
)

func TestResolveAgentMentionsLongestExactName(t *testing.T) {
	candidates := []agentMentionCandidate{
		{ID: "short", Name: "data"},
		{ID: "long", Name: "data engineer"},
		{ID: "trae", Name: "dataleap-global coding developer"},
	}

	got := resolveAgentMentionCandidates(
		"请 @dataleap-global coding developer 实现初始化，再让 @data engineer review。",
		candidates,
	)
	want := []string{"trae", "long"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAgentMentionCandidates() = %#v, want %#v", got, want)
	}
}

func TestResolveAgentMentionsBoundariesAndDeduplication(t *testing.T) {
	candidates := []agentMentionCandidate{
		{ID: "data", Name: "data"},
		{ID: "cn", Name: "研发 工程师"},
		{ID: "duplicate-1", Name: "Same Name"},
		{ID: "duplicate-2", Name: "Same Name"},
	}

	got := resolveAgentMentionCandidates(
		"@database 不匹配；@data, 匹配；@研发 工程师。@data 重复；@Same Name!",
		candidates,
	)
	want := []string{"data", "cn", "duplicate-1", "duplicate-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAgentMentionCandidates() = %#v, want %#v", got, want)
	}
}

func TestResolveAgentMentionsUnknownAndPartialNames(t *testing.T) {
	candidates := []agentMentionCandidate{
		{ID: "trae", Name: "dataleap-global coding developer"},
	}

	for _, content := range []string{
		"@unknown do work",
		"@dataleap-global do work",
		"email@example.com",
	} {
		if got := resolveAgentMentionCandidates(content, candidates); len(got) != 0 {
			t.Fatalf("resolveAgentMentionCandidates(%q) = %#v, want no matches", content, got)
		}
	}
}

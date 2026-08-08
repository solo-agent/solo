package service

import (
	"reflect"
	"testing"
)

func TestFindMentionedAgentIDsSupportsSpaces(t *testing.T) {
	agents := []mentionCandidate{
		{id: "short", name: "Claude"},
		{id: "claude", name: "Claude Remote"},
		{id: "codex", name: "Codex Remote"},
	}
	want := []string{"claude", "codex"}
	got := findMentionedAgentIDs("@Claude Remote and @Codex Remote, take one turn each.", agents)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findMentionedAgentIDs() = %v, want %v", got, want)
	}
}

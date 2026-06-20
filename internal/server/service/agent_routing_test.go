package service

import "testing"

func TestFilterAgentsByID(t *testing.T) {
	agents := []agentChannelInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := filterAgentsByID(agents, []string{"c", "a"})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("unexpected filtered agents: %#v", got)
	}
}

func TestChooseCoordinatorFromEdges(t *testing.T) {
	agents := []agentChannelInfo{{ID: "leader"}, {ID: "worker"}}
	got := chooseCoordinatorFromEdges(agents, []relationshipEdge{{from: "leader", to: "worker"}})
	if len(got) != 1 || got[0].ID != "leader" {
		t.Fatalf("expected leader, got %#v", got)
	}
}

func TestChooseCoordinatorFromEdgesFallback(t *testing.T) {
	agents := []agentChannelInfo{{ID: "first"}, {ID: "second"}}
	got := chooseCoordinatorFromEdges(agents, nil)
	if len(got) != 1 || got[0].ID != "first" {
		t.Fatalf("expected first agent fallback, got %#v", got)
	}
}

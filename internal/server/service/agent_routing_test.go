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

func TestSelectWakeAgentIDsMatchesCurrentTaskRouting(t *testing.T) {
	active := []string{"lead", "be", "fe"}
	edges := []relationshipEdge{{from: "lead", to: "be"}, {from: "lead", to: "fe"}}

	tests := []struct {
		name string
		in   wakeRouteInput
		want []string
	}{
		{
			name: "explicit mention wins",
			in: wakeRouteInput{
				ActiveIDs:    active,
				MentionedIDs: []string{"be"},
				Edges:        edges,
			},
			want: []string{"be"},
		},
		{
			name: "unknown mention suppresses fallback",
			in: wakeRouteInput{
				ActiveIDs:  active,
				HasMention: true,
			},
			want: nil,
		},
		{
			name: "unmentioned task uses coordinator",
			in: wakeRouteInput{
				ActiveIDs: active,
				Edges:     edges,
			},
			want: []string{"lead"},
		},
		{
			name: "no relationship keeps first active fallback",
			in: wakeRouteInput{
				ActiveIDs: active,
			},
			want: []string{"lead"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectWakeAgentIDs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestShouldTriggerAgentForSender(t *testing.T) {
	tests := []struct {
		name        string
		senderType  string
		mentions    []string
		wantTrigger bool
	}{
		{name: "user message triggers", senderType: "user", wantTrigger: true},
		{name: "system message can trigger", senderType: "system", mentions: []string{"lead"}, wantTrigger: true},
		{name: "agent message without mention does not trigger", senderType: "agent", wantTrigger: false},
		{name: "agent message with mention can trigger", senderType: "agent", mentions: []string{"worker"}, wantTrigger: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldTriggerAgentForSender(tt.senderType, tt.mentions); got != tt.wantTrigger {
				t.Fatalf("got %v, want %v", got, tt.wantTrigger)
			}
		})
	}
}

func TestExcludeAgentPreventsSelfMentionLoop(t *testing.T) {
	agents := []agentChannelInfo{{ID: "sender"}, {ID: "peer"}}
	got := excludeAgent(agents, "sender")
	if len(got) != 1 || got[0].ID != "peer" {
		t.Fatalf("unexpected remaining agents: %#v", got)
	}
}

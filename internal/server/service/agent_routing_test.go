package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSelectWakeDecision(t *testing.T) {
	team := []string{"lead", "be", "fe", "qa"}
	edges := []relationshipEdge{
		{from: "lead", to: "be"},
		{from: "lead", to: "fe"},
		{from: "lead", to: "qa"},
	}

	tests := []struct {
		name       string
		facts      wakeRouteFacts
		wantIDs    []string
		wantReason wakeRouteReason
	}{
		{
			name:       "no active agent",
			facts:      wakeRouteFacts{Scope: wakeScopeChannel},
			wantReason: wakeReasonNoActiveAgent,
		},
		{
			name: "explicit mentions only wake active matches",
			facts: wakeRouteFacts{
				Scope:        wakeScopeChannel,
				ActiveIDs:    team,
				MentionedIDs: []string{"missing", "fe", "be"},
				HasMentions:  true,
			},
			wantIDs:    []string{"be", "fe"},
			wantReason: wakeReasonExplicitMention,
		},
		{
			name: "unresolved mention suppresses fallback",
			facts: wakeRouteFacts{
				Scope:       wakeScopeChannel,
				ActiveIDs:   team,
				HasMentions: true,
			},
			wantReason: wakeReasonUnresolvedMention,
		},
		{
			name: "channel uses unique coordinator",
			facts: wakeRouteFacts{
				Scope:        wakeScopeChannel,
				ActiveIDs:    team,
				GraphNodeIDs: team,
				Edges:        edges,
			},
			wantIDs:    []string{"lead"},
			wantReason: wakeReasonUniqueCoordinator,
		},
		{
			name: "small channel fans out without coordinator",
			facts: wakeRouteFacts{
				Scope:     wakeScopeChannel,
				ActiveIDs: []string{"a", "b", "c"},
			},
			wantIDs:    []string{"a", "b", "c"},
			wantReason: wakeReasonSmallTeamFanout,
		},
		{
			name: "large channel uses recent active window",
			facts: wakeRouteFacts{
				Scope:           wakeScopeChannel,
				ActiveIDs:       team,
				RecentActiveIDs: []string{"qa", "be", "missing"},
			},
			wantIDs:    []string{"qa", "be"},
			wantReason: wakeReasonRecentWindow,
		},
		{
			name: "large channel deterministically falls back to first",
			facts: wakeRouteFacts{
				Scope:     wakeScopeChannel,
				ActiveIDs: team,
			},
			wantIDs:    []string{"lead"},
			wantReason: wakeReasonDeterministicFirst,
		},
		{
			name: "multiple roots are not a unique coordinator",
			facts: wakeRouteFacts{
				Scope:        wakeScopeChannel,
				ActiveIDs:    team,
				GraphNodeIDs: team,
				Edges: []relationshipEdge{
					{from: "lead", to: "be"},
					{from: "fe", to: "qa"},
				},
				RecentActiveIDs: []string{"qa"},
			},
			wantIDs:    []string{"qa"},
			wantReason: wakeReasonRecentWindow,
		},
		{
			name: "cycle is not a unique coordinator",
			facts: wakeRouteFacts{
				Scope:        wakeScopeChannel,
				ActiveIDs:    team,
				GraphNodeIDs: team,
				Edges: []relationshipEdge{
					{from: "lead", to: "be"},
					{from: "be", to: "lead"},
					{from: "fe", to: "qa"},
				},
				RecentActiveIDs: []string{"fe"},
			},
			wantIDs:    []string{"fe"},
			wantReason: wakeReasonRecentWindow,
		},
		{
			name: "inactive coordinator is rejected",
			facts: wakeRouteFacts{
				Scope:        wakeScopeChannel,
				ActiveIDs:    team,
				GraphNodeIDs: append([]string{"inactive"}, team...),
				Edges: []relationshipEdge{
					{from: "inactive", to: "lead"},
					{from: "lead", to: "be"},
					{from: "lead", to: "fe"},
					{from: "lead", to: "qa"},
				},
				RecentActiveIDs: []string{"be"},
			},
			wantIDs:    []string{"be"},
			wantReason: wakeReasonRecentWindow,
		},
		{
			name: "thread participant coordinator wins",
			facts: wakeRouteFacts{
				Scope:                wakeScopeThread,
				ActiveIDs:            team,
				GraphNodeIDs:         team,
				Edges:                edges,
				ThreadParticipantIDs: []string{"be", "lead"},
			},
			wantIDs:    []string{"lead"},
			wantReason: wakeReasonThreadCoordinator,
		},
		{
			name: "thread uses most recent participant otherwise",
			facts: wakeRouteFacts{
				Scope:                wakeScopeThread,
				ActiveIDs:            team,
				GraphNodeIDs:         team,
				Edges:                edges,
				ThreadParticipantIDs: []string{"qa", "be"},
			},
			wantIDs:    []string{"qa"},
			wantReason: wakeReasonThreadRecentParticipant,
		},
		{
			name: "thread root agent precedes channel coordinator",
			facts: wakeRouteFacts{
				Scope:             wakeScopeThread,
				ActiveIDs:         team,
				GraphNodeIDs:      team,
				Edges:             edges,
				ThreadRootAgentID: "be",
			},
			wantIDs:    []string{"be"},
			wantReason: wakeReasonThreadRootAgent,
		},
		{
			name: "empty thread uses channel coordinator",
			facts: wakeRouteFacts{
				Scope:        wakeScopeThread,
				ActiveIDs:    team,
				GraphNodeIDs: team,
				Edges:        edges,
			},
			wantIDs:    []string{"lead"},
			wantReason: wakeReasonThreadChannelCoordinator,
		},
		{
			name: "thread never fans out and uses recent channel agent",
			facts: wakeRouteFacts{
				Scope:           wakeScopeThread,
				ActiveIDs:       []string{"a", "b", "c"},
				RecentActiveIDs: []string{"c", "b"},
			},
			wantIDs:    []string{"c"},
			wantReason: wakeReasonThreadRecentChannelAgent,
		},
		{
			name: "thread deterministic fallback remains single",
			facts: wakeRouteFacts{
				Scope:     wakeScopeThread,
				ActiveIDs: []string{"a", "b", "c"},
			},
			wantIDs:    []string{"a"},
			wantReason: wakeReasonDeterministicFirst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectWakeDecision(tt.facts)
			if !reflect.DeepEqual(got.AgentIDs, tt.wantIDs) || got.Reason != tt.wantReason {
				t.Fatalf("selectWakeDecision() = %#v, want IDs %v reason %q", got, tt.wantIDs, tt.wantReason)
			}
		})
	}
}

func TestRelationshipGraphForActiveKeepsInactiveRoot(t *testing.T) {
	nodes, edges := relationshipGraphForActive(
		[]string{"worker", "isolated"},
		[]relationshipEdge{
			{from: "inactive-lead", to: "worker"},
			{from: "unrelated", to: "other"},
		},
	)
	if !reflect.DeepEqual(nodes, []string{"worker", "isolated", "inactive-lead"}) {
		t.Fatalf("nodes = %v", nodes)
	}
	if !reflect.DeepEqual(edges, []relationshipEdge{{from: "inactive-lead", to: "worker"}}) {
		t.Fatalf("edges = %v", edges)
	}
}

func TestWakeRouterUsesPostgresHistoryAndThreadScope(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	channelID := agentRunChannel(t, pool, ownerID)
	agentIDList := []string{
		agentRunAgent(t, pool, ownerID),
		agentRunAgent(t, pool, ownerID),
		agentRunAgent(t, pool, ownerID),
		agentRunAgent(t, pool, ownerID),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1)`, agentIDList)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	for _, agentID := range agentIDList {
		if _, err := pool.Exec(ctx, `
			INSERT INTO channel_members (channel_id, member_type, member_id)
			VALUES ($1, 'agent', $2)
			ON CONFLICT DO NOTHING
		`, channelID, agentID); err != nil {
			t.Fatalf("add channel agent: %v", err)
		}
	}

	baseTime := time.Now().UTC().Add(-time.Hour)
	oldAgentMessageID := uuid.NewString()
	mentionedMessageID := uuid.NewString()
	triggerMessageID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, mentioned_agent_ids, created_at, updated_at)
		VALUES
			($1, $4, 'agent', $5, 'older agent activity', '{}', $8, $8),
			($2, $4, 'user', $6, 'recent mention', ARRAY[$7]::uuid[], $9, $9),
			($3, $4, 'user', $6, 'trigger', '{}', $10, $10)
	`, oldAgentMessageID, mentionedMessageID, triggerMessageID, channelID, agentIDList[0], ownerID, agentIDList[2], baseTime, baseTime.Add(time.Minute), baseTime.Add(2*time.Minute)); err != nil {
		t.Fatalf("insert channel history: %v", err)
	}

	service := &AgentService{pool: pool}
	agents, err := service.getChannelActiveAgents(ctx, channelID)
	if err != nil {
		t.Fatalf("get active agents: %v", err)
	}
	unknownIDs, hasMentions, err := NewMentionService(pool).ResolveMentions(ctx, "ask @missing-agent", channelID)
	if err != nil || !hasMentions || len(unknownIDs) != 0 {
		t.Fatalf("unknown mention = IDs %v hasMentions %v error %v", unknownIDs, hasMentions, err)
	}
	_, unresolvedDecision := service.routeWakeTargets(ctx, agents, wakeRouteRequest{
		Scope:       wakeScopeChannel,
		ChannelID:   channelID,
		HasMentions: hasMentions,
	})
	if unresolvedDecision.Reason != wakeReasonUnresolvedMention || len(unresolvedDecision.AgentIDs) != 0 {
		t.Fatalf("unresolved mention decision = %#v", unresolvedDecision)
	}
	_, decision := service.routeWakeTargets(ctx, agents, wakeRouteRequest{
		Scope:            wakeScopeChannel,
		ChannelID:        channelID,
		TriggerMessageID: triggerMessageID,
	})
	if decision.Reason != wakeReasonRecentWindow || !reflect.DeepEqual(decision.AgentIDs, []string{agentIDList[2], agentIDList[0]}) {
		t.Fatalf("recent-window decision = %#v", decision)
	}

	for _, childID := range agentIDList[1:] {
		if _, err := pool.Exec(ctx, `
			INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type)
			VALUES ($1, $2, 'assigns_to')
		`, agentIDList[0], childID); err != nil {
			t.Fatalf("insert relationship: %v", err)
		}
	}
	_, decision = service.routeWakeTargets(ctx, agents, wakeRouteRequest{
		Scope:            wakeScopeChannel,
		ChannelID:        channelID,
		TriggerMessageID: triggerMessageID,
	})
	if decision.Reason != wakeReasonUniqueCoordinator || !reflect.DeepEqual(decision.AgentIDs, agentIDList[:1]) {
		t.Fatalf("coordinator decision = %#v", decision)
	}

	rootMessageID := uuid.NewString()
	threadID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, created_at, updated_at)
		VALUES ($1, $2, 'agent', $3, 'thread root', $4, $4)
	`, rootMessageID, channelID, agentIDList[1], baseTime.Add(3*time.Minute)); err != nil {
		t.Fatalf("insert thread root: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO threads (id, channel_id, root_message_id, created_at)
		VALUES ($1, $2, $3, $4)
	`, threadID, channelID, rootMessageID, baseTime.Add(3*time.Minute)); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, thread_id, created_at, updated_at)
		VALUES
			($1, $2, 'agent', $3, 'older reply', $4, $5, $5),
			($6, $2, 'agent', $7, 'newer reply', $4, $8, $8)
	`, uuid.NewString(), channelID, agentIDList[2], threadID, baseTime.Add(4*time.Minute),
		uuid.NewString(), agentIDList[3], baseTime.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert thread replies: %v", err)
	}
	_, decision = service.routeWakeTargets(ctx, agents, wakeRouteRequest{
		Scope:     wakeScopeThread,
		ChannelID: channelID,
		ThreadID:  threadID,
	})
	if decision.Reason != wakeReasonThreadRecentParticipant || !reflect.DeepEqual(decision.AgentIDs, agentIDList[3:]) {
		t.Fatalf("thread participant decision = %#v", decision)
	}
}

func TestExcludeIDPreventsSelfMentionLoop(t *testing.T) {
	if got := excludeID([]string{"sender", "peer"}, "sender"); !reflect.DeepEqual(got, []string{"peer"}) {
		t.Fatalf("excludeID() = %v", got)
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

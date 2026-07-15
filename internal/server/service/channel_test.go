package service

import (
	"context"
	"testing"
)

func TestListMembersHidesInactiveAgentMembers(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	activeAgentID := taskSubmitAgent(t, pool, ownerID)
	inactiveAgentID := taskSubmitAgent(t, pool, ownerID)
	channelID := taskSubmitChannel(t, pool, ownerID)
	taskSubmitMember(t, pool, channelID, "user", ownerID)
	taskSubmitMember(t, pool, channelID, "agent", activeAgentID)
	taskSubmitMember(t, pool, channelID, "agent", inactiveAgentID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id IN ($1, $2)`, activeAgentID, inactiveAgentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `UPDATE agents SET is_active = false WHERE id = $1`, inactiveAgentID); err != nil {
		t.Fatalf("deactivate agent: %v", err)
	}

	members, err := NewChannelService(pool).ListMembers(ctx, channelID, ownerID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}

	foundActive := false
	for _, member := range members {
		if member.MemberID == inactiveAgentID {
			t.Fatalf("inactive agent member leaked into ListMembers")
		}
		if member.MemberID == activeAgentID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("active agent member missing from ListMembers")
	}
}

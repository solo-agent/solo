package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestOrdinaryChannelInheritsWorkspaceUsers(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	ownerID := taskSubmitUser(t, pool)
	memberID := taskSubmitUser(t, pool)
	workspaceID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id,name,created_by) VALUES ($1,$2,$3)`, workspaceID, "Channel inheritance "+workspaceID[:8], ownerID); err != nil {
		t.Fatalf("create Workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner'),($1,$3,'member')`, workspaceID, ownerID, memberID); err != nil {
		t.Fatalf("create Workspace members: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, workspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, ownerID, memberID)
	})

	channelID, err := NewChannelService(pool).CreateChannelInWorkspace(
		ctx, "inherited-"+workspaceID[:8], "", "channel", ownerID, workspaceID,
	)
	if err != nil {
		t.Fatalf("CreateChannelInWorkspace: %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT member_id::text,role FROM channel_members WHERE channel_id=$1 AND member_type='user'`, channelID)
	if err != nil {
		t.Fatalf("list inherited members: %v", err)
	}
	defer rows.Close()
	roles := map[string]string{}
	for rows.Next() {
		var id, role string
		if err := rows.Scan(&id, &role); err != nil {
			t.Fatalf("scan inherited member: %v", err)
		}
		roles[id] = role
	}
	if roles[ownerID] != "owner" || roles[memberID] != "member" || len(roles) != 2 {
		t.Fatalf("inherited roles = %#v", roles)
	}
	if _, err := NewChannelService(pool).RemoveMember(ctx, channelID, ownerID, memberID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("RemoveMember inherited User error = %v, want %v", err, ErrPermissionDenied)
	}
}

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

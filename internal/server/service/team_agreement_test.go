package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSharedTeamParticipationRequiresOwnersAndPreservesScope(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	peer := taskSubmitUser(t, pool)
	a := taskSubmitAgent(t, pool, owner)
	b := taskSubmitAgent(t, pool, peer)
	workspace := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,name,created_by) VALUES($1,'Shared regression',$2)`, workspace, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, workspace, owner, peer); err != nil {
		t.Fatal(err)
	}
	channels := NewChannelService(pool)
	channel, err := channels.CreateChannelInWorkspace(ctx, "shared-"+workspace[:8], "", "channel", owner, workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, workspace) })
	if err = channels.AddMember(ctx, channel, owner, "agent", b); err == nil {
		t.Fatal("owner attached somebody else's Agent")
	}
	if err = channels.AddMember(ctx, channel, owner, "agent", a); err != nil {
		t.Fatal(err)
	}
	if err = channels.AddMember(ctx, channel, peer, "agent", b); err != nil {
		t.Fatal(err)
	}
	agentProposal, err := ProposeTeamAgreement(ctx, pool, channel, a, CreateRelationshipRequest{FromAgentID: a, ToAgentID: b, RelType: RelCollaboratesWith, Instruction: "Review feedback: keep exact input/output examples"})
	if err != nil {
		t.Fatal(err)
	}
	if err = DecideTeamAgreement(ctx, pool, channel, a, agentProposal, "accept"); err == nil {
		t.Fatal("Agent approved its own cooperation proposal")
	}
	if err = DecideTeamAgreement(ctx, pool, channel, a, agentProposal, "withdraw"); err != nil {
		t.Fatal(err)
	}
	proposal, err := ProposeTeamAgreement(ctx, pool, channel, owner, CreateRelationshipRequest{FromAgentID: a, ToAgentID: b, RelType: RelAssignsTo, Instruction: "Only share approved delivery evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if err = DecideTeamAgreement(ctx, pool, channel, owner, proposal, "accept"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_relationships WHERE channel_id=$1`, channel).Scan(&count); err != nil || count != 0 {
		t.Fatal("agreement became active before both owners agreed")
	}
	if err = DecideTeamAgreement(ctx, pool, channel, peer, proposal, "accept"); err != nil {
		t.Fatal(err)
	}
	md, err := NewRelationshipsMDGenerator(pool, "").RenderForAgent(ctx, a, channel)
	if err != nil || !strings.Contains(md, "Only share approved") {
		t.Fatalf("agreement not dispatched: %s %v", md, err)
	}
	homeMD, err := NewRelationshipsMDGenerator(pool, "").RenderForAgent(ctx, a, taskSubmitChannel(t, pool, owner))
	if err != nil || strings.Contains(homeMD, "Only share approved") {
		t.Fatal("cross-channel agreement leaked into home dispatch")
	}
	version, err := PublishTeamVersion(ctx, pool, channel, owner, "", "joint publication")
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	if err = pool.QueryRow(ctx, `SELECT team_version_id IS NOT NULL FROM channels WHERE id=$1`, channel).Scan(&active); err != nil || active {
		t.Fatal("team published without peer consent")
	}
	if _, err = PublishTeamVersion(ctx, pool, channel, peer, version, "accept my pinned configuration"); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT team_version_id IS NOT NULL FROM channels WHERE id=$1`, channel).Scan(&active); err != nil || !active {
		t.Fatal("joint publication did not activate")
	}
	mark, err := SaveAgentWorkMark(ctx, pool, b, peer, AgentWorkMarkRequest{ChannelID: channel, Description: "shared obligation", IdempotencyKey: "shared-mark"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = channels.RemoveMember(ctx, channel, peer, b); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM agent_work_marks WHERE id=$1`, mark).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("revoked obligation still open: %s %v", status, err)
	}
	if _, err = PublishTeamVersion(ctx, pool, channel, owner, version, "try revoked snapshot"); err == nil {
		t.Fatal("revoked member resurrected by rollback")
	}
	if err = DecideTeamAgreement(ctx, pool, channel, peer, proposal, "accept"); err == nil {
		t.Fatal("withdrawn agreement resurrected")
	}
}

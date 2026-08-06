package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestThinkingSynthesisDraftSnapshotsPublishedHandoffsAndPathsInPostgres(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id)
		VALUES ($1, 'agent', $2)`, channelID, agentID); err != nil {
		t.Fatalf("add channel Agent: %v", err)
	}

	thinkingSvc := NewThinkingService(pool)
	space, err := thinkingSvc.Ensure(ctx, channelID, ownerID)
	if err != nil {
		t.Fatalf("ensure Thinking space: %v", err)
	}
	root := space.Nodes[0]
	first, err := thinkingSvc.CreateChild(ctx, channelID, root.ID, "Architecture path", ownerID, "manual")
	if err != nil {
		t.Fatalf("create first source node: %v", err)
	}
	second, err := thinkingSvc.CreateChild(ctx, channelID, root.ID, "Delivery path", ownerID, "manual")
	if err != nil {
		t.Fatalf("create second source node: %v", err)
	}
	missing, err := thinkingSvc.CreateChild(ctx, channelID, root.ID, "Unpublished path", ownerID, "manual")
	if err != nil {
		t.Fatalf("create unpublished node: %v", err)
	}

	insertAgentMessage := func(nodeID, content string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO messages
			    (id, channel_id, thinking_node_id, sender_type, sender_id, content)
			VALUES ($1, $2, $3, 'agent', $4, $5)`,
			uuid.NewString(), channelID, nodeID, agentID, content); err != nil {
			t.Fatalf("insert Agent message: %v", err)
		}
	}
	firstHandoff := "# Handoff\n## Confirmed conclusions\nKeep the channel API compatible."
	secondHandoff := "# Handoff\n## Risks and assumptions\nDelivery requires a staged rollout."
	insertAgentMessage(first.ID, "architecture result")
	if _, err := thinkingSvc.SaveCheckpointHandoff(ctx, channelID, first.ID, agentID, firstHandoff); err != nil {
		t.Fatalf("publish first Current State: %v", err)
	}
	insertAgentMessage(second.ID, "delivery result")
	if _, err := thinkingSvc.SaveCheckpointHandoff(ctx, channelID, second.ID, agentID, secondHandoff); err != nil {
		t.Fatalf("publish second Current State: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages
		    (id, channel_id, thinking_node_id, sender_type, sender_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, 'agent', $4, 'newer delivery detail', now() + interval '1 second', now() + interval '1 second')`,
		uuid.NewString(), channelID, second.ID, agentID); err != nil {
		t.Fatalf("make second Current State stale: %v", err)
	}

	var runsBefore int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_runs WHERE channel_id = $1`, channelID).Scan(&runsBefore); err != nil {
		t.Fatalf("count runs before draft: %v", err)
	}

	svc := NewThinkingSynthesisService(pool)
	draft, err := svc.Create(ctx, channelID, ownerID, CreateThinkingSynthesisInput{
		Title:     "Launch convergence",
		Objective: "Produce one compatible staged implementation plan.",
		NodeIDs:   []string{first.ID, second.ID},
		Constraints: ThinkingSynthesisConstraints{
			HardConstraints: []string{" Keep existing channel behavior ", "Use real PostgreSQL validation"},
		},
		Mode:               "single_agent",
		CoordinatorAgentID: agentID,
	})
	if err != nil {
		t.Fatalf("create synthesis draft: %v", err)
	}
	if draft.Status != "draft" || draft.Mode != "single_agent" || draft.CoordinatorAgentID != agentID {
		t.Fatalf("unexpected draft identity: %+v", draft)
	}
	if !draft.HasStaleSources || len(draft.Sources) != 2 {
		t.Fatalf("draft stale=%v sources=%d, want true/2", draft.HasStaleSources, len(draft.Sources))
	}
	if len(draft.Constraints.HardConstraints) != 2 || draft.Constraints.HardConstraints[0] != "Keep existing channel behavior" {
		t.Fatalf("normalized constraints = %#v", draft.Constraints.HardConstraints)
	}
	if draft.Sources[0].HandoffSnapshot != firstHandoff || draft.Sources[0].CheckpointStatusSnapshot != "fresh" {
		t.Fatalf("first source snapshot = %+v", draft.Sources[0])
	}
	if draft.Sources[1].HandoffSnapshot != secondHandoff || draft.Sources[1].CheckpointStatusSnapshot != "stale" {
		t.Fatalf("second source snapshot = %+v", draft.Sources[1])
	}
	for _, source := range draft.Sources {
		if len(source.PathSnapshot) != 2 || source.PathSnapshot[0].ID != root.ID || source.PathSnapshot[1].ID != source.NodeID {
			t.Fatalf("source path snapshot = %+v", source.PathSnapshot)
		}
	}

	if _, err := thinkingSvc.SaveCheckpointHandoff(ctx, channelID, first.ID, agentID,
		"# Handoff\n## Confirmed conclusions\nThis is a later update."); err != nil {
		t.Fatalf("update source after draft: %v", err)
	}
	reloaded, err := svc.Get(ctx, channelID, draft.ID)
	if err != nil {
		t.Fatalf("reload synthesis draft: %v", err)
	}
	if reloaded.Sources[0].HandoffSnapshot != firstHandoff {
		t.Fatalf("source snapshot mutated to %q", reloaded.Sources[0].HandoffSnapshot)
	}
	listed, err := svc.List(ctx, channelID)
	if err != nil || len(listed) != 1 || listed[0].ID != draft.ID {
		t.Fatalf("list syntheses = %+v, err %v", listed, err)
	}
	if _, err := svc.Get(ctx, uuid.NewString(), draft.ID); !errors.Is(err, ErrThinkingSynthesisNotFound) {
		t.Fatalf("cross-channel get error = %v, want ErrThinkingSynthesisNotFound", err)
	}
	if _, err := svc.Create(ctx, channelID, ownerID, CreateThinkingSynthesisInput{
		Objective: "This must fail because one source is unpublished.",
		NodeIDs:   []string{first.ID, missing.ID},
	}); !errors.Is(err, ErrThinkingSynthesisSourceAbsent) {
		t.Fatalf("unpublished source error = %v, want ErrThinkingSynthesisSourceAbsent", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM thinking_nodes WHERE id = $1`, first.ID); err != nil {
		t.Fatalf("delete snapshotted source node: %v", err)
	}
	afterSourceDelete, err := svc.Get(ctx, channelID, draft.ID)
	if err != nil {
		t.Fatalf("reload synthesis after source deletion: %v", err)
	}
	if len(afterSourceDelete.Sources) != 2 || afterSourceDelete.Sources[0].NodeTitle != "Architecture path" {
		t.Fatalf("historical sources after source deletion = %+v", afterSourceDelete.Sources)
	}

	var runsAfter int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_runs WHERE channel_id = $1`, channelID).Scan(&runsAfter); err != nil {
		t.Fatalf("count runs after draft: %v", err)
	}
	if runsAfter != runsBefore {
		t.Fatalf("draft creation started Agent runs: before=%d after=%d", runsBefore, runsAfter)
	}
}

func TestThinkingSynthesisInputValidation(t *testing.T) {
	valid := CreateThinkingSynthesisInput{
		Objective: "Converge",
		NodeIDs:   []string{uuid.NewString(), uuid.NewString()},
	}
	if err := normalizeThinkingSynthesisInput(&valid); err != nil {
		t.Fatalf("valid synthesis input: %v", err)
	}
	if valid.Title != "Converge" || valid.Mode != "single_agent" {
		t.Fatalf("normalized input = %+v", valid)
	}

	duplicate := CreateThinkingSynthesisInput{
		Objective: "Converge",
		NodeIDs:   []string{valid.NodeIDs[0], valid.NodeIDs[0]},
	}
	if err := normalizeThinkingSynthesisInput(&duplicate); !errors.Is(err, ErrThinkingSynthesisInvalid) {
		t.Fatalf("duplicate source error = %v", err)
	}
}

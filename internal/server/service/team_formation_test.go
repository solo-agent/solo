package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type teamFormationTestDocGenerator struct {
	failuresBeforeSuccess int
	calls                 int
}

func (g *teamFormationTestDocGenerator) GenerateForAgent(context.Context, string) error {
	g.calls++
	if g.calls <= g.failuresBeforeSuccess {
		return errors.New("temporary relationship document failure")
	}
	return nil
}

func validTeamPlanForTest() TeamFormationPlan {
	return TeamFormationPlan{
		IntentSummary: "Ship a reliable billing integration",
		Channel: TeamFormationChannel{
			Name:        "Billing Launch Team",
			Description: "Own the billing integration.",
		},
		TemplateID: "dev-team",
	}
}

func TestNormalizeAndValidateTeamPlanUsesOfficialTemplateID(t *testing.T) {
	plan := validTeamPlanForTest()
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if plan.TemplateID != "dev-team" {
		t.Fatalf("template id = %q", plan.TemplateID)
	}
	if plan.Channel.Name != "Billing Launch Team" {
		t.Fatalf("channel name should remain user-facing before provisioning: %q", plan.Channel.Name)
	}
}

func TestNormalizeAndValidateTeamPlanRejectsMissingTemplate(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.TemplateID = ""
	err := normalizeAndValidateTeamPlan(&plan)
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "template_id") {
		t.Fatalf("expected template_id error, got %v", err)
	}
}

func TestFinalizeRelationshipDocumentsRetriesTransientFailure(t *testing.T) {
	generator := &teamFormationTestDocGenerator{failuresBeforeSuccess: 1}
	svc := &TeamFormationService{mdGen: generator}
	result := &TeamFormationResult{
		Members: []TeamFormationResultMember{{ID: "agent-1", Name: "Engineer"}},
	}
	svc.finalizeRelationshipDocuments(result)
	if !result.RelationshipDocsReady || generator.calls != 2 {
		t.Fatalf("unexpected readiness=%v calls=%d warnings=%v", result.RelationshipDocsReady, generator.calls, result.Warnings)
	}
}

func TestFinalizeRelationshipDocumentsExposesPermanentFailure(t *testing.T) {
	generator := &teamFormationTestDocGenerator{failuresBeforeSuccess: 10}
	svc := &TeamFormationService{mdGen: generator}
	result := &TeamFormationResult{
		Members: []TeamFormationResultMember{{ID: "agent-2", Name: "Reviewer"}},
	}
	svc.finalizeRelationshipDocuments(result)
	if result.RelationshipDocsReady || len(result.Warnings) != 1 {
		t.Fatalf("unexpected readiness=%v warnings=%v", result.RelationshipDocsReady, result.Warnings)
	}
}

// TestTeamFormationInheritsSourceChannelWorkspace ensures that the new channel
// is created in the same workspace as the source lucy channel rather than
// silently falling back to the global Solo Public workspace. The original bug
// caused the Open Channel link in the resulting channel_created card to point
// at a channel invisible to the user's currently active workspace.
func TestTeamFormationInheritsSourceChannelWorkspace(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	workspaceID, lucyChannelID, lucyAgentID := seedLucyInPrivateWorkspace(t, pool, ownerID)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM team_formations
			 WHERE source_channel_id = $1 OR target_channel_id = $1
		`, lucyChannelID)
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM channels
			 WHERE id = $1
			    OR (workspace_id = $2 AND name = 'wf-inherit-test')
		`, lucyChannelID, workspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	})

	messageID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content)
		VALUES ($1, $2, 'user', $3, 'please set up a team')
	`, messageID, lucyChannelID, ownerID); err != nil {
		t.Fatalf("seed source message: %v", err)
	}

	svc := NewTeamFormationService(pool, &teamFormationTestDocGenerator{}, noopBroadcaster{})
	result, err := svc.Form(ctx, lucyAgentID, TeamFormationRequest{
		SourceChannelID: lucyChannelID,
		SourceMessageID: messageID,
		Plan: TeamFormationPlan{
			IntentSummary: "Regression test for workspace inheritance",
			Channel: TeamFormationChannel{
				Name:        "wf-inherit-test",
				Description: "Test that the new channel is in the source workspace.",
			},
			TemplateID: "agency-marketing-xiaohongshu-content",
		},
	})
	if err != nil {
		t.Fatalf("Form: %v", err)
	}

	var targetWorkspaceID string
	if err := pool.QueryRow(ctx, `SELECT workspace_id::text FROM channels WHERE id = $1`, result.ChannelID).Scan(&targetWorkspaceID); err != nil {
		t.Fatalf("read target channel workspace: %v", err)
	}
	if targetWorkspaceID != workspaceID {
		t.Fatalf("new channel workspace = %s, want %s (source workspace); channel leaked into another workspace", targetWorkspaceID, workspaceID)
	}
}

// seedLucyInPrivateWorkspace creates a non-public workspace, a Lucy channel
// inside it, and a Lucy agent whose home_channel_id points to that channel.
// Returns (workspaceID, channelID, agentID).
func seedLucyInPrivateWorkspace(t *testing.T, pool *pgxpool.Pool, ownerID string) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	workspaceID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, created_by) VALUES ($1, $2, $3)
	`, workspaceID, "wf-inherit-test-ws", ownerID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')
	`, workspaceID, ownerID); err != nil {
		t.Fatalf("add workspace member: %v", err)
	}

	channelID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO channels (id, workspace_id, name, type, created_by)
		VALUES ($1, $2, 'lucy', 'lucy', $3)
	`, channelID, workspaceID, ownerID); err != nil {
		t.Fatalf("create lucy channel: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner')
	`, channelID, ownerID); err != nil {
		t.Fatalf("add channel member: %v", err)
	}

	agentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, name, owner_id, model_provider, model_name, runtime_id, home_channel_id, kind)
		VALUES ($1, $2, $3, 'codex', 'gpt-test', '11111111-1111-4111-8111-111111111111', $4, 'lucy')
	`, agentID, "wf-inherit-test-lucy", ownerID, channelID); err != nil {
		t.Fatalf("create lucy agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
	})
	return workspaceID, channelID, agentID
}

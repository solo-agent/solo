package service

import (
	"context"
	"errors"
	"strings"
	"testing"
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

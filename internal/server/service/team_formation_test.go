package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		return fmt.Errorf("temporary relationship document failure")
	}
	return nil
}

func validTeamPlanForTest() TeamFormationPlan {
	return TeamFormationPlan{
		IntentSummary: "Ship a reliable billing integration",
		Channel: TeamFormationChannel{
			Name:        "Billing Launch Team",
			Description: "Own the billing integration from design through verification.",
		},
		RelationshipTemplate: "dev-team",
		Members: []TeamFormationMember{
			{
				Ref: "lead", Role: "leader", Name: "Billing Lead",
				Description: "Coordinates delivery.", Instructions: "Own scope, decisions, handoffs, and final review.",
			},
			{
				Ref: "engineer", Role: "engineer", Name: "Billing Engineer",
				Description: "Implements the integration.", Instructions: "Implement and test the billing integration.",
			},
			{
				Ref: "qa", Role: "reviewer", Name: "Billing QA",
				Description: "Validates correctness.", Instructions: "Design acceptance tests and review failure handling.",
			},
		},
	}
}

func testRelationshipTemplateMembers() []templateMember {
	engineer := "Delegate engineering work with scope, interfaces, and acceptance criteria."
	reviewer := "Delegate review work with risk areas and acceptance criteria."
	return []templateMember{
		{Role: "leader", Name: "Lead"},
		{Role: "engineer", Name: "Engineer", Relationship: &engineer},
		{Role: "reviewer", Name: "Reviewer", Relationship: &reviewer},
	}
}

func TestApplyRelationshipTemplateBuildsLeaderDelegationEdges(t *testing.T) {
	plan := validTeamPlanForTest()
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if len(plan.Relationships) != 0 {
		t.Fatalf("client normalization resolved relationships: %+v", plan.Relationships)
	}
	if err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers()); err != nil {
		t.Fatalf("applyRelationshipTemplate: %v", err)
	}
	if got, want := len(plan.Relationships), 2; got != want {
		t.Fatalf("template relationships = %d, want %d", got, want)
	}
	for _, relation := range plan.Relationships {
		if relation.FromRef != "lead" || relation.Type != "assigns_to" || relation.Weight != 1 {
			t.Fatalf("unexpected template relationship: %+v", relation)
		}
	}
}

func TestApplyRelationshipTemplatePreservesOrderedCriteriaForRepeatedRoles(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Members[2].Role = "engineer"
	frontEnd := "Frontend delegation criteria."
	backEnd := "Backend delegation criteria."
	template := []templateMember{
		{Role: "leader", Name: "Lead"},
		{Role: "engineer", Name: "FE", Relationship: &frontEnd},
		{Role: "engineer", Name: "BE", Relationship: &backEnd},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if err := applyRelationshipTemplate(&plan, template); err != nil {
		t.Fatalf("applyRelationshipTemplate: %v", err)
	}
	instructions := map[string]string{}
	for _, relation := range plan.Relationships {
		instructions[relation.ToRef] = relation.Instruction
	}
	if instructions["engineer"] != frontEnd || instructions["qa"] != backEnd {
		t.Fatalf("ordered criteria were not preserved: %+v", instructions)
	}
}

func TestApplyRelationshipTemplateCountsOrdinalsPerRole(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Members = []TeamFormationMember{
		plan.Members[0],
		plan.Members[2],
		{Ref: "frontend", Role: "engineer", Name: "Frontend", Instructions: "Implement the frontend."},
		{Ref: "backend", Role: "engineer", Name: "Backend", Instructions: "Implement the backend."},
	}
	frontEnd := "Frontend delegation criteria."
	backEnd := "Backend delegation criteria."
	review := "Review delegation criteria."
	template := []templateMember{
		{Role: "leader", Name: "Lead"},
		{Role: "engineer", Name: "FE", Relationship: &frontEnd},
		{Role: "engineer", Name: "BE", Relationship: &backEnd},
		{Role: "reviewer", Name: "Reviewer", Relationship: &review},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if err := applyRelationshipTemplate(&plan, template); err != nil {
		t.Fatalf("applyRelationshipTemplate: %v", err)
	}
	instructions := map[string]string{}
	for _, relation := range plan.Relationships {
		instructions[relation.ToRef] = relation.Instruction
	}
	if instructions["qa"] != review || instructions["frontend"] != frontEnd || instructions["backend"] != backEnd {
		t.Fatalf("per-role criteria were not preserved: %+v", instructions)
	}
}

func TestNormalizeAndValidateTeamPlanPromotesFirstMember(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Members[0].Role = "coordinator"
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if plan.Members[0].Role != "leader" {
		t.Fatalf("first role = %q, want leader", plan.Members[0].Role)
	}
}

func TestNormalizeAndValidateTeamPlanRejectsAssignmentCycle(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.RelationshipOverrides = []TeamFormationRelationshipOverride{
		{Operation: "upsert", FromRef: "engineer", ToRef: "lead", Type: "assigns_to", Reason: "Engineer must direct the lead for this workflow."},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers())
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "acyclic") {
		t.Fatalf("expected acyclic plan error, got %v", err)
	}
}

func TestNormalizeAndValidateTeamPlanRejectsKickoffTasks(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Tasks = []TeamFormationTask{{Title: "Do not create me"}}
	err := normalizeAndValidateTeamPlan(&plan)
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "does not create kickoff tasks") {
		t.Fatalf("expected kickoff task error, got %v", err)
	}
}

func TestNormalizeAndValidateTeamPlanRequiresOverrideReason(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.RelationshipOverrides = []TeamFormationRelationshipOverride{
		{Operation: "upsert", FromRef: "engineer", ToRef: "qa", Type: "collaborates_with"},
	}
	err := normalizeAndValidateTeamPlan(&plan)
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected override reason error, got %v", err)
	}
}

func TestApplyRelationshipTemplateAddsReasonedLucyOverride(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.RelationshipOverrides = []TeamFormationRelationshipOverride{
		{
			Operation: "upsert", FromRef: "engineer", ToRef: "qa", Type: "collaborates_with",
			Instruction: "Pair on failure-path verification.", Reason: "Billing error handling needs continuous QA pairing.",
		},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers()); err != nil {
		t.Fatalf("applyRelationshipTemplate: %v", err)
	}
	if got, want := len(plan.Relationships), 3; got != want {
		t.Fatalf("relationships = %d, want %d: %+v", got, want, plan.Relationships)
	}
}

func TestApplyRelationshipTemplateLetsLucyReplaceUnsuitableTemplateEdge(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.RelationshipOverrides = []TeamFormationRelationshipOverride{
		{
			Operation: "remove", FromRef: "lead", ToRef: "qa", Type: "assigns_to",
			Reason: "QA should stay independent from delivery delegation.",
		},
		{
			Operation: "upsert", FromRef: "engineer", ToRef: "qa", Type: "collaborates_with",
			Instruction: "Coordinate verification without creating a reporting line.",
			Reason:      "Peer collaboration better preserves independent review.",
		},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	if err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers()); err != nil {
		t.Fatalf("applyRelationshipTemplate: %v", err)
	}
	keys := make(map[string]bool, len(plan.Relationships))
	for _, relation := range plan.Relationships {
		keys[relationshipKey(relation.FromRef, relation.ToRef, relation.Type)] = true
	}
	if keys["assigns_to:lead:qa"] || !keys["collaborates_with:engineer:qa"] {
		t.Fatalf("Lucy override was not applied: %+v", plan.Relationships)
	}
}

func TestApplyRelationshipTemplateRejectsDisconnectedOverride(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.RelationshipOverrides = []TeamFormationRelationshipOverride{
		{Operation: "remove", FromRef: "lead", ToRef: "qa", Type: "assigns_to", Reason: "Remove the template edge."},
	}
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalizeAndValidateTeamPlan: %v", err)
	}
	err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers())
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "connected") {
		t.Fatalf("expected connectivity error, got %v", err)
	}
}

func TestNormalizeAndValidateTeamPlanRejectsResolvedRelationshipsFromCaller(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Relationships = []TeamFormationRelation{{FromRef: "lead", ToRef: "qa", Type: "assigns_to", Weight: 1}}
	err := normalizeAndValidateTeamPlan(&plan)
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "server-managed") {
		t.Fatalf("expected server-managed relationship error, got %v", err)
	}
}

func TestNormalizeAndValidateTeamPlanRejectsInvalidExistingAgentID(t *testing.T) {
	plan := validTeamPlanForTest()
	plan.Members[0].ExistingAgentID = "not-an-id"
	err := normalizeAndValidateTeamPlan(&plan)
	if !errors.Is(err, ErrInvalidTeamFormationPlan) || !strings.Contains(err.Error(), "existing_agent_id") {
		t.Fatalf("expected existing_agent_id error, got %v", err)
	}
}

func TestSlugifyTeamName(t *testing.T) {
	tests := map[string]string{
		" Billing Launch Team ": "billing-launch-team",
		"支付 项目 / 二期":            "支付-项目-二期",
		"---":                   "team",
	}
	for input, want := range tests {
		if got := slugifyTeamName(input); got != want {
			t.Errorf("slugifyTeamName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSanitizeAgentHandle(t *testing.T) {
	tests := map[string]string{
		"Billing Lead":  "Billing-Lead",
		"支付 / QA":       "支付-QA",
		"research.lead": "research.lead",
	}
	for input, want := range tests {
		if got := sanitizeAgentHandle(input); got != want {
			t.Errorf("sanitizeAgentHandle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildTeamKickoffMessageMakesNoTaskPromise(t *testing.T) {
	plan := validTeamPlanForTest()
	got := buildTeamKickoffMessage(plan, "Ship billing", "billing-team", []TeamFormationResultMember{{Name: "Lead", Role: "leader"}})
	for _, want := range []string{"Relationship template: dev-team", "No tasks were created automatically", "#billing-team"} {
		if !strings.Contains(got, want) {
			t.Fatalf("kickoff %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "Kickoff tasks") {
		t.Fatalf("kickoff still contains task list: %q", got)
	}
}

func TestFinalizeRelationshipDocumentsRetriesTransientFailure(t *testing.T) {
	generator := &teamFormationTestDocGenerator{failuresBeforeSuccess: 2}
	svc := &TeamFormationService{mdGen: generator}
	result := &TeamFormationResult{
		FormationID: "formation-1",
		Members:     []TeamFormationResultMember{{ID: "agent-1", Name: "Engineer"}},
	}

	svc.finalizeRelationshipDocuments(result)
	if !result.RelationshipDocsReady || len(result.Warnings) != 0 || generator.calls != 3 {
		t.Fatalf("unexpected finalized result: %+v calls=%d", result, generator.calls)
	}
}

func TestFinalizeRelationshipDocumentsExposesPermanentFailure(t *testing.T) {
	generator := &teamFormationTestDocGenerator{failuresBeforeSuccess: 99}
	svc := &TeamFormationService{mdGen: generator}
	result := &TeamFormationResult{
		FormationID: "formation-2",
		Members:     []TeamFormationResultMember{{ID: "agent-2", Name: "Reviewer"}},
	}

	svc.finalizeRelationshipDocuments(result)
	if result.RelationshipDocsReady || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "@Reviewer") {
		t.Fatalf("unexpected finalized result: %+v", result)
	}
}

type teamFormationTestBroadcaster struct {
	userID string
	events [][]byte
}

func (b *teamFormationTestBroadcaster) BroadcastToScope(string, string, []byte) {}
func (b *teamFormationTestBroadcaster) BroadcastToChannel(string, []byte)       {}
func (b *teamFormationTestBroadcaster) BroadcastToThread(string, []byte)        {}
func (b *teamFormationTestBroadcaster) Broadcast([]byte)                        {}
func (b *teamFormationTestBroadcaster) SendToUser(userID string, message []byte) {
	b.userID = userID
	b.events = append(b.events, message)
}

func TestTeamFormationServiceIntegration(t *testing.T) {
	dsn := os.Getenv("TEAM_FORMATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEAM_FORMATION_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	ownerID := uuid.New().String()
	lucyID := uuid.New().String()
	sourceChannelID := uuid.New().String()
	sourceMessageID := uuid.New().String()
	unique := strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash)
		VALUES ($1, $2, 'Team Test Owner', 'test')`, ownerID, "team-"+unique+"@example.com")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE channel_id IN (SELECT id FROM channels WHERE created_by = $1)`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM agents WHERE owner_id = $1`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	}()

	if _, err := pool.Exec(ctx, `
		INSERT INTO channels (id, name, description, created_by)
		VALUES ($1, $2, 'Onboarding', $3)`, sourceChannelID, "welcome-team-test-"+unique, ownerID); err != nil {
		t.Fatalf("insert source channel: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner')`, sourceChannelID, ownerID); err != nil {
		t.Fatalf("insert owner membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (
			id, name, description, owner_id, model_provider, model_name,
			system_prompt, custom_env, custom_args
		) VALUES ($1, 'Lucy', 'Onboarding lead', $2, 'codex', '', 'Onboard', '{}'::jsonb, '[]'::jsonb)`,
		lucyID, ownerID); err != nil {
		t.Fatalf("insert Lucy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'agent', $2, 'member')`, sourceChannelID, lucyID); err != nil {
		t.Fatalf("insert Lucy membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content)
		VALUES ($1, $2, 'user', $3, 'Build and ship a reliable billing integration')`,
		sourceMessageID, sourceChannelID, ownerID); err != nil {
		t.Fatalf("insert source message: %v", err)
	}

	plan := validTeamPlanForTest()
	plan.Channel.Name = "billing-team-" + unique
	broadcaster := &teamFormationTestBroadcaster{}
	svc := NewTeamFormationService(pool, nil, broadcaster)
	result, err := svc.Form(ctx, lucyID, TeamFormationRequest{
		SourceChannelID: sourceChannelID,
		SourceMessageID: sourceMessageID[:8],
		Plan:            plan,
	})
	if err != nil {
		t.Fatalf("form team: %v", err)
	}
	if result.Replayed || len(result.Members) != 3 || len(result.Tasks) != 0 || result.RelationshipTemplate != "dev-team" || result.RelationshipOverrides != 0 || result.RelationshipCount != 2 || !result.RelationshipDocsReady {
		t.Fatalf("unexpected result: %+v", result)
	}
	if broadcaster.userID != ownerID || len(broadcaster.events) != 1 {
		t.Fatalf("unexpected broadcasts: user=%q count=%d", broadcaster.userID, len(broadcaster.events))
	}

	var memberCount, relationshipCount, taskCount, taskThreadCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM channel_members WHERE channel_id = $1`, result.ChannelID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM agent_relationships r
		  JOIN channel_members cm ON cm.member_id = r.from_agent_id AND cm.member_type = 'agent'
		 WHERE cm.channel_id = $1`, result.ChannelID).Scan(&relationshipCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE channel_id = $1 AND creator_id = $2`, result.ChannelID, ownerID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM tasks t
		  JOIN threads th ON th.root_message_id = t.message_id
		 WHERE t.channel_id = $1`, result.ChannelID).Scan(&taskThreadCount); err != nil {
		t.Fatal(err)
	}
	if memberCount != 4 || relationshipCount != 2 || taskCount != 0 || taskThreadCount != 0 {
		t.Fatalf("provisioned counts: members=%d relationships=%d tasks=%d task_threads=%d", memberCount, relationshipCount, taskCount, taskThreadCount)
	}

	replayed, err := svc.Form(ctx, lucyID, TeamFormationRequest{
		SourceChannelID: sourceChannelID,
		SourceMessageID: sourceMessageID,
		Plan:            plan,
	})
	if err != nil {
		t.Fatalf("replay team formation: %v", err)
	}
	if !replayed.Replayed || replayed.ChannelID != result.ChannelID || len(broadcaster.events) != 1 {
		t.Fatalf("unexpected replay result: %+v broadcasts=%d", replayed, len(broadcaster.events))
	}
}

func TestTeamFormationStaleLeaseCannotProvisionDuplicateTeam(t *testing.T) {
	dsn := os.Getenv("TEAM_FORMATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEAM_FORMATION_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	ownerID := uuid.New().String()
	lucyID := uuid.New().String()
	sourceChannelID := uuid.New().String()
	sourceMessageID := uuid.New().String()
	unique := strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash)
		VALUES ($1, $2, 'Lease Test Owner', 'test')`, ownerID, "lease-"+unique+"@example.com"); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM agents WHERE owner_id = $1`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	}()
	if _, err := pool.Exec(ctx, `
		INSERT INTO channels (id, name, description, created_by)
		VALUES ($1, $2, 'Onboarding', $3)`, sourceChannelID, "welcome-lease-test-"+unique, ownerID); err != nil {
		t.Fatalf("insert source channel: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner')`, sourceChannelID, ownerID); err != nil {
		t.Fatalf("insert owner membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (
			id, name, description, owner_id, model_provider, model_name,
			system_prompt, custom_env, custom_args
		) VALUES ($1, 'Lucy', 'Onboarding lead', $2, 'codex', '', 'Onboard', '{}'::jsonb, '[]'::jsonb)`,
		lucyID, ownerID); err != nil {
		t.Fatalf("insert Lucy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'agent', $2, 'member')`, sourceChannelID, lucyID); err != nil {
		t.Fatalf("insert Lucy membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content)
		VALUES ($1, $2, 'user', $3, 'Build a lease-safe team')`,
		sourceMessageID, sourceChannelID, ownerID); err != nil {
		t.Fatalf("insert source message: %v", err)
	}

	plan := validTeamPlanForTest()
	plan.Channel.Name = "lease-team-" + unique
	if err := normalizeAndValidateTeamPlan(&plan); err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	if err := applyRelationshipTemplate(&plan, testRelationshipTemplateMembers()); err != nil {
		t.Fatalf("resolve template: %v", err)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	caller := &teamFormationCaller{
		AgentID: lucyID, OwnerID: ownerID, Provider: "codex",
		ChannelID: sourceChannelID, SourceID: sourceMessageID, SourceText: "Build a lease-safe team",
	}
	svc := NewTeamFormationService(pool, nil, nil)
	formationID, firstLease, replay, err := svc.claimFormation(ctx, caller, planJSON)
	if err != nil || replay != nil {
		t.Fatalf("first claim: id=%q replay=%+v err=%v", formationID, replay, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE team_formations SET updated_at = now() - interval '3 minutes'
		 WHERE id = $1`, formationID); err != nil {
		t.Fatalf("age first lease: %v", err)
	}
	secondID, secondLease, replay, err := svc.claimFormation(ctx, caller, planJSON)
	if err != nil || replay != nil || secondID != formationID || secondLease.Equal(firstLease) {
		t.Fatalf("stale retry claim: id=%q lease=%v replay=%+v err=%v", secondID, secondLease, replay, err)
	}

	if _, err := svc.provision(ctx, formationID, firstLease, caller, plan); !errors.Is(err, errTeamFormationLeaseLost) {
		t.Fatalf("stale provision error = %v, want lease lost", err)
	}
	result, err := svc.provision(ctx, formationID, secondLease, caller, plan)
	if err != nil {
		t.Fatalf("active provision: %v", err)
	}
	if result.ChannelID == "" {
		t.Fatal("active provision returned no channel")
	}

	var channelCount, createdAgentCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM channels WHERE created_by = $1 AND id <> $2`, ownerID, sourceChannelID).Scan(&channelCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM agents WHERE owner_id = $1 AND id <> $2`, ownerID, lucyID).Scan(&createdAgentCount); err != nil {
		t.Fatal(err)
	}
	if channelCount != 1 || createdAgentCount != len(plan.Members) {
		t.Fatalf("duplicate resources created: channels=%d agents=%d", channelCount, createdAgentCount)
	}
}

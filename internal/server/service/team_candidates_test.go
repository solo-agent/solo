package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestFormationCandidatesUseRealPermissionsRolesAndBudgets(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	worker := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", worker)
	tmpl, err := NewTemplateService(pool).Get(ctx, "agency-marketing-xiaohongshu-content")
	if err != nil {
		t.Fatal(err)
	}
	role := tmpl.Members[0]
	computer := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO computers(id,name,owner_id,daemon_id,status,runtime_inventory) VALUES($1,'candidate test',$2,$3,'online','[{"type":"claude","available":true}]')`, computer, owner, "daemon-e2e-candidate-"+computer); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE channels SET source_template_id=$2 WHERE id=$1`, channel, tmpl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE agents SET home_channel_id=$2,system_prompt=$3,model_provider='claude',model_name='haiku',runtime_id=$4 WHERE id=$1`, worker, channel, role.Instructions, computer); err != nil {
		t.Fatal(err)
	}
	if _, err = NewTaskService(pool).CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "assigned in this role", Assignee: worker}); err != nil {
		t.Fatal(err)
	}
	caller := &teamFormationCaller{OwnerID: owner, Provider: "claude", RuntimeID: computer}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	candidates, err := formationCandidates(ctx, tx, caller, tmpl, role, TeamMemberRequirement{Ref: role.Ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || !candidates[0].Eligible || candidates[0].Delegations != 1 || candidates[0].TokensPerQualified != nil {
		t.Fatalf("candidate evidence %+v", candidates)
	}
	missing, err := formationCandidates(ctx, tx, caller, tmpl, role, TeamMemberRequirement{Ref: role.Ref, RequiredSkills: []string{"missing-skill"}})
	if err != nil {
		t.Fatal(err)
	}
	if missing[0].Eligible || missing[0].ExcludedReason != "Missing required Skill: missing-skill" {
		t.Fatal("ignored tools", missing)
	}
	mismatch, err := formationCandidates(ctx, tx, caller, tmpl, role, TeamMemberRequirement{Ref: role.Ref, ModelProvider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch[0].Eligible {
		t.Fatal("ignored model condition")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO budget_policies(owner_id,scope_type,agent_id,enabled,monthly_limit_tokens) VALUES($1,'agent',$2,true,0)`, owner, worker); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	blocked, err := formationCandidates(ctx, tx, caller, tmpl, role, TeamMemberRequirement{Ref: role.Ref})
	if err != nil {
		t.Fatal(err)
	}
	if blocked[0].Eligible {
		t.Fatal("ignored exhausted budget")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	outsider := taskSubmitUser(t, pool)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	foreign, err := formationCandidates(ctx, tx, &teamFormationCaller{OwnerID: outsider}, tmpl, role, TeamMemberRequirement{Ref: role.Ref, AgentID: worker})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign) != 0 {
		t.Fatal("explicit ID bypassed ownership")
	}
	if err = validateTeamMemberRequirements(tmpl, []TeamMemberRequirement{{Ref: role.Ref}, {Ref: role.Ref}}); err == nil {
		t.Fatal("duplicate role accepted")
	}
}

func TestFormationReusesIdentityAndScopesApprovedRelationships(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	_, lucyChannel, lucy := seedLucyInPrivateWorkspace(t, pool, owner)
	workspaceRoot := t.TempDir()
	svc := NewTeamFormationService(pool, NewRelationshipsMDGenerator(pool, workspaceRoot), nil)
	form := func(name string, reuse *bool) (*TeamFormationResult, error) {
		message := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content) VALUES($1,$2,'user',$3,'form this team')`, message, lucyChannel, owner); err != nil {
			t.Fatal(err)
		}
		req := TeamFormationRequest{SourceChannelID: lucyChannel, SourceMessageID: message, Plan: TeamFormationPlan{IntentSummary: "real identity reuse", Channel: TeamFormationChannel{Name: name}, TemplateID: "agency-dev-api-doc-gen", ReuseExisting: reuse}}
		result, err := svc.Form(ctx, lucy, req)
		if err != nil {
			return nil, err
		}
		replay, err := svc.Form(ctx, lucy, req)
		if err != nil || !replay.Replayed || replay.ChannelID != result.ChannelID {
			t.Fatal("formation idempotence", err, replay)
		}
		return result, nil
	}
	first, err := form("first-"+uuid.NewString(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Members) != 2 {
		t.Fatal(first)
	}
	// A real workspace may belong to a currently executing Run. Formation must
	// not replace its exact snapshot with an aggregate of every joined channel.
	for _, m := range first.Members {
		dir := filepath.Join(workspaceRoot, m.ID, "workspace")
		if err = os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "RELATIONSHIPS.md"), []byte("active run snapshot"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	second, err := form("second-"+uuid.NewString(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range second.Members {
		if !m.Reused || m.ID != first.Members[i].ID || m.Evidence == nil || m.SelectionReason == "" {
			t.Fatal("identity was cloned", m)
		}
		var home string
		if err = pool.QueryRow(ctx, `SELECT home_channel_id::text FROM agents WHERE id=$1`, m.ID).Scan(&home); err != nil || home != first.ChannelID {
			t.Fatal("home channel changed", home, err)
		}
		if content, e := os.ReadFile(filepath.Join(workspaceRoot, m.ID, "workspace", "RELATIONSHIPS.md")); e != nil || string(content) != "active run snapshot" {
			t.Fatal("formation overwrote active Run relationship snapshot", e)
		}
		if content, e := NewRelationshipsMDGenerator(pool, "").RenderForAgent(ctx, m.ID, second.ChannelID); e != nil || content == "" {
			t.Fatal("new Run relationship snapshot unavailable", e)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM agent_relationships r JOIN agent_relationship_proposals p ON p.relationship_id=r.id WHERE r.channel_id=$1 AND p.channel_id=$1 AND p.status='accepted' AND $2::uuid=ANY(p.approved_owner_ids)`, second.ChannelID, owner).Scan(&count); err != nil || count != second.RelationshipCount {
		t.Fatal("reuse bypassed exact channel agreement", count, err)
	}
	var privateNotes bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages m,jsonb_array_elements(m.metadata->'members') member WHERE m.channel_id=$1 AND member ? 'evidence')`, second.ChannelID).Scan(&privateNotes); err != nil || privateNotes {
		t.Fatal("past private notes leaked to new team", err)
	}
	noReuse := false
	fresh, err := form("fresh-"+uuid.NewString(), &noReuse)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range fresh.Members {
		if m.Reused || m.ID == first.Members[i].ID {
			t.Fatal("explicit new team reused a member")
		}
	}
}

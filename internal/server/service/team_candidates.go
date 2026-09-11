package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/skillloader"
)

// Requirements refine an official template role. They never grant access to a
// Computer or modify the configuration of a member selected for reuse.
type TeamMemberRequirement struct {
	Ref            string   `json:"ref"`
	AgentID        string   `json:"agent_id,omitempty"`
	RequiredSkills []string `json:"required_skills,omitempty"`
	ModelProvider  string   `json:"model_provider,omitempty"`
	ModelName      string   `json:"model_name,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type TeamCandidate struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Ref                string          `json:"ref"`
	ModelProvider      string          `json:"model_provider"`
	ModelName          string          `json:"model_name"`
	RuntimeID          string          `json:"computer_id"`
	Skills             []string        `json:"skills"`
	Qualified          int             `json:"qualified"`
	Reworks            int             `json:"reworks"`
	Mentions           int             `json:"mentions"`
	Claims             int             `json:"claims"`
	Delegations        int             `json:"delegations"`
	CostSamples        int             `json:"cost_samples"`
	CostCondition      string          `json:"cost_condition,omitempty"`
	TokensPerQualified *float64        `json:"tokens_per_qualified"`
	Feedback           json.RawMessage `json:"feedback"`
	Eligible           bool            `json:"eligible"`
	ExcludedReason     string          `json:"excluded_reason,omitempty"`
}

// Keep the eligibility and ordering identical for the read-only preview and the
// transactional provisioner. Callers provide only a Lucy-authorized owner.
func formationCandidates(ctx context.Context, tx pgx.Tx, caller *teamFormationCaller, tmpl *AgentTemplate, role TemplateMember, need TeamMemberRequirement) ([]TeamCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT a.id::text,a.name,a.model_provider,a.model_name,COALESCE(a.runtime_id,''),
 COALESCE((SELECT jsonb_agg(skill->>'name') FROM jsonb_array_elements(a.skills) skill),'[]'),
 COALESCE(c.runtime_inventory,'[]'),
 COALESCE((c.owner_id=$1 OR EXISTS(SELECT 1 FROM computer_members cm WHERE cm.computer_id=c.id AND cm.user_id=$1)) AND c.credential_revoked_at IS NULL AND (c.credential_hash IS NOT NULL OR (c.daemon_id IS NOT NULL AND c.status='online')),false),
 EXISTS(SELECT 1 FROM agent_runs r JOIN agent_revisions v ON v.id=r.agent_revision_id WHERE r.agent_id=a.id AND r.backend_started_at IS NOT NULL AND v.config=solo_agent_config(a) AND r.started_at>now()-interval '90 days')
 FROM agents a JOIN channels home ON home.id=a.home_channel_id
 LEFT JOIN computers c ON c.id::text=a.runtime_id
 WHERE a.owner_id=$1 AND a.kind='agent' AND a.is_active AND NOT home.is_archived
 AND NOT EXISTS(SELECT 1 FROM agent_selection_trials tr WHERE tr.agent_id=a.id)
 AND (a.id::text=$4 OR (home.source_template_id=$2 AND a.system_prompt=$5) OR EXISTS(
 SELECT 1 FROM team_formations f, jsonb_array_elements(COALESCE(f.result->'members','[]')) member
 WHERE f.status='completed' AND f.result->>'template_id'=$2 AND member->>'ref'=$3 AND member->>'id'=a.id::text))
 ORDER BY a.id FOR SHARE OF a`, caller.OwnerID, tmpl.ID, role.Ref, need.AgentID, role.Instructions)
	if err != nil {
		return nil, err
	}
	candidates := []TeamCandidate{}
	for rows.Next() {
		var c TeamCandidate
		var inventory []agent.BackendStatus
		var access, proved bool
		if err = rows.Scan(&c.ID, &c.Name, &c.ModelProvider, &c.ModelName, &c.RuntimeID, &c.Skills, &inventory, &access, &proved); err != nil {
			rows.Close()
			return nil, err
		}
		c.Ref = role.Ref
		c.Eligible = true
		switch {
		case !access:
			c.ExcludedReason = "Computer access is unavailable or revoked"
		case need.ModelProvider != "" && need.ModelProvider != c.ModelProvider:
			c.ExcludedReason = "Provider does not match this role's fixed requirement"
		case need.ModelName != "" && need.ModelName != c.ModelName:
			c.ExcludedReason = "Model does not match this role's fixed requirement"
		default:
			available := false
			for _, backend := range inventory {
				if backend.Type == c.ModelProvider && backend.Available {
					available = true
				}
			}
			// Legacy local transport has no inventory. A current verified Lucy runtime
			// or an actual Run with this exact configuration supplies execution evidence.
			if len(inventory) == 0 {
				available = proved || (c.RuntimeID == caller.RuntimeID && c.ModelProvider == caller.Provider)
			}
			if !available {
				c.ExcludedReason = "Computer has no verified backend for this Provider"
			}
		}
		for _, required := range need.RequiredSkills {
			found := false
			for _, installed := range c.Skills {
				if installed == required {
					found = true
				}
			}
			if !found {
				c.ExcludedReason = "Missing required Skill: " + required
				break
			}
		}
		c.Eligible = c.ExcludedReason == ""
		candidates = append(candidates, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	budgets := NewBudgetService(nil)
	for i := range candidates {
		c := &candidates[i]
		// A preview must not reserve Tokens or create a Run. Reuse the real policy
		// checks under their existing locks and repeat them at actual Run admission.
		if c.Eligible {
			if _, err = budgets.buildRunPlanTx(ctx, tx, c.ID); err != nil {
				var blocked *BudgetStartError
				if !errors.As(err, &blocked) {
					return nil, err
				}
				c.Eligible = false
				c.ExcludedReason = blocked.Error()
			}
		}
		err = tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM tasks t JOIN task_submissions s ON s.id=t.current_submission_id WHERE t.status='done' AND s.submitted_by=$2 AND EXISTS(SELECT 1 FROM task_reviews r WHERE r.submission_id=s.id AND r.decision='accepted') AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1) AND EXISTS(SELECT 1 FROM channels domain WHERE domain.id=t.channel_id AND domain.source_template_id=$3)),
 (SELECT count(*) FROM task_reviews r JOIN task_submissions s ON s.id=r.submission_id JOIN tasks t ON t.id=s.task_id WHERE r.decision='rejected' AND s.submitted_by=$2 AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1) AND EXISTS(SELECT 1 FROM channels domain WHERE domain.id=t.channel_id AND domain.source_template_id=$3)),
 (SELECT count(*) FROM messages msg WHERE $2::uuid=ANY(msg.mentioned_agent_ids) AND NOT msg.is_deleted AND msg.sender_type='user' AND msg.sender_id=$1),
 (SELECT count(*) FROM task_responsibility_events e JOIN tasks t ON t.id=e.task_id WHERE e.assignee_id=$2 AND e.kind='claimed' AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1) AND EXISTS(SELECT 1 FROM channels domain WHERE domain.id=t.channel_id AND domain.source_template_id=$3)),
 (SELECT count(*) FROM task_responsibility_events e JOIN tasks t ON t.id=e.task_id WHERE e.assignee_id=$2 AND e.kind='assigned' AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1) AND EXISTS(SELECT 1 FROM channels domain WHERE domain.id=t.channel_id AND domain.source_template_id=$3)),
 COALESCE((SELECT jsonb_agg(feedback) FROM (
 SELECT jsonb_build_object('task_id',t.id,'channel_id',t.channel_id,'title',t.title,'kind',o.kind,'note',o.note,'category',o.category) feedback FROM task_observations o JOIN tasks t ON t.id=o.task_id
 WHERE o.kind IN ('rework','handoff','reexplanation','recovery') AND EXISTS(SELECT 1 FROM task_submissions s WHERE s.task_id=t.id AND s.submitted_by=$2)
 AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1)
 AND EXISTS(SELECT 1 FROM channels domain WHERE domain.id=t.channel_id AND domain.source_template_id=$3) ORDER BY o.created_at DESC LIMIT 10) recent),'[]')`, caller.OwnerID, c.ID, tmpl.ID).Scan(&c.Qualified, &c.Reworks, &c.Mentions, &c.Claims, &c.Delegations, &c.Feedback)
		if err != nil {
			return nil, err
		}
		// Reuse the delivery denominator: failed terminal Tasks stay in the same
		// explicitly classified cohort. Mixed or unknown conditions yield no cost rank.
		costRows, err := tx.Query(ctx, `SELECT d.metrics FROM task_delivery_measurements d JOIN tasks t ON t.id=d.id JOIN channels domain ON domain.id=t.channel_id
  WHERE domain.source_template_id=$3 AND t.status IN ('done','closed')
  AND (t.claimer_id=$2 OR EXISTS(SELECT 1 FROM task_submissions sub WHERE sub.task_id=t.id AND sub.submitted_by=$2) OR EXISTS(SELECT 1 FROM task_responsibility_events e WHERE e.task_id=t.id AND e.assignee_id=$2))
  AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=t.channel_id AND m.member_type='user' AND m.member_id=$1)
  AND NOT EXISTS(SELECT 1 FROM agent_run_task_links l JOIN agent_runs r ON r.id=l.run_id JOIN agents a ON a.id=r.agent_id LEFT JOIN agent_revisions v ON v.id=r.agent_revision_id WHERE l.task_id=t.id AND v.config IS DISTINCT FROM solo_agent_config(a))`, caller.OwnerID, c.ID, tmpl.ID)
		if err != nil {
			return nil, err
		}
		measured := []TaskDeliveryMetric{}
		for costRows.Next() {
			var m TaskDeliveryMetric
			if err = costRows.Scan(&m); err != nil {
				costRows.Close()
				return nil, err
			}
			redactDeliveryBudgets(&m, caller.OwnerID)
			measured = append(measured, m)
		}
		err = costRows.Err()
		costRows.Close()
		if err != nil {
			return nil, err
		}
		groups := deliveryCohorts(measured)
		if len(groups) == 1 {
			g := groups[0]
			if g.ComparableTasks == g.Tasks && g.ComparableQualified > 0 {
				c.CostSamples = g.ComparableTasks
				cost := float64(g.ComparableTokens) / float64(g.ComparableQualified)
				c.TokensPerQualified = &cost
				condition, _ := json.Marshal([]any{g.Cohort, g.Models, g.Budgets})
				c.CostCondition = string(condition)
			}
		}

	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Eligible != b.Eligible {
			return a.Eligible
		}
		// A fixed add-one prior tempers tiny samples; raw counts stay visible.
		aRate := float64(a.Qualified+1) / float64(a.Qualified+a.Reworks+2)
		bRate := float64(b.Qualified+1) / float64(b.Qualified+b.Reworks+2)
		if aRate != bRate {
			return aRate > bRate
		}
		if a.Qualified != b.Qualified {
			return a.Qualified > b.Qualified
		}
		if a.CostCondition != "" && a.CostCondition == b.CostCondition && a.TokensPerQualified != nil && b.TokensPerQualified != nil && *a.TokensPerQualified != *b.TokensPerQualified {
			return *a.TokensPerQualified < *b.TokensPerQualified
		}
		if a.Mentions != b.Mentions {
			return a.Mentions > b.Mentions
		}
		return a.ID < b.ID
	})
	return candidates, nil
}

func validateTeamMemberRequirements(tmpl *AgentTemplate, needs []TeamMemberRequirement) error {
	roles := map[string]bool{}
	for _, member := range tmpl.Members {
		roles[member.Ref] = true
	}
	seen := map[string]bool{}
	agents := map[string]bool{}
	for _, need := range needs {
		if !roles[need.Ref] || seen[need.Ref] || len(need.RequiredSkills) > 20 || len(need.ModelProvider) > 80 || len(need.ModelName) > 160 || len(need.Reason) > 4000 {
			return fmt.Errorf("%w: invalid or duplicate role requirements", ErrInvalidTeamFormationPlan)
		}
		seen[need.Ref] = true
		if need.AgentID != "" {
			if agents[need.AgentID] {
				return fmt.Errorf("%w: independent roles need different Agents", ErrInvalidTeamFormationPlan)
			}
			agents[need.AgentID] = true
		}
		skillNames := map[string]bool{}
		for _, name := range need.RequiredSkills {
			if !skillloader.ValidBundleName(name) || skillNames[name] {
				return fmt.Errorf("%w: invalid required Skill", ErrInvalidTeamFormationPlan)
			}
			skillNames[name] = true
		}
	}
	return nil
}

func resolveFormationMembers(ctx context.Context, tx pgx.Tx, caller *teamFormationCaller, tmpl *AgentTemplate, plan TeamFormationPlan) (map[string]TeamCandidate, error) {
	if caller == nil {
		return nil, ErrTeamFormationForbidden
	}
	if err := validateTeamMemberRequirements(tmpl, plan.Members); err != nil {
		return nil, err
	}
	roles := map[string]bool{}
	for _, member := range tmpl.Members {
		roles[member.Ref] = true
	}
	edges := map[string]bool{}
	for _, edge := range plan.Relationships {
		key := edge.FromRef + ":" + edge.ToRef + ":" + edge.Type
		if !roles[edge.FromRef] || !roles[edge.ToRef] || edge.FromRef == edge.ToRef || edges[key] || strings.TrimSpace(edge.Instruction) == "" || len(edge.Instruction) > 8000 {
			return nil, fmt.Errorf("%w: invalid relationship role or instruction", ErrInvalidTeamFormationPlan)
		}
		if err := ValidateRelationshipCreate(CreateRelationshipRequest{FromAgentID: edge.FromRef, ToAgentID: edge.ToRef, RelType: edge.Type, Weight: &edge.Weight, Instruction: edge.Instruction}); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidTeamFormationPlan, err)
		}
		edges[key] = true
	}
	needs := map[string]TeamMemberRequirement{}
	for _, need := range plan.Members {
		needs[need.Ref] = need
	}
	result := map[string]TeamCandidate{}
	used := map[string]bool{}
	names := map[string]bool{}
	reuse := plan.ReuseExisting == nil || *plan.ReuseExisting
	for _, role := range tmpl.Members {
		need := needs[role.Ref]
		candidates, err := formationCandidates(ctx, tx, caller, tmpl, role, need)
		if err != nil {
			return nil, err
		}
		var selected *TeamCandidate
		for i := range candidates {
			c := &candidates[i]
			if c.Eligible && !used[c.ID] && !names[strings.ToLower(c.Name)] && (need.AgentID == "" || need.AgentID == c.ID) && (reuse || need.AgentID != "") {
				selected = c
				break
			}
		}
		if selected != nil {
			result[role.Ref] = *selected
			used[selected.ID] = true
			names[strings.ToLower(selected.Name)] = true
			continue
		}
		if need.AgentID != "" || len(need.RequiredSkills) > 0 {
			return nil, fmt.Errorf("%w: role %s has no authorized member satisfying its Agent, Skill, model and budget requirements", ErrInvalidTeamFormationPlan, role.Ref)
		}
		// Do not evade a member's exhausted allowance by silently making a new copy.
		for _, c := range candidates {
			if reuse && strings.Contains(c.ExcludedReason, "Token budget") {
				return nil, fmt.Errorf("%w: %s: %s", ErrInvalidTeamFormationPlan, role.Ref, c.ExcludedReason)
			}
		}
		provider, model := caller.Provider, caller.ModelName
		if need.ModelProvider != "" {
			provider = need.ModelProvider
		}
		if need.ModelName != "" {
			model = need.ModelName
		}
		var inventory []agent.BackendStatus
		var available bool
		err = tx.QueryRow(ctx, `SELECT COALESCE(c.runtime_inventory,'[]'),c.credential_revoked_at IS NULL AND (c.credential_hash IS NOT NULL OR (c.daemon_id IS NOT NULL AND c.status='online')) FROM computers c WHERE c.id::text=$1 AND (c.owner_id=$2 OR EXISTS(SELECT 1 FROM computer_members m WHERE m.computer_id=c.id AND m.user_id=$2)) FOR SHARE OF c`, caller.RuntimeID, caller.OwnerID).Scan(&inventory, &available)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !available {
			return nil, fmt.Errorf("%w: source Computer is unavailable for role %s", ErrInvalidTeamFormationPlan, role.Ref)
		}
		if err != nil {
			return nil, err
		}
		supported := len(inventory) == 0 && provider == caller.Provider
		for _, backend := range inventory {
			if backend.Type == provider && backend.Available {
				supported = true
			}
		}
		if !supported {
			return nil, fmt.Errorf("%w: role %s requires an available %s backend", ErrInvalidTeamFormationPlan, role.Ref, provider)
		}
		result[role.Ref] = TeamCandidate{Ref: role.Ref, Name: role.Name, ModelProvider: provider, ModelName: model, RuntimeID: caller.RuntimeID, Eligible: true}
		names[strings.ToLower(role.Name)] = true
	}
	return result, nil
}

func formationMemberReason(c TeamCandidate, reused bool) string {
	if !reused {
		return "职责暂无合适复用成员；按指定执行配置新建，启动时重新检查预算。"
	}
	cost := "成本样本不足"
	if c.TokensPerQualified != nil {
		cost = fmt.Sprintf("%d 个完整成本样本，平均 %.0f Token", c.CostSamples, *c.TokensPerQualified)
	}
	return fmt.Sprintf("复用已满足权限与工具要求的成员：同模板领域 %d 次合格交付、%d 次退回；%d 次明确提及、%d 次主动认领、%d 次指派；%s。", c.Qualified, c.Reworks, c.Mentions, c.Claims, c.Delegations, cost)
}

type TeamRoleCandidates struct {
	Ref        string          `json:"ref"`
	Role       string          `json:"role"`
	Candidates []TeamCandidate `json:"candidates"`
}

func (s *TeamFormationService) Candidates(ctx context.Context, callerID string, req TeamFormationRequest) ([]TeamRoleCandidates, error) {
	if !teamTemplatePattern.MatchString(req.Plan.TemplateID) || !messageIDPattern.MatchString(req.SourceMessageID) {
		return nil, ErrInvalidTeamFormationPlan
	}
	caller, err := s.authorizeCaller(ctx, callerID, req.SourceChannelID, req.SourceMessageID)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	tmpl, err := loadTemplate(ctx, tx, req.Plan.TemplateID)
	if err != nil {
		return nil, err
	}
	if err = validateTeamMemberRequirements(tmpl, req.Plan.Members); err != nil {
		return nil, err
	}
	requirements := map[string]TeamMemberRequirement{}
	for _, need := range req.Plan.Members {
		requirements[need.Ref] = need
	}
	result := make([]TeamRoleCandidates, 0, len(tmpl.Members))
	for _, member := range tmpl.Members {
		candidates, err := formationCandidates(ctx, tx, caller, tmpl, member, requirements[member.Ref])
		if err != nil {
			return nil, err
		}
		result = append(result, TeamRoleCandidates{Ref: member.Ref, Role: member.Role, Candidates: candidates})
	}
	return result, nil
}

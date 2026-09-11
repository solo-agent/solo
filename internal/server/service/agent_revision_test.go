package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/skillloader"
)

func TestAgentRevisionUsesRegisteredBackends(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	original := taskSubmitAgent(t, pool, owner)
	providers := []string{"openai", "anthropic"}
	for _, meta := range agent.GlobalRegistry().ListMeta() {
		providers = append(providers, meta.Type)
	}
	for _, provider := range providers {
		req := AgentRevisionRequest{Summary: "native provider compatibility", Config: AgentRevisionConfig{ModelProvider: provider, ModelName: "configured-model", SystemPrompt: "preserve existing backend"}}
		if _, err := CreateAgentRevision(ctx, pool, original, owner, req); err != nil {
			t.Fatalf("registered provider %s: %v", provider, err)
		}
	}
	if _, err := CreateAgentRevision(ctx, pool, original, owner, AgentRevisionRequest{Summary: "unknown", Config: AgentRevisionConfig{ModelProvider: "not-registered", ModelName: "model"}}); err == nil {
		t.Fatal("accepted unknown provider")
	}
}

func TestAgentRevisionPostgresPublicationAndTeamPin(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	original := taskSubmitAgent(t, pool, owner)
	evaluation := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", original)
	taskSubmitMember(t, pool, channel, "agent", evaluation)
	config := AgentRevisionConfig{SystemPrompt: "candidate verified behavior", ModelProvider: "claude", ModelName: "sonnet", CustomArgs: []string{}}
	skills, err := skillloader.NormalizeBundles([]skillloader.Bundle{{Name: "checked-output", Files: []skillloader.BundleFile{{Path: "SKILL.md", Content: []byte("---\nname: checked-output\ndescription: Verify output\n---\nRun scripts/check.sh")}, {Path: "scripts/check.sh", Content: []byte("#!/bin/sh\necho 42\n"), Executable: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	config.Skills = skills
	if _, err := pool.Exec(ctx, `UPDATE agents SET system_prompt=$2,model_provider=$3,model_name=$4,custom_args=$5,skills=$6 WHERE id=$1`, evaluation, config.SystemPrompt, config.ModelProvider, config.ModelName, config.CustomArgs, revisionSkillsJSON(config.Skills)); err != nil {
		t.Fatal(err)
	}
	revision, err := CreateAgentRevision(ctx, pool, original, owner, AgentRevisionRequest{Config: config, Summary: "candidate", EvaluationAgentID: evaluation})
	if err != nil {
		t.Fatal(err)
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, revision, "", "no evidence"); err == nil {
		t.Fatal("published unevaluated candidate")
	}
	tasks := NewTaskService(pool)
	task, err := tasks.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "candidate regression", Assignee: evaluation, Contract: &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "verified output"}}, Gate: TaskGate{Kind: "human", ReviewerID: owner}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: evaluation, ChannelID: channel, TriggerType: AgentRunTriggerTask, Source: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	// Updating live configuration cannot change the queued Run's immutable snapshot.
	if _, err = pool.Exec(ctx, `UPDATE agents SET system_prompt='later edit' WHERE id=$1`, evaluation); err != nil {
		t.Fatal(err)
	}
	req := daemonTaskRequest{}
	if err = applyRunRevision(ctx, pool, &req, run.ID); err != nil {
		t.Fatal(err)
	}
	if req.SystemPrompt != config.SystemPrompt || req.AgentRevisionID == "" || skillloader.BundlesDigest(req.Skills) != skillloader.BundlesDigest(config.Skills) {
		t.Fatalf("wrong pinned configuration: %+v", req)
	}
	task, err = tasks.SubmitDelivery(ctx, channel, task.ID, evaluation, run.ID, TaskSubmitRequest{ExpectedTaskVersion: task.Version, IdempotencyKey: "candidate-submit", ArtifactVersion: "verified-v1", Handoff: TaskHandoff{Summary: "verified"}, Evidence: []TaskEvidence{{ID: "E1", Description: "output", Content: "actual fixture output"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.ReviewDelivery(ctx, channel, task.ID, owner, TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: "verified-v1", IdempotencyKey: "candidate-review", Decision: "accepted", Reason: "verified", Checks: []TaskCheck{{RequirementID: "R1", Passed: true, Reason: "reproduced", EvidenceIDs: []string{"E1"}}}}); err != nil {
		t.Fatal(err)
	}
	if err = PublishAgentRevision(ctx, pool, original, evaluation, revision, task.ID, "self publish"); err == nil {
		t.Fatal("Agent published without owner")
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, revision, task.ID, "single-sided acceptance"); err == nil {
		t.Fatal("single-sided evaluation bypassed paired Selection")
	}
	// A publication created before Selection was introduced remains a valid rollback target.
	// The new publication path is exercised by TestSelectionPostgresPlanBudgetAndDecision.
	if _, err = pool.Exec(ctx, `INSERT INTO agent_revision_publications(agent_id,revision_id,published_by,reason) VALUES($1,$2,$3,'Previously published fixture')`, original, revision, owner); err != nil {
		t.Fatal(err)
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, revision, task.ID, "restore previously published candidate"); err != nil {
		t.Fatal(err)
	}
	var prompt string
	if err = pool.QueryRow(ctx, `SELECT system_prompt FROM agents WHERE id=$1`, original).Scan(&prompt); err != nil || prompt != config.SystemPrompt {
		t.Fatalf("not published: %s %v", prompt, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE agent_revisions SET config='{}' WHERE id=$1`, revision); err == nil {
		t.Fatal("mutated snapshot")
	}
	data, err := ListAgentRevisions(ctx, pool, original, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatal("invalid revision metrics")
	}
	version, err := PublishTeamVersion(ctx, pool, channel, owner, "", "release team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE agents SET system_prompt='unpublished edit' WHERE id=$1`, original); err != nil {
		t.Fatal(err)
	}
	teamRun, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: original, ChannelID: channel, TriggerType: AgentRunTriggerTask, Source: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err = applyRunRevision(ctx, pool, &req, teamRun.ID); err != nil {
		t.Fatal(err)
	}
	if req.TeamVersionID != version || req.AgentRevisionID != revision || req.SystemPrompt != config.SystemPrompt || skillloader.BundlesDigest(req.Skills) != skillloader.BundlesDigest(config.Skills) {
		t.Fatal("team pin changed after live edit")
	}
	// A server restart must not resume an old Team session just because the Agent config is unchanged.
	session, err := NewAgentRunService(pool).UpsertSession(ctx, UpsertSessionInput{AgentID: original, Provider: "claude", ExternalSessionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewAgentRunService(pool).BindRunSession(ctx, BindRunSessionInput{RunID: teamRun.ID, SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	// Restore the current live configuration so only the Team ID changes.
	if _, err = pool.Exec(ctx, `UPDATE agents SET system_prompt=$2 WHERE id=$1`, original, config.SystemPrompt); err != nil {
		t.Fatal(err)
	}
	secondVersion, err := PublishTeamVersion(ctx, pool, channel, owner, "", "republish same member config")
	if err != nil {
		t.Fatal(err)
	}
	nextRun, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: original, ChannelID: channel, TriggerType: AgentRunTriggerTask, Source: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	req = daemonTaskRequest{ResumeSessionID: session.ID}
	if err = applyRunRevision(ctx, pool, &req, nextRun.ID); err != nil || !req.ForceFreshSession || req.ResumeSessionID != "" || req.TeamVersionID != secondVersion {
		t.Fatalf("reused previous team session: %+v %v", req, err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM channel_members WHERE channel_id=$1 AND member_id=$2`, channel, evaluation); err != nil {
		t.Fatal(err)
	}
	if _, err = PublishTeamVersion(ctx, pool, channel, owner, version, "restore revoked member"); err == nil {
		t.Fatal("rollback restored revoked membership")
	}
	if _, err = PublishTeamVersion(ctx, pool, channel, owner, "live", "release pin"); err != nil {
		t.Fatal(err)
	}
	var previousID string
	if err = pool.QueryRow(ctx, `SELECT v.id::text FROM agent_revisions v JOIN agent_revision_publications p ON p.revision_id=v.id WHERE v.agent_id=$1 AND NOT (v.config ? 'skills') LIMIT 1`, original).Scan(&previousID); err != nil {
		t.Fatal(err)
	}
	if err = PublishAgentRevision(ctx, pool, original, owner, previousID, "", "restore previous live files"); err != nil {
		t.Fatal(err)
	}
	var removed bool
	if err = pool.QueryRow(ctx, `SELECT skills='[]'::jsonb AND NOT (solo_agent_config(a) ? 'skills') FROM agents a WHERE id=$1`, original).Scan(&removed); err != nil || !removed {
		t.Fatalf("rollback/legacy hash changed: %v", err)
	}
	if _, err = ListAgentRevisions(ctx, pool, original, uuid.NewString()); err == nil {
		t.Fatal("outsider read private configuration")
	}
}

func TestRevisionRuntimeCapabilities(t *testing.T) {
	skills := []skillloader.Bundle{{Name: "checked-output"}}
	for _, c := range []struct {
		request   daemonTaskRequest
		caps      []string
		wantError bool
	}{
		{daemonTaskRequest{}, nil, false},
		{daemonTaskRequest{TeamVersionID: "fixed"}, nil, true},
		{daemonTaskRequest{Skills: skills}, []string{"run_snapshot_v1"}, true},
		{daemonTaskRequest{Skills: skills}, []string{"skill_bundle_v1"}, true},
		{daemonTaskRequest{Skills: skills}, []string{"run_snapshot_v1", "skill_bundle_v1"}, false},
	} {
		if err := validateRevisionRuntime(c.request, c.caps); (err != nil) != c.wantError {
			t.Fatalf("capabilities %v: %v", c.caps, err)
		}
	}
}

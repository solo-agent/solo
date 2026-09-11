package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/skillloader"
)

type AgentRevisionConfig struct {
	Skills        []skillloader.Bundle `json:"skills,omitempty"`
	SystemPrompt  string               `json:"system_prompt"`
	ModelProvider string               `json:"model_provider"`
	ModelName     string               `json:"model_name"`
	CustomArgs    []string             `json:"custom_args"`
}

type AgentRevisionRequest struct {
	Config            AgentRevisionConfig `json:"config"`
	Summary           string              `json:"summary"`
	EvaluationAgentID string              `json:"evaluation_agent_id"`
}

func ListAgentRevisions(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string) (json.RawMessage, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return nil, err
	}
	var data json.RawMessage
	err := pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(v) || jsonb_build_object(
 'published',EXISTS(SELECT 1 FROM agent_revision_publications p WHERE p.revision_id=v.id),
 'accepted_selection',EXISTS(SELECT 1 FROM agent_selections sel WHERE sel.candidate_revision_id=v.id AND sel.status='accepted'),
 'evaluations',COALESCE((SELECT jsonb_agg(DISTINCT jsonb_build_object('task_id',t.id,'title',t.title,'status',t.status)) FROM tasks t JOIN task_submissions s ON s.id=t.current_submission_id JOIN agent_runs r ON r.id=s.run_id JOIN agent_revisions er ON er.id=r.agent_revision_id WHERE r.agent_id=v.evaluation_agent_id AND er.config_hash=v.config_hash),'[]'),
 'deliveries',(SELECT jsonb_build_object('submissions',count(*),'accepted',count(*) FILTER(WHERE EXISTS(SELECT 1 FROM task_reviews tr WHERE tr.submission_id=ts.id AND tr.decision='accepted')),'rejected',count(*) FILTER(WHERE EXISTS(SELECT 1 FROM task_reviews tr WHERE tr.submission_id=ts.id AND tr.decision='rejected')),'needs_human',count(*) FILTER(WHERE EXISTS(SELECT 1 FROM task_reviews tr WHERE tr.submission_id=ts.id AND tr.decision='needs_human'))) FROM task_submissions ts JOIN agent_runs sr ON sr.id=ts.run_id JOIN agent_revisions rv ON rv.id=sr.agent_revision_id WHERE (sr.agent_id=v.agent_id OR sr.agent_id=v.evaluation_agent_id) AND rv.config_hash=v.config_hash),
 'measurements',(SELECT jsonb_build_object('runs',count(*),'completed',count(*) FILTER(WHERE r.status='completed'),'mean_seconds',avg(EXTRACT(EPOCH FROM (r.finished_at-r.backend_started_at))) FILTER(WHERE r.finished_at IS NOT NULL),'input_tokens',sum((r.usage_json->>'input_tokens')::bigint),'output_tokens',sum((r.usage_json->>'output_tokens')::bigint)) FROM agent_runs r JOIN agent_revisions rv ON rv.id=r.agent_revision_id WHERE (r.agent_id=v.agent_id OR r.agent_id=v.evaluation_agent_id) AND rv.config_hash=v.config_hash)
 ) ORDER BY v.created_at DESC),'[]') FROM agent_revisions v WHERE v.agent_id=$1`, agentID).Scan(&data)
	return data, err
}

func CreateAgentRevision(ctx context.Context, pool *pgxpool.Pool, agentID, actorID string, req AgentRevisionRequest) (string, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, actorID); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.Summary) == "" || len(req.Summary) > 8000 || len(req.Config.SystemPrompt) > 128000 || strings.TrimSpace(req.Config.ModelName) == "" || len(req.Config.ModelName) > 200 || len(req.Config.CustomArgs) > 50 {
		return "", invalidDelivery("revision summary and valid configuration are required")
	}
	if _, registered := agent.GlobalRegistry().Meta(req.Config.ModelProvider); !registered && req.Config.ModelProvider != "openai" && req.Config.ModelProvider != "anthropic" {
		return "", invalidDelivery("unsupported model provider")
	}
	if req.Config.CustomArgs == nil {
		req.Config.CustomArgs = []string{}
	}
	for _, arg := range req.Config.CustomArgs {
		if len(arg) > 4000 {
			return "", invalidDelivery("argument too long")
		}
	}
	bundles, err := skillloader.NormalizeBundles(req.Config.Skills)
	if err != nil {
		return "", invalidDelivery(err.Error())
	}
	req.Config.Skills = bundles
	if req.EvaluationAgentID != "" {
		if _, err := uuid.Parse(req.EvaluationAgentID); err != nil {
			return "", invalidDelivery("invalid evaluation Agent")
		}
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents original JOIN agents evaluation ON evaluation.owner_id=original.owner_id WHERE original.id=$1 AND evaluation.id=$2 AND evaluation.id<>original.id AND evaluation.kind<>'lucy' AND evaluation.is_active)`, agentID, req.EvaluationAgentID).Scan(&valid); err != nil {
			return "", err
		}
		if !valid {
			return "", ErrAgentWorkForbidden
		}
	}
	data, err := json.Marshal(req.Config)
	if err != nil {
		return "", err
	}
	var id string
	err = pool.QueryRow(ctx, `INSERT INTO agent_revisions(agent_id,config,config_hash,summary,created_by,evaluation_agent_id) VALUES($1,$2::jsonb,encode(sha256(convert_to(($2::jsonb)::text,'UTF8')),'hex'),$3,$4,$5) ON CONFLICT(agent_id,config_hash) DO UPDATE SET evaluation_agent_id=COALESCE(agent_revisions.evaluation_agent_id,EXCLUDED.evaluation_agent_id) WHERE agent_revisions.evaluation_agent_id IS NULL OR EXCLUDED.evaluation_agent_id IS NULL OR agent_revisions.evaluation_agent_id=EXCLUDED.evaluation_agent_id RETURNING id::text`, agentID, data, req.Summary, actorID, nullableUUID(req.EvaluationAgentID)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", invalidDelivery("this configuration already has an evaluation Agent; reuse its evaluation")
	}
	return id, err
}

func PublishAgentRevision(ctx context.Context, pool *pgxpool.Pool, agentID, actorID, revisionID, taskID, reason string) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 8000 {
		return invalidDelivery("publication reason is required")
	}
	if _, err := uuid.Parse(revisionID); err != nil {
		return invalidDelivery("invalid revision")
	}
	if taskID != "" {
		if _, err := uuid.Parse(taskID); err != nil {
			return invalidDelivery("invalid evaluation task")
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner string
	var previous json.RawMessage
	if err = tx.QueryRow(ctx, `SELECT owner_id::text,solo_agent_config(a) FROM agents a WHERE id=$1 AND is_active FOR UPDATE`, agentID).Scan(&owner, &previous); err != nil {
		return err
	}
	if owner != actorID {
		return ErrAgentWorkForbidden
	}
	var config AgentRevisionConfig
	var permitted bool
	err = tx.QueryRow(ctx, `SELECT config,EXISTS(SELECT 1 FROM agent_revision_publications p WHERE p.revision_id=v.id)
 FROM agent_revisions v WHERE v.id=$1 AND v.agent_id=$2`, revisionID, agentID).Scan(&config, &permitted)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalidDelivery("revision not found")
	}
	if err != nil {
		return err
	}
	if !permitted {
		permitted, err = selectionPublicationAllowed(ctx, tx, agentID, revisionID, previous, "")
		if err != nil {
			return err
		}
		if !permitted {
			return invalidDelivery("an accepted paired Selection against the current live baseline is required")
		}
	}
	// Save the previous live configuration as an explicit rollback target.
	var previousID string
	err = tx.QueryRow(ctx, `INSERT INTO agent_revisions(agent_id,config,config_hash,created_by,summary) VALUES($1,$2::jsonb,encode(sha256(convert_to(($2::jsonb)::text,'UTF8')),'hex'),$3,'Previous live configuration') ON CONFLICT(agent_id,config_hash) DO UPDATE SET id=agent_revisions.id RETURNING id::text`, agentID, previous, actorID).Scan(&previousID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_revision_publications(agent_id,revision_id,published_by,reason) SELECT $1,$2,$3,'Preserved before publication' WHERE NOT EXISTS(SELECT 1 FROM agent_revision_publications WHERE revision_id=$2)`, agentID, previousID, actorID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE agents SET system_prompt=$2,model_provider=$3,model_name=$4,custom_args=$5,skills=$6,updated_at=now() WHERE id=$1`, agentID, config.SystemPrompt, config.ModelProvider, config.ModelName, config.CustomArgs, revisionSkillsJSON(config.Skills)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_revision_publications(agent_id,revision_id,evaluation_task_id,published_by,reason) VALUES($1,$2,$3,$4,$5)`, agentID, revisionID, nullableUUID(taskID), actorID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// applyRunRevision uses the snapshot captured when the Run was created, including after restart.
func applyRunRevision(ctx context.Context, db agentRunRowQuerier, req *daemonTaskRequest, runID string) error {
	var cfg AgentRevisionConfig
	var revisionID, previousID, teamID, previousTeamID, previousRelationshipsHash string
	var lockfile json.RawMessage
	err := db.QueryRow(ctx, `SELECT v.config,v.id::text,COALESCE(r.team_version_id::text,''),COALESCE(tv.lockfile,'{}'),COALESCE(prior.agent_revision_id::text,''),COALESCE(prior.team_version_id::text,''),COALESCE((SELECT e.payload->>'relationships_sha256' FROM agent_run_events e WHERE e.run_id=prior.id AND e.type='execution_configuration' ORDER BY e.created_at DESC LIMIT 1),'')
 FROM agent_runs r JOIN agent_revisions v ON v.id=r.agent_revision_id LEFT JOIN channel_team_versions tv ON tv.id=r.team_version_id
 LEFT JOIN LATERAL(SELECT p.id,p.agent_revision_id,p.team_version_id FROM agent_runs p WHERE p.agent_id=r.agent_id AND p.channel_id=r.channel_id AND p.thinking_node_id IS NOT DISTINCT FROM r.thinking_node_id AND p.id<>r.id AND p.started_at<r.started_at AND p.session_id IS NOT NULL ORDER BY p.started_at DESC LIMIT 1) prior ON true
 WHERE r.id=$1`, runID).Scan(&cfg, &revisionID, &teamID, &lockfile, &previousID, &previousTeamID, &previousRelationshipsHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	req.AgentRevisionID, req.TeamVersionID = revisionID, teamID
	req.SystemPrompt = cfg.SystemPrompt
	req.ModelConfig.Provider = cfg.ModelProvider
	req.ModelConfig.Model = cfg.ModelName
	req.CustomArgs = cfg.CustomArgs
	req.Skills = cfg.Skills
	if teamID != "" {
		req.RelationshipsMarkdown = "# Published Team Lockfile\nUse only your own role and the relationships involving you.\n" + string(lockfile)
	}
	if previousID != "" && (previousID != revisionID || previousTeamID != teamID || (previousRelationshipsHash != "" && previousRelationshipsHash != fmt.Sprintf("%x", sha256.Sum256([]byte(req.RelationshipsMarkdown))))) {
		req.ForceFreshSession = true
		req.ResumeSessionID = ""
	}
	return nil
}

func ListTeamVersions(ctx context.Context, pool *pgxpool.Pool, channelID, actorID string) (json.RawMessage, error) {
	if err := NewTaskService(pool).requireChannelMember(ctx, channelID, actorID); err != nil {
		return nil, err
	}
	var data json.RawMessage
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object('current_version_id',c.team_version_id,'versions',COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY v.created_at DESC) FROM channel_team_versions v WHERE v.channel_id=c.id),'[]')) FROM channels c WHERE c.id=$1`, channelID).Scan(&data)
	return data, err
}

func PublishTeamVersion(ctx context.Context, pool *pgxpool.Pool, channelID, actorID, versionID, reason string) (string, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 8000 {
		return "", invalidDelivery("team publication reason is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	id, err := publishTeamVersionTx(ctx, tx, channelID, actorID, versionID, reason)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func publishTeamVersionTx(ctx context.Context, tx pgx.Tx, channelID, actorID, versionID, reason string) (string, error) {
	var owner string
	var err error
	if err = tx.QueryRow(ctx, `SELECT created_by::text FROM channels WHERE id=$1 AND NOT is_archived AND type='channel' FOR UPDATE`, channelID).Scan(&owner); err != nil {
		return "", err
	}
	if owner != actorID && (versionID == "" || versionID == "live") {
		return "", ErrAgentWorkForbidden
	}
	if versionID == "live" {
		if _, err = tx.Exec(ctx, `UPDATE channels SET team_version_id=NULL WHERE id=$1`, channelID); err != nil {
			return "", err
		}
		return "", nil
	}
	var lockfile json.RawMessage
	if versionID != "" {
		if _, err = uuid.Parse(versionID); err != nil {
			return "", invalidDelivery("invalid team version")
		}
		if err = tx.QueryRow(ctx, `SELECT lockfile FROM channel_team_versions WHERE id=$1 AND channel_id=$2`, versionID, channelID).Scan(&lockfile); err != nil {
			return "", invalidDelivery("team version not found")
		}
	} else {
		lockfile, err = captureTeamLockfileTx(ctx, tx, channelID, actorID)
		if err != nil {
			return "", err
		}
		err = tx.QueryRow(ctx, `INSERT INTO channel_team_versions(channel_id,lockfile,created_by,reason) VALUES($1,$2,$3,$4) RETURNING id::text`, channelID, lockfile, actorID, reason).Scan(&versionID)
		if err != nil {
			return "", err
		}
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT NOT ($2::jsonb ? 'based_on_version_id') OR COALESCE(team_version_id::text,'') IN ($2::jsonb->>'based_on_version_id',$3) FROM channels WHERE id=$1`, channelID, lockfile, versionID).Scan(&valid); err != nil {
		return "", err
	}
	if !valid {
		return "", ErrTaskVersionConflict
	}
	if err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM jsonb_array_elements($2::jsonb->'members') member WHERE NOT EXISTS(SELECT 1 FROM channel_members m JOIN agents a ON a.id=m.member_id WHERE m.channel_id=$1 AND m.member_type='agent' AND m.member_id::text=member->>'agent_id' AND a.is_active))`, channelID, lockfile).Scan(&valid); err != nil {
		return "", err
	}
	if !valid {
		return "", invalidDelivery("team member authorization was revoked; publish a new lockfile")
	}

	if err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM jsonb_array_elements($2::jsonb->'relationships') edge WHERE NOT EXISTS(SELECT 1 FROM agent_relationship_scopes r WHERE r.channel_id=$1 AND to_jsonb(r)=edge))`, channelID, lockfile).Scan(&valid); err != nil {
		return "", err
	}
	if !valid {
		return "", invalidDelivery("a cooperation agreement changed or was withdrawn; publish a new lockfile")
	}

	var application struct {
		SelectionID      string  `json:"selection_id"`
		AgentID          string  `json:"changed_agent_id"`
		PreviousRevision string  `json:"previous_revision_id"`
		BasedOn          *string `json:"based_on_version_id"`
	}
	if err = json.Unmarshal(lockfile, &application); err != nil {
		return "", err
	}
	if application.BasedOn != nil && *application.BasedOn == "" {
		if err = tx.QueryRow(ctx, `SELECT COALESCE(c.team_version_id=$4::uuid,false) OR EXISTS(SELECT 1 FROM agents a JOIN agent_revisions v ON v.id=$3 WHERE a.id=$2 AND solo_agent_config(a)=v.config) FROM channels c WHERE c.id=$1`, channelID, application.AgentID, application.PreviousRevision, versionID).Scan(&valid); err != nil {
			return "", err
		}
		if !valid {
			return "", ErrTaskVersionConflict
		}
	}
	if application.SelectionID != "" {
		var previous json.RawMessage
		var candidate string
		if err = tx.QueryRow(ctx, `SELECT v.config,member->>'revision_id' FROM agent_revisions v CROSS JOIN LATERAL jsonb_array_elements($2::jsonb->'members') member WHERE v.id=$1 AND member->>'agent_id'=$3`, application.PreviousRevision, lockfile, application.AgentID).Scan(&previous, &candidate); err != nil {
			return "", err
		}
		valid, err = selectionPublicationAllowed(ctx, tx, application.AgentID, candidate, previous, application.SelectionID)
		if err != nil {
			return "", err
		}
		if !valid {
			return "", invalidDelivery("comparison evidence changed; review the current proposal before applying")
		}
	}

	if owner != actorID {
		var ownsMember bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' WHERE m.channel_id=$1 AND a.owner_id=$2 AND a.is_active)`, channelID, actorID).Scan(&ownsMember); err != nil {
			return "", err
		}
		if !ownsMember {
			return "", ErrAgentWorkForbidden
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE channel_team_versions SET approved_owner_ids=array_append(approved_owner_ids,$2::uuid) WHERE id=$1 AND NOT ($2::uuid=ANY(approved_owner_ids))`, versionID, actorID); err != nil {
		return "", err
	}
	var allApproved bool
	if err = tx.QueryRow(ctx, `SELECT $2::uuid=ANY(v.approved_owner_ids) AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(v.lockfile->'members') member JOIN agents a ON a.id=(member->>'agent_id')::uuid WHERE NOT(a.owner_id=ANY(v.approved_owner_ids))) FROM channel_team_versions v WHERE v.id=$1`, versionID, owner).Scan(&allApproved); err != nil {
		return "", err
	}
	if !allApproved {
		return versionID, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE channels SET team_version_id=$2 WHERE id=$1`, channelID, versionID); err != nil {
		return "", err
	}
	return versionID, nil
}

func captureTeamLockfileTx(ctx context.Context, tx pgx.Tx, channelID, actorID string) (json.RawMessage, error) {
	var lockfile json.RawMessage
	var err error
	// Lock all configurations while the publication snapshot is formed.
	rows, err := tx.Query(ctx, `SELECT a.id::text FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' WHERE m.channel_id=$1 AND a.is_active ORDER BY a.id FOR UPDATE OF a`, channelID)
	if err != nil {
		return nil, err
	}
	if _, err = collectTaskIDs(rows); err != nil {
		return nil, err
	}

	if _, err = tx.Exec(ctx, `INSERT INTO agent_revisions(agent_id,config,config_hash,created_by,summary) SELECT a.id,solo_agent_config(a),encode(sha256(convert_to(solo_agent_config(a)::text,'UTF8')),'hex'),$2,'Team publication snapshot' FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' WHERE m.channel_id=$1 ON CONFLICT(agent_id,config_hash) DO NOTHING`, channelID, actorID); err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `SELECT jsonb_build_object('schema_version',1,'channel_id',$1::uuid::text,'members',COALESCE((SELECT jsonb_agg(jsonb_build_object('agent_id',a.id,'name',a.name,'owner_id',a.owner_id,'revision_id',v.id,'config_hash',v.config_hash) ORDER BY a.id) FROM agents a JOIN channel_members m ON m.member_id=a.id AND m.member_type='agent' JOIN agent_revisions v ON v.agent_id=a.id AND v.config=solo_agent_config(a) WHERE m.channel_id=$1),'[]'),'relationships',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM agent_relationship_scopes r WHERE r.channel_id=$1),'[]'))`, channelID).Scan(&lockfile)
	if err != nil {
		return nil, err
	}
	return lockfile, nil
}

func revisionSkillsJSON(skills []skillloader.Bundle) json.RawMessage {
	if len(skills) == 0 {
		return json.RawMessage(`[]`)
	}
	data, _ := json.Marshal(skills)
	return data
}

// Check again when a remote lease is accepted: an offline Computer may have
// changed versions after its durable Run was queued.
func validateRevisionRuntime(req daemonTaskRequest, capabilities []string) error {
	if (req.TeamVersionID != "" || len(req.Skills) > 0) && !hasCapability(capabilities, agent.RunSnapshotCapability) {
		return fmt.Errorf("Computer requires run_snapshot_v1; upgrade the Daemon")
	}
	if len(req.Skills) > 0 && !hasCapability(capabilities, skillloader.BundleCapability) {
		return fmt.Errorf("Computer requires skill_bundle_v1; upgrade the Daemon")
	}
	return nil
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
	"github.com/solo-ai/solo/pkg/agent"
)

type SelectionCase struct {
	Title        string            `json:"title"`
	Input        string            `json:"input"`
	Requirements []TaskRequirement `json:"requirements"`
}
type AgentSelectionRequest struct {
	ChannelID           string          `json:"channel_id,omitempty"`
	ReviewerAgentID     string          `json:"reviewer_agent_id,omitempty"`
	CandidateRevisionID string          `json:"candidate_revision_id"`
	ReceiverAgentID     string          `json:"receiver_agent_id"`
	Problem             string          `json:"problem"`
	Change              string          `json:"change"`
	Cases               []SelectionCase `json:"cases"`
	TeamCheck           string          `json:"team_check"`
	TokenBudget         int64           `json:"token_budget"`
	IdempotencyKey      string          `json:"idempotency_key"`
}

func CreateAgentSelection(ctx context.Context, pool *pgxpool.Pool, agentID, ownerID string, req AgentSelectionRequest) (string, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, ownerID); err != nil {
		return "", err
	}
	// Self-service proposals never grant the Agent its owner's decision authority.
	if ownerID == agentID {
		if req.ReviewerAgentID == "" || req.ChannelID == "" {
			return "", invalidDelivery("Agent-started comparisons require a target team and independent reviewer")
		}
		if err := pool.QueryRow(ctx, `SELECT owner_id::text FROM agents WHERE id=$1 AND is_active`, agentID).Scan(&ownerID); err != nil {
			return "", err
		}
	}
	if req.ChannelID != "" {
		if _, err := uuid.Parse(req.ChannelID); err != nil {
			return "", invalidDelivery("invalid target team")
		}
	}
	if req.ReviewerAgentID != "" {
		if _, err := uuid.Parse(req.ReviewerAgentID); err != nil || req.ReviewerAgentID == agentID || req.ReviewerAgentID == req.ReceiverAgentID {
			return "", invalidDelivery("choose an independent owned reviewer, separate from the original and receiver")
		}
	}
	if !deliveryID.MatchString(req.IdempotencyKey) || strings.TrimSpace(req.Problem) == "" || strings.TrimSpace(req.Change) == "" || strings.TrimSpace(req.TeamCheck) == "" || len(req.Problem) > 8000 || len(req.Change) > 8000 || len(req.TeamCheck) > 4000 || len(req.Cases) < 1 || len(req.Cases) > 10 || req.TokenBudget < 1000 || req.TokenBudget > 10000000 {
		return "", invalidDelivery("describe problem, change, 1–10 cases, receiver check and a 1,000–10,000,000 Token budget")
	}
	if _, err := uuid.Parse(req.CandidateRevisionID); err != nil {
		return "", invalidDelivery("invalid candidate revision")
	}
	if _, err := uuid.Parse(req.ReceiverAgentID); err != nil || req.ReceiverAgentID == agentID {
		return "", invalidDelivery("choose a different owned receiver Agent")
	}
	for _, c := range req.Cases {
		if strings.TrimSpace(c.Title) == "" || len(c.Title) > 200 || strings.TrimSpace(c.Input) == "" || len(c.Input) > 16000 {
			return "", invalidDelivery("each case needs a title and exact input")
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "selection-create:"+agentID); err != nil {
		return "", err
	}
	var id, hash, previousWorkspace string
	scopeID := serverworkspace.FilterID(ctx)
	err = tx.QueryRow(ctx, `SELECT s.id::text,s.request_hash,COALESCE((SELECT c.workspace_id::text FROM agent_selection_trials tr JOIN channels c ON c.id=tr.channel_id WHERE tr.selection_id=s.id LIMIT 1),'') FROM agent_selections s WHERE s.agent_id=$1 AND s.idempotency_key=$2`, agentID, req.IdempotencyKey).Scan(&id, &hash, &previousWorkspace)
	if err == nil {
		if hash != deliveryRequestHash(req) || (scopeID != "" && scopeID != previousWorkspace) {
			return "", ErrTaskVersionConflict
		}
		return id, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var workspaceID, computerID, homeID string
	err = tx.QueryRow(ctx, `SELECT c.workspace_id::text,a.runtime_id,c.id::text FROM agents a JOIN channels c ON c.id=a.home_channel_id AND NOT c.is_archived JOIN channel_members m ON m.channel_id=c.id AND m.member_id=$2 AND m.member_type='user' WHERE a.id=$1 AND a.owner_id=$2 AND a.is_active FOR SHARE OF a,c`, agentID, ownerID).Scan(&workspaceID, &computerID, &homeID)
	if err != nil {
		return "", ErrAgentWorkForbidden
	}
	var valid bool
	if scopeID != "" {
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id AND w.deleted_at IS NULL WHERE wm.workspace_id=$1 AND wm.user_id=$2) AND EXISTS(SELECT 1 FROM channels c WHERE c.workspace_id=$1 AND NOT c.is_archived AND (c.id=$4 OR EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=c.id AND m.member_type='agent' AND m.member_id=$3)))`, scopeID, ownerID, agentID, homeID).Scan(&valid)
		if err != nil {
			return "", err
		}
		if !valid {
			return "", ErrAgentWorkForbidden
		}
		workspaceID = scopeID
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM computers c LEFT JOIN computer_members m ON m.computer_id=c.id AND m.user_id=$2 WHERE c.id::text=$1 AND (c.owner_id=$2 OR m.user_id IS NOT NULL) AND ((c.credential_hash IS NOT NULL AND c.credential_revoked_at IS NULL) OR (c.daemon_id IS NOT NULL AND c.status='online'))) AND EXISTS(SELECT 1 FROM agents a WHERE a.id=$3 AND a.owner_id=$2 AND a.kind<>'lucy' AND a.is_active)`, computerID, ownerID, req.ReceiverAgentID).Scan(&valid)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", invalidDelivery("Computer or receiver authorization is unavailable")
	}
	baselineID, baseline, err := captureSelectionRevision(ctx, tx, agentID, ownerID, req.ChannelID)
	if err != nil {
		return "", err
	}
	receiverID, receiver, err := captureSelectionRevision(ctx, tx, req.ReceiverAgentID, ownerID, req.ChannelID)
	if err != nil {
		return "", err
	}
	var reviewerRevisionID, reviewerCloneID string
	var reviewer AgentRevisionConfig
	if req.ReviewerAgentID != "" {
		reviewerRevisionID, reviewer, err = captureSelectionRevision(ctx, tx, req.ReviewerAgentID, ownerID, "")
		if err != nil {
			return "", ErrAgentWorkForbidden
		}
		reviewerCloneID = uuid.NewString()
	}
	var candidate AgentRevisionConfig
	err = tx.QueryRow(ctx, `SELECT config FROM agent_revisions WHERE id=$1 AND agent_id=$2`, req.CandidateRevisionID, agentID).Scan(&candidate)
	if err != nil {
		return "", invalidDelivery("candidate revision is unavailable")
	}
	if baselineID == req.CandidateRevisionID || baseline.ModelProvider != candidate.ModelProvider || baseline.ModelName != candidate.ModelName || deliveryRequestHash(baseline.CustomArgs) != deliveryRequestHash(candidate.CustomArgs) {
		return "", invalidDelivery("compare distinct revisions under identical provider, model and arguments")
	}
	id = uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO agent_selections(id,agent_id,baseline_revision_id,candidate_revision_id,receiver_revision_id,created_by,plan,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb || jsonb_build_object('computer_id',$10::text),$8,$9)`, id, agentID, baselineID, req.CandidateRevisionID, receiverID, ownerID, req, req.IdempotencyKey, deliveryRequestHash(req), computerID)
	if err != nil {
		return "", err
	}
	arms := []struct {
		name, revision string
		config         AgentRevisionConfig
	}{{"reviewer", reviewerRevisionID, reviewer}, {"baseline", baselineID, baseline}, {"candidate", req.CandidateRevisionID, candidate}, {"receiver", receiverID, receiver}}
	if reviewerCloneID == "" {
		arms = arms[1:]
	}
	for _, arm := range arms {
		cloneID, channelID := uuid.NewString(), uuid.NewString()
		if arm.name == "reviewer" {
			cloneID = reviewerCloneID
		}
		name := "selection-" + id[:8] + "-" + arm.name
		if _, err = tx.Exec(ctx, `INSERT INTO channels(id,workspace_id,name,description,type,created_by) VALUES($1,$2,$3,$4,'channel',$5)`, channelID, workspaceID, name, req.Problem, ownerID); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO channel_members(channel_id,member_type,member_id,role) SELECT $1,'user',u.id,CASE WHEN u.id=$3 THEN 'owner' ELSE 'member' END FROM users u WHERE u.id=$3 OR (u.is_active AND EXISTS(SELECT 1 FROM workspace_members m WHERE m.workspace_id=$2 AND m.user_id=u.id))`, channelID, workspaceID, ownerID); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO agents(id,name,owner_id,home_channel_id,kind,model_provider,model_name,system_prompt,custom_args,skills,runtime_id,avatar_url) VALUES($1,$2,$3,$4,'agent',$5,$6,$7,$8,$9,$10,$11)`, cloneID, name, ownerID, channelID, arm.config.ModelProvider, arm.config.ModelName, arm.config.SystemPrompt, arm.config.CustomArgs, revisionSkillsJSON(arm.config.Skills), computerID, "dicebear:pixel-art:agent-"+cloneID); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO channel_members(channel_id,member_type,member_id,role) VALUES($1,'agent',$2,'member')`, channelID, cloneID); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO agent_selection_trials(selection_id,arm,agent_id,channel_id,revision_id,token_budget) VALUES($1,$2,$3,$4,$5,$6)`, id, arm.name, cloneID, channelID, arm.revision, req.TokenBudget); err != nil {
			return "", err
		}
		if arm.name == "reviewer" {
			continue
		}
		if reviewerCloneID != "" {
			if _, err = tx.Exec(ctx, `INSERT INTO channel_members(channel_id,member_type,member_id,role) VALUES($1,'agent',$2,'member')`, channelID, reviewerCloneID); err != nil {
				return "", err
			}
		}
		for i, c := range req.Cases {
			contract := TaskContract{Requirements: c.Requirements, Gate: TaskGate{Kind: "human", ReviewerID: ownerID, MaxRevisions: 3}}
			if reviewerCloneID != "" {
				contract.Gate = TaskGate{Kind: "agent", ReviewerID: reviewerCloneID, MaxRevisions: 3}
			}
			input := c.Input
			if arm.name == "receiver" {
				contract.Requirements = []TaskRequirement{{ID: "compatibility", Text: req.TeamCheck}}
				input = "Consume the candidate's exact submission supplied at execution. " + req.TeamCheck
			}
			if err = validateTaskContract(ctx, tx, channelID, ownerID, &contract); err != nil {
				return "", err
			}
			taskID := uuid.NewString()
			messageID := uuid.NewString()
			if _, err = tx.Exec(ctx, `INSERT INTO messages(id,channel_id,sender_type,sender_id,content,content_type) VALUES($1,$2,'system','00000000-0000-0000-0000-000000000000',$3,'system')`, messageID, channelID, fmt.Sprintf("Task #%d: %s", i+1, c.Title)); err != nil {
				return "", err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO threads(channel_id,root_message_id) VALUES($1,$2)`, channelID, messageID); err != nil {
				return "", err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO tasks(id,task_number,channel_id,creator_id,title,description,status,claimer_id,priority,contract,message_id) VALUES($1,$2,$3,$4,$5,$6,'in_progress',$7,'none',$8,$9)`, taskID, i+1, channelID, ownerID, c.Title, input, cloneID, contract, messageID); err != nil {
				return "", err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO agent_selection_tasks(selection_id,arm,case_index,task_id) VALUES($1,$2,$3,$4)`, id, arm.name, i, taskID); err != nil {
				return "", err
			}
		}
	}
	return id, tx.Commit(ctx)
}

func captureSelectionRevision(ctx context.Context, tx pgx.Tx, agentID, ownerID, channelID string) (string, AgentRevisionConfig, error) {
	var id string
	var config AgentRevisionConfig
	err := tx.QueryRow(ctx, `INSERT INTO agent_revisions(agent_id,config,config_hash,created_by,summary) SELECT id,solo_agent_config(a),encode(sha256(convert_to(solo_agent_config(a)::text,'UTF8')),'hex'),$2,'Selection frozen configuration' FROM agents a WHERE id=$1 AND owner_id=$2 AND is_active ON CONFLICT(agent_id,config_hash) DO UPDATE SET id=agent_revisions.id RETURNING id::text,config`, agentID, ownerID).Scan(&id, &config)
	if err != nil || channelID == "" {
		return id, config, err
	}
	var teamVersion string
	err = tx.QueryRow(ctx, `SELECT COALESCE(c.team_version_id::text,'') FROM channels c JOIN channel_members m ON m.channel_id=c.id AND m.member_type='agent' AND m.member_id=$2 JOIN channel_members owner ON owner.channel_id=c.id AND owner.member_type='user' AND owner.member_id=$3 WHERE c.id=$1 AND c.type='channel' AND NOT c.is_archived AND ($4='' OR c.workspace_id::text=$4)`, channelID, agentID, ownerID, serverworkspace.FilterID(ctx)).Scan(&teamVersion)
	if err != nil {
		return "", config, ErrAgentWorkForbidden
	}
	if teamVersion != "" {
		err = tx.QueryRow(ctx, `SELECT v.id::text,v.config FROM channel_team_versions tv CROSS JOIN LATERAL jsonb_array_elements(tv.lockfile->'members') member JOIN agent_revisions v ON v.id=(member->>'revision_id')::uuid AND v.agent_id=$2::uuid WHERE tv.id=$1 AND member->>'agent_id'=$2::text`, teamVersion, agentID).Scan(&id, &config)
		if err != nil {
			return "", config, invalidDelivery("Agent is not in the target team's approved version")
		}
	}
	return id, config, nil
}

// selectionResults is also the immutable decision receipt. No wall-clock or UI-derived estimates.
func selectionResults(ctx context.Context, db agentRunRowQuerier, id string) (json.RawMessage, error) {
	var data json.RawMessage
	err := db.QueryRow(ctx, `SELECT jsonb_build_object(
 'tasks',COALESCE((SELECT jsonb_agg(jsonb_build_object('arm',st.arm,'case_index',st.case_index,'task_id',t.id,'channel_id',t.channel_id,'title',t.title,'status',t.status,'version',t.version,'submission_id',t.current_submission_id,'input_submission_id',st.input_submission_id,'attempts',st.attempts,'last_error',st.last_error,
 'verified',EXISTS(SELECT 1 FROM task_submissions sub JOIN task_reviews review ON review.submission_id=sub.id AND review.decision='accepted' JOIN agent_runs r ON r.id=sub.run_id JOIN agent_revisions v ON v.id=r.agent_revision_id JOIN agent_selection_trials tr ON tr.selection_id=st.selection_id AND tr.arm=st.arm JOIN agent_revisions expected ON expected.id=tr.revision_id WHERE sub.id=t.current_submission_id AND r.agent_id=tr.agent_id AND v.config_hash=expected.config_hash AND r.finished_at IS NOT NULL AND t.status='done'
 AND (NOT EXISTS(SELECT 1 FROM agent_selection_trials rt WHERE rt.selection_id=st.selection_id AND rt.arm='reviewer') OR EXISTS(
 SELECT 1 FROM agent_selection_trials rt JOIN agent_runs rr ON rr.agent_id=rt.agent_id JOIN agent_run_task_links link ON link.run_id=rr.id AND link.task_id=t.id
 WHERE rt.selection_id=st.selection_id AND rt.arm='reviewer' AND review.reviewer_id=rt.agent_id AND rr.status='completed' AND review.created_at BETWEEN rr.started_at AND rr.finished_at))),
 'consumed_current',st.arm<>'receiver' OR st.input_submission_id=(SELECT c.current_submission_id FROM tasks c JOIN agent_selection_tasks ct ON ct.task_id=c.id WHERE ct.selection_id=st.selection_id AND ct.case_index=st.case_index AND ct.arm='candidate')) ORDER BY st.arm,st.case_index) FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=$1),'[]'),
 'trials',COALESCE((SELECT jsonb_agg(jsonb_build_object('arm',tr.arm,'agent_id',tr.agent_id,'channel_id',tr.channel_id,'token_budget',tr.token_budget,'config_valid',a.is_active AND solo_agent_config(a)=v.config AND COALESCE(a.custom_env,'{}'::jsonb)='{}'::jsonb AND a.runtime_id=(SELECT plan->>'computer_id' FROM agent_selections WHERE id=tr.selection_id),
 'runs',(SELECT count(*) FROM agent_runs r WHERE r.agent_id=tr.agent_id),
 'active_runs',(SELECT count(*) FROM agent_runs r WHERE r.agent_id=tr.agent_id AND r.finished_at IS NULL),
 'unknown_runs',(SELECT count(*) FROM agent_run_token_usage u WHERE u.agent_id=tr.agent_id AND u.state='usage_unknown'),
 'actual_tokens',(SELECT COALESCE(sum(actual_tokens),0) FROM agent_run_token_usage u WHERE u.agent_id=tr.agent_id),
 'accounted_tokens',(SELECT COALESCE(sum(COALESCE(actual_tokens,reserved_tokens)),0) FROM agent_run_token_usage u WHERE u.agent_id=tr.agent_id),
 'execution_seconds',(SELECT COALESCE(sum(EXTRACT(EPOCH FROM (r.finished_at-r.backend_started_at))),0) FROM agent_runs r WHERE r.agent_id=tr.agent_id)) ORDER BY tr.arm) FROM agent_selection_trials tr JOIN agents a ON a.id=tr.agent_id JOIN agent_revisions v ON v.id=tr.revision_id WHERE tr.selection_id=$1),'[]'))`, id).Scan(&data)
	return data, err
}

func ListAgentSelections(ctx context.Context, pool *pgxpool.Pool, agentID, ownerID string) (json.RawMessage, error) {
	if err := requireAgentWorkOwner(ctx, pool, agentID, ownerID); err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `SELECT to_jsonb(s) || jsonb_build_object('decisions',COALESCE((SELECT jsonb_agg(to_jsonb(d) ORDER BY d.created_at DESC,d.id) FROM agent_selection_decisions d WHERE d.selection_id=s.id),'[]')) FROM agent_selections s WHERE s.agent_id=$1 AND ($2='' OR EXISTS(SELECT 1 FROM agent_selection_trials tr JOIN channels c ON c.id=tr.channel_id WHERE tr.selection_id=s.id AND c.workspace_id::text=$2)) ORDER BY s.created_at DESC`, agentID, serverworkspace.FilterID(ctx))
	if err != nil {
		return nil, err
	}
	var items []map[string]json.RawMessage
	for rows.Next() {
		var item map[string]json.RawMessage
		if err = rows.Scan(&item); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []map[string]json.RawMessage{}
	}
	for _, item := range items {
		var id string
		_ = json.Unmarshal(item["id"], &id)
		item["results"], err = selectionResults(ctx, pool, id)
		if err != nil {
			return nil, err
		}
		ready := validateSelectionAcceptance(item["results"]) == nil
		item["ready_to_apply"], _ = json.Marshal(ready)
	}
	return json.Marshal(items)
}

func DecideAgentSelection(ctx context.Context, pool *pgxpool.Pool, agentID, ownerID, id, decision, reason string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = decideAgentSelectionTx(ctx, tx, agentID, ownerID, id, decision, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func decideAgentSelectionTx(ctx context.Context, tx pgx.Tx, agentID, ownerID, id, decision, reason string) error {
	if decision != "accepted" && decision != "rejected" && decision != "observe" {
		return invalidDelivery("choose accepted, rejected or observe")
	}
	if _, err := uuid.Parse(id); err != nil || strings.TrimSpace(reason) == "" || len(reason) > 8000 {
		return invalidDelivery("Selection and decision reason are required")
	}
	var found string
	err := tx.QueryRow(ctx, `SELECT s.id::text FROM agent_selections s JOIN agents a ON a.id=s.agent_id WHERE s.id=$1 AND s.agent_id=$2 AND a.owner_id=$3 FOR UPDATE OF s`, id, agentID, ownerID).Scan(&found)
	if err != nil {
		return ErrAgentWorkForbidden
	}
	data, err := selectionResults(ctx, tx, id)
	if err != nil {
		return err
	}
	if decision == "accepted" {
		if err = validateSelectionAcceptance(data); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_selection_decisions(selection_id,decision,reason,evidence,created_by) VALUES($1,$2,$3,$4,$5)`, id, decision, strings.TrimSpace(reason), data, ownerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_selections SET status=$2 WHERE id=$1`, id, decision); err != nil {
		return err
	}
	return nil
}

func validateSelectionAcceptance(data json.RawMessage) error {
	var result struct {
		Tasks []struct {
			Arm      string `json:"arm"`
			Status   string `json:"status"`
			Verified bool   `json:"verified"`
			Consumed bool   `json:"consumed_current"`
			Attempts int    `json:"attempts"`
		} `json:"tasks"`
		Trials []struct {
			Arm     string `json:"arm"`
			Valid   bool   `json:"config_valid"`
			Runs    int    `json:"runs"`
			Active  int    `json:"active_runs"`
			Unknown int    `json:"unknown_runs"`
			Used    int64  `json:"accounted_tokens"`
			Limit   int64  `json:"token_budget"`
		} `json:"trials"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	trialCount := 3
	for _, tr := range result.Trials {
		if tr.Arm == "reviewer" {
			trialCount = 4
		}
	}
	if len(result.Trials) != trialCount || len(result.Tasks) < 3 {
		return invalidDelivery("incomplete comparison")
	}
	for _, tr := range result.Trials {
		if !tr.Valid || tr.Runs == 0 || tr.Active != 0 || tr.Unknown != 0 || tr.Used > tr.Limit {
			return invalidDelivery("all trials require unchanged configuration, finished Runs and known usage within the agreed budget")
		}
	}
	for _, task := range result.Tasks {
		if task.Attempts == 0 || !TerminalStatuses[task.Status] {
			return invalidDelivery("every baseline and candidate case must finish before selection")
		}
		if task.Arm != "baseline" && (!task.Verified || !task.Consumed) {
			return invalidDelivery("candidate and receiver checks must accept the exact current output")
		}
	}
	return nil
}

func selectionPublicationAllowed(ctx context.Context, tx pgx.Tx, agentID, revisionID string, live json.RawMessage, selectionID string) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT s.id::text,d.evidence FROM agent_selections s JOIN agent_revisions baseline ON baseline.id=s.baseline_revision_id JOIN LATERAL(SELECT evidence,decision FROM agent_selection_decisions WHERE selection_id=s.id ORDER BY created_at DESC,id DESC LIMIT 1) d ON d.decision='accepted' WHERE s.agent_id=$1 AND s.candidate_revision_id=$2 AND s.status='accepted' AND baseline.config=$3::jsonb AND ($4='' OR s.id::text=$4) FOR SHARE OF s`, agentID, revisionID, live, selectionID)
	if err != nil {
		return false, err
	}
	type receipt struct {
		id       string
		evidence json.RawMessage
	}
	var receipts []receipt
	for rows.Next() {
		var r receipt
		if err = rows.Scan(&r.id, &r.evidence); err != nil {
			rows.Close()
			return false, err
		}
		receipts = append(receipts, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, r := range receipts {
		current, err := selectionResults(ctx, tx, r.id)
		if err != nil {
			return false, err
		}
		if validateSelectionAcceptance(current) != nil {
			continue
		}
		var same bool
		if err = tx.QueryRow(ctx, `SELECT $1::jsonb=$2::jsonb`, current, r.evidence).Scan(&same); err != nil {
			return false, err
		}
		if same {
			return true, nil
		}
	}
	return false, nil
}

func validateSelectionRunTx(ctx context.Context, tx pgx.Tx, input StartRunInput) error {
	var id, channel, status, arm string
	var valid bool
	err := tx.QueryRow(ctx, `SELECT s.id::text,tr.channel_id::text,s.status,tr.arm,solo_agent_config(a)=v.config AND COALESCE(a.custom_env,'{}'::jsonb)='{}'::jsonb AND a.runtime_id=s.plan->>'computer_id' AND NOT EXISTS(SELECT 1 FROM channels c WHERE c.id=tr.channel_id AND c.team_version_id IS NOT NULL) FROM agent_selection_trials tr JOIN agent_selections s ON s.id=tr.selection_id JOIN agents a ON a.id=tr.agent_id JOIN agent_revisions v ON v.id=tr.revision_id WHERE tr.agent_id=$1 FOR SHARE OF s`, input.AgentID).Scan(&id, &channel, &status, &arm, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if arm == "reviewer" {
		if !valid || (status != "running" && status != "observe") || input.ReviewSubmissionID == "" {
			return invalidDelivery("Selection reviewer executes only its current queued reviews")
		}
		var reviewValid bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_review_deliveries d JOIN task_submissions sub ON sub.id=d.submission_id JOIN tasks t ON t.id=sub.task_id JOIN agent_selection_tasks st ON st.task_id=t.id WHERE st.selection_id=$1 AND d.submission_id=$2 AND d.reviewer_id=$3 AND t.channel_id=$4 AND t.status='in_review' AND t.current_submission_id=sub.id AND t.version=sub.task_version+1)`, id, input.ReviewSubmissionID, input.AgentID, input.ChannelID).Scan(&reviewValid)
		if err != nil {
			return err
		}
		if !reviewValid {
			return invalidDelivery("invalid Selection review")
		}
		return nil
	}
	if input.SelectionTaskID == "" || channel != input.ChannelID || !valid || (status != "running" && status != "observe") {
		return invalidDelivery("Selection members execute only their fixed evaluation tasks while the comparison is open")
	}
	var taskValid bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=$1 AND t.id=$2 AND t.channel_id=$3 AND t.claimer_id=$4 AND t.status='in_progress')`, id, input.SelectionTaskID, channel, input.AgentID).Scan(&taskValid)
	if err != nil {
		return err
	}
	if !taskValid {
		return invalidDelivery("invalid Selection task")
	}
	return nil
}

func (s *AgentService) dispatchSelectionTask(ctx context.Context) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// A receiver cannot consume a candidate that permanently failed this case.
	if _, err = tx.Exec(ctx, `WITH blocked AS (SELECT st.task_id FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id JOIN agent_selections sel ON sel.id=st.selection_id JOIN agent_selection_tasks ct ON ct.selection_id=st.selection_id AND ct.arm='candidate' AND ct.case_index=st.case_index JOIN tasks candidate ON candidate.id=ct.task_id WHERE st.arm='receiver' AND sel.status IN ('running','observe') AND t.status='in_progress' AND candidate.status IN ('closed','cancelled') AND NOT EXISTS(SELECT 1 FROM agent_runs r JOIN agent_selection_trials tr ON tr.agent_id=r.agent_id WHERE tr.selection_id=st.selection_id AND tr.arm='receiver' AND r.finished_at IS NULL) FOR UPDATE OF t SKIP LOCKED), recorded AS (UPDATE agent_selection_tasks SET last_error='Candidate failed; no accepted output is available to consume' WHERE task_id IN (SELECT task_id FROM blocked)) UPDATE tasks SET status='closed',updated_at=now() WHERE id IN (SELECT task_id FROM blocked)`); err != nil {
		return false, err
	}
	// Exhausted runs retain their evidence and costs, then release the next fixed sample.
	if _, err = tx.Exec(ctx, `WITH exhausted AS (SELECT st.task_id FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id JOIN agent_runs r ON r.id=st.run_id JOIN agent_selections sel ON sel.id=st.selection_id WHERE st.attempts>=3 AND t.status='in_progress' AND r.finished_at IS NOT NULL AND sel.status IN ('running','observe') AND st.next_attempt_at<=now() FOR UPDATE OF t SKIP LOCKED), recorded AS (UPDATE agent_selection_tasks SET last_error='Execution attempts exhausted without an accepted delivery' WHERE task_id IN (SELECT task_id FROM exhausted)) UPDATE tasks SET status='closed',updated_at=now() WHERE id IN (SELECT task_id FROM exhausted)`); err != nil {
		return false, err
	}
	var selectionID, taskID, agentID, channelID, arm, title, input string
	var number, caseIndex int
	var version int64
	err = tx.QueryRow(ctx, `SELECT st.selection_id::text,t.id::text,tr.agent_id::text,t.channel_id::text,st.arm,t.title,COALESCE(t.description,''),t.task_number,st.case_index,t.version FROM agent_selection_tasks st JOIN agent_selections sel ON sel.id=st.selection_id JOIN agent_selection_trials tr ON tr.selection_id=st.selection_id AND tr.arm=st.arm JOIN tasks t ON t.id=st.task_id JOIN agents a ON a.id=tr.agent_id AND a.is_active JOIN channels c ON c.id=t.channel_id AND NOT c.is_archived LEFT JOIN agent_runs r ON r.id=st.run_id
 WHERE sel.status IN ('running','observe') AND t.status='in_progress' AND t.claimer_id=tr.agent_id AND st.attempts<3 AND st.next_attempt_at<=now() AND (r.id IS NULL OR r.finished_at IS NOT NULL)
 AND (st.dispatched_version<t.version OR r.finished_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM agent_runs busy WHERE busy.agent_id=tr.agent_id AND busy.finished_at IS NULL)
 AND EXISTS(SELECT 1 FROM agent_inbox_heads h WHERE h.agent_id=tr.agent_id AND h.kind='selection' AND h.work_id=t.id::text)
 AND NOT EXISTS(SELECT 1 FROM agent_selection_tasks prev JOIN tasks pt ON pt.id=prev.task_id WHERE prev.selection_id=st.selection_id AND prev.arm=st.arm AND prev.case_index<st.case_index AND pt.status NOT IN ('done','closed','cancelled'))
 AND (st.arm<>'receiver' OR EXISTS(SELECT 1 FROM agent_selection_tasks ct JOIN tasks candidate ON candidate.id=ct.task_id WHERE ct.selection_id=st.selection_id AND ct.arm='candidate' AND ct.case_index=st.case_index AND candidate.status='done'))
 ORDER BY st.next_attempt_at,st.case_index,st.arm FOR UPDATE OF st,t SKIP LOCKED LIMIT 1`).Scan(&selectionID, &taskID, &agentID, &channelID, &arm, &title, &input, &number, &caseIndex, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(ctx); err != nil {
			return false, err
		}
		return s.notifySelectionOutcome(ctx)
	}
	if err != nil {
		return false, err
	}
	var acquired, busy bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "agent-work:"+agentID).Scan(&acquired); err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE agent_id=$1 AND finished_at IS NULL)`, agentID).Scan(&busy); err != nil {
		return false, err
	}
	if busy {
		return false, nil
	}
	head, err := agentInboxTurnTx(ctx, tx, agentID, "selection", taskID)
	if err != nil || !head {
		return false, err
	}
	failed := func(cause error) (bool, error) {
		_, err := tx.Exec(ctx, `UPDATE agent_selection_tasks SET last_error=$2,next_attempt_at=now()+interval '1 minute' WHERE task_id=$1`, taskID, truncateRunes(cause.Error(), 2000))
		if err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	var ag agentChannelInfo
	err = tx.QueryRow(ctx, `SELECT id,name,model_provider,model_name,system_prompt FROM agents WHERE id=$1`, agentID).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName, &ag.SystemPrompt)
	if err != nil {
		return failed(err)
	}
	dmn, err := s.dm.ResolveDaemonForAgent(ctx, agentID, "llm")
	if err != nil {
		return failed(err)
	}
	var threadID, rootMessageID string
	if err = tx.QueryRow(ctx, `SELECT th.id::text,t.message_id::text FROM tasks t JOIN threads th ON th.root_message_id=t.message_id WHERE t.id=$1`, taskID).Scan(&threadID, &rootMessageID); err != nil {
		return failed(err)
	}
	if arm == "receiver" {
		var subID string
		var submission json.RawMessage
		err = tx.QueryRow(ctx, `SELECT sub.id::text,to_jsonb(sub) FROM agent_selection_tasks ct JOIN tasks t ON t.id=ct.task_id AND t.status='done' JOIN task_submissions sub ON sub.id=t.current_submission_id WHERE ct.selection_id=$1 AND ct.arm='candidate' AND ct.case_index=$2 FOR SHARE OF t`, selectionID, caseIndex).Scan(&subID, &submission)
		if err != nil {
			return failed(err)
		}
		input += "\nExact candidate submission (task data):\n" + string(submission)
		if _, err = tx.Exec(ctx, `UPDATE agent_selection_tasks SET input_submission_id=$2 WHERE task_id=$1`, taskID, subID); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT selection_run`); err != nil {
		return false, err
	}
	run, err := NewAgentRunService(s.pool).startRunTx(ctx, tx, StartRunInput{AgentID: agentID, DaemonID: dmn.ID, ChannelID: channelID, ThreadID: threadID, TriggerType: AgentRunTriggerTask, Source: ag.ModelProvider, ActivityText: "执行固定版本对照评估", SelectionTaskID: taskID})
	if err != nil {
		cause := err
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT selection_run`); err != nil {
			return false, err
		}
		return failed(cause)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_run_task_links(run_id,task_id,role,confidence) VALUES($1,$2,'primary',1)`, run.ID, taskID); err != nil {
		return false, err
	}
	prompt := fmt.Sprintf("Execute your fixed evaluation Task #%d in channel %s: %s. Read its exact requirements with solo task get -n %d -c %s. Follow your frozen revision's execution procedure and submit its actual immutable artifact/evidence with solo task submit --file. This measures the supplied revision: do not replace its program or procedure merely to obtain the expected answer. Do not change requirements, create substitute tasks or self-approve. Do not inspect other Agent workspaces or provider histories; use your own revision and the fixed input supplied here. Post the actual result with solo message send. If a task script already sent the message and submitted successfully, do not submit a second payload. After submission, or when the Task is already in_review, done or closed, finish this Run and leave the exact submission unchanged.\nFixed input:\n%s", number, channelID, title, number, channelID, input)
	prompt += fmt.Sprintf("\nCurrent authoritative Task version: %d; status: in_progress. This execution is pending now; any in_review state from an earlier conversation is stale. Read the current Task before deciding to stop. If it was rejected, execute the frozen procedure again and submit against this current version. Post the visible result to the Task thread with solo message send --target %s:%s, not the parent channel.", version, channelID, rootMessageID[:8])
	req := daemonTaskRequest{AgentID: agentID, ChannelID: channelID, ThreadID: threadID, Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}}, SystemPrompt: ag.SystemPrompt, ModelConfig: agent.ModelConfig{Provider: ag.ModelProvider, Model: ag.ModelName}, OriginTaskID: taskID, TaskLinkRole: AgentRunTaskRolePrimary, ResultContract: agentResultContractVisibleMessage, PrestartedRun: true, RunID: run.ID}
	if _, err = tx.Exec(ctx, `UPDATE agent_selection_tasks SET run_id=$2,dispatched_version=$3,attempts=attempts+1,last_error='',next_attempt_at=now()+interval '1 minute' WHERE task_id=$1`, taskID, run.ID, version); err != nil {
		return false, err
	}
	if err = s.persistRemoteMessageRunTx(ctx, tx, dmn, run, req, ag); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	req.RemotePrequeued = dmn.ComputerID != ""
	go s.runStreamingAgentTask(context.Background(), dmn, req, ag, run)
	return true, nil
}

// Reuse the durable Agent inbox to report a comparison once per meaningful
// outcome. No extra scheduler, polling Run or notification table is needed.
func (s *AgentService) notifySelectionOutcome(ctx context.Context) (bool, error) {
	var id, agentID, channelID, outcome string
	err := s.pool.QueryRow(ctx, `WITH outcomes AS (
 SELECT sel.id,sel.agent_id,c.id AS channel_id,
 CASE WHEN NOT EXISTS(SELECT 1 FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=sel.id AND t.status NOT IN ('done','closed','cancelled')) THEN 'completed' ELSE 'needs_input' END AS outcome
 FROM agent_selections sel JOIN agents a ON a.id=sel.agent_id AND a.is_active JOIN users u ON u.id=a.owner_id AND u.is_active JOIN channels c ON c.id=NULLIF(sel.plan->>'channel_id','')::uuid AND NOT c.is_archived
 WHERE sel.status IN ('running','observe')
 AND EXISTS(SELECT 1 FROM channel_members m WHERE m.channel_id=c.id AND m.member_type='agent' AND m.member_id=a.id)
 AND NOT EXISTS(SELECT 1 FROM agent_selection_trials tr JOIN agent_runs r ON r.agent_id=tr.agent_id WHERE tr.selection_id=sel.id AND r.finished_at IS NULL)
 AND (NOT EXISTS(SELECT 1 FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id WHERE st.selection_id=sel.id AND t.status NOT IN ('done','closed','cancelled')) OR EXISTS(SELECT 1 FROM agent_selection_tasks st JOIN tasks t ON t.id=st.task_id JOIN task_review_deliveries d ON d.submission_id=t.current_submission_id WHERE st.selection_id=sel.id AND t.status='in_review' AND d.attempts>=3)))
 SELECT id::text,agent_id::text,channel_id::text,outcome FROM outcomes o WHERE NOT EXISTS(SELECT 1 FROM agent_pending_work p WHERE p.agent_id=o.agent_id AND p.channel_id=o.channel_id AND p.payload#>>'{messages,0,content}' LIKE 'Selection '||o.id::text||' '||o.outcome||'.%') ORDER BY id LIMIT 1`).Scan(&id, &agentID, &channelID, &outcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	prompt := fmt.Sprintf("Selection %s %s. Read solo work selections once and report the actual outcome of this exact comparison in the original channel. Explain the concrete improvement or failure, actual costs, remaining uncertainty and this team's scope. If ready, point the owner to your existing member sidebar's improvement proposal; adoption belongs to the owner. If independent review needs input, name the affected task and precise missing evidence or decision. Do not claim success without verified results, approve your own revision, poll, create a replacement Task, or start another comparison. This is a brief outcome report, not a new execution task.", id, outcome)
	err = s.enqueueAgentWork(ctx, daemonTaskRequest{AgentID: agentID, ChannelID: channelID, Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}}, ResultContract: agentResultContractVisibleMessage})
	return err == nil, err
}

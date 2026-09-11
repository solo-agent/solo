package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/pkg/agent"
)

var (
	ErrTaskVersionConflict  = errors.New("task or submission changed; reload before continuing")
	ErrTaskContractRequired = errors.New("this task requires a versioned handoff and review")
	ErrTaskDeliveryInvalid  = errors.New("invalid task delivery")
	ErrTaskNotReviewer      = errors.New("you are not the designated reviewer")
	ErrTaskReviewLimit      = errors.New("revision limit reached; creator must revise the contract")
)

type TaskRequirement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type TaskGate struct {
	HumanReviewMode string              `json:"human_review_mode,omitempty"`
	Code            *agent.CodeGateSpec `json:"code,omitempty"`
	Kind            string              `json:"kind"`
	ReviewerID      string              `json:"reviewer_id"`
	MaxRevisions    int                 `json:"max_revisions"`
}

type TaskContract struct {
	Requirements []TaskRequirement `json:"requirements"`
	Gate         TaskGate          `json:"gate"`
}

type TaskEvidence struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Inline content is immutable in PostgreSQL. External references must have a digest.
	Content string `json:"content,omitempty"`
	URI     string `json:"uri,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type TaskHandoff struct {
	Summary   string `json:"summary"`
	Changes   string `json:"changes"`
	Risks     string `json:"risks"`
	NextSteps string `json:"next_steps"`
}

// Agents commonly produce bullet arrays. Canonicalize both text and text lists
// before validating or hashing, so retries have one stable representation.
func (h *TaskHandoff) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, destination := range map[string]*string{"summary": &h.Summary, "changes": &h.Changes, "risks": &h.Risks, "next_steps": &h.NextSteps} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, destination); err == nil {
			continue
		}
		var items []string
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("handoff.%s must be text or a list of text", key)
		}
		*destination = strings.Join(items, "\n")
	}
	return nil
}

type TaskSubmitRequest struct {
	ExpectedTaskVersion int64          `json:"expected_task_version"`
	IdempotencyKey      string         `json:"idempotency_key"`
	ArtifactVersion     string         `json:"artifact_version"`
	Handoff             TaskHandoff    `json:"handoff"`
	Evidence            []TaskEvidence `json:"evidence"`
}

type TaskCheck struct {
	RequirementID string   `json:"requirement_id"`
	Passed        bool     `json:"passed"`
	EvidenceIDs   []string `json:"evidence_ids"`
	Reason        string   `json:"reason"`
}

type TaskReviewRequest struct {
	CodeGateRunID   string         `json:"-"`
	SubmissionID    string         `json:"submission_id"`
	ArtifactVersion string         `json:"artifact_version"`
	Decision        string         `json:"decision"`
	Reason          string         `json:"reason"`
	Checks          []TaskCheck    `json:"checks"`
	Evidence        []TaskEvidence `json:"evidence"`
	IdempotencyKey  string         `json:"idempotency_key"`
}

type TaskSubmission struct {
	ID              string                 `json:"id"`
	TaskID          string                 `json:"task_id"`
	SubmittedBy     string                 `json:"submitted_by"`
	TaskVersion     int64                  `json:"task_version"`
	Contract        TaskContract           `json:"contract"`
	Handoff         TaskHandoff            `json:"handoff"`
	ArtifactVersion string                 `json:"artifact_version"`
	Evidence        []TaskEvidence         `json:"evidence"`
	CreatedAt       time.Time              `json:"created_at"`
	Reviews         []TaskSubmissionReview `json:"reviews"`
}

type TaskSubmissionReview struct {
	ReviewerID string         `json:"reviewer_id"`
	Decision   string         `json:"decision"`
	Reason     string         `json:"reason"`
	Checks     []TaskCheck    `json:"checks"`
	Evidence   []TaskEvidence `json:"evidence"`
	CreatedAt  time.Time      `json:"created_at"`
}

var deliveryID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var deliveryDigest = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func invalidDelivery(message string) error {
	return fmt.Errorf("%w: %s", ErrTaskDeliveryInvalid, message)
}

func (s *TaskService) ValidateContract(ctx context.Context, channelID, creatorID string, contract *TaskContract) error {
	return validateTaskContract(ctx, s.pool, channelID, creatorID, contract)
}

func validateTaskContract(ctx context.Context, db agentRunRowQuerier, channelID, creatorID string, contract *TaskContract) error {
	if contract == nil {
		return nil
	}
	if len(contract.Requirements) == 0 || len(contract.Requirements) > 50 {
		return invalidDelivery("1–50 requirements are required")
	}
	seen := map[string]bool{}
	for _, requirement := range contract.Requirements {
		if !deliveryID.MatchString(requirement.ID) || seen[requirement.ID] || strings.TrimSpace(requirement.Text) == "" || len(requirement.Text) > 4000 {
			return invalidDelivery("requirements need unique IDs and nonempty text")
		}
		seen[requirement.ID] = true
	}
	gate := &contract.Gate
	if gate.HumanReviewMode != "" && (gate.Kind != "human" || gate.HumanReviewMode != "decision") {
		return invalidDelivery("human_review_mode decision is only valid for a human Gate")
	}
	if gate.Kind != "human" && gate.Kind != "agent" && gate.Kind != "code" {
		return invalidDelivery("gate kind must be human, agent, or code")
	}
	if gate.ReviewerID == "" && gate.Kind == "human" {
		gate.ReviewerID = creatorID
	}
	if _, err := uuid.Parse(gate.ReviewerID); err != nil {
		return invalidDelivery("reviewer_id must be a UUID")
	}
	if gate.MaxRevisions == 0 {
		gate.MaxRevisions = 3
	}
	if gate.MaxRevisions < 1 || gate.MaxRevisions > 20 {
		return invalidDelivery("max_revisions must be 1–20")
	}
	if gate.Kind == "code" {
		if gate.Code == nil || !filepath.IsAbs(gate.Code.RepositoryPath) || len(gate.Code.RepositoryPath) > 4096 || strings.ContainsRune(gate.Code.RepositoryPath, 0) || !agent.FullGitCommit.MatchString(gate.Code.BaseCommit) {
			return invalidDelivery("code Gate requires a repository and full base commit")
		}
		if gate.Code.TimeoutSeconds == 0 {
			gate.Code.TimeoutSeconds = 120
		}
		if gate.Code.TimeoutSeconds < 1 || gate.Code.TimeoutSeconds > 300 {
			return invalidDelivery("code Gate timeout must be 1–300 seconds")
		}
		var owns bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1 AND owner_id=$2 AND is_active)`, gate.ReviewerID, creatorID).Scan(&owns); err != nil {
			return err
		}
		if !owns {
			return invalidDelivery("only the Reviewer Agent owner can authorize code Gate commands")
		}
		covered := map[string]bool{}
		for _, check := range gate.Code.Checks {
			if !seen[check.RequirementID] || covered[check.RequirementID] || len(check.Command) == 0 || len(check.Command) > 50 {
				return invalidDelivery("each requirement needs one nonempty code check")
			}
			for _, arg := range check.Command {
				if len(arg) > 16000 || strings.ContainsRune(arg, 0) {
					return invalidDelivery("invalid code check argument")
				}
			}
			if strings.TrimSpace(check.Command[0]) == "" {
				return invalidDelivery("check executable is required")
			}
			covered[check.RequirementID] = true
		}
		if len(covered) != len(seen) {
			return invalidDelivery("every requirement needs a configured code check")
		}
	} else if gate.Code != nil {
		return invalidDelivery("code checks require the code Gate")
	}
	memberType := "user"
	if gate.Kind == "agent" || gate.Kind == "code" {
		memberType = "agent"
	}
	var allowed bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channel_members WHERE channel_id=$1 AND member_id=$2 AND member_type=$3)`, channelID, gate.ReviewerID, memberType).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return invalidDelivery("reviewer must be a channel member of the selected kind")
	}
	return nil
}

func validateEvidence(evidence []TaskEvidence, required bool) error {
	if (required && len(evidence) == 0) || len(evidence) > 50 {
		return invalidDelivery("1–50 evidence items are required")
	}
	seen := map[string]bool{}
	for _, item := range evidence {
		if !deliveryID.MatchString(item.ID) || seen[item.ID] || strings.TrimSpace(item.Description) == "" {
			return invalidDelivery("evidence needs unique IDs and a description")
		}
		seen[item.ID] = true
		if len(item.Content) > 64000 || len(item.Description) > 4000 || len(item.URI) > 4000 {
			return invalidDelivery("evidence is too large")
		}
		if item.Content == "" && (item.URI == "" || !deliveryDigest.MatchString(item.SHA256)) {
			return invalidDelivery("evidence needs inline content or a URI with SHA256")
		}
		if item.SHA256 != "" && !deliveryDigest.MatchString(item.SHA256) {
			return invalidDelivery("invalid SHA256")
		}
		if item.Content != "" && item.SHA256 != "" {
			digest := sha256.Sum256([]byte(item.Content))
			if !strings.EqualFold(item.SHA256, hex.EncodeToString(digest[:])) {
				return invalidDelivery("evidence content does not match SHA256")
			}
		}
	}
	return nil
}

func deliveryRequestHash(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (s *TaskService) SubmitDelivery(ctx context.Context, channelID, taskID, actorID, runID string, req TaskSubmitRequest) (*Task, error) {
	if !deliveryID.MatchString(req.IdempotencyKey) || req.ExpectedTaskVersion < 1 || strings.TrimSpace(req.Handoff.Summary) == "" || strings.TrimSpace(req.ArtifactVersion) == "" || len(req.ArtifactVersion) > 256 {
		return nil, invalidDelivery("version, idempotency key, artifact version and handoff summary are required")
	}
	if len(req.Handoff.Summary)+len(req.Handoff.Changes)+len(req.Handoff.Risks)+len(req.Handoff.NextSteps) > 64000 {
		return nil, invalidDelivery("handoff is too large")
	}
	if err := validateEvidence(req.Evidence, true); err != nil {
		return nil, err
	}
	task, err := s.GetTask(ctx, channelID, taskID, actorID)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT status, COALESCE(claimer_id::text,''), version, contract FROM tasks WHERE id=$1 FOR UPDATE`, task.ID).Scan(&task.Status, &task.ClaimerID, &task.Version, &task.Contract); err != nil {
		return nil, err
	}
	if task.ClaimerID != actorID {
		return nil, ErrTaskNotClaimer
	}
	if task.Contract == nil {
		return nil, ErrTaskContractRequired
	}
	var previousHash string
	err = tx.QueryRow(ctx, `SELECT request_hash FROM task_submissions WHERE task_id=$1 AND idempotency_key=$2 AND submitted_by=$3`, task.ID, req.IdempotencyKey, actorID).Scan(&previousHash)
	if err == nil {
		if previousHash != deliveryRequestHash(req) {
			return nil, ErrTaskVersionConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return s.GetTask(ctx, channelID, task.ID, actorID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if task.Version != req.ExpectedTaskVersion {
		return nil, ErrTaskVersionConflict
	}
	if task.Status != TaskStatusInProgress {
		return nil, ErrTaskNotSubmittable
	}
	if task.Contract.Gate.ReviewerID == actorID {
		return nil, invalidDelivery("author and reviewer must be different")
	}
	if task.Contract.Gate.Kind == "code" && !agent.FullGitCommit.MatchString(req.ArtifactVersion) {
		return nil, invalidDelivery("code delivery artifact_version must be a full lowercase Git commit")
	}
	var open, revisions int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM tasks WHERE parent_task_id=$1 AND status NOT IN ('done','closed')), (SELECT count(*) FROM task_submissions WHERE task_id=$1)`, task.ID).Scan(&open, &revisions); err != nil {
		return nil, err
	}
	if open > 0 {
		return nil, ErrTaskHasOpenSubtasks
	}
	if revisions >= task.Contract.Gate.MaxRevisions {
		return nil, ErrTaskReviewLimit
	}
	if runID != "" {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE id=$1 AND agent_id=$2)`, runID, actorID).Scan(&valid); err != nil {
			return nil, err
		}
		if !valid {
			return nil, invalidDelivery("run does not belong to submitting agent")
		}
	}
	submissionID := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO task_submissions(id,task_id,submitted_by,run_id,task_version,contract,handoff,artifact_version,evidence,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, submissionID, task.ID, actorID, nullableStr(runID), task.Version, task.Contract, req.Handoff, req.ArtifactVersion, req.Evidence, req.IdempotencyKey, deliveryRequestHash(req))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status='in_review',current_submission_id=$2,updated_at=now() WHERE id=$1`, task.ID, submissionID); err != nil {
		return nil, err
	}
	if task.Contract.Gate.Kind == "agent" || task.Contract.Gate.Kind == "code" {
		if _, err := tx.Exec(ctx, `INSERT INTO task_review_deliveries(submission_id,reviewer_id) VALUES($1,$2)`, submissionID, task.Contract.Gate.ReviewerID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetTask(ctx, channelID, task.ID, actorID)
}

func (s *TaskService) ReviewDelivery(ctx context.Context, channelID, taskID, actorID string, req TaskReviewRequest) (*Task, error) {
	if req.Evidence == nil {
		req.Evidence = []TaskEvidence{}
	}
	if req.Checks == nil {
		req.Checks = []TaskCheck{}
	}
	if _, err := uuid.Parse(req.SubmissionID); err != nil {
		return nil, invalidDelivery("submission_id must be a UUID")
	}
	if !deliveryID.MatchString(req.IdempotencyKey) || (req.Decision != "accepted" && req.Decision != "rejected" && req.Decision != "needs_human") {
		return nil, invalidDelivery("decision and idempotency key are required")
	}
	if strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 16000 {
		return nil, invalidDelivery("review reason is required (max 16000 bytes)")
	}
	if err := validateEvidence(req.Evidence, false); err != nil {
		return nil, err
	}
	task, err := s.GetTask(ctx, channelID, taskID, actorID)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT status,version,contract,COALESCE(current_submission_id::text,''),COALESCE(claimer_id::text,'') FROM tasks WHERE id=$1 FOR UPDATE`, task.ID).Scan(&task.Status, &task.Version, &task.Contract, &task.CurrentSubmissionID, &task.ClaimerID); err != nil {
		return nil, err
	}
	if task.Contract == nil {
		return nil, ErrTaskContractRequired
	}
	allowed := task.Contract.Gate.ReviewerID == actorID
	if !allowed && task.CreatorID == actorID {
		// A human creator can resolve a recorded escalation, but cannot silently replace a Gate.
		if err := tx.QueryRow(ctx, `SELECT (EXISTS(SELECT 1 FROM task_reviews WHERE submission_id=$1 AND decision='needs_human') OR EXISTS(SELECT 1 FROM task_review_deliveries d LEFT JOIN agent_runs r ON r.id=d.run_id WHERE d.submission_id=$1 AND d.attempts>=3 AND (r.id IS NULL OR r.finished_at IS NOT NULL))) AND EXISTS(SELECT 1 FROM users WHERE id=$2)`, req.SubmissionID, actorID).Scan(&allowed); err != nil {
			return nil, err
		}
	}
	if !allowed || task.ClaimerID == actorID {
		return nil, ErrTaskNotReviewer
	}
	var previousHash string
	err = tx.QueryRow(ctx, `SELECT request_hash FROM task_reviews WHERE submission_id=$1 AND idempotency_key=$2 AND reviewer_id=$3 AND task_id=$4`, req.SubmissionID, req.IdempotencyKey, actorID, task.ID).Scan(&previousHash)
	if err == nil {
		if previousHash != deliveryRequestHash(req) {
			return nil, ErrTaskVersionConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return s.GetTask(ctx, channelID, task.ID, actorID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if task.Contract.Gate.Kind == "code" && actorID == task.Contract.Gate.ReviewerID {
		var valid bool
		if req.CodeGateRunID == "" {
			return nil, ErrTaskNotReviewer
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_review_deliveries d JOIN agent_runs r ON r.id=d.run_id WHERE d.submission_id=$1 AND r.id::text=$2 AND r.agent_id=$3 AND r.source='code_gate' AND r.finished_at IS NULL)`, req.SubmissionID, req.CodeGateRunID, actorID).Scan(&valid); err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrTaskNotReviewer
		}
	}
	if task.Status != TaskStatusInReview || task.CurrentSubmissionID != req.SubmissionID {
		return nil, ErrTaskVersionConflict
	}
	var sub TaskSubmission
	if err := tx.QueryRow(ctx, `SELECT task_version,contract,artifact_version,evidence FROM task_submissions WHERE id=$1 AND task_id=$2`, req.SubmissionID, task.ID).Scan(&sub.TaskVersion, &sub.Contract, &sub.ArtifactVersion, &sub.Evidence); err != nil {
		return nil, err
	}
	if sub.TaskVersion+1 != task.Version || sub.ArtifactVersion != req.ArtifactVersion {
		return nil, ErrTaskVersionConflict
	}
	if err := validateReviewChecks(sub.Contract, sub.Evidence, req); err != nil {
		return nil, err
	}
	nextStatus := TaskStatusInReview
	if req.Decision == "accepted" {
		nextStatus = TaskStatusDone
	}
	if req.Decision == "rejected" {
		nextStatus = TaskStatusInProgress
	}
	if _, err := tx.Exec(ctx, `INSERT INTO task_reviews(task_id,reviewer_id,decision,reason,submission_id,evidence,checks,idempotency_key,request_hash,next_owner_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, task.ID, actorID, req.Decision, req.Reason, req.SubmissionID, req.Evidence, req.Checks, req.IdempotencyKey, deliveryRequestHash(req), nullableStr(task.ClaimerID)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status=$2,updated_at=now() WHERE id=$1`, task.ID, nextStatus); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM task_review_deliveries WHERE submission_id=$1`, req.SubmissionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	updated, err := s.GetTask(ctx, channelID, task.ID, actorID)
	if err == nil && s.agentNotifier != nil && req.Decision == "rejected" {
		_ = s.agentNotifier.NotifyRejected(ctx, task.ID, actorID, req.Reason)
	}
	return updated, err
}

func validateReviewChecks(contract TaskContract, evidence []TaskEvidence, req TaskReviewRequest) error {
	// A result decision records consent, never fabricated technical checks.
	// Existing contracts and all Agent/code Gates retain per-requirement review.
	if contract.Gate.Kind == "human" && contract.Gate.HumanReviewMode == "decision" {
		if len(req.Checks) != 0 {
			return invalidDelivery("a human result decision must not claim technical checks")
		}
		return nil
	}
	known := map[string]bool{}
	for _, e := range append(append([]TaskEvidence{}, evidence...), req.Evidence...) {
		known[e.ID] = true
	}
	requirements := map[string]bool{}
	for _, r := range contract.Requirements {
		requirements[r.ID] = true
	}
	seen := map[string]bool{}
	for _, check := range req.Checks {
		if !requirements[check.RequirementID] || seen[check.RequirementID] || strings.TrimSpace(check.Reason) == "" || len(check.Reason) > 4000 {
			return invalidDelivery("checks require known unique requirement IDs and reasons")
		}
		seen[check.RequirementID] = true
		for _, id := range check.EvidenceIDs {
			if !known[id] {
				return invalidDelivery("check references unknown evidence")
			}
		}
		if req.Decision == "accepted" && (!check.Passed || len(check.EvidenceIDs) == 0) {
			return invalidDelivery("acceptance requires passed checks with evidence")
		}
	}
	if req.Decision == "accepted" && len(seen) != len(requirements) {
		return invalidDelivery("acceptance must check every requirement")
	}
	return nil
}

func (s *TaskService) ListSubmissions(ctx context.Context, channelID, taskID, actorID string) ([]TaskSubmission, error) {
	task, err := s.GetTask(ctx, channelID, taskID, actorID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT s.id,s.task_id,s.submitted_by,s.task_version,s.contract,s.handoff,s.artifact_version,s.evidence,s.created_at,COALESCE((SELECT jsonb_agg(jsonb_build_object('reviewer_id',r.reviewer_id,'decision',r.decision,'reason',r.reason,'checks',r.checks,'evidence',r.evidence,'created_at',r.created_at) ORDER BY r.created_at) FROM task_reviews r WHERE r.submission_id=s.id),'[]') FROM task_submissions s WHERE task_id=$1 ORDER BY s.created_at DESC`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TaskSubmission{}
	for rows.Next() {
		var sub TaskSubmission
		if err := rows.Scan(&sub.ID, &sub.TaskID, &sub.SubmittedBy, &sub.TaskVersion, &sub.Contract, &sub.Handoff, &sub.ArtifactVersion, &sub.Evidence, &sub.CreatedAt, &sub.Reviews); err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	return result, rows.Err()
}

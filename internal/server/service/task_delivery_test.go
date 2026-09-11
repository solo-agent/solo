package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/solo-ai/solo/pkg/agent"
)

func TestRuntimeContractSubmissionExampleMatchesValidator(t *testing.T) {
	prompt := agent.BuildSystemPrompt(agent.AgentConfig{}, agent.ChannelContext{}, "", nil)
	_, example, found := strings.Cut(prompt, "**Contract submission JSON:**\n```json\n")
	if !found {
		t.Fatal("Runtime cannot discover the submission format")
	}
	example, _, found = strings.Cut(example, "\n```")
	var request TaskSubmitRequest
	if !found || json.Unmarshal([]byte(example), &request) != nil {
		t.Fatal("Runtime submission example is not valid request JSON")
	}
	if request.ExpectedTaskVersion < 1 || request.IdempotencyKey == "" || request.ArtifactVersion == "" || request.Handoff.Summary == "" {
		t.Fatal("Runtime submission example is missing required fields")
	}
	if err := validateEvidence(request.Evidence, true); err != nil {
		t.Fatalf("Runtime teaches an invalid evidence format: %v", err)
	}
}

func TestCodeGateReviewRequiresItsActualRun(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	reviewer := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	taskSubmitMember(t, pool, channel, "agent", reviewer)
	svc := NewTaskService(pool)
	commit := strings.Repeat("a", 40)
	contract := &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "actual test passes"}}, Gate: TaskGate{Kind: "code", ReviewerID: reviewer, Code: &agent.CodeGateSpec{RepositoryPath: "/tmp/authorized-repo", BaseCommit: commit, Checks: []agent.CodeGateCheck{{RequirementID: "R1", Command: []string{"make", "test"}}}}}}
	task, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "code gate", Assignee: author, Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	submit := TaskSubmitRequest{ExpectedTaskVersion: task.Version, ArtifactVersion: "mutable-branch", IdempotencyKey: "code-sub", Handoff: TaskHandoff{Summary: "ready"}, Evidence: []TaskEvidence{{ID: "E1", Description: "source", Content: "verified"}}}
	if _, err = svc.SubmitDelivery(ctx, channel, task.ID, author, "", submit); err == nil {
		t.Fatal("accepted branch in code Gate")
	}
	submit.ArtifactVersion = commit
	task, err = svc.SubmitDelivery(ctx, channel, task.ID, author, "", submit)
	if err != nil {
		t.Fatal(err)
	}
	review := TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: commit, Decision: "accepted", Reason: "check passed", IdempotencyKey: "code-review", Checks: []TaskCheck{{RequirementID: "R1", Passed: true, Reason: "executed", EvidenceIDs: []string{"E1"}}}}
	if _, err = svc.ReviewDelivery(ctx, channel, task.ID, reviewer, review); !errors.Is(err, ErrTaskNotReviewer) {
		t.Fatal("model impersonated code gate", err)
	}
	run, err := NewAgentRunService(pool).StartRun(ctx, StartRunInput{AgentID: reviewer, ChannelID: channel, TriggerType: AgentRunTriggerTask, Source: "code_gate"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE task_review_deliveries SET run_id=$2 WHERE submission_id=$1`, task.CurrentSubmissionID, run.ID); err != nil {
		t.Fatal(err)
	}
	review.CodeGateRunID = run.ID
	result, err := svc.ReviewDelivery(ctx, channel, task.ID, reviewer, review)
	if err != nil || result.Status != TaskStatusDone {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err = svc.ReviewDelivery(ctx, channel, task.ID, reviewer, review); err != nil {
		t.Fatal("lost idempotence after queue consumed", err)
	}
	newTitle := "changed after acceptance"
	if _, err = svc.UpdateTask(ctx, channel, task.ID, owner, TaskUpdateRequest{Title: &newTitle, ExpectedTaskVersion: result.Version}); !errors.Is(err, ErrTaskDeliveryInvalid) {
		t.Fatalf("changed accepted task without reopening: %v", err)
	}
}

func TestDeliveryEvidenceAndAcceptanceValidation(t *testing.T) {
	contract := TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "works"}}}
	evidence := []TaskEvidence{{ID: "E1", Description: "output", Content: "verified"}}
	review := TaskReviewRequest{Decision: "accepted", Checks: []TaskCheck{{RequirementID: "R1", Passed: true, Reason: "verified", EvidenceIDs: []string{"E1"}}}}
	if err := validateEvidence(evidence, true); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewChecks(contract, evidence, review); err != nil {
		t.Fatal(err)
	}
	review.Checks[0].EvidenceIDs = []string{"missing"}
	if !errors.Is(validateReviewChecks(contract, evidence, review), ErrTaskDeliveryInvalid) {
		t.Fatal("accepted unknown evidence")
	}
	review.Checks = nil
	if !errors.Is(validateReviewChecks(contract, evidence, review), ErrTaskDeliveryInvalid) {
		t.Fatal("accepted missing requirement")
	}
	if !errors.Is(validateEvidence([]TaskEvidence{{ID: "E1", Description: "mutable URL", URI: "https://example.test/latest"}}, true), ErrTaskDeliveryInvalid) {
		t.Fatal("accepted unversioned external evidence")
	}
	if !errors.Is(validateEvidence([]TaskEvidence{{ID: "E1", Description: "wrong hash", Content: "test", SHA256: strings.Repeat("a", 64)}}, true), ErrTaskDeliveryInvalid) {
		t.Fatal("accepted incorrect digest")
	}
}

// All lifecycle and race assertions run against migrated PostgreSQL and the real service.
func TestTaskDeliveryPostgresLifecycle(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creator := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, creator)
	channel := taskSubmitChannel(t, pool, creator)
	taskSubmitMember(t, pool, channel, "user", creator)
	taskSubmitMember(t, pool, channel, "agent", author)
	svc := NewTaskService(pool)
	contract := &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "return a verified result"}}, Gate: TaskGate{Kind: "human", ReviewerID: creator, MaxRevisions: 5}}
	task, err := svc.CreateTask(ctx, channel, creator, TaskCreateRequest{Title: "versioned delivery", Assignee: author, Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	if task.Version != 1 || task.Contract == nil {
		t.Fatalf("missing create contract: %+v", task)
	}
	if _, err := svc.SubmitTask(ctx, channel, task.ID, author); !errors.Is(err, ErrTaskContractRequired) {
		t.Fatalf("legacy submit: %v", err)
	}
	requests := TaskSubmitRequest{ExpectedTaskVersion: task.Version, IdempotencyKey: "submit-1", ArtifactVersion: "v1", Handoff: TaskHandoff{Summary: "work complete"}, Evidence: []TaskEvidence{{ID: "E1", Description: "result", Content: "verified output"}}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.SubmitDelivery(ctx, channel, task.ID, author, "", requests)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("idempotent concurrent submit: %v", err)
		}
	}
	history, err := svc.ListSubmissions(ctx, channel, task.ID, creator)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("duplicate submission: %d", len(history))
	}
	if _, err := svc.AcceptTask(ctx, channel, task.ID, creator); !errors.Is(err, ErrTaskContractRequired) {
		t.Fatalf("legacy acceptance: %v", err)
	}
	review := TaskReviewRequest{SubmissionID: history[0].ID, ArtifactVersion: "v1", Decision: "accepted", Reason: "verified", IdempotencyKey: "review-1", Checks: []TaskCheck{{RequirementID: "R1", Passed: true, Reason: "reproduced", EvidenceIDs: []string{"E1"}}}}
	if _, err := svc.ReviewDelivery(ctx, channel, task.ID, author, review); !errors.Is(err, ErrTaskNotReviewer) {
		t.Fatalf("self-review: %v", err)
	}
	current, err := svc.GetTask(ctx, channel, task.ID, creator)
	if err != nil {
		t.Fatal(err)
	}
	description := "new requirement scope"
	updated, err := svc.UpdateTask(ctx, channel, task.ID, creator, TaskUpdateRequest{Description: &description, ExpectedTaskVersion: current.Version})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != TaskStatusInProgress || updated.CurrentSubmissionID != "" {
		t.Fatalf("old submission still active: %+v", updated)
	}
	if _, err := svc.ReviewDelivery(ctx, channel, task.ID, creator, review); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("stale acceptance: %v", err)
	}
	requests.ExpectedTaskVersion = updated.Version
	requests.IdempotencyKey = "submit-2"
	requests.ArtifactVersion = "v2"
	updated, err = svc.SubmitDelivery(ctx, channel, task.ID, author, "", requests)
	if err != nil {
		t.Fatal(err)
	}
	review.SubmissionID = updated.CurrentSubmissionID
	review.ArtifactVersion = "v2"
	review.Decision = "rejected"
	review.IdempotencyKey = "review-2"
	review.Reason = "missing edge case"
	updated, err = svc.ReviewDelivery(ctx, channel, task.ID, creator, review)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != TaskStatusInProgress {
		t.Fatalf("reject status: %s", updated.Status)
	}
	requests.ExpectedTaskVersion = updated.Version
	requests.IdempotencyKey = "submit-3"
	requests.ArtifactVersion = "v3"
	updated, err = svc.SubmitDelivery(ctx, channel, task.ID, author, "", requests)
	if err != nil {
		t.Fatal(err)
	}
	review.SubmissionID = updated.CurrentSubmissionID
	review.ArtifactVersion = "v3"
	review.Decision = "accepted"
	review.IdempotencyKey = "review-3"
	updated, err = svc.ReviewDelivery(ctx, channel, task.ID, creator, review)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != TaskStatusDone {
		t.Fatalf("accepted status: %s", updated.Status)
	}
	if _, err := svc.ReviewDelivery(ctx, channel, task.ID, creator, review); err != nil {
		t.Fatalf("idempotent review: %v", err)
	}
	review.Reason = "different payload"
	if _, err := svc.ReviewDelivery(ctx, channel, task.ID, creator, review); !errors.Is(err, ErrTaskVersionConflict) {
		t.Fatalf("changed idempotent request: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_reviews WHERE task_id=$1 AND decision='accepted'`, task.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("persisted accept count %d: %v", count, err)
	}
}

func TestTaskDeliveryPostgresChildrenAndBypass(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	creator := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, creator)
	channel := taskSubmitChannel(t, pool, creator)
	taskSubmitMember(t, pool, channel, "user", creator)
	taskSubmitMember(t, pool, channel, "agent", author)
	svc := NewTaskService(pool)
	parent, err := svc.CreateTask(ctx, channel, creator, TaskCreateRequest{Title: "parent", Assignee: author, Contract: &TaskContract{Requirements: []TaskRequirement{{ID: "R1", Text: "children complete"}}, Gate: TaskGate{Kind: "human", ReviewerID: creator}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTask(ctx, channel, creator, TaskCreateRequest{Title: "open child", ParentTaskID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SubmitDelivery(ctx, channel, parent.ID, author, "", TaskSubmitRequest{ExpectedTaskVersion: parent.Version, IdempotencyKey: "submit", ArtifactVersion: "v1", Handoff: TaskHandoff{Summary: "done"}, Evidence: []TaskEvidence{{ID: "E1", Description: "result", Content: "output"}}})
	if !errors.Is(err, ErrTaskHasOpenSubtasks) {
		t.Fatalf("open children allowed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET status='done' WHERE id=$1`, parent.ID); err == nil {
		t.Fatal("database allowed bypassing Gate")
	}
}

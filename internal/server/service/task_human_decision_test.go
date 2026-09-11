package service

import (
	"context"
	"errors"
	"testing"
)

func TestHumanResultDecisionPersistsWithoutInventedChecks(t *testing.T) {
	pool := taskSubmitTestPool(t)
	ctx := context.Background()
	owner := taskSubmitUser(t, pool)
	author := taskSubmitAgent(t, pool, owner)
	channel := taskSubmitChannel(t, pool, owner)
	taskSubmitMember(t, pool, channel, "user", owner)
	taskSubmitMember(t, pool, channel, "agent", author)
	svc := NewTaskService(pool)
	contract := &TaskContract{
		Requirements: []TaskRequirement{{ID: "R1", Text: "Provide the requested result"}},
		Gate:         TaskGate{Kind: "human", ReviewerID: owner, HumanReviewMode: "decision"},
	}
	task, err := svc.CreateTask(ctx, channel, owner, TaskCreateRequest{Title: "Human result decision", Assignee: author, Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	task, err = svc.SubmitDelivery(ctx, channel, task.ID, author, "", TaskSubmitRequest{
		ExpectedTaskVersion: task.Version, IdempotencyKey: "result", ArtifactVersion: "result-v1",
		Handoff:  TaskHandoff{Summary: "Result supplied"},
		Evidence: []TaskEvidence{{ID: "E1", Description: "Result", Content: "20 + 22 = 42"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	review := TaskReviewRequest{SubmissionID: task.CurrentSubmissionID, ArtifactVersion: "result-v1", Decision: "accepted", Reason: "I accept this result", IdempotencyKey: "owner-decision"}
	if _, err := svc.ReviewDelivery(ctx, channel, task.ID, author, review); !errors.Is(err, ErrTaskNotReviewer) {
		t.Fatalf("author may not make the owner decision: %v", err)
	}
	task, err = svc.ReviewDelivery(ctx, channel, task.ID, owner, review)
	if err != nil || task.Status != TaskStatusDone {
		t.Fatalf("owner decision failed: %+v, %v", task, err)
	}
	history, err := svc.ListSubmissions(ctx, channel, task.ID, owner)
	if err != nil || len(history) != 1 || len(history[0].Reviews) != 1 {
		t.Fatalf("missing persisted history: %+v, %v", history, err)
	}
	if history[0].Contract.Gate.HumanReviewMode != "decision" || len(history[0].Reviews[0].Checks) != 0 || history[0].Reviews[0].Reason != review.Reason {
		t.Fatal("result decision was confused with technical verification")
	}
	review.Checks = []TaskCheck{{RequirementID: "R1", Passed: true, Reason: "pretend independent check", EvidenceIDs: []string{"E1"}}}
	if validateReviewChecks(*contract, history[0].Evidence, review) == nil {
		t.Fatal("a result decision accepted invented technical check records")
	}
	review.Checks = nil
	contract.Gate.HumanReviewMode = ""
	if validateReviewChecks(*contract, history[0].Evidence, review) == nil {
		t.Fatal("legacy strict human review was weakened")
	}
	contract.Gate.Kind = "agent"
	contract.Gate.ReviewerID = author
	contract.Gate.HumanReviewMode = "decision"
	if svc.ValidateContract(ctx, channel, owner, contract) == nil {
		t.Fatal("Agent Gate bypassed independent requirement checks")
	}
}

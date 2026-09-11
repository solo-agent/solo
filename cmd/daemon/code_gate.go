package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/pkg/agent"
)

func (h *daemonHandler) processCodeReview(ctx context.Context, req runTaskRequest) {
	finalizer := &backendTurnFinalizer{h: h, req: req}
	defer finalizer.finalize()
	fail := func(err error) {
		finalizer.finish(taskStatusFailed, "error", map[string]interface{}{"agent_id": req.AgentID, "error": err.Error()})
	}
	for _, id := range []string{req.AgentID, req.RunID, req.CodeReview.SubmissionID} {
		if _, err := uuid.Parse(id); err != nil {
			fail(err)
			return
		}
	}
	workDir := h.workspaceManager.WorkspaceDir(req.AgentID)
	if err := os.MkdirAll(workDir, 0700); err != nil {
		fail(err)
		return
	}
	if _, err := agent.PrepareRevisionSkills(workDir, req.ModelConfig.Provider, req.Skills); err != nil {
		fail(err)
		return
	}
	h.pushBackendStarted(req)
	path := filepath.Join(h.workspaceManager.WorkspacePath(req.AgentID), "code-reviews", req.CodeReview.SubmissionID+"-"+req.RunID)
	result := agent.RunCodeGate(ctx, *req.CodeReview, path)
	review := service.TaskReviewRequest{SubmissionID: req.CodeReview.SubmissionID, ArtifactVersion: req.CodeReview.ArtifactVersion, Decision: result.Decision, Reason: result.Reason, IdempotencyKey: "code-" + req.RunID}
	receipt, _ := json.Marshal(map[string]string{"worktree": result.Worktree, "commit": result.Commit, "reason": result.Reason})
	if len(result.Checks) == 0 {
		review.Evidence = []service.TaskEvidence{{ID: "gate-receipt", Description: "Runtime code verification receipt", Content: string(receipt)}}
	}
	for i, check := range result.Checks {
		id := fmt.Sprintf("gate-%d", i+1)
		review.Evidence = append(review.Evidence, service.TaskEvidence{ID: id, Description: "Configured check " + check.RequirementID, Content: string(receipt) + "\n" + check.Output})
		review.Checks = append(review.Checks, service.TaskCheck{RequirementID: check.RequirementID, Passed: check.Passed, EvidenceIDs: []string{id}, Reason: result.Reason})
	}
	body, _ := json.Marshal(review)
	url := fmt.Sprintf("%s/api/v1/channels/%s/tasks/%s/review", h.serverURL, req.ChannelID, req.OriginTaskID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		fail(err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+req.AgentToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := h.httpClient.Do(request)
	if err != nil {
		fail(err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		fail(fmt.Errorf("persist code review: %d %s", response.StatusCode, body))
		return
	}
	finalizer.finish(taskStatusCompleted, "complete", map[string]interface{}{"agent_id": req.AgentID, "code_gate": result})
}

func (h *daemonHandler) taskWorktree(w http.ResponseWriter, r *http.Request, agentID, channelID string, number int, token string) {
	if _, err := uuid.Parse(agentID); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid agent"})
		return
	}
	if _, err := uuid.Parse(channelID); err != nil || number < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid task"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/channels/%s/tasks/%d", h.serverURL, channelID, number), nil)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := h.httpClient.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(response.Body, 4096))
		return
	}
	var task service.Task
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	if task.ClaimerID != agentID || task.Contract == nil || task.Contract.Gate.Code == nil {
		writeJSON(w, 403, map[string]string{"error": "only the assigned Agent can prepare this code Task"})
		return
	}
	if _, err := uuid.Parse(task.ID); err != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid task ID"})
		return
	}
	gate := task.Contract.Gate.Code
	path, err := agent.PrepareTaskWorktree(ctx, gate.RepositoryPath, filepath.Join(h.workspaceManager.WorkspaceDir(agentID), "tasks", task.ID), gate.BaseCommit)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"path": path, "base_commit": gate.BaseCommit, "task_id": task.ID})
}

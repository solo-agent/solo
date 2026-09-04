package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAgentRunServiceLifecycle(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	outsiderID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	var agentName string
	if err := pool.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, agentID).Scan(&agentName); err != nil {
		t.Fatalf("load agent name: %v", err)
	}
	channelID := agentRunChannel(t, pool, ownerID)
	messageID := agentRunMessage(t, pool, channelID, ownerID)
	taskID := agentRunTask(t, pool, channelID, ownerID)
	var runID string
	t.Cleanup(func() {
		if runID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id = $1`, runID)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, outsiderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	if _, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:  agentID,
		Provider: "codex",
	}); err == nil {
		t.Fatal("UpsertSession without external_session_id or transcript_path succeeded, want error")
	}

	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "codex",
		ExternalSessionID: "provider-session-1",
		TranscriptPath:    agentRunTranscriptFile(t),
	})
	if err != nil {
		t.Fatalf("CreateOrResumeSession: %v", err)
	}

	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:          agentID,
		DaemonID:         "daemon-agent-run-test",
		TriggerType:      AgentRunTriggerMessage,
		TriggerMessageID: messageID,
		ChannelID:        channelID,
		Status:           AgentRunStatusQueued,
		ActivityText:     "等待执行",
		Source:           "codex",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	runID = run.ID
	if run.Status != AgentRunStatusQueued {
		t.Fatalf("run status = %q, want %q", run.Status, AgentRunStatusQueued)
	}
	if run.SessionID != "" {
		t.Fatalf("run session_id = %q, want empty before provider session is known", run.SessionID)
	}
	daemonID, err := svc.GetRunDaemonID(ctx, run.ID)
	if err != nil || daemonID != "daemon-agent-run-test" {
		t.Fatalf("run daemon_id = %q, %v", daemonID, err)
	}
	activeOnDaemon, err := svc.ListActiveRunsByDaemon(ctx, daemonID)
	if err != nil || !agentRunListContains(activeOnDaemon, run.ID) {
		t.Fatalf("active runs for daemon = %#v, %v", activeOnDaemon, err)
	}
	run, err = svc.BindRunSession(ctx, BindRunSessionInput{
		RunID:     run.ID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("BindRunSession: %v", err)
	}
	if run.SessionID != session.ID {
		t.Fatalf("bound run session_id = %q, want %q", run.SessionID, session.ID)
	}
	if run.Status != AgentRunStatusQueued || run.BackendStartedAt != nil {
		t.Fatalf("bound run = status %q backend_started_at %v, want queued without backend start", run.Status, run.BackendStartedAt)
	}
	for scope, check := range map[string]func(string) (bool, error){
		"run":     func(userID string) (bool, error) { return svc.UserCanAccessRun(ctx, userID, run.ID) },
		"session": func(userID string) (bool, error) { return svc.UserCanAccessSession(ctx, userID, session.ID) },
		"task":    func(userID string) (bool, error) { return svc.UserCanAccessTask(ctx, userID, taskID) },
	} {
		if allowed, err := check(ownerID); err != nil || !allowed {
			t.Fatalf("owner access to %s = %v, %v", scope, allowed, err)
		}
		if allowed, err := check(outsiderID); err != nil || allowed {
			t.Fatalf("outsider access to %s = %v, %v", scope, allowed, err)
		}
	}
	run, err = svc.MarkBackendStarted(ctx, run.ID)
	if err != nil {
		t.Fatalf("MarkBackendStarted: %v", err)
	}
	if run.Status != AgentRunStatusRunning || run.BackendStartedAt == nil {
		t.Fatalf("started run = status %q backend_started_at %v, want running with timestamp", run.Status, run.BackendStartedAt)
	}
	firstBackendStartedAt := *run.BackendStartedAt
	run, err = svc.MarkBackendStarted(ctx, run.ID)
	if err != nil {
		t.Fatalf("replay MarkBackendStarted: %v", err)
	}
	if run.BackendStartedAt == nil || !run.BackendStartedAt.Equal(firstBackendStartedAt) {
		t.Fatalf("replayed backend_started_at = %v, want %v", run.BackendStartedAt, firstBackendStartedAt)
	}
	activeRuns, err := svc.ListActiveRuns(ctx)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	var activeRun AgentRun
	foundActive := false
	for _, active := range activeRuns {
		if active.ID == run.ID {
			activeRun = active
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("ListActiveRuns did not include active run %s", run.ID)
	}
	if activeRun.AgentName != agentName {
		t.Fatalf("active run agent_name = %q, want %q", activeRun.AgentName, agentName)
	}
	transcript, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(transcript) != 1 || transcript[0].Text != "hello from transcript" {
		t.Fatalf("transcript = %+v, want session transcript fallback", transcript)
	}
	if _, err := svc.UpdateRunTranscript(ctx, UpdateRunTranscriptInput{
		RunID:          run.ID,
		TranscriptPath: session.TranscriptPath,
	}); err != nil {
		t.Fatalf("UpdateRunTranscript: %v", err)
	}
	if _, err := svc.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
		SessionID:      session.ID,
		TranscriptPath: agentRunTranscriptFileWithText(t, "newer session transcript"),
	}); err != nil {
		t.Fatalf("UpdateSessionMetadata newer transcript: %v", err)
	}
	transcript, err = svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript after snapshot: %v", err)
	}
	if len(transcript) != 1 || transcript[0].Text != "hello from transcript" {
		t.Fatalf("transcript = %+v, want run transcript snapshot", transcript)
	}

	pathOnly, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:        agentID,
		Provider:       "codex",
		TranscriptPath: agentRunTranscriptFileWithText(t, "path only transcript"),
	})
	if err != nil {
		t.Fatalf("UpsertSession with transcript path: %v", err)
	}
	pathOnlyAgain, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:        agentID,
		Provider:       "codex",
		TranscriptPath: pathOnly.TranscriptPath,
	})
	if err != nil {
		t.Fatalf("UpsertSession with same transcript path: %v", err)
	}
	if pathOnlyAgain.ID != pathOnly.ID {
		t.Fatalf("path-only session id = %q, want existing %q", pathOnlyAgain.ID, pathOnly.ID)
	}

	if err := svc.LinkTask(ctx, LinkRunTaskInput{
		RunID:      run.ID,
		TaskID:     taskID,
		Role:       AgentRunTaskRolePrimary,
		Confidence: 1,
	}); err != nil {
		t.Fatalf("LinkTask: %v", err)
	}

	event1, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   run.ID,
		Type:    AgentRunEventRunStarted,
		Message: "创建 run",
	})
	if err != nil {
		t.Fatalf("AppendEvent #1: %v", err)
	}
	event2, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:    run.ID,
		Type:     AgentRunEventToolStarted,
		Message:  "Bash: npm test",
		ToolName: "Bash",
	})
	if err != nil {
		t.Fatalf("AppendEvent #2: %v", err)
	}
	if event1.Seq != 1 || event2.Seq != 2 {
		t.Fatalf("event seq = %d, %d; want 1, 2", event1.Seq, event2.Seq)
	}
	longOutput := strings.Repeat("x", 3000)
	slimmedEvent, err := svc.AppendEvent(ctx, AppendRunEventInput{
		RunID:    run.ID,
		Type:     AgentRunEventToolFinished,
		Message:  longOutput,
		ToolName: "Bash",
		Payload: map[string]any{
			"call_id":  "call-1",
			"is_error": false,
			"output":   longOutput,
		},
	})
	if err != nil {
		t.Fatalf("AppendEvent slimmedEvent: %v", err)
	}
	if len(slimmedEvent.Message) >= len(longOutput) {
		t.Fatalf("event message was not slimmed: got %d bytes, want < %d", len(slimmedEvent.Message), len(longOutput))
	}
	var slimmedPayload map[string]any
	if err := json.Unmarshal(slimmedEvent.Payload, &slimmedPayload); err != nil {
		t.Fatalf("slimmed payload json: %v", err)
	}
	if slimmedPayload["call_id"] != "call-1" || slimmedPayload["is_error"] != false {
		t.Fatalf("slimmed payload lost metadata: %+v", slimmedPayload)
	}
	output, _ := slimmedPayload["output"].(string)
	if len(output) >= len(longOutput) {
		t.Fatalf("payload output was not slimmed: got %d bytes, want < %d", len(output), len(longOutput))
	}

	updated, err := svc.UpdateStatus(ctx, UpdateRunStatusInput{
		RunID:            run.ID,
		Status:           AgentRunStatusRunning,
		ActivityText:     "Bash: npm test",
		ToolName:         "Bash",
		ToolInputSummary: "Bash: npm test",
	})
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != AgentRunStatusRunning || updated.ToolName != "Bash" {
		t.Fatalf("updated run = %+v", updated)
	}

	finished, err := svc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       AgentRunStatusCompleted,
		ActivityText: "已完成",
	})
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if finished.Status != AgentRunStatusCompleted {
		t.Fatalf("finished status = %q, want %q", finished.Status, AgentRunStatusCompleted)
	}
	if finished.FinishedAt == nil {
		t.Fatal("finished_at is nil")
	}
	finishedAt := *finished.FinishedAt
	lateUpdate, err := svc.UpdateStatus(ctx, UpdateRunStatusInput{
		RunID:        run.ID,
		Status:       AgentRunStatusRunning,
		ActivityText: "迟到的运行事件",
	})
	if !errors.Is(err, ErrAgentRunAlreadyFinished) {
		t.Fatalf("late UpdateStatus error = %v, want ErrAgentRunAlreadyFinished", err)
	}
	if lateUpdate.Status != AgentRunStatusCompleted || lateUpdate.ActivityText != "已完成" {
		t.Fatalf("late update changed terminal run: %+v", lateUpdate)
	}
	lateFinish, err := svc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       AgentRunStatusTimeout,
		ActivityText: "迟到的超时",
	})
	if !errors.Is(err, ErrAgentRunAlreadyFinished) {
		t.Fatalf("late FinishRun error = %v, want ErrAgentRunAlreadyFinished", err)
	}
	if lateFinish.Status != AgentRunStatusCompleted || lateFinish.FinishedAt == nil || !lateFinish.FinishedAt.Equal(finishedAt) {
		t.Fatalf("late finisher replaced first terminal state: %+v", lateFinish)
	}
	if _, err := svc.MarkBackendStarted(ctx, run.ID); err == nil {
		t.Fatal("MarkBackendStarted revived a terminal run")
	}
	lateExternalID := "late-session-" + uuid.NewString()
	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: run.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: lateExternalID,
	}); !errors.Is(err, ErrAgentRunAlreadyFinished) {
		t.Fatalf("late BindProviderSession error = %v, want ErrAgentRunAlreadyFinished", err)
	}
	var lateSessionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_sessions WHERE agent_id=$1 AND external_session_id=$2`, agentID, lateExternalID).Scan(&lateSessionCount); err != nil {
		t.Fatal(err)
	}
	if lateSessionCount != 0 {
		t.Fatalf("late BindProviderSession created %d active Sessions", lateSessionCount)
	}
	recentRuns, err := svc.ListRecentRuns(ctx)
	if err != nil {
		t.Fatalf("ListRecentRuns: %v", err)
	}
	if !agentRunListContains(recentRuns, run.ID) {
		t.Fatalf("ListRecentRuns did not include completed run %s", run.ID)
	}

	failedRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		SessionID:    session.ID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusQueued,
		ActivityText: "等待执行",
	})
	if err != nil {
		t.Fatalf("StartRun failedRun: %v", err)
	}
	if _, err := svc.FinishRun(ctx, FinishRunInput{
		RunID:        failedRun.ID,
		Status:       AgentRunStatusFailed,
		ActivityText: "执行失败",
	}); err != nil {
		t.Fatalf("FinishRun failedRun: %v", err)
	}
	if err := svc.LinkTask(ctx, LinkRunTaskInput{
		RunID:  failedRun.ID,
		TaskID: taskID,
		Role:   AgentRunTaskRolePrimary,
	}); err != nil {
		t.Fatalf("LinkTask failedRun: %v", err)
	}
	taskRuns, err := svc.ListRunsByTask(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRunsByTask: %v", err)
	}
	if len(taskRuns) != 1 || taskRuns[0].ID != run.ID {
		t.Fatalf("ListRunsByTask = %+v, want only completed run %s", taskRuns, run.ID)
	}
	taskSummaries, err := svc.ListAgentTasks(ctx, agentID)
	if err != nil {
		t.Fatalf("ListAgentTasks: %v", err)
	}
	var foundTask *AgentTaskSummary
	for i := range taskSummaries {
		if taskSummaries[i].ID == taskID {
			foundTask = &taskSummaries[i]
			break
		}
	}
	if foundTask == nil {
		t.Fatal("ListAgentTasks did not include linked task")
	}
	if foundTask.LinkedRunCount != 1 || foundTask.LastRunID != run.ID {
		t.Fatalf("task summary = %+v, want linked_run_count=1 last_run_id=%s", foundTask, run.ID)
	}
	activeRuns, err = svc.ListActiveRuns(ctx)
	if err != nil {
		t.Fatalf("ListActiveRuns: %v", err)
	}
	for _, active := range activeRuns {
		if active.ID == failedRun.ID {
			t.Fatalf("failed run %s should not be listed as active", failedRun.ID)
		}
	}
}

func TestForceFreshRecoveryRetiresPreviousSession(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	oldSession, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: "recovery-old-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create previous Session: %v", err)
	}
	if _, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, SessionID: oldSession.ID, TriggerType: AgentRunTriggerMessage,
		ChannelID: channelID, Status: AgentRunStatusFailed, Source: "codex",
	}); err != nil {
		t.Fatalf("start failed Run: %v", err)
	}
	recoveryRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: channelID,
		Status: AgentRunStatusQueued, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start recovery Run: %v", err)
	}
	dispatch, err := svc.ResolveSessionDispatch(ctx, ResolveSessionDispatchInput{
		RunID: recoveryRun.ID, AgentID: agentID, ChannelID: channelID, Provider: "codex",
		ResumeSessionID: oldSession.ExternalSessionID, ForceFreshSession: true, SupportsContextRollover: true,
	})
	if err != nil {
		t.Fatalf("resolve fresh recovery: %v", err)
	}
	if !dispatch.ForceFreshSession || dispatch.RetireSessionID != oldSession.ExternalSessionID || dispatch.RolloverFromSessionID != oldSession.ID {
		t.Fatalf("fresh recovery dispatch = %+v", dispatch)
	}
	var oldStatus, persistedIntent string
	if err := pool.QueryRow(ctx, `
		SELECT s.status, COALESCE(r.rollover_from_session_id::text, '')
		  FROM agent_sessions s JOIN agent_runs r ON r.id = $2
		 WHERE s.id = $1`, oldSession.ID, recoveryRun.ID,
	).Scan(&oldStatus, &persistedIntent); err != nil {
		t.Fatalf("load pending recovery state: %v", err)
	}
	if oldStatus != AgentSessionStatusRolloverPending || persistedIntent != oldSession.ID {
		t.Fatalf("pending recovery state = status %q intent %q", oldStatus, persistedIntent)
	}

	newExternalID := "recovery-new-" + uuid.NewString()
	bound, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: recoveryRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil {
		t.Fatalf("bind fresh recovery Session: %v", err)
	}
	if bound.RolloverEvent == nil || bound.Session.Status != AgentSessionStatusActive {
		t.Fatalf("bound fresh recovery = %+v", bound)
	}
	var activeCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_sessions WHERE agent_id = $1 AND status = $2`, agentID, AgentSessionStatusActive).Scan(&activeCount); err != nil {
		t.Fatalf("count active Sessions: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_sessions WHERE id = $1`, oldSession.ID).Scan(&oldStatus); err != nil {
		t.Fatalf("load retired Session: %v", err)
	}
	if activeCount != 1 || oldStatus != AgentSessionStatusClosed {
		t.Fatalf("completed recovery = active %d old status %q", activeCount, oldStatus)
	}

	replayedRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: channelID,
		Status: AgentRunStatusQueued, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start replayed recovery Run: %v", err)
	}
	replayed, err := svc.ResolveSessionDispatch(ctx, ResolveSessionDispatchInput{
		RunID: replayedRun.ID, AgentID: agentID, ChannelID: channelID, Provider: "codex",
		ResumeSessionID: oldSession.ExternalSessionID, ForceFreshSession: true, SupportsContextRollover: true,
	})
	if err != nil {
		t.Fatalf("resolve replayed recovery: %v", err)
	}
	if replayed.ResumeSessionID != newExternalID || replayed.ForceFreshSession || replayed.RolloverFromSessionID != "" {
		t.Fatalf("replayed recovery dispatch = %+v", replayed)
	}
}

func TestAgentRunSessionRolloverConvergesConcurrentDispatches(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	oldSession, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: "rollover-old-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create old Session: %v", err)
	}
	sourceRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, SessionID: oldSession.ID, TriggerType: AgentRunTriggerMessage,
		ChannelID: channelID, Status: AgentRunStatusRunning, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start source Run: %v", err)
	}
	currentDispatch, err := svc.ResolveSessionDispatch(ctx, ResolveSessionDispatchInput{
		RunID: sourceRun.ID, AgentID: agentID, ChannelID: channelID, Provider: "codex",
	})
	if err != nil {
		t.Fatalf("resolve current Run Session: %v", err)
	}
	if currentDispatch.ResumeSessionID != oldSession.ExternalSessionID || currentDispatch.ColdStart || currentDispatch.ForceFreshSession {
		t.Fatalf("current Run dispatch = %+v, want its active Session", currentDispatch)
	}

	peerSourceRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, SessionID: oldSession.ID, TriggerType: AgentRunTriggerMessage,
		ChannelID: channelID, Status: AgentRunStatusRunning, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start peer source Run: %v", err)
	}
	type rolloverCall struct {
		result *RequestSessionRolloverResult
		err    error
	}
	calls := make(chan rolloverCall, 2)
	for _, runID := range []string{sourceRun.ID, peerSourceRun.ID} {
		go func() {
			result, err := svc.RequestSessionRollover(ctx, RequestSessionRolloverInput{
				RunID: runID, Reason: "ineffective_compaction", Continuity: "continue task",
			})
			calls <- rolloverCall{result: result, err: err}
		}()
	}
	createdRequests := 0
	requestOwnerID := ""
	for range 2 {
		call := <-calls
		if call.err != nil {
			t.Fatalf("concurrent rollover request: %v", call.err)
		}
		if call.result.SessionID != oldSession.ID {
			t.Fatalf("concurrent rollover Session = %q, want %q", call.result.SessionID, oldSession.ID)
		}
		if call.result.Created {
			createdRequests++
			if call.result.Event == nil {
				t.Fatal("created rollover request has no event")
			}
			requestOwnerID = call.result.Event.RunID
		}
	}
	if createdRequests != 1 {
		t.Fatalf("created rollover requests = %d, want 1", createdRequests)
	}
	winnerRun, replayRun := sourceRun, peerSourceRun
	if requestOwnerID == peerSourceRun.ID {
		winnerRun, replayRun = peerSourceRun, sourceRun
	}
	pending, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: oldSession.ExternalSessionID,
	})
	if err != nil || pending.ID != oldSession.ID || pending.Status != AgentSessionStatusRolloverPending {
		t.Fatalf("pending Session upsert = %+v, %v", pending, err)
	}
	replayedRequest, err := svc.RequestSessionRollover(ctx, RequestSessionRolloverInput{RunID: sourceRun.ID})
	if err != nil || replayedRequest.Created || replayedRequest.Event != nil {
		t.Fatalf("pending rollover replay = %+v, %v", replayedRequest, err)
	}

	startReplacementRun := func() *AgentRun {
		t.Helper()
		run, err := svc.StartRun(ctx, StartRunInput{
			AgentID: agentID, TriggerType: AgentRunTriggerMessage, ChannelID: channelID,
			Status: AgentRunStatusQueued, Source: "codex",
		})
		if err != nil {
			t.Fatalf("start replacement Run: %v", err)
		}
		return run
	}
	firstRun := startReplacementRun()
	secondRun := startReplacementRun()
	resolve := func(runID string) SessionDispatch {
		t.Helper()
		dispatch, err := svc.ResolveSessionDispatch(ctx, ResolveSessionDispatchInput{
			RunID: runID, AgentID: agentID, ChannelID: channelID, Provider: "codex",
			SupportsContextRollover: true,
		})
		if err != nil {
			t.Fatalf("resolve Run %s: %v", runID, err)
		}
		return dispatch
	}
	for _, run := range []*AgentRun{firstRun, secondRun} {
		dispatch := resolve(run.ID)
		if !dispatch.ForceFreshSession || dispatch.RetireSessionID != oldSession.ExternalSessionID || dispatch.RolloverFromSessionID != oldSession.ID {
			t.Fatalf("initial dispatch for %s = %+v", run.ID, dispatch)
		}
	}
	pendingCurrent := resolve(winnerRun.ID)
	if !pendingCurrent.ForceFreshSession || pendingCurrent.RetireSessionID != oldSession.ExternalSessionID || pendingCurrent.RolloverFromSessionID != oldSession.ID {
		t.Fatalf("current pending dispatch = %+v", pendingCurrent)
	}
	var peerSessionID, peerIntent string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(session_id::text, ''), COALESCE(rollover_from_session_id::text, '') FROM agent_runs WHERE id = $1`, winnerRun.ID).Scan(&peerSessionID, &peerIntent); err != nil {
		t.Fatalf("load pending current Run: %v", err)
	}
	if peerSessionID != oldSession.ID || peerIntent != oldSession.ID {
		t.Fatalf("pending current Run session=%q rollover=%q, want old Session", peerSessionID, peerIntent)
	}

	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: winnerRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: oldSession.ExternalSessionID,
	}); !errors.Is(err, ErrSessionRolloverMismatch) {
		t.Fatalf("fresh bind reused predecessor: %v", err)
	}
	newExternalID := "rollover-new-" + uuid.NewString()
	bound, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: winnerRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil {
		t.Fatalf("bind current pending replacement: %v", err)
	}
	if bound.RolloverEvent == nil || bound.Run.SessionID != bound.Session.ID || bound.Session.Status != AgentSessionStatusActive {
		t.Fatalf("current pending replacement bind = %+v", bound)
	}
	replacementID := bound.Session.ID
	directLoserBind, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: firstRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil {
		t.Fatalf("bind directly accepted loser to winner: %v", err)
	}
	if directLoserBind.RolloverEvent != nil || directLoserBind.Run.SessionID != replacementID || directLoserBind.Run.RolloverFromSessionID != "" {
		t.Fatalf("direct loser convergence = %+v", directLoserBind)
	}
	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: secondRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: "other-winner-" + uuid.NewString(),
	}); !errors.Is(err, ErrSessionRolloverMismatch) {
		t.Fatalf("direct loser accepted a different provider Session: %v", err)
	}

	replayedRequest, err = svc.RequestSessionRollover(ctx, RequestSessionRolloverInput{RunID: replayRun.ID})
	if err != nil || replayedRequest.Created || replayedRequest.Event != nil {
		t.Fatalf("closed rollover replay = %+v, %v", replayedRequest, err)
	}
	for _, run := range []*AgentRun{firstRun, secondRun} {
		converged := resolve(run.ID)
		if converged.ResumeSessionID != newExternalID || converged.ForceFreshSession || converged.RolloverFromSessionID != "" {
			t.Fatalf("converged dispatch for %s = %+v, want active replacement", run.ID, converged)
		}
		var persistedIntent string
		if err := pool.QueryRow(ctx, `SELECT COALESCE(rollover_from_session_id::text, '') FROM agent_runs WHERE id = $1`, run.ID).Scan(&persistedIntent); err != nil {
			t.Fatalf("load converged Run: %v", err)
		}
		if persistedIntent != "" {
			t.Fatalf("converged Run %s rollover_from_session_id = %q, want cleared", run.ID, persistedIntent)
		}
	}

	secondBound, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: secondRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil {
		t.Fatalf("bind converged Run: %v", err)
	}
	if secondBound.Run.SessionID != replacementID || secondBound.RolloverEvent != nil {
		t.Fatalf("converged bind = %+v", secondBound)
	}
	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: secondRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: "conflict-" + uuid.NewString(),
	}); !errors.Is(err, ErrSessionRolloverMismatch) {
		t.Fatalf("conflicting replay bind: %v", err)
	}

	var sessionCount, requestedCount, completedCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_sessions WHERE agent_id = $1`, agentID).Scan(&sessionCount); err != nil {
		t.Fatalf("count Sessions: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_run_events WHERE type = $1 AND run_id = ANY($2::uuid[])`, AgentRunEventSessionRolloverRequested, []string{sourceRun.ID, peerSourceRun.ID}).Scan(&requestedCount); err != nil {
		t.Fatalf("count requested events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_run_events WHERE type = $1 AND run_id = ANY($2::uuid[])`, AgentRunEventSessionRolloverCompleted, []string{sourceRun.ID, peerSourceRun.ID, firstRun.ID, secondRun.ID}).Scan(&completedCount); err != nil {
		t.Fatalf("count completed events: %v", err)
	}
	if sessionCount != 2 || requestedCount != 1 || completedCount != 1 {
		t.Fatalf("persisted rollover state: sessions=%d requested=%d completed=%d", sessionCount, requestedCount, completedCount)
	}

	closedWithWinnerRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, SessionID: oldSession.ID, TriggerType: AgentRunTriggerMessage,
		ChannelID: channelID, Status: AgentRunStatusQueued, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start closed current Run with winner: %v", err)
	}
	closedWithWinner := resolve(closedWithWinnerRun.ID)
	if closedWithWinner.ResumeSessionID != newExternalID || closedWithWinner.ColdStart || closedWithWinner.ForceFreshSession {
		t.Fatalf("closed current dispatch with winner = %+v", closedWithWinner)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(session_id::text, '') FROM agent_runs WHERE id = $1`, closedWithWinnerRun.ID).Scan(&peerSessionID); err != nil {
		t.Fatalf("load closed current Run with winner: %v", err)
	}
	if peerSessionID != "" {
		t.Fatalf("closed current Run retained Session %q", peerSessionID)
	}
	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: closedWithWinnerRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	}); err != nil {
		t.Fatalf("bind closed current Run to winner: %v", err)
	}

	orphanClosed, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: "orphan-closed-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create orphan closed Session: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_sessions SET status = $2 WHERE id = $1`, orphanClosed.ID, AgentSessionStatusClosed); err != nil {
		t.Fatalf("close orphan Session: %v", err)
	}
	orphanClosed, err = svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: orphanClosed.ExternalSessionID,
	})
	if err != nil || orphanClosed.Status != AgentSessionStatusClosed {
		t.Fatalf("orphan closed Session upsert = %+v, %v", orphanClosed, err)
	}
	closedWithoutWinnerRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID: agentID, SessionID: orphanClosed.ID, TriggerType: AgentRunTriggerMessage,
		ChannelID: channelID, Status: AgentRunStatusQueued, Source: "codex",
	})
	if err != nil {
		t.Fatalf("start closed current Run without winner: %v", err)
	}
	closedWithoutWinner := resolve(closedWithoutWinnerRun.ID)
	if !closedWithoutWinner.ColdStart || closedWithoutWinner.ResumeSessionID != "" || closedWithoutWinner.ForceFreshSession || closedWithoutWinner.RolloverFromSessionID != "" {
		t.Fatalf("closed current dispatch without winner = %+v", closedWithoutWinner)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(session_id::text, '') FROM agent_runs WHERE id = $1`, closedWithoutWinnerRun.ID).Scan(&peerSessionID); err != nil {
		t.Fatalf("load closed current Run without winner: %v", err)
	}
	if peerSessionID != "" {
		t.Fatalf("closed current Run without winner retained Session %q", peerSessionID)
	}
	if _, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: closedWithoutWinnerRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: "orphan-new-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("bind cold-started Run after closed Session: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE agent_sessions SET status = $2 WHERE id = $1`, replacementID, AgentSessionStatusClosed); err != nil {
		t.Fatalf("close replacement: %v", err)
	}
	closed, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil || closed.Status != AgentSessionStatusClosed {
		t.Fatalf("closed Session upsert = %+v, %v", closed, err)
	}
	replayed, err := svc.BindProviderSession(ctx, BindProviderSessionInput{
		RunID: winnerRun.ID, AgentID: agentID, Provider: "codex", ExternalSessionID: newExternalID,
	})
	if err != nil || replayed.RolloverEvent != nil || replayed.Session.Status != AgentSessionStatusClosed {
		t.Fatalf("late completed bind replay = %+v, %v", replayed, err)
	}
}

func TestResolveExecutingThinkingNodeFailsClosed(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	spaceID := uuid.NewString()
	nodeID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM thinking_spaces WHERE id = $1`, spaceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})
	_, err := pool.Exec(ctx,
		`INSERT INTO thinking_spaces (id, channel_id, created_by) VALUES ($1, $2, $3)`,
		spaceID, channelID, ownerID)
	if err != nil {
		t.Fatalf("create Thinking space: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO thinking_nodes (id, space_id, agent_id, title, source, created_by)
		VALUES ($1, $2, $3, 'Root', 'root', $4)`, nodeID, spaceID, agentID, ownerID)
	if err != nil {
		t.Fatalf("create Thinking node: %v", err)
	}
	svc := NewAgentRunService(pool)
	nodeRun, err := svc.StartRun(ctx, StartRunInput{
		AgentID:        agentID,
		TriggerType:    AgentRunTriggerMessage,
		ChannelID:      channelID,
		ThinkingNodeID: nodeID,
		Status:         AgentRunStatusQueued,
	})
	if err != nil {
		t.Fatalf("start queued node run: %v", err)
	}
	if got, err := svc.ResolveExecutingThinkingNode(ctx, agentID, channelID); err != nil || got != "" {
		t.Fatalf("queued scope = %q, %v, want empty", got, err)
	}
	if _, err := svc.UpdateStatus(ctx, UpdateRunStatusInput{RunID: nodeRun.ID, Status: AgentRunStatusRunning}); err != nil {
		t.Fatalf("mark node run executing: %v", err)
	}
	if got, err := svc.ResolveExecutingThinkingNode(ctx, agentID, channelID); err != nil || got != nodeID {
		t.Fatalf("executing scope = %q, %v, want %q", got, err, nodeID)
	}

	if _, err := svc.StartRun(ctx, StartRunInput{
		AgentID:     agentID,
		TriggerType: AgentRunTriggerMessage,
		ChannelID:   channelID,
		Status:      AgentRunStatusRunning,
	}); err != nil {
		t.Fatalf("start conflicting channel run: %v", err)
	}
	if _, err := svc.ResolveExecutingThinkingNode(ctx, agentID, channelID); !errors.Is(err, ErrAmbiguousAgentRunScope) {
		t.Fatalf("ambiguous scope error = %v, want %v", err, ErrAmbiguousAgentRunScope)
	}
}

func TestListActiveRunsForUserFiltersVisibility(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	otherID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	otherAgentID := agentRunAgent(t, pool, otherID)
	channelID := agentRunChannel(t, pool, ownerID)
	otherChannelID := agentRunChannel(t, pool, otherID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = ANY($1)`, []string{agentID, otherAgentID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = ANY($1)`, []string{channelID, otherChannelID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1)`, []string{agentID, otherAgentID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{ownerID, otherID})
	})

	svc := NewAgentRunService(pool)
	visible, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusThinking,
		ActivityText: "visible",
	})
	if err != nil {
		t.Fatalf("StartRun visible: %v", err)
	}
	hidden, err := svc.StartRun(ctx, StartRunInput{
		AgentID:      otherAgentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    otherChannelID,
		Status:       AgentRunStatusThinking,
		ActivityText: "hidden",
	})
	if err != nil {
		t.Fatalf("StartRun hidden: %v", err)
	}

	runs, err := svc.ListActiveRunsForUser(ctx, ownerID)
	if err != nil {
		t.Fatalf("ListActiveRunsForUser: %v", err)
	}
	if !agentRunListContains(runs, visible.ID) {
		t.Fatalf("visible run %s missing from %+v", visible.ID, runs)
	}
	if agentRunListContains(runs, hidden.ID) {
		t.Fatalf("hidden run %s leaked into %+v", hidden.ID, runs)
	}
}

func TestParseOpenClawMessageTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"type":"message","timestamp":"2026-06-28T08:56:32Z","message":{"role":"user","content":"hello from openclaw"}}`))
	if len(entries) != 1 || entries[0].Role != "user" || entries[0].Text != "hello from openclaw" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCodexPayloadTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello from codex"}]}}`))
	if len(entries) != 1 || entries[0].Role != "user" || entries[0].Text != "hello from codex" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCodexPayloadSkipsDeveloperMessages(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"hidden instructions"}]}}`))
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want no visible transcript entries", entries)
	}
}

func TestParseCodexToolPayload(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"timestamp":"2026-06-28T08:55:55Z","type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"call-1","arguments":"{\"cmd\":\"go test\"}"}}`))
	if len(entries) != 1 || entries[0].Type != "tool_use" || entries[0].ToolName != "shell" || entries[0].ToolID != "call-1" || entries[0].Text == "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseOpenClawTrajectoryTranscriptLine(t *testing.T) {
	entries := parseTranscriptLine(json.RawMessage(`{"type":"prompt.submitted","ts":"2026-06-28T08:56:32Z","data":{"prompt":"hello from trajectory"}}`))
	if len(entries) != 1 || entries[0].Text != "hello from trajectory" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGetRunTranscriptResolvesProviderPathWhenRunPathMissing(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	codexHome := filepath.Join(t.TempDir(), ".codex")
	externalID := "codex-session-1"
	path := filepath.Join(codexHome, "sessions", "2026", "06", "28", "rollout-2026-06-28T12-00-00-"+externalID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"live transcript"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)

	svc := NewAgentRunService(pool)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "codex",
		ExternalSessionID: externalID,
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:     agentID,
		SessionID:   session.ID,
		TriggerType: AgentRunTriggerManual,
		ChannelID:   channelID,
		Status:      AgentRunStatusRunning,
		Source:      "codex",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	entries, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "live transcript" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGetRunTranscriptReadsHermesStateDBDirectly(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, ".hermes", "state.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	sql := `
CREATE TABLE sessions (id TEXT PRIMARY KEY);
CREATE TABLE messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT,
  tool_name TEXT,
  tool_calls TEXT,
  reasoning TEXT,
  reasoning_content TEXT,
  timestamp REAL NOT NULL,
  active INTEGER NOT NULL DEFAULT 1
);
INSERT INTO sessions (id) VALUES ('hermes-live');
INSERT INTO messages (session_id, role, content, timestamp) VALUES ('hermes-live', 'assistant', 'from state db', 1760000000.5);
`
	cmd := exec.Command("sqlite3", dbPath, sql)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, output)
	}

	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	svc := NewAgentRunService(pool)
	session, err := svc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentID,
		Provider:          "hermes",
		ExternalSessionID: "hermes-live",
		TranscriptPath:    filepath.Join(home, ".solo", "hermes-transcripts", "hermes-live.jsonl"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	run, err := svc.StartRun(ctx, StartRunInput{
		AgentID:     agentID,
		SessionID:   session.ID,
		TriggerType: AgentRunTriggerManual,
		ChannelID:   channelID,
		Status:      AgentRunStatusRunning,
		Source:      "hermes",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE agent_runs SET started_at = $2, updated_at = $2 WHERE id = $1`, run.ID, time.Unix(1760000000, 0).UTC()); err != nil {
		t.Fatalf("set run time: %v", err)
	}

	entries, err := svc.GetRunTranscript(ctx, run.ID, 10)
	if err != nil {
		t.Fatalf("GetRunTranscript: %v", err)
	}
	if len(entries) != 1 || entries[0].Text != "from state db" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestAgentRunVisibleMessageDelivery(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentID,
		TriggerType:  AgentRunTriggerMessage,
		ChannelID:    channelID,
		Status:       AgentRunStatusRunning,
		ActivityText: "执行中",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	scope, err := runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, "", "")
	if err != nil || scope == nil || scope.ThreadID != "" {
		t.Fatalf("ResolveMessageDeliveryScope correct scope = %+v, %v", scope, err)
	}
	scope, err = runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, uuid.NewString(), "", "")
	if err != nil || scope != nil {
		t.Fatalf("ResolveMessageDeliveryScope other scope = %+v, %v", scope, err)
	}
	if _, err := runSvc.ResolveMessageDeliveryScope(ctx, run.ID, uuid.NewString(), channelID, "", ""); err == nil {
		t.Fatal("ResolveMessageDeliveryScope accepted another agent")
	}
	otherChannelID := agentRunChannel(t, pool, ownerID)
	if otherChannelID == channelID {
		otherChannelID = uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`, otherChannelID, "agent-run-other-"+otherChannelID[:8], ownerID); err != nil {
			t.Fatalf("create other channel: %v", err)
		}
	}
	otherRootMessageID := agentRunMessage(t, pool, otherChannelID, ownerID)
	otherThreadID, _, err := NewThreadService(pool).GetOrCreateThread(ctx, otherChannelID, otherRootMessageID)
	if err != nil {
		t.Fatalf("GetOrCreateThread other channel: %v", err)
	}
	scope, err = runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, otherThreadID, "")
	if err != nil || scope != nil {
		t.Fatalf("ResolveMessageDeliveryScope foreign thread = %+v, %v", scope, err)
	}

	visible, err := runSvc.HasVisibleMessage(ctx, run.ID)
	if err != nil || visible {
		t.Fatalf("HasVisibleMessage before insert = %v, %v", visible, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, metadata)
		VALUES ($1, $2, 'agent', $3, 'delivered', jsonb_build_object('agent_run_id', $4::text, 'delivery', 'visible'))`,
		uuid.NewString(), channelID, agentID, run.ID,
	)
	if err != nil {
		t.Fatalf("insert visible message: %v", err)
	}
	visible, err = runSvc.HasVisibleMessage(ctx, run.ID)
	if err != nil || !visible {
		t.Fatalf("HasVisibleMessage after insert = %v, %v", visible, err)
	}
}

func TestAgentRunCoalescedWakeThreadCountsAsVisibleDelivery(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	firstID := agentRunMessage(t, pool, channelID, ownerID)
	latestID := agentRunMessage(t, pool, channelID, ownerID)
	unrelatedID := agentRunMessage(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	var firstSeq, latestSeq int64
	if err := pool.QueryRow(ctx, `SELECT min(seq), max(seq) FROM messages WHERE id = ANY($1::uuid[])`, []string{firstID, latestID}).Scan(&firstSeq, &latestSeq); err != nil {
		t.Fatalf("load wake range: %v", err)
	}
	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, TriggerMessageID: latestID,
		ChannelID: channelID, Status: AgentRunStatusRunning, WakeFirstSeq: firstSeq, WakeLatestSeq: latestSeq,
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	firstThreadID, _, err := NewThreadService(pool).GetOrCreateThread(ctx, channelID, firstID)
	if err != nil {
		t.Fatalf("create first wake Thread: %v", err)
	}
	unrelatedThreadID, _, err := NewThreadService(pool).GetOrCreateThread(ctx, channelID, unrelatedID)
	if err != nil {
		t.Fatalf("create unrelated Thread: %v", err)
	}

	scope, err := runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, firstThreadID, "")
	if err != nil || scope == nil || scope.ThreadID != "" {
		t.Fatalf("coalesced wake Thread scope = %+v, %v", scope, err)
	}
	scope, err = runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, unrelatedThreadID, "")
	if err != nil || scope != nil {
		t.Fatalf("unrelated Thread scope = %+v, %v", scope, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, thread_id, sender_type, sender_id, content, metadata)
		VALUES ($1, $2, $3, 'agent', $4, 'coalesced delivery', jsonb_build_object('agent_run_id', $5::text))`,
		uuid.NewString(), channelID, firstThreadID, agentID, run.ID,
	); err != nil {
		t.Fatalf("insert coalesced delivery: %v", err)
	}
	visible, err := runSvc.HasVisibleMessage(ctx, run.ID)
	if err != nil || !visible {
		t.Fatalf("HasVisibleMessage for coalesced wake Thread = %v, %v", visible, err)
	}
}

func TestAgentRunDirectTriggerThreadCountsAsVisibleDelivery(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	agentID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	triggerID := agentRunMessage(t, pool, channelID, ownerID)
	otherRootID := agentRunMessage(t, pool, channelID, ownerID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE created_by = $1`, ownerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	runSvc := NewAgentRunService(pool)
	run, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID: agentID, TriggerType: AgentRunTriggerMessage, TriggerMessageID: triggerID,
		ChannelID: channelID, Status: AgentRunStatusRunning, ActivityText: "执行中",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	var triggerThreadID, unrelatedThreadID string
	if err := pool.QueryRow(ctx, `INSERT INTO threads (channel_id, root_message_id) VALUES ($1, $2) RETURNING id`, channelID, triggerID).Scan(&triggerThreadID); err != nil {
		t.Fatalf("create trigger thread: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO threads (channel_id, root_message_id) VALUES ($1, $2) RETURNING id`, channelID, otherRootID).Scan(&unrelatedThreadID); err != nil {
		t.Fatalf("create unrelated thread: %v", err)
	}

	scope, err := runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, triggerThreadID, "")
	if err != nil || scope == nil || scope.ThreadID != "" {
		t.Fatalf("direct trigger Thread scope = %+v, %v", scope, err)
	}
	scope, err = runSvc.ResolveMessageDeliveryScope(ctx, run.ID, agentID, channelID, unrelatedThreadID, "")
	if err != nil || scope != nil {
		t.Fatalf("unrelated Thread scope = %+v, %v", scope, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, thread_id, sender_type, sender_id, content, metadata)
		VALUES ($1, $2, $3, 'agent', $4, 'thread delivery', jsonb_build_object('agent_run_id', $5::text))`,
		uuid.NewString(), channelID, triggerThreadID, agentID, run.ID,
	); err != nil {
		t.Fatalf("insert Thread delivery: %v", err)
	}
	visible, err := runSvc.HasVisibleMessage(ctx, run.ID)
	if err != nil || !visible {
		t.Fatalf("HasVisibleMessage for direct trigger Thread = %v, %v", visible, err)
	}
}

func agentRunListContains(runs []AgentRun, id string) bool {
	for _, run := range runs {
		if run.ID == id {
			return true
		}
	}
	return false
}

func agentRunTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func agentRunUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	email := fmt.Sprintf("agent-run-%s@example.test", id)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, $3, 'test')`,
		id, email, "Agent Run Tester",
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE channels SET is_archived=true WHERE created_by=$1 AND name LIKE 'agent-run-home-%'`, id)
		_, _ = pool.Exec(context.Background(), `UPDATE users SET is_active=false WHERE id=$1`, id)
	})
	return id
}

func agentRunAgent(t *testing.T, pool *pgxpool.Pool, ownerID string) string {
	t.Helper()
	id := uuid.NewString()
	channelID := agentRunChannel(t, pool, ownerID)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO agents (id, name, owner_id, model_name, home_channel_id)
		 VALUES ($1, $2, $3, 'test-model', $4)`,
		id, "agent-run-"+id[:8], ownerID, channelID,
	)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return id
}

func agentRunChannel(t *testing.T, pool *pgxpool.Pool, creatorID string) string {
	t.Helper()
	var existingID string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE((
			SELECT id::text
			  FROM channels
			 WHERE created_by = $1
			   AND type = 'channel'
			   AND name LIKE 'agent-run-home-%'
			 ORDER BY created_at ASC
			 LIMIT 1
		), '')
	`, creatorID).Scan(&existingID); err != nil {
		t.Fatalf("find channel: %v", err)
	}
	if existingID != "" {
		return existingID
	}

	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, created_by) VALUES ($1, $2, $3)`,
		id, "agent-run-home-"+id[:8], creatorID,
	)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return id
}

func agentRunMessage(t *testing.T, pool *pgxpool.Pool, channelID, senderID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content) VALUES ($1, $2, 'user', $3, 'please work')`,
		id, channelID, senderID,
	)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	return id
}

func agentRunTask(t *testing.T, pool *pgxpool.Pool, channelID, creatorID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tasks (id, channel_id, creator_id, title, status, priority, task_number)
		 VALUES ($1, $2, $3, 'agent-run-test', $4, 'normal',
		   (SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE channel_id = $2))`,
		id, channelID, creatorID, TaskStatusTodo,
	)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}

func agentRunTranscriptFile(t *testing.T) string {
	return agentRunTranscriptFileWithText(t, "hello from transcript")
}

func agentRunTranscriptFileWithText(t *testing.T, text string) string {
	t.Helper()
	path := t.TempDir() + "/session.jsonl"
	raw := fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":%q}}`+"\n", time.Now().UTC().Format(time.RFC3339), text)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}

func agentRunTranscriptFileWithUsage(t *testing.T, text string, inputTokens, outputTokens int) string {
	t.Helper()
	path := t.TempDir() + "/session.jsonl"
	raw := fmt.Sprintf(
		`{"type":"user","timestamp":%q,"message":{"content":%q,"usage":{"input_tokens":%d,"output_tokens":%d}}}`+"\n",
		time.Now().UTC().Format(time.RFC3339), text, inputTokens, outputTokens,
	)
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}

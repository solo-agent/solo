package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTaskTrajectoryExportUsesStoredMetadataWithoutInternalContent(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	otherUserID := agentRunUser(t, pool)
	agentAID := agentRunAgent(t, pool, ownerID)
	agentBID := agentRunAgent(t, pool, ownerID)
	channelID := agentRunChannel(t, pool, ownerID)
	otherChannelID := agentRunChannel(t, pool, otherUserID)
	if _, err := pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'user', $2, 'owner')`, channelID, ownerID); err != nil {
		t.Fatalf("add channel owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE agent_id = ANY($1::uuid[])`, []string{agentAID, agentBID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_sessions WHERE agent_id = ANY($1::uuid[])`, []string{agentAID, agentBID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, otherChannelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, otherChannelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agents WHERE id = ANY($1::uuid[])`, []string{agentAID, agentBID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{ownerID, otherUserID})
	})

	taskSvc := NewTaskService(pool)
	root, err := taskSvc.CreateTask(ctx, channelID, ownerID, TaskCreateRequest{
		Title:       "Prepare release notes",
		Description: "Summarize the ordinary release work",
	})
	if err != nil {
		t.Fatalf("create root task: %v", err)
	}
	child, err := taskSvc.CreateTask(ctx, channelID, ownerID, TaskCreateRequest{
		Title:        "Check changelog",
		ParentTaskID: root.ID,
	})
	if err != nil {
		t.Fatalf("create child task: %v", err)
	}
	crossChannelTaskID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, channel_id, creator_id, title, status, priority, task_number, parent_task_id)
		VALUES ($1, $2, $3, 'cross-channel-secret-task', 'todo', 'normal', 1, $4)`,
		crossChannelTaskID, otherChannelID, otherUserID, root.ID); err != nil {
		t.Fatalf("create defensive cross-channel child: %v", err)
	}
	artifactID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, task_id, channel_id, kind, title, html_path, summary, created_by)
		VALUES ($1, $2, $3, 'task_snapshot', 'Release notes', '/private/artifact.html', 'published result', $4)`,
		artifactID, root.ID, channelID, ownerID); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	runSvc := NewAgentRunService(pool)
	session, err := runSvc.UpsertSession(ctx, UpsertSessionInput{
		AgentID:           agentAID,
		Provider:          "codex",
		ExternalSessionID: "trajectory-session",
		TranscriptPath:    agentRunTranscriptFileWithText(t, "reviewed the changelog"),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	completedRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentAID,
		SessionID:    session.ID,
		TriggerType:  AgentRunTriggerTask,
		ChannelID:    channelID,
		Status:       AgentRunStatusRunning,
		ActivityText: "reviewing changelog",
		Source:       "codex",
		Usage:        map[string]any{"input_tokens": 12, "private_context": "secret-usage"},
	})
	if err != nil {
		t.Fatalf("start completed run: %v", err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: completedRun.ID, TaskID: root.ID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatalf("link root task: %v", err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: completedRun.ID, TaskID: child.ID, Role: AgentRunTaskRoleRelated, Confidence: 0.75}); err != nil {
		t.Fatalf("link child task: %v", err)
	}
	actionID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_run_task_actions (id, run_id, task_id, actor_id, action)
		VALUES ($1, $2, $3, $4, 'submit')`, actionID, completedRun.ID, root.ID, agentAID); err != nil {
		t.Fatalf("insert task action: %v", err)
	}
	outboundMessageID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, origin_run_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'agent', $4, 'private outbound content', 'text', now(), now())`,
		outboundMessageID, channelID, completedRun.ID, agentAID); err != nil {
		t.Fatalf("insert outbound message: %v", err)
	}
	firstEvent, err := runSvc.AppendEvent(ctx, AppendRunEventInput{RunID: completedRun.ID, Type: AgentRunEventRunStarted, Message: "started"})
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	secondEvent, err := runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   completedRun.ID,
		Type:    AgentRunEventToolFinished,
		Message: "secret event output",
		Payload: map[string]any{"call_id": "call-1", "is_error": false, "output": "secret-token"},
	})
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}
	if _, err := runSvc.FinishRun(ctx, FinishRunInput{RunID: completedRun.ID, Status: AgentRunStatusCompleted, ActivityText: "done"}); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	runningRun, err := runSvc.StartRun(ctx, StartRunInput{
		AgentID:      agentBID,
		TriggerType:  AgentRunTriggerTask,
		ChannelID:    channelID,
		Status:       AgentRunStatusRunning,
		ActivityText: "drafting notes",
		Source:       "codex",
	})
	if err != nil {
		t.Fatalf("start running run: %v", err)
	}
	if err := runSvc.LinkTask(ctx, LinkRunTaskInput{RunID: runningRun.ID, TaskID: child.ID, Role: AgentRunTaskRolePrimary, Confidence: 1}); err != nil {
		t.Fatalf("link running run: %v", err)
	}

	snapshot, err := NewTaskTrajectoryService(pool).Export(ctx, root.ID, ownerID)
	if err != nil {
		t.Fatalf("export trajectory: %v", err)
	}
	if snapshot.SchemaVersion != TaskTrajectorySchemaVersion || snapshot.RootTaskID != root.ID {
		t.Fatalf("snapshot identity = %+v", snapshot)
	}
	if len(snapshot.Tasks) != 2 || snapshot.Tasks[0].ID != root.ID || snapshot.Tasks[1].ParentTaskID != root.ID {
		t.Fatalf("task tree = %+v", snapshot.Tasks)
	}
	if len(snapshot.Runs) != 2 {
		t.Fatalf("runs = %+v, want 2", snapshot.Runs)
	}

	completed := trajectoryRunByID(t, snapshot.Runs, completedRun.ID)
	if len(completed.TaskLinks) != 2 {
		t.Fatalf("completed task links = %+v, want 2", completed.TaskLinks)
	}
	if len(completed.Events) != 2 || completed.Events[0].ID != firstEvent.ID || completed.Events[1].ID != secondEvent.ID {
		t.Fatalf("ordered events = %+v", completed.Events)
	}
	if !completed.Events[1].ContentOmitted || completed.Events[1].Metadata["call_id"] != "call-1" || completed.Events[1].Metadata["is_error"] != false {
		t.Fatalf("metadata-only event = %+v", completed.Events[1])
	}
	if completed.Transcript.Status != "reference_recorded" || completed.Transcript.TextExported || completed.Transcript.Association != "unverified_time_window" {
		t.Fatalf("completed transcript = %+v", completed.Transcript)
	}
	if completed.Usage["input_tokens"] != int64(12) {
		t.Fatalf("sanitized usage = %+v", completed.Usage)
	}
	running := trajectoryRunByID(t, snapshot.Runs, runningRun.ID)
	if running.Transcript.Status != "missing" {
		t.Fatalf("running transcript = %+v, want missing", running.Transcript)
	}
	if !trajectoryHasWarning(snapshot.Warnings, "run_in_progress", runningRun.ID) || !trajectoryHasWarning(snapshot.Warnings, "transcript_reference_missing", runningRun.ID) {
		t.Fatalf("warnings = %+v", snapshot.Warnings)
	}
	if snapshot.Coverage.ParentRunLinks != "unavailable" || snapshot.Coverage.TranscriptAvailability != "reference_recorded_only_unverified_time_window" || snapshot.Coverage.Completeness != "partial_stored_records" {
		t.Fatalf("coverage = %+v", snapshot.Coverage)
	}
	if snapshot.Coverage.OutboundMessageRuns != "stored_metadata_only" || snapshot.Coverage.TaskLifecycleEvents != "stored_records" {
		t.Fatalf("causal coverage = %+v", snapshot.Coverage)
	}
	if len(snapshot.TaskActions) != 1 || snapshot.TaskActions[0].ID != actionID || snapshot.TaskActions[0].RunID != completedRun.ID {
		t.Fatalf("task actions = %+v", snapshot.TaskActions)
	}
	if len(snapshot.OutboundMessages) != 1 || snapshot.OutboundMessages[0].ID != outboundMessageID || !snapshot.OutboundMessages[0].ContentOmitted {
		t.Fatalf("outbound messages = %+v", snapshot.OutboundMessages)
	}
	if len(snapshot.Artifacts) != 1 || snapshot.Artifacts[0].ID != artifactID || snapshot.Artifacts[0].URL != "/api/v1/artifacts/"+artifactID {
		t.Fatalf("artifacts = %+v", snapshot.Artifacts)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	jsonText := string(raw)
	for _, secret := range []string{"transcript_path", `"raw"`, `"input"`, "reviewed the changelog", "secret event output", "secret-token", "secret-usage", "private outbound content", "/private/artifact.html", "cross-channel-secret-task"} {
		if strings.Contains(jsonText, secret) {
			t.Fatalf("snapshot leaked internal content %q: %s", secret, jsonText)
		}
	}

	if _, err := NewTaskTrajectoryService(pool).Export(ctx, root.ID, otherUserID); !errors.Is(err, ErrTaskNotChannelMember) {
		t.Fatalf("unauthorized export error = %v, want %v", err, ErrTaskNotChannelMember)
	}
}

func TestTaskTrajectoryExportCapsLargeTaskTrees(t *testing.T) {
	pool := agentRunTestPool(t)
	ctx := context.Background()
	ownerID := agentRunUser(t, pool)
	channelID := agentRunChannel(t, pool, ownerID)
	if _, err := pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'user', $2, 'owner')`, channelID, ownerID); err != nil {
		t.Fatalf("add channel owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channel_members WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	root, err := NewTaskService(pool).CreateTask(ctx, channelID, ownerID, TaskCreateRequest{Title: "large trajectory root"})
	if err != nil {
		t.Fatalf("create root task: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, channel_id, creator_id, title, status, priority, task_number, parent_task_id)
		SELECT gen_random_uuid(), $1, $2, 'child-' || n, 'todo', 'normal', n + 1, $3
		  FROM generate_series(1, $4) AS n`, channelID, ownerID, root.ID, maxTrajectoryTasks+5); err != nil {
		t.Fatalf("create child tasks: %v", err)
	}

	snapshot, err := NewTaskTrajectoryService(pool).Export(ctx, root.ID, ownerID)
	if err != nil {
		t.Fatalf("export capped trajectory: %v", err)
	}
	if len(snapshot.Tasks) != maxTrajectoryTasks {
		t.Fatalf("task count = %d, want cap %d", len(snapshot.Tasks), maxTrajectoryTasks)
	}
	if snapshot.Coverage.Completeness != "partial_truncated_stored_records" || !trajectoryHasWarning(snapshot.Warnings, "task_limit_reached", "") {
		t.Fatalf("coverage = %+v, warnings = %+v", snapshot.Coverage, snapshot.Warnings)
	}
}

func TestTaskTrajectoryEventMetadataRejectsUntrustedValues(t *testing.T) {
	metadata, omitted := taskTrajectoryEventMetadata(json.RawMessage(`{
		"call_id":"contains a space",
		"task_id":"not-a-uuid",
		"is_error":true,
		"output":{"secret":"value"},
		"unknown":"secret"
	}`))
	if !omitted {
		t.Fatal("untrusted event fields were not marked omitted")
	}
	if len(metadata) != 1 || metadata["is_error"] != true {
		t.Fatalf("metadata = %+v, want only is_error", metadata)
	}
}

func TestTaskTrajectoryTranscriptOnlyClaimsStoredReference(t *testing.T) {
	finishedAt := time.Now().UTC()
	run := TaskTrajectoryRun{ID: "run-reference", StartedAt: finishedAt.Add(-time.Minute), FinishedAt: &finishedAt}
	warnings := attachTaskTrajectoryTranscriptReference(&run, taskTrajectoryRunSource{referenceRecorded: true}, finishedAt)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if run.Transcript.Status != "reference_recorded" || run.Transcript.TextExported {
		t.Fatalf("transcript = %+v", run.Transcript)
	}
}

func trajectoryRunByID(t *testing.T, runs []TaskTrajectoryRun, id string) TaskTrajectoryRun {
	t.Helper()
	for _, run := range runs {
		if run.ID == id {
			return run
		}
	}
	t.Fatalf("run %s not found in %+v", id, runs)
	return TaskTrajectoryRun{}
}

func trajectoryHasWarning(warnings []TaskTrajectoryWarning, code, runID string) bool {
	for _, warning := range warnings {
		if warning.Code == code && warning.RunID == runID {
			return true
		}
	}
	return false
}

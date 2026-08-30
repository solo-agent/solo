package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/metrics"
)

type channelWakePolicy struct {
	ChannelType         string
	WorkspaceVisibility string
}

func (p channelWakePolicy) suppressesUnmentionedAgentWake() bool {
	return p.ChannelType == "channel" && p.WorkspaceVisibility == "public"
}

func (p channelWakePolicy) requiresVisibleResult(reason wakeRouteReason) bool {
	return reason == wakeReasonExplicitMention || p.ChannelType == "dm" || p.ChannelType == "lucy"
}

func (s *AgentService) getChannelWakePolicy(ctx context.Context, channelID string) (channelWakePolicy, error) {
	var policy channelWakePolicy
	err := s.pool.QueryRow(ctx, `
		SELECT c.type, w.visibility
		  FROM channels c
		  JOIN workspaces w ON w.id = c.workspace_id
		 WHERE c.id = $1 AND c.is_archived = false AND w.deleted_at IS NULL`, channelID,
	).Scan(&policy.ChannelType, &policy.WorkspaceVisibility)
	return policy, err
}

type pendingMessageWake struct {
	AgentID              string
	ChannelID            string
	ThreadID             string
	ScopeKey             string
	FirstMessageSeq      int64
	LatestMessageSeq     int64
	RequiresVisibleReply bool
	TriggerMessageID     string
}

func messageWakeScopeKey(threadID string) string {
	if threadID == "" {
		return "channel"
	}
	return "thread:" + threadID
}

func positiveMessageSeqRange(messages []agent.Message) (int64, int64) {
	var first, latest int64
	for _, message := range messages {
		if message.Seq <= 0 {
			continue
		}
		if first == 0 || message.Seq < first {
			first = message.Seq
		}
		if message.Seq > latest {
			latest = message.Seq
		}
	}
	return first, latest
}

func positiveMessageSeqCount(messages []agent.Message) int {
	count := 0
	for _, message := range messages {
		if message.Seq > 0 {
			count++
		}
	}
	return count
}

// dispatchOrQueueMessageWake is the only dispatch entry for persisted Channel,
// Thread, DM, and Lucy messages. The database slot closes the gap between the
// routing goroutine and Run creation across multiple Server processes.
func (s *AgentService) dispatchOrQueueMessageWake(ctx context.Context, daemon *DaemonInfo, taskReq daemonTaskRequest, ag agentChannelInfo) {
	firstSeq, latestSeq := positiveMessageSeqRange(taskReq.Messages)
	if latestSeq == 0 && taskReq.TriggerMessageID != "" {
		if err := s.pool.QueryRow(ctx, `SELECT seq FROM messages WHERE id = $1`, taskReq.TriggerMessageID).Scan(&latestSeq); err != nil {
			slog.Warn("failed to load message sequence for Agent wake", "message_id", taskReq.TriggerMessageID, "error", err)
			return
		}
		firstSeq = latestSeq
	}
	if firstSeq == 0 || latestSeq == 0 {
		slog.Warn("refusing Agent message wake without a persisted sequence", "agent_id", ag.ID, "channel_id", taskReq.ChannelID)
		return
	}
	taskReq.WakeFirstMessageSeq = firstSeq
	taskReq.WakeLatestMessageSeq = latestSeq
	taskReq.WakeMessageCount = positiveMessageSeqCount(taskReq.Messages)
	run, queued, err := s.claimMessageWake(ctx, daemon, taskReq, ag, firstSeq, latestSeq)
	if err != nil {
		slog.Warn("failed to claim Agent message wake", "agent_id", ag.ID, "channel_id", taskReq.ChannelID, "error", err)
		errorCode := err.Error()
		var budgetErr *BudgetStartError
		if errors.As(err, &budgetErr) {
			errorCode = budgetErr.Code()
		}
		s.broadcastAgentError(taskReq.ThreadID, taskReq.ChannelID, ag.ID, ag.Name, errorCode)
		return
	}
	if queued {
		slog.Info("coalesced Agent message wake",
			"agent_id", ag.ID,
			"channel_id", taskReq.ChannelID,
			"thread_id", taskReq.ThreadID,
			"first_message_seq", firstSeq,
			"latest_message_seq", latestSeq,
		)
		go func() {
			if err := s.advancePendingMessageWake(context.Background(), ag.ID, taskReq.ChannelID); err != nil {
				slog.Debug("pending Agent messages remain queued", "agent_id", ag.ID, "channel_id", taskReq.ChannelID, "error", err)
			}
		}()
		return
	}

	taskReq.PrestartedRun = true
	taskReq.RemotePrequeued = daemon.ComputerID != ""
	go s.runStreamingAgentTask(context.Background(), daemon, taskReq, ag, run)
}

func (s *AgentService) claimMessageWake(ctx context.Context, daemon *DaemonInfo, taskReq daemonTaskRequest, ag agentChannelInfo, firstSeq, latestSeq int64) (*AgentRun, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	if err := lockMessageWakeSlot(ctx, tx, ag.ID, taskReq.ChannelID); err != nil {
		return nil, false, err
	}
	activeRunID, err := findActiveMessageRunTx(ctx, tx, ag.ID, taskReq.ChannelID)
	if err != nil {
		return nil, false, err
	}
	if activeRunID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE agent_message_wake_slots
			   SET active_run_id = $3, updated_at = now()
			 WHERE agent_id = $1 AND channel_id = $2`, ag.ID, taskReq.ChannelID, activeRunID); err != nil {
			return nil, false, err
		}
		if err := upsertPendingMessageWakeTx(ctx, tx, pendingMessageWake{
			AgentID:              ag.ID,
			ChannelID:            taskReq.ChannelID,
			ThreadID:             taskReq.ThreadID,
			ScopeKey:             messageWakeScopeKey(taskReq.ThreadID),
			FirstMessageSeq:      firstSeq,
			LatestMessageSeq:     latestSeq,
			RequiresVisibleReply: taskReq.ResultContract == agentResultContractVisibleMessage,
		}); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	var pendingExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM agent_pending_message_wakes
			 WHERE agent_id = $1 AND channel_id = $2
		)`, ag.ID, taskReq.ChannelID).Scan(&pendingExists); err != nil {
		return nil, false, err
	}
	if pendingExists {
		if err := upsertPendingMessageWakeTx(ctx, tx, pendingMessageWake{
			AgentID:              ag.ID,
			ChannelID:            taskReq.ChannelID,
			ThreadID:             taskReq.ThreadID,
			ScopeKey:             messageWakeScopeKey(taskReq.ThreadID),
			FirstMessageSeq:      firstSeq,
			LatestMessageSeq:     latestSeq,
			RequiresVisibleReply: taskReq.ResultContract == agentResultContractVisibleMessage,
		}); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}

	activityText := "等待执行"
	if daemon.ComputerID != "" && daemon.Status != DaemonStatusOnline {
		activityText = agentActivityWaitingComputer
	}
	runSvc := NewAgentRunService(s.pool)
	run, err := runSvc.startRunTx(ctx, tx, StartRunInput{
		AgentID:          ag.ID,
		DaemonID:         daemon.ID,
		TriggerType:      AgentRunTriggerMessage,
		TriggerMessageID: taskReq.TriggerMessageID,
		ChannelID:        taskReq.ChannelID,
		ThreadID:         taskReq.ThreadID,
		Status:           AgentRunStatusQueued,
		ActivityText:     activityText,
		Source:           taskReq.ModelConfig.Provider,
		FreshnessSeenSeq: latestSeq,
		WakeFirstSeq:     firstSeq,
		WakeLatestSeq:    latestSeq,
		WakeVisible:      taskReq.ResultContract == agentResultContractVisibleMessage,
	})
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE agent_message_wake_slots
		   SET active_run_id = $3, updated_at = now()
		 WHERE agent_id = $1 AND channel_id = $2`, ag.ID, taskReq.ChannelID, run.ID); err != nil {
		return nil, false, err
	}
	if err := s.persistRemoteMessageRunTx(ctx, tx, daemon, run, taskReq, ag); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	if daemon.ComputerID != "" {
		metrics.Global.IncRemoteRunsQueued()
	}
	if err := runSvc.hydrateAgentRunTokenUsage(ctx, run); err != nil {
		slog.Warn("message Run created but token usage could not be reloaded", "run_id", run.ID, "error", err)
	}
	return run, false, nil
}

func lockMessageWakeSlot(ctx context.Context, tx pgx.Tx, agentID, channelID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_message_wake_slots (agent_id, channel_id)
		VALUES ($1, $2)
		ON CONFLICT (agent_id, channel_id) DO NOTHING`, agentID, channelID); err != nil {
		return err
	}
	var ignored string
	return tx.QueryRow(ctx, `
		SELECT COALESCE(active_run_id::text, '')
		  FROM agent_message_wake_slots
		 WHERE agent_id = $1 AND channel_id = $2
		 FOR UPDATE`, agentID, channelID).Scan(&ignored)
}

func findActiveMessageRunTx(ctx context.Context, tx pgx.Tx, agentID, channelID string) (string, error) {
	var runID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		  FROM agent_runs
		 WHERE agent_id = $1
		   AND channel_id = $2
		   AND trigger_type = $3
		   AND trigger_message_id IS NOT NULL
		   AND thinking_node_id IS NULL
		   AND finished_at IS NULL
		 ORDER BY started_at ASC, id ASC
		 LIMIT 1`, agentID, channelID, AgentRunTriggerMessage).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return runID, err
}

func upsertPendingMessageWakeTx(ctx context.Context, tx pgx.Tx, wake pendingMessageWake) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO agent_pending_message_wakes (
			agent_id, channel_id, scope_key, thread_id, first_message_seq,
			latest_message_seq, requires_visible_result
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (agent_id, channel_id, scope_key)
		DO UPDATE SET
			first_message_seq = LEAST(agent_pending_message_wakes.first_message_seq, EXCLUDED.first_message_seq),
			latest_message_seq = GREATEST(agent_pending_message_wakes.latest_message_seq, EXCLUDED.latest_message_seq),
			requires_visible_result = agent_pending_message_wakes.requires_visible_result OR EXCLUDED.requires_visible_result,
			updated_at = now()`,
		wake.AgentID, wake.ChannelID, wake.ScopeKey, nullableUUID(wake.ThreadID),
		wake.FirstMessageSeq, wake.LatestMessageSeq, wake.RequiresVisibleReply,
	)
	return err
}

// advancePendingMessageWake repairs the slot after any terminal path and, when
// possible, atomically claims the oldest pending scope as the next Run.
func (s *AgentService) advancePendingMessageWake(ctx context.Context, agentID, channelID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockMessageWakeSlot(ctx, tx, agentID, channelID); err != nil {
		return err
	}
	activeRunID, err := findActiveMessageRunTx(ctx, tx, agentID, channelID)
	if err != nil {
		return err
	}
	if activeRunID != "" {
		_, err = tx.Exec(ctx, `
			UPDATE agent_message_wake_slots SET active_run_id = $3, updated_at = now()
			 WHERE agent_id = $1 AND channel_id = $2`, agentID, channelID, activeRunID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var ag agentChannelInfo
	if err := tx.QueryRow(ctx, `
		SELECT id::text, name, model_provider, model_name, system_prompt
		  FROM agents
		 WHERE id = $1 AND is_active = true`, agentID,
	).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName, &ag.SystemPrompt); err != nil {
		return err
	}

	var wake pendingMessageWake
	for {
		err = tx.QueryRow(ctx, `
			SELECT agent_id::text, channel_id::text, COALESCE(thread_id::text, ''), scope_key,
			       first_message_seq, latest_message_seq, requires_visible_result
			  FROM agent_pending_message_wakes
			 WHERE agent_id = $1 AND channel_id = $2
			 ORDER BY created_at ASC, scope_key ASC
			 LIMIT 1
			 FOR UPDATE`, agentID, channelID,
		).Scan(&wake.AgentID, &wake.ChannelID, &wake.ThreadID, &wake.ScopeKey,
			&wake.FirstMessageSeq, &wake.LatestMessageSeq, &wake.RequiresVisibleReply)
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `
				UPDATE agent_message_wake_slots SET active_run_id = NULL, updated_at = now()
				 WHERE agent_id = $1 AND channel_id = $2`, agentID, channelID)
			if err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			SELECT id::text
			  FROM messages
			 WHERE channel_id = $1
			   AND seq BETWEEN $2 AND $3
			   AND is_deleted = false
			   AND thinking_node_id IS NULL
			   AND (($4 = '' AND thread_id IS NULL) OR thread_id = NULLIF($4, '')::uuid)
			 ORDER BY seq DESC
			 LIMIT 1`, channelID, wake.FirstMessageSeq, wake.LatestMessageSeq, wake.ThreadID,
		).Scan(&wake.TriggerMessageID)
		if !errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM agent_pending_message_wakes
			 WHERE agent_id = $1 AND channel_id = $2 AND scope_key = $3`, agentID, channelID, wake.ScopeKey); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	dmn, err := s.dm.ResolveDaemonForAgent(ctx, agentID, "llm")
	if err != nil {
		return err
	}
	taskReq, err := s.buildPendingMessageWakeTask(ctx, wake, ag)
	if err != nil {
		return err
	}

	activityText := "等待执行"
	if dmn.ComputerID != "" && dmn.Status != DaemonStatusOnline {
		activityText = agentActivityWaitingComputer
	}
	runSvc := NewAgentRunService(s.pool)
	run, err := runSvc.startRunTx(ctx, tx, StartRunInput{
		AgentID:          agentID,
		DaemonID:         dmn.ID,
		TriggerType:      AgentRunTriggerMessage,
		TriggerMessageID: wake.TriggerMessageID,
		ChannelID:        channelID,
		ThreadID:         wake.ThreadID,
		Status:           AgentRunStatusQueued,
		ActivityText:     activityText,
		Source:           ag.ModelProvider,
		FreshnessSeenSeq: wake.LatestMessageSeq,
		WakeFirstSeq:     wake.FirstMessageSeq,
		WakeLatestSeq:    wake.LatestMessageSeq,
		WakeVisible:      wake.RequiresVisibleReply,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM agent_pending_message_wakes
		 WHERE agent_id = $1 AND channel_id = $2 AND scope_key = $3`, agentID, channelID, wake.ScopeKey); err != nil {
		return err
	}
	if err := s.persistRemoteMessageRunTx(ctx, tx, dmn, run, taskReq, ag); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE agent_message_wake_slots SET active_run_id = $3, updated_at = now()
		 WHERE agent_id = $1 AND channel_id = $2`, agentID, channelID, run.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if dmn.ComputerID != "" {
		metrics.Global.IncRemoteRunsQueued()
	}
	if err := runSvc.hydrateAgentRunTokenUsage(ctx, run); err != nil {
		slog.Warn("coalesced message Run token usage could not be reloaded", "run_id", run.ID, "error", err)
	}
	taskReq.PrestartedRun = true
	taskReq.RemotePrequeued = dmn.ComputerID != ""
	go s.runStreamingAgentTask(context.Background(), dmn, taskReq, ag, run)
	return nil
}

// persistRemoteMessageRunTx closes the crash window between claiming a wake
// range and queuing it for a remote Computer. On Server restart the Daemon can
// accept this payload directly from PostgreSQL even if the dispatch goroutine
// never started.
func (s *AgentService) persistRemoteMessageRunTx(ctx context.Context, tx pgx.Tx, dmn *DaemonInfo, run *AgentRun, taskReq daemonTaskRequest, ag agentChannelInfo) error {
	if dmn.ComputerID == "" {
		return nil
	}
	taskReq.RunID = run.ID
	taskReq.TaskID = run.ID
	taskReq.AgentName = ag.Name
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name, id::text) FROM channels WHERE id = $1`, taskReq.ChannelID).Scan(&taskReq.ChannelName)
	var customEnv, customArgs json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT custom_env, custom_args FROM agents WHERE id = $1`, ag.ID).Scan(&customEnv, &customArgs); err == nil {
		_ = json.Unmarshal(customEnv, &taskReq.CustomEnv)
		_ = json.Unmarshal(customArgs, &taskReq.CustomArgs)
	}
	if taskReq.NodeID == "" && taskReq.ResumeSessionID == "" && !taskReq.ForceFreshSession {
		_ = tx.QueryRow(ctx, `
			SELECT sess.external_session_id
			  FROM agent_runs r
			  JOIN agent_sessions sess ON sess.id = r.session_id
			 WHERE r.agent_id = $1 AND r.channel_id = $2 AND r.id <> $3
			   AND r.thinking_node_id IS NULL AND sess.provider = $4
			   AND sess.status = 'active' AND COALESCE(sess.external_session_id, '') <> ''
			 ORDER BY sess.last_active_at DESC, r.updated_at DESC LIMIT 1`,
			ag.ID, taskReq.ChannelID, run.ID, taskReq.ModelConfig.Provider,
		).Scan(&taskReq.ResumeSessionID)
	}
	payload, err := json.Marshal(taskReq)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE agent_runs
		   SET computer_id = $2::uuid, daemon_id = $2::text, dispatch_payload = $3,
		       delivery_expires_at = $4, execution_attempt_id = NULL, accepted_at = NULL,
		       updated_at = now()
		 WHERE id = $1 AND finished_at IS NULL`,
		run.ID, dmn.ComputerID, payload, time.Now().UTC().Add(remoteRunDeliveryTTL),
	)
	return err
}

func (s *AgentService) requeueUndeliveredMessageWake(ctx context.Context, run *AgentRun) error {
	if run == nil || run.TriggerType != AgentRunTriggerMessage || run.ThinkingNodeID != "" {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockMessageWakeSlot(ctx, tx, run.AgentID, run.ChannelID); err != nil {
		return err
	}
	var first, latest int64
	var visible bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(wake_first_message_seq, 0), COALESCE(wake_latest_message_seq, 0),
		       wake_requires_visible_result
		  FROM agent_runs WHERE id = $1 AND finished_at IS NULL AND accepted_at IS NULL`, run.ID,
	).Scan(&first, &latest, &visible); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	if first == 0 || latest == 0 {
		return tx.Commit(ctx)
	}
	if err := upsertPendingMessageWakeTx(ctx, tx, pendingMessageWake{
		AgentID: run.AgentID, ChannelID: run.ChannelID, ThreadID: run.ThreadID,
		ScopeKey: messageWakeScopeKey(run.ThreadID), FirstMessageSeq: first,
		LatestMessageSeq: latest, RequiresVisibleReply: visible,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AgentService) buildPendingMessageWakeTask(ctx context.Context, wake pendingMessageWake, ag agentChannelInfo) (daemonTaskRequest, error) {
	var turnMessages, coldStartMessages []agent.Message
	var err error
	if wake.ThreadID == "" {
		turnMessages, err = s.getChannelMessagesInSeqRange(ctx, wake.ChannelID, wake.FirstMessageSeq, wake.LatestMessageSeq)
	} else {
		turnMessages, coldStartMessages, err = s.getThreadMessagesInSeqRange(ctx, wake.ChannelID, wake.ThreadID, wake.FirstMessageSeq, wake.LatestMessageSeq)
	}
	if err != nil {
		return daemonTaskRequest{}, err
	}
	if len(turnMessages) == 0 {
		return daemonTaskRequest{}, errors.New("coalesced wake has no remaining messages")
	}
	mentionedIDs, err := s.getMentionedAgentIDsInSeqRange(ctx, wake.ChannelID, wake.ThreadID, wake.FirstMessageSeq, wake.LatestMessageSeq)
	if err != nil {
		return daemonTaskRequest{}, err
	}
	resultContract := agentResultContractNone
	if wake.RequiresVisibleReply {
		resultContract = agentResultContractVisibleMessage
	}
	return daemonTaskRequest{
		AgentID:              ag.ID,
		ChannelID:            wake.ChannelID,
		ThreadID:             wake.ThreadID,
		TriggerMessageID:     wake.TriggerMessageID,
		Messages:             turnMessages,
		ColdStartMessages:    coldStartMessages,
		SystemPrompt:         ag.SystemPrompt,
		ModelConfig:          agent.ModelConfig{Provider: ag.ModelProvider, Model: ag.ModelName},
		TaskContext:          s.getChannelOpenTasksSummary(ctx, wake.ChannelID),
		AgentChain:           []string{ag.ID},
		MentionedNames:       s.resolveMentionedNames(ctx, mentionedIDs),
		ResultContract:       resultContract,
		ModelSeenSeq:         wake.LatestMessageSeq,
		WakeFirstMessageSeq:  wake.FirstMessageSeq,
		WakeLatestMessageSeq: wake.LatestMessageSeq,
		WakeMessageCount:     len(turnMessages),
	}, nil
}

func (s *AgentService) getChannelMessagesInSeqRange(ctx context.Context, channelID string, firstSeq, latestSeq int64) ([]agent.Message, error) {
	var channelName, channelType string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(name, id::text), type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType); err != nil {
		return nil, err
	}
	target := "#" + channelName
	if channelType == "dm" {
		target = "dm:@" + channelName
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, seq, sender_type, sender_id::text, content, created_at, COALESCE(attachment_ids, '{}')
		  FROM messages
		 WHERE channel_id = $1 AND thread_id IS NULL AND thinking_node_id IS NULL
		   AND is_deleted = false AND seq BETWEEN $2 AND $3
		 ORDER BY seq ASC`, channelID, firstSeq, latestSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []agent.Message
	for rows.Next() {
		var id, senderType, senderID, content string
		var seq int64
		var createdAt time.Time
		var attachmentIDs []string
		if err := rows.Scan(&id, &seq, &senderType, &senderID, &content, &createdAt, &attachmentIDs); err != nil {
			return nil, err
		}
		role := agent.RoleUser
		if senderType == "agent" {
			role = agent.RoleAssistant
		}
		shortID := id
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		messageContent, attachments := s.enrichMessageContentAndAttachments(ctx, content, attachmentIDs)
		formatted := fmt.Sprintf("New message received:\n\n[target=%s msg=%s time=%s type=%s] @%s: %s",
			target, shortID, createdAt.UTC().Format(time.RFC3339), senderType,
			s.resolveSenderName(ctx, senderType, senderID), messageContent)
		messages = append(messages, agent.Message{Role: role, Content: formatted, SenderID: senderID, Attachments: attachments, Seq: seq})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		messages[len(messages)-1].Content += "\n\nThese messages arrived while your previous turn was running. Handle them together in sequence. Complete all your work before stopping.\nReply in the channel or create/reply in a thread as appropriate; use each message's `target` and `msg` fields to choose the exact target."
	}
	return messages, nil
}

func (s *AgentService) getThreadMessagesInSeqRange(ctx context.Context, channelID, threadID string, firstSeq, latestSeq int64) ([]agent.Message, []agent.Message, error) {
	threadMessages, err := NewThreadService(s.pool).GetThreadContextMessages(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	var channelName, channelType string
	if err := s.pool.QueryRow(ctx, `SELECT name, type FROM channels WHERE id = $1`, channelID).Scan(&channelName, &channelType); err != nil {
		return nil, nil, err
	}
	target := "#" + channelName
	if channelType == "dm" {
		target = "dm:@" + channelName
	}
	var turnMessages []agent.Message
	coldStartMessages := make([]agent.Message, 0, len(threadMessages))
	for _, message := range threadMessages {
		role := agent.RoleUser
		if message.SenderType == "agent" {
			role = agent.RoleAssistant
		}
		shortID := message.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		senderName := message.SenderName
		if senderName == "" {
			senderName = message.SenderID
		}
		messageContent, attachments := s.enrichMessageContentAndAttachments(ctx, message.Content, message.AttachmentIDs)
		formatted := agent.Message{
			Role: role,
			Content: fmt.Sprintf("[target=%s:%s msg=%s time=%s type=%s] @%s: %s",
				target, shortID, shortID, message.CreatedAt.UTC().Format(time.RFC3339), message.SenderType, senderName, messageContent),
			SenderID: message.SenderID, Attachments: attachments, Seq: message.Seq,
		}
		coldStartMessages = append(coldStartMessages, formatted)
		if message.Seq >= firstSeq && message.Seq <= latestSeq {
			turnMessages = append(turnMessages, formatted)
		}
	}
	if len(turnMessages) > 0 {
		turnMessages[len(turnMessages)-1].Content += "\n\nThese messages arrived while your previous turn was running. Handle them together in sequence and reply in this thread."
	}
	return turnMessages, coldStartMessages, nil
}

func (s *AgentService) getMentionedAgentIDsInSeqRange(ctx context.Context, channelID, threadID string, firstSeq, latestSeq int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT mentioned_id::text
		  FROM messages m
		 CROSS JOIN LATERAL unnest(COALESCE(m.mentioned_agent_ids, '{}')) AS mentioned_id
		 WHERE m.channel_id = $1 AND m.seq BETWEEN $2 AND $3
		   AND m.is_deleted = false AND m.thinking_node_id IS NULL
		   AND (($4 = '' AND m.thread_id IS NULL) OR m.thread_id = NULLIF($4, '')::uuid)
		 ORDER BY mentioned_id::text`, channelID, firstSeq, latestSeq, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *AgentService) drainPendingMessageWakes(ctx context.Context) error {
	var tableExists bool
	if err := s.pool.QueryRow(ctx, `SELECT to_regclass('agent_pending_message_wakes') IS NOT NULL`).Scan(&tableExists); err != nil || !tableExists {
		return err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT agent_id::text, channel_id::text
		  FROM agent_pending_message_wakes
		 ORDER BY agent_id::text, channel_id::text
		 LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type key struct{ agentID, channelID string }
	var keys []key
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.agentID, &item.channelID); err != nil {
			return err
		}
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range keys {
		if err := s.advancePendingMessageWake(ctx, item.agentID, item.channelID); err != nil {
			slog.Warn("failed to drain pending Agent messages", "agent_id", item.agentID, "channel_id", item.channelID, "error", err)
		}
	}
	return nil
}

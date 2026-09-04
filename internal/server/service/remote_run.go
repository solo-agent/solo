package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/pkg/metrics"
)

const remoteRunDeliveryTTL = 24 * time.Hour

var (
	ErrRemoteRunNotFound = errors.New("remote Run not found")
	ErrRemoteRunExpired  = errors.New("remote Run delivery expired")
	ErrRemoteRunAttempt  = errors.New("remote Run attempt mismatch")
)

type RemoteRunDelivery struct {
	RunID          string          `json:"run_id"`
	TaskID         string          `json:"task_id"`
	AttemptID      string          `json:"execution_attempt_id"`
	Payload        json.RawMessage `json:"payload"`
	AgentToken     string          `json:"agent_token"`
	TokenExpiresAt time.Time       `json:"token_expires_at"`
}

type RemoteRunEventInput struct {
	TaskID       string          `json:"task_id"`
	AttemptID    string          `json:"execution_attempt_id"`
	ConnectionID string          `json:"connection_id"`
	SourceSeq    int64           `json:"source_seq"`
	Event        string          `json:"event"`
	Data         json.RawMessage `json:"data"`
}

type remoteRunStream struct {
	events    chan SSEDaemonEvent
	incoming  chan remoteDeliveryEvent
	done      chan struct{}
	closeOnce sync.Once
}

type remoteDeliveryEvent struct {
	attemptID string
	sourceSeq int64
	event     SSEDaemonEvent
}

func (dm *DaemonManager) QueueRemoteRun(ctx context.Context, daemon *DaemonInfo, request any) (string, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var identity struct {
		RunID  string `json:"run_id"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(payload, &identity); err != nil || identity.RunID == "" || identity.TaskID == "" {
		return "", errors.New("remote Run requires run_id and task_id")
	}
	computerID := daemon.ComputerID
	if computerID == "" {
		computerID = daemon.ID
	}
	command, err := dm.pool.Exec(ctx, `
		UPDATE agent_runs
		   SET computer_id = $2::uuid,
		       daemon_id = $2::text,
		       dispatch_payload = $3,
		       delivery_expires_at = $4,
		       execution_attempt_id = NULL,
		       accepted_at = NULL,
		       updated_at = now()
		 WHERE id = $1 AND finished_at IS NULL`, identity.RunID, computerID, payload, time.Now().UTC().Add(remoteRunDeliveryTTL))
	if err != nil {
		return "", err
	}
	if command.RowsAffected() != 1 {
		return "", ErrRemoteRunNotFound
	}
	metrics.Global.IncRemoteRunsQueued()
	return identity.TaskID, nil
}

func (dm *DaemonManager) PendingRemoteRuns(ctx context.Context, computerID string) ([]string, error) {
	rows, err := dm.pool.Query(ctx, `
		SELECT id::text
		  FROM agent_runs
		 WHERE computer_id = $1
		   AND finished_at IS NULL
		   AND execution_attempt_id IS NULL
		   AND delivery_expires_at > now()
		 ORDER BY started_at, id`, computerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]string, 0)
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		runs = append(runs, runID)
	}
	return runs, rows.Err()
}

func (dm *DaemonManager) AcceptRemoteRun(ctx context.Context, computerID, runID, connectionID string) (*RemoteRunDelivery, error) {
	if err := dm.AuthorizeControlLease(computerID, connectionID); err != nil {
		return nil, err
	}
	tx, err := dm.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var delivery RemoteRunDelivery
	var agentID, agentName, runChannelID, runProvider string
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT r.id::text, COALESCE(r.dispatch_payload, '{}'::jsonb),
		       COALESCE(r.execution_attempt_id::text, ''), r.delivery_expires_at,
		       r.agent_id::text, COALESCE(a.name, r.agent_id::text),
		       COALESCE(r.channel_id::text, ''), COALESCE(r.source, '')
		  FROM agent_runs r
		  JOIN agents a ON a.id = r.agent_id
		 WHERE r.id = $1 AND r.computer_id = $2 AND r.finished_at IS NULL
		 FOR UPDATE`, runID, computerID,
	).Scan(&delivery.RunID, &delivery.Payload, &delivery.AttemptID, &expiresAt, &agentID, &agentName, &runChannelID, &runProvider)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRemoteRunNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(expiresAt) {
		return nil, ErrRemoteRunExpired
	}
	var taskReq daemonTaskRequest
	if err := json.Unmarshal(delivery.Payload, &taskReq); err != nil || taskReq.TaskID == "" {
		return nil, errors.New("stored Run payload is invalid")
	}
	delivery.TaskID = taskReq.TaskID
	if taskReq.AgentID == "" {
		taskReq.AgentID = agentID
	}
	if taskReq.ChannelID == "" {
		taskReq.ChannelID = runChannelID
	}
	if taskReq.ModelConfig.Provider == "" {
		taskReq.ModelConfig.Provider = runProvider
	}
	if taskReq.ChannelID != "" && taskReq.ModelConfig.Provider != "" {
		supportsRollover := false
		if daemon, ok := dm.GetDaemon(computerID); ok {
			supportsRollover = hasCapability(daemon.Capabilities, contextRolloverCapability)
		}
		dispatch, err := NewAgentRunService(dm.pool).resolveSessionDispatchTx(ctx, tx, ResolveSessionDispatchInput{
			RunID:                   runID,
			AgentID:                 agentID,
			ChannelID:               taskReq.ChannelID,
			Provider:                taskReq.ModelConfig.Provider,
			ThinkingNodeID:          taskReq.NodeID,
			ResumeSessionID:         taskReq.ResumeSessionID,
			ForceFreshSession:       taskReq.ForceFreshSession,
			SupportsContextRollover: supportsRollover,
		})
		if err != nil {
			return nil, err
		}
		agentService := dm.agentService
		if agentService == nil {
			agentService = &AgentService{pool: dm.pool}
		}
		if err := agentService.applySessionDispatch(ctx, tx, &taskReq, dispatch); err != nil {
			return nil, err
		}
	}
	delivery.Payload, err = json.Marshal(taskReq)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_runs SET dispatch_payload = $2, updated_at = now() WHERE id = $1`, runID, delivery.Payload); err != nil {
		return nil, err
	}
	if delivery.AttemptID == "" {
		delivery.AttemptID = uuid.NewString()
		if _, err := tx.Exec(ctx, `
			UPDATE agent_runs
			   SET execution_attempt_id = $2, accepted_at = now(), delivery_count = delivery_count + 1, updated_at = now()
			 WHERE id = $1`, runID, delivery.AttemptID); err != nil {
			return nil, err
		}
	}
	token, err := auth.GenerateAgentRunToken(agentID, agentName, delivery.RunID, computerID)
	if err != nil {
		return nil, err
	}
	delivery.AgentToken = token
	delivery.TokenExpiresAt = time.Now().Add(auth.AgentRunTokenDuration)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	metrics.Global.IncRemoteRunAccepts()
	return &delivery, nil
}

func (dm *DaemonManager) AppendRemoteRunEvent(ctx context.Context, computerID, runID string, input RemoteRunEventInput) error {
	if input.TaskID == "" || input.AttemptID == "" || input.ConnectionID == "" || input.SourceSeq <= 0 || input.Event == "" {
		return errors.New("incomplete remote Run event")
	}
	if err := dm.AuthorizeControlLease(computerID, input.ConnectionID); err != nil {
		return err
	}
	var valid bool
	if err := dm.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM agent_runs
		   WHERE id = $1 AND computer_id = $2 AND execution_attempt_id = $3
		     AND dispatch_payload->>'task_id' = $4
		)`, runID, computerID, input.AttemptID, input.TaskID).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrRemoteRunAttempt
	}
	if len(input.Data) == 0 {
		input.Data = json.RawMessage(`{}`)
	}
	command, err := dm.pool.Exec(ctx, `
		INSERT INTO agent_run_delivery_events (run_id, task_id, attempt_id, source_seq, event, data)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (run_id, attempt_id, source_seq) DO NOTHING`,
		runID, input.TaskID, input.AttemptID, input.SourceSeq, input.Event, input.Data)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 1 {
		dm.deliverRemoteRunEvent(input.TaskID, input.AttemptID, input.SourceSeq, SSEDaemonEvent{Event: input.Event, Data: string(input.Data)})
	} else {
		metrics.Global.IncRemoteEventDupes()
	}
	return nil
}

func (dm *DaemonManager) SubscribeRemoteTask(ctx context.Context, taskID string) (<-chan SSEDaemonEvent, error) {
	stream := &remoteRunStream{
		events: make(chan SSEDaemonEvent, 256), incoming: make(chan remoteDeliveryEvent, 1024), done: make(chan struct{}),
	}
	dm.remoteMu.Lock()
	if previous := dm.remoteStreams[taskID]; previous != nil {
		previous.close()
	}
	dm.remoteStreams[taskID] = stream
	dm.remoteMu.Unlock()

	rows, err := dm.pool.Query(ctx, `
		SELECT attempt_id::text, source_seq, event, data
		  FROM agent_run_delivery_events
		 WHERE task_id = $1
		 ORDER BY created_at, source_seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := make([]remoteDeliveryEvent, 0)
	for rows.Next() {
		var attemptID, event string
		var sourceSeq int64
		var data json.RawMessage
		if err := rows.Scan(&attemptID, &sourceSeq, &event, &data); err != nil {
			return nil, err
		}
		history = append(history, remoteDeliveryEvent{attemptID: attemptID, sourceSeq: sourceSeq, event: SSEDaemonEvent{Event: event, Data: string(data)}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	go stream.pump(ctx, history)
	go func() {
		<-ctx.Done()
		dm.remoteMu.Lock()
		if dm.remoteStreams[taskID] == stream {
			delete(dm.remoteStreams, taskID)
		}
		dm.remoteMu.Unlock()
		stream.close()
	}()
	return stream.events, nil
}

func (dm *DaemonManager) deliverRemoteRunEvent(taskID, attemptID string, sourceSeq int64, event SSEDaemonEvent) {
	dm.remoteMu.Lock()
	stream := dm.remoteStreams[taskID]
	dm.remoteMu.Unlock()
	if stream != nil {
		stream.enqueue(remoteDeliveryEvent{attemptID: attemptID, sourceSeq: sourceSeq, event: event})
	}
}

func (stream *remoteRunStream) pump(ctx context.Context, history []remoteDeliveryEvent) {
	defer close(stream.events)
	seen := make(map[string]bool)
	deliver := func(item remoteDeliveryEvent) bool {
		key := fmt.Sprintf("%s:%d", item.attemptID, item.sourceSeq)
		if seen[key] {
			return true
		}
		seen[key] = true
		select {
		case stream.events <- item.event:
			return item.event.Event != "done"
		case <-ctx.Done():
			return false
		case <-stream.done:
			return false
		}
	}
	for _, item := range history {
		if !deliver(item) {
			return
		}
	}
	for {
		select {
		case item := <-stream.incoming:
			if !deliver(item) {
				return
			}
		case <-ctx.Done():
			return
		case <-stream.done:
			return
		}
	}
}

func (stream *remoteRunStream) enqueue(event remoteDeliveryEvent) {
	select {
	case stream.incoming <- event:
	case <-stream.done:
	}
}

func (stream *remoteRunStream) close() { stream.closeOnce.Do(func() { close(stream.done) }) }

func (dm *DaemonManager) reconcileRemoteConnection(ctx context.Context, computerID string, active []ActiveRunAttempt) {
	activeByRun := make(map[string]string, len(active))
	activeIDs := make([]string, 0, len(active))
	for _, attempt := range active {
		activeByRun[attempt.RunID] = attempt.AttemptID
		activeIDs = append(activeIDs, attempt.RunID)
	}
	rows, err := dm.pool.Query(ctx, `
		SELECT id::text, execution_attempt_id::text
		  FROM agent_runs
		 WHERE computer_id = $1 AND finished_at IS NULL AND execution_attempt_id IS NOT NULL`, computerID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var runID, attemptID string
			if rows.Scan(&runID, &attemptID) == nil && activeByRun[runID] != attemptID {
				_, _ = dm.pool.Exec(ctx, `UPDATE agent_runs SET execution_attempt_id = NULL, accepted_at = NULL, updated_at = now() WHERE id = $1 AND execution_attempt_id = $2`, runID, attemptID)
			}
		}
	}
	if dm.agentService != nil {
		if daemon, ok := dm.GetDaemon(computerID); ok {
			dm.agentService.ReconcileRemoteRuns(ctx, daemon)
		}
	}
	for _, runID := range activeIDs {
		dm.NotifyRun(computerID, runID)
	}
	pending, err := dm.PendingRemoteRuns(ctx, computerID)
	if err == nil {
		for _, runID := range pending {
			dm.NotifyRun(computerID, runID)
		}
	}
}

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/internal/server/workspace"
)

// DaemonStatus represents the operational status of a daemon instance.
type DaemonStatus string

const (
	DaemonStatusOnline  DaemonStatus = "online"
	DaemonStatusOffline DaemonStatus = "offline"
)

// PendingTaskInfo holds metadata for a task dispatched to a daemon.
type PendingTaskInfo struct {
	TaskID           string     `json:"task_id"`
	AgentID          string     `json:"agent_id"`
	DaemonID         string     `json:"daemon_id"`
	RunID            string     `json:"run_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	BackendStartedAt *time.Time `json:"backend_started_at,omitempty"`
	TimeoutPhase     string     `json:"-"`
}

// DaemonInfo holds runtime state for a registered daemon instance.
type DaemonInfo struct {
	ID               string       `json:"daemon_id"`
	ComputerID       string       `json:"computer_id,omitempty"`
	Host             string       `json:"host"`
	Port             int          `json:"port"`
	Version          string       `json:"version"`
	Capabilities     []string     `json:"capabilities"`
	MaxConcurrent    int          `json:"max_concurrent"`
	CurrentLoad      int32        `json:"current_load"`
	AgentTypes       []string     `json:"agent_types"`
	Status           DaemonStatus `json:"status"`
	LastHeartbeat    time.Time    `json:"last_heartbeat"`
	MissedHeartbeats int          `json:"-"`
	RegisteredAt     time.Time    `json:"registered_at"`
}

// DaemonManager manages daemon instance registration, heartbeat monitoring,
// pending task tracking, and load-balanced task dispatching.
type DaemonManager struct {
	mu      sync.RWMutex
	daemons map[string]*DaemonInfo

	pool       *pgxpool.Pool
	hub        realtime.Broadcaster
	httpClient *http.Client

	heartbeatInterval time.Duration
	maxMissedHB       int

	// Pending tasks indexed by taskID for cleanup when daemons go offline.
	pendingTasks map[string]PendingTaskInfo

	// Queue wait and provider execution have distinct clocks. A task waiting
	// for a persistent-session turn must not consume its execution budget.
	queueTimeout     time.Duration
	executionTimeout time.Duration

	stopCh chan struct{}

	controlMu          sync.RWMutex
	controlConnections map[string]*DaemonControlConnection
	controlGrace       map[string]controlLeaseGrace
	rpcWaiters         map[string]chan ControlEnvelope

	remoteMu      sync.Mutex
	remoteStreams map[string]*remoteRunStream
	agentService  *AgentService
}

// NewDaemonManager creates a new DaemonManager.
func NewDaemonManager(pool *pgxpool.Pool, hub realtime.Broadcaster) *DaemonManager {
	return &DaemonManager{
		daemons:            make(map[string]*DaemonInfo),
		pendingTasks:       make(map[string]PendingTaskInfo),
		pool:               pool,
		hub:                hub,
		httpClient:         &http.Client{Timeout: 10 * time.Second},
		heartbeatInterval:  30 * time.Second,
		maxMissedHB:        3,
		queueTimeout:       agentRunQueueTimeout,
		executionTimeout:   agentRunExecutionTimeout,
		stopCh:             make(chan struct{}),
		controlConnections: make(map[string]*DaemonControlConnection),
		controlGrace:       make(map[string]controlLeaseGrace),
		rpcWaiters:         make(map[string]chan ControlEnvelope),
		remoteStreams:      make(map[string]*remoteRunStream),
	}
}

func (dm *DaemonManager) SetAgentService(agentService *AgentService) {
	dm.agentService = agentService
}

// Start begins the heartbeat monitoring goroutine.
func (dm *DaemonManager) Start() {
	go dm.healthCheckLoop()
	slog.Info("daemon manager started", "heartbeat_interval", dm.heartbeatInterval, "max_missed", dm.maxMissedHB)
}

// Stop stops the heartbeat monitoring goroutine.
func (dm *DaemonManager) Stop() {
	close(dm.stopCh)
	dm.closeControlConnections()
}

// Register registers a new daemon instance or updates an existing one.
func (dm *DaemonManager) Register(info *DaemonInfo) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	existing, ok := dm.daemons[info.ID]
	if ok {
		// Update existing
		existing.Host = info.Host
		existing.Port = info.Port
		existing.Version = info.Version
		existing.Capabilities = info.Capabilities
		existing.MaxConcurrent = info.MaxConcurrent
		existing.AgentTypes = info.AgentTypes
		existing.Status = DaemonStatusOnline
		existing.LastHeartbeat = time.Now()
		existing.MissedHeartbeats = 0

		slog.Info("daemon re-registered", "daemon_id", info.ID)
		return
	}

	info.Status = DaemonStatusOnline
	info.LastHeartbeat = time.Now()
	info.RegisteredAt = time.Now()
	dm.daemons[info.ID] = info

	slog.Info("daemon registered", "daemon_id", info.ID, "host", info.Host, "port", info.Port)
}

// Heartbeat updates the last heartbeat time for a daemon.
// Returns false if the daemon is not registered.
func (dm *DaemonManager) Heartbeat(daemonID string, load int32) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	info, ok := dm.daemons[daemonID]
	if !ok {
		return false
	}

	info.LastHeartbeat = time.Now()
	info.MissedHeartbeats = 0
	info.Status = DaemonStatusOnline
	info.CurrentLoad = load

	return true
}

// GetDaemon returns a daemon by ID.
func (dm *DaemonManager) GetDaemon(daemonID string) (*DaemonInfo, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	info, ok := dm.daemons[daemonID]
	if !ok {
		return nil, false
	}
	return info, true
}

// Unregister removes a daemon and its pending tasks from tracking.
// Called when a daemon shuts down cleanly.
func (dm *DaemonManager) Unregister(daemonID string) {
	dm.mu.Lock()
	delete(dm.daemons, daemonID)

	timedOutTasks := make([]PendingTaskInfo, 0)
	for taskID, task := range dm.pendingTasks {
		if task.DaemonID == daemonID {
			timedOutTasks = append(timedOutTasks, task)
			delete(dm.pendingTasks, taskID)
		}
	}
	dm.mu.Unlock()

	for _, task := range timedOutTasks {
		dm.daemonLostPendingTaskRun(task)
	}

	slog.Info("daemon unregistered",
		"daemon_id", daemonID,
		"cleaned_tasks", len(timedOutTasks),
	)
}

// ListDaemons returns all daemons with their status.
func (dm *DaemonManager) ListDaemons() []*DaemonInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*DaemonInfo, 0, len(dm.daemons))
	for _, d := range dm.daemons {
		cp := *d
		result = append(result, &cp)
	}
	return result
}

// TrackTask records a pending task dispatched to a daemon.
// Used for cleanup when a daemon goes offline.
func (dm *DaemonManager) TrackTask(taskID, daemonID, agentID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.pendingTasks[taskID] = PendingTaskInfo{
		TaskID:    taskID,
		AgentID:   agentID,
		DaemonID:  daemonID,
		CreatedAt: time.Now(),
	}
}

// AttachTaskRun records the agent run created for a pending daemon task.
func (dm *DaemonManager) AttachTaskRun(taskID, runID string) {
	if taskID == "" || runID == "" {
		return
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()
	task, ok := dm.pendingTasks[taskID]
	if !ok {
		return
	}
	task.RunID = runID
	dm.pendingTasks[taskID] = task
}

// MarkTaskBackendStarted moves pending-task timeout accounting from queue wait
// to provider execution. Replayed events keep the first authoritative time.
func (dm *DaemonManager) MarkTaskBackendStarted(taskID string, startedAt *time.Time) {
	if taskID == "" {
		return
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()
	task, ok := dm.pendingTasks[taskID]
	if !ok || task.BackendStartedAt != nil {
		return
	}
	started := time.Now()
	if startedAt != nil {
		started = *startedAt
	}
	task.BackendStartedAt = &started
	dm.pendingTasks[taskID] = task
}

// RemoveTask removes a task from the pending tracking once it completes or errors.
func (dm *DaemonManager) RemoveTask(taskID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.pendingTasks, taskID)
}

// GetDaemonPendingTasks returns all pending tasks for a given daemon.
func (dm *DaemonManager) GetDaemonPendingTasks(daemonID string) []PendingTaskInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var result []PendingTaskInfo
	for _, t := range dm.pendingTasks {
		if t.DaemonID == daemonID {
			result = append(result, t)
		}
	}
	return result
}

func (dm *DaemonManager) IsTaskTracked(taskID string) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	_, ok := dm.pendingTasks[taskID]
	return ok
}

// SelectDaemon picks the daemon with the lowest current load.
// Returns nil if no online daemon is available.
func (dm *DaemonManager) SelectDaemon(capability string) *DaemonInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var best *DaemonInfo
	var bestLoad int32 = 1<<31 - 1

	for _, d := range dm.daemons {
		if d.Status != DaemonStatusOnline {
			continue
		}
		load := d.CurrentLoad
		if load >= int32(d.MaxConcurrent) {
			continue
		}
		if capability != "" && !hasCapability(d.Capabilities, capability) {
			continue
		}
		if load < bestLoad {
			best = d
			bestLoad = load
		}
	}

	return best
}

// ResolveDaemonForAgent returns the daemon owned by the Agent's persisted
// computer binding. Legacy unbound Agents remain supported only when there is
// exactly one usable daemon, so adding a second computer cannot silently move
// their work.
func (dm *DaemonManager) ResolveDaemonForAgent(ctx context.Context, agentID, capability string) (*DaemonInfo, error) {
	if dm.pool == nil {
		if daemon := dm.SelectDaemon(capability); daemon != nil {
			return daemon, nil
		}
		return nil, fmt.Errorf("no available daemon for agent %s", agentID)
	}

	var runtimeID, daemonID string
	var paired bool
	err := dm.pool.QueryRow(ctx, `
		SELECT COALESCE(a.runtime_id, ''), COALESCE(c.daemon_id, ''),
		       COALESCE(c.credential_hash IS NOT NULL AND c.credential_revoked_at IS NULL, false)
		  FROM agents a
		  LEFT JOIN computers c ON c.id::text = a.runtime_id
		 WHERE a.id = $1`, agentID,
	).Scan(&runtimeID, &daemonID, &paired)
	if err != nil {
		return nil, fmt.Errorf("resolve agent computer: %w", err)
	}
	if runtimeID != "" {
		if daemon, ok := dm.GetDaemon(runtimeID); ok && daemon.ComputerID != "" {
			return daemon, nil
		}
		if paired {
			return &DaemonInfo{ID: runtimeID, ComputerID: runtimeID, Status: DaemonStatusOffline, Capabilities: []string{"llm"}, MaxConcurrent: 10}, nil
		}
		if daemonID == "" {
			return nil, fmt.Errorf("agent %s is bound to computer %s without a daemon", agentID, runtimeID)
		}
		daemon, ok := dm.GetDaemon(daemonID)
		if !ok || !daemonUsable(daemon, capability) {
			return nil, fmt.Errorf("agent %s daemon %s is unavailable", agentID, daemonID)
		}
		return daemon, nil
	}

	var only *DaemonInfo
	for _, daemon := range dm.ListDaemons() {
		if !daemonUsable(daemon, capability) {
			continue
		}
		if only != nil {
			return nil, fmt.Errorf("agent %s has no computer binding and multiple daemons are available", agentID)
		}
		only = daemon
	}
	if only == nil {
		return nil, fmt.Errorf("no available daemon for agent %s", agentID)
	}
	if _, err := dm.pool.Exec(ctx, `
		UPDATE agents a
		   SET runtime_id = c.id::text, updated_at = now()
		  FROM computers c
		 WHERE a.id = $1
		   AND a.runtime_id IS NULL
		   AND (c.id::text = $2 OR c.daemon_id = $2)`, agentID, only.ID); err != nil {
		return nil, fmt.Errorf("persist agent computer binding: %w", err)
	}
	return only, nil
}

func daemonUsable(daemon *DaemonInfo, capability string) bool {
	if daemon == nil || daemon.Status != DaemonStatusOnline {
		return false
	}
	if capability == "" {
		return true
	}
	return daemon.CurrentLoad < int32(daemon.MaxConcurrent) && hasCapability(daemon.Capabilities, capability)
}

// SendTask dispatches a task to a specific daemon via HTTP.
func (dm *DaemonManager) SendTask(ctx context.Context, daemon *DaemonInfo, req interface{}) ([]byte, error) {
	url := fmt.Sprintf("http://%s:%d/internal/daemon/run", daemon.Host, daemon.Port)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal task request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create task request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := dm.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send task to daemon: %w", err)
	}
	defer resp.Body.Close()

	var result bytes.Buffer
	if _, err := result.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read task response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, result.String())
	}

	return result.Bytes(), nil
}

// ProxyBackendDetect asks the single online daemon to inspect its own machine.
// Multi-computer selection belongs in the public API once the UI can choose a computer.
func (dm *DaemonManager) ProxyBackendDetect(ctx context.Context) ([]byte, error) {
	var selected *DaemonInfo
	for _, daemon := range dm.ListDaemons() {
		if daemon.Status != DaemonStatusOnline {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("multiple online daemons; computer selection is required")
		}
		selected = daemon
	}
	if selected == nil {
		return nil, fmt.Errorf("no online daemon")
	}
	return dm.proxyBackendDetect(ctx, selected)
}

func (dm *DaemonManager) proxyBackendDetect(ctx context.Context, selected *DaemonInfo) ([]byte, error) {
	if selected.ComputerID != "" {
		return dm.CallControlRPC(ctx, selected.ComputerID, "backend.detect", map[string]any{})
	}

	url := fmt.Sprintf("http://%s:%d/internal/daemon/backends/detect", selected.Host, selected.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create backend detection request: %w", err)
	}
	resp, err := dm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("detect backends on daemon: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read backend detection response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

func (dm *DaemonManager) ProxyBackendDetectForComputer(ctx context.Context, computerID string) ([]byte, error) {
	daemon, ok := dm.GetDaemon(computerID)
	if !ok && dm.pool != nil {
		var daemonID string
		if err := dm.pool.QueryRow(ctx, `
			SELECT COALESCE(daemon_id, '') FROM computers
			 WHERE id = $1 AND status = 'online'`, computerID).Scan(&daemonID); err == nil && daemonID != "" {
			daemon, ok = dm.GetDaemon(daemonID)
		}
	}
	if !ok || daemon.Status != DaemonStatusOnline {
		return nil, ErrComputerOffline
	}
	return dm.proxyBackendDetect(ctx, daemon)
}

// CleanupThinkingSessions broadcasts node process cleanup to every online
// daemon. Runtime affinity is not durable, so the operation is intentionally
// idempotent and fan-out based.
func (dm *DaemonManager) CleanupThinkingSessions(ctx context.Context, nodeIDs []string) error {
	payload, err := json.Marshal(struct {
		NodeIDs []string `json:"node_ids"`
	}{NodeIDs: nodeIDs})
	if err != nil {
		return fmt.Errorf("marshal Thinking cleanup request: %w", err)
	}

	var firstErr error
	for _, daemon := range dm.ListDaemons() {
		if daemon.Status != DaemonStatusOnline {
			continue
		}
		if daemon.ComputerID != "" {
			if _, err := dm.CallControlRPC(ctx, daemon.ComputerID, "thinking.cleanup", map[string]any{"node_ids": nodeIDs}); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		url := fmt.Sprintf("http://%s:%d/internal/daemon/thinking/cleanup", daemon.Host, daemon.Port)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("create Thinking cleanup request: %w", err)
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := dm.httpClient.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("send Thinking cleanup to daemon %s: %w", daemon.ID, err)
			}
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && firstErr == nil {
			firstErr = fmt.Errorf("daemon %s returned status %d on Thinking cleanup", daemon.ID, resp.StatusCode)
		}
	}
	return firstErr
}

// CleanupAgents force-closes every scoped runtime session for the supplied
// Agents and removes their local runtime state from their bound daemon.
func (dm *DaemonManager) CleanupAgents(ctx context.Context, agentIDs []string) error {
	var firstErr error
	for _, agentID := range agentIDs {
		daemon, err := dm.ResolveDaemonForAgent(ctx, agentID, "")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if daemon.ComputerID != "" {
			if _, err := dm.CallControlRPC(ctx, daemon.ComputerID, "agent.cleanup", map[string]string{"agent_id": agentID}); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		url := fmt.Sprintf("http://%s:%d/internal/daemon/agents/%s/cleanup", daemon.Host, daemon.Port, agentID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resp, err := dm.httpClient.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && firstErr == nil {
			firstErr = fmt.Errorf("daemon %s returned status %d while cleaning agent %s", daemon.ID, resp.StatusCode, agentID)
		}
	}
	return firstErr
}

// --- SSE Streaming task support ---

// SSEDaemonEvent represents a single event from the daemon's SSE stream.
type SSEDaemonEvent struct {
	Event string
	Data  string
}

// StreamTask dispatches a task to a daemon and immediately connects to its SSE
// event stream. It returns a channel of SSEDaemonEvent that the caller must
// consume until it is closed (indicating the stream ended).
//
// The caller should cancel ctx to stop reading the stream early.
func (dm *DaemonManager) StreamTask(ctx context.Context, daemon *DaemonInfo, req interface{}) (<-chan SSEDaemonEvent, error) {
	if daemon.ComputerID != "" {
		taskID, err := dm.QueueRemoteRun(ctx, daemon, req)
		if err != nil {
			return nil, fmt.Errorf("queue remote Run: %w", err)
		}
		events, err := dm.SubscribeRemoteTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		var identity struct {
			RunID string `json:"run_id"`
		}
		raw, _ := json.Marshal(req)
		_ = json.Unmarshal(raw, &identity)
		dm.NotifyRun(daemon.ComputerID, identity.RunID)
		return events, nil
	}
	// Send the task first
	taskResp, err := dm.SendTask(ctx, daemon, req)
	if err != nil {
		return nil, fmt.Errorf("send task: %w", err)
	}

	// Parse task_id from response
	var taskResult struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(taskResp, &taskResult); err != nil {
		return nil, fmt.Errorf("parse task response: %w", err)
	}

	taskID := taskResult.TaskID
	if taskID == "" {
		return nil, fmt.Errorf("daemon returned empty task_id")
	}

	return dm.SubscribeTask(ctx, daemon, taskID)
}

// SubscribeTask reconnects to an already-dispatched daemon task. The daemon
// replays lifecycle events, so a restarted server can converge the same Run.
func (dm *DaemonManager) SubscribeTask(ctx context.Context, daemon *DaemonInfo, taskID string) (<-chan SSEDaemonEvent, error) {
	if daemon.ComputerID != "" {
		return dm.SubscribeRemoteTask(ctx, taskID)
	}
	// Connect to SSE endpoint
	eventsURL := fmt.Sprintf("http://%s:%d/internal/daemon/tasks/%s/events", daemon.Host, daemon.Port, taskID)

	sseCtx, cancel := context.WithCancel(ctx)

	httpReq, err := http.NewRequestWithContext(sseCtx, http.MethodGet, eventsURL, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create SSE request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect to SSE stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("daemon returned status %d for task events: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	eventCh := make(chan SSEDaemonEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer cancel()
		defer close(eventCh)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

		var currentEvent SSEDaemonEvent

		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "event: ") {
				currentEvent.Event = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				currentEvent.Data = strings.TrimPrefix(line, "data: ")
			} else if line == "" {
				// Empty line: end of event
				if currentEvent.Event != "" || currentEvent.Data != "" {
					select {
					case eventCh <- currentEvent:
					case <-sseCtx.Done():
						return
					}
				}
				currentEvent = SSEDaemonEvent{}
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			slog.Error("SSE stream scanner error", "task_id", taskID, "error", err)
		}

		// Emit a final done event if we didn't already get one
		if currentEvent.Event != "" || currentEvent.Data != "" {
			select {
			case eventCh <- currentEvent:
			default:
			}
		}
	}()

	return eventCh, nil
}

// CancelTask sends a cancel request to the daemon for a specific task.
func (dm *DaemonManager) CancelTask(ctx context.Context, daemon *DaemonInfo, taskID string) error {
	if daemon.ComputerID != "" {
		_, err := dm.CallControlRPC(ctx, daemon.ComputerID, "task.cancel", map[string]string{"task_id": taskID})
		return err
	}
	url := fmt.Sprintf("http://%s:%d/internal/daemon/tasks/%s/cancel", daemon.Host, daemon.Port, taskID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create cancel request: %w", err)
	}

	resp, err := dm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send cancel request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned status %d on cancel", resp.StatusCode)
	}

	return nil
}

// ---- workspace.Proxy implementation ----

// FindDaemonForAgent finds the Agent's bound online daemon for workspace files.
func (dm *DaemonManager) FindDaemonForAgent(ctx context.Context, agentID string) (*workspace.Daemon, bool) {
	daemon, err := dm.ResolveDaemonForAgent(ctx, agentID, "")
	if err != nil {
		return nil, false
	}
	return &workspace.Daemon{Host: daemon.Host, Port: daemon.Port, ComputerID: daemon.ComputerID}, true
}

// ProxyWorkspaceList sends a workspace list request to a daemon.
func (dm *DaemonManager) ProxyWorkspaceList(ctx context.Context, daemon *workspace.Daemon, agentID, path string) ([]byte, error) {
	if daemon.ComputerID != "" {
		return dm.CallControlRPC(ctx, daemon.ComputerID, "workspace.list", map[string]string{"agent_id": agentID, "path": path})
	}
	params := url.Values{}
	params.Set("agent_id", agentID)
	params.Set("path", path)
	urlStr := fmt.Sprintf("http://%s:%d/internal/daemon/workspace/list?%s",
		daemon.Host, daemon.Port, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy workspace list: %w", err)
	}

	resp, err := dm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy workspace list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy workspace list: daemon returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB cap
}

// ProxyWorkspaceRead sends a workspace read request to a daemon.
func (dm *DaemonManager) ProxyWorkspaceRead(ctx context.Context, daemon *workspace.Daemon, agentID, path string) ([]byte, error) {
	if daemon.ComputerID != "" {
		return dm.CallControlRPC(ctx, daemon.ComputerID, "workspace.read", map[string]string{"agent_id": agentID, "path": path})
	}
	params := url.Values{}
	params.Set("agent_id", agentID)
	params.Set("path", path)
	urlStr := fmt.Sprintf("http://%s:%d/internal/daemon/workspace/read?%s",
		daemon.Host, daemon.Port, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy workspace read: %w", err)
	}

	resp, err := dm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy workspace read: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy workspace read: daemon returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB cap
}

// ProxySkillList sends a skill list request to a daemon.
func (dm *DaemonManager) ProxySkillList(ctx context.Context, daemon *workspace.Daemon, agentID string) ([]byte, error) {
	if daemon.ComputerID != "" {
		var provider string
		if err := dm.pool.QueryRow(ctx, `SELECT COALESCE(model_provider, '') FROM agents WHERE id = $1 AND is_active = true`, agentID).Scan(&provider); err != nil {
			return nil, err
		}
		return dm.CallControlRPC(ctx, daemon.ComputerID, "skills.list", map[string]string{"agent_id": agentID, "provider": provider})
	}
	params := url.Values{}
	params.Set("agent_id", agentID)
	urlStr := fmt.Sprintf("http://%s:%d/internal/daemon/skills?%s",
		daemon.Host, daemon.Port, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("proxy skill list: %w", err)
	}

	resp, err := dm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy skill list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy skill list: daemon returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB cap
}

// --- Health check loop ---

// healthCheckLoop runs periodically to mark daemons as offline after missed heartbeats.
func (dm *DaemonManager) healthCheckLoop() {
	ticker := time.NewTicker(dm.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dm.checkHealth()
		case <-dm.stopCh:
			return
		}
	}
}

func (dm *DaemonManager) checkHealth() {
	var lostTasks []PendingTaskInfo

	dm.mu.Lock()
	now := time.Now()
	for id, info := range dm.daemons {
		sinceHB := now.Sub(info.LastHeartbeat)
		if sinceHB <= dm.heartbeatInterval {
			continue
		}

		info.MissedHeartbeats++
		slog.Warn("daemon missed heartbeat",
			"daemon_id", id,
			"missed", info.MissedHeartbeats,
			"since_hb", sinceHB,
			"heartbeat_interval", dm.heartbeatInterval,
		)

		if info.MissedHeartbeats < dm.maxMissedHB {
			continue
		}

		// Mark as offline
		info.Status = DaemonStatusOffline
		slog.Warn("daemon marked as offline",
			"daemon_id", id,
			"missed_heartbeats", info.MissedHeartbeats,
		)

		// Remote tasks are durable in PostgreSQL and survive control-socket
		// disconnects. Legacy inbound daemons still fail their in-memory tasks.
		if info.ComputerID != "" {
			continue
		}

		// Clean up pending tasks for this legacy daemon.
		cleanedCount := 0
		for taskID, task := range dm.pendingTasks {
			if task.DaemonID == id {
				lostTasks = append(lostTasks, task)
				delete(dm.pendingTasks, taskID)
				cleanedCount++
			}
		}
		if cleanedCount > 0 {
			slog.Info("cleaned up pending tasks for offline daemon",
				"daemon_id", id,
				"task_count", cleanedCount,
			)
		}
	}

	// Remove tasks that have exceeded their phase-specific timeout threshold.
	timedOutTasks := dm.removeStaleTasks(now)
	dm.mu.Unlock()

	for _, task := range lostTasks {
		dm.daemonLostPendingTaskRun(task)
	}
	for _, task := range timedOutTasks {
		dm.timeoutPendingTaskRun(task)
	}
}

// removeStaleTasks removes pending tasks that have exceeded the timeout for
// their current phase.
func (dm *DaemonManager) removeStaleTasks(now time.Time) []PendingTaskInfo {
	cleaned := 0
	timedOutTasks := make([]PendingTaskInfo, 0)
	for taskID, task := range dm.pendingTasks {
		deadlineBase := task.CreatedAt
		timeout := dm.queueTimeout
		phase := "queue"
		if task.BackendStartedAt != nil {
			deadlineBase = *task.BackendStartedAt
			timeout = dm.executionTimeout
			phase = "execution"
		}
		if daemon := dm.daemons[task.DaemonID]; daemon != nil && daemon.ComputerID != "" && task.BackendStartedAt == nil {
			timeout = remoteRunDeliveryTTL
		}
		if now.Sub(deadlineBase) > timeout {
			task.TimeoutPhase = phase
			timedOutTasks = append(timedOutTasks, task)
			delete(dm.pendingTasks, taskID)
			cleaned++
		}
	}
	if cleaned > 0 {
		slog.Warn("cleaned up stale pending tasks",
			"task_count", cleaned,
			"queue_timeout", dm.queueTimeout,
			"execution_timeout", dm.executionTimeout,
		)
	}
	return timedOutTasks
}

func (dm *DaemonManager) timeoutPendingTaskRun(task PendingTaskInfo) {
	activity := agentActivityTimeout
	if task.TimeoutPhase == "queue" {
		activity = agentActivityQueueTimeout
	}
	dm.finishPendingTaskRun(task, AgentRunStatusTimeout, activity, agentFailureTimeout, true)
}

func (dm *DaemonManager) daemonLostPendingTaskRun(task PendingTaskInfo) {
	dm.finishPendingTaskRun(task, AgentRunStatusFailed, agentActivityDaemonLost, agentFailureDaemonLost, true)
}

func (dm *DaemonManager) finishPendingTaskRun(task PendingTaskInfo, status AgentRunStatus, activity, failureCode string, retryable bool) {
	if task.RunID == "" || dm.pool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if daemon, ok := dm.GetDaemon(task.DaemonID); ok && daemon.Status == DaemonStatusOnline {
		if err := dm.CancelTask(ctx, daemon, task.TaskID); err != nil {
			retryable = false
			slog.Warn("failed to stop daemon task before terminal transition", "task_id", task.TaskID, "daemon_id", task.DaemonID, "error", err)
		}
	}

	runSvc := NewAgentRunService(dm.pool)
	run, err := runSvc.GetRun(ctx, task.RunID)
	if err != nil {
		slog.Warn("failed to load pending task run", "task_id", task.TaskID, "run_id", task.RunID, "error", err)
		return
	}
	if !isActiveAgentRunStatus(run.Status) {
		return
	}
	finished, err := runSvc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       status,
		ActivityText: activity,
	})
	if errors.Is(err, ErrAgentRunAlreadyFinished) {
		return
	}
	if err != nil {
		slog.Warn("failed to finish pending task run", "task_id", task.TaskID, "run_id", task.RunID, "error", err)
		return
	}
	event, eventErr := runSvc.AppendEvent(ctx, AppendRunEventInput{
		RunID:   finished.ID,
		Type:    AgentRunEventError,
		Message: activity,
		Payload: map[string]any{
			"status":       status,
			"failure_code": failureCode,
			"retryable":    retryable,
		},
	})
	if eventErr != nil {
		slog.Warn("failed to append pending task failure event", "task_id", task.TaskID, "run_id", task.RunID, "error", eventErr)
	}
	if dm.hub != nil {
		if event != nil {
			dm.hub.Broadcast(realtime.Envelope("agent.run.event", map[string]any{
				"id":         event.ID,
				"run_id":     finished.ID,
				"agent_id":   finished.AgentID,
				"agent_name": finished.AgentName,
				"channel_id": finished.ChannelID,
				"thread_id":  finished.ThreadID,
				"seq":        event.Seq,
				"event_type": event.Type,
				"message":    event.Message,
				"payload":    json.RawMessage(event.Payload),
				"timestamp":  event.CreatedAt.UTC().Format(time.RFC3339),
			}))
		}
		dm.hub.Broadcast(realtime.Envelope("agent.run.finished", runPayload(finished, finished.AgentID, finished.AgentName, "")))
	}
}

func hasCapability(caps []string, cap string) bool {
	for _, c := range caps {
		if c == cap {
			return true
		}
	}
	return false
}

// --- Daemon Register/Heartbeat request types ---

// DaemonSystemInfo is the system info reported by the daemon.
type DaemonSystemInfo struct {
	OS       string `json:"os"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

type DaemonRegisterRequest struct {
	DaemonID      string           `json:"daemon_id"`
	Host          string           `json:"host"`
	Port          int              `json:"port"`
	Version       string           `json:"version"`
	Capabilities  []string         `json:"capabilities"`
	MaxConcurrent int              `json:"max_concurrent"`
	CurrentLoad   int32            `json:"current_load"`
	AgentTypes    []string         `json:"agent_types"`
	SystemInfo    DaemonSystemInfo `json:"system_info"`
	Tasks         []string         `json:"tasks,omitempty"`
}

type DaemonRegisterResponse struct {
	Status            string `json:"status"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
}

type DaemonHeartbeatRequest struct {
	DaemonID    string           `json:"daemon_id"`
	Load        int32            `json:"load"`
	MaxLoad     int              `json:"max_load"`
	UptimeSec   int64            `json:"uptime_seconds"`
	ActiveTasks []string         `json:"active_tasks"`
	AgentIDs    []string         `json:"agent_ids,omitempty"`
	SystemInfo  DaemonSystemInfo `json:"system_info"`
}

type DaemonHeartbeatResponse struct {
	Status       string   `json:"status"`
	PendingTasks []string `json:"pending_tasks,omitempty"`
}

// --- Task callback types (Daemon -> Server) ---

type TaskCompleteRequest struct {
	TaskID    string `json:"task_id"`
	AgentID   string `json:"agent_id"`
	ChannelID string `json:"channel_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	Content   string `json:"content"`
	MessageID string `json:"message_id"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

type TaskErrorRequest struct {
	TaskID    string `json:"task_id"`
	AgentID   string `json:"agent_id"`
	ChannelID string `json:"channel_id"`
	Error     string `json:"error"`
}

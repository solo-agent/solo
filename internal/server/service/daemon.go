package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	TaskID    string    `json:"task_id"`
	AgentID   string    `json:"agent_id"`
	DaemonID  string    `json:"daemon_id"`
	RunID     string    `json:"run_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// DaemonInfo holds runtime state for a registered daemon instance.
type DaemonInfo struct {
	ID               string       `json:"daemon_id"`
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

	// taskTimeout dictates how long a pending task can remain without
	// completion before it is considered stale and removed.
	taskTimeout time.Duration

	stopCh chan struct{}
}

// NewDaemonManager creates a new DaemonManager.
func NewDaemonManager(pool *pgxpool.Pool, hub realtime.Broadcaster) *DaemonManager {
	return &DaemonManager{
		daemons:           make(map[string]*DaemonInfo),
		pendingTasks:      make(map[string]PendingTaskInfo),
		pool:              pool,
		hub:               hub,
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		heartbeatInterval: 30 * time.Second,
		maxMissedHB:       3,
		taskTimeout:       6 * time.Minute,
		stopCh:            make(chan struct{}),
	}
}

// Start begins the heartbeat monitoring goroutine.
func (dm *DaemonManager) Start() {
	go dm.healthCheckLoop()
	slog.Info("daemon manager started", "heartbeat_interval", dm.heartbeatInterval, "max_missed", dm.maxMissedHB)
}

// Stop stops the heartbeat monitoring goroutine.
func (dm *DaemonManager) Stop() {
	close(dm.stopCh)
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
		dm.timeoutPendingTaskRun(task)
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

// FindDaemonForAgent finds an online daemon that can serve workspace files.
// TODO: implement agent-to-daemon affinity when persistent agent-daemon
// assignment is available. Currently returns the first online daemon,
// which is correct for single-daemon deployments.
func (dm *DaemonManager) FindDaemonForAgent(ctx context.Context, agentID string) (*workspace.Daemon, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, d := range dm.daemons {
		if d.Status != DaemonStatusOnline {
			continue
		}
		return &workspace.Daemon{
			Host: d.Host,
			Port: d.Port,
		}, true
	}
	return nil, false
}

// ProxyWorkspaceList sends a workspace list request to a daemon.
func (dm *DaemonManager) ProxyWorkspaceList(ctx context.Context, daemon *workspace.Daemon, agentID, path string) ([]byte, error) {
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
	var timedOutTasks []PendingTaskInfo

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

		// Clean up pending tasks for this daemon
		cleanedCount := 0
		for taskID, task := range dm.pendingTasks {
			if task.DaemonID == id {
				timedOutTasks = append(timedOutTasks, task)
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

	// Remove tasks that have exceeded the timeout threshold.
	timedOutTasks = append(timedOutTasks, dm.removeStaleTasks(now)...)
	dm.mu.Unlock()

	for _, task := range timedOutTasks {
		dm.timeoutPendingTaskRun(task)
	}
}

// removeStaleTasks removes pending tasks that have exceeded the timeout duration.
func (dm *DaemonManager) removeStaleTasks(now time.Time) []PendingTaskInfo {
	cleaned := 0
	timedOutTasks := make([]PendingTaskInfo, 0)
	for taskID, task := range dm.pendingTasks {
		if now.Sub(task.CreatedAt) > dm.taskTimeout {
			timedOutTasks = append(timedOutTasks, task)
			delete(dm.pendingTasks, taskID)
			cleaned++
		}
	}
	if cleaned > 0 {
		slog.Warn("cleaned up stale pending tasks",
			"task_count", cleaned,
			"timeout", dm.taskTimeout,
		)
	}
	return timedOutTasks
}

func (dm *DaemonManager) timeoutPendingTaskRun(task PendingTaskInfo) {
	if task.RunID == "" || dm.pool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runSvc := NewAgentRunService(dm.pool)
	run, err := runSvc.GetRun(ctx, task.RunID)
	if err != nil {
		slog.Warn("failed to load pending task run for timeout", "task_id", task.TaskID, "run_id", task.RunID, "error", err)
		return
	}
	if !isActiveAgentRunStatus(run.Status) {
		return
	}
	finished, err := runSvc.FinishRun(ctx, FinishRunInput{
		RunID:        run.ID,
		Status:       AgentRunStatusTimeout,
		ActivityText: agentActivityTimeout,
	})
	if err != nil {
		slog.Warn("failed to timeout pending task run", "task_id", task.TaskID, "run_id", task.RunID, "error", err)
		return
	}
	if dm.hub != nil {
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

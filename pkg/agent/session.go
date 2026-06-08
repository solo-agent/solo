package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// failedStartRetryInterval is how long to wait before retrying after a process
// exits immediately (indicating a CLI misconfiguration, not a transient failure).
const failedStartRetryInterval = 30 * time.Second

// AgentSessionManager manages a pool of agent sessions 
// processes stay alive indefinitely (no idle timeout), crash recovery is
// automatic via --resume, and concurrent starts are rate-limited.
type AgentSessionManager struct {
	backend      PersistentBackend
	workspaceMgr *WorkspaceManager
	memoryMgr    *MemoryManager
	logger       *slog.Logger

	sessions map[string]*agentSessionEntry
	mu       sync.RWMutex

	activeTurns map[string]chan struct{}
	turnsMu     sync.Mutex

	// pendingMessages holds messages queued while an agent is busy.
	// Used for freshness hold: before an agent's reply is persisted,
	// we flush pending messages and let the agent revise.
	pendingMessages map[string][]Message
	pendingMu       sync.Mutex

	// startSlots limits concurrent agent process starts to prevent CPU
	// spikes when multiple agents are triggered at once.
	startSlots chan struct{}

	// failedStarts tracks timestamps of recent failed session creations
	// to prevent retry storms when the CLI is misconfigured or missing.
	failedStarts   map[string]time.Time
	failedStartsMu sync.Mutex
}

// agentSessionEntry wraps a session with lifetime metadata.
type agentSessionEntry struct {
	AgentID     string
	Session     *PersistentSession // nil when asleep
	AgentConfig AgentConfig
	ChannelCtx  ChannelContext
	CreatedAt   time.Time
	LastActive  time.Time
	sessionID   string // preserved across sleep/wake for --resume
	asleep      bool
}

// NewAgentSessionManager creates a new session manager.
func NewAgentSessionManager(backend PersistentBackend, workspaceMgr *WorkspaceManager, memoryMgr *MemoryManager, logger *slog.Logger) *AgentSessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	slots := make(chan struct{}, 5) // max 5 concurrent starts
	return &AgentSessionManager{
		backend:      backend,
		workspaceMgr: workspaceMgr,
		memoryMgr:    memoryMgr,
		logger:       logger,
		sessions:        make(map[string]*agentSessionEntry),
		activeTurns:     make(map[string]chan struct{}),
		pendingMessages: make(map[string][]Message),
		startSlots:      slots,
		failedStarts:    make(map[string]time.Time),
	}
}

// GetOrCreateSession returns an existing session or creates one.
// Asleep sessions are automatically woken via --resume.
func (m *AgentSessionManager) GetOrCreateSession(ctx context.Context, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, initialMessages []Message, mentionedNames []string) (*PersistentSession, error) {
	m.mu.RLock()
	entry, exists := m.sessions[agentID]
	m.mu.RUnlock()

	if exists {
		if m.isSessionAlive(entry) {
			return m.deliverToSession(ctx, agentID, entry, initialMessages)
		}
		return m.createSession(ctx, agentID, agentCfg, channelCtx, initialMessages, entry.sessionID, mentionedNames)
	}

	// Check retry cooldown for agents with recent failed starts.
	if m.inFailedCooldown(agentID) {
		m.logger.Warn("session: skipping start, in cooldown after recent failure", "agent_id", agentID)
		return nil, fmt.Errorf("session start cooldown for agent %s", agentID)
	}

	return m.createSession(ctx, agentID, agentCfg, channelCtx, initialMessages, "", mentionedNames)
}

// DeliverMessage sends a message to an active session.
func (m *AgentSessionManager) DeliverMessage(ctx context.Context, agentID string, messages []Message) (*PersistentSession, error) {
	m.mu.RLock()
	entry, exists := m.sessions[agentID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no session for agent %s", agentID)
	}
	if entry.asleep {
		return nil, fmt.Errorf("session for agent %s is asleep — use GetOrCreateSession to wake", agentID)
	}
	if !m.isSessionAlive(entry) {
		return nil, fmt.Errorf("session for agent %s has exited", agentID)
	}

	return m.deliverToSession(ctx, agentID, entry, messages)
}

// QueueIfBusy attempts to deliver a message. If the agent is currently
// processing another turn, the message is queued for freshness hold instead
// of blocking. Returns true if the message was queued.
func (m *AgentSessionManager) QueueIfBusy(agentID string, msg Message) bool {
	m.turnsMu.Lock()
	ch, exists := m.activeTurns[agentID]
	if !exists {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		m.activeTurns[agentID] = ch
	}
	m.turnsMu.Unlock()

	// Non-blocking try: if turn is available, message would have been
	// delivered directly. If not available, queue for freshness hold.
	select {
	case <-ch:
		// Turn is free — release and let caller deliver normally.
		ch <- struct{}{}
		return false
	default:
		// Turn is held — queue the message.
		m.pendingMu.Lock()
		m.pendingMessages[agentID] = append(m.pendingMessages[agentID], msg)
		count := len(m.pendingMessages[agentID])
		m.pendingMu.Unlock()
		m.logger.Info("session: message queued", "agent_id", agentID, "pending_count", count)
		// v1.3: Write inbox notification to agent stdin.
		m.notifyInbox(agentID, count)
		return true
	}
}

// FlushPending returns and clears all pending messages for an agent.
// Called after a turn completes to check if newer messages arrived.
func (m *AgentSessionManager) FlushPending(agentID string) []Message {
	m.pendingMu.Lock()
	msgs := m.pendingMessages[agentID]
	delete(m.pendingMessages, agentID)
	m.pendingMu.Unlock()
	return msgs
}

// IsActive returns true if the agent has a running (non-asleep) session.
func (m *AgentSessionManager) IsActive(agentID string) bool {
	m.mu.RLock()
	entry, exists := m.sessions[agentID]
	m.mu.RUnlock()
	return exists && !entry.asleep && m.isSessionAlive(entry)
}

// CloseSession terminates a session (active or asleep).
func (m *AgentSessionManager) CloseSession(agentID string) error {
	m.mu.Lock()
	entry, exists := m.sessions[agentID]
	if exists {
		delete(m.sessions, agentID)
	}
	m.mu.Unlock()

	if !exists {
		return nil
	}

	m.logger.Info("session: closing", "agent_id", agentID)
	if !entry.asleep && entry.Session != nil {
		return m.backend.Close(entry.Session)
	}
	return nil
}

// CloseAll terminates all sessions.
func (m *AgentSessionManager) CloseAll() {
	m.mu.Lock()
	entries := make([]*agentSessionEntry, 0, len(m.sessions))
	for id, entry := range m.sessions {
		entries = append(entries, entry)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, entry := range entries {
		if !entry.asleep && entry.Session != nil {
			m.logger.Info("session: closing (shutdown)", "agent_id", entry.AgentID)
			_ = m.backend.Close(entry.Session)
		}
	}
}


// ── Internal ─────────────────────────────────────────────────────────────────

func (m *AgentSessionManager) isSessionAlive(entry *agentSessionEntry) bool {
	if entry.asleep || entry.Session == nil {
		return false
	}
	state, ok := entry.Session.state.(SessionStater)
	if !ok {
		return false
	}
	return state.IsAlive()
}

func (m *AgentSessionManager) deliverToSession(ctx context.Context, agentID string, entry *agentSessionEntry, messages []Message) (*PersistentSession, error) {
	release := m.acquireTurn(agentID)
	defer release()

	m.logger.Info("session: delivering message", "agent_id", agentID)
	ps, err := m.backend.Send(ctx, entry.Session, messages)
	if err != nil {
		m.logger.Error("session: Send failed", "agent_id", agentID, "error", err)
		m.mu.Lock()
		delete(m.sessions, agentID)
		m.mu.Unlock()
		return nil, err
	}

	entry.LastActive = time.Now()
	entry.Session = ps
	if state, ok := ps.state.(SessionStater); ok {
		if sid := state.SessionID(); sid != "" {
			entry.sessionID = sid
		}
	}
	return ps, nil
}


func (m *AgentSessionManager) createSession(ctx context.Context, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, messages []Message, prevSessionID string, mentionedNames []string) (*PersistentSession, error) {
	release := m.acquireTurn(agentID)
	defer release()

	m.mu.RLock()
	entry, exists := m.sessions[agentID]
	m.mu.RUnlock()
	if exists && m.isSessionAlive(entry) {
		return m.backend.Send(ctx, entry.Session, messages)
	}

	m.logger.Info("session: creating", "agent_id", agentID, "resume", prevSessionID)

	ws, err := m.workspaceMgr.Prepare(agentID, nil)
	if err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}

	memoryContent := ""
	if m.memoryMgr != nil {
		memoryContent, _ = m.memoryMgr.Load(agentID)
	}

	systemPrompt := BuildSystemPrompt(agentCfg, channelCtx, memoryContent, mentionedNames)

	executeReq := &ExecuteRequest{
		AgentID:  agentID,
		Messages: messages,
	}
	executeOpts := &ExecuteOptions{
		SystemPrompt: systemPrompt,
		WorkspaceDir: ws.WorkDir,
		Model:        agentCfg.Model,
		MaxTokens:    agentCfg.MaxTokens,
		Temperature:  agentCfg.Temperature,
		Env:          agentCfg.Env,
		CustomArgs:   agentCfg.CustomArgs,
	}
	if prevSessionID != "" {
		executeOpts.CustomArgs = append(executeOpts.CustomArgs, "--resume", prevSessionID)
		executeOpts.ResumeSessionID = prevSessionID
	}

	// v1.3: Rate-limit concurrent starts to prevent CPU spikes when
	// multiple agents are triggered simultaneously (
	// max 5 concurrent, FIFO queue with 500ms dequeue interval).
	queueLen := len(m.startSlots)
	m.logger.Info("session: start queued", "agent_id", agentID, "queue", queueLen, "max", cap(m.startSlots))
	m.startSlots <- struct{}{} // acquire slot (blocks if 5 already starting)
	time.Sleep(500 * time.Millisecond)            // dequeue interval
	m.logger.Info("session: dequeued start", "agent_id", agentID, "queue", len(m.startSlots))

	ps, err := m.backend.Start(ctx, executeReq, executeOpts)
	<-m.startSlots // release slot
	if err != nil {
		return nil, fmt.Errorf("start persistent session: %w", err)
	}

	entry = &agentSessionEntry{
		AgentID:     agentID,
		Session:     ps,
		AgentConfig: agentCfg,
		ChannelCtx:  channelCtx,
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
		sessionID:   prevSessionID,
	}

	m.mu.Lock()
	if old, ok := m.sessions[agentID]; ok && !old.asleep && old.Session != nil {
		_ = m.backend.Close(old.Session)
	}
	m.sessions[agentID] = entry
	m.mu.Unlock()

	go m.watchCrash(agentID, agentCfg, channelCtx, entry)

	// If the process died immediately (CLI broken/missing), record failure
	// to prevent retry storms on the next trigger.
	if !m.isSessionAlive(entry) {
		m.failedStartsMu.Lock()
		m.failedStarts[agentID] = time.Now()
		m.failedStartsMu.Unlock()
		m.logger.Warn("session: created but process died immediately, cooling down", "agent_id", agentID)
	}

	m.logger.Info("session: created", "agent_id", agentID)
	return ps, nil
}

// inFailedCooldown returns true if this agent had a failed start recently
// and should not be retried yet.
func (m *AgentSessionManager) inFailedCooldown(agentID string) bool {
	m.failedStartsMu.Lock()
	defer m.failedStartsMu.Unlock()
	t, ok := m.failedStarts[agentID]
	if !ok {
		return false
	}
	if time.Since(t) > failedStartRetryInterval {
		delete(m.failedStarts, agentID)
		return false
	}
	return true
}

// watchCrash monitors a session. On unexpected exit (crash, not sleep),
// auto-restarts with --resume to recover context.
func (m *AgentSessionManager) watchCrash(agentID string, agentCfg AgentConfig, channelCtx ChannelContext, entry *agentSessionEntry) {
	state, ok := entry.Session.state.(SessionStater)
	if !ok {
		return
	}

	<-state.Done()

	m.mu.RLock()
	currentEntry, exists := m.sessions[agentID]
	m.mu.RUnlock()
	if !exists || currentEntry != entry || entry.asleep {
		return
	}

	resumeID := entry.sessionID
	if sid := state.SessionID(); sid != "" {
		resumeID = sid
	}

	m.logger.Warn("session: crashed, auto-restarting",
		"agent_id", agentID,
		"session_id", resumeID,
	)

	m.mu.Lock()
	delete(m.sessions, agentID)
	m.mu.Unlock()

	restartMsg := Message{
		Role:    RoleUser,
		Content: "Your session has been restored after a restart. Context is preserved via --resume. Continue from where you left off.",
	}

	_, err := m.createSession(context.Background(), agentID, agentCfg, channelCtx, []Message{restartMsg}, resumeID, nil)
	if err != nil {
		m.logger.Error("session: crash recovery failed", "agent_id", agentID, "error", err)
	}
}



// notifyInbox writes a lightweight notification to the agent's stdin,
// "1 pending inbox message(s)" pattern. The agent sees
// this notification at the start of its next stdin read and can call
// solo message check to pull the actual content.
func (m *AgentSessionManager) notifyInbox(agentID string, count int) {
	m.mu.RLock()
	entry, exists := m.sessions[agentID]
	m.mu.RUnlock()
	if !exists || entry.asleep || entry.Session == nil {
		return
	}
	state, ok := entry.Session.state.(SessionStater)
	if !ok {
		return
	}
	notification := fmt.Sprintf("\n[Solo] %d pending message(s). Use `solo message check` when ready.\n", count)
	_ = state.Notify(notification)
}

func (m *AgentSessionManager) acquireTurn(agentID string) func() {
	m.turnsMu.Lock()
	ch, exists := m.activeTurns[agentID]
	if !exists {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		m.activeTurns[agentID] = ch
	}
	m.turnsMu.Unlock()

	<-ch

	return func() {
		ch <- struct{}{}
	}
}

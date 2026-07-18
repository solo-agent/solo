package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// failedStartRetryInterval is how long to wait before retrying after a process
// exits immediately (indicating a CLI misconfiguration, not a transient failure).
const failedStartRetryInterval = 30 * time.Second

// AgentSessionManager manages a pool of Agent and Thinking-node sessions.
// Crash recovery is automatic via --resume, concurrent starts are rate-limited,
// and callers may explicitly sleep idle Thinking sessions without affecting
// ordinary Agent session lifetime.
type AgentSessionManager struct {
	backend      PersistentBackend
	workspaceMgr *WorkspaceManager
	memoryMgr    *MemoryManager
	logger       *slog.Logger

	sessions map[string]*agentSessionEntry
	mu       sync.RWMutex

	activeTurns       map[string]chan struct{}
	agentTurns        map[string]chan struct{}
	activeAgentScopes map[string]string
	turnsMu           sync.Mutex

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
	mu          sync.RWMutex
	SessionKey  string
	AgentID     string
	Session     *PersistentSession // nil when asleep
	AgentConfig AgentConfig
	ChannelCtx  ChannelContext
	CreatedAt   time.Time
	LastActive  time.Time
	sessionID   string // preserved across sleep/wake for --resume
	asleep      bool
}

func (e *agentSessionEntry) snapshot() (*PersistentSession, bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Session, e.asleep, e.sessionID
}

func (e *agentSessionEntry) updateSession(ps *PersistentSession) {
	sessionID := ""
	if state, ok := ps.state.(SessionStater); ok {
		sessionID = state.SessionID()
	}
	e.mu.Lock()
	e.LastActive = time.Now()
	e.Session = ps
	if sessionID != "" {
		e.sessionID = sessionID
	}
	e.mu.Unlock()
}

// NewAgentSessionManager creates a new session manager.
func NewAgentSessionManager(backend PersistentBackend, workspaceMgr *WorkspaceManager, memoryMgr *MemoryManager, logger *slog.Logger) *AgentSessionManager {
	if logger == nil {
		logger = slog.Default()
	}
	slots := make(chan struct{}, 5) // max 5 concurrent starts
	return &AgentSessionManager{
		backend:           backend,
		workspaceMgr:      workspaceMgr,
		memoryMgr:         memoryMgr,
		logger:            logger,
		sessions:          make(map[string]*agentSessionEntry),
		activeTurns:       make(map[string]chan struct{}),
		agentTurns:        make(map[string]chan struct{}),
		activeAgentScopes: make(map[string]string),
		pendingMessages:   make(map[string][]Message),
		startSlots:        slots,
		failedStarts:      make(map[string]time.Time),
	}
}

// AgentSessionKey is the stable pool key for Solo's existing Agent-wide session.
func AgentSessionKey(agentID string) string {
	return "agent:" + agentID
}

// ThinkingSessionKey is the stable pool key for one isolated Thinking node session.
func ThinkingSessionKey(nodeID string) string {
	return "thinking:" + nodeID
}

// ActiveThinkingNodeID returns the Thinking node whose scoped Session
// currently owns this Agent's turn. The value is runtime-owned routing state,
// not metadata supplied by the Agent or its CLI.
func (m *AgentSessionManager) ActiveThinkingNodeID(agentID string) (string, bool) {
	m.turnsMu.Lock()
	defer m.turnsMu.Unlock()
	sessionKey := m.activeAgentScopes[agentID]
	nodeID, ok := strings.CutPrefix(sessionKey, "thinking:")
	return nodeID, ok && nodeID != ""
}

// GetOrCreateSession returns an existing session or creates one.
// Asleep sessions are automatically woken via --resume.
func (m *AgentSessionManager) GetOrCreateSession(ctx context.Context, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, initialMessages []Message, mentionedNames []string) (*PersistentSession, error) {
	return m.GetOrCreateScopedSession(ctx, AgentSessionKey(agentID), agentID, agentCfg, channelCtx, initialMessages, "", mentionedNames)
}

// GetOrCreateScopedSession returns or creates a persistent session whose pool
// identity is independent from the Agent identity that owns the runtime.
func (m *AgentSessionManager) GetOrCreateScopedSession(ctx context.Context, sessionKey, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, initialMessages []Message, resumeSessionID string, mentionedNames []string) (*PersistentSession, error) {
	m.mu.RLock()
	entry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()

	if exists {
		if m.isSessionAlive(entry) {
			return m.deliverToSession(ctx, sessionKey, entry, initialMessages)
		}
		_, _, resumeID := entry.snapshot()
		return m.createSession(ctx, sessionKey, agentID, agentCfg, channelCtx, initialMessages, firstNonEmpty(resumeID, resumeSessionID), mentionedNames)
	}

	// Check retry cooldown for agents with recent failed starts.
	if m.inFailedCooldown(sessionKey) {
		m.logger.Warn("session: skipping start, in cooldown after recent failure", "agent_id", agentID, "session_key", sessionKey)
		return nil, fmt.Errorf("session start cooldown for %s", sessionKey)
	}

	return m.createSession(ctx, sessionKey, agentID, agentCfg, channelCtx, initialMessages, resumeSessionID, mentionedNames)
}

// DeliverMessage sends a message to an active session.
func (m *AgentSessionManager) DeliverMessage(ctx context.Context, agentID string, messages []Message) (*PersistentSession, error) {
	return m.DeliverScopedMessage(ctx, AgentSessionKey(agentID), messages)
}

// DeliverScopedMessage sends a message to an active scoped session.
func (m *AgentSessionManager) DeliverScopedMessage(ctx context.Context, sessionKey string, messages []Message) (*PersistentSession, error) {
	m.mu.RLock()
	entry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no session for %s", sessionKey)
	}
	_, asleep, _ := entry.snapshot()
	if asleep {
		return nil, fmt.Errorf("session %s is asleep — use GetOrCreateScopedSession to wake", sessionKey)
	}
	if !m.isSessionAlive(entry) {
		return nil, fmt.Errorf("session %s has exited", sessionKey)
	}

	return m.deliverToSession(ctx, sessionKey, entry, messages)
}

// QueueIfBusy attempts to deliver a message. If the agent is currently
// processing another turn, the message is queued for freshness hold instead
// of blocking. Returns true if the message was queued.
func (m *AgentSessionManager) QueueIfBusy(agentID string, msg Message) bool {
	return m.QueueScopedIfBusy(AgentSessionKey(agentID), msg)
}

// QueueScopedIfBusy queues a message when the scoped session is in a turn.
func (m *AgentSessionManager) QueueScopedIfBusy(sessionKey string, msg Message) bool {
	m.turnsMu.Lock()
	ch, exists := m.activeTurns[sessionKey]
	if !exists {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		m.activeTurns[sessionKey] = ch
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
		m.pendingMessages[sessionKey] = append(m.pendingMessages[sessionKey], msg)
		count := len(m.pendingMessages[sessionKey])
		m.pendingMu.Unlock()
		m.logger.Info("session: message queued", "session_key", sessionKey, "pending_count", count)
		// v1.3: Write inbox notification to agent stdin.
		m.notifyInbox(sessionKey, count)
		return true
	}
}

// FlushPending returns and clears all pending messages for an agent.
// Called after a turn completes to check if newer messages arrived.
func (m *AgentSessionManager) FlushPending(agentID string) []Message {
	return m.FlushScopedPending(AgentSessionKey(agentID))
}

// FlushScopedPending returns and clears queued messages for a scoped session.
func (m *AgentSessionManager) FlushScopedPending(sessionKey string) []Message {
	m.pendingMu.Lock()
	msgs := m.pendingMessages[sessionKey]
	delete(m.pendingMessages, sessionKey)
	m.pendingMu.Unlock()
	return msgs
}

// IsActive returns true if the agent has a running (non-asleep) session.
func (m *AgentSessionManager) IsActive(agentID string) bool {
	return m.IsScopedActive(AgentSessionKey(agentID))
}

// IsScopedActive reports whether a scoped persistent process is alive.
func (m *AgentSessionManager) IsScopedActive(sessionKey string) bool {
	m.mu.RLock()
	entry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()
	if !exists {
		return false
	}
	_, asleep, _ := entry.snapshot()
	return !asleep && m.isSessionAlive(entry)
}

// ActiveAgentIDs returns the IDs of all agents with an active (non-asleep,
// process-alive) persistent session. This is used by the daemon heartbeat to
// report which agents are "online" on this computer — distinct from the
// task-level ActiveAgentIDs which only reports currently-executing tasks.
func (m *AgentSessionManager) ActiveAgentIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]bool)
	var ids []string
	for _, entry := range m.sessions {
		_, asleep, _ := entry.snapshot()
		if !asleep && m.isSessionAlive(entry) && !seen[entry.AgentID] {
			seen[entry.AgentID] = true
			ids = append(ids, entry.AgentID)
		}
	}
	return ids
}

// CloseSession terminates a session (active or asleep).
func (m *AgentSessionManager) CloseSession(agentID string) error {
	return m.closeScopedSession(AgentSessionKey(agentID), false)
}

// CloseThinkingSession terminates one node-scoped provider Session while
// preserving its persisted provider ID for audit.
func (m *AgentSessionManager) CloseThinkingSession(nodeID string) error {
	return m.closeScopedSession(ThinkingSessionKey(nodeID), false)
}

// ForceCloseThinkingSession terminates one node process immediately. It is
// used when the owning channel is archived and no further turn may complete.
func (m *AgentSessionManager) ForceCloseThinkingSession(nodeID string) error {
	return m.closeScopedSession(ThinkingSessionKey(nodeID), true)
}

// SleepIdleThinkingSessions gracefully releases idle node processes while
// retaining their provider session IDs for the next --resume.
func (m *AgentSessionManager) SleepIdleThinkingSessions(idleBefore time.Time) (int, error) {
	m.mu.RLock()
	keys := make([]string, 0)
	for sessionKey := range m.sessions {
		if strings.HasPrefix(sessionKey, "thinking:") {
			keys = append(keys, sessionKey)
		}
	}
	m.mu.RUnlock()

	slept := 0
	var firstErr error
	for _, sessionKey := range keys {
		release, ok := m.tryAcquireScopedTurn(sessionKey)
		if !ok {
			continue
		}

		m.mu.RLock()
		entry := m.sessions[sessionKey]
		m.mu.RUnlock()
		if entry == nil {
			release()
			continue
		}

		entry.mu.Lock()
		if entry.asleep || entry.Session == nil || !entry.LastActive.Before(idleBefore) {
			entry.mu.Unlock()
			release()
			continue
		}
		ps := entry.Session
		if ps.SessionID != "" {
			entry.sessionID = ps.SessionID
		}
		entry.Session = nil
		entry.asleep = true
		entry.mu.Unlock()

		m.logger.Info("session: sleeping idle Thinking process", "agent_id", entry.AgentID, "session_key", sessionKey)
		if err := m.backend.Close(ps); err != nil && firstErr == nil {
			firstErr = err
		}
		slept++
		release()
	}
	return slept, firstErr
}

func (m *AgentSessionManager) closeScopedSession(sessionKey string, force bool) error {
	m.mu.Lock()
	entry, exists := m.sessions[sessionKey]
	if exists {
		delete(m.sessions, sessionKey)
	}
	m.mu.Unlock()

	if !exists {
		return nil
	}

	m.logger.Info("session: closing", "agent_id", entry.AgentID, "session_key", sessionKey, "force", force)
	ps, asleep, _ := entry.snapshot()
	if !asleep && ps != nil {
		if force {
			return m.backend.ForceClose(ps)
		}
		return m.backend.Close(ps)
	}
	return nil
}

// ForceCloseSession immediately kills the agent's subprocess without graceful
// exit. Used by hard cleanup paths (agent deletion) where in-flight turns
// can be discarded.
func (m *AgentSessionManager) ForceCloseSession(agentID string) error {
	m.mu.Lock()
	entries := make([]*agentSessionEntry, 0)
	for sessionKey, entry := range m.sessions {
		if entry.AgentID == agentID {
			entries = append(entries, entry)
			delete(m.sessions, sessionKey)
		}
	}
	m.mu.Unlock()

	var firstErr error
	for _, entry := range entries {
		m.logger.Warn("session: force-closing", "agent_id", agentID, "session_key", entry.SessionKey)
		ps, asleep, _ := entry.snapshot()
		if !asleep && ps != nil {
			if err := m.backend.ForceClose(ps); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
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
		ps, asleep, _ := entry.snapshot()
		if !asleep && ps != nil {
			m.logger.Info("session: closing (shutdown)", "agent_id", entry.AgentID)
			_ = m.backend.Close(ps)
		}
	}
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (m *AgentSessionManager) isSessionAlive(entry *agentSessionEntry) bool {
	ps, asleep, _ := entry.snapshot()
	if asleep || ps == nil {
		return false
	}
	state, ok := ps.state.(SessionStater)
	if !ok {
		return false
	}
	return state.IsAlive()
}

func (m *AgentSessionManager) deliverToSession(ctx context.Context, sessionKey string, entry *agentSessionEntry, messages []Message) (*PersistentSession, error) {
	release := m.acquireTurn(sessionKey, entry.AgentID)
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			release()
		}
	}()

	m.logger.Info("session: delivering message", "agent_id", entry.AgentID, "session_key", sessionKey)
	previous, asleep, _ := entry.snapshot()
	if asleep || previous == nil {
		return nil, fmt.Errorf("session %s went to sleep before delivery", sessionKey)
	}
	ps, err := m.backend.Send(ctx, previous, messages)
	if err != nil {
		m.logger.Error("session: Send failed", "agent_id", entry.AgentID, "session_key", sessionKey, "error", err)
		m.mu.Lock()
		delete(m.sessions, sessionKey)
		m.mu.Unlock()
		return nil, err
	}
	if ps == nil {
		return nil, fmt.Errorf("session backend returned a nil session for %s", sessionKey)
	}

	entry.updateSession(ps)
	holdTurnUntilResult(ps, release)
	releaseOnReturn = false
	return ps, nil
}

func (m *AgentSessionManager) createSession(ctx context.Context, sessionKey, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, messages []Message, prevSessionID string, mentionedNames []string) (*PersistentSession, error) {
	release := m.acquireTurn(sessionKey, agentID)
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			release()
		}
	}()

	m.mu.RLock()
	entry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()
	if exists && m.isSessionAlive(entry) {
		previous, _, _ := entry.snapshot()
		ps, err := m.backend.Send(ctx, previous, messages)
		if err != nil {
			return nil, err
		}
		if ps == nil {
			return nil, fmt.Errorf("session backend returned a nil session for %s", sessionKey)
		}
		entry.updateSession(ps)
		holdTurnUntilResult(ps, release)
		releaseOnReturn = false
		return ps, nil
	}

	m.logger.Info("session: creating", "agent_id", agentID, "session_key", sessionKey, "resume", prevSessionID)

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
	m.startSlots <- struct{}{}         // acquire slot (blocks if 5 already starting)
	time.Sleep(500 * time.Millisecond) // dequeue interval
	m.logger.Info("session: dequeued start", "agent_id", agentID, "queue", len(m.startSlots))

	ps, err := m.backend.Start(ctx, executeReq, executeOpts)
	<-m.startSlots // release slot
	if err != nil {
		return nil, fmt.Errorf("start persistent session: %w", err)
	}

	entry = &agentSessionEntry{
		SessionKey:  sessionKey,
		AgentID:     agentID,
		Session:     ps,
		AgentConfig: agentCfg,
		ChannelCtx:  channelCtx,
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
		sessionID:   firstNonEmpty(ps.SessionID, prevSessionID),
	}

	m.mu.Lock()
	if old, ok := m.sessions[sessionKey]; ok {
		oldSession, oldAsleep, _ := old.snapshot()
		if !oldAsleep && oldSession != nil {
			_ = m.backend.Close(oldSession)
		}
	}
	m.sessions[sessionKey] = entry
	m.mu.Unlock()

	state, _ := ps.state.(SessionStater)
	go m.watchCrash(sessionKey, agentID, agentCfg, channelCtx, entry, state)
	holdTurnUntilResult(ps, release)
	releaseOnReturn = false

	// If the process died immediately (CLI broken/missing), record failure
	// to prevent retry storms on the next trigger.
	if !m.isSessionAlive(entry) {
		m.failedStartsMu.Lock()
		m.failedStarts[sessionKey] = time.Now()
		m.failedStartsMu.Unlock()
		m.logger.Warn("session: created but process died immediately, cooling down", "agent_id", agentID, "session_key", sessionKey)
	}

	m.logger.Info("session: created", "agent_id", agentID, "session_key", sessionKey)
	return ps, nil
}

func holdTurnUntilResult(ps *PersistentSession, release func()) {
	src := ps.Result
	dst := make(chan *Result, 1)
	ps.Result = dst
	go func() {
		defer release()
		defer close(dst)
		if src == nil {
			return
		}
		if result, ok := <-src; ok {
			dst <- result
		}
	}()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
func (m *AgentSessionManager) watchCrash(sessionKey, agentID string, agentCfg AgentConfig, channelCtx ChannelContext, entry *agentSessionEntry, state SessionStater) {
	if state == nil {
		return
	}

	<-state.Done()

	m.mu.RLock()
	currentEntry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()
	_, asleep, resumeID := entry.snapshot()
	if !exists || currentEntry != entry || asleep {
		return
	}

	if sid := state.SessionID(); sid != "" {
		resumeID = sid
	}

	m.logger.Warn("session: crashed, auto-restarting",
		"agent_id", agentID,
		"session_key", sessionKey,
		"session_id", resumeID,
	)

	m.mu.Lock()
	delete(m.sessions, sessionKey)
	m.mu.Unlock()

	restartMsg := Message{
		Role:    RoleUser,
		Content: "Your session has been restored after a restart. Context is preserved via --resume. Continue from where you left off.",
	}

	_, err := m.createSession(context.Background(), sessionKey, agentID, agentCfg, channelCtx, []Message{restartMsg}, resumeID, nil)
	if err != nil {
		m.logger.Error("session: crash recovery failed", "agent_id", agentID, "session_key", sessionKey, "error", err)
	}
}

// notifyInbox writes a lightweight notification to the agent's stdin,
// "1 pending inbox message(s)" pattern. The agent sees
// this notification at the start of its next stdin read and can call
// solo message check to pull the actual content.
func (m *AgentSessionManager) notifyInbox(sessionKey string, count int) {
	m.mu.RLock()
	entry, exists := m.sessions[sessionKey]
	m.mu.RUnlock()
	if !exists {
		return
	}
	ps, asleep, _ := entry.snapshot()
	if asleep || ps == nil {
		return
	}
	state, ok := ps.state.(SessionStater)
	if !ok {
		return
	}
	notification := fmt.Sprintf("\n[Solo] %d pending message(s). Use `solo message check` when ready.\n", count)
	_ = state.Notify(notification)
}

func (m *AgentSessionManager) acquireTurn(sessionKey, agentID string) func() {
	m.turnsMu.Lock()
	ch, exists := m.activeTurns[sessionKey]
	if !exists {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		m.activeTurns[sessionKey] = ch
	}
	agentCh, exists := m.agentTurns[agentID]
	if !exists {
		agentCh = make(chan struct{}, 1)
		agentCh <- struct{}{}
		m.agentTurns[agentID] = agentCh
	}
	m.turnsMu.Unlock()

	<-ch
	<-agentCh
	m.turnsMu.Lock()
	m.activeAgentScopes[agentID] = sessionKey
	m.turnsMu.Unlock()

	return func() {
		m.turnsMu.Lock()
		if m.activeAgentScopes[agentID] == sessionKey {
			delete(m.activeAgentScopes, agentID)
		}
		m.turnsMu.Unlock()
		agentCh <- struct{}{}
		ch <- struct{}{}
	}
}

func (m *AgentSessionManager) tryAcquireScopedTurn(sessionKey string) (func(), bool) {
	m.turnsMu.Lock()
	ch, exists := m.activeTurns[sessionKey]
	if !exists {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		m.activeTurns[sessionKey] = ch
	}
	m.turnsMu.Unlock()

	select {
	case <-ch:
		return func() { ch <- struct{}{} }, true
	default:
		return nil, false
	}
}

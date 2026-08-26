package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAcquireTurnCancellationDoesNotLeakSemaphore(t *testing.T) {
	mgr := NewAgentSessionManager(nil, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	firstRelease, err := mgr.acquireTurn(context.Background(), "agent:one", "one")
	if err != nil {
		t.Fatalf("acquire first turn: %v", err)
	}

	waitCtx, cancel := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	go func() {
		_, waitErr := mgr.acquireTurn(waitCtx, "agent:one", "one")
		waitResult <- waitErr
	}()
	cancel()
	select {
	case waitErr := <-waitResult:
		if waitErr == nil || !strings.Contains(waitErr.Error(), context.Canceled.Error()) {
			t.Fatalf("cancelled waiter error = %v, want context canceled", waitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled turn waiter did not unblock")
	}

	firstRelease()
	nextCtx, nextCancel := context.WithTimeout(context.Background(), time.Second)
	defer nextCancel()
	nextRelease, err := mgr.acquireTurn(nextCtx, "agent:one", "one")
	if err != nil {
		t.Fatalf("acquire after cancelled waiter: %v", err)
	}
	nextRelease()
}

func TestSessionManagerScopesSessionsWithoutCloningAgent(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude"}

	first, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "first"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start first node: %v", err)
	}
	if result := <-first.Result; result == nil || result.Status != "completed" {
		t.Fatalf("first result = %#v", result)
	}

	secondTurn, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "second"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("reuse first node: %v", err)
	}
	if result := <-secondTurn.Result; result == nil || result.Status != "completed" {
		t.Fatalf("second result = %#v", result)
	}
	if secondTurn.SessionID != first.SessionID {
		t.Fatalf("same node session changed: first=%q second=%q", first.SessionID, secondTurn.SessionID)
	}

	otherNode, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-b"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "other"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start second node: %v", err)
	}
	if result := <-otherNode.Result; result == nil || result.Status != "completed" {
		t.Fatalf("other result = %#v", result)
	}
	if otherNode.SessionID == first.SessionID {
		t.Fatalf("different nodes shared external session %q", first.SessionID)
	}

	backend.mu.Lock()
	starts, sends := append([]string(nil), backend.startAgentIDs...), backend.sendCount
	backend.mu.Unlock()
	if len(starts) != 2 || starts[0] != "agent-1" || starts[1] != "agent-1" {
		t.Fatalf("starts = %v, want two sessions owned by agent-1", starts)
	}
	if sends != 1 {
		t.Fatalf("send count = %d, want 1", sends)
	}
	if ids := mgr.ActiveAgentIDs(); len(ids) != 1 || ids[0] != "agent-1" {
		t.Fatalf("ActiveAgentIDs = %v, want deduplicated agent-1", ids)
	}
	if err := mgr.ForceCloseSession("agent-1"); err != nil {
		t.Fatalf("ForceCloseSession: %v", err)
	}
	backend.mu.Lock()
	closed := backend.forceCloseCount
	backend.mu.Unlock()
	if closed != 2 {
		t.Fatalf("force close count = %d, want all 2 scoped sessions", closed)
	}
}

func TestSessionManagerRestartsScopedSessionWhenModelChanges(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	sessionKey := ChannelSessionKey("agent-1", "channel-1")
	firstMessages := []Message{{Role: RoleUser, Content: "first"}}

	first, err := mgr.GetOrCreateScopedSession(
		context.Background(), sessionKey, "agent-1",
		AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude", Model: "sonnet"},
		ChannelContext{}, firstMessages, firstMessages, "", nil,
	)
	if err != nil {
		t.Fatalf("start first model: %v", err)
	}
	<-first.Result

	coldStart := []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "first reply"},
		{Role: RoleUser, Content: "second"},
	}
	second, err := mgr.GetOrCreateScopedSession(
		context.Background(), sessionKey, "agent-1",
		AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude", Model: "haiku"},
		ChannelContext{}, []Message{{Role: RoleUser, Content: "second"}}, coldStart, "", nil,
	)
	if err != nil {
		t.Fatalf("restart with second model: %v", err)
	}
	<-second.Result

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if first.SessionID == second.SessionID {
		t.Fatalf("model change reused session %q", first.SessionID)
	}
	if len(backend.startOptions) != 2 || backend.startOptions[0].Model != "sonnet" || backend.startOptions[1].Model != "haiku" {
		t.Fatalf("start models = %#v", backend.startOptions)
	}
	if backend.sendCount != 0 || backend.closeCount != 1 {
		t.Fatalf("send=%d close=%d, want 0/1", backend.sendCount, backend.closeCount)
	}
	if got := backend.startMessages[1]; len(got) != len(coldStart) || got[2].Content != "second" {
		t.Fatalf("restart messages = %#v, want cold-start context", got)
	}
}

func TestSessionManagerRestartsScopedSessionWhenProjectContextChanges(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	sessionKey := ChannelSessionKey("agent-1", "channel-1")
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude", ProjectSource: "repo-a", ProjectBaseline: "main"}

	first, err := mgr.GetOrCreateScopedSession(context.Background(), sessionKey, "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "first"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start first project context: %v", err)
	}
	<-first.Result

	cfg.ProjectSource = "repo-b"
	second, err := mgr.GetOrCreateScopedSession(context.Background(), sessionKey, "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "second"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("restart with new project context: %v", err)
	}
	<-second.Result

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.closeCount != 1 || len(backend.startOptions) != 2 || !strings.Contains(backend.startOptions[1].SystemPrompt, "Channel project source: repo-b") {
		t.Fatalf("close/start/prompt = %d/%d/%q", backend.closeCount, len(backend.startOptions), backend.startOptions[1].SystemPrompt)
	}
}

func TestSessionManagerClosesReturnedThinkingSession(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	ps, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("returned-node"), "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "handoff"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start node: %v", err)
	}
	<-ps.Result
	if err := mgr.CloseThinkingSession("returned-node"); err != nil {
		t.Fatalf("close returned node: %v", err)
	}
	if mgr.IsScopedActive(ThinkingSessionKey("returned-node")) {
		t.Fatal("returned node session is still active")
	}
	backend.mu.Lock()
	closed := backend.closeCount
	backend.mu.Unlock()
	if closed != 1 {
		t.Fatalf("close count = %d, want 1", closed)
	}
}

func TestSessionManagerCloseAllForceCloses(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	ps, err := mgr.GetOrCreateSession(context.Background(), "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "work"}}, nil)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	<-ps.Result

	mgr.CloseAll()

	backend.mu.Lock()
	forced, closed := backend.forceCloseCount, backend.closeCount
	backend.mu.Unlock()
	if forced != 1 || closed != 0 {
		t.Fatalf("shutdown closes = force %d graceful %d, want 1/0", forced, closed)
	}
	if mgr.IsActive("agent-1") {
		t.Fatal("session remains active after CloseAll")
	}
}

func TestSessionManagerSleepsOnlyIdleThinkingSessionsAndResumes(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude"}

	thinking, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("idle-node"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "think"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start Thinking session: %v", err)
	}
	if result := <-thinking.Result; result == nil || result.Status != "completed" {
		t.Fatalf("Thinking result = %#v", result)
	}

	agentSession, err := mgr.GetOrCreateSession(context.Background(), "agent-2", AgentConfig{AgentID: "agent-2", Name: "Agent 2", Provider: "claude"}, ChannelContext{}, []Message{{Role: RoleUser, Content: "normal"}}, nil)
	if err != nil {
		t.Fatalf("start ordinary Agent session: %v", err)
	}
	if result := <-agentSession.Result; result == nil || result.Status != "completed" {
		t.Fatalf("Agent result = %#v", result)
	}

	waitForScopedTurnRelease(t, mgr, ThinkingSessionKey("idle-node"))
	mgr.mu.RLock()
	thinkingEntry := mgr.sessions[ThinkingSessionKey("idle-node")]
	agentEntry := mgr.sessions[AgentSessionKey("agent-2")]
	mgr.mu.RUnlock()
	old := time.Now().Add(-time.Hour)
	thinkingEntry.mu.Lock()
	thinkingEntry.LastActive = old
	thinkingEntry.mu.Unlock()
	agentEntry.mu.Lock()
	agentEntry.LastActive = old
	agentEntry.mu.Unlock()

	slept, err := mgr.SleepIdleThinkingSessions(time.Now().Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("sleep idle Thinking sessions: %v", err)
	}
	if slept != 1 {
		t.Fatalf("slept = %d, want 1", slept)
	}
	if mgr.IsScopedActive(ThinkingSessionKey("idle-node")) {
		t.Fatal("idle Thinking session is still active")
	}
	if !mgr.IsActive("agent-2") {
		t.Fatal("ordinary Agent session was incorrectly slept")
	}

	resumed, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("idle-node"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "continue"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("resume Thinking session: %v", err)
	}
	if result := <-resumed.Result; result == nil || result.Status != "completed" {
		t.Fatalf("resumed result = %#v", result)
	}
	if resumed.SessionID != thinking.SessionID {
		t.Fatalf("resumed SessionID = %q, want %q", resumed.SessionID, thinking.SessionID)
	}

	backend.mu.Lock()
	starts, closes := len(backend.startAgentIDs), backend.closeCount
	backend.mu.Unlock()
	if starts != 3 {
		t.Fatalf("start count = %d, want initial Thinking + ordinary Agent + resumed Thinking", starts)
	}
	if closes != 1 {
		t.Fatalf("graceful close count = %d, want 1 idle Thinking process", closes)
	}
}

func TestSessionManagerSleepsIdleAgentSessionAndResumes(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude"}
	sessionKey := ChannelSessionKey("agent-1", "channel-1")

	first, err := mgr.GetOrCreateScopedSession(context.Background(), sessionKey, "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "first"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start Agent session: %v", err)
	}
	if result := <-first.Result; result == nil || result.Status != "completed" {
		t.Fatalf("Agent result = %#v", result)
	}
	waitForScopedTurnRelease(t, mgr, sessionKey)

	mgr.mu.RLock()
	entry := mgr.sessions[sessionKey]
	mgr.mu.RUnlock()
	entry.mu.Lock()
	entry.LastActive = time.Now().Add(-time.Hour)
	entry.mu.Unlock()

	slept, err := mgr.SleepIdleAgentSessions(time.Now().Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("sleep idle Agent sessions: %v", err)
	}
	if slept != 1 || mgr.IsScopedActive(sessionKey) {
		t.Fatalf("slept = %d active = %v, want 1/false", slept, mgr.IsScopedActive(sessionKey))
	}
	if ids := mgr.ActiveAgentIDs(); len(ids) != 0 {
		t.Fatalf("ActiveAgentIDs = %v, want no live process", ids)
	}
	if ids := mgr.CachedAgentIDs(); len(ids) != 1 || ids[0] != "agent-1" {
		t.Fatalf("CachedAgentIDs = %v, want resumable agent-1", ids)
	}

	resumed, err := mgr.GetOrCreateScopedSession(context.Background(), sessionKey, "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "continue"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("resume Agent session: %v", err)
	}
	if result := <-resumed.Result; result == nil || result.Status != "completed" {
		t.Fatalf("resumed result = %#v", result)
	}
	if resumed.SessionID != first.SessionID {
		t.Fatalf("resumed SessionID = %q, want %q", resumed.SessionID, first.SessionID)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.closeCount != 1 || len(backend.startOptions) != 2 {
		t.Fatalf("close/start counts = %d/%d, want 1/2", backend.closeCount, len(backend.startOptions))
	}
	if backend.startOptions[1].ResumeSessionID != first.SessionID {
		t.Fatalf("resume ID = %q, want %q", backend.startOptions[1].ResumeSessionID, first.SessionID)
	}
}

func TestSessionManagerUsesProtocolResumeWithoutInjectingCLIFlag(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{
		AgentID:    "agent-1",
		Name:       "Agent",
		Provider:   "codex",
		CustomArgs: []string{"--configured-flag"},
	}

	resumed, err := mgr.GetOrCreateScopedSession(
		context.Background(),
		ThinkingSessionKey("resume-node"),
		"agent-1",
		cfg,
		ChannelContext{},
		[]Message{{Role: RoleUser, Content: "continue"}},
		nil,
		"provider-session-1",
		nil,
	)
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}
	<-resumed.Result

	backend.mu.Lock()
	opts := backend.startOptions[0]
	backend.mu.Unlock()
	if opts.ResumeSessionID != "provider-session-1" {
		t.Fatalf("ResumeSessionID = %q, want provider-session-1", opts.ResumeSessionID)
	}
	if len(opts.CustomArgs) != 1 || opts.CustomArgs[0] != "--configured-flag" {
		t.Fatalf("CustomArgs = %v, want only configured flags", opts.CustomArgs)
	}
}

func TestSessionCrashWaitsForNextTrackedRequestToResume(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	done := make(chan struct{})
	close(done)
	state := earlyReturnState{doneCh: done, sessionID: "provider-session-1"}
	ps := &PersistentSession{SessionID: "provider-session-1", state: state}
	entry := &agentSessionEntry{
		SessionKey: "agent:agent-1",
		AgentID:    "agent-1",
		Session:    ps,
		sessionID:  "provider-session-1",
	}
	mgr.sessions[entry.SessionKey] = entry

	mgr.watchCrash(entry.SessionKey, entry.AgentID, AgentConfig{}, ChannelContext{}, entry, state)

	current, asleep, resumeID := entry.snapshot()
	if current != nil || !asleep || resumeID != "provider-session-1" {
		t.Fatalf("crashed entry = session:%v asleep:%v resume:%q", current, asleep, resumeID)
	}
	backend.mu.Lock()
	starts := len(backend.startAgentIDs)
	backend.mu.Unlock()
	if starts != 0 {
		t.Fatalf("crash recovery started %d untracked turns, want 0", starts)
	}
}

func TestSessionManagerDoesNotSleepThinkingSessionWithActiveTurn(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	_, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("busy-node"), "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "working"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start Thinking session: %v", err)
	}
	mgr.mu.RLock()
	entry := mgr.sessions[ThinkingSessionKey("busy-node")]
	mgr.mu.RUnlock()
	entry.mu.Lock()
	entry.LastActive = time.Now().Add(-time.Hour)
	entry.mu.Unlock()

	slept, err := mgr.SleepIdleThinkingSessions(time.Now().Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("sleep idle Thinking sessions: %v", err)
	}
	if slept != 0 {
		t.Fatalf("slept = %d, want 0 while turn is active", slept)
	}
	if !mgr.IsScopedActive(ThinkingSessionKey("busy-node")) {
		t.Fatal("active Thinking turn was closed")
	}
	backend.finishStart()
}

func TestSessionManagerDoesNotSleepAgentSessionWithActiveTurn(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	sessionKey := ChannelSessionKey("agent-1", "channel-1")

	_, err := mgr.GetOrCreateScopedSession(context.Background(), sessionKey, "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "working"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("start Agent session: %v", err)
	}
	mgr.mu.RLock()
	entry := mgr.sessions[sessionKey]
	mgr.mu.RUnlock()
	old := time.Now().Add(-time.Hour)
	entry.mu.Lock()
	entry.LastActive = old
	entry.mu.Unlock()

	slept, err := mgr.SleepIdleAgentSessions(time.Now().Add(-30 * time.Minute))
	if err != nil {
		t.Fatalf("sleep idle Agent sessions: %v", err)
	}
	if slept != 0 || !mgr.IsScopedActive(sessionKey) {
		t.Fatalf("slept = %d active = %v, want 0/true", slept, mgr.IsScopedActive(sessionKey))
	}

	backend.finishStart()
	waitForScopedTurnRelease(t, mgr, sessionKey)
	entry.mu.RLock()
	lastActive := entry.LastActive
	entry.mu.RUnlock()
	if !lastActive.After(old) {
		t.Fatalf("LastActive = %v, want turn completion after %v", lastActive, old)
	}
	slept, err = mgr.SleepIdleAgentSessions(time.Now().Add(-time.Minute))
	if err != nil || slept != 0 {
		t.Fatalf("freshly completed Agent sleep = %d, %v, want 0/nil", slept, err)
	}
}

func waitForScopedTurnRelease(t *testing.T, mgr *AgentSessionManager, sessionKey string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if release, ok := mgr.tryAcquireScopedTurn(sessionKey); ok {
			release()
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("turn for %s did not release", sessionKey)
}

func TestChannelSessionKeyIsolatesChannels(t *testing.T) {
	first := ChannelSessionKey("agent-1", "channel-a")
	second := ChannelSessionKey("agent-1", "channel-b")
	if first == second {
		t.Fatalf("channel-scoped session keys collided: %q", first)
	}
	if first != ChannelSessionKey("agent-1", "channel-a") {
		t.Fatal("channel-scoped session key is not stable")
	}
}

func TestSessionManagerExposesRuntimeOwnedThinkingScopeWhileTurnIsActive(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	_, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", AgentConfig{
		AgentID:  "agent-1",
		Name:     "Agent",
		Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "hello"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("GetOrCreateScopedSession: %v", err)
	}
	if nodeID, ok := mgr.ActiveThinkingNodeID("agent-1"); !ok || nodeID != "node-a" {
		t.Fatalf("ActiveThinkingNodeID = %q, %v, want node-a, true", nodeID, ok)
	}

	backend.finishStart()
	deadline := time.After(500 * time.Millisecond)
	for {
		if _, ok := mgr.ActiveThinkingNodeID("agent-1"); !ok {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Thinking scope stayed active after the turn result closed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type earlyReturnBackend struct {
	startResultCh chan *Result
	sendResultCh  chan *Result
	doneCh        chan struct{}
}

func newEarlyReturnBackend() *earlyReturnBackend {
	return &earlyReturnBackend{
		startResultCh: make(chan *Result, 1),
		sendResultCh:  make(chan *Result, 1),
		doneCh:        make(chan struct{}),
	}
}

func (b *earlyReturnBackend) Name() string { return "early-return" }

func (b *earlyReturnBackend) Execute(context.Context, *ExecuteRequest, *ExecuteOptions) (*Session, error) {
	return nil, nil
}

func (b *earlyReturnBackend) Start(context.Context, *ExecuteRequest, *ExecuteOptions) (*PersistentSession, error) {
	msgCh := make(chan OutputChunk)
	return &PersistentSession{
		Messages:  msgCh,
		Result:    b.startResultCh,
		Stop:      func() error { return nil },
		SessionID: "session-1",
		state:     earlyReturnState{doneCh: b.doneCh, sessionID: "session-1"},
	}, nil
}

func (b *earlyReturnBackend) Send(_ context.Context, previous *PersistentSession, _ []Message) (*PersistentSession, error) {
	msgCh := make(chan OutputChunk)
	return &PersistentSession{
		Messages:  msgCh,
		Result:    b.sendResultCh,
		Stop:      func() error { return nil },
		SessionID: previous.SessionID,
		state:     previous.state,
	}, nil
}

func (b *earlyReturnBackend) Close(*PersistentSession) error { return nil }

func (b *earlyReturnBackend) ForceClose(*PersistentSession) error { return nil }

func (b *earlyReturnBackend) finishStart() {
	b.startResultCh <- &Result{Status: "completed"}
	close(b.startResultCh)
}

func (b *earlyReturnBackend) finishSend() {
	b.sendResultCh <- &Result{Status: "completed"}
	close(b.sendResultCh)
}

type earlyReturnState struct {
	doneCh    chan struct{}
	sessionID string
}

func (s earlyReturnState) IsAlive() bool { return true }

func (s earlyReturnState) SessionID() string { return s.sessionID }

func (s earlyReturnState) Done() <-chan struct{} { return s.doneCh }

func (s earlyReturnState) Notify(string) error { return nil }

type scopedRecordingBackend struct {
	mu              sync.Mutex
	startAgentIDs   []string
	startMessages   [][]Message
	sendCount       int
	forceCloseCount int
	closeCount      int
	startOptions    []ExecuteOptions
}

func (b *scopedRecordingBackend) Name() string { return "scoped-recording" }

func (b *scopedRecordingBackend) Execute(context.Context, *ExecuteRequest, *ExecuteOptions) (*Session, error) {
	return nil, nil
}

func (b *scopedRecordingBackend) Start(_ context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	b.mu.Lock()
	b.startAgentIDs = append(b.startAgentIDs, req.AgentID)
	b.startMessages = append(b.startMessages, append([]Message(nil), req.Messages...))
	b.startOptions = append(b.startOptions, *opts)
	id := opts.ResumeSessionID
	if id == "" {
		id = "session-" + fmt.Sprint(len(b.startAgentIDs))
	}
	b.mu.Unlock()
	return completedPersistentSession(id), nil
}

func (b *scopedRecordingBackend) Send(_ context.Context, previous *PersistentSession, _ []Message) (*PersistentSession, error) {
	b.mu.Lock()
	b.sendCount++
	b.mu.Unlock()
	return completedPersistentSession(previous.SessionID), nil
}

func (b *scopedRecordingBackend) Close(*PersistentSession) error {
	b.mu.Lock()
	b.closeCount++
	b.mu.Unlock()
	return nil
}

func (b *scopedRecordingBackend) ForceClose(*PersistentSession) error {
	b.mu.Lock()
	b.forceCloseCount++
	b.mu.Unlock()
	return nil
}

func completedPersistentSession(id string) *PersistentSession {
	messages := make(chan OutputChunk)
	close(messages)
	result := make(chan *Result, 1)
	result <- &Result{Status: "completed"}
	close(result)
	return &PersistentSession{
		Messages:  messages,
		Result:    result,
		Stop:      func() error { return nil },
		SessionID: id,
		state:     earlyReturnState{doneCh: make(chan struct{}), sessionID: id},
	}
}

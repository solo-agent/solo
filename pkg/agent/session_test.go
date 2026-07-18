package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestSessionManagerScopesSessionsWithoutCloningAgent(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude"}

	first, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "first"}}, "", nil)
	if err != nil {
		t.Fatalf("start first node: %v", err)
	}
	if result := <-first.Result; result == nil || result.Status != "completed" {
		t.Fatalf("first result = %#v", result)
	}

	secondTurn, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "second"}}, "", nil)
	if err != nil {
		t.Fatalf("reuse first node: %v", err)
	}
	if result := <-secondTurn.Result; result == nil || result.Status != "completed" {
		t.Fatalf("second result = %#v", result)
	}
	if secondTurn.SessionID != first.SessionID {
		t.Fatalf("same node session changed: first=%q second=%q", first.SessionID, secondTurn.SessionID)
	}

	otherNode, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-b"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "other"}}, "", nil)
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

func TestSessionManagerClosesReturnedThinkingSession(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	ps, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("returned-node"), "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "handoff"}}, "", nil)
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

func TestSessionManagerSleepsOnlyIdleThinkingSessionsAndResumes(t *testing.T) {
	backend := &scopedRecordingBackend{}
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())
	cfg := AgentConfig{AgentID: "agent-1", Name: "Agent", Provider: "claude"}

	thinking, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("idle-node"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "think"}}, "", nil)
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

	resumed, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("idle-node"), "agent-1", cfg, ChannelContext{}, []Message{{Role: RoleUser, Content: "continue"}}, "", nil)
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

func TestSessionManagerDoesNotSleepThinkingSessionWithActiveTurn(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	_, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("busy-node"), "agent-1", AgentConfig{
		AgentID: "agent-1", Name: "Agent", Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "working"}}, "", nil)
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

func TestSessionManagerHoldsTurnUntilEarlyStartResultCloses(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	ps, err := mgr.GetOrCreateSession(context.Background(), "agent-1", AgentConfig{
		AgentID:      "agent-1",
		Name:         "Agent",
		SystemPrompt: "You are Agent.",
		Provider:     "opencode",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	if ps.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", ps.SessionID)
	}

	if !mgr.QueueIfBusy("agent-1", Message{Role: RoleUser, Content: "second"}) {
		t.Fatal("QueueIfBusy = false while initial result is still open, want true")
	}

	backend.finishStart()

	deadline := time.After(500 * time.Millisecond)
	for {
		if !mgr.QueueIfBusy("agent-1", Message{Role: RoleUser, Content: "third"}) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("QueueIfBusy stayed true after initial result closed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestSessionManagerExposesRuntimeOwnedThinkingScopeWhileTurnIsActive(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	_, err := mgr.GetOrCreateScopedSession(context.Background(), ThinkingSessionKey("node-a"), "agent-1", AgentConfig{
		AgentID:  "agent-1",
		Name:     "Agent",
		Provider: "claude",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "hello"}}, "", nil)
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

func TestSessionManagerHoldsTurnUntilDeliveredResultCloses(t *testing.T) {
	backend := newEarlyReturnBackend()
	mgr := NewAgentSessionManager(backend, NewWorkspaceManager(t.TempDir()), nil, slog.Default())

	first, err := mgr.GetOrCreateSession(context.Background(), "agent-1", AgentConfig{
		AgentID:      "agent-1",
		Name:         "Agent",
		SystemPrompt: "You are Agent.",
		Provider:     "codex",
	}, ChannelContext{}, []Message{{Role: RoleUser, Content: "first"}}, nil)
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	backend.finishStart()
	if result := <-first.Result; result == nil || result.Status != "completed" {
		t.Fatalf("start result = %#v, want completed", result)
	}

	second, err := mgr.DeliverMessage(context.Background(), "agent-1", []Message{{Role: RoleUser, Content: "second"}})
	if err != nil {
		t.Fatalf("DeliverMessage: %v", err)
	}
	if !mgr.QueueIfBusy("agent-1", Message{Role: RoleUser, Content: "third"}) {
		t.Fatal("QueueIfBusy = false while delivered result is still open, want true")
	}

	backend.finishSend()
	if result := <-second.Result; result == nil || result.Status != "completed" {
		t.Fatalf("send result = %#v, want completed", result)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if !mgr.QueueIfBusy("agent-1", Message{Role: RoleUser, Content: "fourth"}) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("QueueIfBusy stayed true after delivered result closed")
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
	sendCount       int
	forceCloseCount int
	closeCount      int
}

func (b *scopedRecordingBackend) Name() string { return "scoped-recording" }

func (b *scopedRecordingBackend) Execute(context.Context, *ExecuteRequest, *ExecuteOptions) (*Session, error) {
	return nil, nil
}

func (b *scopedRecordingBackend) Start(_ context.Context, req *ExecuteRequest, opts *ExecuteOptions) (*PersistentSession, error) {
	b.mu.Lock()
	b.startAgentIDs = append(b.startAgentIDs, req.AgentID)
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

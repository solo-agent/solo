package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

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

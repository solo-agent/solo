package service

import (
	"context"
	"testing"
)

func TestSubtaskNotifier_NotifyClaimerToCreator(t *testing.T) {
	pool := setupTestPool(t)
	hub := &fakeHub{}
	aID := createTestAgent(t, pool, "Alice")
	bID := createTestAgent(t, pool, "Bob")
	channelID, _ := createTestChannel(t, pool)
	addChannelMember(t, pool, channelID, aID, "agent")
	addChannelMember(t, pool, channelID, bID, "agent")
	createTestRelationship(t, pool, bID, aID, "assigns_to", nil, 1.0)
	parentID := createTestTask(t, pool, channelID, aID, "T-parent", nil)
	subID := createTestTask(t, pool, channelID, aID, "T-sub", &parentID)

	svc := NewSubtaskNotifier(pool, hub)
	err := svc.NotifyClaim(context.Background(), subID, bID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hub.sentToChannel) == 0 {
		t.Error("expected channel notification, got none")
	}
}

func TestSubtaskNotifier_NoEdgeSkips(t *testing.T) {
	pool := setupTestPool(t)
	hub := &fakeHub{}
	aID := createTestAgent(t, pool, "Alice")
	bID := createTestAgent(t, pool, "Bob")
	channelID, _ := createTestChannel(t, pool)
	addChannelMember(t, pool, channelID, aID, "agent")
	addChannelMember(t, pool, channelID, bID, "agent")
	// NO assigns_to edge.
	parentID := createTestTask(t, pool, channelID, aID, "T-parent", nil)
	subID := createTestTask(t, pool, channelID, aID, "T-sub", &parentID)

	svc := NewSubtaskNotifier(pool, hub)
	err := svc.NotifyClaim(context.Background(), subID, bID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hub.sentToChannel) != 0 {
		t.Error("expected no notification without edge, got one")
	}
}

// fakeHub satisfies realtime.Broadcaster for tests.
type fakeHub struct {
	sentToChannel     []string
	channelPayloads   [][]byte
}

func (f *fakeHub) BroadcastToScope(scopeType, scopeID string, msg []byte) {
	if scopeType == "channel" {
		f.sentToChannel = append(f.sentToChannel, scopeID)
		f.channelPayloads = append(f.channelPayloads, msg)
	}
}

func (f *fakeHub) BroadcastToChannel(channelID string, msg []byte) {
	f.sentToChannel = append(f.sentToChannel, channelID)
	f.channelPayloads = append(f.channelPayloads, msg)
}

func (f *fakeHub) SendToUser(userID string, msg []byte) {}

func (f *fakeHub) BroadcastToThread(threadID string, msg []byte) {}

func (f *fakeHub) Broadcast(msg []byte) {}
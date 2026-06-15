package service

import (
	"context"
	"strings"
	"testing"
)

type recordingHub struct {
	broadcasts [][]byte
}

func (r *recordingHub) BroadcastToScope(scopeType, scopeID string, msg []byte) {
	r.broadcasts = append(r.broadcasts, msg)
}
func (r *recordingHub) BroadcastToChannel(channelID string, msg []byte) {
	r.broadcasts = append(r.broadcasts, msg)
}
func (r *recordingHub) SendToUser(userID string, msg []byte) {}
func (r *recordingHub) BroadcastToThread(threadID string, msg []byte) {}
func (r *recordingHub) Broadcast(msg []byte) {
	r.broadcasts = append(r.broadcasts, msg)
}

func TestRelationshipEvents_PublishCreated(t *testing.T) {
	hub := &recordingHub{}
	pub := NewRelationshipEventPublisher(hub)
	pub.PublishCreated(context.Background(), "rel-1", "agent-A", "agent-B", "assigns_to")

	if len(hub.broadcasts) == 0 {
		t.Fatal("expected broadcast, got none")
	}
	body := string(hub.broadcasts[0])
	if !strings.Contains(body, "relationship_created") {
		t.Errorf("expected relationship_created in payload, got: %s", body)
	}
	if !strings.Contains(body, "rel-1") {
		t.Errorf("expected rel id in payload, got: %s", body)
	}
}

func TestRelationshipEvents_PublishUpdated(t *testing.T) {
	hub := &recordingHub{}
	pub := NewRelationshipEventPublisher(hub)
	pub.PublishUpdated(context.Background(), "rel-2", "agent-A", "agent-B", "collaborates_with")
	if len(hub.broadcasts) == 0 {
		t.Fatal("expected broadcast, got none")
	}
	body := string(hub.broadcasts[0])
	if !strings.Contains(body, "relationship_updated") {
		t.Errorf("expected relationship_updated in payload, got: %s", body)
	}
}

func TestRelationshipEvents_PublishDeleted(t *testing.T) {
	hub := &recordingHub{}
	pub := NewRelationshipEventPublisher(hub)
	pub.PublishDeleted(context.Background(), "rel-3", "agent-A", "agent-B", "assigns_to")
	if len(hub.broadcasts) == 0 {
		t.Fatal("expected broadcast, got none")
	}
	body := string(hub.broadcasts[0])
	if !strings.Contains(body, "relationship_deleted") {
		t.Errorf("expected relationship_deleted in payload, got: %s", body)
	}
}

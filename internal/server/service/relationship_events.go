package service

import (
	"context"
	"encoding/json"

	"github.com/solo-ai/solo/internal/realtime"
)

// RelationshipEventPublisher emits relationship change events on the hub.
type RelationshipEventPublisher struct {
	hub realtime.Broadcaster
}

func NewRelationshipEventPublisher(hub realtime.Broadcaster) *RelationshipEventPublisher {
	return &RelationshipEventPublisher{hub: hub}
}

func (p *RelationshipEventPublisher) PublishCreated(ctx context.Context, relID, fromAgent, toAgent, relType string) {
	p.publish(ctx, "relationship_created", relID, fromAgent, toAgent, relType)
}

func (p *RelationshipEventPublisher) PublishUpdated(ctx context.Context, relID, fromAgent, toAgent, relType string) {
	p.publish(ctx, "relationship_updated", relID, fromAgent, toAgent, relType)
}

func (p *RelationshipEventPublisher) PublishDeleted(ctx context.Context, relID, fromAgent, toAgent, relType string) {
	p.publish(ctx, "relationship_deleted", relID, fromAgent, toAgent, relType)
}

func (p *RelationshipEventPublisher) publish(ctx context.Context, eventType, relID, fromAgent, toAgent, relType string) {
	if p.hub == nil {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"type": eventType,
		"payload": map[string]string{
			"id":            relID,
			"from_agent_id": fromAgent,
			"to_agent_id":   toAgent,
			"rel_type":      relType,
		},
	})
	p.hub.Broadcast(payload)
}

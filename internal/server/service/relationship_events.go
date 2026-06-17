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

// PublishAgentDeleted broadcasts an agent_deleted event after a soft-delete
// cascades its relationships. Frontends use this to prune local graph / list
// state that referenced the agent; the deleted_relationship_ids list lets
// them invalidate any cached relationship objects without a follow-up fetch.
func (p *RelationshipEventPublisher) PublishAgentDeleted(ctx context.Context, agentID string, deletedRelationshipIDs []string) {
	if p.hub == nil {
		return
	}
	if deletedRelationshipIDs == nil {
		deletedRelationshipIDs = []string{}
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"type": "agent_deleted",
		"payload": map[string]interface{}{
			"agent_id":                agentID,
			"deleted_relationship_ids": deletedRelationshipIDs,
		},
	})
	p.hub.Broadcast(payload)
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

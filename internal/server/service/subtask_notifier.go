package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// SubtaskNotifier sends notifications when a sub-task is claimed or completed.
type SubtaskNotifier struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

func NewSubtaskNotifier(pool *pgxpool.Pool, hub realtime.Broadcaster) *SubtaskNotifier {
	return &SubtaskNotifier{pool: pool, hub: hub}
}

// NotifyClaim notifies the sub-task creator that someone claimed it.
func (s *SubtaskNotifier) NotifyClaim(ctx context.Context, taskID, claimerID string) error {
	return s.notify(ctx, taskID, claimerID, "claim", "claimed")
}

// NotifyComplete notifies the sub-task creator that someone completed it.
func (s *SubtaskNotifier) NotifyComplete(ctx context.Context, taskID, claimerID string) error {
	return s.notify(ctx, taskID, claimerID, "complete", "completed")
}

func (s *SubtaskNotifier) notify(ctx context.Context, taskID, claimerID, eventType, verb string) error {
	var creatorID, channelID, title string
	err := s.pool.QueryRow(ctx, `
		SELECT t.creator_id, t.channel_id, t.title
		  FROM tasks t
		 WHERE t.id = $1
	`, taskID).Scan(&creatorID, &channelID, &title)
	if err != nil {
		return fmt.Errorf("lookup task: %w", err)
	}
	if creatorID == claimerID {
		// Self-claim — no notification.
		return nil
	}

	// Reverse lookup: is there an assigns_to edge from claimer → creator?
	var hasEdge bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agent_relationships
			WHERE from_agent_id = $1
			  AND to_agent_id = $2
			  AND rel_type = 'assigns_to'
		)
	`, claimerID, creatorID).Scan(&hasEdge)
	if err != nil {
		return fmt.Errorf("reverse assigns_to check: %w", err)
	}
	if !hasEdge {
		slog.Info("subtask notify skipped — no assigns_to edge",
			"task_id", taskID, "claimer_id", claimerID, "creator_id", creatorID)
		return nil
	}

	if s.hub == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"type": "subtask_notification",
		"payload": map[string]string{
			"event":      eventType,
			"task_id":    taskID,
			"task_title": title,
			"claimer_id": claimerID,
			"creator_id": creatorID,
			"message":    fmt.Sprintf("@%s's sub-task %s %s by @%s", creatorID, title, verb, claimerID),
		},
	})
	s.hub.BroadcastToChannel(channelID, payload)
	return nil
}
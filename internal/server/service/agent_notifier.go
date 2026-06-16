package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// AgentNotifier sends DM system messages to agents for task lifecycle events.
// Messages are persisted to the messages table (sender_type='system', thread_id=NULL)
// so they enter the agent's JSONL context via getRecentMessages.
// After persisting, it triggers the agent via TriggerAgentResponse so the notification
// is processed immediately rather than waiting for the next polling cycle.
type AgentNotifier struct {
	pool     *pgxpool.Pool
	hub      realtime.Broadcaster
	agentSvc *AgentService
}

func NewAgentNotifier(pool *pgxpool.Pool, hub realtime.Broadcaster, agentSvc *AgentService) *AgentNotifier {
	return &AgentNotifier{pool: pool, hub: hub, agentSvc: agentSvc}
}

// ensureAgentDM finds or creates a DM channel between the agent and its owner.
func (n *AgentNotifier) ensureAgentDM(ctx context.Context, agentID string) (string, error) {
	var ownerID string
	err := n.pool.QueryRow(ctx, `SELECT owner_id FROM agents WHERE id = $1`, agentID).Scan(&ownerID)
	if err != nil {
		return "", fmt.Errorf("lookup agent owner: %w", err)
	}

	// Check for existing DM between owner (user) and agent.
	var dmID string
	err = n.pool.QueryRow(ctx, `
		SELECT dm1.channel_id
		  FROM dm_members dm1
		  JOIN dm_members dm2 ON dm1.channel_id = dm2.channel_id
		  JOIN channels c ON c.id = dm1.channel_id AND c.type = 'dm' AND c.is_archived = false
		 WHERE dm1.member_type = 'user' AND dm1.member_id = $1
		   AND dm2.member_type = 'agent' AND dm2.member_id = $2
		 LIMIT 1
	`, ownerID, agentID).Scan(&dmID)
	if err == nil && dmID != "" {
		return dmID, nil
	}

	// Create new DM channel.
	tx, err := n.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var agentName string
	err = tx.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, agentID).Scan(&agentName)
	if err != nil {
		return "", fmt.Errorf("lookup agent name: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO channels (name, description, type, created_by)
		VALUES ($1, 'Direct Message', 'dm', $2)
		RETURNING id
	`, agentName, ownerID).Scan(&dmID)
	if err != nil {
		return "", fmt.Errorf("create DM channel: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner'), ($1, 'agent', $3, 'member')
	`, dmID, ownerID, agentID)
	if err != nil {
		return "", fmt.Errorf("add channel_members: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO dm_members (channel_id, member_type, member_id)
		VALUES ($1, 'user', $2), ($1, 'agent', $3)
	`, dmID, ownerID, agentID)
	if err != nil {
		return "", fmt.Errorf("add dm_members: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit DM creation: %w", err)
	}

	slog.Info("DM created for agent notifications", "dm_id", dmID, "agent_id", agentID, "owner_id", ownerID)
	return dmID, nil
}

// taskNotification holds data for building a notification message.
type taskNotification struct {
	taskID      string
	taskNumber  int
	taskTitle   string
	channelName string
	creatorID   string
	actorName   string
}

func (n *AgentNotifier) lookupTaskNotification(ctx context.Context, taskID, actorID string) (*taskNotification, error) {
	var tn taskNotification
	tn.taskID = taskID

	err := n.pool.QueryRow(ctx, `
		SELECT t.title, t.task_number, COALESCE(ch.name, 'DM') as channel_name, t.creator_id
		  FROM tasks t
		  LEFT JOIN channels ch ON ch.id = t.channel_id
		 WHERE t.id = $1
	`, taskID).Scan(&tn.taskTitle, &tn.taskNumber, &tn.channelName, &tn.creatorID)
	if err != nil {
		return nil, fmt.Errorf("lookup task: %w", err)
	}

	err = n.pool.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, actorID).Scan(&tn.actorName)
	if err != nil {
		tn.actorName = actorID
	}

	return &tn, nil
}

// Notify sends a generic DM system message to an agent's owner-agent DM
// and triggers the agent immediately.
func (n *AgentNotifier) Notify(ctx context.Context, agentID string, content string) error {
	dmID, err := n.ensureAgentDM(ctx, agentID)
	if err != nil {
		return fmt.Errorf("ensure DM: %w", err)
	}
	return n.sendSystemMessage(ctx, dmID, content)
}

// sendSystemMessage persists a system message to the DM and broadcasts via WS.
func (n *AgentNotifier) sendSystemMessage(ctx context.Context, dmID, content string) error {
	msgID := uuid.New().String()
	now := time.Now()

	_, err := n.pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $4)
	`, msgID, dmID, content, now)
	if err != nil {
		return fmt.Errorf("insert system message: %w", err)
	}

	if n.hub != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type": "message.new",
			"payload": map[string]interface{}{
				"id":           msgID,
				"channel_id":   dmID,
				"sender_type":  "system",
				"sender_id":    "system",
				"sender_name":  "Solo",
				"content":      content,
				"content_type": "system",
				"created_at":   now.UTC().Format(time.RFC3339),
			},
		})
		n.hub.BroadcastToChannel(dmID, payload)
	}

	// Trigger the target agent so it processes the notification immediately.
	if n.agentSvc != nil {
		go n.agentSvc.TriggerAgentResponse(ctx, dmID, msgID, "system", "system", nil, false, nil)
	}

	return nil
}

// NotifyClaim sends a DM system message when a task is claimed.
func (n *AgentNotifier) NotifyClaim(ctx context.Context, taskID, claimerID string) error {
	tn, err := n.lookupTaskNotification(ctx, taskID, claimerID)
	if err != nil {
		return err
	}
	if tn.creatorID == claimerID {
		return nil
	}

	dmID, err := n.ensureAgentDM(ctx, tn.creatorID)
	if err != nil {
		return fmt.Errorf("ensure DM: %w", err)
	}

	content := fmt.Sprintf(
		"📋 Task claimed — #%d %s in #%s was claimed by @%s.\n\n→ Next: @%s is working on this. Wait for completion, then check if the result is correct and close the parent task if satisfied.",
		tn.taskNumber, tn.taskTitle, tn.channelName, tn.actorName, tn.actorName,
	)
	return n.sendSystemMessage(ctx, dmID, content)
}

// NotifyComplete sends a DM system message when a task is completed.
func (n *AgentNotifier) NotifyComplete(ctx context.Context, taskID, claimerID string) error {
	tn, err := n.lookupTaskNotification(ctx, taskID, claimerID)
	if err != nil {
		return err
	}
	if tn.creatorID == claimerID {
		return nil
	}

	dmID, err := n.ensureAgentDM(ctx, tn.creatorID)
	if err != nil {
		return fmt.Errorf("ensure DM: %w", err)
	}

	content := fmt.Sprintf(
		"✅ Task completed — #%d %s in #%s was completed by @%s.\n\n→ Next: Go to #%s to review the result. If OK, close the parent task to unblock the chain. If not, leave feedback for @%s.",
		tn.taskNumber, tn.taskTitle, tn.channelName, tn.actorName, tn.channelName, tn.actorName,
	)
	return n.sendSystemMessage(ctx, dmID, content)
}

// NotifyEscalation sends a DM system message when a task is overdue.
func (n *AgentNotifier) NotifyEscalation(ctx context.Context, taskID, claimerID string) error {
	tn, err := n.lookupTaskNotification(ctx, taskID, claimerID)
	if err != nil {
		return err
	}
	if tn.creatorID == claimerID {
		return nil
	}

	dmID, err := n.ensureAgentDM(ctx, tn.creatorID)
	if err != nil {
		return fmt.Errorf("ensure DM: %w", err)
	}

	content := fmt.Sprintf(
		"⚠️ Task overdue — #%d %s in #%s (claimed by @%s) is overdue and has been escalated to you.\n\n→ Next:\n1. Delegate to another agent with similar skills (check RELATIONSHIPS.md for candidates)\n2. Or check with @%s in #%s for status\n3. If you cannot handle this, inform the user directly",
		tn.taskNumber, tn.taskTitle, tn.channelName, tn.actorName, tn.actorName, tn.channelName,
	)
	return n.sendSystemMessage(ctx, dmID, content)
}

// NotifyRemind sends a DM system message to the claimer when a task is past
// deadline with timeout_action=remind. This is a soft nudge, not escalation.
func (n *AgentNotifier) NotifyRemind(ctx context.Context, taskID, claimerID string, deadline time.Time) error {
	tn, err := n.lookupTaskNotification(ctx, taskID, claimerID)
	if err != nil {
		return err
	}

	dmID, err := n.ensureAgentDM(ctx, claimerID)
	if err != nil {
		return fmt.Errorf("ensure DM: %w", err)
	}

	content := fmt.Sprintf(
		"⏰ Task overdue — #%d %s in #%s is past deadline (%s).\n\n→ Next: Check progress and update the task status, or extend the deadline.",
		tn.taskNumber, tn.taskTitle, tn.channelName, deadline.Format(time.RFC3339),
	)
	return n.sendSystemMessage(ctx, dmID, content)
}

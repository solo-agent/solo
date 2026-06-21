package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

type AgentNotifier struct {
	pool     *pgxpool.Pool
	hub      realtime.Broadcaster
	agentSvc *AgentService
}

func NewAgentNotifier(pool *pgxpool.Pool, hub realtime.Broadcaster, agentSvc *AgentService) *AgentNotifier {
	return &AgentNotifier{pool: pool, hub: hub, agentSvc: agentSvc}
}

func (n *AgentNotifier) NotifyReviewReady(ctx context.Context, taskID, actorID string) error {
	info, err := n.taskNotifyInfo(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	return n.notifyAgent(ctx, info.CreatorID, fmt.Sprintf("Task #%d %s is ready for review by @%s.", info.TaskNumber, info.Title, info.ActorName))
}

func (n *AgentNotifier) NotifyAccepted(ctx context.Context, taskID, actorID string) error {
	info, err := n.taskNotifyInfo(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	return n.notifyAgent(ctx, info.ClaimerID, fmt.Sprintf("Task #%d %s was accepted by @%s.", info.TaskNumber, info.Title, info.ActorName))
}

func (n *AgentNotifier) NotifyRejected(ctx context.Context, taskID, actorID, reason string) error {
	info, err := n.taskNotifyInfo(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	return n.notifyAgent(ctx, info.ClaimerID, fmt.Sprintf("Task #%d %s was rejected by @%s.\nReason: %s", info.TaskNumber, info.Title, info.ActorName, reason))
}

func (n *AgentNotifier) NotifyClosed(ctx context.Context, taskID, actorID string) error {
	info, err := n.taskNotifyInfo(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	return n.notifyAgent(ctx, info.ClaimerID, fmt.Sprintf("Task #%d %s was closed by @%s.", info.TaskNumber, info.Title, info.ActorName))
}

func (n *AgentNotifier) NotifyReopened(ctx context.Context, taskID, actorID string) error {
	info, err := n.taskNotifyInfo(ctx, taskID, actorID)
	if err != nil {
		return err
	}
	return n.notifyAgent(ctx, info.ClaimerID, fmt.Sprintf("Task #%d %s was reopened by @%s.", info.TaskNumber, info.Title, info.ActorName))
}

type taskNotifyInfo struct {
	TaskNumber int
	Title      string
	CreatorID  string
	ClaimerID  string
	ActorName  string
}

func (n *AgentNotifier) taskNotifyInfo(ctx context.Context, taskID, actorID string) (*taskNotifyInfo, error) {
	var info taskNotifyInfo
	if err := n.pool.QueryRow(ctx, `
		SELECT task_number, title, creator_id::text, COALESCE(claimer_id::text, '')
		FROM tasks
		WHERE id = $1
	`, taskID).Scan(&info.TaskNumber, &info.Title, &info.CreatorID, &info.ClaimerID); err != nil {
		return nil, err
	}
	info.ActorName = n.displayName(ctx, actorID)
	return &info, nil
}

func (n *AgentNotifier) notifyAgent(ctx context.Context, agentID, content string) error {
	if agentID == "" {
		return nil
	}
	dmID, ok, err := n.ensureAgentDM(ctx, agentID)
	if err != nil || !ok {
		return err
	}
	return n.sendSystemMessage(ctx, dmID, content)
}

func (n *AgentNotifier) ensureAgentDM(ctx context.Context, agentID string) (string, bool, error) {
	var ownerID, agentName string
	if err := n.pool.QueryRow(ctx, `SELECT owner_id::text, name FROM agents WHERE id = $1 AND is_active = true`, agentID).Scan(&ownerID, &agentName); err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	var dmID string
	err := n.pool.QueryRow(ctx, `
		SELECT dm1.channel_id::text
		FROM dm_members dm1
		JOIN dm_members dm2 ON dm1.channel_id = dm2.channel_id
		JOIN channels c ON c.id = dm1.channel_id AND c.type = 'dm' AND c.is_archived = false
		WHERE dm1.member_type = 'user' AND dm1.member_id = $1
		  AND dm2.member_type = 'agent' AND dm2.member_id = $2
		LIMIT 1
	`, ownerID, agentID).Scan(&dmID)
	if err == nil {
		return dmID, true, nil
	}
	if err != pgx.ErrNoRows {
		return "", false, err
	}

	tx, err := n.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx, `
		INSERT INTO channels (name, description, type, created_by)
		VALUES ($1, 'Direct Message', 'dm', $2)
		RETURNING id::text
	`, agentName, ownerID).Scan(&dmID); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO channel_members (channel_id, member_type, member_id, role)
		VALUES ($1, 'user', $2, 'owner'), ($1, 'agent', $3, 'member')
	`, dmID, ownerID, agentID); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dm_members (channel_id, member_type, member_id)
		VALUES ($1, 'user', $2), ($1, 'agent', $3)
	`, dmID, ownerID, agentID); err != nil {
		return "", false, err
	}
	return dmID, true, tx.Commit(ctx)
}

func (n *AgentNotifier) sendSystemMessage(ctx context.Context, dmID, content string) error {
	msgID := uuid.New().String()
	now := time.Now()
	if _, err := n.pool.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $4)
	`, msgID, dmID, content, now); err != nil {
		return err
	}

	if n.hub != nil {
		payload := realtime.Envelope("message.new", map[string]any{
			"id":           msgID,
			"channel_id":   dmID,
			"sender_type":  "system",
			"sender_id":    "system",
			"sender_name":  "Solo",
			"content":      content,
			"content_type": "system",
			"created_at":   now.UTC().Format(time.RFC3339),
		})
		n.hub.BroadcastToChannel(dmID, payload)
	}
	if n.agentSvc != nil {
		go n.agentSvc.TriggerAgentResponse(context.Background(), dmID, msgID, "system", "system", nil, false, nil)
	}
	return nil
}

func (n *AgentNotifier) displayName(ctx context.Context, id string) string {
	var name string
	_ = n.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT display_name FROM users WHERE id = $1),
			(SELECT name FROM agents WHERE id = $1),
			$1::text
		)
	`, id).Scan(&name)
	return name
}

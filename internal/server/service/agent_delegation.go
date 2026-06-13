package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// Sentinel errors for delegation state transitions.
var (
	ErrDelegationNotFound      = errors.New("delegation not found")
	ErrInvalidStatusTransition = errors.New("invalid status transition for delegation")
)

type AgentDelegationService struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

// SetHub injects a WebSocket broadcaster for delegation_updated events.
func (s *AgentDelegationService) SetHub(hub realtime.Broadcaster) {
	s.hub = hub
}

type AgentDelegation struct {
	ID              string  `json:"id"`
	FromAgentID     string  `json:"from_agent_id"`
	ToAgentID       string  `json:"to_agent_id"`
	TaskID          *string `json:"task_id,omitempty"`
	ChannelID       string  `json:"channel_id"`
	Status          string  `json:"status"`
	Message         *string `json:"message,omitempty"`
	StartIfInactive bool    `json:"start_if_inactive"`
	RejectionReason *string `json:"rejection_reason,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func NewAgentDelegationService(pool *pgxpool.Pool) *AgentDelegationService {
	return &AgentDelegationService{pool: pool}
}

// Create creates a new delegation in "queued" status.
func (s *AgentDelegationService) Create(ctx context.Context, req CreateDelegationRequest) (*AgentDelegation, error) {
	if req.FromAgentID == req.ToAgentID {
		return nil, fmt.Errorf("cannot delegate to yourself")
	}

	var d AgentDelegation
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agent_delegations (from_agent_id, to_agent_id, task_id, channel_id, message, start_if_inactive)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, from_agent_id, to_agent_id, task_id, channel_id, status, message,
		          start_if_inactive, rejection_reason, created_at::text, updated_at::text
	`, req.FromAgentID, req.ToAgentID, req.TaskID, req.ChannelID, req.Message, req.StartIfInactive).Scan(
		&d.ID, &d.FromAgentID, &d.ToAgentID, &d.TaskID, &d.ChannelID, &d.Status, &d.Message,
		&d.StartIfInactive, &d.RejectionReason, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create delegation: %w", err)
	}
	return &d, nil
}

// Accept transitions a delegation from queued/delivered to started.
func (s *AgentDelegationService) Accept(ctx context.Context, delegationID, agentID string) (*AgentDelegation, error) {
	d, err := s.transitionStatus(ctx, delegationID, agentID, "started", []string{"queued", "delivered"})
	if err != nil {
		return nil, err
	}
	s.broadcastDelegationUpdated(d)
	return d, nil
}

// MarkDelivered marks a delegation as delivered (BUG-009).
func (s *AgentDelegationService) MarkDelivered(ctx context.Context, delegationID, agentID string) (*AgentDelegation, error) {
	d, err := s.transitionStatus(ctx, delegationID, agentID, "delivered", []string{"queued"})
	if err != nil {
		return nil, err
	}
	s.broadcastDelegationUpdated(d)
	return d, nil
}

// Reject transitions a delegation from queued/delivered to rejected.
func (s *AgentDelegationService) Reject(ctx context.Context, delegationID, agentID, reason string) (*AgentDelegation, error) {
	d, err := s.transitionStatus(ctx, delegationID, agentID, "rejected", []string{"queued", "delivered"})
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `UPDATE agent_delegations SET rejection_reason = $1 WHERE id = $2`, reason, delegationID)
	if err != nil {
		return nil, err
	}
	d.RejectionReason = &reason
	s.broadcastDelegationUpdated(d)
	return d, nil
}

// MarkComplete marks a delegation as completed.
func (s *AgentDelegationService) MarkComplete(ctx context.Context, delegationID, agentID string) (*AgentDelegation, error) {
	d, err := s.transitionStatus(ctx, delegationID, agentID, "completed", []string{"started"})
	if err != nil {
		return nil, err
	}
	s.broadcastDelegationUpdated(d)
	return d, nil
}

// MarkFailed marks a delegation as failed.
func (s *AgentDelegationService) MarkFailed(ctx context.Context, delegationID, agentID string) (*AgentDelegation, error) {
	d, err := s.transitionStatus(ctx, delegationID, agentID, "failed", []string{"queued", "delivered", "started"})
	if err != nil {
		return nil, err
	}
	s.broadcastDelegationUpdated(d)
	return d, nil
}

// broadcastDelegationUpdated sends a delegation_updated WS event to the
// delegation's channel so all connected clients see the state change.
func (s *AgentDelegationService) broadcastDelegationUpdated(d *AgentDelegation) {
	if s.hub == nil || d == nil {
		return
	}
	payload, err := json.Marshal(d)
	if err != nil {
		slog.Warn("delegation: failed to marshal broadcast payload", "delegation_id", d.ID, "error", err)
		return
	}
	envelope := realtime.Envelope("delegation_updated", json.RawMessage(payload))
	s.hub.BroadcastToChannel(d.ChannelID, envelope)
}

func (s *AgentDelegationService) transitionStatus(ctx context.Context, id, agentID, newStatus string, allowedFrom []string) (*AgentDelegation, error) {
	var d AgentDelegation
	err := s.pool.QueryRow(ctx, `
		UPDATE agent_delegations
		SET status = $1, updated_at = $2
		WHERE id = $3 AND to_agent_id = $4 AND status = ANY($5)
		RETURNING id, from_agent_id, to_agent_id, task_id, channel_id, status, message,
		          start_if_inactive, rejection_reason, created_at::text, updated_at::text
	`, newStatus, time.Now().UTC(), id, agentID, allowedFrom).Scan(
		&d.ID, &d.FromAgentID, &d.ToAgentID, &d.TaskID, &d.ChannelID, &d.Status, &d.Message,
		&d.StartIfInactive, &d.RejectionReason, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrInvalidStatusTransition
	}
	if err != nil {
		return nil, fmt.Errorf("transition delegation: %w", err)
	}
	return &d, nil
}

// ListIncoming returns delegations addressed to the given agent.
func (s *AgentDelegationService) ListIncoming(ctx context.Context, agentID string, status string) ([]AgentDelegation, error) {
	return s.list(ctx, "to_agent_id", agentID, status)
}

// ListOutgoing returns delegations created by the given agent.
func (s *AgentDelegationService) ListOutgoing(ctx context.Context, agentID string, status string) ([]AgentDelegation, error) {
	return s.list(ctx, "from_agent_id", agentID, status)
}

func (s *AgentDelegationService) list(ctx context.Context, column, agentID, status string) ([]AgentDelegation, error) {
	query := fmt.Sprintf(`
		SELECT id, from_agent_id, to_agent_id, task_id, channel_id, status, message,
		       start_if_inactive, rejection_reason, created_at::text, updated_at::text
		FROM agent_delegations WHERE %s = $1`, column)
	args := []any{agentID}

	if status != "" {
		query += " AND status = $2"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list delegations: %w", err)
	}
	defer rows.Close()

	var delegations []AgentDelegation
	for rows.Next() {
		var d AgentDelegation
		if err := rows.Scan(&d.ID, &d.FromAgentID, &d.ToAgentID, &d.TaskID, &d.ChannelID,
			&d.Status, &d.Message, &d.StartIfInactive, &d.RejectionReason, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan delegation: %w", err)
		}
		delegations = append(delegations, d)
	}
	return delegations, nil
}

type CreateDelegationRequest struct {
	FromAgentID     string  `json:"from_agent_id"`
	ToAgentID       string  `json:"to_agent_id"`
	TaskID          *string `json:"task_id,omitempty"`
	ChannelID       string  `json:"channel_id"`
	Message         *string `json:"message,omitempty"`
	StartIfInactive bool    `json:"start_if_inactive,omitempty"`
}

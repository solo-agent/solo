package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// AgentRelationshipService manages agent-to-agent relationships.
type AgentRelationshipService struct {
	pool *pgxpool.Pool
	hub  realtime.Broadcaster
}

// SetHub injects a WebSocket broadcaster for real-time events.
func (s *AgentRelationshipService) SetHub(hub realtime.Broadcaster) {
	s.hub = hub
}

// AgentRelationship represents a single relationship between two agents.
type AgentRelationship struct {
	ID          string  `json:"id"`
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	RelType     string  `json:"rel_type"`
	ChannelID   *string `json:"channel_id,omitempty"`
	Weight      float64 `json:"weight"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func NewAgentRelationshipService(pool *pgxpool.Pool) *AgentRelationshipService {
	return &AgentRelationshipService{pool: pool}
}

// Valid relationship types (post Item 1: 2-type model).
var ValidRelTypes = map[string]bool{
	"assigns_to":        true,
	"collaborates_with": true,
}

// Create inserts a new relationship after validating constraints.
// Returns an error if the relationship would create a cycle (assigns_to only).
func (s *AgentRelationshipService) Create(ctx context.Context, req CreateRelationshipRequest) (*AgentRelationship, error) {
	if req.FromAgentID == req.ToAgentID {
		return nil, fmt.Errorf("self-referential relationships are not allowed")
	}

	// BUG-006: Enforce channel_id scope rules.
	// Post-Item 1: assigns_to is channel-scoped or global; collaborates_with is channel-scoped.
	// Allow both to be channel-scoped (channel_id optional for assigns_to, required for collaborates_with).
	switch req.RelType {
	case "assigns_to":
		// OK either way — global or channel-scoped.
	case "collaborates_with":
		if req.ChannelID == nil || *req.ChannelID == "" {
			return nil, fmt.Errorf("rel_type collaborates_with requires a channel_id")
		}
	default:
		return nil, fmt.Errorf("invalid rel_type: %s", req.RelType)
	}

	// BUG-010: Validate weight range.
	if req.Weight < 0.0 || req.Weight > 10.0 {
		return nil, fmt.Errorf("weight must be between 0.0 and 10.0, got %f", req.Weight)
	}

	// Cycle detection for assigns_to.
	if req.RelType == "assigns_to" {
		if err := s.checkReportsToCycle(ctx, req.FromAgentID, req.ToAgentID); err != nil {
			return nil, err
		}
	}

	var rel AgentRelationship
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, channel_id, weight)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		          created_at::text, updated_at::text
	`, req.FromAgentID, req.ToAgentID, req.RelType, req.ChannelID, req.Weight).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.ChannelID,
		&rel.Weight, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create relationship: %w", err)
	}
	// Broadcast relationship_created event.
	if s.hub != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "relationship_created",
			"payload": rel,
		})
		s.hub.Broadcast(payload)
	}

	return &rel, nil
}

// checkReportsToCycle uses BFS to walk all reports_to chains upward from
// toAgent. If fromAgent appears anywhere in the transitive closure, adding
// this edge would form a cycle.
func (s *AgentRelationshipService) checkReportsToCycle(ctx context.Context, fromAgentID, toAgentID string) error {
	visited := map[string]bool{fromAgentID: true}
	queue := []string{toAgentID}
	depth := 0

	for len(queue) > 0 && depth < 100 {
		next := queue[0]
		queue = queue[1:]
		if visited[next] {
			continue
		}
		visited[next] = true
		depth++

		rows, err := s.pool.Query(ctx, `
			SELECT to_agent_id FROM agent_relationships
			WHERE from_agent_id = $1 AND rel_type = 'assigns_to'
		`, next)
		if err != nil {
			return fmt.Errorf("cycle check failed: %w", err)
		}
		var managers []string
		for rows.Next() {
			var m string
			if err := rows.Scan(&m); err != nil {
				rows.Close()
				return err
			}
			managers = append(managers, m)
		}
		rows.Close()
		queue = append(queue, managers...)
	}
	if depth >= 100 {
		return fmt.Errorf("reports_to chain exceeds maximum depth (100)")
	}
	return nil
}

// ListByAgent returns all relationships involving the given agent.
func (s *AgentRelationshipService) ListByAgent(ctx context.Context, agentID string) ([]AgentRelationship, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		       created_at::text, updated_at::text
		FROM agent_relationships
		WHERE from_agent_id = $1 OR to_agent_id = $1
		ORDER BY created_at DESC
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list relationships: %w", err)
	}
	defer rows.Close()
	return scanRelationships(rows)
}

// ListByChannel returns channel-scoped relationships (BUG-008: data leak fix).
func (s *AgentRelationshipService) ListByChannel(ctx context.Context, channelID string) ([]AgentRelationship, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		       created_at::text, updated_at::text
		FROM agent_relationships
		WHERE channel_id = $1
		ORDER BY rel_type, created_at DESC
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel relationships: %w", err)
	}
	defer rows.Close()
	return scanRelationships(rows)
}

// UpdateWeight changes the weight of an existing relationship.
func (s *AgentRelationshipService) UpdateWeight(ctx context.Context, id string, weight float64) (*AgentRelationship, error) {
	// BUG-010: Validate weight range.
	if weight < 0.0 || weight > 10.0 {
		return nil, fmt.Errorf("weight must be between 0.0 and 10.0, got %f", weight)
	}

	var rel AgentRelationship
	err := s.pool.QueryRow(ctx, `
		UPDATE agent_relationships SET weight = $1, updated_at = now()
		WHERE id = $2
		RETURNING id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		          created_at::text, updated_at::text
	`, weight, id).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.ChannelID,
		&rel.Weight, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("relationship not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("update relationship: %w", err)
	}
	return &rel, nil
}

// Delete removes a relationship by ID.
func (s *AgentRelationshipService) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agent_relationships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete relationship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("relationship not found: %s", id)
	}
	// Broadcast relationship_deleted event.
	if s.hub != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "relationship_deleted",
			"payload": map[string]string{"id": id},
		})
		s.hub.Broadcast(payload)
	}

	return nil
}

type CreateRelationshipRequest struct {
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	RelType     string  `json:"rel_type"`
	ChannelID   *string `json:"channel_id,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
}

// List returns relationships with optional query filters (T1.1.3).
func (s *AgentRelationshipService) List(ctx context.Context, fromAgentID, toAgentID, relType, channelID string) ([]AgentRelationship, error) {
	query := `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		       created_at::text, updated_at::text
		FROM agent_relationships WHERE 1=1`
	args := []any{}
	argIdx := 1

	if fromAgentID != "" {
		query += fmt.Sprintf(" AND from_agent_id = $%d", argIdx)
		args = append(args, fromAgentID)
		argIdx++
	}
	if toAgentID != "" {
		query += fmt.Sprintf(" AND to_agent_id = $%d", argIdx)
		args = append(args, toAgentID)
		argIdx++
	}
	if relType != "" {
		query += fmt.Sprintf(" AND rel_type = $%d", argIdx)
		args = append(args, relType)
		argIdx++
	}
	if channelID != "" {
		query += fmt.Sprintf(" AND channel_id = $%d", argIdx)
		args = append(args, channelID)
		argIdx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list relationships: %w", err)
	}
	defer rows.Close()
	return scanRelationships(rows)
}

// CheckCycle checks whether adding a relationship from fromAgent to toAgent
// with the given rel_type would create a cycle (T1.1.3).
func (s *AgentRelationshipService) CheckCycle(ctx context.Context, fromAgentID, toAgentID, relType string) (bool, error) {
	if relType != "assigns_to" {
		return false, nil // only assigns_to can create cycles
	}
	visited := map[string]bool{fromAgentID: true}
	queue := []string{toAgentID}
	depth := 0

	for len(queue) > 0 && depth < 100 {
		next := queue[0]
		queue = queue[1:]
		if visited[next] {
			continue
		}
		visited[next] = true
		depth++

		rows, err := s.pool.Query(ctx, `
			SELECT to_agent_id FROM agent_relationships
			WHERE from_agent_id = $1 AND rel_type = 'assigns_to'
		`, next)
		if err != nil {
			return false, fmt.Errorf("cycle check failed: %w", err)
		}
		var managers []string
		for rows.Next() {
			var m string
			if err := rows.Scan(&m); err != nil {
				rows.Close()
				return false, err
			}
			managers = append(managers, m)
		}
		rows.Close()
		queue = append(queue, managers...)
	}
	if depth >= 100 {
		return false, fmt.Errorf("reports_to chain exceeds maximum depth (100)")
	}
	return false, nil
}

func scanRelationships(rows pgx.Rows) ([]AgentRelationship, error) {
	var rels []AgentRelationship
	for rows.Next() {
		var r AgentRelationship
		if err := rows.Scan(&r.ID, &r.FromAgentID, &r.ToAgentID, &r.RelType,
			&r.ChannelID, &r.Weight, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		rels = append(rels, r)
	}
	return rels, nil
}

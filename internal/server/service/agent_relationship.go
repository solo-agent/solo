package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentRelationshipService manages agent-to-agent relationships.
type AgentRelationshipService struct {
	pool *pgxpool.Pool
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

// Create inserts a new relationship after validating constraints.
// Returns an error if the relationship would create a cycle (reports_to only).
func (s *AgentRelationshipService) Create(ctx context.Context, req CreateRelationshipRequest) (*AgentRelationship, error) {
	if req.FromAgentID == req.ToAgentID {
		return nil, fmt.Errorf("self-referential relationships are not allowed")
	}

	// Cycle detection for reports_to.
	if req.RelType == "reports_to" {
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
			WHERE from_agent_id = $1 AND rel_type = 'reports_to'
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

// ListByChannel returns channel-scoped relationships.
func (s *AgentRelationshipService) ListByChannel(ctx context.Context, channelID string) ([]AgentRelationship, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		       created_at::text, updated_at::text
		FROM agent_relationships
		WHERE channel_id = $1
		   OR rel_type IN ('reports_to', 'escalates_to')
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
	return nil
}

type CreateRelationshipRequest struct {
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	RelType     string  `json:"rel_type"`
	ChannelID   *string `json:"channel_id,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
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

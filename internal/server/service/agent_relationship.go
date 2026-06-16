package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// AgentRelationshipService manages agent-to-agent relationships.
type AgentRelationshipService struct {
	pool           *pgxpool.Pool
	hub            realtime.Broadcaster
	eventPublisher *RelationshipEventPublisher
	mdGen          *RelationshipsMDGenerator
}

// SetHub injects a WebSocket broadcaster for real-time events.
func (s *AgentRelationshipService) SetHub(hub realtime.Broadcaster) {
	s.hub = hub
}

// SetEventPublisher injects the relationship event publisher.
func (s *AgentRelationshipService) SetEventPublisher(p *RelationshipEventPublisher) {
	s.eventPublisher = p
}

// SetMDGenerator injects the relationships MD generator.
func (s *AgentRelationshipService) SetMDGenerator(g *RelationshipsMDGenerator) {
	s.mdGen = g
}

// AgentRelationship represents a single relationship between two agents.
type AgentRelationship struct {
	ID          string  `json:"id"`
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	RelType     string  `json:"rel_type"`
	ChannelID   *string `json:"channel_id,omitempty"`
	Weight      float64 `json:"weight"`
	Instruction string  `json:"instruction,omitempty"`
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

	// Both assigns_to and collaborates_with are global — no channel_id required.
	if _, ok := ValidRelTypes[req.RelType]; !ok {
		return nil, fmt.Errorf("invalid rel_type: %s", req.RelType)
	}

	// BUG-010: Validate weight range.
	if req.Weight < 0.0 || req.Weight > 10.0 {
		return nil, fmt.Errorf("weight must be between 0.0 and 10.0, got %f", req.Weight)
	}

	// Cycle detection for assigns_to.
	if req.RelType == "assigns_to" {
		if err := s.checkAssignsToCycle(ctx, req.FromAgentID, req.ToAgentID); err != nil {
			return nil, err
		}
	}

	var rel AgentRelationship
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, channel_id, weight, instruction)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, from_agent_id, to_agent_id, rel_type, channel_id, weight, instruction,
		          created_at::text, updated_at::text
	`, req.FromAgentID, req.ToAgentID, req.RelType, req.ChannelID, req.Weight, req.Instruction).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.ChannelID,
		&rel.Weight, &rel.Instruction, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create relationship: %w", err)
	}
	// Broadcast relationship_created event.
	if s.eventPublisher != nil {
		s.eventPublisher.PublishCreated(ctx, rel.ID, rel.FromAgentID, rel.ToAgentID, rel.RelType)
	}
	if s.mdGen != nil {
		if err := s.mdGen.GenerateForAgent(ctx, rel.FromAgentID); err != nil {
			slog.Warn("regenerate RELATIONSHIPS.md failed", "agent_id", rel.FromAgentID, "err", err)
		}
		if err := s.mdGen.GenerateForAgent(ctx, rel.ToAgentID); err != nil {
			slog.Warn("regenerate RELATIONSHIPS.md failed", "agent_id", rel.ToAgentID, "err", err)
		}
	}

	return &rel, nil
}

// checkAssignsToCycle uses BFS to walk all assigns_to chains upward from
// toAgent. If fromAgent appears anywhere in the transitive closure, adding
// this edge would form a cycle.
func (s *AgentRelationshipService) checkAssignsToCycle(ctx context.Context, fromAgentID, toAgentID string) error {
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
		return fmt.Errorf("assigns_to chain exceeds maximum depth (100)")
	}
	return nil
}

// ListByAgent returns all relationships involving the given agent.
func (s *AgentRelationshipService) ListByAgent(ctx context.Context, agentID string) ([]AgentRelationship, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight, COALESCE(instruction, '') as instruction,
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
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight, COALESCE(instruction, '') as instruction,
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
		          COALESCE(instruction, '') as instruction, created_at::text, updated_at::text
	`, weight, id).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.ChannelID,
		&rel.Weight, &rel.Instruction, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("relationship not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("update relationship: %w", err)
	}
	return &rel, nil
}

// UpdateInstruction changes the instruction text of an existing relationship.
func (s *AgentRelationshipService) UpdateInstruction(ctx context.Context, id string, instruction string) (*AgentRelationship, error) {
	var rel AgentRelationship
	err := s.pool.QueryRow(ctx, `
		UPDATE agent_relationships SET instruction = $1, updated_at = now()
		WHERE id = $2
		RETURNING id, from_agent_id, to_agent_id, rel_type, channel_id, weight,
		          COALESCE(instruction, '') as instruction, created_at::text, updated_at::text
	`, instruction, id).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.ChannelID,
		&rel.Weight, &rel.Instruction, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("relationship not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("update relationship instruction: %w", err)
	}
	// Regenerate RELATIONSHIPS.md for both agents.
	if s.mdGen != nil {
		s.mdGen.GenerateForAgent(ctx, rel.FromAgentID)
		s.mdGen.GenerateForAgent(ctx, rel.ToAgentID)
	}
	return &rel, nil
}

// Delete removes a relationship by ID.
func (s *AgentRelationshipService) Delete(ctx context.Context, id string) error {
	var fromAgent, toAgent, relType string
	err := s.pool.QueryRow(ctx, `
		SELECT from_agent_id::text, to_agent_id::text, rel_type
		  FROM agent_relationships
		 WHERE id = $1
	`, id).Scan(&fromAgent, &toAgent, &relType)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("relationship not found: %s", id)
	}
	if err != nil {
		return fmt.Errorf("lookup relationship: %w", err)
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM agent_relationships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete relationship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("relationship not found: %s", id)
	}
	// Broadcast relationship_deleted event.
	if s.eventPublisher != nil {
		s.eventPublisher.PublishDeleted(ctx, id, fromAgent, toAgent, relType)
	}
	if s.mdGen != nil && fromAgent != "" {
		if err := s.mdGen.GenerateForAgent(ctx, fromAgent); err != nil {
			slog.Warn("regenerate RELATIONSHIPS.md failed", "agent_id", fromAgent, "err", err)
		}
	}
	if s.mdGen != nil && toAgent != "" {
		if err := s.mdGen.GenerateForAgent(ctx, toAgent); err != nil {
			slog.Warn("regenerate RELATIONSHIPS.md failed", "agent_id", toAgent, "err", err)
		}
	}

	return nil
}

type CreateRelationshipRequest struct {
	FromAgentID string  `json:"from_agent_id"`
	ToAgentID   string  `json:"to_agent_id"`
	RelType     string  `json:"rel_type"`
	ChannelID   *string `json:"channel_id,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Instruction string  `json:"instruction,omitempty"`
}

// List returns relationships with optional query filters (T1.1.3).
func (s *AgentRelationshipService) List(ctx context.Context, fromAgentID, toAgentID, relType, channelID string) ([]AgentRelationship, error) {
	query := `
		SELECT id, from_agent_id, to_agent_id, rel_type, channel_id, weight, COALESCE(instruction, '') as instruction,
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
		return false, fmt.Errorf("assigns_to chain exceeds maximum depth (100)")
	}
	return false, nil
}

func scanRelationships(rows pgx.Rows) ([]AgentRelationship, error) {
	var rels []AgentRelationship
	for rows.Next() {
		var r AgentRelationship
		if err := rows.Scan(&r.ID, &r.FromAgentID, &r.ToAgentID, &r.RelType,
			&r.ChannelID, &r.Weight, &r.Instruction, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		rels = append(rels, r)
	}
	return rels, nil
}

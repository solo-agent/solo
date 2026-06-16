package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TemplateService applies team templates to create agents + relationships.
type TemplateService struct {
	pool *pgxpool.Pool
	md   *RelationshipsMDGenerator
}

func NewTemplateService(pool *pgxpool.Pool, md *RelationshipsMDGenerator) *TemplateService {
	return &TemplateService{pool: pool, md: md}
}

type ApplyResult struct {
	CreatedAgentIDs        []string `json:"created_agent_ids"`
	CreatedRelationshipIDs []string `json:"created_relationship_ids"`
	TemplateID             string   `json:"template_id"`
}

type templateMember struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
	Relationship string `json:"relationship"`
}

// Apply creates agents and relationships for the given template in a transaction.
// ownerID must reference an existing user (FK constraint).
func (s *TemplateService) Apply(ctx context.Context, templateID, ownerID string) (*ApplyResult, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT members FROM agent_templates WHERE id = $1`, templateID).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}
	var members []templateMember
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("parse template members: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	nameToID := make(map[string]string, len(members))
	var createdAgents []string
	for _, m := range members {
		var newID string
		err := tx.QueryRow(ctx, `
			INSERT INTO agents (name, owner_id, model_provider, model_name, system_prompt, is_active)
			VALUES ($1, $2, 'anthropic', 'claude-sonnet-4-5', $3, true)
			RETURNING id
		`, m.Name, ownerID, m.Instructions).Scan(&newID)
		if err != nil {
			return nil, fmt.Errorf("insert agent %s: %w", m.Name, err)
		}
		nameToID[m.Name] = newID
		createdAgents = append(createdAgents, newID)
	}

	var leaderName string
	for _, m := range members {
		if m.Role == "leader" {
			leaderName = m.Name
			break
		}
	}
	if leaderName == "" && len(members) > 0 {
		leaderName = members[0].Name
	}
	leaderID := nameToID[leaderName]

	var createdRels []string
	for _, m := range members {
		if m.Name == leaderName {
			continue
		}
		otherID := nameToID[m.Name]
		var relID string
		err := tx.QueryRow(ctx, `
			INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, weight, instruction)
			VALUES ($1, $2, 'assigns_to', 1.0, $3)
			RETURNING id
		`, leaderID, otherID, m.Relationship).Scan(&relID)
		if err != nil {
			return nil, fmt.Errorf("insert assigns_to: %w", err)
		}
		createdRels = append(createdRels, relID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	for _, id := range createdAgents {
		if err := s.md.GenerateForAgent(ctx, id); err != nil {
			slog.Warn("md generate failed for template agent", "agent_id", id, "err", err)
		}
	}

	return &ApplyResult{
		CreatedAgentIDs:        createdAgents,
		CreatedRelationshipIDs: createdRels,
		TemplateID:             templateID,
	}, nil
}

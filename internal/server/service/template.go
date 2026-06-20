package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TemplateService struct {
	pool  *pgxpool.Pool
	mdGen *RelationshipsMDGenerator
}

func NewTemplateService(pool *pgxpool.Pool, mdGen ...*RelationshipsMDGenerator) *TemplateService {
	s := &TemplateService{pool: pool}
	if len(mdGen) > 0 {
		s.mdGen = mdGen[0]
	}
	return s
}

type AgentTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	MemberCount int    `json:"member_count"`
}

type ApplyTemplateResult struct {
	CreatedAgentIDs        []string `json:"created_agent_ids"`
	CreatedRelationshipIDs []string `json:"created_relationship_ids"`
	TemplateID             string   `json:"template_id"`
}

type templateMember struct {
	Role         string  `json:"role"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Instructions string  `json:"instructions"`
	Relationship *string `json:"relationship"`
}

func (s *TemplateService) List(ctx context.Context) ([]AgentTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, category, icon, jsonb_array_length(members)
		  FROM agent_templates
		 ORDER BY category ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []AgentTemplate{}
	for rows.Next() {
		var tmpl AgentTemplate
		if err := rows.Scan(&tmpl.ID, &tmpl.Name, &tmpl.Description, &tmpl.Category, &tmpl.Icon, &tmpl.MemberCount); err != nil {
			return nil, err
		}
		templates = append(templates, tmpl)
	}
	return templates, rows.Err()
}

func (s *TemplateService) Apply(ctx context.Context, templateID, ownerID, modelProvider string) (*ApplyTemplateResult, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT members FROM agent_templates WHERE id = $1`, templateID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}

	var members []templateMember
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("template has no members")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var computerID *string
	var cid string
	if err := tx.QueryRow(ctx, `SELECT id FROM computers WHERE status = 'online' ORDER BY created_at ASC LIMIT 1`).Scan(&cid); err == nil && cid != "" {
		computerID = &cid
	}

	nameToID := map[string]string{}
	createdAgents := []string{}
	for _, member := range members {
		var existingID string
		if err := tx.QueryRow(ctx,
			`SELECT id FROM agents WHERE owner_id = $1 AND name = $2 AND is_active = true`,
			ownerID, member.Name,
		).Scan(&existingID); err == nil {
			nameToID[member.Name] = existingID
			continue
		}

		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO agents (name, description, owner_id, model_provider, model_name, system_prompt, runtime_id, custom_env, custom_args)
			VALUES ($1, $2, $3, $4, '', $5, $6, '{}'::jsonb, '[]'::jsonb)
			RETURNING id
		`, member.Name, member.Description, ownerID, modelProvider, member.Instructions, computerID).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("create agent %s: %w", member.Name, err)
		}
		nameToID[member.Name] = id
		createdAgents = append(createdAgents, id)
	}

	leaderName := members[0].Name
	for _, member := range members {
		if member.Role == "leader" {
			leaderName = member.Name
			break
		}
	}
	leaderID := nameToID[leaderName]

	createdRelationships := []string{}
	for _, member := range members {
		if member.Name == leaderName || member.Relationship == nil {
			continue
		}
		toID := nameToID[member.Name]
		var existingID string
		if err := tx.QueryRow(ctx,
			`SELECT id FROM agent_relationships WHERE from_agent_id = $1 AND to_agent_id = $2 AND rel_type = 'assigns_to'`,
			leaderID, toID,
		).Scan(&existingID); err == nil {
			continue
		}

		var relID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, weight, instruction)
			VALUES ($1, $2, 'assigns_to', 1, $3)
			RETURNING id
		`, leaderID, toID, *member.Relationship).Scan(&relID); err != nil {
			return nil, fmt.Errorf("create relationship: %w", err)
		}
		createdRelationships = append(createdRelationships, relID)
	}

	var allID string
	_ = tx.QueryRow(ctx,
		`SELECT c.id FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user' AND cm.member_id = $1
		   AND c.name LIKE 'all-%' AND c.is_archived = false
		 LIMIT 1`,
		ownerID,
	).Scan(&allID)
	if allID != "" {
		for _, id := range createdAgents {
			_, _ = tx.Exec(ctx,
				`INSERT INTO channel_members (channel_id, member_type, member_id, role)
				 VALUES ($1, 'agent', $2, 'member')
				 ON CONFLICT DO NOTHING`,
				allID, id,
			)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if s.mdGen != nil {
		for _, id := range nameToID {
			_ = s.mdGen.GenerateForAgent(ctx, id)
		}
	}

	return &ApplyTemplateResult{
		CreatedAgentIDs:        createdAgents,
		CreatedRelationshipIDs: createdRelationships,
		TemplateID:             templateID,
	}, nil
}

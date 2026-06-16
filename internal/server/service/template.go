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
	Role          string `json:"role"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Instructions  string `json:"instructions"`
	Relationship  string `json:"relationship"`
	Collaboration string `json:"collaboration,omitempty"`
}

// Apply creates agents and relationships for the given template.
// Idempotent: skips agents that already exist (same owner + name + active).
// Auto-binds to first online computer and joins the user's #all-* channel.
func (s *TemplateService) Apply(ctx context.Context, templateID, ownerID, modelProvider string) (*ApplyResult, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT members FROM agent_templates WHERE id = $1`, templateID).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}
	var members []templateMember
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("parse template members: %w", err)
	}

	// Find #all-* channel for auto-join (post-commit).
	var allID string
	_ = s.pool.QueryRow(ctx,
		`SELECT c.id FROM channels c
		 JOIN channel_members cm ON cm.channel_id = c.id
		 WHERE cm.member_type = 'user' AND cm.member_id = $1
		 AND c.name LIKE 'all-%' AND c.is_archived = false
		 LIMIT 1`,
		ownerID,
	).Scan(&allID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Auto-bind to first online computer inside tx.
	var computerID *string
	{
		var cid string
		if err := tx.QueryRow(ctx,
			`SELECT id FROM computers WHERE status = 'online' ORDER BY created_at ASC LIMIT 1`,
		).Scan(&cid); err == nil && cid != "" {
			computerID = &cid
		}
	}

	// Create agents — skip if same owner+name+active already exists.
	nameToID := make(map[string]string, len(members))
	var createdAgents []string

	for _, m := range members {
		// Idempotent: reuse existing active agent.
		var existingID string
		if err := tx.QueryRow(ctx,
			`SELECT id FROM agents WHERE owner_id = $1 AND name = $2 AND is_active = true`,
			ownerID, m.Name,
		).Scan(&existingID); err == nil {
			nameToID[m.Name] = existingID
			continue
		}

		modelName := ""

		var newID string
		err := tx.QueryRow(ctx, `
			INSERT INTO agents (name, description, owner_id, model_provider, model_name, system_prompt, runtime_id, custom_env, custom_args, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, '{}'::jsonb, '[]'::jsonb, true)
			RETURNING id
		`, m.Name, m.Description, ownerID, modelProvider, modelName, m.Instructions, computerID).Scan(&newID)
		if err != nil {
			return nil, fmt.Errorf("insert agent %s: %w", m.Name, err)
		}
		nameToID[m.Name] = newID
		createdAgents = append(createdAgents, newID)
	}

	// Find leader.
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

	// Create assigns_to edges (leader → member).
	var createdRels []string
	for _, m := range members {
		if m.Name == leaderName {
			continue
		}
		otherID := nameToID[m.Name]

		// Idempotent: skip if relationship already exists.
		var existingRelID string
		if tx.QueryRow(ctx,
			`SELECT id FROM agent_relationships WHERE from_agent_id = $1 AND to_agent_id = $2 AND rel_type = 'assigns_to'`,
			leaderID, otherID,
		).Scan(&existingRelID) == nil {
			continue
		}

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

	// Create collaborates_with edges between non-leader members.
	nonLeaders := make([]templateMember, 0)
	for _, m := range members {
		if m.Name != leaderName {
			nonLeaders = append(nonLeaders, m)
		}
	}
	for i := 0; i < len(nonLeaders); i++ {
		for j := i + 1; j < len(nonLeaders); j++ {
			a, b := nonLeaders[i], nonLeaders[j]
			instruction := ""
			if a.Collaboration != "" {
				instruction = a.Collaboration
			} else if b.Collaboration != "" {
				instruction = b.Collaboration
			}
			if instruction == "" {
				continue
			}

			aid, bid := nameToID[a.Name], nameToID[b.Name]

			// Idempotent: skip if collaboration already exists (either direction).
			var existingRelID string
			if tx.QueryRow(ctx,
				`SELECT id FROM agent_relationships WHERE rel_type = 'collaborates_with' AND ((from_agent_id = $1 AND to_agent_id = $2) OR (from_agent_id = $2 AND to_agent_id = $1))`,
				aid, bid,
			).Scan(&existingRelID) == nil {
				continue
			}

			var relID string
			err := tx.QueryRow(ctx, `
				INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, weight, instruction)
				VALUES ($1, $2, 'collaborates_with', 1.0, $3)
				RETURNING id
			`, aid, bid, instruction).Scan(&relID)
			if err != nil {
				return nil, fmt.Errorf("insert collaborates_with: %w", err)
			}
			createdRels = append(createdRels, relID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Post-commit: regenerate RELATIONSHIPS.md for ALL affected agents
	// (both new and reused).
	for name, id := range nameToID {
		if err := s.md.GenerateForAgent(ctx, id); err != nil {
			slog.Warn("md generate failed for template agent", "agent_name", name, "agent_id", id, "err", err)
		}
	}

	// Auto-join #all-* channel for NEW agents.
	if allID != "" {
		for _, id := range createdAgents {
			_, _ = s.pool.Exec(ctx,
				`INSERT INTO channel_members (channel_id, member_type, member_id, role)
				 VALUES ($1, 'agent', $2, 'member')
				 ON CONFLICT DO NOTHING`,
				allID, id,
			)
		}
	}

	return &ApplyResult{
		CreatedAgentIDs:        createdAgents,
		CreatedRelationshipIDs: createdRels,
		TemplateID:             templateID,
	}, nil
}

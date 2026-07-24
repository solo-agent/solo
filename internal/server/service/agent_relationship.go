package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RelAssignsTo        = "assigns_to"
	RelCollaboratesWith = "collaborates_with"
)

var (
	ErrRelationshipNotFound = errors.New("relationship not found")
	ErrRelationshipCycle    = errors.New("assigns_to relationship would create a cycle")
)

type AgentRelationshipService struct {
	pool  *pgxpool.Pool
	mdGen *RelationshipsMDGenerator
}

func NewAgentRelationshipService(pool *pgxpool.Pool, mdGen ...*RelationshipsMDGenerator) *AgentRelationshipService {
	s := &AgentRelationshipService{pool: pool}
	if len(mdGen) > 0 {
		s.mdGen = mdGen[0]
	}
	return s
}

type AgentRelationship struct {
	ID          string    `json:"id"`
	FromAgentID string    `json:"from_agent_id"`
	ToAgentID   string    `json:"to_agent_id"`
	RelType     string    `json:"rel_type"`
	Weight      float64   `json:"weight"`
	Instruction string    `json:"instruction"`
	ChannelID   string    `json:"channel_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateRelationshipRequest struct {
	FromAgentID string   `json:"from_agent_id"`
	ToAgentID   string   `json:"to_agent_id"`
	RelType     string   `json:"rel_type"`
	Weight      *float64 `json:"weight,omitempty"`
	Instruction string   `json:"instruction,omitempty"`
}

type UpdateRelationshipRequest struct {
	RelType     *string  `json:"rel_type,omitempty"`
	Weight      *float64 `json:"weight,omitempty"`
	Instruction *string  `json:"instruction,omitempty"`
}

func IsValidRelationshipType(relType string) bool {
	return relType == RelAssignsTo || relType == RelCollaboratesWith
}

func ValidateRelationshipCreate(req CreateRelationshipRequest) error {
	if req.FromAgentID == "" || req.ToAgentID == "" || req.RelType == "" {
		return errors.New("from_agent_id, to_agent_id, and rel_type are required")
	}
	if req.FromAgentID == req.ToAgentID {
		return errors.New("self relationships are not allowed")
	}
	if !IsValidRelationshipType(req.RelType) {
		return fmt.Errorf("invalid rel_type: %s", req.RelType)
	}
	if req.Weight != nil && (*req.Weight < 0 || *req.Weight > 10) {
		return errors.New("weight must be between 0 and 10")
	}
	return nil
}

func ValidateRelationshipUpdate(req UpdateRelationshipRequest) error {
	if req.RelType == nil && req.Weight == nil && req.Instruction == nil {
		return errors.New("rel_type, weight, or instruction is required")
	}
	if req.RelType != nil && !IsValidRelationshipType(*req.RelType) {
		return fmt.Errorf("invalid rel_type: %s", *req.RelType)
	}
	if req.Weight != nil && (*req.Weight < 0 || *req.Weight > 10) {
		return errors.New("weight must be between 0 and 10")
	}
	return nil
}

func (s *AgentRelationshipService) Create(ctx context.Context, userID string, req CreateRelationshipRequest) (*AgentRelationship, error) {
	if err := ValidateRelationshipCreate(req); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	channelID, err := lockRelationshipChannel(ctx, tx, userID, req.FromAgentID, req.ToAgentID)
	if err != nil {
		return nil, err
	}
	if req.RelType == RelAssignsTo {
		hasCycle, err := wouldCreateAssignsToCycle(ctx, tx, channelID, req.FromAgentID, req.ToAgentID, "")
		if err != nil {
			return nil, err
		}
		if hasCycle {
			return nil, ErrRelationshipCycle
		}
	}

	weight := 1.0
	if req.Weight != nil {
		weight = *req.Weight
	}

	var rel AgentRelationship
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_relationships (from_agent_id, to_agent_id, rel_type, weight, instruction)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
		RETURNING id::text, from_agent_id::text, to_agent_id::text, rel_type,
		          weight, instruction, created_at, updated_at
	`, req.FromAgentID, req.ToAgentID, req.RelType, weight, req.Instruction).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.Weight,
		&rel.Instruction, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rel.ChannelID = channelID
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.regenerateRelationshipDocs(ctx, rel.FromAgentID, rel.ToAgentID)
	return &rel, nil
}

func (s *AgentRelationshipService) List(ctx context.Context, userID, agentID string) ([]AgentRelationship, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.from_agent_id::text, r.to_agent_id::text,
		       r.rel_type, r.weight, r.instruction, fa.home_channel_id::text,
		       r.created_at, r.updated_at
		  FROM agent_relationships r
		  JOIN agents fa ON fa.id = r.from_agent_id
		  JOIN agents ta ON ta.id = r.to_agent_id
		 WHERE fa.owner_id = $1::uuid AND ta.owner_id = $1::uuid
		   AND fa.is_active = true AND ta.is_active = true
		   AND fa.home_channel_id = ta.home_channel_id
		   AND ($2 = '' OR r.from_agent_id::text = $2 OR r.to_agent_id::text = $2)
		 ORDER BY r.created_at DESC
	`, userID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rels := []AgentRelationship{}
	for rows.Next() {
		var rel AgentRelationship
		if err := rows.Scan(&rel.ID, &rel.FromAgentID, &rel.ToAgentID,
			&rel.RelType, &rel.Weight, &rel.Instruction, &rel.ChannelID,
			&rel.CreatedAt, &rel.UpdatedAt); err != nil {
			return nil, err
		}
		rels = append(rels, rel)
	}
	return rels, rows.Err()
}

func (s *AgentRelationshipService) Update(ctx context.Context, userID, id string, req UpdateRelationshipRequest) (*AgentRelationship, error) {
	if err := ValidateRelationshipUpdate(req); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var fromAgentID, toAgentID, channelID string
	err = tx.QueryRow(ctx, `
		SELECT r.from_agent_id::text, r.to_agent_id::text, fa.home_channel_id::text
		  FROM agent_relationships r
		  JOIN agents fa ON fa.id = r.from_agent_id
		  JOIN agents ta ON ta.id = r.to_agent_id
		 WHERE r.id = $1
		   AND fa.owner_id = $2::uuid AND ta.owner_id = $2::uuid
		   AND fa.is_active = true AND ta.is_active = true
		   AND fa.home_channel_id = ta.home_channel_id
	`, id, userID).Scan(&fromAgentID, &toAgentID, &channelID)
	if err == pgx.ErrNoRows {
		return nil, ErrRelationshipNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := lockAgentRelationshipChannel(ctx, tx, channelID); err != nil {
		return nil, err
	}
	if req.RelType != nil && *req.RelType == RelAssignsTo {
		hasCycle, err := wouldCreateAssignsToCycle(ctx, tx, channelID, fromAgentID, toAgentID, id)
		if err != nil {
			return nil, err
		}
		if hasCycle {
			return nil, ErrRelationshipCycle
		}
	}

	var rel AgentRelationship
	err = tx.QueryRow(ctx, `
		UPDATE agent_relationships r
		   SET weight = COALESCE($3, r.weight),
		       instruction = COALESCE($4, r.instruction),
		       rel_type = COALESCE($5, r.rel_type),
		       updated_at = now()
		  FROM agents fa, agents ta
		 WHERE r.id = $1
		   AND fa.id = r.from_agent_id AND ta.id = r.to_agent_id
		   AND fa.owner_id = $2::uuid AND ta.owner_id = $2::uuid
		   AND fa.is_active = true AND ta.is_active = true
		   AND fa.home_channel_id = ta.home_channel_id
		RETURNING r.id::text, r.from_agent_id::text, r.to_agent_id::text,
		          r.rel_type, r.weight, r.instruction, fa.home_channel_id::text,
		          r.created_at, r.updated_at
	`, id, userID, req.Weight, req.Instruction, req.RelType).Scan(
		&rel.ID, &rel.FromAgentID, &rel.ToAgentID, &rel.RelType, &rel.Weight,
		&rel.Instruction, &rel.ChannelID, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrRelationshipNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.regenerateRelationshipDocs(ctx, rel.FromAgentID, rel.ToAgentID)
	return &rel, nil
}

func lockRelationshipChannel(ctx context.Context, tx pgx.Tx, userID, fromAgentID, toAgentID string) (string, error) {
	var channelID string
	err := tx.QueryRow(ctx, `
		SELECT fa.home_channel_id::text
		  FROM agents fa
		  JOIN agents ta ON ta.id = $2::uuid
		 WHERE fa.id = $1::uuid
		   AND fa.owner_id = $3::uuid
		   AND ta.owner_id = $3::uuid
		   AND fa.is_active = true
		   AND ta.is_active = true
		   AND fa.home_channel_id = ta.home_channel_id
	`, fromAgentID, toAgentID, userID).Scan(&channelID)
	if err == pgx.ErrNoRows {
		return "", ErrRelationshipNotFound
	}
	if err != nil {
		return "", err
	}
	if err := lockAgentRelationshipChannel(ctx, tx, channelID); err != nil {
		return "", err
	}
	return channelID, nil
}

func lockAgentRelationshipChannel(ctx context.Context, tx pgx.Tx, channelID string) error {
	_, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtext('solo-agent-relationships:' || $1))
	`, channelID)
	return err
}

func wouldCreateAssignsToCycle(
	ctx context.Context,
	tx pgx.Tx,
	channelID, fromAgentID, toAgentID, excludeRelationshipID string,
) (bool, error) {
	var hasCycle bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE reachable(agent_id) AS (
			SELECT $2::uuid
			UNION
			SELECT r.to_agent_id
			  FROM agent_relationships r
			  JOIN reachable current ON current.agent_id = r.from_agent_id
			  JOIN agents source ON source.id = r.from_agent_id
			 WHERE r.rel_type = 'assigns_to'
			   AND source.home_channel_id = $1::uuid
			   AND ($4 = '' OR r.id::text <> $4)
		)
		SELECT EXISTS(
			SELECT 1 FROM reachable WHERE agent_id = $3::uuid
		)
	`, channelID, toAgentID, fromAgentID, excludeRelationshipID).Scan(&hasCycle)
	return hasCycle, err
}

func (s *AgentRelationshipService) Delete(ctx context.Context, userID, id string) error {
	var fromID, toID string
	_ = s.pool.QueryRow(ctx, `
		SELECT r.from_agent_id::text, r.to_agent_id::text
		  FROM agent_relationships r
		  JOIN agents fa ON fa.id = r.from_agent_id
		  JOIN agents ta ON ta.id = r.to_agent_id
		 WHERE r.id = $1
		   AND fa.owner_id = $2::uuid AND ta.owner_id = $2::uuid
		   AND fa.is_active = true AND ta.is_active = true
	`, id, userID).Scan(&fromID, &toID)

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM agent_relationships r
		USING agents fa, agents ta
		WHERE r.id = $1
		  AND fa.id = r.from_agent_id AND ta.id = r.to_agent_id
		  AND fa.owner_id = $2::uuid AND ta.owner_id = $2::uuid
		  AND fa.is_active = true AND ta.is_active = true
	`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRelationshipNotFound
	}
	s.regenerateRelationshipDocs(ctx, fromID, toID)
	return nil
}

func (s *AgentRelationshipService) regenerateRelationshipDocs(ctx context.Context, agentIDs ...string) {
	if s.mdGen == nil {
		return
	}
	for _, id := range agentIDs {
		if id != "" {
			_ = s.mdGen.GenerateForAgent(ctx, id)
		}
	}
}

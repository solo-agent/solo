package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	minThinkingSynthesisSources = 2
	maxThinkingSynthesisSources = 8
	maxSynthesisConstraintItems = 24
	maxSynthesisConstraintRunes = 500
)

var (
	ErrThinkingSynthesisNotFound     = errors.New("thinking synthesis not found")
	ErrThinkingSynthesisInvalid      = errors.New("invalid thinking synthesis")
	ErrThinkingSynthesisSourceAbsent = errors.New("selected node has no published Current State or final Handoff")
)

type ThinkingSynthesisConstraints struct {
	MustPreserve    []string `json:"must_preserve"`
	MustExclude     []string `json:"must_exclude"`
	HardConstraints []string `json:"hard_constraints"`
	Preferences     []string `json:"preferences"`
	OpenQuestions   []string `json:"open_questions"`
}

type ThinkingSynthesisPathNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Depth int    `json:"depth"`
}

type ThinkingSynthesisSource struct {
	NodeID                   string                      `json:"node_id"`
	NodeTitle                string                      `json:"node_title"`
	HandoffKind              string                      `json:"handoff_kind"`
	HandoffSnapshot          string                      `json:"handoff_snapshot"`
	HandoffAt                time.Time                   `json:"handoff_at"`
	CheckpointStatusSnapshot string                      `json:"checkpoint_status_snapshot"`
	PathSnapshot             []ThinkingSynthesisPathNode `json:"path_snapshot"`
	UserNote                 string                      `json:"user_note,omitempty"`
	SortOrder                int                         `json:"sort_order"`
}

type ThinkingSynthesis struct {
	ID                 string                       `json:"id"`
	SpaceID            string                       `json:"space_id"`
	ChannelID          string                       `json:"channel_id"`
	CreatedBy          string                       `json:"created_by"`
	Title              string                       `json:"title"`
	Objective          string                       `json:"objective"`
	Constraints        ThinkingSynthesisConstraints `json:"constraints"`
	Mode               string                       `json:"mode"`
	CoordinatorAgentID string                       `json:"coordinator_agent_id,omitempty"`
	CoordinatorName    string                       `json:"coordinator_name,omitempty"`
	Status             string                       `json:"status"`
	ResultArtifactID   string                       `json:"result_artifact_id,omitempty"`
	ResultNodeID       string                       `json:"result_node_id,omitempty"`
	Error              string                       `json:"error,omitempty"`
	HasStaleSources    bool                         `json:"has_stale_sources"`
	Sources            []ThinkingSynthesisSource    `json:"sources"`
	CreatedAt          time.Time                    `json:"created_at"`
	UpdatedAt          time.Time                    `json:"updated_at"`
}

type CreateThinkingSynthesisInput struct {
	Title              string
	Objective          string
	NodeIDs            []string
	Constraints        ThinkingSynthesisConstraints
	Mode               string
	CoordinatorAgentID string
}

type ThinkingSynthesisService struct {
	pool *pgxpool.Pool
}

func NewThinkingSynthesisService(pool *pgxpool.Pool) *ThinkingSynthesisService {
	return &ThinkingSynthesisService{pool: pool}
}

func (s *ThinkingSynthesisService) Create(ctx context.Context, channelID, actorID string, input CreateThinkingSynthesisInput) (*ThinkingSynthesis, error) {
	if err := normalizeThinkingSynthesisInput(&input); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var spaceID string
	if err := tx.QueryRow(ctx, `
		SELECT space.id::text
		  FROM thinking_spaces space
		  JOIN channels channel ON channel.id = space.channel_id
		 WHERE space.channel_id = $1 AND channel.is_archived = false`, channelID).Scan(&spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThinkingNotFound
		}
		return nil, err
	}

	if input.CoordinatorAgentID != "" {
		var coordinatorAvailable bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				  FROM channel_members member
				  JOIN agents agent ON agent.id = member.member_id AND agent.is_active = true
				 WHERE member.channel_id = $1 AND member.member_type = 'agent'
				   AND member.member_id = $2
			)`, channelID, input.CoordinatorAgentID).Scan(&coordinatorAvailable); err != nil {
			return nil, err
		}
		if !coordinatorAvailable {
			return nil, fmt.Errorf("%w: coordinator Agent is not active in this channel", ErrThinkingSynthesisInvalid)
		}
	}

	constraintsJSON, err := json.Marshal(input.Constraints)
	if err != nil {
		return nil, err
	}
	synthesisID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO thinking_syntheses
		    (id, space_id, created_by, title, objective, constraints, mode, coordinator_agent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		synthesisID, spaceID, actorID, input.Title, input.Objective, constraintsJSON, input.Mode, nullableUUID(input.CoordinatorAgentID)); err != nil {
		return nil, err
	}

	for index, nodeID := range input.NodeIDs {
		source, err := loadThinkingSynthesisSource(ctx, tx, spaceID, nodeID, index)
		if err != nil {
			return nil, err
		}
		pathJSON, err := json.Marshal(source.PathSnapshot)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO thinking_synthesis_sources
			    (synthesis_id, node_id, node_title_snapshot, handoff_kind, handoff_snapshot, handoff_at,
			     checkpoint_status_snapshot, path_snapshot, user_note, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			synthesisID, source.NodeID, source.NodeTitle, source.HandoffKind, source.HandoffSnapshot, source.HandoffAt,
			source.CheckpointStatusSnapshot, pathJSON, source.UserNote, source.SortOrder); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, channelID, synthesisID)
}

func (s *ThinkingSynthesisService) Get(ctx context.Context, channelID, synthesisID string) (*ThinkingSynthesis, error) {
	var synthesis ThinkingSynthesis
	var constraintsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT synthesis.id::text, synthesis.space_id::text, space.channel_id::text,
		       synthesis.created_by::text, synthesis.title, synthesis.objective,
		       synthesis.constraints, synthesis.mode,
		       COALESCE(synthesis.coordinator_agent_id::text, ''), COALESCE(agent.name, ''),
		       synthesis.status, COALESCE(synthesis.result_artifact_id::text, ''),
		       COALESCE(synthesis.result_node_id::text, ''), synthesis.error,
		       synthesis.created_at, synthesis.updated_at
		  FROM thinking_syntheses synthesis
		  JOIN thinking_spaces space ON space.id = synthesis.space_id AND space.channel_id = $1
		  LEFT JOIN agents agent ON agent.id = synthesis.coordinator_agent_id
		 WHERE synthesis.id = $2`, channelID, synthesisID).Scan(
		&synthesis.ID, &synthesis.SpaceID, &synthesis.ChannelID, &synthesis.CreatedBy,
		&synthesis.Title, &synthesis.Objective, &constraintsJSON, &synthesis.Mode,
		&synthesis.CoordinatorAgentID, &synthesis.CoordinatorName, &synthesis.Status,
		&synthesis.ResultArtifactID, &synthesis.ResultNodeID, &synthesis.Error,
		&synthesis.CreatedAt, &synthesis.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThinkingSynthesisNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(constraintsJSON, &synthesis.Constraints); err != nil {
		return nil, err
	}
	normalizeSynthesisConstraints(&synthesis.Constraints)

	rows, err := s.pool.Query(ctx, `
		SELECT source.node_id::text, source.node_title_snapshot, source.handoff_kind,
		       source.handoff_snapshot, source.handoff_at,
		       source.checkpoint_status_snapshot, source.path_snapshot,
		       source.user_note, source.sort_order
		  FROM thinking_synthesis_sources source
		 WHERE source.synthesis_id = $1
		 ORDER BY source.sort_order`, synthesis.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	synthesis.Sources = []ThinkingSynthesisSource{}
	for rows.Next() {
		var source ThinkingSynthesisSource
		var pathJSON []byte
		if err := rows.Scan(
			&source.NodeID, &source.NodeTitle, &source.HandoffKind,
			&source.HandoffSnapshot, &source.HandoffAt,
			&source.CheckpointStatusSnapshot, &pathJSON,
			&source.UserNote, &source.SortOrder,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(pathJSON, &source.PathSnapshot); err != nil {
			return nil, err
		}
		if source.CheckpointStatusSnapshot == "stale" {
			synthesis.HasStaleSources = true
		}
		synthesis.Sources = append(synthesis.Sources, source)
	}
	return &synthesis, rows.Err()
}

func (s *ThinkingSynthesisService) List(ctx context.Context, channelID string) ([]ThinkingSynthesis, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT synthesis.id::text
		  FROM thinking_syntheses synthesis
		  JOIN thinking_spaces space ON space.id = synthesis.space_id
		 WHERE space.channel_id = $1
		 ORDER BY synthesis.created_at DESC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	result := make([]ThinkingSynthesis, 0, len(ids))
	for _, id := range ids {
		synthesis, err := s.Get(ctx, channelID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, *synthesis)
	}
	return result, nil
}

func loadThinkingSynthesisSource(ctx context.Context, tx pgx.Tx, spaceID, nodeID string, sortOrder int) (*ThinkingSynthesisSource, error) {
	var source ThinkingSynthesisSource
	var checkpointHandoff, returnedHandoff string
	var checkpointAt, returnedAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT node.id::text, node.title, node.checkpoint_handoff,
		       node.checkpoint_handoff_at, node.returned_handoff, node.returned_at,
		       CASE
		         WHEN node.returned_at IS NOT NULL THEN 'final'
		         WHEN node.checkpoint_handoff = '' OR node.checkpoint_handoff_at IS NULL THEN 'missing'
		         WHEN EXISTS (
		           SELECT 1 FROM messages latest
		            WHERE latest.thinking_node_id = node.id
		              AND latest.sender_type = 'agent'
		              AND COALESCE(latest.is_deleted, false) = false
		              AND latest.created_at > node.checkpoint_handoff_at
		         ) THEN 'stale'
		         ELSE 'fresh'
		       END
		  FROM thinking_nodes node
		 WHERE node.id = $1 AND node.space_id = $2
		 FOR SHARE OF node`, nodeID, spaceID).Scan(
		&source.NodeID, &source.NodeTitle, &checkpointHandoff,
		&checkpointAt, &returnedHandoff, &returnedAt,
		&source.CheckpointStatusSnapshot,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: node %s is not in this Thinking space", ErrThinkingSynthesisInvalid, nodeID)
	}
	if err != nil {
		return nil, err
	}
	switch {
	case returnedAt != nil:
		if strings.TrimSpace(returnedHandoff) == "" {
			return nil, fmt.Errorf("%w: node %s has a final timestamp without a Handoff", ErrThinkingSynthesisInvalid, nodeID)
		}
		source.HandoffKind = "returned"
		source.HandoffSnapshot = returnedHandoff
		source.HandoffAt = *returnedAt
		source.CheckpointStatusSnapshot = "final"
	case checkpointAt != nil && strings.TrimSpace(checkpointHandoff) != "":
		source.HandoffKind = "checkpoint"
		source.HandoffSnapshot = checkpointHandoff
		source.HandoffAt = *checkpointAt
	case source.CheckpointStatusSnapshot == "missing":
		return nil, fmt.Errorf("%w: node %s", ErrThinkingSynthesisSourceAbsent, nodeID)
	default:
		return nil, fmt.Errorf("%w: node %s has inconsistent Handoff state", ErrThinkingSynthesisInvalid, nodeID)
	}
	source.SortOrder = sortOrder

	rows, err := tx.Query(ctx, `
		WITH RECURSIVE path AS (
			SELECT node.id, node.parent_id, node.title, node.depth
			  FROM thinking_nodes node WHERE node.id = $1
			UNION ALL
			SELECT parent.id, parent.parent_id, parent.title, parent.depth
			  FROM thinking_nodes parent
			  JOIN path child ON child.parent_id = parent.id
		)
		SELECT id::text, title, depth FROM path ORDER BY depth`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	source.PathSnapshot = []ThinkingSynthesisPathNode{}
	for rows.Next() {
		var item ThinkingSynthesisPathNode
		if err := rows.Scan(&item.ID, &item.Title, &item.Depth); err != nil {
			return nil, err
		}
		source.PathSnapshot = append(source.PathSnapshot, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &source, nil
}

func normalizeThinkingSynthesisInput(input *CreateThinkingSynthesisInput) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Objective = strings.TrimSpace(input.Objective)
	input.CoordinatorAgentID = strings.TrimSpace(input.CoordinatorAgentID)
	input.Mode = strings.TrimSpace(input.Mode)
	if input.Mode == "" {
		input.Mode = "single_agent"
	}
	if input.Mode != "single_agent" && input.Mode != "review_team" {
		return fmt.Errorf("%w: mode must be single_agent or review_team", ErrThinkingSynthesisInvalid)
	}
	if input.Objective == "" || utf8.RuneCountInString(input.Objective) > 4000 {
		return fmt.Errorf("%w: objective must be between 1 and 4000 characters", ErrThinkingSynthesisInvalid)
	}
	if input.Title == "" {
		input.Title = truncateSynthesisRunes(input.Objective, 80)
	}
	if utf8.RuneCountInString(input.Title) > 150 {
		return fmt.Errorf("%w: title exceeds 150 characters", ErrThinkingSynthesisInvalid)
	}
	if len(input.NodeIDs) < minThinkingSynthesisSources || len(input.NodeIDs) > maxThinkingSynthesisSources {
		return fmt.Errorf("%w: select between %d and %d nodes", ErrThinkingSynthesisInvalid, minThinkingSynthesisSources, maxThinkingSynthesisSources)
	}
	seen := make(map[string]bool, len(input.NodeIDs))
	for index, nodeID := range input.NodeIDs {
		nodeID = strings.TrimSpace(nodeID)
		if _, err := uuid.Parse(nodeID); err != nil {
			return fmt.Errorf("%w: invalid node ID", ErrThinkingSynthesisInvalid)
		}
		if seen[nodeID] {
			return fmt.Errorf("%w: duplicate node ID", ErrThinkingSynthesisInvalid)
		}
		seen[nodeID] = true
		input.NodeIDs[index] = nodeID
	}
	normalizeSynthesisConstraints(&input.Constraints)
	for name, values := range map[string][]string{
		"must_preserve":    input.Constraints.MustPreserve,
		"must_exclude":     input.Constraints.MustExclude,
		"hard_constraints": input.Constraints.HardConstraints,
		"preferences":      input.Constraints.Preferences,
		"open_questions":   input.Constraints.OpenQuestions,
	} {
		if len(values) > maxSynthesisConstraintItems {
			return fmt.Errorf("%w: %s exceeds %d items", ErrThinkingSynthesisInvalid, name, maxSynthesisConstraintItems)
		}
		for _, value := range values {
			if utf8.RuneCountInString(value) > maxSynthesisConstraintRunes {
				return fmt.Errorf("%w: %s item exceeds %d characters", ErrThinkingSynthesisInvalid, name, maxSynthesisConstraintRunes)
			}
		}
	}
	return nil
}

func normalizeSynthesisConstraints(constraints *ThinkingSynthesisConstraints) {
	constraints.MustPreserve = normalizeConstraintItems(constraints.MustPreserve)
	constraints.MustExclude = normalizeConstraintItems(constraints.MustExclude)
	constraints.HardConstraints = normalizeConstraintItems(constraints.HardConstraints)
	constraints.Preferences = normalizeConstraintItems(constraints.Preferences)
	constraints.OpenQuestions = normalizeConstraintItems(constraints.OpenQuestions)
}

func normalizeConstraintItems(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func truncateSynthesisRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

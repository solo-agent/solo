package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MentionValidator checks @mention compatibility with assigns_to relationships.
type MentionValidator struct {
	pool *pgxpool.Pool
}

func NewMentionValidator(pool *pgxpool.Pool) *MentionValidator {
	return &MentionValidator{pool: pool}
}

// ValidationResult indicates whether a set of @mentions is allowed.
type ValidationResult struct {
	AllAllowed bool
	Violations []MentionViolation
}

// MentionViolation is a single @mention that has no assigns_to edge from mentioner to mentionee.
type MentionViolation struct {
	MentionerID string
	MentioneeID string
}

// Validate returns whether all (mentioner → mentionee) pairs have an active
// assigns_to edge. Self-mentions are ignored.
func (s *MentionValidator) Validate(ctx context.Context, mentionerID string, mentionedIDs []string) (*ValidationResult, error) {
	if mentionerID == "" {
		return nil, fmt.Errorf("mentionerID is required")
	}

	result := &ValidationResult{AllAllowed: true}
	for _, mentioneeID := range mentionedIDs {
		if mentioneeID == mentionerID {
			continue
		}
		var exists bool
		err := s.pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM agent_relationships
				WHERE from_agent_id = $1
				  AND to_agent_id = $2
				  AND rel_type = 'assigns_to'
			)
		`, mentionerID, mentioneeID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("check assigns_to: %w", err)
		}
		if !exists {
			result.AllAllowed = false
			result.Violations = append(result.Violations, MentionViolation{
				MentionerID: mentionerID,
				MentioneeID: mentioneeID,
			})
		}
	}
	return result, nil
}

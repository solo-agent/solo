package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pattern for matching @mentions in message content.
// Supports alphanumeric names with underscores, hyphens, dots, and Unicode characters.
var mentionPattern = regexp.MustCompile(`@([\p{L}\p{N}_\-\.]+)`)

// MentionService handles @mention parsing and resolution.
type MentionService struct {
	pool *pgxpool.Pool
}

// NewMentionService creates a new MentionService.
func NewMentionService(pool *pgxpool.Pool) *MentionService {
	return &MentionService{pool: pool}
}

// ResolveMentions parses @mention references in content and resolves them
// to agent member IDs in the given channel.
//
// Returns:
//   - mentionedIDs: deduplicated list of agent IDs that were successfully resolved
//   - hasMentions: true if the content contained any @mention patterns
//   - error: any database error encountered
//
// Only resolves Agent mentions (not user mentions). Only returns IDs of
// agents that are active members of the channel.
func (s *MentionService) ResolveMentions(ctx context.Context, content, channelID string) (mentionedIDs []string, hasMentions bool, err error) {
	// Parse @names from content
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, false, nil
	}

	hasMentions = true

	// Deduplicate mention names
	nameSet := make(map[string]bool, len(matches))
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name != "" {
			nameSet[name] = true
		}
	}

	if len(nameSet) == 0 {
		return []string{}, true, nil
	}

	// Build query arguments
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}

	// Resolve @names to agent IDs by joining agents with channel_members
	query := `SELECT a.id, a.name
	           FROM agents a
	           JOIN channel_members cm ON cm.member_id = a.id AND cm.member_type = 'agent'
	           WHERE cm.channel_id = $1
	             AND a.is_active = true
	             AND a.name = ANY($2)`

	rows, err := s.pool.Query(ctx, query, channelID, names)
	if err != nil {
		return nil, true, err
	}
	defer rows.Close()

	var agentIDs []string
	seen := make(map[string]bool)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, true, err
		}
		if !seen[id] {
			agentIDs = append(agentIDs, id)
			seen[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, true, err
	}

	if agentIDs == nil {
		return []string{}, true, nil
	}

	return agentIDs, true, nil
}

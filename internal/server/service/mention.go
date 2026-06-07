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

// ResolveUserMentions parses @mention patterns in content, resolves them to
// user IDs by display_name, and records them in user_mentions.
// Returns the list of mentioned user IDs for WS broadcast.
func (s *MentionService) ResolveUserMentions(ctx context.Context, content, messageID string) (mentionedUserIDs []string, err error) {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	nameSet := make(map[string]bool, len(matches))
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name != "" {
			nameSet[name] = true
		}
	}

	if len(nameSet) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}

	// Resolve display_names to user IDs
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM users WHERE display_name = ANY($1)`, names,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(userIDs) == 0 {
		return nil, nil
	}

	// Batch insert into user_mentions
	for _, uid := range userIDs {
		_, err := s.pool.Exec(ctx,
			`INSERT INTO user_mentions (message_id, mentioned_user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			messageID, uid,
		)
		if err != nil {
			return nil, err
		}
	}

	return userIDs, nil
}

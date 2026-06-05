package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InboxService handles inbox aggregation queries.
type InboxService struct {
	pool *pgxpool.Pool
}

// NewInboxService creates a new InboxService.
func NewInboxService(pool *pgxpool.Pool) *InboxService {
	return &InboxService{pool: pool}
}

// InboxItem represents a single inbox entry.
type InboxItem struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"` // thread_reply, dm, mention
	ChannelID      *string   `json:"channel_id"`
	ChannelName    *string   `json:"channel_name"`
	ThreadID       *string   `json:"thread_id"`
	DMID           *string   `json:"dm_id"`
	MessageID      string    `json:"message_id"`
	SenderName     string    `json:"sender_name"`
	SenderAvatar   *string   `json:"sender_avatar"`
	ContentPreview string    `json:"content_preview"`
	IsMention      bool      `json:"is_mention"`
	IsUnread       bool      `json:"is_unread"`
	CreatedAt      time.Time `json:"created_at"`
}

// UnreadCount holds per-category unread counts.
type UnreadCount struct {
	Total         int `json:"total"`
	Mentions      int `json:"mentions"`
	ThreadReplies int `json:"thread_replies"`
	DM            int `json:"dm"`
}

// List returns inbox items for a user with cursor-based pagination.
// Items are ordered by created_at DESC. The 'before' parameter is an ISO 8601
// timestamp used as a cursor; limit caps the returned item count (max 50).
func (s *InboxService) List(ctx context.Context, userID string, before time.Time, limit int) ([]InboxItem, bool, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}

	// Get user's last_read_at for is_unread calculation.
	var lastReadAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(last_read_at, now() - INTERVAL '1 hour')
		 FROM user_inbox_state WHERE user_id = $1`, userID,
	).Scan(&lastReadAt)
	if err != nil {
		// No state row yet — treat everything as unread.
		lastReadAt = time.Time{}
	}

	// Get user's display_name for @mention ILIKE matching.
	var displayName string
	err = s.pool.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName)
	if err != nil {
		displayName = ""
	}

	// Query params expansion: userID, before, lastReadAt, displayName, limit+1.
	// Use UNION ALL across three sources.
	query := `
		SELECT id, item_type, channel_id, channel_name, thread_id, dm_id,
		       sender_name, sender_avatar, content_preview, is_mention, created_at
		FROM (
			-- Thread replies: messages in threads I created or replied to, from others.
			SELECT m.id,
			       'thread_reply' AS item_type,
			       c.id::text AS channel_id,
			       c.name AS channel_name,
			       t.id::text AS thread_id,
			       NULL::text AS dm_id,
			       COALESCE(u.display_name, a.name, 'Unknown') AS sender_name,
			       NULL::text AS sender_avatar,
			       LEFT(m.content, 50) AS content_preview,
			       false AS is_mention,
			       m.created_at
			FROM messages m
			JOIN threads t ON m.thread_id = t.id
			JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			WHERE m.sender_id != $1
			  AND m.thread_id IS NOT NULL
			  AND t.channel_id IN (
			      SELECT DISTINCT m2.channel_id
			      FROM messages m2
			      JOIN threads t2 ON m2.thread_id = t2.id
			      WHERE m2.sender_id = $1
			  )
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false

			UNION ALL

			-- DM messages: messages in DMs I'm a member of, from others.
			SELECT m.id,
			       'dm' AS item_type,
			       NULL::text AS channel_id,
			       NULL::text AS channel_name,
			       NULL::text AS thread_id,
			       c.id::text AS dm_id,
			       COALESCE(u.display_name, a.name, 'Unknown') AS sender_name,
			       NULL::text AS sender_avatar,
			       LEFT(m.content, 50) AS content_preview,
			       false AS is_mention,
			       m.created_at
			FROM messages m
			JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
			JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			WHERE m.sender_id != $1
			  AND m.thread_id IS NULL
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false

			UNION ALL

			-- @Mentions: messages that @mention the user by display_name.
			-- NOTE: @mention detection uses ILIKE against the user's display_name.
			-- The messages table has mentioned_agent_ids (uuid[]) for agent mentions
			-- but no mentioned_user_ids. For user mentions we fall back to pattern
			-- matching against message content. This is less precise than a
			-- dedicated JSONB column but avoids a new migration for v1.5.
			SELECT m.id,
			       'mention' AS item_type,
			       c.id::text AS channel_id,
			       c.name AS channel_name,
			       m.thread_id::text AS thread_id,
			       NULL::text AS dm_id,
			       COALESCE(u.display_name, a.name, 'Unknown') AS sender_name,
			       NULL::text AS sender_avatar,
			       LEFT(m.content, 50) AS content_preview,
			       true AS is_mention,
			       m.created_at
			FROM messages m
			JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			WHERE m.sender_id != $1
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false
			  AND $3 != ''
			  AND (m.content ILIKE '%@' || $3 || '%'
			       OR m.content ILIKE '%@' || $3 || ' %'
			       OR m.content ILIKE '%' || $3 || '%')
		) sub
		ORDER BY created_at DESC
		LIMIT $4
	`

	args := []any{userID, before, displayName, limit + 1}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("inbox list: %w", err)
	}
	defer rows.Close()

	items := make([]InboxItem, 0, limit)
	for rows.Next() {
		var item InboxItem
		if err := rows.Scan(&item.ID, &item.Type, &item.ChannelID, &item.ChannelName,
			&item.ThreadID, &item.DMID, &item.SenderName, &item.SenderAvatar,
			&item.ContentPreview, &item.IsMention, &item.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan inbox item: %w", err)
		}
		item.IsUnread = item.CreatedAt.After(lastReadAt)
		item.MessageID = item.ID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate inbox items: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	if items == nil {
		items = []InboxItem{}
	}
	return items, hasMore, nil
}

// UnreadCount returns per-category unread counts for a user.
func (s *InboxService) UnreadCount(ctx context.Context, userID string) (*UnreadCount, error) {
	// Get user's last_read_at
	var lastReadAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(last_read_at, now() - INTERVAL '1 hour')
		 FROM user_inbox_state WHERE user_id = $1`, userID,
	).Scan(&lastReadAt)
	if err != nil {
		lastReadAt = time.Time{}
	}

	var displayName string
	err = s.pool.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName)
	if err != nil {
		displayName = ""
	}

	result := &UnreadCount{}

	// Thread reply unreads
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN threads t ON m.thread_id = t.id
		 JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
		 WHERE m.sender_id != $1
		   AND m.thread_id IS NOT NULL
		   AND t.channel_id IN (
		       SELECT DISTINCT m2.channel_id
		       FROM messages m2
		       JOIN threads t2 ON m2.thread_id = t2.id
		       WHERE m2.sender_id = $1
		   )
		   AND m.created_at > $2
		   AND COALESCE(m.is_deleted, false) = false`,
		userID, lastReadAt,
	).Scan(&result.ThreadReplies)
	if err != nil {
		result.ThreadReplies = 0
	}

	// DM unreads
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
		 JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
		 WHERE m.sender_id != $1
		   AND m.thread_id IS NULL
		   AND m.created_at > $2
		   AND COALESCE(m.is_deleted, false) = false`,
		userID, lastReadAt,
	).Scan(&result.DM)
	if err != nil {
		result.DM = 0
	}

	// @Mention unreads
	if displayName != "" {
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*)
			 FROM messages m
			 JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
			 WHERE m.sender_id != $1
			   AND m.created_at > $2
			   AND COALESCE(m.is_deleted, false) = false
			   AND (m.content ILIKE '%@' || $3 || '%'
			        OR m.content ILIKE '%@' || $3 || ' %'
			        OR m.content ILIKE '%' || $3 || '%')`,
			userID, lastReadAt, displayName,
		).Scan(&result.Mentions)
		if err != nil {
			result.Mentions = 0
		}
	}

	result.Total = result.ThreadReplies + result.DM + result.Mentions
	return result, nil
}

// MarkRead updates the user's last_read_at timestamp to now.
// Uses UPSERT to create the row if it doesn't exist.
func (s *InboxService) MarkRead(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_inbox_state (user_id, last_read_at, updated_at)
		 VALUES ($1, now(), now())
		 ON CONFLICT (user_id) DO UPDATE SET last_read_at = now(), updated_at = now()`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("mark inbox read: %w", err)
	}
	return nil
}

// GetLastReadAt returns the user's last_read_at timestamp (or zero time if no row exists).
func (s *InboxService) GetLastReadAt(ctx context.Context, userID string) (time.Time, error) {
	var lastReadAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT last_read_at FROM user_inbox_state WHERE user_id = $1`, userID,
	).Scan(&lastReadAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("get last read at: %w", err)
	}
	return lastReadAt, nil
}

// truncateStr truncates a string to maxLen runes with "..." suffix.
// Pre-allocates for the empty string case.
func truncateStr(s string, maxLen int) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	var b strings.Builder
	b.Grow(maxLen + 3)
	b.WriteString(string(runes[:maxLen]))
	b.WriteString("...")
	return b.String()
}

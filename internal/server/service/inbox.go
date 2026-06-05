package service

import (
	"context"
	"fmt"
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
	CreatedAt      time.Time `json:"created_at"`
}

// UnreadCount holds per-category inbox item counts.
type UnreadCount struct {
	Total         int `json:"total"`
	Mentions      int `json:"mentions"`
	ThreadReplies int `json:"thread_replies"`
	DM            int `json:"dm"`
}

// List returns inbox items for a user with cursor-based pagination.
// Excludes messages the user has dismissed.
func (s *InboxService) List(ctx context.Context, userID string, before time.Time, limit int) ([]InboxItem, bool, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}

	var displayName string
	err := s.pool.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName)
	if err != nil {
		displayName = ""
	}

	query := `
		SELECT id, item_type, channel_id, channel_name, thread_id, dm_id,
		       sender_name, sender_avatar, content_preview, is_mention, created_at
		FROM (
			-- Thread replies
			SELECT m.id,
			       'thread_reply' AS item_type,
			       c.id::text AS channel_id,
			       c.name AS channel_name,
			       t.root_message_id::text AS thread_id,
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
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NOT NULL
			  AND t.channel_id IN (
			      SELECT DISTINCT m2.channel_id
			      FROM messages m2
			      JOIN threads t2 ON m2.thread_id = t2.id
			      WHERE m2.sender_id = $1
			  )
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false
			  AND d.message_id IS NULL

			UNION ALL

			-- DM messages
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
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NULL
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false
			  AND d.message_id IS NULL

			UNION ALL

			-- @Mentions
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
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.created_at < $2
			  AND COALESCE(m.is_deleted, false) = false
			  AND $3 != ''
			  AND d.message_id IS NULL
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

// UnreadCount returns per-category inbox item counts (excluding dismissed).
func (s *InboxService) UnreadCount(ctx context.Context, userID string) (*UnreadCount, error) {
	var displayName string
	err := s.pool.QueryRow(ctx,
		`SELECT display_name FROM users WHERE id = $1`, userID,
	).Scan(&displayName)
	if err != nil {
		displayName = ""
	}

	result := &UnreadCount{}

	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN threads t ON m.thread_id = t.id
		 JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
		 LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
		 WHERE m.sender_id != $1
		   AND m.sender_type IN ('user', 'agent')
		   AND m.thread_id IS NOT NULL
		   AND t.channel_id IN (
		       SELECT DISTINCT m2.channel_id
		       FROM messages m2
		       JOIN threads t2 ON m2.thread_id = t2.id
		       WHERE m2.sender_id = $1
		   )
		   AND COALESCE(m.is_deleted, false) = false
		   AND d.message_id IS NULL`,
		userID,
	).Scan(&result.ThreadReplies)
	if err != nil {
		result.ThreadReplies = 0
	}

	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
		 JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
		 LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
		 WHERE m.sender_id != $1
		   AND m.sender_type IN ('user', 'agent')
		   AND m.thread_id IS NULL
		   AND COALESCE(m.is_deleted, false) = false
		   AND d.message_id IS NULL`,
		userID,
	).Scan(&result.DM)
	if err != nil {
		result.DM = 0
	}

	if displayName != "" {
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*)
			 FROM messages m
			 JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
			 LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			 WHERE m.sender_id != $1
			   AND m.sender_type IN ('user', 'agent')
			   AND COALESCE(m.is_deleted, false) = false
			   AND d.message_id IS NULL
			   AND (m.content ILIKE '%@' || $2 || '%'
			        OR m.content ILIKE '%@' || $2 || ' %'
			        OR m.content ILIKE '%' || $2 || '%')`,
			userID, displayName,
		).Scan(&result.Mentions)
		if err != nil {
			result.Mentions = 0
		}
	}

	result.Total = result.ThreadReplies + result.DM + result.Mentions
	return result, nil
}

// Dismiss removes a single message from the user's inbox.
func (s *InboxService) Dismiss(ctx context.Context, userID, messageID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_inbox_dismissals (user_id, message_id)
		 VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		userID, messageID,
	)
	if err != nil {
		return fmt.Errorf("dismiss inbox item: %w", err)
	}
	return nil
}

// DismissAll removes all currently visible inbox messages for a user.
func (s *InboxService) DismissAll(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_inbox_dismissals (user_id, message_id)
		 SELECT $1, sub.id FROM (
			SELECT m.id
			FROM messages m
			JOIN threads t ON m.thread_id = t.id
			JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NOT NULL
			  AND t.channel_id IN (
			      SELECT DISTINCT m2.channel_id FROM messages m2
			      JOIN threads t2 ON m2.thread_id = t2.id
			      WHERE m2.sender_id = $1
			  )
			  AND COALESCE(m.is_deleted, false) = false
			  AND d.message_id IS NULL

			UNION

			SELECT m.id
			FROM messages m
			JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
			JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NULL
			  AND COALESCE(m.is_deleted, false) = false
			  AND d.message_id IS NULL

			UNION

			SELECT m.id
			FROM messages m
			JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_dismissals d ON d.user_id = $1 AND d.message_id = m.id
			CROSS JOIN LATERAL (SELECT display_name FROM users WHERE id = $1) un
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent')
			  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND COALESCE(m.is_deleted, false) = false
			  AND un.display_name != ''
			  AND d.message_id IS NULL
			  AND (m.content ILIKE '%@' || un.display_name || '%'
			       OR m.content ILIKE '%@' || un.display_name || ' %'
			       OR m.content ILIKE '%' || un.display_name || '%')
		 ) sub
		 ON CONFLICT DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("dismiss all inbox: %w", err)
	}
	return nil
}

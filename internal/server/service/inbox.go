package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	serverworkspace "github.com/solo-ai/solo/internal/server/workspace"
)

type InboxService struct {
	pool *pgxpool.Pool
}

func NewInboxService(pool *pgxpool.Pool) *InboxService {
	return &InboxService{pool: pool}
}

type InboxItem struct {
	ID               string    `json:"id"`
	Type             string    `json:"type"`
	ChannelID        *string   `json:"channel_id"`
	ChannelName      *string   `json:"channel_name"`
	ThreadID         *string   `json:"thread_id"`
	DMID             *string   `json:"dm_id"`
	MessageID        string    `json:"message_id"`
	SenderName       string    `json:"sender_name"`
	SenderAvatar     *string   `json:"sender_avatar"`
	ContentPreview   string    `json:"content_preview"`
	IsMention        bool      `json:"is_mention"`
	IsUnread         bool      `json:"is_unread"`
	CreatedAt        time.Time `json:"created_at"`
	ParentSenderName *string   `json:"parent_sender_name"`
	ParentSenderType *string   `json:"parent_sender_type"`
	ParentSenderID   *string   `json:"parent_sender_id"`
	ParentContent    *string   `json:"parent_content"`
	ParentTaskNumber int       `json:"parent_task_number,omitempty"`
}

type UnreadCount struct {
	Total         int `json:"total"`
	Mentions      int `json:"mentions"`
	ThreadReplies int `json:"thread_replies"`
	DM            int `json:"dm"`
}

const listInboxQuery = `
	SELECT id, item_type, channel_id, channel_name, thread_id, dm_id,
	       sender_name, sender_avatar, content_preview, is_mention, created_at,
	       is_unread, parent_sender_name, parent_sender_type, parent_sender_id, parent_content, parent_task_number
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
		       EXISTS (
		           SELECT 1 FROM user_mentions um
		           WHERE um.message_id = m.id AND um.mentioned_user_id = $1
		       ) AS is_mention,
		       m.created_at,
		       r.message_id IS NULL AS is_unread,
		       COALESCE(pu.display_name, pa.name) AS parent_sender_name,
		       pm.sender_type AS parent_sender_type,
		       pm.sender_id::text AS parent_sender_id,
		       pm.content AS parent_content,
		       COALESCE(pt.task_number, 0) AS parent_task_number
		FROM messages m
		JOIN threads t ON m.thread_id = t.id
		JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
		LEFT JOIN messages pm ON pm.id = t.root_message_id
		LEFT JOIN tasks pt ON pt.message_id = pm.id
		LEFT JOIN users pu ON pm.sender_type = 'user' AND pm.sender_id = pu.id
		LEFT JOIN agents pa ON pm.sender_type = 'agent' AND pm.sender_id = pa.id
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
		WHERE m.sender_id != $1
		  AND m.sender_type IN ('user', 'agent')
		  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		  AND m.thread_id IS NOT NULL
		  AND (
		      pm.sender_type = 'user' AND pm.sender_id = $1
		      OR EXISTS (
		          SELECT 1 FROM user_mentions um
		          WHERE um.message_id = m.id AND um.mentioned_user_id = $1
		      )
		  )
		  AND m.created_at < $2
		  AND m.created_at > $3
		  AND (COALESCE($5::text[], '{}'::text[]) = '{}'::text[] OR 'thread_reply' = ANY($5::text[]))
		  AND ($6 = '' OR COALESCE(u.display_name, a.name) ILIKE '%' || $6 || '%')
		  AND COALESCE(m.is_deleted, false) = false
		  AND c.workspace_id = $7

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
		       m.created_at,
		       r.message_id IS NULL AS is_unread,
		       NULL::text AS parent_sender_name,
		       NULL::text AS parent_sender_type,
		       NULL::text AS parent_sender_id,
		       NULL::text AS parent_content,
		       0 AS parent_task_number
		FROM messages m
		JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
		JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
		WHERE m.sender_id != $1
		  AND m.sender_type IN ('user', 'agent')
		  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		  AND m.thread_id IS NULL
		  AND m.created_at < $2
		  AND m.created_at > $3
		  AND ($5::text[] = '{}' OR 'dm' = ANY($5::text[]))
		  AND ($6 = '' OR COALESCE(u.display_name, a.name) ILIKE '%' || $6 || '%')
		  AND COALESCE(m.is_deleted, false) = false
		  AND c.workspace_id = $7

		UNION ALL

		-- @Mentions via user_mentions
		SELECT m.id,
		       'mention' AS item_type,
		       c.id::text AS channel_id,
		       c.name AS channel_name,
		       NULL::text AS thread_id,
		       NULL::text AS dm_id,
		       COALESCE(u.display_name, a.name, 'Unknown') AS sender_name,
		       NULL::text AS sender_avatar,
		       LEFT(m.content, 50) AS content_preview,
		       true AS is_mention,
		       m.created_at,
		       r.message_id IS NULL AS is_unread,
		       NULL::text AS parent_sender_name,
		       NULL::text AS parent_sender_type,
		       NULL::text AS parent_sender_id,
		       NULL::text AS parent_content,
		       0 AS parent_task_number
		FROM messages m
		JOIN user_mentions um ON um.message_id = m.id AND um.mentioned_user_id = $1
		JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
		WHERE m.sender_id != $1
		  AND m.sender_type IN ('user', 'agent')
		  AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		  AND m.thread_id IS NULL
		  AND m.created_at < $2
		  AND m.created_at > $3
		  AND ($5::text[] = '{}' OR 'mention' = ANY($5::text[]))
		  AND ($6 = '' OR COALESCE(u.display_name, a.name) ILIKE '%' || $6 || '%')
		  AND COALESCE(m.is_deleted, false) = false
		  AND c.workspace_id = $7
	) sub
	ORDER BY created_at DESC
	LIMIT $4
`

func (s *InboxService) List(ctx context.Context, userID string, before time.Time, limit int, types []string, senderFilter string) ([]InboxItem, bool, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}

	var clearedBefore time.Time
	workspaceID := serverworkspace.ContextID(ctx)
	s.pool.QueryRow(ctx,
		`SELECT COALESCE(cleared_before, '1970-01-01'::timestamptz)
		 FROM workspace_inbox_state WHERE user_id = $1 AND workspace_id=$2`, userID, workspaceID,
	).Scan(&clearedBefore)

	args := []any{userID, before, clearedBefore, limit + 1, types, senderFilter, workspaceID}
	rows, err := s.pool.Query(ctx, listInboxQuery, args...)
	if err != nil {
		return nil, false, fmt.Errorf("inbox list: %w", err)
	}
	defer rows.Close()

	items := make([]InboxItem, 0, limit)
	for rows.Next() {
		var item InboxItem
		if err := rows.Scan(&item.ID, &item.Type, &item.ChannelID, &item.ChannelName,
			&item.ThreadID, &item.DMID, &item.SenderName, &item.SenderAvatar,
			&item.ContentPreview, &item.IsMention, &item.CreatedAt, &item.IsUnread,
			&item.ParentSenderName, &item.ParentSenderType, &item.ParentSenderID, &item.ParentContent, &item.ParentTaskNumber); err != nil {
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

func (s *InboxService) UnreadCount(ctx context.Context, userID string) (*UnreadCount, error) {
	readFilter := `AND m.id NOT IN (SELECT message_id FROM user_inbox_reads WHERE user_id = $1)`
	workspaceID := serverworkspace.ContextID(ctx)

	result := &UnreadCount{}

	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN threads t ON m.thread_id = t.id
		 JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
		 LEFT JOIN messages pm ON pm.id = t.root_message_id
		 LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		 LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		 WHERE m.sender_id != $1
		   AND m.sender_type IN ('user', 'agent')
		   AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		   AND m.thread_id IS NOT NULL
		   AND (
		       pm.sender_type = 'user' AND pm.sender_id = $1
		       OR EXISTS (
		           SELECT 1 FROM user_mentions um
		           WHERE um.message_id = m.id AND um.mentioned_user_id = $1
		       )
		   )
		   AND COALESCE(m.is_deleted, false) = false
		   AND c.workspace_id = $2
		   `+readFilter,
		userID, workspaceID,
	).Scan(&result.ThreadReplies)
	if err != nil {
		result.ThreadReplies = 0
	}

	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
		 JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
		 LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		 LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		 WHERE m.sender_id != $1
		   AND m.sender_type IN ('user', 'agent')
		   AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		   AND m.thread_id IS NULL
		   AND COALESCE(m.is_deleted, false) = false
		   AND c.workspace_id = $2
		   `+readFilter,
		userID, workspaceID,
	).Scan(&result.DM)
	if err != nil {
		result.DM = 0
	}

	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM messages m
		 JOIN user_mentions um ON um.message_id = m.id AND um.mentioned_user_id = $1
		 JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
		 LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		 LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		 WHERE m.sender_id != $1
		   AND m.sender_type IN ('user', 'agent')
		   AND (u.id IS NOT NULL OR a.id IS NOT NULL)
		   AND m.thread_id IS NULL
		   AND COALESCE(m.is_deleted, false) = false
		   AND c.workspace_id = $2
		   `+readFilter,
		userID, workspaceID,
	).Scan(&result.Mentions)
	if err != nil {
		result.Mentions = 0
	}

	result.Total = result.ThreadReplies + result.DM + result.Mentions
	return result, nil
}

func (s *InboxService) MarkRead(ctx context.Context, userID, messageID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_inbox_reads (user_id, message_id)
		 SELECT $1, m.id FROM messages m JOIN channels c ON c.id=m.channel_id
		 WHERE m.id=$2 AND c.workspace_id=$3
		 ON CONFLICT DO NOTHING`,
		userID, messageID, serverworkspace.ContextID(ctx),
	)
	if err != nil {
		return fmt.Errorf("mark inbox item read: %w", err)
	}
	return nil
}

func (s *InboxService) ClearAll(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workspace_inbox_state (user_id, workspace_id, cleared_before, updated_at)
		 VALUES ($1, $2, now(), now())
		 ON CONFLICT (user_id, workspace_id) DO UPDATE SET cleared_before = now(), updated_at = now()`,
		userID, serverworkspace.ContextID(ctx),
	)
	if err != nil {
		return fmt.Errorf("clear inbox: %w", err)
	}
	return s.MarkAllRead(ctx, userID)
}

func (s *InboxService) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_inbox_reads (user_id, message_id)
		 SELECT $1, sub.id FROM (
			SELECT m.id FROM messages m
			JOIN threads t ON m.thread_id = t.id
			JOIN channels c ON t.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN messages pm ON pm.id = t.root_message_id
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent') AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NOT NULL
			  AND (
			      pm.sender_type = 'user' AND pm.sender_id = $1
			      OR EXISTS (
			          SELECT 1 FROM user_mentions um
			          WHERE um.message_id = m.id AND um.mentioned_user_id = $1
			      )
			  )
			  AND COALESCE(m.is_deleted, false) = false
			  AND c.workspace_id=$2
			  AND r.message_id IS NULL
			UNION
			SELECT m.id FROM messages m
			JOIN channels c ON m.channel_id = c.id AND c.type = 'dm'
			JOIN dm_members dm ON dm.channel_id = c.id AND dm.member_id = $1
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent') AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NULL AND COALESCE(m.is_deleted, false) = false
			  AND c.workspace_id=$2
			  AND r.message_id IS NULL
			UNION
			SELECT m.id FROM messages m
			JOIN user_mentions um ON um.message_id = m.id AND um.mentioned_user_id = $1
			JOIN channels c ON m.channel_id = c.id AND c.type != 'dm'
			LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
			LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
			LEFT JOIN user_inbox_reads r ON r.user_id = $1 AND r.message_id = m.id
			WHERE m.sender_id != $1
			  AND m.sender_type IN ('user', 'agent') AND (u.id IS NOT NULL OR a.id IS NOT NULL)
			  AND m.thread_id IS NULL AND COALESCE(m.is_deleted, false) = false AND r.message_id IS NULL
			  AND c.workspace_id=$2
		 ) sub ON CONFLICT DO NOTHING`,
		userID, serverworkspace.ContextID(ctx),
	)
	if err != nil {
		return fmt.Errorf("mark all inbox read: %w", err)
	}
	return nil
}

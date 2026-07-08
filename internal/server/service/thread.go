package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ThreadService handles thread business logic.
type ThreadService struct {
	pool *pgxpool.Pool
}

// NewThreadService creates a new ThreadService.
func NewThreadService(pool *pgxpool.Pool) *ThreadService {
	return &ThreadService{pool: pool}
}

// ThreadMessage represents a message in a thread for agent context.
type ThreadMessage struct {
	ID            string    `json:"id"`
	ThreadID      string    `json:"thread_id"`
	SenderType    string    `json:"sender_type"`
	SenderID      string    `json:"sender_id"`
	SenderName    string    `json:"sender_name"`
	Content       string    `json:"content"`
	AttachmentIDs []string  `json:"attachment_ids,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetThreadContextMessages returns all messages in a thread, ordered chronologically,
// to be used as context for Agent auto-response.
// If the thread does not exist, it returns ErrThreadNotFound.
func (s *ThreadService) GetThreadContextMessages(ctx context.Context, threadID string) ([]ThreadMessage, error) {
	// Verify thread exists
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM threads WHERE id = $1)`, threadID,
	).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrThreadNotFound
	}

	// Fetch all messages in the thread, ordered chronologically (oldest first)
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, COALESCE(m.thread_id::text, ''), m.sender_type, m.sender_id,
		        COALESCE(u.display_name, a.name, m.sender_id::text) AS sender_name,
			m.content, COALESCE(m.attachment_ids, '{}') as attachment_ids, m.created_at
		 FROM messages m
		 LEFT JOIN users u ON m.sender_id = u.id
		 LEFT JOIN agents a ON m.sender_id = a.id
		 WHERE m.thread_id = $1
		 ORDER BY created_at ASC, id ASC`,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ThreadMessage
	for rows.Next() {
		var msg ThreadMessage
		if err := rows.Scan(&msg.ID, &msg.ThreadID, &msg.SenderType, &msg.SenderID, &msg.SenderName, &msg.Content, &msg.AttachmentIDs, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if messages == nil {
		messages = []ThreadMessage{}
	}

	return messages, nil
}

// GetOrCreateThread finds or creates a thread for the given root message in a channel.
// Returns the thread ID and whether it was newly created.
func (s *ThreadService) GetOrCreateThread(ctx context.Context, channelID, rootMessageID string) (string, bool, error) {
	var threadID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM threads WHERE root_message_id = $1 AND channel_id = $2`,
		rootMessageID, channelID,
	).Scan(&threadID)
	if err == nil {
		return threadID, false, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}

	// Create new thread — use ON CONFLICT to handle concurrent callers
	// that might also try to create a thread for the same root_message_id.
	err = s.pool.QueryRow(ctx,
		`INSERT INTO threads (channel_id, root_message_id, last_reply_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (root_message_id) DO UPDATE SET root_message_id = EXCLUDED.root_message_id
		 RETURNING id`,
		channelID, rootMessageID,
	).Scan(&threadID)
	if err != nil {
		return "", false, err
	}

	return threadID, true, nil
}

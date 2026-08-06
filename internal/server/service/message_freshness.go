package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const FreshnessHeldMessageLimit = 5

var ErrFreshnessRunUnavailable = errors.New("agent run is unavailable for freshness check")

type AgentSendFreshnessInput struct {
	RunID     string
	AgentID   string
	ChannelID string
	ThreadID  string
}

type FreshnessHeldMessage struct {
	ID         string
	Seq        int64
	SenderType string
	SenderID   string
	SenderName string
	Content    string
	CreatedAt  time.Time
}

type AgentSendFreshnessHold struct {
	Messages            []FreshnessHeldMessage
	NewMessageCount     int
	ShownMessageCount   int
	OmittedMessageCount int
	SeenUpToSeq         int64
}

// LockMessageScope serializes visible message writes for one conversation
// scope across Server instances. All canonical REST and legacy WS writers use
// the same transaction lock; the critical section only covers check/insert.
func LockMessageScope(ctx context.Context, tx pgx.Tx, channelID, threadID, thinkingNodeID string) error {
	scopeKey := channelID + ":" + threadID + ":" + thinkingNodeID
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, scopeKey)
	return err
}

// CheckAndHoldAgentSend returns a hold when the active Run has not observed
// newer visible messages in its own Channel/DM/Thread scope. Callers must hold
// the scope lock and either commit the cursor advance or insert the message in
// the same transaction.
func CheckAndHoldAgentSend(ctx context.Context, tx pgx.Tx, input AgentSendFreshnessInput) (*AgentSendFreshnessHold, error) {
	var seenSeq sql.NullInt64
	err := tx.QueryRow(ctx, `
		SELECT freshness_seen_seq
		  FROM agent_runs
		 WHERE id = $1
		   AND agent_id = $2
		   AND channel_id = $3
		   AND COALESCE(thread_id::text, '') = $4
		   AND thinking_node_id IS NULL
		   AND finished_at IS NULL
		 FOR UPDATE`,
		input.RunID, input.AgentID, input.ChannelID, input.ThreadID,
	).Scan(&seenSeq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFreshnessRunUnavailable
		}
		return nil, err
	}
	if !seenSeq.Valid {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT m.id::text, m.seq, m.sender_type, m.sender_id::text,
		       COALESCE(u.display_name, a.name, m.sender_id::text),
		       m.content, m.created_at, COUNT(*) OVER()
		  FROM messages m
		  LEFT JOIN users u ON m.sender_type = 'user' AND u.id = m.sender_id
		  LEFT JOIN agents a ON m.sender_type = 'agent' AND a.id = m.sender_id
		 WHERE m.channel_id = $1
		   AND COALESCE(m.thread_id::text, '') = $2
		   AND m.thinking_node_id IS NULL
		   AND COALESCE(m.is_deleted, false) = false
		   AND m.seq > $3
		   AND m.sender_type = 'agent'
		   AND COALESCE(m.metadata->>'agent_run_id', '') <> $4
		 ORDER BY m.seq DESC
		 LIMIT $5`,
		input.ChannelID, input.ThreadID, seenSeq.Int64, input.RunID, FreshnessHeldMessageLimit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]FreshnessHeldMessage, 0, FreshnessHeldMessageLimit)
	total := 0
	var latestSeq int64
	for rows.Next() {
		var message FreshnessHeldMessage
		if err := rows.Scan(
			&message.ID, &message.Seq, &message.SenderType, &message.SenderID,
			&message.SenderName, &message.Content, &message.CreatedAt, &total,
		); err != nil {
			return nil, err
		}
		if message.Seq > latestSeq {
			latestSeq = message.Seq
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}

	// The query returns newest first; the model should receive chronological context.
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}

	if _, err := tx.Exec(ctx, `
		UPDATE agent_runs
		   SET freshness_seen_seq = GREATEST(COALESCE(freshness_seen_seq, 0), $2),
		       freshness_held_at = now(),
		       updated_at = now()
		 WHERE id = $1`, input.RunID, latestSeq); err != nil {
		return nil, err
	}

	return &AgentSendFreshnessHold{
		Messages:            messages,
		NewMessageCount:     total,
		ShownMessageCount:   len(messages),
		OmittedMessageCount: total - len(messages),
		SeenUpToSeq:         latestSeq,
	}, nil
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const FreshnessHeldMessageLimit = 5

var ErrFreshnessRunUnavailable = errors.New("agent run is unavailable for freshness check")
var ErrFreshnessDraftUnchanged = errors.New("this draft was held for human corrections; revise it, or explicitly confirm the latest seen sequence with a reason before sending again")

type AgentSendFreshnessInput struct {
	KeepAfterSeq    int64
	FreshnessReason string
	Draft           string
	RunID           string
	AgentID         string
	ChannelID       string
	ThreadID        string
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

// TaskCorrectionRun associates a human follow-up only with one identifiable
// active task in its original thread. Call within the message scope transaction.
// A finishing Run does not reject the new message; the normal inbox still owns it.
func TaskCorrectionRun(ctx context.Context, tx pgx.Tx, channelID, threadID string) (string, error) {
	if threadID == "" {
		return "", nil
	}
	var runID string
	err := tx.QueryRow(ctx, `SELECT CASE WHEN count(DISTINCT r.id)=1 THEN min(r.id::text) ELSE '' END
 FROM agent_runs r JOIN threads th ON th.id=r.thread_id AND th.channel_id=r.channel_id
 JOIN tasks t ON t.message_id=th.root_message_id AND t.channel_id=th.channel_id
 JOIN agent_run_task_links link ON link.run_id=r.id AND link.task_id=t.id
 WHERE r.channel_id=$1 AND r.thread_id=$2 AND r.thinking_node_id IS NULL
 AND r.finished_at IS NULL AND t.status NOT IN ('done','closed','cancelled')`, channelID, threadID).Scan(&runID)
	return runID, err
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
		if input.KeepAfterSeq != 0 {
			return nil, ErrFreshnessDraftUnchanged
		}
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
		   AND (m.sender_type = 'agent' OR (m.sender_type='user' AND m.metadata->>'correction_of_run_id'=$4))
		   AND COALESCE(m.metadata->>'agent_run_id', '') <> $4
		 ORDER BY m.seq ASC
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
		var matchesDraft, needsDecision bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_held_drafts WHERE run_id=$1 AND btrim(content)=btrim($2)),EXISTS(SELECT 1 FROM agent_message_consumptions WHERE run_id=$1)`, input.RunID, input.Draft).Scan(&matchesDraft, &needsDecision); err != nil {
			return nil, err
		}
		if input.KeepAfterSeq != 0 && (!matchesDraft || input.KeepAfterSeq != seenSeq.Int64 || strings.TrimSpace(input.FreshnessReason) == "" || len(input.FreshnessReason) > 4000) {
			return nil, ErrFreshnessDraftUnchanged
		}
		if matchesDraft && needsDecision && input.KeepAfterSeq == 0 {
			return nil, ErrFreshnessDraftUnchanged
		}
		return nil, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE agent_runs
		   SET freshness_seen_seq = GREATEST(COALESCE(freshness_seen_seq, 0), $2),
		       freshness_held_at = now(),
		       updated_at = now()
		 WHERE id = $1`, input.RunID, latestSeq); err != nil {
		return nil, err
	}

	if input.Draft != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO agent_held_drafts(run_id,channel_id,thread_id,content,based_on_seq) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id) DO UPDATE SET content=EXCLUDED.content,based_on_seq=EXCLUDED.based_on_seq,created_at=now()`, input.RunID, input.ChannelID, nullableStr(input.ThreadID), input.Draft, seenSeq.Int64); err != nil {
			return nil, err
		}
	}
	for _, message := range messages {
		if message.SenderType == "user" {
			if _, err := tx.Exec(ctx, `INSERT INTO agent_message_consumptions(agent_id,message_id,run_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, input.AgentID, message.ID, input.RunID); err != nil {
				return nil, err
			}
		}
	}
	return &AgentSendFreshnessHold{
		Messages:            messages,
		NewMessageCount:     total,
		ShownMessageCount:   len(messages),
		OmittedMessageCount: total - len(messages),
		SeenUpToSeq:         latestSeq,
	}, nil
}

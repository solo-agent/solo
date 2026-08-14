package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrChannelPostingRestricted = errors.New("posting is restricted in this Channel")

// CheckHumanChannelPosting enforces group moderation for ordinary Channels.
// Agent messages and private/Lucy/DM scopes are deliberately handled elsewhere.
func CheckHumanChannelPosting(ctx context.Context, pool *pgxpool.Pool, channelID, userID string) error {
	var channelType, policy, role string
	var muted bool
	err := pool.QueryRow(ctx, `
		SELECT c.type, c.posting_policy, COALESCE(wm.role,''),
		       EXISTS(SELECT 1 FROM channel_member_mutes mute
		               WHERE mute.channel_id=c.id AND mute.user_id=$2
		                 AND (mute.expires_at IS NULL OR mute.expires_at>now()))
		  FROM channels c
		  LEFT JOIN workspace_members wm ON wm.workspace_id=c.workspace_id AND wm.user_id=$2
		 WHERE c.id=$1`, channelID, userID).Scan(&channelType, &policy, &role, &muted)
	if err != nil {
		return ErrChannelPostingRestricted
	}
	if channelType != "channel" {
		return nil
	}
	if muted || (policy == "admins_only" && role != "owner" && role != "admin") {
		return ErrChannelPostingRestricted
	}
	return nil
}

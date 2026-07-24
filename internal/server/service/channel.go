package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrChannelNotFound   = errors.New("channel not found")
	ErrMemberNotFound    = errors.New("member not found")
	ErrNotChannelMember  = errors.New("user is not a channel member")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrAlreadyMember     = errors.New("user is already a channel member")
	ErrUserNotFound      = errors.New("user not found")
	ErrAgentNotFound     = errors.New("agent not found")
	ErrChannelNameExists = errors.New("a channel with this name already exists")
	ErrThreadNotFound    = errors.New("thread not found")
)

// ChannelService handles channel business logic.
type ChannelService struct {
	pool *pgxpool.Pool
}

// NewChannelService creates a new ChannelService.
func NewChannelService(pool *pgxpool.Pool) *ChannelService {
	return &ChannelService{pool: pool}
}

// Member represents a channel member with user details for list responses.
type Member struct {
	ChannelID   string    `json:"channel_id"`
	MemberType  string    `json:"member_type"`
	MemberID    string    `json:"member_id"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Email       string    `json:"email,omitempty"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}

// CreateChannel creates a new channel and adds the creator as an owner member.
func (s *ChannelService) CreateChannel(ctx context.Context, name, description, channelType, createdBy string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Check if name already exists for active channels of type 'channel'
	if channelType == "channel" {
		var exists bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM channels
				WHERE name = $1 AND type = 'channel' AND is_archived = false
			)`, name,
		).Scan(&exists)
		if err != nil {
			return "", err
		}
		if exists {
			return "", ErrChannelNameExists
		}
	}

	var channelID string
	err = tx.QueryRow(ctx,
		`INSERT INTO channels (name, description, type, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		name, description, channelType, createdBy,
	).Scan(&channelID)
	if err != nil {
		return "", err
	}

	// Add creator as owner
	_, err = tx.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, 'user', $2, 'owner')`,
		channelID, createdBy,
	)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return channelID, nil
}

// AddMember adds a user to a channel. Only owners and admins can add members.
func (s *ChannelService) AddMember(ctx context.Context, channelID, requesterID, memberType, memberID string) error {
	// Verify channel exists
	var channelExists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE id = $1 AND is_archived = false)`, channelID,
	).Scan(&channelExists)
	if err != nil {
		return err
	}
	if !channelExists {
		return ErrChannelNotFound
	}

	// Verify requester is channel owner or admin
	var requesterRole string
	err = s.pool.QueryRow(ctx,
		`SELECT role FROM channel_members
		 WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2`,
		channelID, requesterID,
	).Scan(&requesterRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotChannelMember
		}
		return err
	}
	if requesterRole != "owner" && requesterRole != "admin" {
		return ErrPermissionDenied
	}

	// Verify member existence based on type
	switch memberType {
	case "user":
		var userExists bool
		err = s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_active = true)`, memberID,
		).Scan(&userExists)
		if err != nil {
			return err
		}
		if !userExists {
			return ErrUserNotFound
		}
	case "agent":
		var agentOwnerID, homeChannelID, kind string
		err = s.pool.QueryRow(ctx,
			`SELECT owner_id, home_channel_id, kind
			   FROM agents
			  WHERE id = $1 AND is_active = true`, memberID,
		).Scan(&agentOwnerID, &homeChannelID, &kind)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAgentNotFound
			}
			return err
		}
		// Verify the agent belongs to the requesting user
		if agentOwnerID != requesterID || kind != "agent" || homeChannelID != channelID {
			return ErrAgentNotFound
		}
	}

	// Check if already a member
	var alreadyMember bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type = $2 AND member_id = $3
		)`, channelID, memberType, memberID,
	).Scan(&alreadyMember)
	if err != nil {
		return err
	}
	if alreadyMember {
		return ErrAlreadyMember
	}

	// Add the member with default 'member' role
	_, err = s.pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, member_type, member_id, role)
		 VALUES ($1, $2, $3, 'member')`,
		channelID, memberType, memberID,
	)
	return err
}

// RemoveMember removes a member from a channel. Only owners and admins can remove members.
// Members can remove themselves.
func (s *ChannelService) RemoveMember(ctx context.Context, channelID, requesterID, memberID string) (string, error) {
	// Verify member exists in channel
	var memberType, currentRole string
	err := s.pool.QueryRow(ctx,
		`SELECT member_type, role FROM channel_members
		 WHERE channel_id = $1 AND member_id = $2`,
		channelID, memberID,
	).Scan(&memberType, &currentRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrMemberNotFound
		}
		return "", err
	}

	// Check permissions
	if memberID != requesterID {
		// Only owner/admin can remove other members
		var requesterRole string
		err = s.pool.QueryRow(ctx,
			`SELECT role FROM channel_members
			 WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2`,
			channelID, requesterID,
		).Scan(&requesterRole)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrNotChannelMember
			}
			return "", err
		}
		if requesterRole != "owner" && requesterRole != "admin" {
			return "", ErrPermissionDenied
		}

		// Only owners can remove other owners/admins
		if (currentRole == "owner" || currentRole == "admin") && requesterRole != "owner" {
			return "", ErrPermissionDenied
		}
	}

	if memberType == "agent" {
		tx, txErr := s.pool.Begin(ctx)
		if txErr != nil {
			return "", txErr
		}
		defer tx.Rollback(ctx)
		result, txErr := tx.Exec(ctx, `
			UPDATE agents
			   SET is_active = false, updated_at = now()
			 WHERE id = $2
			   AND home_channel_id = $1
			   AND kind = 'agent'
			   AND is_active = true
		`, channelID, memberID)
		if txErr != nil {
			return "", txErr
		}
		if result.RowsAffected() == 0 {
			return "", ErrMemberNotFound
		}
		if _, txErr = tx.Exec(ctx, `
			UPDATE tasks
			   SET status = 'todo', claimer_id = NULL, updated_at = now()
			 WHERE channel_id = $1
			   AND claimer_id = $2
			   AND status IN ('in_progress', 'in_review')
		`, channelID, memberID); txErr != nil {
			return "", txErr
		}
		if _, txErr = tx.Exec(ctx, `
			UPDATE agent_runs
			   SET status = 'cancelled',
			       activity_text = 'Cancelled because the Agent was removed',
			       updated_at = now(),
			       finished_at = COALESCE(finished_at, now())
			 WHERE agent_id = $1
			   AND status IN (
			       'queued', 'thinking', 'running', 'streaming',
			       'waiting_input', 'waiting_approval'
			   )
		`, memberID); txErr != nil {
			return "", txErr
		}
		if _, txErr = tx.Exec(ctx, `
			UPDATE agent_sessions
			   SET status = 'closed', last_active_at = now()
			 WHERE agent_id = $1 AND status = 'active'
		`, memberID); txErr != nil {
			return "", txErr
		}
		if _, txErr = tx.Exec(ctx, `
			UPDATE computers
			   SET agent_ids = array_remove(agent_ids, $1::uuid), updated_at = now()
			 WHERE $1::uuid = ANY(agent_ids)
		`, memberID); txErr != nil {
			return "", txErr
		}
		if _, txErr = tx.Exec(ctx, `
			DELETE FROM channel_members
			 WHERE channel_id = $1
			   AND member_type = 'agent'
			   AND member_id = $2
		`, channelID, memberID); txErr != nil {
			return "", txErr
		}
		if txErr = tx.Commit(ctx); txErr != nil {
			return "", txErr
		}
		return memberType, nil
	}
	_, err = s.pool.Exec(ctx, `
		DELETE FROM channel_members
		 WHERE channel_id = $1 AND member_id = $2
	`, channelID, memberID)
	return memberType, err
}

// ListMembers returns all members of a channel.
func (s *ChannelService) ListMembers(ctx context.Context, channelID, requesterID string) ([]Member, error) {
	// Verify requester is a member of the channel
	var isMember bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, channelID, requesterID,
	).Scan(&isMember)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotChannelMember
	}

	// List members - join with users for user members, agents for agent members
	rows, err := s.pool.Query(ctx,
		`SELECT cm.channel_id, cm.member_type, cm.member_id,
				COALESCE(u.display_name, a.name, 'Unknown'), COALESCE(u.email, ''),
				COALESCE(u.avatar_url, a.avatar_url, ''),
				cm.role, cm.joined_at
		 FROM channel_members cm
		 LEFT JOIN users u ON cm.member_type = 'user' AND cm.member_id = u.id
		 LEFT JOIN agents a ON cm.member_type = 'agent' AND cm.member_id = a.id
		 WHERE cm.channel_id = $1
		   AND (cm.member_type != 'agent' OR COALESCE(a.is_active, false) = true)
		 ORDER BY cm.joined_at ASC`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ChannelID, &m.MemberType, &m.MemberID,
			&m.DisplayName, &m.Email, &m.AvatarURL, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if members == nil {
		members = []Member{}
	}

	return members, nil
}

// IsChannelMember checks if a user is a member of a channel.
func (s *ChannelService) IsChannelMember(ctx context.Context, channelID, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM channel_members
			WHERE channel_id = $1 AND member_type IN ('user', 'agent') AND member_id = $2
		)`, channelID, userID,
	).Scan(&exists)
	return exists, err
}

// ResolveChannelName looks up a channel ID by its name.
func (s *ChannelService) ResolveChannelName(ctx context.Context, name string) (string, bool) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM channels WHERE name = $1`, name).Scan(&id)
	if err != nil {
		return "", false
	}
	return id, true
}

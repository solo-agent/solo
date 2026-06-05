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
		var agentOwnerID string
		err = s.pool.QueryRow(ctx,
			`SELECT owner_id FROM agents WHERE id = $1 AND is_active = true`, memberID,
		).Scan(&agentOwnerID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAgentNotFound
			}
			return err
		}
		// Verify the agent belongs to the requesting user
		if agentOwnerID != requesterID {
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
func (s *ChannelService) RemoveMember(ctx context.Context, channelID, requesterID, memberID string) error {
	// Verify member exists in channel
	var memberType, currentRole string
	err := s.pool.QueryRow(ctx,
		`SELECT member_type, role FROM channel_members
		 WHERE channel_id = $1 AND member_id = $2`,
		channelID, memberID,
	).Scan(&memberType, &currentRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMemberNotFound
		}
		return err
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
				return ErrNotChannelMember
			}
			return err
		}
		if requesterRole != "owner" && requesterRole != "admin" {
			return ErrPermissionDenied
		}

		// Only owners can remove other owners/admins
		if (currentRole == "owner" || currentRole == "admin") && requesterRole != "owner" {
			return ErrPermissionDenied
		}
	}

	_, err = s.pool.Exec(ctx,
		`DELETE FROM channel_members
		 WHERE channel_id = $1 AND member_id = $2`,
		channelID, memberID,
	)
	return err
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
				cm.role, cm.joined_at
		 FROM channel_members cm
		 LEFT JOIN users u ON cm.member_type = 'user' AND cm.member_id = u.id
		 LEFT JOIN agents a ON cm.member_type = 'agent' AND cm.member_id = a.id
		 WHERE cm.channel_id = $1
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
			&m.DisplayName, &m.Email, &m.Role, &m.JoinedAt); err != nil {
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

package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelBinding represents a project repository binding for a channel.
type ChannelBinding struct {
	ChannelID  string    `json:"channel_id"`
	RepoURL    string    `json:"repo_url"`
	RepoBranch string    `json:"repo_branch"`
	BindPath   string    `json:"bind_path"`
	BoundBy    string    `json:"bound_by"`
	BoundAt    time.Time `json:"bound_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ChannelBindingService manages channel-to-repository bindings.
type ChannelBindingService struct {
	pool *pgxpool.Pool
}

// NewChannelBindingService creates a new ChannelBindingService.
func NewChannelBindingService(pool *pgxpool.Pool) *ChannelBindingService {
	return &ChannelBindingService{pool: pool}
}

// channelWorkspaceRoot returns the base path for a channel's workspace.
func channelWorkspaceRoot(channelID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".solo", "channels", channelID, "workspace")
}

// BindProject binds a project repository to a channel.
// Only channel admins (owner role) can bind a project.
// The git clone runs asynchronously to avoid blocking the HTTP request.
func (s *ChannelBindingService) BindProject(ctx context.Context, channelID, repoURL, branch, userID string) (*ChannelBinding, error) {
	if !s.isChannelAdmin(ctx, channelID, userID) {
		return nil, fmt.Errorf("only channel admin can bind a project")
	}

	var existing bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channel_bindings WHERE channel_id = $1)`,
		channelID,
	).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("check existing binding: %w", err)
	}
	if existing {
		return nil, fmt.Errorf("channel already has a project binding")
	}

	if branch == "" {
		branch = "main"
	}

	bindPath := filepath.Join(channelWorkspaceRoot(channelID), "repo")
	now := time.Now().UTC()

	_, err = s.pool.Exec(ctx, `
		INSERT INTO channel_bindings (channel_id, repo_url, repo_branch, bind_path, bound_by, bound_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, channelID, repoURL, branch, bindPath, userID, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert binding: %w", err)
	}

	// Clone the repo asynchronously.
	go s.cloneRepo(channelID, repoURL, branch, bindPath)

	return &ChannelBinding{
		ChannelID:  channelID,
		RepoURL:    repoURL,
		RepoBranch: branch,
		BindPath:   bindPath,
		BoundBy:    userID,
		BoundAt:    now,
		UpdatedAt:  now,
	}, nil
}

// GetBinding returns the binding for a channel.
func (s *ChannelBindingService) GetBinding(ctx context.Context, channelID string) (*ChannelBinding, error) {
	var b ChannelBinding
	err := s.pool.QueryRow(ctx, `
		SELECT channel_id, repo_url, repo_branch, bind_path, bound_by, bound_at, updated_at
		FROM channel_bindings WHERE channel_id = $1
	`, channelID).Scan(&b.ChannelID, &b.RepoURL, &b.RepoBranch, &b.BindPath, &b.BoundBy, &b.BoundAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get binding: %w", err)
	}
	return &b, nil
}

// UnbindProject removes the project binding for a channel.
// Does not delete local files.
func (s *ChannelBindingService) UnbindProject(ctx context.Context, channelID, userID string) error {
	if !s.isChannelAdmin(ctx, channelID, userID) {
		return fmt.Errorf("only channel admin can unbind a project")
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM channel_bindings WHERE channel_id = $1`,
		channelID,
	)
	if err != nil {
		return fmt.Errorf("unbind: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no binding found for this channel")
	}
	return nil
}

// ResolveWorkingDirectory returns the best working directory for an agent in a
// channel: worktree > channel repo > agent personal workspace.
func (s *ChannelBindingService) ResolveWorkingDirectory(ctx context.Context, channelID, agentID string) string {
	// Check for channel repo binding.
	var bindPath string
	err := s.pool.QueryRow(ctx,
		`SELECT bind_path FROM channel_bindings WHERE channel_id = $1`,
		channelID,
	).Scan(&bindPath)
	if err == nil && bindPath != "" {
		if _, err := os.Stat(bindPath); err == nil {
			return bindPath
		}
	}

	// Fall back to agent personal workspace.
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".solo", "agents", agentID, "workspace")
}

// isChannelAdmin checks if the user has the owner role in the channel.
func (s *ChannelBindingService) isChannelAdmin(ctx context.Context, channelID, userID string) bool {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM channel_members
		 WHERE channel_id = $1 AND member_id = $2`,
		channelID, userID,
	).Scan(&role)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

// cloneRepo runs git clone asynchronously. Results are logged.
func (s *ChannelBindingService) cloneRepo(channelID, repoURL, branch, bindPath string) {
	if err := os.MkdirAll(filepath.Dir(bindPath), 0755); err != nil {
		slog.Error("channel binding: failed to create workspace dir",
			"channel_id", channelID, "path", filepath.Dir(bindPath), "error", err)
		return
	}

	cmd := exec.Command("git", "clone", "--branch", branch, "--single-branch", repoURL, bindPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("channel binding: git clone failed",
			"channel_id", channelID, "repo", repoURL, "branch", branch, "error", err, "output", string(output))
		return
	}
	slog.Info("channel binding: repo cloned",
		"channel_id", channelID, "path", bindPath)
}

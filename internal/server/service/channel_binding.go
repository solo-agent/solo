package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Fire-and-forget tree scan to surface any clone issues immediately and
	// warm the FS page cache so the first user-facing GetWorkspace is fast.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.ScanWorkspace(ctx, channelID); err != nil {
			slog.Warn("channel binding: post-clone scan failed",
				"channel_id", channelID, "error", err)
			return
		}
		slog.Info("channel binding: post-clone scan completed", "channel_id", channelID)
	}()
}

const maxScanDepth = 20

// FileNode represents a node in the workspace file tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"is_dir"`
	Size     int64      `json:"size,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

// ScanWorkspace walks the workspace directory for a channel's binding
// and returns the file tree. Returns an error if no binding exists or the
// workspace has not been cloned yet.
func (s *ChannelBindingService) ScanWorkspace(ctx context.Context, channelID string) (*FileNode, error) {
	binding, err := s.GetBinding(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("no binding for channel %s: %w", channelID, err)
	}

	bindPath := binding.BindPath
	info, err := os.Stat(bindPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace not yet cloned: %w", err)
		}
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory")
	}

	return s.scanDir(bindPath, 0)
}

// scanDir recursively builds a FileNode tree for a directory.
func (s *ChannelBindingService) scanDir(dirPath string, depth int) (*FileNode, error) {
	if depth > maxScanDepth {
		return nil, nil
	}

	node := &FileNode{
		Name:     filepath.Base(dirPath),
		Path:     dirPath,
		IsDir:    true,
		Children: []FileNode{},
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(dirPath, entry.Name())
		if entry.IsDir() {
			child, err := s.scanDir(fullPath, depth+1)
			if err != nil {
				continue
			}
			if child != nil {
				node.Children = append(node.Children, *child)
			}
		} else {
			fi, err := entry.Info()
			if err != nil {
				continue
			}
			node.Children = append(node.Children, FileNode{
				Name:  fi.Name(),
				Path:  fullPath,
				IsDir: false,
				Size:  fi.Size(),
			})
		}
	}

	return node, nil
}

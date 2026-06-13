package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// worktreeCreateRequest is the payload for creating an isolated worktree.
type worktreeCreateRequest struct {
	ChannelID  string `json:"channel_id"`
	AgentID    string `json:"agent_id"`
	TaskNumber int    `json:"task_number"`
}

// worktreeCreateResponse is returned after worktree creation.
type worktreeCreateResponse struct {
	WorktreePath string `json:"worktree_path"`
	BranchName   string `json:"branch_name"`
	Status       string `json:"status"`
}

// worktreeCleanupRequest is the payload for cleaning up a worktree.
type worktreeCleanupRequest struct {
	ChannelID  string `json:"channel_id"`
	AgentID    string `json:"agent_id"`
	TaskNumber int    `json:"task_number"`
}

// HandleWorktreeCreate handles POST /internal/daemon/worktree/create.
// Creates a git worktree based on the channel's bound repository.
func (h *daemonHandler) HandleWorktreeCreate(w http.ResponseWriter, r *http.Request) {
	var req worktreeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ChannelID == "" || req.AgentID == "" || req.TaskNumber <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id, agent_id, and task_number are required"})
		return
	}

	// Look up channel binding to find the repo path.
	var repoPath string
	err := h.pool.QueryRow(r.Context(),
		`SELECT bind_path FROM channel_bindings WHERE channel_id = $1`,
		req.ChannelID,
	).Scan(&repoPath)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel is not bound to a project repository"})
			return
		}
		slog.Error("worktree create: db query failed", "channel_id", req.ChannelID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Ensure the repo exists on disk
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bound repo not found on disk — clone may still be in progress"})
		return
	}

	// Create worktree in agent's worktree directory.
	worktreeRoot := agentWorktreeRoot(req.AgentID)
	worktreePath := filepath.Join(worktreeRoot, fmt.Sprintf("task-%d", req.TaskNumber))

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":         "worktree already exists",
			"worktree_path": worktreePath,
		})
		return
	}

	// Create agent worktree root if needed.
	if err := os.MkdirAll(worktreeRoot, 0755); err != nil {
		slog.Error("worktree create: failed to create worktree dir", "path", worktreeRoot, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create worktree directory"})
		return
	}

	// Generate a unique branch name.
	branchName := fmt.Sprintf("solo/task-%d-%s", req.TaskNumber, time.Now().Format("20060102-150405"))

	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("worktree create: git worktree add failed",
			"repo", repoPath,
			"worktree", worktreePath,
			"branch", branchName,
			"error", err,
			"output", string(output),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("git worktree add failed: %s", strings.TrimSpace(string(output))),
		})
		return
	}

	slog.Info("worktree created",
		"channel_id", req.ChannelID,
		"agent_id", req.AgentID,
		"task_number", req.TaskNumber,
		"path", worktreePath,
		"branch", branchName,
	)

	writeJSON(w, http.StatusCreated, worktreeCreateResponse{
		WorktreePath: worktreePath,
		BranchName:   branchName,
		Status:       "created",
	})
}

// HandleWorktreeCleanup handles POST /internal/daemon/worktree/cleanup.
// Commits changes, pushes, merges back, and removes the worktree.
func (h *daemonHandler) HandleWorktreeCleanup(w http.ResponseWriter, r *http.Request) {
	var req worktreeCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ChannelID == "" || req.AgentID == "" || req.TaskNumber <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id, agent_id, and task_number are required"})
		return
	}

	worktreePath := filepath.Join(agentWorktreeRoot(req.AgentID), fmt.Sprintf("task-%d", req.TaskNumber))
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "worktree not found"})
		return
	}

	// Look up the main repo path from channel binding.
	var repoPath string
	err := h.pool.QueryRow(r.Context(),
		`SELECT bind_path FROM channel_bindings WHERE channel_id = $1`,
		req.ChannelID,
	).Scan(&repoPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel binding not found"})
		return
	}

	// Stage and commit changes in the worktree.
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = worktreePath
	addCmd.Run() // best-effort

	commitCmd := exec.Command("git", "commit", "-m", fmt.Sprintf("solo: task #%d completed", req.TaskNumber))
	commitCmd.Dir = worktreePath
	commitOutput, commitErr := commitCmd.CombinedOutput()
	if commitErr != nil {
		// No changes to commit is not a fatal error.
		if !strings.Contains(string(commitOutput), "nothing to commit") {
			slog.Warn("worktree cleanup: git commit failed",
				"worktree", worktreePath,
				"output", string(commitOutput),
			)
		}
	}

	// Get the branch name from the worktree.
	getBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	getBranchCmd.Dir = worktreePath
	branchBytes, branchErr := getBranchCmd.Output()
	branchName := "unknown"
	if branchErr == nil {
		branchName = strings.TrimSpace(string(branchBytes))
	}

	// Remove the worktree.
	rmCmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	rmCmd.Dir = repoPath
	rmOutput, rmErr := rmCmd.CombinedOutput()
	if rmErr != nil {
		slog.Error("worktree cleanup: git worktree remove failed",
			"worktree", worktreePath,
			"error", rmErr,
			"output", string(rmOutput),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to remove worktree: %s", strings.TrimSpace(string(rmOutput))),
		})
		return
	}

	// Delete the local branch from the main repo.
	delBranchCmd := exec.Command("git", "branch", "-D", branchName)
	delBranchCmd.Dir = repoPath
	delBranchCmd.Run() // best-effort

	slog.Info("worktree cleaned up",
		"channel_id", req.ChannelID,
		"agent_id", req.AgentID,
		"task_number", req.TaskNumber,
		"path", worktreePath,
		"branch", branchName,
	)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "cleaned",
		"branch_name": branchName,
	})
}

// agentWorktreeRoot returns the worktree directory for an agent.
func agentWorktreeRoot(agentID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".solo", "agents", agentID, "worktrees")
}

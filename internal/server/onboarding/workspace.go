package onboarding

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// SeedAgentKnowledge writes Lucy's MEMORY.md and notes/ files into the
// agent's workspace directory (~/.solo/agents/{agentID}/workspace/).
// This is the agent's cwd — where BuildSystemPrompt tells it to read MEMORY.md.
// This is best-effort: failures are logged but not returned.
func SeedAgentKnowledge(agentID, displayName, email string) {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("onboarding: cannot find home dir for knowledge seeding", "agent_id", agentID, "error", err)
		return
	}

	workspaceDir := filepath.Join(home, ".solo", "agents", agentID, "workspace")
	notesDir := filepath.Join(workspaceDir, "notes")

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		slog.Warn("onboarding: cannot create notes dir", "agent_id", agentID, "path", notesDir, "error", err)
		return
	}

	files := KnowledgeFiles(displayName, email)
	for relPath, content := range files {
		fullPath := filepath.Join(workspaceDir, relPath)
		// Ensure parent dir exists (handles nested paths like notes/foo.md)
		if dir := filepath.Dir(fullPath); dir != workspaceDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				slog.Warn("onboarding: cannot create parent dir", "agent_id", agentID, "path", dir, "error", err)
				continue
			}
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			slog.Warn("onboarding: cannot write knowledge file", "agent_id", agentID, "path", relPath, "error", err)
			continue
		}
		slog.Info("onboarding: knowledge file written", "agent_id", agentID, "path", relPath)
	}

	slog.Info("onboarding: knowledge seeding complete", "agent_id", agentID, "file_count", len(files))
	fmt.Println("onboarding: knowledge seeding complete")
}

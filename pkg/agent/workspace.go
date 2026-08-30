package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AgentConfig is the local cache of Agent configuration stored in
// solo-config.json inside the agent's workspace root.
type AgentConfig struct {
	AgentID               string            `json:"agent_id"`
	Name                  string            `json:"name"`
	SystemPrompt          string            `json:"system_prompt"`
	ThinkingRuntimePrompt string            `json:"thinking_runtime_prompt,omitempty"`
	Model                 string            `json:"model"`
	Provider              string            `json:"provider"`
	Effort                string            `json:"effort,omitempty"`
	MaxTurns              int               `json:"max_turns,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	CustomArgs            []string          `json:"custom_args,omitempty"`
	// Runtime context for system prompt.
	WorkspacePath string `json:"workspace_path,omitempty"`
	ServerID      string `json:"server_id,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	OS            string `json:"os,omitempty"`
}

// ChannelContext provides channel-level context for InjectConfig.
// It carries the current channel identity and the type of trigger
// that initiated the agent execution.
type ChannelContext struct {
	ChannelID   string      `json:"channel_id"`
	ChannelName string      `json:"channel_name"`
	Description string      `json:"description,omitempty"`
	TriggerType TriggerType `json:"trigger_type"`
	// OtherAgents lists (name, workspace) pairs for other agents in this channel.
	// Enables cross-agent workspace access for verification (Cindy pattern).
	OtherAgents []AgentWorkspace `json:"other_agents,omitempty"`
}

// AgentWorkspace identifies another agent's workspace for cross-agent access.
type AgentWorkspace struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
}

// Workspace represents a prepared Agent workspace directory.
type Workspace struct {
	RootDir   string // ~/.solo/agents/<id>/
	WorkDir   string // ~/.solo/agents/<id>/workspace/ (execution CWD)
	OutputDir string // ~/.solo/agents/<id>/output/
}

// WorkspaceManager manages per-Agent workspace directories under
// ~/.solo/agents/. It provides lifecycle operations: create the
// directory structure, inject runtime configuration, and clean up.
type WorkspaceManager struct {
	rootDir string // base path, e.g. $HOME/.solo/agents
}

// NewWorkspaceManager creates a WorkspaceManager rooted at the given
// base path. If basePath is empty it defaults to $HOME/.solo/agents.
func NewWorkspaceManager(basePath string) *WorkspaceManager {
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		basePath = filepath.Join(home, ".solo", "agents")
	}
	return &WorkspaceManager{rootDir: basePath}
}

// Prepare creates the Agent workspace directory structure and writes
// the initial configuration files. It is idempotent — calling it
// multiple times with the same agentID safely reuses existing directories.
//
// The created structure:
//
//	~/.solo/agents/<agentID>/
//	├── workspace/          # execution CWD for the agent subprocess
//	├── output/             # output artifacts directory
//	└── solo-config.json    # local cache of Agent configuration
func (wm *WorkspaceManager) Prepare(agentID string, config *AgentConfig) (*Workspace, error) {
	if agentID == "" {
		return nil, fmt.Errorf("workspace: agent ID is required")
	}

	rootDir := filepath.Join(wm.rootDir, agentID)
	workDir := filepath.Join(rootDir, "workspace")
	outputDir := filepath.Join(rootDir, "output")

	// Create directory structure (MkdirAll is idempotent).
	for _, dir := range []string{workDir, outputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("workspace: create directory %s: %w", dir, err)
		}
	}

	ws := &Workspace{
		RootDir:   rootDir,
		WorkDir:   workDir,
		OutputDir: outputDir,
	}

	// Write solo-config.json if config is provided.
	if config != nil {
		if err := writeConfig(rootDir, config); err != nil {
			return nil, fmt.Errorf("workspace: write config: %w", err)
		}
	}

	return ws, nil
}

// WorkspacePath returns the root directory path for the given agent.
func (wm *WorkspaceManager) WorkspacePath(agentID string) string {
	return filepath.Join(wm.rootDir, agentID)
}

// WorkspaceDir returns the WorkDir path for an agent.
func (wm *WorkspaceManager) WorkspaceDir(agentID string) string {
	return filepath.Join(wm.rootDir, agentID, "workspace")
}

// Cleanup removes the entire Agent workspace directory tree.
// It is safe to call on a non-existent workspace.
func (wm *WorkspaceManager) Cleanup(agentID string) error {
	if agentID == "" {
		return fmt.Errorf("workspace: agent ID is required")
	}
	return os.RemoveAll(wm.WorkspacePath(agentID))
}

// ---------------------------------------------------------------------------
// InjectRuntimeConfig (W1-03-BE)
// ---------------------------------------------------------------------------

// InjectConfig writes the runtime CLAUDE.md configuration into the agent's
// workspace directory before each execution. It overwrites the existing
// CLAUDE.md with the current channel context, trigger type, and agent
// identity, so the CLI agent (especially Claude Code) reads fresh context
// on every startup.
//
// The generated CLAUDE.md contains:
//   - Agent Identity (name, ID, system prompt)
//   - Solo Platform Rules (Solo communication and behavior protocol)
//   - Channel Context (channel name, ID, description, trigger type)
func (wm *WorkspaceManager) InjectConfig(ctx context.Context, agentID string, channelCtx *ChannelContext) error {
	if agentID == "" {
		return fmt.Errorf("workspace: agent ID is required")
	}

	// Ensure workspace directory exists.
	workDir := filepath.Join(wm.WorkspacePath(agentID), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("workspace: ensure workdir: %w", err)
	}

	// Ensure the workspace directory exists. The system prompt file
	// (.solo/system-prompt.md) is written by claude.go via --system-prompt-file.
	return nil
}

// SyncSoloSkillsForProvider copies Solo-managed skills into the runtime-specific
// skill directories inside an agent workspace.
func SyncSoloSkillsForProvider(skillsRoot, workDir, provider string) error {
	if skillsRoot == "" || workDir == "" {
		return nil
	}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("workspace: read skills root: %w", err)
	}
	targetRoots := skillTargetRoots(workDir, provider)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(skillsRoot, entry.Name())
		for _, targetRoot := range targetRoots {
			if err := copyDir(src, filepath.Join(targetRoot, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func skillTargetRoots(workDir, provider string) []string {
	agentsSkills := filepath.Join(workDir, ".agents", "skills")
	switch strings.ToLower(provider) {
	case "claude":
		return []string{filepath.Join(workDir, ".claude", "skills"), agentsSkills}
	case "codex":
		return []string{filepath.Join(workDir, ".codex", "skills"), agentsSkills}
	case "opencode":
		return []string{filepath.Join(workDir, ".opencode", "skills"), filepath.Join(workDir, ".claude", "skills"), agentsSkills}
	case "openclaw":
		return []string{filepath.Join(workDir, "skills"), agentsSkills}
	default:
		return []string{agentsSkills}
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// writeConfig writes the agent's solo-config.json.
func writeConfig(rootDir string, config *AgentConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(filepath.Join(rootDir, "solo-config.json"), data, 0o644)
}

// readConfig reads the solo-config.json for the given agent.
// Returns an empty AgentConfig (with AgentID set) if the file does not exist.
func readConfig(wm *WorkspaceManager, agentID string) (*AgentConfig, error) {
	path := filepath.Join(wm.WorkspacePath(agentID), "solo-config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentConfig{AgentID: agentID}, nil
		}
		return nil, err
	}
	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

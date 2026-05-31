package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentConfig is the local cache of Agent configuration stored in
// solo-config.json inside the agent's workspace root.
type AgentConfig struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	SystemPrompt string            `json:"system_prompt"`
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	MaxTokens    int               `json:"max_tokens"`
	Temperature  float64           `json:"temperature"`
	Env          map[string]string `json:"env,omitempty"`
	CustomArgs   []string          `json:"custom_args,omitempty"`
	// Runtime context for system prompt (Slock-aligned).
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

	// Write an initial CLAUDE.md only if one does not already exist.
	// This ensures agents created after the workspace already exists
	// (e.g. after a Daemon restart) still get a baseline config without
	// overwriting manual modifications.
	if err := writeInitialCLAUDE(workDir, config); err != nil {
		return nil, fmt.Errorf("workspace: write initial CLAUDE.md: %w", err)
	}

	return ws, nil
}

// WorkspacePath returns the root directory path for the given agent.
func (wm *WorkspaceManager) WorkspacePath(agentID string) string {
	return filepath.Join(wm.rootDir, agentID)
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

// writeInitialCLAUDE writes a baseline CLAUDE.md into the workspace directory
// if one does not already exist. This ensures agents have a starting
// configuration without overwriting user or agent modifications.
func writeInitialCLAUDE(workDir string, config *AgentConfig) error {
	path := filepath.Join(workDir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — do not overwrite
	}

	var b strings.Builder
	b.WriteString("# Solo Agent Configuration\n\n")

	if config != nil && config.Name != "" {
		b.WriteString("## Agent Identity\n\n")
		fmt.Fprintf(&b, "You are **%s**", config.Name)
		if config.AgentID != "" {
			fmt.Fprintf(&b, " (ID: `%s`)", config.AgentID)
		}
		b.WriteString(" on the Solo platform.\n\n")
	}

	if config != nil && config.SystemPrompt != "" {
		b.WriteString(config.SystemPrompt)
		b.WriteString("\n\n")
	}

	b.WriteString("## Solo Platform Rules\n\n")
	b.WriteString("- You participate in channel conversations as a team member.\n")
	b.WriteString("- You are triggered when a user sends a message or @mentions you.\n")
	b.WriteString("- Respond naturally and concisely using Markdown formatting.\n")
	b.WriteString("- Use MEMORY.md to store information you learn across conversations.\n")
	b.WriteString("- Update MEMORY.md when you learn important information about users, decisions, or facts.\n")
	b.WriteString("- Your workspace is at `./workspace/`; use this directory for file operations.\n")
	b.WriteString("- Output files go to `../output/` for the user to access.\n\n")

	b.WriteString("## Communication\n\n")
	b.WriteString("- Messages are delivered via stdin as structured JSON (stream-json format).\n")
	b.WriteString("- Your response is streamed back via stdout, token by token.\n")
	b.WriteString("- To call a tool, output a tool_use event. Tool results come back as tool_result events.\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// buildRuntimeCLAUDE generates the CLAUDE.md content for a specific

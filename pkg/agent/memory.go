package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/solo-ai/solo/pkg/llm"
)

// MemoryManager manages per-Agent MEMORY.md files within the agent workspace
// directory (~/.solo/agents/<agentID>/MEMORY.md). It supports loading,
// appending, and summarizing agent memories across conversations.
//
// The MEMORY.md file tracks:
//   - Preferences: user- or project-specific preferences learned over time
//   - Knowledge: facts, decisions, and context about the project
//   - Recent Conversations: dated entries summarizing key exchanges
//
// Each Append or Summarize call persists content into the MEMORY.md file so
// that it survives agent restarts and is available to BuildSystemPrompt on
// subsequent executions.
type MemoryManager struct {
	basePath string // root of the agent workspace, e.g. ~/.solo/agents
	mu       sync.Mutex
}

// NewMemoryManager creates a MemoryManager. If basePath is empty it defaults
// to $HOME/.solo/agents.
func NewMemoryManager(basePath string) *MemoryManager {
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		basePath = filepath.Join(home, ".solo", "agents")
	}
	return &MemoryManager{basePath: basePath}
}

// Load reads the MEMORY.md file for the given agent and returns its content
// as a string. Returns an empty string if the file does not exist or is empty.
// Callers should pass the returned string to BuildSystemPrompt as the
// memoryContent parameter.
func (m *MemoryManager) Load(agentID string) (string, error) {
	if agentID == "" {
		return "", fmt.Errorf("memory: agent ID is required")
	}

	path := m.memoryFilePath(agentID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("memory: read MEMORY.md for agent %s: %w", agentID, err)
	}

	content := strings.TrimSpace(string(data))
	return content, nil
}

// Append appends a new entry to the MEMORY.md file under the "Recent
// Conversations" section. The entry is timestamped and appended at the end
// of the file. If the MEMORY.md file does not yet exist, a default header
// is created first.
func (m *MemoryManager) Append(agentID string, entry string) error {
	if agentID == "" {
		return fmt.Errorf("memory: agent ID is required")
	}
	if entry == "" {
		return nil // nothing to append
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.memoryFilePath(agentID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memory: create directory: %w", err)
	}

	// Read existing content or build a new header.
	var content string
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("memory: read existing MEMORY.md: %w", err)
		}
		// File does not exist — create a new one with header.
		agentName := agentID
		content = fmt.Sprintf("# Agent Memory — %s\nLast updated: %s\n\n## Preferences\n\n(No preferences recorded yet.)\n\n## Knowledge\n\n(No knowledge recorded yet.)\n\n## Recent Conversations\n\n",
			agentName, time.Now().UTC().Format("2006-01-02"))
	} else {
		content = string(data)
		// Ensure the content ends with a newline before appending.
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
	}

	// Append the new entry under Recent Conversations.
	today := time.Now().UTC().Format("2006-01-02")
	entryLines := strings.Split(strings.TrimSpace(entry), "\n")
	formattedEntry := fmt.Sprintf("- %s: %s", today, entryLines[0])
	for _, line := range entryLines[1:] {
		formattedEntry += "\n  " + line
	}
	formattedEntry += "\n"

	content += formattedEntry

	// Update the "Last updated" timestamp.
	content = updateLastUpdated(content, time.Now().UTC())

	return os.WriteFile(path, []byte(content), 0o644)
}

// Summarize replaces the entire MEMORY.md content with a new summary
// produced by the agent (typically from an LLM call). The conversation
// parameter is the raw conversation transcript that the agent used to
// produce the summary.
//
// The expected summary format is a complete MEMORY.md document:
//
//	# Agent Memory — <agent-name>
//	Last updated: <date>
//
//	## Preferences
//	- ...
//
//	## Knowledge
//	- ...
//
//	## Recent Conversations
//	- ...
func (m *MemoryManager) Summarize(agentID string, summary string) error {
	if agentID == "" {
		return fmt.Errorf("memory: agent ID is required")
	}
	if summary == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.memoryFilePath(agentID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memory: create directory: %w", err)
	}

	// If the summary does not start with a level-1 heading, wrap it with a default header.
	content := strings.TrimSpace(summary)
	if !strings.HasPrefix(content, "# ") {
		var b strings.Builder
		fmt.Fprintf(&b, "# Agent Memory — %s\n", agentID)
		fmt.Fprintf(&b, "Last updated: %s\n\n", time.Now().UTC().Format("2006-01-02"))
		b.WriteString(content)
		content = b.String()
	}

	// Ensure the content ends with a newline.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	return os.WriteFile(path, []byte(content), 0o644)
}

// AutoSummarize automatically generates a MEMORY.md summary using an LLM
// provider. It only triggers when the conversation has more than 5 messages.
// It sends the current MEMORY.md content plus the conversation history to the
// LLM and replaces MEMORY.md with the generated summary.
//
// The summary format follows the standard MEMORY.md structure:
//   - Preferences: user- or project-specific preferences learned over time
//   - Knowledge: facts, decisions, and context about the project
//   - Recent Conversations: dated entries summarizing key exchanges
//
// The conversation parameter is the full conversation transcript (including
// system, user, and assistant messages).
func (m *MemoryManager) AutoSummarize(ctx context.Context, agentID string, conversation []Message, provider llm.Provider) error {
	if agentID == "" {
		return fmt.Errorf("memory: agent ID is required")
	}
	if provider == nil {
		return fmt.Errorf("memory: LLM provider is required")
	}

	// Only trigger summarization if the conversation has enough messages.
	const minMessagesForSummary = 5
	if len(conversation) <= minMessagesForSummary {
		return nil
	}

	// Load current MEMORY.md content.
	currentMemory, err := m.Load(agentID)
	if err != nil {
		return fmt.Errorf("memory: load existing memory for summarization: %w", err)
	}

	// Build the summarization prompt.
	prompt := buildAutoSummarizePrompt(currentMemory, conversation)

	// Call the LLM provider to generate a summary.
	resp, err := provider.Complete(ctx, &llm.CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		SystemPrompt: "You are a memory management assistant. Your task is to summarize agent conversations into structured memory entries.",
		Temperature: 0.3,
		MaxTokens:   4096,
	})
	if err != nil {
		return fmt.Errorf("memory: LLM summarization call failed: %w", err)
	}

	// Use the Summarize method to write the new content.
	return m.Summarize(agentID, resp.Content)
}

// buildAutoSummarizePrompt constructs the prompt for the LLM summarization
// call. It includes the current MEMORY.md content and the conversation history.
func buildAutoSummarizePrompt(currentMemory string, conversation []Message) string {
	var b strings.Builder

	b.WriteString("Please summarize the following agent conversation into a structured MEMORY.md file.\n\n")

	b.WriteString("## Format Requirements\n\n")
	b.WriteString("The output must follow this structure:\n\n")
	b.WriteString("# Agent Memory — <agent-name>\n")
	b.WriteString("Last updated: <date>\n\n")
	b.WriteString("## Preferences\n")
	b.WriteString("- List any user or project preferences learned from the conversation\n\n")
	b.WriteString("## Knowledge\n")
	b.WriteString("- List facts, decisions, and context learned about the project\n\n")
	b.WriteString("## Recent Conversations\n")
	b.WriteString("- <date>: Brief summary of key exchanges\n\n")

	if currentMemory != "" {
		b.WriteString("## Current Memory Content (to merge with new information)\n\n")
		b.WriteString(currentMemory)
		b.WriteString("\n\n")
	}

	b.WriteString("## Conversation to Summarize\n\n")
	for _, msg := range conversation {
		role := string(msg.Role)
		if role == "" {
			role = "unknown"
		}
		fmt.Fprintf(&b, "### %s\n\n", role)
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("Generate the updated MEMORY.md content now. Include the full memory structure with Preferences, Knowledge, and Recent Conversations sections. Preserve important information from the current memory while incorporating new insights from the conversation.")

	return b.String()
}

// Delete removes the MEMORY.md file for the given agent.
// It is safe to call on a non-existent file.
func (m *MemoryManager) Delete(agentID string) error {
	if agentID == "" {
		return fmt.Errorf("memory: agent ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.memoryFilePath(agentID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("memory: delete MEMORY.md for agent %s: %w", agentID, err)
	}
	return nil
}

// memoryFilePath returns the absolute path to the agent's MEMORY.md file.
func (m *MemoryManager) memoryFilePath(agentID string) string {
	return filepath.Join(m.basePath, agentID, "workspace", "MEMORY.md")
}

// updateLastUpdated replaces or appends the "Last updated:" line in the
// MEMORY.md content. It looks for an existing "Last updated:" line and
// replaces it; if none is found it appends one after the title heading.
func updateLastUpdated(content string, t time.Time) string {
	formatted := t.Format("2006-01-02")
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Last updated:") {
			lines[i] = "Last updated: " + formatted
			return strings.Join(lines, "\n")
		}
	}

	// No existing "Last updated" line — insert after first heading.
	insertAt := 1
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") && i+1 < len(lines) {
			insertAt = i + 1
			break
		}
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, "Last updated: "+formatted)
	newLines = append(newLines, lines[insertAt:]...)

	return strings.Join(newLines, "\n")
}

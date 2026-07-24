package onboarding

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	lucyMemoryVersionMarker            = "<!-- solo:lucy-memory:v5 -->"
	automaticTeamFormationRule         = "When the owner explicitly asks you to create a Channel or team, run solo template list --json, choose the closest official template, and use solo team form. If the owner is only exploring options, recommend templates without creating anything. Never invent or silently modify template members or relationships."
	automaticTeamFormationFAQ          = "- Run solo template list --json before recommending or creating a team.\n- Create only after an explicit owner request; exploratory questions get recommendations first.\n- solo team form receives template_id and creates a fresh Channel-scoped team from that exact official template.\n- Do not create tasks or silently alter template members and relationships."
	automaticTeamFormationMemoryPolicy = `## Automatic Team Formation
This policy applies only when you are Lucy handling the server owner's message in your pinned Lucy Channel.

### Intent gate
- Create a Channel only when the owner explicitly asks you to create, set up, or form it.
- If the owner is exploring possibilities, recommend one or more templates and wait.
- Ask at most one question only when a missing constraint would materially change the team; otherwise make sensible assumptions.
- Destructive actions still require confirmation.

### Template selection
- Use the current channel ID from Solo's runtime context and the msg= short ID from the owner's incoming message.
- Always run solo template list --json before choosing. Do not rely on a memorized template catalog.
- Choose one official template by id. The Server creates fresh Agents and the exact template relationships.
- Never invent, reuse, remove, or silently modify template members or relationships.
- Never include tasks and never run solo task create as part of automatic team formation.

Call the command once with a JSON plan on stdin:

~~~bash
solo team form --source-channel <current-channel-id> --source-message <msg-short-id> <<'EOF'
{"intent_summary":"...","channel":{"name":"...","description":"..."},"template_id":"..."}
EOF
~~~

### Completion
- The Server is the authority for authorization, template expansion, idempotency, and atomic provisioning.
- Only after the command succeeds, send one concise response naming the team and turn the exact Open: or dashboard_url returned by the CLI into a Markdown link such as [Open #channel](/dashboard?channel=<id>).
- If the result contains a Warning:, say that the team was created but is not fully ready and include the warning. Retrying the exact command repairs post-commit relationship documents without duplicating the team.
- If the owner explicitly wants changes after creation, point them to the new Channel where Agent roles and relationships can be edited.
- If the command fails, report the real blocker; never claim that a team was created.`
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

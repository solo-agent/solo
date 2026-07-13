package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	lucyPlaybookVersionMarker        = "<!-- solo:lucy-onboarding-playbook:v3 -->"
	lucyFAQVersionMarker             = "<!-- solo:lucy-onboarding-faq:v3 -->"
	legacyLucyPlaybookVersionMarker  = "<!-- solo:lucy-onboarding-playbook:v2 -->"
	legacyLucyFAQVersionMarker       = "<!-- solo:lucy-onboarding-faq:v2 -->"
	legacyManualSetupRule            = "For new channels and agents, tell users to use the + buttons in the sidebar. Suggest names and descriptions based on their work context."
	legacyAutomaticTeamFormationRule = "When the user gives a sufficiently clear goal, infer a compact specialist team and use the solo team form command to provision it in a new channel. Do not make the user manually pick roles or use sidebar + buttons. Ask at most one blocking question; otherwise make reasonable assumptions. Only claim success after the command returns successfully."
	automaticTeamFormationRule       = "When the user gives a sufficiently clear goal, infer a compact specialist team and use the solo team form command to provision it in a new channel. Choose the closest official relationship template (dev-team, content-team, or research-team); keep its base relationships unless a minimal, reasoned override is materially better. Do not create initial tasks automatically. Ask at most one blocking question; otherwise make reasonable assumptions. Only claim success after the command returns successfully."
	legacyManualSetupFAQ             = "- Use the + buttons in the Agents and Channels sidebar sections.\n- Walk users through step by step. Suggest names/descriptions based on their context."
	legacyAutomaticTeamFormationFAQ  = "- In the onboarding channel, a clear work intent can be turned into a ready-to-use team automatically.\n- Infer complementary roles, create a new channel, add the agents and kickoff tasks, then return the exact channel link.\n- Use manual + buttons only when the user explicitly wants manual setup or fine-grained editing."
	automaticTeamFormationFAQ        = "- In the onboarding channel, a clear work intent can be turned into a ready-to-use team automatically.\n- Infer complementary roles, choose the closest official relationship template, create a new channel, and add the agents with template-first relationships.\n- Do not create initial tasks automatically; create tasks only after scope and ownership are agreed.\n- Use manual + buttons only when the user explicitly wants manual setup or fine-grained editing."
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

// UpgradeExistingLucyKnowledge updates only Solo-owned, known onboarding text
// in existing Lucy workspaces. It deliberately avoids replacing the whole
// playbook so user-authored notes survive upgrades.
func UpgradeExistingLucyKnowledge(ctx context.Context, pool *pgxpool.Pool, workspaceRoot string) error {
	if pool == nil {
		return nil
	}
	rows, err := pool.Query(ctx, `
		SELECT id::text
		  FROM agents
		 WHERE is_active = true AND lower(name) = 'lucy'`)
	if err != nil {
		return fmt.Errorf("query existing Lucy agents: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return fmt.Errorf("scan existing Lucy agent: %w", err)
		}
		changed, err := UpgradeLucyKnowledge(workspaceRoot, agentID)
		if err != nil {
			slog.Warn("onboarding: failed to upgrade Lucy knowledge", "agent_id", agentID, "error", err)
			continue
		}
		if changed {
			slog.Info("onboarding: upgraded Lucy knowledge", "agent_id", agentID, "version", 3)
		}
	}
	return rows.Err()
}

// UpgradeLucyKnowledge applies the current automatic-team rule to one existing
// workspace. The exact legacy sentence is the migration anchor; unknown custom
// content is left untouched.
func UpgradeLucyKnowledge(workspaceRoot, agentID string) (bool, error) {
	notesRoot := filepath.Join(workspaceRoot, agentID, "workspace", "notes")
	playbookChanged, err := upgradeLucyKnowledgeFile(
		filepath.Join(notesRoot, "onboarding_playbook.md"),
		lucyPlaybookVersionMarker,
		[]string{legacyLucyPlaybookVersionMarker},
		[]string{legacyManualSetupRule, legacyAutomaticTeamFormationRule},
		automaticTeamFormationRule,
		"# Lucy Onboarding Playbook",
	)
	if err != nil {
		return false, err
	}
	faqChanged, err := upgradeLucyKnowledgeFile(
		filepath.Join(notesRoot, "onboarding_knowledge_faq.md"),
		lucyFAQVersionMarker,
		[]string{legacyLucyFAQVersionMarker},
		[]string{legacyManualSetupFAQ, legacyAutomaticTeamFormationFAQ},
		automaticTeamFormationFAQ,
		"# Lucy Onboarding Knowledge FAQ",
	)
	if err != nil {
		return false, err
	}
	return playbookChanged || faqChanged, nil
}

func upgradeLucyKnowledgeFile(path, versionMarker string, legacyMarkers, legacyTexts []string, currentText, heading string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read onboarding knowledge file: %w", err)
	}
	content := string(data)
	if strings.Contains(content, versionMarker) {
		return false, nil
	}

	updated := content
	foundOwnedText := strings.Contains(updated, currentText)
	if !foundOwnedText {
		for _, legacyText := range legacyTexts {
			if strings.Contains(updated, legacyText) {
				updated = strings.Replace(updated, legacyText, currentText, 1)
				foundOwnedText = true
				break
			}
		}
	}
	if !foundOwnedText {
		return false, nil
	}
	for _, legacyMarker := range legacyMarkers {
		updated = strings.ReplaceAll(updated, legacyMarker, "")
	}
	updated = strings.Replace(updated, heading, heading+"\n\n"+versionMarker, 1)
	if updated == content {
		return false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat onboarding knowledge file: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".onboarding-knowledge-*")
	if err != nil {
		return false, fmt.Errorf("create onboarding knowledge temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return false, fmt.Errorf("chmod onboarding knowledge temp file: %w", err)
	}
	if _, err := tmp.WriteString(updated); err != nil {
		tmp.Close()
		return false, fmt.Errorf("write onboarding knowledge temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close onboarding knowledge temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return false, fmt.Errorf("replace onboarding knowledge file: %w", err)
	}
	return true, nil
}

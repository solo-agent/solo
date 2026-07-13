package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeLucyKnowledgeReplacesOnlyLegacyRule(t *testing.T) {
	root := t.TempDir()
	agentID := "lucy-1"
	path := filepath.Join(root, agentID, "workspace", "notes", "onboarding_playbook.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# Lucy Onboarding Playbook\n\n" + legacyManualSetupRule + "\n\nCustom owner note.\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	faqPath := filepath.Join(root, agentID, "workspace", "notes", "onboarding_knowledge_faq.md")
	faqOriginal := "# Lucy Onboarding Knowledge FAQ\n\n## FAQ 15\n" + legacyManualSetupFAQ + "\n\nCustom FAQ note.\n"
	if err := os.WriteFile(faqPath, []byte(faqOriginal), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := UpgradeLucyKnowledge(root, agentID)
	if err != nil || !changed {
		t.Fatalf("UpgradeLucyKnowledge() = %v, %v; want true, nil", changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{lucyPlaybookVersionMarker, automaticTeamFormationRule, "Custom owner note."} {
		if !strings.Contains(got, want) {
			t.Fatalf("upgraded playbook does not contain %q: %s", want, got)
		}
	}
	if strings.Contains(got, legacyManualSetupRule) {
		t.Fatalf("legacy rule remains after upgrade: %s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
	faqData, err := os.ReadFile(faqPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{lucyFAQVersionMarker, automaticTeamFormationFAQ, "Custom FAQ note."} {
		if !strings.Contains(string(faqData), want) {
			t.Fatalf("upgraded FAQ does not contain %q: %s", want, faqData)
		}
	}
	if strings.Contains(string(faqData), legacyManualSetupFAQ) {
		t.Fatalf("legacy FAQ remains after upgrade: %s", faqData)
	}
	if strings.Contains(string(faqData), "kickoff tasks") {
		t.Fatalf("FAQ still promises automatic kickoff tasks: %s", faqData)
	}

	changed, err = UpgradeLucyKnowledge(root, agentID)
	if err != nil || changed {
		t.Fatalf("second UpgradeLucyKnowledge() = %v, %v; want false, nil", changed, err)
	}
}

func TestUpgradeLucyKnowledgeUpgradesV2AutomaticTeamRule(t *testing.T) {
	root := t.TempDir()
	agentID := "lucy-v2"
	notes := filepath.Join(root, agentID, "workspace", "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	playbookPath := filepath.Join(notes, "onboarding_playbook.md")
	playbook := "# Lucy Onboarding Playbook\n\n" + legacyLucyPlaybookVersionMarker + "\n\n" + legacyAutomaticTeamFormationRule + "\n\nCustom v2 note.\n"
	if err := os.WriteFile(playbookPath, []byte(playbook), 0o644); err != nil {
		t.Fatal(err)
	}
	faqPath := filepath.Join(notes, "onboarding_knowledge_faq.md")
	faq := "# Lucy Onboarding Knowledge FAQ\n\n" + legacyLucyFAQVersionMarker + "\n\n" + legacyAutomaticTeamFormationFAQ + "\n"
	if err := os.WriteFile(faqPath, []byte(faq), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := UpgradeLucyKnowledge(root, agentID)
	if err != nil || !changed {
		t.Fatalf("UpgradeLucyKnowledge() = %v, %v; want true, nil", changed, err)
	}
	for path, wants := range map[string][]string{
		playbookPath: {lucyPlaybookVersionMarker, automaticTeamFormationRule, "Custom v2 note."},
		faqPath:      {lucyFAQVersionMarker, automaticTeamFormationFAQ},
	} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		got := string(data)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("upgraded %s does not contain %q: %s", path, want, got)
			}
		}
		if strings.Contains(got, ":v2 -->") || strings.Contains(got, "kickoff tasks") {
			t.Fatalf("v2 behavior remains in %s: %s", path, got)
		}
	}
}

func TestKnowledgeFilesUseTemplateFirstRelationshipsWithoutInitialTasks(t *testing.T) {
	files := KnowledgeFiles("Owner", "owner@example.com")
	playbook := files["notes/onboarding_playbook.md"]
	faq := files["notes/onboarding_knowledge_faq.md"]
	for _, want := range []string{lucyPlaybookVersionMarker, "dev-team", "Do not create initial tasks automatically"} {
		if !strings.Contains(playbook, want) {
			t.Fatalf("playbook does not contain %q", want)
		}
	}
	for _, want := range []string{lucyFAQVersionMarker, "template-first relationships", "Do not create initial tasks automatically"} {
		if !strings.Contains(faq, want) {
			t.Fatalf("FAQ does not contain %q", want)
		}
	}
}

func TestUpgradeLucyKnowledgePreservesUnknownCustomPlaybook(t *testing.T) {
	root := t.TempDir()
	agentID := "lucy-custom"
	path := filepath.Join(root, agentID, "workspace", "notes", "onboarding_playbook.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# Lucy Onboarding Playbook\n\nEntirely custom workflow.\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := UpgradeLucyKnowledge(root, agentID)
	if err != nil || changed {
		t.Fatalf("UpgradeLucyKnowledge() = %v, %v; want false, nil", changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("custom playbook changed: %q", data)
	}
}

package onboarding

import (
	"strings"
	"testing"
)

func TestLucyKnowledgeUsesRuntimeTemplateCatalog(t *testing.T) {
	files := KnowledgeFiles("Owner", "owner@example.com")
	combined := files["MEMORY.md"] + files["notes/onboarding_playbook.md"] + files["notes/onboarding_knowledge_faq.md"]
	for _, required := range []string{
		"solo template list --json",
		`"template_id":"..."`,
		"explicitly asks",
		"Never invent",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("Lucy knowledge missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`"relationship_template"`,
		`"relationship_overrides"`,
		`"existing_agent_id"`,
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("Lucy knowledge still contains legacy field %q", forbidden)
		}
	}
}

package agent

import (
	"testing"
	"unicode/utf8"
)

func TestRoleTemplateCharCounts(t *testing.T) {
	for _, tmpl := range RoleTemplateList() {
		n := utf8.RuneCountInString(tmpl.Prompt)
		if n < 200 || n > 700 {
			t.Errorf("%s: prompt has %d chars, want 200-700", tmpl.Key, n)
		} else {
			t.Logf("%-10s %-12s  %3d chars  OK", tmpl.Key, tmpl.DisplayName, n)
		}
	}
}

func TestRoleTemplateListOrder(t *testing.T) {
	list := RoleTemplateList()
	expected := []string{"leader", "pm", "rd", "fe", "qa"}
	if len(list) != 5 {
		t.Fatalf("expected 5 templates, got %d", len(list))
	}
	for i, tmpl := range list {
		if tmpl.Key != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], tmpl.Key)
		}
	}
}

func TestRoleTemplateAllFields(t *testing.T) {
	for _, tmpl := range RoleTemplateList() {
		if tmpl.Key == "" {
			t.Error("template has empty Key")
		}
		if tmpl.DisplayName == "" {
			t.Errorf("%s: empty DisplayName", tmpl.Key)
		}
		if tmpl.Description == "" {
			t.Errorf("%s: empty Description", tmpl.Key)
		}
		if tmpl.Prompt == "" {
			t.Errorf("%s: empty Prompt", tmpl.Key)
		}
	}
}

package main

import (
	"regexp"
	"testing"
)

// TestClaimRegex validates the claim protocol regex pattern used by the agent
// service to detect inline task claim directives (/claim #N).
func TestClaimRegex(t *testing.T) {
	re := regexp.MustCompile(`(?i)/(?:claim)\s+#(\d+)`)
	tests := []struct {
		input  string
		match  bool
		number string
	}{
		{"/claim #1", true, "1"},
		{"/CLAIM #5", true, "5"},
		{"/Claim #10", true, "10"},
		{"/claim #42", true, "42"},
		{"no claim", false, ""},
		{"/claim", false, ""},
		{"/claim #", false, ""},
		{"claim #1", false, ""},
		{"some /claim #7 text", true, "7"},
	}
	for _, tt := range tests {
		m := re.FindStringSubmatch(tt.input)
		if tt.match && m == nil {
			t.Errorf("expected match for %q", tt.input)
		}
		if tt.match && m != nil && m[1] != tt.number {
			t.Errorf("expected number %q, got %q for %q", tt.number, m[1], tt.input)
		}
		if !tt.match && m != nil {
			t.Errorf("expected no match for %q, got %v", tt.input, m)
		}
	}
}

// TestUpdateRegex validates the update protocol regex pattern used by the agent
// service to detect inline task status update directives (/done #N, /review #N, etc.).
func TestUpdateRegex(t *testing.T) {
	re := regexp.MustCompile(`(?i)/(done|review|progress|in_progress|todo)\s+#(\d+)`)
	tests := []struct {
		input string
		match bool
		cmd   string
	}{
		{"/done #1", true, "done"},
		{"/review #3", true, "review"},
		{"/progress #5", true, "progress"},
		{"/in_progress #6", true, "in_progress"},
		{"/todo #7", true, "todo"},
		{"/DONE #9", true, "DONE"},
		{"/Review #10", true, "Review"},
		{"no command", false, ""},
		{"/done", false, ""},
		{"/done #", false, ""},
		{"done #1", false, ""},
	}
	for _, tt := range tests {
		m := re.FindStringSubmatch(tt.input)
		if tt.match && m == nil {
			t.Errorf("expected match for %q", tt.input)
		}
		if tt.match && m != nil && m[1] != tt.cmd {
			t.Errorf("expected cmd %q, got %q for %q", tt.cmd, m[1], tt.input)
		}
		if !tt.match && m != nil {
			t.Errorf("expected no match for %q, got %v", tt.input, m)
		}
	}
}

// TestClaimRegexMultipleMatches verifies that FindAllStringSubmatch picks up
// multiple claim directives in the same output.
func TestClaimRegexMultipleMatches(t *testing.T) {
	re := regexp.MustCompile(`(?i)/(?:claim)\s+#(\d+)`)
	text := "I'll handle /claim #1 and then look at /claim #3 after that."
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0][1] != "1" {
		t.Errorf("expected first match number 1, got %q", matches[0][1])
	}
	if matches[1][1] != "3" {
		t.Errorf("expected second match number 3, got %q", matches[1][1])
	}
}

// TestUpdateRegexMultipleMatches verifies that FindAllStringSubmatch picks up
// multiple update directives in the same output.
func TestUpdateRegexMultipleMatches(t *testing.T) {
	re := regexp.MustCompile(`(?i)/(done|review|progress|in_progress|todo)\s+#(\d+)`)
	text := "/done #1 and /review #2 are complete."
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0][1] != "done" || matches[0][2] != "1" {
		t.Errorf("expected first match done/1, got %s/%s", matches[0][1], matches[0][2])
	}
	if matches[1][1] != "review" || matches[1][2] != "2" {
		t.Errorf("expected second match review/2, got %s/%s", matches[1][1], matches[1][2])
	}
}

package agent

import "testing"

func TestSessionConfigInvalidatesChangedCapabilities(t *testing.T) {
	base := AgentConfig{Model: "model", Provider: "provider", SystemPrompt: "role", RelationshipsMarkdown: "team", CustomArgs: []string{"--flag"}}
	entry := &agentSessionEntry{AgentConfig: base}
	if !sessionConfigMatches(entry, base) {
		t.Fatal("unchanged configuration rejected")
	}
	for _, change := range []func(*AgentConfig){
		func(c *AgentConfig) { c.SkillsDigest = "new-skills" },
		func(c *AgentConfig) { c.TeamVersionID = "other-team" },
		func(c *AgentConfig) { c.AgentRevisionID = "other-revision" },
		func(c *AgentConfig) { c.SystemPrompt = "new role" },
		func(c *AgentConfig) { c.RelationshipsMarkdown = "new team" },
		func(c *AgentConfig) { c.Provider = "other provider" },
		func(c *AgentConfig) { c.CustomArgs = []string{"--other"} },
	} {
		next := base
		change(&next)
		if sessionConfigMatches(entry, next) {
			t.Fatalf("changed configuration reused: %+v", next)
		}
	}
}

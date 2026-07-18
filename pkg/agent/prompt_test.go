package agent

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_Identity(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-1", Name: "TestBot", SystemPrompt: "You are a test bot."},
		ChannelContext{ChannelID: "chan-1", ChannelName: "general", TriggerType: TriggerChat},
		"", nil,
	)
	assertHas(t, p, "Who you are")
	assertHas(t, p, "TestBot")
	assertHas(t, p, "agent-1")
	assertHas(t, p, "You are a test bot.")
	assertHas(t, p, "#general")
	assertNotHas(t, p, "## Thinking Runtime")
}

func TestBuildSystemPrompt_ThinkingRuntimeIsSeparateFromInitialRole(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "TestBot", SystemPrompt: "You lead the team.", ThinkingRuntimePrompt: "You are in node abc."},
		ChannelContext{TriggerType: TriggerChat}, "", nil,
	)
	initial := strings.Index(p, "## Initial role\n\nYou lead the team.")
	thinking := strings.Index(p, "## Thinking Runtime\n\nYou are in node abc.")
	if initial < 0 || thinking < initial {
		t.Fatalf("Thinking Runtime must be a section after Initial role:\n%s", p)
	}
}

func TestBuildSystemPrompt_RuntimeContext(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-2", Name: "Bot", WorkspacePath: "/tmp/ws",
			ServerID: "https://solo.example.com", Hostname: "my-mac.local", OS: "darwin arm64"},
		ChannelContext{TriggerType: TriggerChat},
		"", nil,
	)
	assertHas(t, p, "Runtime Context")
	assertHas(t, p, "Agent ID: agent-2")
	assertHas(t, p, "Workspace: /tmp/ws")
	assertHas(t, p, "Handle: @Bot")
	assertHas(t, p, "Server ID: https://solo.example.com")
	assertHas(t, p, "Hostname: my-mac.local")
	assertHas(t, p, "OS: darwin arm64")
}

func TestBuildSystemPrompt_CRITICALRULES(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "CRITICAL RULES")
	assertHas(t, p, "only output channel")
	assertHas(t, p, "Do not combine multiple")
	assertHas(t, p, "If you are coordinating others")
}

func TestBuildSystemPrompt_DoesNotEmbedLucyOnboardingPolicy(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "Lucy"},
		ChannelContext{ChannelID: "channel-1", ChannelName: "welcome-owner", TriggerType: TriggerChat},
		"", nil,
	)
	assertNotHas(t, p, "## Automatic Team Formation")
	assertNotHas(t, p, `"relationship_template":"dev-team"`)
}

func TestBuildSystemPrompt_StartupSequence(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Startup sequence")
	assertHas(t, p, "Read RELATIONSHIPS.md")
	assertHas(t, p, "Read MEMORY.md")
	assertHas(t, p, "stop and wait")
	assertHas(t, p, "Complete ALL your work before stopping")
}

func TestBuildSystemPrompt_RelationshipsBeforeMessaging(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "Bot", WorkspacePath: "/tmp/bot-workspace"},
		ChannelContext{TriggerType: TriggerChat},
		"", nil,
	)
	assertHas(t, p, "Agent Relationships — CHECK BEFORE ACTING")
	assertHas(t, p, "cat /tmp/bot-workspace/RELATIONSHIPS.md")
	assertHas(t, p, "Re-read it before processing any task")
	assertNotHas(t, p, "work independently")

	relationships := strings.Index(p, "## Agent Relationships")
	messaging := strings.Index(p, "## Messaging")
	if relationships < 0 || messaging < 0 || relationships > messaging {
		t.Fatalf("expected relationships section before messaging")
	}
}

func TestBuildSystemPrompt_CLICommands(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Communication — solo CLI ONLY")
	assertHas(t, p, "solo task list")
	assertHas(t, p, "solo task claim")
	assertHas(t, p, "solo task submit")
	assertHas(t, p, "solo task accept")
	assertHas(t, p, "solo task reject")
	assertHas(t, p, "solo task create")
	assertHas(t, p, "solo task unclaim")
	assertHas(t, p, "solo message send")
	assertHas(t, p, "solo message read")
	assertHas(t, p, "solo message check")
	assertHas(t, p, "solo channel members")
	assertHas(t, p, "solo server info")
	assertHas(t, p, "solo thread unfollow")
	assertHas(t, p, "solo channel join")
	assertHas(t, p, "only output channel")
}

func TestBuildSystemPrompt_MentionedNames(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "Bot"},
		ChannelContext{ChannelName: "general", TriggerType: TriggerMention},
		"", []string{"Bot", "OtherBot"},
	)
	assertHas(t, p, "@mentioned: @Bot, @OtherBot")
	assertHas(t, p, "You WERE @mentioned")
}

func TestBuildSystemPrompt_MentionedNames_NotMe(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "Bot"},
		ChannelContext{ChannelName: "general", TriggerType: TriggerChat},
		"", []string{"OtherBot"},
	)
	assertHas(t, p, "@mentioned: @OtherBot")
	assertHas(t, p, "NOT @mentioned")
}

func TestBuildSystemPrompt_DM(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{Name: "DMBot"},
		ChannelContext{ChannelID: "dm-1", ChannelName: "dm", TriggerType: TriggerDM},
		"", nil,
	)
	assertHas(t, p, "Private DM")
	assertHas(t, p, "one-on-one")
}

func TestBuildSystemPrompt_Thread(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{},
		ChannelContext{ChannelID: "thread-1", TriggerType: TriggerThread},
		"", nil,
	)
	assertHas(t, p, "Thread reply")
	assertHas(t, p, "thread context")
}

func TestBuildSystemPrompt_Mention(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-2", Name: "MentionBot"},
		ChannelContext{ChannelID: "chan-1", ChannelName: "general", TriggerType: TriggerMention},
		"", nil,
	)
	assertHas(t, p, "@mentioned")
	assertHas(t, p, "Respond directly")
}

func TestBuildSystemPrompt_WithMemory(t *testing.T) {
	memory := "User prefers concise answers."
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-1", Name: "MemoryBot"},
		ChannelContext{TriggerType: TriggerChat},
		memory, nil,
	)
	assertHas(t, p, "Your Saved Memory")
	assertHas(t, p, "User prefers concise answers")
}

func TestBuildSystemPrompt_NoMemory(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-1", Name: "NoMemoryBot"},
		ChannelContext{TriggerType: TriggerChat},
		"", nil,
	)
	assertHas(t, p, "No Prior Memory")
	assertHas(t, p, "Start building MEMORY.md")
}

func TestBuildSystemPrompt_TaskWorkflow(t *testing.T) {
	p := BuildSystemPrompt(
		AgentConfig{AgentID: "agent-1", Name: "TaskBot"},
		ChannelContext{TriggerType: TriggerChat},
		"", nil,
	)
	assertHas(t, p, "Decision rule")
	assertHas(t, p, "Lifecycle")
	assertHas(t, p, "in_progress")
	assertHas(t, p, "in_review")
	assertHas(t, p, "done")
	assertHas(t, p, "solo task submit")
	assertHas(t, p, "solo task create")
}

func TestBuildSystemPrompt_Etiquette(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Conversation etiquette")
	assertHas(t, p, "don't echo or summarize")
	assertHas(t, p, "Do NOT prefix messages with your own @name")
	assertHas(t, p, "Do NOT send confirmation messages")
	assertHas(t, p, "Skip idle narration")
}

func TestBuildSystemPrompt_Workspace(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Workspace & Memory")
	assertHas(t, p, "MEMORY.md")
	assertHas(t, p, "Compaction safety")
	assertHas(t, p, "What to memorize")
	assertHas(t, p, "How to organize memory")
	assertNotHas(t, p, "Deliverable: ./path/to/result.html")
}

func TestBuildSystemPrompt_AllTriggers(t *testing.T) {
	for _, trigger := range []TriggerType{TriggerChat, TriggerMention, TriggerDM, TriggerThread} {
		t.Run(string(trigger), func(t *testing.T) {
			p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: trigger}, "", nil)
			if !strings.Contains(p, "Instructions") {
				t.Errorf("trigger %s: expected Instructions", trigger)
			}
		})
	}
}

func TestBuildSystemPrompt_MessageFormat(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Messaging")
	assertHas(t, p, "target=#general")
	assertHas(t, p, "time=")
	assertHas(t, p, "msg=")
	assertHas(t, p, "type=")
}

func TestBuildSystemPrompt_MessageNotifications(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Message Notifications")
	assertHas(t, p, "inbox")
	assertHas(t, p, "message check")
}

func TestBuildSystemPrompt_Formatting(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Formatting — Mentions & Channel Refs")
	assertHas(t, p, "Formatting — URLs in non-English text")
}

func TestBuildSystemPrompt_Capabilities(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Capabilities")
	assertHas(t, p, "not confined to any directory")
}

func TestBuildSystemPrompt_SplittingTasks(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Splitting tasks for parallel execution")
	assertHas(t, p, "Group by phase")
	assertHas(t, p, "Prefer independent subtasks")
}

func TestBuildSystemPrompt_Threads(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Threads are sub-conversations")
	assertHas(t, p, "cannot be nested")
	assertHas(t, p, "thread unfollow")
}

func TestBuildSystemPrompt_ChannelAwareness(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "Channel awareness")
	assertHas(t, p, "Stay on topic")
	assertHas(t, p, "Reply in context")
}

func TestBuildSystemPrompt_MentionsSection(t *testing.T) {
	p := BuildSystemPrompt(AgentConfig{Name: "Bot"}, ChannelContext{TriggerType: TriggerChat}, "", nil)
	assertHas(t, p, "@Mentions")
	assertHas(t, p, "Mention others, not yourself")
}

func assertHas(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q", substr)
	}
}

func assertNotHas(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("did not expect %q", substr)
	}
}

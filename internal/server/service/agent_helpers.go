package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

func (s *AgentService) resultReminderMessage(ctx context.Context, run *AgentRun) (agent.Message, error) {
	if run.ChannelID == "" {
		return agent.Message{}, fmt.Errorf("visible-result reminder has no Run channel")
	}
	target, rootMessageID := run.ChannelID, ""
	if run.ThreadID != "" {
		if err := s.pool.QueryRow(ctx, `SELECT root_message_id::text FROM threads WHERE id=$1 AND channel_id=$2`, run.ThreadID, run.ChannelID).Scan(&rootMessageID); err != nil {
			return agent.Message{}, fmt.Errorf("resolve visible-result reminder thread: %w", err)
		}
		target += ":" + rootMessageID
	}
	content := fmt.Sprintf("[target=%s msg=%s type=system]\n%s\nUse solo message send --target '%s' with the result as its message body. This is the current Run's target; ignore earlier introduction or Session targets.", target, rootMessageID, agentResultReminderPrompt, target)
	if run.ThinkingNodeID != "" {
		content += fmt.Sprintf("\nRemain in the current Thinking node %s; its identity is already bound by the Runtime.", run.ThinkingNodeID)
	}
	return agent.Message{Role: agent.RoleUser, Content: content, SenderID: "system"}, nil
}

// resolveMentionedNames resolves agent IDs to their display names.
func (s *AgentService) resolveMentionedNames(ctx context.Context, agentIDs []string) []string {
	if len(agentIDs) == 0 {
		return nil
	}
	names := make([]string, 0, len(agentIDs))
	for _, id := range agentIDs {
		var name string
		err := s.pool.QueryRow(ctx,
			`SELECT name FROM agents WHERE id = $1`,
			id,
		).Scan(&name)
		if err != nil {
			slog.Warn("failed to resolve agent name", "agent_id", id, "error", err)
			continue
		}
		names = append(names, name)
	}
	return names
}

// TriggerAgentGreeting sends a greeting prompt to a newly-joined agent,
// asking it to introduce itself in the channel. If greeting is non-empty,
// it is used as the agent's private context instead of the generic prompt.
func (s *AgentService) TriggerAgentGreeting(ctx context.Context, channelID, agentID string, greeting string) {
	// Get agent info
	var ag agentChannelInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, model_provider, model_name, system_prompt
		 FROM agents WHERE id = $1 AND is_active = true`,
		agentID,
	).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName,
		&ag.SystemPrompt)
	if err != nil {
		slog.Error("TriggerAgentGreeting: failed to get agent info", "agent_id", agentID, "error", err)
		return
	}

	// Get channel name
	var channelName string
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(name, '') FROM channels WHERE id = $1`,
		channelID,
	).Scan(&channelName)

	// Build greeting content — use custom greeting if provided, otherwise generic.
	greetingContent := greeting
	if greetingContent == "" {
		greetingContent = fmt.Sprintf(
			"You have just joined the channel #%s. This is your first time here.\n\n"+
				"Please introduce yourself briefly in the channel:\n"+
				"- Say hi and state your name (@%s)\n"+
				"- Briefly describe your role and how you can help\n"+
				"- Keep it short and friendly\n\n"+
				"Use `solo message send` to post your introduction to the channel.",
			channelName, ag.Name,
		)
	}
	greetingContent += fmt.Sprintf(
		"\n\nSend this greeting to the exact Channel target `%s`. Use that UUID as the `solo message send --target` value; do not replace it with a Channel name or a message short ID.",
		channelID,
	)

	taskReq := daemonTaskRequest{
		AgentID:   ag.ID,
		ChannelID: channelID,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: greetingContent, SenderID: ""},
		},
		SystemPrompt:   ag.SystemPrompt,
		ResultContract: agentResultContractVisibleMessage,
		ModelConfig: agent.ModelConfig{
			Provider: ag.ModelProvider,
			Model:    ag.ModelName,
		},
	}

	slog.Info("triggering agent greeting",
		"agent_id", ag.ID,
		"agent_name", ag.Name,
		"channel_id", channelID,
	)

	if err := s.enqueueAgentWork(ctx, taskReq); err != nil {
		slog.Warn("queue Agent greeting", "agent_id", ag.ID, "error", err)
	}
}

// BroadcastMemberEvent broadcasts a member.added / member.removed event to the
// channel so the frontend can refetch the member list in real time.
func (s *AgentService) BroadcastMemberEvent(channelID, eventType, memberType, memberID, memberName string) {
	payload := map[string]interface{}{
		"channel_id":  channelID,
		"member_type": memberType,
		"member_id":   memberID,
		"member_name": memberName,
	}
	s.hub.BroadcastToChannel(channelID, realtime.Envelope(eventType, payload))
}

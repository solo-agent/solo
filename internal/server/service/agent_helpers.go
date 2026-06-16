package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/solo-ai/solo/internal/realtime"
	"github.com/solo-ai/solo/pkg/agent"
)

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

	daemon := s.dm.SelectDaemon("llm")
	if daemon == nil {
		slog.Warn("TriggerAgentGreeting: no available daemon", "agent_id", agentID)
		return
	}

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

	taskReq := daemonTaskRequest{
		TaskID:    uuid.New().String(),
		AgentID:   ag.ID,
		ChannelID: channelID,
		Messages: []agent.Message{
			{
				Role: agent.RoleUser,
				Content: fmt.Sprintf(
					"[target=#%s msg=%s time=%s type=system] @%s:\n%s",
					channelName,
					uuid.New().String()[:8],
					time.Now().UTC().Format(time.RFC3339),
					ag.Name,
					greetingContent,
				),
				SenderID: "",
			},
		},
		SystemPrompt: ag.SystemPrompt,
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

	go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
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

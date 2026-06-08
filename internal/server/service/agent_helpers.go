package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
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
// asking it to introduce itself in the channel.
func (s *AgentService) TriggerAgentGreeting(ctx context.Context, channelID, agentID string) {
	// Get agent info
	var ag agentChannelInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, model_provider, model_name, system_prompt, temperature, max_tokens
		 FROM agents WHERE id = $1 AND is_active = true`,
		agentID,
	).Scan(&ag.ID, &ag.Name, &ag.ModelProvider, &ag.ModelName,
		&ag.SystemPrompt, &ag.Temperature, &ag.MaxTokens)
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

	// Build a greeting prompt 
	greetingContent := fmt.Sprintf(
		"You have just joined the channel #%s. This is your first time here.\n\n"+
			"Please introduce yourself briefly in the channel:\n"+
			"- Say hi and state your name (@%s)\n"+
			"- Briefly describe your role and how you can help\n"+
			"- Keep it short and friendly\n\n"+
			"Use `solo message send` to post your introduction to the channel.",
		channelName, ag.Name,
	)

	taskReq := daemonTaskRequest{
		TaskID:    uuid.New().String(),
		AgentID:   ag.ID,
		ChannelID: channelID,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: greetingContent, SenderID: ""},
		},
		SystemPrompt: ag.SystemPrompt,
		ModelConfig: agent.ModelConfig{
			Provider:    ag.ModelProvider,
			Model:       ag.ModelName,
			Temperature: ag.Temperature,
			MaxTokens:   ag.MaxTokens,
		},
	}

	slog.Info("triggering agent greeting",
		"agent_id", ag.ID,
		"agent_name", ag.Name,
		"channel_id", channelID,
	)

	go s.handleStreamingAgentTask(context.Background(), daemon, taskReq, ag)
}

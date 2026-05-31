package ws

import (
	"log/slog"
	"time"

	"github.com/solo-ai/solo/internal/realtime"
)

// BroadcastTaskCreated broadcasts a task.created event to channel subscribers.
func BroadcastTaskCreated(hub realtime.Broadcaster, payload TaskCreatedPayload) {
	envelope := Envelope(EventTaskCreated, payload)
	hub.BroadcastToChannel(payload.ChannelID, envelope)
}

// BroadcastTaskUpdated broadcasts a task.updated event to channel subscribers.
func BroadcastTaskUpdated(hub realtime.Broadcaster, payload TaskUpdatedPayload) {
	envelope := Envelope(EventTaskUpdated, payload)
	hub.BroadcastToChannel(payload.ChannelID, envelope)
}

// BroadcastTaskDeleted broadcasts a task.deleted event to channel subscribers.
func BroadcastTaskDeleted(hub realtime.Broadcaster, payload TaskDeletedPayload) {
	envelope := Envelope(EventTaskDeleted, payload)
	hub.BroadcastToChannel(payload.ChannelID, envelope)
}

// BroadcastAgentThinking broadcasts an agent.thinking event to channel subscribers.
func BroadcastAgentThinking(hub realtime.Broadcaster, channelID, agentID, agentName, thought string) {
	payload := AgentThinkingPayload{
		ChannelID: channelID,
		AgentID:   agentID,
		AgentName: agentName,
		Thought:   thought,
	}

	envelope := Envelope(EventAgentThinking, payload)
	hub.BroadcastToChannel(channelID, envelope)

	slog.Debug("broadcast agent thinking",
		"channel_id", channelID,
		"agent_id", agentID,
	)
}

// BroadcastAgentToken broadcasts a message.agent_typing event with streamed token.
func BroadcastAgentToken(hub realtime.Broadcaster, channelID, agentID, messageID, token, accumulated string, done bool) {
	payload := AgentStreamTokenPayload{
		ChannelID:   channelID,
		AgentID:     agentID,
		MessageID:   messageID,
		Content:     token,
		Accumulated: accumulated,
		Done:        done,
	}

	envelope := Envelope(EventAgentStreamToken, payload)
	hub.BroadcastToChannel(channelID, envelope)
}

// BroadcastAgentMessage broadcasts a message.new event with the final agent message.
func BroadcastAgentMessage(hub realtime.Broadcaster, channelID, agentID, agentName, messageID, content string) {
	msgData := MessageNewPayload{
		ID:          messageID,
		ChannelID:   channelID,
		SenderType:  "agent",
		SenderID:    agentID,
		SenderName:  agentName,
		Content:     content,
		ContentType: "text",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	// Send final token with done=true first
	BroadcastAgentToken(hub, channelID, agentID, messageID, "", content, true)

	// Then send the complete message
	envelope := Envelope(EventMessageNew, msgData)
	hub.BroadcastToChannel(channelID, envelope)

	slog.Debug("broadcast agent message",
		"channel_id", channelID,
		"agent_id", agentID,
		"message_id", messageID,
	)
}

// BroadcastAgentError broadcasts an agent.error event to channel subscribers.
func BroadcastAgentError(hub realtime.Broadcaster, channelID, agentID, agentName, errMsg string) {
	payload := AgentErrorPayload{
		ChannelID: channelID,
		AgentID:   agentID,
		AgentName: agentName,
		Error:     errMsg,
	}

	envelope := Envelope(EventAgentError, payload)
	hub.BroadcastToChannel(channelID, envelope)

	slog.Error("broadcast agent error",
		"channel_id", channelID,
		"agent_id", agentID,
		"error", errMsg,
	)
}

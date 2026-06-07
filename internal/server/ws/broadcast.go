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

// BroadcastInboxUpdated sends an inbox.updated event to all connections of a
// specific user. Called after a new message is created that may trigger an
// inbox update for thread participants, DM recipients, or @mentioned users.
func BroadcastInboxUpdated(hub realtime.Broadcaster, userID string) {
	envelope := Envelope(EventInboxUpdated, struct{}{})
	hub.SendToUser(userID, envelope)
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

// BroadcastAgentChunk broadcasts an agent.chunk event to channel subscribers.
func BroadcastAgentChunk(hub realtime.Broadcaster, channelID, agentID, agentName, chunkType, content string, tool *ToolRef) {
	payload := AgentChunkPayload{
		ChannelID: channelID,
		AgentID:   agentID,
		AgentName: agentName,
		ChunkType: chunkType,
		Content:   content,
		Tool:      tool,
	}
	envelope := Envelope(EventAgentChunk, payload)
	hub.BroadcastToChannel(channelID, envelope)
}

// BroadcastAgentDone broadcasts an agent.done event to channel subscribers when
// a task terminates. The frontend uses this as the authoritative terminal
// signal (replaces the previous 3s inactivity heuristic).
func BroadcastAgentDone(hub realtime.Broadcaster, channelID, agentID, agentName, taskID, finalState string) {
	payload := AgentDonePayload{
		ChannelID:  channelID,
		AgentID:    agentID,
		AgentName:  agentName,
		TaskID:     taskID,
		FinalState: finalState,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	envelope := Envelope(EventAgentDone, payload)
	hub.BroadcastToChannel(channelID, envelope)

	slog.Debug("broadcast agent done",
		"channel_id", channelID,
		"agent_id", agentID,
		"final_state", finalState,
	)
}

// BroadcastAgentActivity broadcasts an agent.activity event to channel
// subscribers. Carries the island-facing status and a short activity_text
// summary derived from the agent's OutputChunk. Powers the AgentIsland
// floating UI; the island subscribes to this single event instead of
// multiple chunk/typing/thinking streams.
func BroadcastAgentActivity(hub realtime.Broadcaster, channelID, agentID, agentName, status, activityText, toolName, toolInputSummary, source string) {
	payload := AgentActivityPayload{
		ChannelID:        channelID,
		AgentID:          agentID,
		AgentName:        agentName,
		Status:           status,
		ActivityText:     activityText,
		ToolName:         toolName,
		ToolInputSummary: toolInputSummary,
		Source:           source,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	envelope := Envelope(EventAgentActivity, payload)
	hub.BroadcastToChannel(channelID, envelope)

	slog.Debug("broadcast agent activity",
		"channel_id", channelID,
		"agent_id", agentID,
		"status", status,
	)
}

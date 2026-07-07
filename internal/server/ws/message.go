package ws

import (
	"github.com/solo-ai/solo/internal/realtime"
)

// WSMessage is re-exported from realtime so existing callers (and tests)
// that import the ws package can keep using ws.WSMessage without
// reaching across the realtime package boundary directly. The
// canonical definition lives in realtime (moved there to break the
// service → ws import cycle when realtime needed the broadcaster).
type WSMessage = realtime.WSMessage

// Event types (client -> server)
const (
	EventSubscribe         = "subscribe"
	EventUnsubscribe       = "unsubscribe"
	EventMessageSend       = "message.send"
	EventChannelJoin       = "channel.join"
	EventChannelLeave      = "channel.leave"
	EventTypingStart       = "typing.start"
	EventTypingStop        = "typing.stop"
	EventThreadReply       = "thread.reply"
	EventThreadSubscribe   = "thread.subscribe"
	EventThreadUnsubscribe = "thread.unsubscribe"
	EventDMSubscribe       = "dm.subscribe"
	EventDMUnsubscribe     = "dm.unsubscribe"
	EventTaskCancel        = "task.cancel"
)

// Event types (server -> client)
const (
	EventMessageNew        = "message.new"
	EventMessageUpdated    = "message.updated"
	EventMessageDeleted    = "message.deleted"
	EventChannelUpdated    = "channel.updated"
	EventMemberJoined      = "member.joined"
	EventMemberLeft        = "member.left"
	EventTyping            = "typing"
	EventError             = "error"
	EventSystem            = "system"
	EventThreadMessageNew  = "thread.message.new"
	EventThreadReplyNotify = "thread.reply"

	// Agent status events (SOLO-46-B)
	EventAgentThinking = "agent.thinking"
	EventAgentTyping   = "agent.typing"
	EventAgentStatus   = "agent.status"
	EventAgentError    = "agent.error"

	// Agent streaming events (SOLO-50-B / SOLO-51-B)
	EventAgentStreamToken = "message.agent_typing"

	// Agent chunk events (SOLO-agent-view)
	EventAgentChunk = "agent.chunk"

	// Agent activity event (SOLO-island PR1) — derived from OutputChunk
	// events, carries the island-facing status and a short activity_text
	// summary. Powers the AgentIsland floating UI.
	EventAgentActivity = "agent.activity"

	EventAgentRunStarted  = "agent.run.started"
	EventAgentRunUpdated  = "agent.run.updated"
	EventAgentRunEvent    = "agent.run.event"
	EventAgentRunFinished = "agent.run.finished"

	// DM events (SOLO-57-B)
	EventDMMessageNew = "dm.message.new"

	// Task events (SOLO-122-B)
	EventTaskCreated = "task.created"
	EventTaskUpdated = "task.updated"
	EventTaskDeleted = "task.deleted"

	// Inbox events (v1.5)
	EventInboxUpdated = "inbox.updated"
)

// Envelope creates a JSON-encoded WSMessage for broadcasting.
// Thin re-export of realtime.Envelope so the existing call sites
// in this package keep working unchanged. New code that needs
// envelope construction outside the ws package should use
// realtime.Envelope directly to avoid the historical
// service → ws circular dependency.
func Envelope(msgType string, payload any) []byte {
	return realtime.Envelope(msgType, payload)
}

// ----- Client -> Server payloads -----

type SubscribePayload struct {
	ChannelID string `json:"channel_id"`
}

type DMSubscribePayload struct {
	DMChannelID string `json:"dm_id"`
}

type DMUnsubscribePayload struct {
	DMChannelID string `json:"dm_id"`
}

type UnsubscribePayload struct {
	ChannelID string `json:"channel_id"`
}

type MessageSendPayload struct {
	ChannelID     string   `json:"channel_id"`
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

type TypingPayload struct {
	ChannelID string `json:"channel_id"`
}

type ThreadReplyPayload struct {
	ChannelID     string   `json:"channel_id"`
	ThreadID      string   `json:"thread_id"`
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

type ThreadSubscribePayload struct {
	ChannelID string `json:"channel_id"`
	ThreadID  string `json:"thread_id"`
}

type ThreadUnsubscribePayload struct {
	ChannelID string `json:"channel_id"`
	ThreadID  string `json:"thread_id"`
}

type TaskCancelPayload struct {
	ChannelID string `json:"channel_id"`
	TaskID    string `json:"task_id"`
}

// ----- Server -> Client payloads -----

type AttachmentMeta struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type MessageNewPayload struct {
	ID                 string           `json:"id"`
	ChannelID          string           `json:"channel_id"`
	SenderType         string           `json:"sender_type"`
	SenderID           string           `json:"sender_id"`
	SenderName         string           `json:"sender_name,omitempty"`
	Content            string           `json:"content"`
	ContentType        string           `json:"content_type"`
	ThreadID           string           `json:"thread_id,omitempty"`
	MentionedAgentIDs  []string         `json:"mentioned_agent_ids,omitempty"`
	AttachmentIDs      []string         `json:"attachment_ids,omitempty"`
	Attachments        []AttachmentMeta `json:"attachments,omitempty"`
	CreatedAt          string           `json:"created_at"`
	TaskNumber         int              `json:"task_number,omitempty"`
	TaskStatus         string           `json:"task_status,omitempty"`
	TaskClaimerName    string           `json:"task_claimer_name,omitempty"`
	TaskClaimerDeleted bool             `json:"task_claimer_deleted"`
}

// AgentStreamTokenPayload is broadcast on message.agent_typing for streaming tokens.
type AgentStreamTokenPayload struct {
	ChannelID   string `json:"channel_id"`
	AgentID     string `json:"agent_id"`
	MessageID   string `json:"message_id"`
	Content     string `json:"content"`     // incremental token content
	Accumulated string `json:"accumulated"` // full accumulated content so far
	Done        bool   `json:"done"`        // true for the final chunk
}

// AgentThinkingPayload is broadcast on agent.thinking.
type AgentThinkingPayload struct {
	ChannelID string `json:"channel_id"`
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name,omitempty"`
	Thought   string `json:"thought,omitempty"`
}

// AgentErrorPayload is broadcast on agent.error.
type AgentErrorPayload struct {
	ChannelID string `json:"channel_id"`
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name,omitempty"`
	Error     string `json:"error"`
}

// AgentChunkPayload is broadcast on agent.chunk for each agent output chunk.
type AgentChunkPayload struct {
	ChannelID string   `json:"channel_id"`
	AgentID   string   `json:"agent_id"`
	AgentName string   `json:"agent_name"`
	ChunkType string   `json:"chunk_type"` // thinking, tool_use, tool_result, text, error
	Content   string   `json:"content"`
	Tool      *ToolRef `json:"tool,omitempty"`
}

// AgentActivityPayload is broadcast on agent.activity. It carries the
// island-facing status and a one-line activity_text summary, derived by
// the daemon from agent.OutputChunk events. Powers the AgentIsland
// floating UI; replaces the previous chunk-based heuristic in
// useAgentChunks for the island pill state.
type AgentActivityPayload struct {
	ChannelID        string `json:"channel_id"`
	AgentID          string `json:"agent_id"`
	AgentName        string `json:"agent_name,omitempty"`
	Status           string `json:"status"`        // island status: idle | thinking | running | streaming | waiting_approval | error
	ActivityText     string `json:"activity_text"` // one-line summary in zh-CN
	ToolName         string `json:"tool_name,omitempty"`
	ToolInputSummary string `json:"tool_input_summary,omitempty"` // e.g. "Bash: npm test"
	Source           string `json:"source,omitempty"`             // claude | codex | gemini | kiro | ...; metadata only, not shown in UI
	Timestamp        string `json:"timestamp"`
}

type AgentRunPayload struct {
	RunID            string `json:"run_id"`
	SessionID        string `json:"session_id,omitempty"`
	AgentID          string `json:"agent_id"`
	AgentName        string `json:"agent_name,omitempty"`
	TaskID           string `json:"task_id,omitempty"`
	ChannelID        string `json:"channel_id,omitempty"`
	ThreadID         string `json:"thread_id,omitempty"`
	Status           string `json:"status"`
	ActivityText     string `json:"activity_text,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	ToolInputSummary string `json:"tool_input_summary,omitempty"`
	Source           string `json:"source,omitempty"`
	Timestamp        string `json:"timestamp"`
}

type AgentRunEventPayload struct {
	RunID     string         `json:"run_id"`
	SessionID string         `json:"session_id,omitempty"`
	AgentID   string         `json:"agent_id"`
	AgentName string         `json:"agent_name,omitempty"`
	ChannelID string         `json:"channel_id,omitempty"`
	ThreadID  string         `json:"thread_id,omitempty"`
	Seq       int            `json:"seq"`
	Type      string         `json:"event_type"`
	Message   string         `json:"message,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// ToolRef carries tool call metadata in an agent chunk.
type ToolRef struct {
	Name   string `json:"name"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	CallID string `json:"call_id,omitempty"`
}

// ThreadMessageItem is the message portion of a thread.message.new payload.
type ThreadMessageItem struct {
	ID            string           `json:"id"`
	ChannelID     string           `json:"channel_id"`
	ThreadID      string           `json:"thread_id"`
	SenderType    string           `json:"sender_type"`
	SenderID      string           `json:"sender_id"`
	SenderName    string           `json:"sender_name,omitempty"`
	Content       string           `json:"content"`
	ContentType   string           `json:"content_type"`
	AttachmentIDs []string         `json:"attachment_ids,omitempty"`
	Attachments   []AttachmentMeta `json:"attachments,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

// ThreadMetadataItem is the thread metadata portion of a thread.message.new payload.
type ThreadMetadataItem struct {
	ThreadID    string `json:"thread_id"`
	ReplyCount  int    `json:"reply_count"`
	LastReplyAt string `json:"last_reply_at"`
}

// ThreadMessageNewPayload is broadcast on thread.message.new.
type ThreadMessageNewPayload struct {
	Message ThreadMessageItem  `json:"message"`
	Thread  ThreadMetadataItem `json:"thread"`
}

// LatestReplyItem contains the latest reply info for thread reply notifications.
type LatestReplyItem struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name,omitempty"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

type ThreadReplyNotifyPayload struct {
	ChannelID     string           `json:"channel_id"`
	ThreadID      string           `json:"thread_id"`
	RootMessageID string           `json:"root_message_id"`
	ReplyCount    int              `json:"reply_count"`
	LastReplyAt   string           `json:"last_reply_at"`
	LatestReply   *LatestReplyItem `json:"latest_reply,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MessageUpdatedPayload is broadcast on message.updated.
type MessageUpdatedPayload struct {
	ID                 string   `json:"id"`
	ChannelID          string   `json:"channel_id"`
	SenderType         string   `json:"sender_type"`
	SenderID           string   `json:"sender_id"`
	SenderName         string   `json:"sender_name,omitempty"`
	Content            string   `json:"content"`
	ContentType        string   `json:"content_type"`
	IsEdited           bool     `json:"is_edited"`
	UpdatedAt          string   `json:"updated_at"`
	AttachmentIDs      []string `json:"attachment_ids,omitempty"`
	TaskNumber         int      `json:"task_number,omitempty"`
	TaskStatus         string   `json:"task_status,omitempty"`
	TaskClaimerName    string   `json:"task_claimer_name,omitempty"`
	TaskClaimerDeleted bool     `json:"task_claimer_deleted"`
	ReplyCount         int      `json:"reply_count,omitempty"`
}

// MessageDeletedPayload is broadcast on message.deleted.
type MessageDeletedPayload struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// DMMessageNewPayload is broadcast on dm.message.new.
// Fields are flat (not nested) to match frontend WSServerEvent dm.message.new type.
type DMMessageNewPayload struct {
	DMID          string           `json:"dm_id"`
	ID            string           `json:"id"`
	ChannelID     string           `json:"channel_id"`
	SenderType    string           `json:"sender_type"`
	SenderID      string           `json:"sender_id"`
	SenderName    string           `json:"sender_name,omitempty"`
	Content       string           `json:"content"`
	ContentType   string           `json:"content_type"`
	AttachmentIDs []string         `json:"attachment_ids,omitempty"`
	Attachments   []AttachmentMeta `json:"attachments,omitempty"`
	ThreadID      string           `json:"thread_id,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

// TaskCreatedPayload is broadcast on task.created.
type TaskCreatedPayload struct {
	ID               string `json:"id"`
	TaskNumber       int    `json:"task_number"`
	ChannelID        string `json:"channel_id"`
	CreatorID        string `json:"creator_id"`
	CreatorName      string `json:"creator_name,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	Status           string `json:"status"`
	ClaimerID        string `json:"claimer_id,omitempty"`
	ClaimerName      string `json:"claimer_name,omitempty"`
	Priority         string `json:"priority"`
	DueDate          string `json:"due_date,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	SubtaskCount     int    `json:"subtask_count,omitempty"`
	DoneSubtaskCount int    `json:"done_subtask_count,omitempty"`
	ArtifactStatus   string `json:"artifact_status,omitempty"`
}

// TaskUpdatedPayload is broadcast on task.updated.
type TaskUpdatedPayload struct {
	ID               string `json:"id"`
	TaskNumber       int    `json:"task_number"`
	ChannelID        string `json:"channel_id"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	Status           string `json:"status"`
	ClaimerID        string `json:"claimer_id,omitempty"`
	ClaimerName      string `json:"claimer_name,omitempty"`
	Priority         string `json:"priority"`
	DueDate          string `json:"due_date,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	UpdatedAt        string `json:"updated_at"`
	SubtaskCount     int    `json:"subtask_count,omitempty"`
	DoneSubtaskCount int    `json:"done_subtask_count,omitempty"`
	ArtifactStatus   string `json:"artifact_status,omitempty"`
}

// TaskDeletedPayload is broadcast on task.deleted.
type TaskDeletedPayload struct {
	ID         string `json:"id"`
	ChannelID  string `json:"channel_id"`
	TaskNumber int    `json:"task_number"`
}

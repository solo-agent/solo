package realtime

// Scope types recognised by the broadcaster.
const (
	ScopeChannel = "channel"
	ScopeUser    = "user"
	ScopeThread  = "thread"
)

// Broadcaster is the abstraction every realtime event producer should depend
// on instead of *Hub directly.
type Broadcaster interface {
	// BroadcastToScope fans a message out to every connection currently
	// subscribed to ({scopeType, scopeID}) on this node.
	BroadcastToScope(scopeType, scopeID string, message []byte)

	// BroadcastToChannel is a shortcut for BroadcastToScope("channel", channelID, message).
	BroadcastToChannel(channelID string, message []byte)

	// SendToUser delivers a message to every connection belonging to userID.
	SendToUser(userID string, message []byte)

	// BroadcastToThread is a shortcut for BroadcastToScope("thread", threadID, message).
	BroadcastToThread(threadID string, message []byte)

	// Broadcast fans a message out to every connection on this node.
	Broadcast(message []byte)
}

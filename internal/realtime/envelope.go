package realtime

import "encoding/json"

// Envelope creates a JSON-encoded WSMessage for broadcasting. It mirrors
// the shape used by internal/server/ws.Envelope — moved here so producers
// outside the ws package (notably internal/server/service) can construct
// well-formed events without taking on a circular dependency.
//
// The shape is:
//
//	{"type": "<event>", "payload": {<fields>}}
//
// The payload is marshalled with json.Marshal, then the envelope with
// json.Marshal of the parent object holding a json.RawMessage — that
// avoids double-quoting the payload object.
func Envelope(msgType string, payload any) []byte {
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(WSMessage{Type: msgType, Payload: raw})
	return data
}

// WSMessage is the on-the-wire shape of every event. Defined here (rather
// than in internal/server/ws) so producers can construct envelopes
// without importing ws.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

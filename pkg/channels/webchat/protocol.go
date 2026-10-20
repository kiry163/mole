package webchat

import (
	"time"
)

// Protocol message types.
const (
	// TypeMessageSend is sent from client to server.
	TypeMessageSend = "message.send"
	TypeMediaSend   = "media.send"
	TypePing        = "ping"

	// TypeMessageCreate is sent from server to client.
	TypeMessageCreate = "message.create"
	TypeMessageUpdate = "message.update"
	TypeMessageDelete = "message.delete"
	TypeTypingStart   = "typing.start"
	TypeTypingStop    = "typing.stop"
	TypeError         = "error"
	TypePong          = "pong"

	PayloadKeyContent     = "content"
	PayloadKeyKind        = "kind"
	PayloadKeyPlaceholder = "placeholder"
	PayloadKeyToolCalls   = "tool_calls"
	PayloadKeyModelName   = "model_name"
	PayloadKeyUsage       = "usage"

	MessageKindThought   = "thought"
	MessageKindToolCalls = "tool_calls"
)

// WebChatMessage is the wire format for all WebChat Protocol messages.
type WebChatMessage struct {
	Type      string         `json:"type"`
	ID        string         `json:"id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// newMessage creates a WebChatMessage with the given type and payload.
func newMessage(msgType string, payload map[string]any) WebChatMessage {
	return WebChatMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

func newErrorWithPayload(code, message string, extra map[string]any) WebChatMessage {
	payload := map[string]any{
		"code":    code,
		"message": message,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return newMessage(TypeError, payload)
}

// newError creates an error WebChatMessage.
func newError(code, message string) WebChatMessage {
	return newErrorWithPayload(code, message, nil)
}

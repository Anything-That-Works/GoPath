package ws

import "github.com/google/uuid"

type MessageType string

const (
	TypeText       MessageType = "text"
	TypeFile       MessageType = "file"
	TypeEdit       MessageType = "edit"
	TypeDelete     MessageType = "delete"
	TypeRead       MessageType = "read"
	TypeTyping     MessageType = "typing"
	TypeStopTyping MessageType = "stop_typing"
	TypeAck        MessageType = "ack"
	TypeDelivered  MessageType = "delivered"
	TypeError      MessageType = "error"
)

// incoming from client
type IncomingMessage struct {
	Type           MessageType `json:"type"`
	ConversationID uuid.UUID   `json:"conversation_id"`
	Content        string      `json:"content,omitempty"`
	FileID         *uuid.UUID  `json:"file_id,omitempty"`
	ReplyToID      *uuid.UUID  `json:"reply_to_id,omitempty"`
	MessageID      *uuid.UUID  `json:"message_id,omitempty"` // for edit/delete/read
}

// outgoing to client
type OutgoingMessage struct {
	Type           MessageType `json:"type"`
	MessageID      *uuid.UUID  `json:"message_id,omitempty"`
	ConversationID *uuid.UUID  `json:"conversation_id,omitempty"`
	SenderID       *uuid.UUID  `json:"sender_id,omitempty"`
	Content        string      `json:"content,omitempty"`
	FileID         *uuid.UUID  `json:"file_id,omitempty"`
	FileURL        string      `json:"file_url,omitempty"`
	ReplyToID      *uuid.UUID  `json:"reply_to_id,omitempty"`
	IsEdited       bool        `json:"is_edited,omitempty"`
	CreatedAt      string      `json:"created_at,omitempty"`
	Error          string      `json:"error,omitempty"`
}

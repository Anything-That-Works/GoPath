package ws

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/google/uuid"
)

type MessageHandler struct {
	Hub     *Hub
	DB      *database.Queries
	Storage StorageProvider
}

type StorageProvider interface {
	URL(path string) string
}

func NewMessageHandler(hub *Hub, db *database.Queries, storage StorageProvider) *MessageHandler {
	return &MessageHandler{Hub: hub, DB: db, Storage: storage}
}

func (h *MessageHandler) Handle(client *Client, data []byte) {
	var msg IncomingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		client.SendMessage(OutgoingMessage{
			Type:  TypeError,
			Error: "Invalid message format",
		})
		return
	}

	switch msg.Type {
	case TypeText, TypeFile:
		h.handleSendMessage(client, msg)
	case TypeEdit:
		h.handleEditMessage(client, msg)
	case TypeDelete:
		h.handleDeleteMessage(client, msg)
	case TypeRead:
		h.handleReadMessage(client, msg)
	case TypeTyping, TypeStopTyping:
		h.handleTyping(client, msg)
	default:
		client.SendMessage(OutgoingMessage{
			Type:  TypeError,
			Error: "Unknown message type",
		})
	}
}

func (h *MessageHandler) handleSendMessage(client *Client, msg IncomingMessage) {
	if msg.Content == "" && msg.FileID == nil {
		client.SendMessage(OutgoingMessage{
			Type:  TypeError,
			Error: "Message must have content or file",
		})
		return
	}

	// build nullable fields
	var content sql.NullString
	if msg.Content != "" {
		content = sql.NullString{String: msg.Content, Valid: true}
	}

	var fileID uuid.NullUUID
	if msg.FileID != nil {
		fileID = uuid.NullUUID{UUID: *msg.FileID, Valid: true}
	}

	var replyToID uuid.NullUUID
	if msg.ReplyToID != nil {
		replyToID = uuid.NullUUID{UUID: *msg.ReplyToID, Valid: true}
	}

	// save to DB
	savedMsg, err := h.DB.CreateMessage(context.Background(), database.CreateMessageParams{
		ConversationID: client.ConversationID,
		SenderID:       client.UserID,
		Content:        content,
		FileID:         fileID,
		ReplyToID:      replyToID,
	})
	if err != nil {
		log.Printf("failed to save message: %v", err)
		client.SendMessage(OutgoingMessage{
			Type:  TypeError,
			Error: "Failed to save message",
		})
		return
	}

	// update conversation timestamp
	_ = h.DB.UpdateConversationTimestamp(context.Background(), client.ConversationID)

	// build outgoing message
	outgoing := OutgoingMessage{
		Type:           msg.Type,
		MessageID:      &savedMsg.ID,
		ConversationID: &savedMsg.ConversationID,
		SenderID:       &savedMsg.SenderID,
		Content:        msg.Content,
		CreatedAt:      savedMsg.CreatedAt.Format(time.RFC3339),
	}

	if msg.FileID != nil {
		outgoing.FileID = msg.FileID
		// fetch file URL
		file, err := h.DB.GetFileByID(context.Background(), *msg.FileID)
		if err == nil {
			outgoing.FileURL = h.Storage.URL(file.Path)
		}
	}

	if msg.ReplyToID != nil {
		outgoing.ReplyToID = msg.ReplyToID
	}

	// send ack back to sender with message ID
	client.SendMessage(OutgoingMessage{
		Type:      TypeAck,
		MessageID: &savedMsg.ID,
		CreatedAt: savedMsg.CreatedAt.Format(time.RFC3339),
	})

	// broadcast to other members
	h.Hub.BroadcastToConversation(client.ConversationID, client.UserID, outgoing)

	// mark as delivered for online members
	h.markDeliveredForOnlineMembers(savedMsg.ID, client.ConversationID, client.UserID)
}

func (h *MessageHandler) handleEditMessage(client *Client, msg IncomingMessage) {
	if msg.MessageID == nil {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "message_id required"})
		return
	}
	if msg.Content == "" {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "content required"})
		return
	}

	edited, err := h.DB.EditMessage(context.Background(), database.EditMessageParams{
		ID:       *msg.MessageID,
		Content:  sql.NullString{String: msg.Content, Valid: true},
		SenderID: client.UserID,
	})
	if err != nil {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "Failed to edit message"})
		return
	}

	outgoing := OutgoingMessage{
		Type:           TypeEdit,
		MessageID:      &edited.ID,
		ConversationID: &edited.ConversationID,
		SenderID:       &edited.SenderID,
		Content:        edited.Content.String,
		IsEdited:       true,
		CreatedAt:      edited.UpdatedAt.Format(time.RFC3339),
	}

	client.SendMessage(OutgoingMessage{Type: TypeAck, MessageID: &edited.ID})
	h.Hub.BroadcastToConversation(client.ConversationID, client.UserID, outgoing)
}

func (h *MessageHandler) handleDeleteMessage(client *Client, msg IncomingMessage) {
	if msg.MessageID == nil {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "message_id required"})
		return
	}

	err := h.DB.SoftDeleteMessage(context.Background(), database.SoftDeleteMessageParams{
		ID:       *msg.MessageID,
		SenderID: client.UserID,
	})
	if err != nil {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "Failed to delete message"})
		return
	}

	outgoing := OutgoingMessage{
		Type:           TypeDelete,
		MessageID:      msg.MessageID,
		ConversationID: &client.ConversationID,
	}

	client.SendMessage(OutgoingMessage{Type: TypeAck, MessageID: msg.MessageID})
	h.Hub.BroadcastToConversation(client.ConversationID, client.UserID, outgoing)
}

func (h *MessageHandler) handleReadMessage(client *Client, msg IncomingMessage) {
	if msg.MessageID == nil {
		client.SendMessage(OutgoingMessage{Type: TypeError, Error: "message_id required"})
		return
	}

	_ = h.DB.MarkMessageRead(context.Background(), database.MarkMessageReadParams{
		MessageID: *msg.MessageID,
		UserID:    client.UserID,
	})

	_ = h.DB.UpdateLastRead(context.Background(), database.UpdateLastReadParams{
		ConversationID: client.ConversationID,
		UserID:         client.UserID,
	})

	outgoing := OutgoingMessage{
		Type:           TypeRead,
		MessageID:      msg.MessageID,
		ConversationID: &client.ConversationID,
		SenderID:       &client.UserID,
	}

	h.Hub.BroadcastToConversation(client.ConversationID, client.UserID, outgoing)
}

func (h *MessageHandler) handleTyping(client *Client, msg IncomingMessage) {
	h.Hub.BroadcastToConversation(client.ConversationID, client.UserID, OutgoingMessage{
		Type:           msg.Type,
		ConversationID: &client.ConversationID,
		SenderID:       &client.UserID,
	})
}

func (h *MessageHandler) markDeliveredForOnlineMembers(messageID uuid.UUID, conversationID uuid.UUID, senderID uuid.UUID) {
	members, err := h.DB.GetConversationMembers(context.Background(), conversationID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.UserID == senderID {
			continue
		}
		if h.Hub.IsUserOnline(conversationID, member.UserID) {
			_ = h.DB.UpsertMessageReceipt(context.Background(), database.UpsertMessageReceiptParams{
				MessageID: messageID,
				UserID:    member.UserID,
			})
			// notify sender of delivery
			h.Hub.SendToUser(conversationID, senderID, OutgoingMessage{
				Type:      TypeDelivered,
				MessageID: &messageID,
				SenderID:  &member.UserID,
			})
		}
	}
}

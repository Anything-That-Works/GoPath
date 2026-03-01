package ws

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

type Hub struct {
	// conversation_id -> userID -> set of clients (multiple devices per user)
	Rooms      map[uuid.UUID]map[uuid.UUID]map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Rooms:      make(map[uuid.UUID]map[uuid.UUID]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			if _, ok := h.Rooms[client.ConversationID]; !ok {
				h.Rooms[client.ConversationID] = make(map[uuid.UUID]map[*Client]bool)
			}
			if _, ok := h.Rooms[client.ConversationID][client.UserID]; !ok {
				h.Rooms[client.ConversationID][client.UserID] = make(map[*Client]bool)
			}
			h.Rooms[client.ConversationID][client.UserID][client] = true
			h.mu.Unlock()

		case client := <-h.Unregister:
			h.mu.Lock()
			if users, ok := h.Rooms[client.ConversationID]; ok {
				if clients, ok := users[client.UserID]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(users, client.UserID)
					}
				}
				if len(users) == 0 {
					delete(h.Rooms, client.ConversationID)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastToConversation sends to all clients in a conversation except the sender
func (h *Hub) BroadcastToConversation(conversationID uuid.UUID, senderID uuid.UUID, msg OutgoingMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if users, ok := h.Rooms[conversationID]; ok {
		for userID, clients := range users {
			if userID == senderID {
				continue
			}
			for client := range clients {
				select {
				case client.Send <- data:
				default:
					close(client.Send)
					delete(clients, client)
				}
			}
		}
	}
}

// SendToUser sends to all connections of a specific user in a conversation
func (h *Hub) SendToUser(conversationID uuid.UUID, userID uuid.UUID, msg OutgoingMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if users, ok := h.Rooms[conversationID]; ok {
		if clients, ok := users[userID]; ok {
			for client := range clients {
				select {
				case client.Send <- data:
				default:
					close(client.Send)
					delete(clients, client)
				}
			}
		}
	}
}

// IsUserOnline checks if a user has any active connections in a conversation
func (h *Hub) IsUserOnline(conversationID uuid.UUID, userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if users, ok := h.Rooms[conversationID]; ok {
		if clients, ok := users[userID]; ok {
			return len(clients) > 0
		}
	}
	return false
}

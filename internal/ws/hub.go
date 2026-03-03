package ws

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub struct {
	Rooms      map[uuid.UUID]map[uuid.UUID]map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	stop       chan struct{}
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Rooms:      make(map[uuid.UUID]map[uuid.UUID]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		stop:       make(chan struct{}),
	}
}

func (h *Hub) Stop() {
	close(h.stop)

	h.mu.Lock()
	defer h.mu.Unlock()

	// close all client connections concurrently
	var wg sync.WaitGroup
	for _, users := range h.Rooms {
		for _, clients := range users {
			for client := range clients {
				wg.Add(1)
				go func(c *Client) {
					defer wg.Done()
					_ = c.Conn.WriteMessage(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
					)
					_ = c.Conn.Close()
				}(client)
			}
		}
	}
	wg.Wait()
	h.Rooms = make(map[uuid.UUID]map[uuid.UUID]map[*Client]bool)
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.stop:
			return
		case client := <-h.Register:
			h.mu.Lock()
			if _, ok := h.Rooms[client.ConversationID]; !ok {
				h.Rooms[client.ConversationID] = make(map[uuid.UUID]map[*Client]bool)
			}
			if _, ok := h.Rooms[client.ConversationID][client.UserID]; !ok {
				h.Rooms[client.ConversationID][client.UserID] = make(map[*Client]bool)
			}
			isFirstConnection := len(h.Rooms[client.ConversationID][client.UserID]) == 0
			h.Rooms[client.ConversationID][client.UserID][client] = true
			h.mu.Unlock()

			if isFirstConnection {
				go h.BroadcastToConversation(client.ConversationID, client.UserID, OutgoingMessage{
					Type:           TypeOnline,
					ConversationID: &client.ConversationID,
					SenderID:       &client.UserID,
				})
			}

		case client := <-h.Unregister:
			h.mu.Lock()
			isLastConnection := false
			if users, ok := h.Rooms[client.ConversationID]; ok {
				if clients, ok := users[client.UserID]; ok {
					delete(clients, client)
					if len(clients) == 0 {
						isLastConnection = true
						delete(users, client.UserID)
					}
				}
				if len(users) == 0 {
					delete(h.Rooms, client.ConversationID)
				}
			}
			select {
			case _, ok := <-client.Send:
				if ok {
					close(client.Send)
				}
			default:
				close(client.Send)
			}
			h.mu.Unlock()

			if isLastConnection {
				go h.BroadcastToConversation(client.ConversationID, client.UserID, OutgoingMessage{
					Type:           TypeOffline,
					ConversationID: &client.ConversationID,
					SenderID:       &client.UserID,
				})
			}
		}
	}
}

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

func (h *Hub) GetOnlineUsers(conversationID uuid.UUID) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	onlineUsers := []uuid.UUID{}
	if users, ok := h.Rooms[conversationID]; ok {
		for userID := range users {
			onlineUsers = append(onlineUsers, userID)
		}
	}
	return onlineUsers
}

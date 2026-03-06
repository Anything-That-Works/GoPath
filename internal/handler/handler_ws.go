package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/Anything-That-Works/GoPath/internal/ws"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
		for _, allowed := range strings.Split(allowedOrigins, ",") {
			if origin == strings.TrimSpace(allowed) {
				return true
			}
		}
		return false
	},
}

func (handler *Handler) HandlerWebSocket(hub *ws.Hub, msgHandler *ws.MessageHandler) http.HandlerFunc {
	return handler.MiddlewareAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
		if !ok {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
			return
		}

		conversationIDStr := r.URL.Query().Get("conversation_id")
		conversationID, err := uuid.Parse(conversationIDStr)
		if err != nil {
			respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid conversation ID"})
			return
		}

		// verify membership
		_, err = handler.ApiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
			ConversationID: conversationID,
			UserID:         userID,
		})
		if err != nil {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := &ws.Client{
			ID:             uuid.New(),
			UserID:         userID,
			ConversationID: conversationID,
			Hub:            hub,
			Conn:           conn,
			Send:           make(chan []byte, 256),
		}

		hub.Register <- client

		go client.WritePump()
		go client.ReadPump(msgHandler.Handle)
	})
}

package main

import (
	"net/http"

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
		return true // restrict in production
	},
}

func (apiCfg *apiConfig) handlerWebSocket(hub *ws.Hub, msgHandler *ws.MessageHandler) http.HandlerFunc {
	return apiCfg.middlewareAuth(func(w http.ResponseWriter, r *http.Request) {
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
		_, err = apiCfg.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
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

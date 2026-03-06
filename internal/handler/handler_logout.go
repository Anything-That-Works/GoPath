package handler

import (
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (handler *Handler) HandlerLogout(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	err := handler.ApiConfig.DB.RevokeRefreshToken(r.Context(), userID)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to logout"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Logged out successfully",
	})
}

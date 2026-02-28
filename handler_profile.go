package main

import (
	"database/sql"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	user, err := apiConfig.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "User not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch user"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Profile fetched successfully",
		Data:    databaseUserToUserSummary(user),
	})
}

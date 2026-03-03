package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	cacheKey := cache.KeyUserProfile(userID.String())

	// try cache first
	if cached, err := apiConfig.Cache.Get(r.Context(), cacheKey); err == nil {
		var user model.UserSummary
		if err := json.Unmarshal([]byte(cached), &user); err == nil {
			respondWithJSON(w, 200, model.APIResponse{
				Success: true,
				Message: "Profile fetched successfully",
				Data:    user,
			})
			return
		}
	}

	// cache miss — fetch from DB
	user, err := apiConfig.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "User not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch user"})
		return
	}

	summary := databaseUserToUserSummary(user)

	// store in cache
	if data, err := json.Marshal(summary); err == nil {
		_ = apiConfig.Cache.Set(r.Context(), cacheKey, string(data), cache.TTLUserProfile)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Profile fetched successfully",
		Data:    summary,
	})
}

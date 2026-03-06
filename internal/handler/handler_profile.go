package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (handler *Handler) HandlerGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Failed to get user from context"})
		return
	}

	cacheKey := cache.KeyUserProfile(userID.String())

	// try cache first
	if cached, err := handler.ApiConfig.Cache.Get(r.Context(), cacheKey); err == nil {
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
	user, err := handler.ApiConfig.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "User not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch user"})
		return
	}

	summary := model.DatabaseUserToUserSummary(user)

	// store in cache
	if data, err := json.Marshal(summary); err == nil {
		_ = handler.ApiConfig.Cache.Set(r.Context(), cacheKey, string(data), cache.TTLUserProfile)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Profile fetched successfully",
		Data:    summary,
	})
}

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerLogout(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		RefreshToken string `json:"refresh_token"`
	}

	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.RefreshToken == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Refresh token required"})
		return
	}

	// hash the token to look it up
	sum := sha256.Sum256([]byte(params.RefreshToken))
	tokenHash := fmt.Sprintf("%x", sum)

	existing, err := apiConfig.DB.GetRefreshTokenByHash(r.Context(), tokenHash)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Invalid refresh token"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch refresh token"})
		return
	}

	// make sure token belongs to the requesting user
	if existing.UserID != userID {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Forbidden"})
		return
	}

	err = apiConfig.DB.RevokeRefreshToken(r.Context(), existing.ID)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to logout"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Logged out successfully",
	})
}

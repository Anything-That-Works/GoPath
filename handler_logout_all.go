package main

import (
	"database/sql"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/auth"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerLogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	err := apiConfig.DB.RevokeAllUserRefreshTokens(r.Context(), userID)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to revoke tokens"})
		return
	}

	tr, tokenHash, err := auth.GenerateToken(userID, apiConfig.JWTSecretKey)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to generate token"})
		return
	}

	_, err = apiConfig.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: tr.RefreshToken.Expires,
		UserAgent: sql.NullString{String: r.UserAgent(), Valid: true},
		IpAddress: getIPAddress(r),
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to store refresh token"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Logged out of all devices successfully",
		Data:    tr,
	})
}

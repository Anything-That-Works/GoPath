package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Anything-That-Works/GoPath/internal/auth"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerRefreshToken(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		RefreshToken string `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		log.Println("Decode error:", err)
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.RefreshToken == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Refresh token required"})
		return
	}

	// hash the incoming token to look it up
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

	// check if revoked
	if existing.RevokedAt.Valid {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Refresh token has been revoked"})
		return
	}

	// check if expired
	if existing.ExpiresAt.Before(time.Now()) {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Refresh token has expired"})
		return
	}

	// generate new tokens
	tr, newTokenHash, err := auth.GenerateToken(existing.UserID, apiConfig.JWTSecretKey)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to generate token"})
		return
	}

	// store new refresh token
	newToken, err := apiConfig.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		UserID:    existing.UserID,
		TokenHash: newTokenHash,
		ExpiresAt: tr.RefreshToken.Expires,
		UserAgent: sql.NullString{String: r.UserAgent(), Valid: true},
		IpAddress: getIPAddress(r),
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to store refresh token"})
		return
	}

	// rotate — revoke old token and point to new one
	err = apiConfig.DB.RotateRefreshToken(r.Context(), database.RotateRefreshTokenParams{
		ID:                existing.ID,
		ReplacedByTokenID: uuid.NullUUID{UUID: newToken.ID, Valid: true},
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to rotate refresh token"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Token refreshed successfully",
		Data:    tr,
	})
}

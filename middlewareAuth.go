package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/Anything-That-Works/GoPath/internal/auth"
	"github.com/Anything-That-Works/GoPath/internal/model"
)

type contextKey string

const contextKeyUserID contextKey = "userID"

func (apiCfg *apiConfig) middlewareAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Invalid authorization header format"})
			return
		}

		claims, err := auth.ValidateJWT(parts[1], apiCfg.JWTSecretKey)
		if err != nil {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Invalid or expired token"})
			return
		}

		ctx := context.WithValue(r.Context(), contextKeyUserID, claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

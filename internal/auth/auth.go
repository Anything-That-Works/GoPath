package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AuthData struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID uuid.UUID, secretKey []byte) (model.TokenResponse, string, error) {
	at, err := jwt.NewWithClaims(jwt.SigningMethodHS256, AuthData{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(secretKey)
	if err != nil {
		return model.TokenResponse{}, "", err
	}

	rawToken, tokenHash, err := GenerateRefreshToken()
	if err != nil {
		return model.TokenResponse{}, "", err
	}

	now := time.Now()
	return model.TokenResponse{
		AccessToken: at,
		RefreshToken: model.RefreshToken{
			Token:   rawToken,
			Created: now,
			Expires: now.Add(7 * 24 * time.Hour),
		},
	}, tokenHash, nil
}

func GenerateRefreshToken() (rawToken string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	rawToken = base64.URLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(rawToken))
	hash = fmt.Sprintf("%x", sum)
	return rawToken, hash, nil
}

func ValidateJWT(tokenString string, secretKey []byte) (*AuthData, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AuthData{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AuthData)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

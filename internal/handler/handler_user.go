package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/Anything-That-Works/GoPath/internal/auth"
	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (handler *Handler) HandlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		payload := model.APIResponse{
			Success: false,
			Message: "Invalid request payload",
			Data:    nil,
		}
		respondWithJSON(w, 400, payload)
		return
	}

	if params.Email == "" || params.Password == "" {
		payload := model.APIResponse{
			Success: false,
			Message: "Email and password required",
			Data:    nil,
		}
		respondWithJSON(w, 400, payload)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(params.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("GenerateFromPassword error:", err)
		payload := model.APIResponse{
			Success: false,
			Message: "Failed to process password",
			Data:    nil,
		}
		respondWithJSON(w, 500, payload)
		return
	}

	user, err := handler.ApiConfig.DB.CreateUser(r.Context(), database.CreateUserParams{
		Name: sql.NullString{
			String: params.Name,
			Valid:  params.Name != "",
		},
		Email:        params.Email,
		PasswordHash: string(hash),
	})

	if err != nil {
		log.Println("CreateUser error:", err)
		payload := model.APIResponse{
			Success: false,
			Message: "Failed to create user",
			Data:    nil,
		}
		respondWithJSON(w, 500, payload)
		return
	}

	tr, tokenHash, err := auth.GenerateToken(user.ID, handler.ApiConfig.JWTSecretKey)
	if err != nil {
		log.Println("GenerateToken error:", err)
		payload := model.APIResponse{
			Success: false,
			Message: "Failed to generate token",
			Data:    nil,
		}
		respondWithJSON(w, 500, payload)
		return
	}
	ip := handler.getIPAddress(r)

	_, err = handler.ApiConfig.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: tr.RefreshToken.Expires,
		UserAgent: sql.NullString{String: r.UserAgent(), Valid: true},
		IpAddress: ip,
	})
	if err != nil {
		log.Println("CreateRefreshToken error:", err)
		payload := model.APIResponse{
			Success: false,
			Message: "Failed to store refresh token",
			Data:    nil,
		}
		respondWithJSON(w, 500, payload)
		return
	}

	payload := model.APIResponse{
		Success: true,
		Message: "User created successfully",
		Data:    tr,
	}
	respondWithJSON(w, 201, payload)
}

func (handler *Handler) HandlerLookupUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		ID    string `json:"user_id"`
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.ID == "" && params.Email == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "id or email required"})
		return
	}

	var user database.User
	var err error

	if params.ID != "" {
		userID, parseErr := uuid.Parse(params.ID)
		if parseErr != nil {
			respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid user ID"})
			return
		}
		user, err = handler.ApiConfig.DB.GetUserByID(r.Context(), userID)
	} else {
		user, err = handler.ApiConfig.DB.GetUserByEmail(r.Context(), params.Email)
	}

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
		Message: "User fetched successfully",
		Data:    model.DatabaseUserToUserSummary(user),
	})
}

func (handler *Handler) HandlerEmailExists(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.Email == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "email required"})
		return
	}

	exists, err := handler.ApiConfig.DB.UserExistsByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to check email"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Email existence checked successfully",
		Data:    map[string]bool{"exists": exists},
	})
}

func (handler *Handler) HandlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Password *string `json:"password"`
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

	if params.Name == nil && params.Password == nil && params.Email == nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Nothing to update"})
		return
	}

	existingUser, err := handler.ApiConfig.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "User not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch user"})
		return
	}

	passwordHash := existingUser.PasswordHash
	if params.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*params.Password), bcrypt.DefaultCost)
		if err != nil {
			respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to process password"})
			return
		}
		passwordHash = string(hash)
	}

	var newName sql.NullString
	if params.Name != nil {
		newName = sql.NullString{String: *params.Name, Valid: true}
	} else {
		newName = existingUser.Name
	}

	newEmail := existingUser.Email
	if params.Email != nil {
		newEmail = *params.Email
	}

	user, err := handler.ApiConfig.DB.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:           userID,
		Name:         newName,
		Email:        newEmail,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			respondWithJSON(w, 409, model.APIResponse{Success: false, Message: "Email already in use"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to update user"})
		return
	}

	if err := handler.ApiConfig.Cache.Delete(r.Context(), cache.KeyUserProfile(userID.String())); err != nil {
		log.Println("Cache.Delete error:", err)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "User updated successfully",
		Data:    model.DatabaseUserToUserSummary(user),
	})
}

func (handler *Handler) HandlerLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.Email == "" || params.Password == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Email and password required"})
		return
	}

	user, err := handler.ApiConfig.DB.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Invalid email or password"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch user"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(params.Password)); err != nil {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Invalid email or password"})
		return
	}

	tr, tokenHash, err := auth.GenerateToken(user.ID, handler.ApiConfig.JWTSecretKey)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to generate token"})
		return
	}

	_, err = handler.ApiConfig.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: tr.RefreshToken.Expires,
		UserAgent: sql.NullString{String: r.UserAgent(), Valid: true},
		IpAddress: handler.getIPAddress(r),
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to store refresh token"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Login successful",
		Data:    tr,
	})
}

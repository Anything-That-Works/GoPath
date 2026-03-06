package handler

import "github.com/Anything-That-Works/GoPath/internal/model"

type contextKey string

const contextKeyUserID contextKey = "userID"

type Handler struct {
	ApiConfig *model.ApiConfig
}

func New(apiConfig *model.ApiConfig) *Handler {
	return &Handler{ApiConfig: apiConfig}
}

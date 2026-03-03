package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
)

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, model.APIResponse{
		Success: false,
		Message: msg,
	})
}

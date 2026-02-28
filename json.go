package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
)

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, model.APIResponse{
		Success: false,
		Message: msg,
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)

	if err != nil {
		log.Printf("Failed to marshal JSON response %v", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(dat)
}

package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/google/uuid"
)

const maxFileSize = 10 << 20 // 10MB

var allowedMimeTypes = map[string]bool{
	// images
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	// videos
	"video/mp4":       true,
	"video/quicktime": true,
	// documents
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       true,
	// audio
	"audio/mpeg": true,
	"audio/ogg":  true,
}

var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".mp4":  true,
	".mov":  true,
	".pdf":  true,
	".docx": true,
	".xlsx": true,
	".mp3":  true,
	".ogg":  true,
}

func (apiConfig *apiConfig) handlerUploadFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	// limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "File too large, max 10MB allowed"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "File is required"})
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Println("Failed to close file:", err)
		}
	}()

	// validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "File type not allowed"})
		return
	}

	// validate mime type
	mimeType := header.Header.Get("Content-Type")
	if !allowedMimeTypes[mimeType] {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "File type not allowed"})
		return
	}

	// save to storage
	path, err := apiConfig.Storage.Save(file, header.Filename, mimeType)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to save file"})
		return
	}

	// save to db
	savedFile, err := apiConfig.DB.CreateFile(r.Context(), database.CreateFileParams{
		UploaderID: userID,
		Name:       header.Filename,
		MimeType:   mimeType,
		Size:       header.Size,
		Path:       path,
	})
	if err != nil {
		if deleteErr := apiConfig.Storage.Delete(path); deleteErr != nil {
			fmt.Println("Failed to delete file after DB error:", deleteErr)
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to save file metadata"})
		return
	}

	respondWithJSON(w, 201, model.APIResponse{
		Success: true,
		Message: "File uploaded successfully",
		Data: map[string]interface{}{
			"id":        savedFile.ID,
			"name":      savedFile.Name,
			"mime_type": savedFile.MimeType,
			"size":      savedFile.Size,
			"url":       apiConfig.Storage.URL(savedFile.Path),
		},
	})
}

func (apiConfig *apiConfig) handlerServeFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}
	_ = userID // can be used for access control later

	filename := r.URL.Path[len("/v1/files/"):]
	if filename == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Filename required"})
		return
	}

	http.ServeFile(w, r, filepath.Join("./uploads", filename))
}

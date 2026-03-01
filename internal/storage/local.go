package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type LocalStorage struct {
	BasePath string
	BaseURL  string
}

func NewLocalStorage(basePath string, baseURL string) *LocalStorage {
	if err := os.MkdirAll(basePath, os.ModePerm); err != nil {
		fmt.Println("Error creating directory:", err)
	}
	return &LocalStorage{BasePath: basePath, BaseURL: baseURL}
}

func (s *LocalStorage) Save(file multipart.File, filename string, mimeType string) (string, error) {
	ext := filepath.Ext(filename)
	newFilename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.New().String(), ext)
	destPath := filepath.Join(s.BasePath, newFilename)

	dest, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := dest.Close(); err != nil {
			fmt.Println("Error closing file:", err)
		}
	}()

	if _, err = io.Copy(dest, file); err != nil {
		return "", err
	}

	return newFilename, nil
}

func (s *LocalStorage) Delete(path string) error {
	return os.Remove(filepath.Join(s.BasePath, path))
}

func (s *LocalStorage) URL(path string) string {
	return fmt.Sprintf("%s/v1/files/%s", s.BaseURL, path)
}

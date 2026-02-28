package storage

import "mime/multipart"

type FileStorage interface {
	Save(file multipart.File, filename string, mimeType string) (path string, err error)
	Delete(path string) error
	URL(path string) string
}

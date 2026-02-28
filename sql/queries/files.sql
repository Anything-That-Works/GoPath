-- name: CreateFile :one
INSERT INTO files (uploader_id, name, mime_type, size, path)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFileByID :one
SELECT * FROM files WHERE id = $1;

-- name: DeleteFile :exec
DELETE FROM files WHERE id = $1 AND uploader_id = $2;
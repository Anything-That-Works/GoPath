-- name: CreateMessage :one
INSERT INTO messages (conversation_id, sender_id, content, file_id, reply_to_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMessagesByConversation :many
SELECT * FROM messages
WHERE conversation_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = $1;

-- name: EditMessage :one
UPDATE messages
SET content = $2, is_edited = TRUE, updated_at = NOW()
WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMessage :exec
UPDATE messages
SET deleted_at = NOW()
WHERE id = $1 AND sender_id = $2;

-- name: UpsertMessageReceipt :exec
INSERT INTO message_receipts (message_id, user_id, delivered_at)
VALUES ($1, $2, NOW())
ON CONFLICT (message_id, user_id) DO UPDATE
SET delivered_at = COALESCE(message_receipts.delivered_at, NOW());

-- name: MarkMessageRead :exec
INSERT INTO message_receipts (message_id, user_id, read_at)
VALUES ($1, $2, NOW())
ON CONFLICT (message_id, user_id) DO UPDATE
SET read_at = NOW();

-- name: GetMessageReceipts :many
SELECT * FROM message_receipts WHERE message_id = $1;
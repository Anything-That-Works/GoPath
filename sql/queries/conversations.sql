-- name: CreateConversation :one
INSERT INTO conversations (created_by, is_group, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetConversationByID :one
SELECT * FROM conversations WHERE id = $1;

-- name: GetDirectConversation :one
SELECT c.* FROM conversations c
JOIN conversation_members cm1 ON cm1.conversation_id = c.id AND cm1.user_id = $1
JOIN conversation_members cm2 ON cm2.conversation_id = c.id AND cm2.user_id = $2
WHERE c.is_group = FALSE
LIMIT 1;

-- name: GetUserConversations :many
SELECT c.* FROM conversations c
JOIN conversation_members cm ON cm.conversation_id = c.id
WHERE cm.user_id = $1
ORDER BY c.updated_at DESC
LIMIT $2 OFFSET $3;

-- name: AddConversationMember :exec
INSERT INTO conversation_members (conversation_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: RemoveConversationMember :exec
DELETE FROM conversation_members
WHERE conversation_id = $1 AND user_id = $2;

-- name: SetMemberRole :exec
UPDATE conversation_members
SET role = $3
WHERE conversation_id = $1 AND user_id = $2;

-- name: GetConversationMember :one
SELECT * FROM conversation_members
WHERE conversation_id = $1 AND user_id = $2;

-- name: GetConversationMembers :many
SELECT user_id, role, joined_at, last_read_at
FROM conversation_members
WHERE conversation_id = $1
ORDER BY joined_at ASC;

-- name: GetFirstAdminOrMember :one
SELECT user_id, role FROM conversation_members
WHERE conversation_id = $1 AND user_id != $2
ORDER BY
    CASE WHEN role = 'admin' THEN 0 ELSE 1 END,
    joined_at ASC
LIMIT 1;

-- name: UpdateLastRead :exec
UPDATE conversation_members
SET last_read_at = NOW()
WHERE conversation_id = $1 AND user_id = $2;

-- name: UpdateConversationName :one
UPDATE conversations
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateConversationTimestamp :exec
UPDATE conversations
SET updated_at = NOW()
WHERE id = $1;
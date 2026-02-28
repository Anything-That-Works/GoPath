-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, name, email, password_hash)
VALUES (gen_random_uuid(), NOW(), NOW(), $1, $2, $3 )
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: UserExistsByEmail :one
SELECT EXISTS ( SELECT 1 FROM users WHERE email = $1);

-- name: UpdateUser :one
UPDATE users 
SET 
    name = COALESCE($2, name),
    email = COALESCE($3, email),
    password_hash = COALESCE($4, password_hash),
    updated_at = NOW() 
WHERE id = $1 
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
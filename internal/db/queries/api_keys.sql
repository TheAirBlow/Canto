-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash) VALUES ($1, $2, $3) RETURNING *;

-- name: GetUserByAPIKeyHash :one
SELECT users.* FROM users
JOIN api_keys ON api_keys.user_id = users.id
WHERE api_keys.key_hash = $1;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE key_hash = $1;

-- name: ListAPIKeysForUser :many
SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at;

-- name: DeleteAPIKey :execrows
DELETE FROM api_keys WHERE id = $1 AND user_id = $2;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3) RETURNING *;

-- name: GetSessionUser :one
SELECT users.*, sessions.expires_at AS session_expires_at FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now();

-- name: DeleteOtherSessionsForUser :exec
DELETE FROM sessions WHERE user_id = $1 AND token_hash != $2;

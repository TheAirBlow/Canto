-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING *;

-- name: CreateAdminUser :one
INSERT INTO users (username, password_hash, is_admin) VALUES ($1, $2, TRUE) RETURNING *;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE is_admin = TRUE;

-- name: GetUsersByIDs :many
SELECT * FROM users WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListPublicUsers :many
SELECT * FROM users WHERE public AND id > sqlc.arg(after)::bigint ORDER BY id LIMIT sqlc.arg(max_rows)::int;

-- name: UpdateUserProfile :one
UPDATE users SET display_name = $2, description = $3, public = $4 WHERE id = $1 RETURNING *;

-- name: SetUserImage :one
UPDATE users SET image_id = $2 WHERE id = $1 RETURNING *;

-- name: UpdateUserUsername :one
UPDATE users SET username = $2 WHERE id = $1 RETURNING *;

-- name: UpdateUserPassword :one
UPDATE users SET password_hash = $2 WHERE id = $1 RETURNING *;

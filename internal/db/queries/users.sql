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

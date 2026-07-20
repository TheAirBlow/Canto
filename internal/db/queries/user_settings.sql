-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE user_id = $1;

-- name: UpsertUserSettings :one
INSERT INTO user_settings (user_id, settings) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET settings = excluded.settings
RETURNING *;

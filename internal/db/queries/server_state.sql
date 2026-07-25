-- name: GetServerState :one
SELECT clean_shutdown FROM server_state WHERE id = true;

-- name: MarkServerRunning :exec
UPDATE server_state SET clean_shutdown = false WHERE id = true;

-- name: MarkServerShutdownClean :exec
UPDATE server_state SET clean_shutdown = true WHERE id = true;

-- name: CreateListen :one
INSERT INTO listens (user_id, song_id, listened_at, client, duration_played_ms, extra)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: SearchListensForUser :many
SELECT l.id, l.song_id, l.listened_at
FROM listens l
JOIN songs s ON s.id = l.song_id
WHERE l.user_id = sqlc.arg(user_id)::bigint
  AND (
    s.name ILIKE '%' || sqlc.arg(query)::text || '%'
    OR EXISTS (
      SELECT 1 FROM song_artists sa JOIN artists a ON a.id = sa.artist_id
      WHERE sa.song_id = s.id AND a.name ILIKE '%' || sqlc.arg(query)::text || '%'
    )
  )
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int;

-- name: ListListensForUser :many
SELECT * FROM listens WHERE user_id = sqlc.arg(user_id)::bigint
ORDER BY listened_at
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

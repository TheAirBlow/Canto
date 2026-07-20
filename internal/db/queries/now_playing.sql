-- name: UpsertNowPlaying :exec
WITH expired AS (
    DELETE FROM now_playing
    WHERE user_id != sqlc.arg(user_id)::bigint
      AND started_at + (COALESCE(duration_ms, 0) || ' milliseconds')::interval < now()
)
INSERT INTO now_playing (user_id, song_id, started_at, duration_ms)
VALUES (sqlc.arg(user_id)::bigint, sqlc.arg(song_id)::bigint, now(), sqlc.arg(duration_ms)::integer)
ON CONFLICT (user_id) DO UPDATE SET
    song_id = excluded.song_id,
    started_at = now(),
    duration_ms = excluded.duration_ms;

-- name: GetNowPlaying :one
SELECT * FROM now_playing
WHERE user_id = $1
  AND started_at + (COALESCE(duration_ms, 0) || ' milliseconds')::interval >= now();

-- name: DeleteAllNowPlaying :exec
DELETE FROM now_playing;

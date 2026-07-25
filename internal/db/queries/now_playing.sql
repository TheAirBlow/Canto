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

-- name: ListNowPlayingForSong :many
-- user_id scopes to one caller-permitted user (bypassing u.public, already checked upstream); unset means every public user.
SELECT np.song_id, np.started_at, np.duration_ms, u.id AS user_id, u.username, u.display_name, u.image_id AS user_image_id
FROM now_playing np JOIN users u ON u.id = np.user_id
WHERE np.song_id = sqlc.arg(song_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NOT NULL OR u.public)
  AND (sqlc.narg(user_id)::bigint IS NULL OR u.id = sqlc.narg(user_id)::bigint)
  AND np.started_at + (COALESCE(np.duration_ms, 0) || ' milliseconds')::interval >= now();

-- name: ListNowPlayingForAlbum :many
-- See ListNowPlayingForSong for the user_id semantics.
SELECT np.song_id, np.started_at, np.duration_ms, u.id AS user_id, u.username, u.display_name, u.image_id AS user_image_id
FROM now_playing np
JOIN song_albums sa ON sa.song_id = np.song_id
JOIN users u ON u.id = np.user_id
WHERE sa.album_id = sqlc.arg(album_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NOT NULL OR u.public)
  AND (sqlc.narg(user_id)::bigint IS NULL OR u.id = sqlc.narg(user_id)::bigint)
  AND np.started_at + (COALESCE(np.duration_ms, 0) || ' milliseconds')::interval >= now();

-- name: ListNowPlayingForArtist :many
-- See ListNowPlayingForSong for the user_id semantics.
SELECT np.song_id, np.started_at, np.duration_ms, u.id AS user_id, u.username, u.display_name, u.image_id AS user_image_id
FROM now_playing np
JOIN song_artists sar ON sar.song_id = np.song_id
JOIN users u ON u.id = np.user_id
WHERE sar.artist_id = sqlc.arg(artist_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NOT NULL OR u.public)
  AND (sqlc.narg(user_id)::bigint IS NULL OR u.id = sqlc.narg(user_id)::bigint)
  AND np.started_at + (COALESCE(np.duration_ms, 0) || ' milliseconds')::interval >= now();

-- name: ListNowPlayingAllPublic :many
SELECT np.song_id, np.started_at, np.duration_ms, u.id AS user_id, u.username, u.display_name, u.image_id AS user_image_id
FROM now_playing np JOIN users u ON u.id = np.user_id
WHERE u.public
  AND np.started_at + (COALESCE(np.duration_ms, 0) || ' milliseconds')::interval >= now();

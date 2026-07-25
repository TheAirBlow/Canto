-- name: CreateListen :one
-- ON CONFLICT DO NOTHING absorbs a client retry or a re-run bulk import resubmitting the exact
-- same listen; the caller sees pgx.ErrNoRows and treats it as an already-recorded no-op.
INSERT INTO listens (user_id, song_id, listened_at, client, duration_played_ms, extra)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, song_id, listened_at) DO NOTHING
RETURNING *;

-- name: EarliestListenedAt :one
SELECT min(listened_at)::timestamptz FROM listens
WHERE sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id)::bigint;

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

-- name: ListListensForUserFiltered :many
SELECT l.* FROM listens l
WHERE l.user_id = sqlc.arg(user_id)::bigint
  AND l.listened_at >= sqlc.arg(from_time)::timestamptz
  AND l.listened_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(song_id)::bigint IS NULL OR l.song_id = sqlc.narg(song_id)::bigint)
  AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_albums sa WHERE sa.song_id = l.song_id AND sa.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_artists sar WHERE sar.song_id = l.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
  AND (sqlc.narg(source_type)::text IS NULL OR EXISTS (
        SELECT 1 FROM sources src WHERE src.entity_type = 'song' AND src.entity_id = l.song_id AND src.source_type = sqlc.narg(source_type)::text))
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: ListListensForSong :many
-- Eligibility-filtered (rollup.Eligible's threshold) so this list, its count, and the song's
-- entity_global_stats.plays all agree on what counts as a real listen. user_id optionally
-- scopes to one caller-permitted user (already checked upstream).
SELECT l.id, l.song_id, l.listened_at, u.id AS user_id, u.public, u.username, u.display_name, u.image_id AS user_image_id
FROM listens l
JOIN songs s ON s.id = l.song_id
JOIN users u ON u.id = l.user_id
WHERE l.song_id = sqlc.arg(song_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountListensForSong :one
SELECT count(*) FROM listens l
JOIN songs s ON s.id = l.song_id
WHERE l.song_id = sqlc.arg(song_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: CountListensForAlbum :one
SELECT count(*) FROM listens l
JOIN songs s ON s.id = l.song_id
JOIN song_albums sa ON sa.song_id = l.song_id
WHERE sa.album_id = sqlc.arg(album_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: CountListensForArtist :one
SELECT count(*) FROM listens l
JOIN songs s ON s.id = l.song_id
JOIN song_artists sar ON sar.song_id = l.song_id
WHERE sar.artist_id = sqlc.arg(artist_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: ListListensForAlbum :many
-- Eligibility-filtered; see ListListensForSong.
SELECT l.id, l.song_id, l.listened_at, u.id AS user_id, u.public, u.username, u.display_name, u.image_id AS user_image_id
FROM listens l
JOIN songs s ON s.id = l.song_id
JOIN song_albums sa ON sa.song_id = l.song_id
JOIN users u ON u.id = l.user_id
WHERE sa.album_id = sqlc.arg(album_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: ListListensForArtist :many
-- Eligibility-filtered; see ListListensForSong.
SELECT l.id, l.song_id, l.listened_at, u.id AS user_id, u.public, u.username, u.display_name, u.image_id AS user_image_id
FROM listens l
JOIN songs s ON s.id = l.song_id
JOIN song_artists sar ON sar.song_id = l.song_id
JOIN users u ON u.id = l.user_id
WHERE sar.artist_id = sqlc.arg(artist_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: CountListensForUserFiltered :one
SELECT count(*) FROM listens l
WHERE l.user_id = sqlc.arg(user_id)::bigint
  AND l.listened_at >= sqlc.arg(from_time)::timestamptz
  AND l.listened_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(song_id)::bigint IS NULL OR l.song_id = sqlc.narg(song_id)::bigint)
  AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_albums sa WHERE sa.song_id = l.song_id AND sa.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_artists sar WHERE sar.song_id = l.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
  AND (sqlc.narg(source_type)::text IS NULL OR EXISTS (
        SELECT 1 FROM sources src WHERE src.entity_type = 'song' AND src.entity_id = l.song_id AND src.source_type = sqlc.narg(source_type)::text));

-- name: ListListensAllFiltered :many
SELECT l.id, l.song_id, l.listened_at, l.duration_played_ms, u.id AS user_id, u.public, u.username, u.display_name, u.image_id AS user_image_id
FROM listens l
JOIN users u ON u.id = l.user_id
WHERE l.listened_at >= sqlc.arg(from_time)::timestamptz
  AND l.listened_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(song_id)::bigint IS NULL OR l.song_id = sqlc.narg(song_id)::bigint)
  AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_albums sa WHERE sa.song_id = l.song_id AND sa.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_artists sar WHERE sar.song_id = l.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
  AND (sqlc.narg(source_type)::text IS NULL OR EXISTS (
        SELECT 1 FROM sources src WHERE src.entity_type = 'song' AND src.entity_id = l.song_id AND src.source_type = sqlc.narg(source_type)::text))
ORDER BY l.listened_at DESC
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: DeleteListenForUser :one
DELETE FROM listens WHERE id = sqlc.arg(id)::bigint AND user_id = sqlc.arg(user_id)::bigint
RETURNING id, song_id, listened_at, duration_played_ms;

-- name: CountListensAllFiltered :one
SELECT count(*) FROM listens l
WHERE l.listened_at >= sqlc.arg(from_time)::timestamptz
  AND l.listened_at < sqlc.arg(to_time)::timestamptz
  AND (sqlc.narg(song_id)::bigint IS NULL OR l.song_id = sqlc.narg(song_id)::bigint)
  AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_albums sa WHERE sa.song_id = l.song_id AND sa.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_artists sar WHERE sar.song_id = l.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
  AND (sqlc.narg(source_type)::text IS NULL OR EXISTS (
        SELECT 1 FROM sources src WHERE src.entity_type = 'song' AND src.entity_id = l.song_id AND src.source_type = sqlc.narg(source_type)::text));

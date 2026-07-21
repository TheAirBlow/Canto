-- name: GetStatsCache :one
SELECT * FROM stats_cache
WHERE user_id IS NOT DISTINCT FROM sqlc.narg(user_id)::bigint
  AND artist_id IS NOT DISTINCT FROM sqlc.narg(artist_id)::bigint
  AND album_id IS NOT DISTINCT FROM sqlc.narg(album_id)::bigint
  AND song_id IS NOT DISTINCT FROM sqlc.narg(song_id)::bigint
  AND resource = sqlc.arg(resource)::stats_resource
  AND params = sqlc.arg(params)::jsonb;

-- name: UpsertStatsCache :one
INSERT INTO stats_cache (user_id, artist_id, album_id, song_id, resource, params, data)
VALUES (sqlc.narg(user_id)::bigint, sqlc.narg(artist_id)::bigint, sqlc.narg(album_id)::bigint, sqlc.narg(song_id)::bigint,
        sqlc.arg(resource)::stats_resource, sqlc.arg(params)::jsonb, sqlc.arg(data)::jsonb)
ON CONFLICT (user_id, artist_id, album_id, song_id, resource, params)
DO UPDATE SET data = excluded.data, computed_at = now()
RETURNING *;

-- name: ListStatsCache :many
SELECT * FROM stats_cache ORDER BY id;

-- name: SourcesBreakdown :many
-- Stays a live query (not rolled up): source_type can change on re-correlation, so a
-- rollup snapshot would go stale independent of new listens.
WITH user_listens AS (
    SELECT l.* FROM listens l
    JOIN songs s ON s.id = l.song_id
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
      AND l.listened_at >= sqlc.arg(from_time)::timestamptz
      AND l.listened_at < sqlc.arg(to_time)::timestamptz
      AND (s.duration_ms IS NULL OR s.duration_ms <= 0
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
      AND NOT EXISTS (
        SELECT 1 FROM artist_blacklist bl
        JOIN song_artists sa ON sa.artist_id = bl.artist_id
        WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
      )
),
primary_source AS (
    SELECT song_id, source_type FROM (
        SELECT entity_id AS song_id, source_type,
               row_number() OVER (PARTITION BY entity_id ORDER BY (correlation_method = 'source_id') DESC, id) AS rn
        FROM sources WHERE entity_type = 'song'
    ) t WHERE rn = 1
)
SELECT coalesce(ps.source_type::text, 'unknown') AS source_type, count(*)::bigint AS listen_count
FROM user_listens ul
LEFT JOIN primary_source ps ON ps.song_id = ul.song_id
GROUP BY coalesce(ps.source_type::text, 'unknown')
ORDER BY count(*) DESC;

-- name: InterestHistory :many
-- Stays a live query: already scoped to one entity's own listens, small and bounded.
SELECT l.listened_at
FROM listens l
JOIN songs s ON s.id = l.song_id
WHERE (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
  AND NOT EXISTS (
    SELECT 1 FROM artist_blacklist bl JOIN song_artists sa ON sa.artist_id = bl.artist_id
    WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
  )
  AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_artists sar WHERE sar.song_id = l.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
  AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM song_albums sa2 WHERE sa2.song_id = l.song_id AND sa2.album_id = sqlc.narg(album_id)::bigint))
  AND (sqlc.narg(song_id)::bigint IS NULL OR l.song_id = sqlc.narg(song_id)::bigint)
ORDER BY l.listened_at;

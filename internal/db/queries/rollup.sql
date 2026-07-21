-- name: CheckBlacklistedSongs :many
-- Returns every (user_id, song_id) pair, from the sets given, that is currently blacklisted.
-- The two arrays are checked as independent sets, not paired positionally, so callers must
-- re-check membership of their own (user_id, song_id) pairs against the result.
SELECT DISTINCT bl.user_id, sa.song_id
FROM artist_blacklist bl
JOIN song_artists sa ON sa.artist_id = bl.artist_id
WHERE bl.user_id = ANY(sqlc.arg(user_ids)::bigint[])
  AND sa.song_id = ANY(sqlc.arg(song_ids)::bigint[]);

-- name: UpsertDailySongListens :exec
INSERT INTO daily_song_listens (user_id, song_id, day, listen_count, minutes_ms)
SELECT unnest(sqlc.arg(user_ids)::bigint[]),
       unnest(sqlc.arg(song_ids)::bigint[]),
       unnest(sqlc.arg(days)::date[]),
       unnest(sqlc.arg(listen_counts)::int[]),
       unnest(sqlc.arg(minutes_ms)::bigint[])
ON CONFLICT (user_id, song_id, day) DO UPDATE SET
    listen_count = daily_song_listens.listen_count + excluded.listen_count,
    minutes_ms = daily_song_listens.minutes_ms + excluded.minutes_ms;

-- name: UpsertClockCells :exec
INSERT INTO clock_cells (user_id, day, hour, listen_count)
SELECT unnest(sqlc.arg(user_ids)::bigint[]),
       unnest(sqlc.arg(days)::date[]),
       unnest(sqlc.arg(hours)::smallint[]),
       unnest(sqlc.arg(listen_counts)::int[])
ON CONFLICT (user_id, day, hour) DO UPDATE SET
    listen_count = clock_cells.listen_count + excluded.listen_count;

-- name: InsertFirstListens :many
-- Candidates must already be deduped to one row per (user_id, entity_type, entity_id),
-- keeping the earliest first_at, before calling this. Returned rows are genuine first
-- sightings (the discoveries in this batch); rows silently dropped by DO NOTHING already existed.
INSERT INTO user_entity_first_listen (user_id, entity_type, entity_id, first_at)
SELECT unnest(sqlc.arg(user_ids)::bigint[]),
       unnest(sqlc.arg(entity_types)::text[])::entity_type,
       unnest(sqlc.arg(entity_ids)::bigint[]),
       unnest(sqlc.arg(first_ats)::timestamptz[])
ON CONFLICT (user_id, entity_type, entity_id) DO NOTHING
RETURNING user_id, entity_type, entity_id;

-- name: UpsertEntityGlobalPlays :exec
INSERT INTO entity_global_stats (entity_type, entity_id, plays, first_listened_at)
SELECT unnest(sqlc.arg(entity_types)::text[])::entity_type,
       unnest(sqlc.arg(entity_ids)::bigint[]),
       unnest(sqlc.arg(plays)::int[]),
       unnest(sqlc.arg(first_ats)::timestamptz[])
ON CONFLICT (entity_type, entity_id) DO UPDATE SET
    plays = entity_global_stats.plays + excluded.plays,
    first_listened_at = LEAST(entity_global_stats.first_listened_at, excluded.first_listened_at);

-- name: BumpEntityGlobalUniqueListeners :exec
-- entity_types/entity_ids/counts must already be aggregated (one row per entity, count =
-- how many of that entity's rows InsertFirstListens actually returned in this batch).
INSERT INTO entity_global_stats (entity_type, entity_id, unique_listeners)
SELECT unnest(sqlc.arg(entity_types)::text[])::entity_type,
       unnest(sqlc.arg(entity_ids)::bigint[]),
       unnest(sqlc.arg(counts)::int[])
ON CONFLICT (entity_type, entity_id) DO UPDATE SET
    unique_listeners = entity_global_stats.unique_listeners + excluded.unique_listeners;

-- name: GetUserListenStates :many
SELECT * FROM user_listen_state WHERE user_id = ANY(sqlc.arg(user_ids)::bigint[]);

-- name: UpsertUserListenStates :exec
INSERT INTO user_listen_state (user_id, last_listened_at, current_streak, longest_streak)
SELECT unnest(sqlc.arg(user_ids)::bigint[]),
       unnest(sqlc.arg(last_listened_ats)::timestamptz[]),
       unnest(sqlc.arg(current_streaks)::int[]),
       unnest(sqlc.arg(longest_streaks)::int[])
ON CONFLICT (user_id) DO UPDATE SET
    last_listened_at = excluded.last_listened_at,
    current_streak = excluded.current_streak,
    longest_streak = excluded.longest_streak;

-- name: ComputeUserStreak :one
-- All-time streak recompute from daily_song_listens, used to reconcile user_listen_state
-- after a bulk import (whose entries arrive out of chronological order) and by the admin
-- full-recompute command.
WITH days AS (
    SELECT DISTINCT day FROM daily_song_listens WHERE user_id = sqlc.arg(user_id)::bigint
),
day_groups AS (SELECT day, day - (row_number() OVER (ORDER BY day)) * INTERVAL '1 day' AS g FROM days),
streaks AS (SELECT g, count(*) AS len, max(day) AS last_day FROM day_groups GROUP BY g)
SELECT
    coalesce((SELECT max(len) FROM streaks), 0)::bigint AS longest_streak,
    coalesce((SELECT len FROM streaks WHERE last_day >= (now() AT TIME ZONE 'UTC')::date - 1 ORDER BY last_day DESC LIMIT 1), 0)::bigint AS current_streak,
    (SELECT max(listened_at) FROM listens WHERE user_id = sqlc.arg(user_id)::bigint)::timestamptz AS last_listened_at;

-- name: RollupLongestStreak :one
-- Range-scoped, per-user only (a "streak" spanning every user isn't a meaningful concept).
WITH days AS (
    SELECT DISTINCT day FROM daily_song_listens
    WHERE user_id = sqlc.arg(user_id)::bigint AND day >= sqlc.arg(from_day)::date AND day < sqlc.arg(to_day)::date
),
day_groups AS (SELECT day, day - (row_number() OVER (ORDER BY day)) * INTERVAL '1 day' AS g FROM days),
streaks AS (SELECT count(*) AS len FROM day_groups GROUP BY g)
SELECT coalesce(max(len), 0)::bigint AS longest_streak FROM streaks;

-- name: RollupSummaryStats :one
WITH scoped AS (
    SELECT * FROM daily_song_listens dsl
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
      AND dsl.day >= sqlc.arg(from_day)::date
      AND dsl.day < sqlc.arg(to_day)::date
)
SELECT
    (SELECT coalesce(sum(listen_count), 0) FROM scoped)::bigint AS listen_count,
    (SELECT count(DISTINCT song_id) FROM scoped)::bigint AS unique_tracks,
    (SELECT count(DISTINCT sa.album_id) FROM scoped s JOIN song_albums sa ON sa.song_id = s.song_id)::bigint AS unique_albums,
    (SELECT count(DISTINCT sar.artist_id) FROM scoped s JOIN song_artists sar ON sar.song_id = s.song_id)::bigint AS unique_artists,
    ((SELECT coalesce(sum(minutes_ms), 0) FROM scoped)::float8 / 60000.0)::float8 AS minutes_listened,
    (SELECT count(DISTINCT day) FROM scoped)::bigint AS days_active;

-- name: RollupAvgSessionLengthMs :one
-- Sessions need exact listen timestamps, which day-grain rollups can't reconstruct, so
-- this stays a live query over listens (bounded to one user + range, index-backed).
WITH scoped AS (
    SELECT l.* FROM listens l
    WHERE l.user_id = sqlc.arg(user_id)::bigint
      AND l.listened_at >= sqlc.arg(from_time)::timestamptz
      AND l.listened_at < sqlc.arg(to_time)::timestamptz
),
prev_listen AS (SELECT listened_at, lag(listened_at) OVER (ORDER BY listened_at) AS prev_at FROM scoped),
session_starts AS (
    SELECT listened_at, CASE WHEN prev_at IS NULL OR listened_at - prev_at > INTERVAL '5 minutes' THEN 1 ELSE 0 END AS is_new
    FROM prev_listen
),
sessions AS (SELECT listened_at, sum(is_new) OVER (ORDER BY listened_at) AS session_id FROM session_starts),
session_spans AS (SELECT session_id, min(listened_at) AS start_at, max(listened_at) AS end_at FROM sessions GROUP BY session_id)
SELECT coalesce(avg(extract(epoch FROM end_at) - extract(epoch FROM start_at)) * 1000, 0)::float8 AS avg_session_length_ms FROM session_spans;

-- name: RollupTopArtists :many
WITH scoped AS (
    SELECT * FROM daily_song_listens dsl
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
      AND dsl.day >= sqlc.arg(from_day)::date
      AND dsl.day < sqlc.arg(to_day)::date
)
SELECT sar.artist_id, a.name, sum(s.listen_count)::bigint AS listen_count
FROM scoped s
JOIN song_artists sar ON sar.song_id = s.song_id
JOIN artists a ON a.id = sar.artist_id
GROUP BY sar.artist_id, a.name
ORDER BY listen_count DESC, a.name
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: RollupTopAlbums :many
WITH scoped AS (
    SELECT * FROM daily_song_listens dsl
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
      AND dsl.day >= sqlc.arg(from_day)::date
      AND dsl.day < sqlc.arg(to_day)::date
)
SELECT sa.album_id, al.name, sum(s.listen_count)::bigint AS listen_count
FROM scoped s
JOIN song_albums sa ON sa.song_id = s.song_id
JOIN albums al ON al.id = sa.album_id
GROUP BY sa.album_id, al.name
ORDER BY listen_count DESC, al.name
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: RollupTopTracks :many
WITH scoped AS (
    SELECT * FROM daily_song_listens dsl
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
      AND dsl.day >= sqlc.arg(from_day)::date
      AND dsl.day < sqlc.arg(to_day)::date
      AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
            SELECT 1 FROM song_artists sar WHERE sar.song_id = dsl.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
      AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
            SELECT 1 FROM song_albums sa2 WHERE sa2.song_id = dsl.song_id AND sa2.album_id = sqlc.narg(album_id)::bigint))
)
SELECT s.song_id, sg.name, sum(s.listen_count)::bigint AS listen_count
FROM scoped s
JOIN songs sg ON sg.id = s.song_id
GROUP BY s.song_id, sg.name
ORDER BY listen_count DESC, sg.name
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(row_offset)::int;

-- name: RollupActivityBuckets :many
WITH scoped AS (
    SELECT * FROM daily_song_listens dsl
    WHERE (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
      AND (sqlc.narg(artist_id)::bigint IS NULL OR EXISTS (
            SELECT 1 FROM song_artists sar WHERE sar.song_id = dsl.song_id AND sar.artist_id = sqlc.narg(artist_id)::bigint))
      AND (sqlc.narg(album_id)::bigint IS NULL OR EXISTS (
            SELECT 1 FROM song_albums sa2 WHERE sa2.song_id = dsl.song_id AND sa2.album_id = sqlc.narg(album_id)::bigint))
      AND (sqlc.narg(song_id)::bigint IS NULL OR dsl.song_id = sqlc.narg(song_id)::bigint)
)
SELECT gs.bucket, coalesce(sum(sc.listen_count), 0)::bigint AS listen_count
FROM generate_series(sqlc.arg(from_time)::timestamptz, sqlc.arg(to_time)::timestamptz, sqlc.arg(step)::interval) AS gs(bucket)
LEFT JOIN scoped sc
  ON (sc.day::timestamp AT TIME ZONE 'UTC') >= gs.bucket AND (sc.day::timestamp AT TIME ZONE 'UTC') < gs.bucket + sqlc.arg(step)::interval
GROUP BY gs.bucket ORDER BY gs.bucket;

-- name: RollupDiscoveryBuckets :many
WITH buckets AS (
    SELECT bucket FROM generate_series(sqlc.arg(from_time)::timestamptz, sqlc.arg(to_time)::timestamptz, sqlc.arg(step)::interval) AS gs(bucket)
),
totals AS (
    SELECT b.bucket, coalesce(sum(dsl.listen_count), 0)::bigint AS total
    FROM buckets b
    LEFT JOIN daily_song_listens dsl
      ON (dsl.day::timestamp AT TIME ZONE 'UTC') >= b.bucket AND (dsl.day::timestamp AT TIME ZONE 'UTC') < b.bucket + sqlc.arg(step)::interval
      AND (sqlc.narg(user_id)::bigint IS NULL OR dsl.user_id = sqlc.narg(user_id)::bigint)
    GROUP BY b.bucket
),
discoveries AS (
    SELECT b.bucket, count(fs.first_at)::bigint AS discoveries
    FROM buckets b
    LEFT JOIN user_entity_first_listen fs
      ON fs.entity_type = 'song' AND fs.first_at >= b.bucket AND fs.first_at < b.bucket + sqlc.arg(step)::interval
      AND (sqlc.narg(user_id)::bigint IS NULL OR fs.user_id = sqlc.narg(user_id)::bigint)
    GROUP BY b.bucket
)
SELECT t.bucket, t.total, d.discoveries
FROM totals t JOIN discoveries d ON d.bucket = t.bucket
ORDER BY t.bucket;

-- name: RollupClockGrid :many
SELECT extract(dow FROM day)::smallint AS day_of_week, hour, sum(listen_count)::bigint AS listen_count
FROM clock_cells
WHERE (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id)::bigint)
  AND day >= sqlc.arg(from_day)::date
  AND day < sqlc.arg(to_day)::date
GROUP BY extract(dow FROM day), hour;

-- name: RollupTopDay :one
SELECT day, sum(listen_count)::bigint AS listen_count
FROM daily_song_listens
WHERE user_id = sqlc.arg(user_id)::bigint
  AND day >= sqlc.arg(from_day)::date
  AND day < sqlc.arg(to_day)::date
GROUP BY day
ORDER BY listen_count DESC, day
LIMIT 1;

-- name: RollupNewEntityCounts :one
SELECT
    count(*) FILTER (WHERE entity_type = 'song')::bigint AS new_tracks,
    count(*) FILTER (WHERE entity_type = 'album')::bigint AS new_albums,
    count(*) FILTER (WHERE entity_type = 'artist')::bigint AS new_artists
FROM user_entity_first_listen
WHERE user_id = sqlc.arg(user_id)::bigint
  AND first_at >= sqlc.arg(from_time)::timestamptz
  AND first_at < sqlc.arg(to_time)::timestamptz;

-- name: RollupEntitySummary :one
SELECT plays, unique_listeners, first_listened_at
FROM entity_global_stats
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: TruncateRollupTables :exec
TRUNCATE daily_song_listens, clock_cells, user_listen_state, user_entity_first_listen, entity_global_stats;

-- name: RebuildDailySongListens :exec
-- Mirrors rollup.Eligible's threshold exactly.
WITH eligible_listens AS (
    SELECT l.user_id, l.song_id, l.listened_at,
           coalesce(l.duration_played_ms, s.duration_ms, 0) AS played_ms
    FROM listens l
    JOIN songs s ON s.id = l.song_id
    WHERE (s.duration_ms IS NULL OR s.duration_ms <= 0
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
      AND NOT EXISTS (
        SELECT 1 FROM artist_blacklist bl JOIN song_artists sa ON sa.artist_id = bl.artist_id
        WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
      )
)
INSERT INTO daily_song_listens (user_id, song_id, day, listen_count, minutes_ms)
SELECT user_id, song_id, (listened_at AT TIME ZONE 'UTC')::date, count(*)::int, coalesce(sum(played_ms), 0)::bigint
FROM eligible_listens
GROUP BY user_id, song_id, (listened_at AT TIME ZONE 'UTC')::date;

-- name: RebuildClockCells :exec
WITH eligible_listens AS (
    SELECT l.user_id, l.song_id, l.listened_at,
           coalesce(l.duration_played_ms, s.duration_ms, 0) AS played_ms
    FROM listens l
    JOIN songs s ON s.id = l.song_id
    WHERE (s.duration_ms IS NULL OR s.duration_ms <= 0
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
      AND NOT EXISTS (
        SELECT 1 FROM artist_blacklist bl JOIN song_artists sa ON sa.artist_id = bl.artist_id
        WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
      )
)
INSERT INTO clock_cells (user_id, day, hour, listen_count)
SELECT user_id, (listened_at AT TIME ZONE 'UTC')::date, extract(hour FROM listened_at AT TIME ZONE 'UTC')::smallint, count(*)::int
FROM eligible_listens
GROUP BY user_id, (listened_at AT TIME ZONE 'UTC')::date, extract(hour FROM listened_at AT TIME ZONE 'UTC');

-- name: RebuildEntityFirstListens :exec
WITH eligible_listens AS (
    SELECT l.user_id, l.song_id, l.listened_at,
           coalesce(l.duration_played_ms, s.duration_ms, 0) AS played_ms
    FROM listens l
    JOIN songs s ON s.id = l.song_id
    WHERE (s.duration_ms IS NULL OR s.duration_ms <= 0
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
      AND NOT EXISTS (
        SELECT 1 FROM artist_blacklist bl JOIN song_artists sa ON sa.artist_id = bl.artist_id
        WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
      )
)
INSERT INTO user_entity_first_listen (user_id, entity_type, entity_id, first_at)
SELECT user_id, 'song'::entity_type, song_id, min(listened_at) FROM eligible_listens GROUP BY user_id, song_id
UNION ALL
SELECT el.user_id, 'album'::entity_type, sa.album_id, min(el.listened_at)
FROM eligible_listens el JOIN song_albums sa ON sa.song_id = el.song_id
GROUP BY el.user_id, sa.album_id
UNION ALL
SELECT el.user_id, 'artist'::entity_type, sar.artist_id, min(el.listened_at)
FROM eligible_listens el JOIN song_artists sar ON sar.song_id = el.song_id
GROUP BY el.user_id, sar.artist_id;

-- name: RebuildEntityGlobalStats :exec
WITH eligible_listens AS (
    SELECT l.user_id, l.song_id, l.listened_at,
           coalesce(l.duration_played_ms, s.duration_ms, 0) AS played_ms
    FROM listens l
    JOIN songs s ON s.id = l.song_id
    WHERE (s.duration_ms IS NULL OR s.duration_ms <= 0
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
           OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000)
      AND NOT EXISTS (
        SELECT 1 FROM artist_blacklist bl JOIN song_artists sa ON sa.artist_id = bl.artist_id
        WHERE bl.user_id = l.user_id AND sa.song_id = l.song_id
      )
)
INSERT INTO entity_global_stats (entity_type, entity_id, plays, unique_listeners, first_listened_at)
SELECT entity_type, entity_id, count(*)::int AS plays, count(DISTINCT user_id)::int AS unique_listeners, min(first_at)
FROM (
    SELECT user_id, 'song'::entity_type AS entity_type, song_id AS entity_id, listened_at AS first_at FROM eligible_listens
    UNION ALL
    SELECT el.user_id, 'album'::entity_type, sa.album_id, el.listened_at
    FROM eligible_listens el JOIN song_albums sa ON sa.song_id = el.song_id
    UNION ALL
    SELECT el.user_id, 'artist'::entity_type, sar.artist_id, el.listened_at
    FROM eligible_listens el JOIN song_artists sar ON sar.song_id = el.song_id
) t
GROUP BY entity_type, entity_id;

-- name: RebuildUserListenStates :exec
-- Must run after RebuildDailySongListens (reuses daily_song_listens for the day-grain streak scan).
WITH days AS (
    SELECT DISTINCT user_id, day FROM daily_song_listens
),
day_groups AS (
    SELECT user_id, day, day - (row_number() OVER (PARTITION BY user_id ORDER BY day)) * INTERVAL '1 day' AS g
    FROM days
),
streaks AS (SELECT user_id, g, count(*) AS len, max(day) AS last_day FROM day_groups GROUP BY user_id, g),
longest AS (SELECT user_id, max(len) AS longest_streak FROM streaks GROUP BY user_id),
current AS (
    SELECT DISTINCT ON (user_id) user_id, len AS current_streak
    FROM streaks
    WHERE last_day >= (now() AT TIME ZONE 'UTC')::date - 1
    ORDER BY user_id, last_day DESC
),
last_at AS (SELECT user_id, max(listened_at) AS last_listened_at FROM listens GROUP BY user_id)
INSERT INTO user_listen_state (user_id, last_listened_at, current_streak, longest_streak)
SELECT la.user_id, la.last_listened_at, coalesce(c.current_streak, 0)::int, coalesce(l.longest_streak, 0)::int
FROM last_at la
LEFT JOIN longest l ON l.user_id = la.user_id
LEFT JOIN current c ON c.user_id = la.user_id;

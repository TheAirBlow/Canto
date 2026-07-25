-- name: DecrementDailySongListens :exec
UPDATE daily_song_listens SET listen_count = listen_count - 1, played_ms = GREATEST(played_ms - sqlc.arg(played_ms)::bigint, 0)
WHERE user_id = sqlc.arg(user_id)::bigint AND song_id = sqlc.arg(song_id)::bigint AND day = sqlc.arg(day)::date;

-- name: PruneEmptyDailySongListens :exec
DELETE FROM daily_song_listens
WHERE user_id = sqlc.arg(user_id)::bigint AND song_id = sqlc.arg(song_id)::bigint AND day = sqlc.arg(day)::date AND listen_count <= 0;

-- name: DecrementClockCell :exec
UPDATE clock_cells SET listen_count = listen_count - 1
WHERE user_id = sqlc.arg(user_id)::bigint AND day = sqlc.arg(day)::date AND hour = sqlc.arg(hour)::smallint;

-- name: PruneEmptyClockCell :exec
DELETE FROM clock_cells
WHERE user_id = sqlc.arg(user_id)::bigint AND day = sqlc.arg(day)::date AND hour = sqlc.arg(hour)::smallint AND listen_count <= 0;

-- name: RemainingUserSongPlays :one
SELECT count(*)::bigint FROM listens l JOIN songs s ON s.id = l.song_id
WHERE l.user_id = sqlc.arg(user_id)::bigint AND l.song_id = sqlc.arg(song_id)::bigint
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: RemainingUserAlbumPlays :one
SELECT count(*)::bigint FROM listens l JOIN songs s ON s.id = l.song_id JOIN song_albums sa ON sa.song_id = l.song_id
WHERE l.user_id = sqlc.arg(user_id)::bigint AND sa.album_id = sqlc.arg(album_id)::bigint
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: RemainingUserArtistPlays :one
SELECT count(*)::bigint FROM listens l JOIN songs s ON s.id = l.song_id JOIN song_artists sar ON sar.song_id = l.song_id
WHERE l.user_id = sqlc.arg(user_id)::bigint AND sar.artist_id = sqlc.arg(artist_id)::bigint
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: DecrementEntityGlobalPlays :exec
UPDATE entity_global_stats SET plays = plays - 1, played_ms = GREATEST(played_ms - sqlc.arg(played_ms)::bigint, 0)
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: DecrementEntityGlobalUniqueListeners :exec
UPDATE entity_global_stats SET unique_listeners = unique_listeners - 1
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: PruneEmptyEntityGlobalStats :exec
DELETE FROM entity_global_stats
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint AND plays <= 0;

-- name: GetEntityGlobalFirstListenedAt :one
SELECT first_listened_at FROM entity_global_stats
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: SetEntityGlobalFirstListenedAt :exec
UPDATE entity_global_stats SET first_listened_at = sqlc.arg(first_listened_at)::timestamptz
WHERE entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: GetFirstListenForUserEntity :one
SELECT first_at FROM user_entity_first_listen
WHERE user_id = sqlc.arg(user_id)::bigint AND entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: SetFirstListenForUserEntity :exec
UPDATE user_entity_first_listen SET first_at = sqlc.arg(first_at)::timestamptz
WHERE user_id = sqlc.arg(user_id)::bigint AND entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: DeleteFirstListenForUserEntity :exec
DELETE FROM user_entity_first_listen
WHERE user_id = sqlc.arg(user_id)::bigint AND entity_type = sqlc.arg(entity_type)::entity_type AND entity_id = sqlc.arg(entity_id)::bigint;

-- name: MinEligibleListenedAtForSong :one
SELECT min(l.listened_at)::timestamptz FROM listens l JOIN songs s ON s.id = l.song_id
WHERE l.song_id = sqlc.arg(song_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: MinEligibleListenedAtForAlbum :one
SELECT min(l.listened_at)::timestamptz FROM listens l JOIN songs s ON s.id = l.song_id JOIN song_albums sa ON sa.song_id = l.song_id
WHERE sa.album_id = sqlc.arg(album_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: MinEligibleListenedAtForArtist :one
SELECT min(l.listened_at)::timestamptz FROM listens l JOIN songs s ON s.id = l.song_id JOIN song_artists sar ON sar.song_id = l.song_id
WHERE sar.artist_id = sqlc.arg(artist_id)::bigint
  AND (sqlc.narg(user_id)::bigint IS NULL OR l.user_id = sqlc.narg(user_id)::bigint)
  AND (s.duration_ms IS NULL OR s.duration_ms <= 0
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) * 2 >= s.duration_ms
       OR coalesce(l.duration_played_ms, s.duration_ms, 0) >= 240000);

-- name: GetSongByID :one
SELECT * FROM songs WHERE id = $1;

-- name: GetSongsByIDs :many
SELECT * FROM songs WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: FindSongBySource :one
SELECT s.* FROM songs s
JOIN sources src ON src.entity_type = 'song' AND src.entity_id = s.id
WHERE src.source_type = $1 AND src.extracted_id = $2;

-- name: FindSongByExactName :one
SELECT * FROM songs WHERE name = $1 LIMIT 1;

-- name: FindSongByExactNameForArtists :one
SELECT s.* FROM songs s
JOIN song_artists sa ON sa.song_id = s.id
WHERE s.name = sqlc.arg(name)::text AND sa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[])
LIMIT 1;

-- name: FindSongByExactNameForArtistsAndAlbum :one
SELECT s.* FROM songs s
JOIN song_artists sa ON sa.song_id = s.id
JOIN song_albums sal ON sal.song_id = s.id
WHERE s.name = sqlc.arg(name)::text
  AND sa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[])
  AND sal.album_id = sqlc.arg(album_id)::bigint
LIMIT 1;

-- name: FindSongsByExactName :many
SELECT * FROM songs WHERE name = $1;

-- name: FindSongsByExactNameForArtists :many
SELECT DISTINCT s.* FROM songs s
JOIN song_artists sa ON sa.song_id = s.id
WHERE s.name = sqlc.arg(name)::text AND sa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[]);

-- name: FindSongsByExactNameForArtistsAndAlbum :many
SELECT DISTINCT s.* FROM songs s
JOIN song_artists sa ON sa.song_id = s.id
JOIN song_albums sal ON sal.song_id = s.id
WHERE s.name = sqlc.arg(name)::text
  AND sa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[])
  AND sal.album_id = sqlc.arg(album_id)::bigint;

-- name: CreateSong :one
INSERT INTO songs (name, name_normalized, name_romanized, duration_ms) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateSong :one
UPDATE songs SET name = $2, name_normalized = $3, name_romanized = $4, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetSongImage :one
UPDATE songs SET image_id = $2, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetSongPinned :one
UPDATE songs SET pinned = $2 WHERE id = $1 RETURNING *;

-- name: DeleteSong :execrows
DELETE FROM songs WHERE id = $1;

-- name: LinkSongArtist :exec
INSERT INTO song_artists (song_id, artist_id, position) VALUES ($1, $2, $3)
ON CONFLICT (song_id, artist_id) DO UPDATE SET position = excluded.position;

-- name: DeleteSongArtists :exec
DELETE FROM song_artists WHERE song_id = $1;

-- name: ListSongArtists :many
SELECT * FROM song_artists WHERE song_id = $1 ORDER BY position;

-- name: LinkSongAlbum :exec
INSERT INTO song_albums (song_id, album_id, track_number) VALUES ($1, $2, $3)
ON CONFLICT (song_id, album_id) DO UPDATE SET track_number = COALESCE(song_albums.track_number, excluded.track_number);

-- name: UpdateSongThumbnail :exec
UPDATE songs SET image_id = $2, updated_at = now() WHERE id = $1 AND NOT pinned;

-- name: ListStaleSongs :many
SELECT * FROM songs WHERE updated_at < sqlc.arg(before)::timestamptz AND NOT pinned ORDER BY updated_at LIMIT sqlc.arg(max_rows)::int;

-- name: ListSongs :many
SELECT * FROM songs WHERE id > sqlc.arg(after)::bigint ORDER BY id LIMIT sqlc.arg(max_rows)::int;

-- name: ListArtistsForSong :many
SELECT a.* FROM artists a
JOIN song_artists sa ON sa.artist_id = a.id
WHERE sa.song_id = $1
ORDER BY sa.position;

-- name: GetAlbumForSong :one
SELECT al.*, sal.track_number FROM albums al
JOIN song_albums sal ON sal.album_id = al.id
WHERE sal.song_id = $1
LIMIT 1;

-- name: GetSongsPrimaryArtistAlbum :many
SELECT s.id AS song_id,
       coalesce((SELECT ar.id FROM song_artists sa JOIN artists ar ON ar.id = sa.artist_id
        WHERE sa.song_id = s.id ORDER BY sa.position LIMIT 1), 0)::bigint AS artist_id,
       coalesce((SELECT ar.name FROM song_artists sa JOIN artists ar ON ar.id = sa.artist_id
        WHERE sa.song_id = s.id ORDER BY sa.position LIMIT 1), '')::text AS artist_name,
       coalesce((SELECT al.id FROM song_albums sal JOIN albums al ON al.id = sal.album_id
        WHERE sal.song_id = s.id ORDER BY sal.track_number LIMIT 1), 0)::bigint AS album_id,
       coalesce((SELECT al.name FROM song_albums sal JOIN albums al ON al.id = sal.album_id
        WHERE sal.song_id = s.id ORDER BY sal.track_number LIMIT 1), '')::text AS album_name
FROM songs s
WHERE s.id = ANY(sqlc.arg(song_ids)::bigint[]);

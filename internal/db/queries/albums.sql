-- name: GetAlbumByID :one
SELECT * FROM albums WHERE id = $1;

-- name: GetAlbumsByIDs :many
SELECT * FROM albums WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: FindAlbumBySource :one
SELECT a.* FROM albums a
JOIN sources s ON s.entity_type = 'album' AND s.entity_id = a.id
WHERE s.source_type = $1 AND s.extracted_id = $2;

-- name: FindAlbumByExactName :one
SELECT * FROM albums WHERE name = $1 LIMIT 1;

-- name: FindAlbumByExactNameForArtists :one
SELECT a.* FROM albums a
JOIN album_artists aa ON aa.album_id = a.id
WHERE a.name = sqlc.arg(name)::text AND aa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[])
LIMIT 1;

-- name: FindAlbumsByExactName :many
SELECT * FROM albums WHERE name = $1;

-- name: FindAlbumsByExactNameForArtists :many
SELECT DISTINCT a.* FROM albums a
JOIN album_artists aa ON aa.album_id = a.id
WHERE a.name = sqlc.arg(name)::text AND aa.artist_id = ANY(sqlc.arg(artist_ids)::bigint[]);

-- name: CreateAlbum :one
INSERT INTO albums (name, name_normalized, name_romanized, release_date) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateAlbum :one
UPDATE albums SET name = $2, name_normalized = $3, name_romanized = $4, description = $5, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetAlbumImage :one
UPDATE albums SET image_id = $2, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetAlbumPinned :one
UPDATE albums SET pinned = $2 WHERE id = $1 RETURNING *;

-- name: DeleteAlbum :execrows
DELETE FROM albums WHERE id = $1;

-- name: LinkAlbumArtist :exec
INSERT INTO album_artists (album_id, artist_id, position) VALUES ($1, $2, $3)
ON CONFLICT (album_id, artist_id) DO UPDATE SET position = excluded.position;

-- name: DeleteAlbumArtists :exec
DELETE FROM album_artists WHERE album_id = $1;

-- name: ListAlbumArtists :many
SELECT * FROM album_artists WHERE album_id = $1 ORDER BY position;

-- name: UpdateAlbumMetadata :exec
UPDATE albums SET description = $2, image_id = $3, updated_at = now() WHERE id = $1 AND NOT pinned;

-- name: ListStaleAlbums :many
SELECT * FROM albums WHERE updated_at < sqlc.arg(before)::timestamptz AND NOT pinned ORDER BY updated_at LIMIT sqlc.arg(max_rows)::int;

-- name: ListAlbums :many
SELECT * FROM albums WHERE id > sqlc.arg(after)::bigint ORDER BY id LIMIT sqlc.arg(max_rows)::int;

-- name: ListArtistsForAlbum :many
SELECT a.* FROM artists a
JOIN album_artists aa ON aa.artist_id = a.id
WHERE aa.album_id = $1
ORDER BY aa.position;

-- name: ListSongsForAlbum :many
SELECT s.*, sal.track_number FROM songs s
JOIN song_albums sal ON sal.song_id = s.id
WHERE sal.album_id = $1
ORDER BY sal.track_number NULLS LAST, s.name;

-- name: ListSongIDsForAlbum :many
SELECT song_id FROM song_albums WHERE album_id = $1;

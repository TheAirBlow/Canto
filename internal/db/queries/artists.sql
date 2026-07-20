-- name: GetArtistByID :one
SELECT * FROM artists WHERE id = $1;

-- name: GetArtistsByIDs :many
SELECT * FROM artists WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: FindArtistBySource :one
SELECT a.* FROM artists a
JOIN sources s ON s.entity_type = 'artist' AND s.entity_id = a.id
WHERE s.source_type = $1 AND s.extracted_id = $2;

-- name: FindArtistByExactName :one
SELECT * FROM artists WHERE name = $1 OR name_normalized = $1 LIMIT 1;

-- name: CreateArtist :one
INSERT INTO artists (name, name_normalized) VALUES ($1, $2) RETURNING *;

-- name: UpdateArtist :one
UPDATE artists SET name = $2, name_normalized = $3, description = $4, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: UpdateArtistMetadata :exec
UPDATE artists SET description = $2, image_id = $3, updated_at = now() WHERE id = $1 AND NOT pinned;

-- name: SetArtistImage :one
UPDATE artists SET image_id = $2, pinned = TRUE, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetArtistPinned :one
UPDATE artists SET pinned = $2 WHERE id = $1 RETURNING *;

-- name: DeleteArtist :execrows
DELETE FROM artists WHERE id = $1;

-- name: ListStaleArtists :many
SELECT * FROM artists WHERE updated_at < sqlc.arg(before)::timestamptz AND NOT pinned ORDER BY updated_at LIMIT sqlc.arg(max_rows)::int;

-- name: ListArtists :many
SELECT * FROM artists WHERE id > sqlc.arg(after)::bigint ORDER BY id LIMIT sqlc.arg(max_rows)::int;

-- name: ListAlbumsForArtist :many
SELECT al.* FROM albums al
JOIN album_artists aa ON aa.album_id = al.id
WHERE aa.artist_id = $1
ORDER BY al.release_date NULLS LAST, al.name;

-- name: ListSongsForArtist :many
SELECT s.* FROM songs s
JOIN song_artists sa ON sa.song_id = s.id
WHERE sa.artist_id = $1
ORDER BY s.name;

-- name: TrigramMatchArtistsByNormalized :many
SELECT id, similarity(name_normalized, sqlc.arg(query)::text)::float8 AS sim FROM artists
WHERE name_normalized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: TrigramMatchArtistsByRomanized :many
SELECT id, similarity(name_romanized, sqlc.arg(query)::text)::float8 AS sim FROM artists
WHERE name_romanized != '' AND name_romanized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: TrigramMatchAlbumsByNormalized :many
SELECT id, similarity(name_normalized, sqlc.arg(query)::text)::float8 AS sim FROM albums
WHERE name_normalized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: TrigramMatchAlbumsByRomanized :many
SELECT id, similarity(name_romanized, sqlc.arg(query)::text)::float8 AS sim FROM albums
WHERE name_romanized != '' AND name_romanized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: TrigramMatchSongsByNormalized :many
SELECT id, similarity(name_normalized, sqlc.arg(query)::text)::float8 AS sim FROM songs
WHERE name_normalized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: TrigramMatchSongsByRomanized :many
SELECT id, similarity(name_romanized, sqlc.arg(query)::text)::float8 AS sim FROM songs
WHERE name_romanized != '' AND name_romanized % sqlc.arg(query)::text
ORDER BY sim DESC LIMIT sqlc.arg(max_rows)::int;

-- name: BlacklistArtist :exec
INSERT INTO artist_blacklist (user_id, artist_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UnblacklistArtist :exec
DELETE FROM artist_blacklist WHERE user_id = $1 AND artist_id = $2;

-- name: ListBlacklistedArtists :many
SELECT a.* FROM artists a
JOIN artist_blacklist b ON b.artist_id = a.id
WHERE b.user_id = $1
ORDER BY a.name;

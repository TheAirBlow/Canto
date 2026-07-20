-- name: RepointSourcesForMerge :exec
UPDATE sources SET entity_id = sqlc.arg(new_entity_id)::bigint
WHERE entity_type = sqlc.arg(entity_type) AND entity_id = sqlc.arg(old_entity_id)::bigint;

-- name: RepointAlbumArtistsForArtistMerge :exec
UPDATE album_artists SET artist_id = sqlc.arg(new_artist_id)::bigint
WHERE artist_id = sqlc.arg(old_artist_id)::bigint
  AND album_id NOT IN (SELECT album_id FROM album_artists WHERE artist_id = sqlc.arg(new_artist_id)::bigint);

-- name: DeleteRemainingAlbumArtistsForArtist :exec
DELETE FROM album_artists WHERE artist_id = sqlc.arg(old_artist_id)::bigint;

-- name: RepointSongArtistsForArtistMerge :exec
UPDATE song_artists SET artist_id = sqlc.arg(new_artist_id)::bigint
WHERE artist_id = sqlc.arg(old_artist_id)::bigint
  AND song_id NOT IN (SELECT song_id FROM song_artists WHERE artist_id = sqlc.arg(new_artist_id)::bigint);

-- name: DeleteRemainingSongArtistsForArtist :exec
DELETE FROM song_artists WHERE artist_id = sqlc.arg(old_artist_id)::bigint;

-- name: RepointBlacklistForArtistMerge :exec
UPDATE artist_blacklist SET artist_id = sqlc.arg(new_artist_id)::bigint
WHERE artist_id = sqlc.arg(old_artist_id)::bigint
  AND user_id NOT IN (SELECT user_id FROM artist_blacklist WHERE artist_id = sqlc.arg(new_artist_id)::bigint);

-- name: DeleteRemainingBlacklistForArtist :exec
DELETE FROM artist_blacklist WHERE artist_id = sqlc.arg(old_artist_id)::bigint;

-- name: RepointAlbumArtistsForAlbumMerge :exec
UPDATE album_artists SET album_id = sqlc.arg(new_album_id)::bigint
WHERE album_id = sqlc.arg(old_album_id)::bigint
  AND artist_id NOT IN (SELECT artist_id FROM album_artists WHERE album_id = sqlc.arg(new_album_id)::bigint);

-- name: DeleteRemainingAlbumArtistsForAlbum :exec
DELETE FROM album_artists WHERE album_id = sqlc.arg(old_album_id)::bigint;

-- name: RepointSongAlbumsForAlbumMerge :exec
UPDATE song_albums SET album_id = sqlc.arg(new_album_id)::bigint
WHERE album_id = sqlc.arg(old_album_id)::bigint
  AND song_id NOT IN (SELECT song_id FROM song_albums WHERE album_id = sqlc.arg(new_album_id)::bigint);

-- name: DeleteRemainingSongAlbumsForAlbum :exec
DELETE FROM song_albums WHERE album_id = sqlc.arg(old_album_id)::bigint;

-- name: RepointSongArtistsForSongMerge :exec
UPDATE song_artists SET song_id = sqlc.arg(new_song_id)::bigint
WHERE song_id = sqlc.arg(old_song_id)::bigint
  AND artist_id NOT IN (SELECT artist_id FROM song_artists WHERE song_id = sqlc.arg(new_song_id)::bigint);

-- name: DeleteRemainingSongArtistsForSong :exec
DELETE FROM song_artists WHERE song_id = sqlc.arg(old_song_id)::bigint;

-- name: RepointSongAlbumsForSongMerge :exec
UPDATE song_albums SET song_id = sqlc.arg(new_song_id)::bigint
WHERE song_id = sqlc.arg(old_song_id)::bigint
  AND album_id NOT IN (SELECT album_id FROM song_albums WHERE song_id = sqlc.arg(new_song_id)::bigint);

-- name: DeleteRemainingSongAlbumsForSong :exec
DELETE FROM song_albums WHERE song_id = sqlc.arg(old_song_id)::bigint;

-- name: RepointListensForSongMerge :exec
UPDATE listens SET song_id = sqlc.arg(new_song_id)::bigint WHERE song_id = sqlc.arg(old_song_id)::bigint;

-- name: RepointNowPlayingForSongMerge :exec
UPDATE now_playing SET song_id = sqlc.arg(new_song_id)::bigint WHERE song_id = sqlc.arg(old_song_id)::bigint;

package correlate

import (
	"context"
	"fmt"

	"Canto/internal/db"
)

// MergeEntity repoints every reference from loserID to winnerID and deletes loserID, all via q.
func MergeEntity(ctx context.Context, q *db.Queries, entityType db.EntityType, loserID, winnerID int64) error {
	if err := q.RepointSourcesForMerge(ctx, db.RepointSourcesForMergeParams{NewEntityID: winnerID, EntityType: entityType, OldEntityID: loserID}); err != nil {
		return fmt.Errorf("correlate: repoint sources for merge: %w", err)
	}
	if err := q.RepointAliases(ctx, db.RepointAliasesParams{EntityType: entityType, OldEntityID: loserID, NewEntityID: winnerID}); err != nil {
		return fmt.Errorf("correlate: repoint aliases for merge: %w", err)
	}
	if err := q.DeleteMergeSuggestionsForEntity(ctx, db.DeleteMergeSuggestionsForEntityParams{EntityType: entityType, EntityID: loserID}); err != nil {
		return fmt.Errorf("correlate: delete merge suggestions for merge: %w", err)
	}

	switch entityType {
	case db.EntityTypeArtist:
		if err := q.RepointAlbumArtistsForArtistMerge(ctx, db.RepointAlbumArtistsForArtistMergeParams{NewArtistID: winnerID, OldArtistID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint album artists for artist merge: %w", err)
		}
		if err := q.DeleteRemainingAlbumArtistsForArtist(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining album artists for artist merge: %w", err)
		}
		if err := q.RepointSongArtistsForArtistMerge(ctx, db.RepointSongArtistsForArtistMergeParams{NewArtistID: winnerID, OldArtistID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint song artists for artist merge: %w", err)
		}
		if err := q.DeleteRemainingSongArtistsForArtist(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining song artists for artist merge: %w", err)
		}
		if err := q.RepointBlacklistForArtistMerge(ctx, db.RepointBlacklistForArtistMergeParams{NewArtistID: winnerID, OldArtistID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint blacklist for artist merge: %w", err)
		}
		if err := q.DeleteRemainingBlacklistForArtist(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining blacklist for artist merge: %w", err)
		}
		if _, err := q.DeleteArtist(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete merged artist: %w", err)
		}

	case db.EntityTypeAlbum:
		if err := q.RepointAlbumArtistsForAlbumMerge(ctx, db.RepointAlbumArtistsForAlbumMergeParams{NewAlbumID: winnerID, OldAlbumID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint album artists for album merge: %w", err)
		}
		if err := q.DeleteRemainingAlbumArtistsForAlbum(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining album artists for album merge: %w", err)
		}
		if err := q.RepointSongAlbumsForAlbumMerge(ctx, db.RepointSongAlbumsForAlbumMergeParams{NewAlbumID: winnerID, OldAlbumID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint song albums for album merge: %w", err)
		}
		if err := q.DeleteRemainingSongAlbumsForAlbum(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining song albums for album merge: %w", err)
		}
		if _, err := q.DeleteAlbum(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete merged album: %w", err)
		}

	case db.EntityTypeSong:
		if err := q.RepointSongArtistsForSongMerge(ctx, db.RepointSongArtistsForSongMergeParams{NewSongID: winnerID, OldSongID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint song artists for song merge: %w", err)
		}
		if err := q.DeleteRemainingSongArtistsForSong(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining song artists for song merge: %w", err)
		}
		if err := q.RepointSongAlbumsForSongMerge(ctx, db.RepointSongAlbumsForSongMergeParams{NewSongID: winnerID, OldSongID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint song albums for song merge: %w", err)
		}
		if err := q.DeleteRemainingSongAlbumsForSong(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete remaining song albums for song merge: %w", err)
		}
		if err := q.RepointListensForSongMerge(ctx, db.RepointListensForSongMergeParams{NewSongID: winnerID, OldSongID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint listens for song merge: %w", err)
		}
		if err := q.RepointNowPlayingForSongMerge(ctx, db.RepointNowPlayingForSongMergeParams{NewSongID: winnerID, OldSongID: loserID}); err != nil {
			return fmt.Errorf("correlate: repoint now playing for song merge: %w", err)
		}
		if _, err := q.DeleteSong(ctx, loserID); err != nil {
			return fmt.Errorf("correlate: delete merged song: %w", err)
		}
	}

	return nil
}

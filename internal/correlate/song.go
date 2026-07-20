package correlate

import (
	"context"
	"fmt"
	"log/slog"

	"Canto/internal/db"
	"Canto/internal/search"
)

// ResolveSong finds or creates the song matching extractedID/name, links it to artistIDs and to albumID, and returns its id and whether this call created it.
func (e *Engine) ResolveSong(ctx context.Context, name string, sourceType db.SourceType, extractedID, rawURL string, durationMs *int32, artistIDs []int64, albumID *int64, trackNumber *int32, matchers []FuzzyMatcher, normalize bool) (int64, bool, error) {
	songID, created, err := e.resolveSong(ctx, name, sourceType, extractedID, rawURL, durationMs, artistIDs, albumID, matchers, normalize)
	if err != nil {
		return 0, false, err
	}
	for _, artistID := range artistIDs {
		if err := e.queries.LinkSongArtist(ctx, db.LinkSongArtistParams{SongID: songID, ArtistID: artistID}); err != nil {
			return 0, false, fmt.Errorf("correlate: link song artist: %w", err)
		}
	}
	if albumID != nil {
		if err := e.queries.LinkSongAlbum(ctx, db.LinkSongAlbumParams{SongID: songID, AlbumID: *albumID, TrackNumber: trackNumber}); err != nil {
			return 0, false, fmt.Errorf("correlate: link song album: %w", err)
		}
	}
	return songID, created, nil
}

// resolveSong finds or creates the song, without linking it to any artist or album.
func (e *Engine) resolveSong(ctx context.Context, name string, sourceType db.SourceType, extractedID, rawURL string, durationMs *int32, artistIDs []int64, albumID *int64, matchers []FuzzyMatcher, normalize bool) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	matchName := name
	if normalize {
		matchName = nameNorm
	}

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{SourceType: sourceType, ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: song resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: song source lookup: %w", err)
		}
	}

	if id, found, err := chainMatch(ctx, matchers, "song", matchName, artistIDs, albumID); err != nil {
		return 0, false, fmt.Errorf("correlate: song fuzzy match: %w", err)
	} else if found {
		slog.Debug("correlate: song resolved via fuzzy match", "id", id, "name", name)
		if err := e.attachSource(ctx, e.queries, db.EntityTypeSong, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}

	return e.createSongLocked(ctx, name, nameNorm, matchName, sourceType, rawURL, extractedID, durationMs, artistIDs, albumID, matchers)
}

// createSongLocked serializes creation of a new song under an advisory lock keyed by name.
func (e *Engine) createSongLocked(ctx context.Context, name, nameNorm, matchName string, sourceType db.SourceType, rawURL, extractedID string, durationMs *int32, artistIDs []int64, albumID *int64, matchers []FuzzyMatcher) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin song create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if err := q.AdvisoryLockEntityName(ctx, db.AdvisoryLockEntityNameParams{NameNormalized: nameNorm, Seed: advisoryLockSeed["song"]}); err != nil {
		return 0, false, fmt.Errorf("correlate: song advisory lock: %w", err)
	}

	if id, found, err := chainMatch(ctx, matchers, "song", matchName, artistIDs, albumID); err != nil {
		return 0, false, fmt.Errorf("correlate: song fuzzy recheck: %w", err)
	} else if found {
		if err := e.attachSource(ctx, q, db.EntityTypeSong, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, tx.Commit(ctx)
	}

	created, err := q.CreateSong(ctx, db.CreateSongParams{Name: name, NameNormalized: nameNorm, DurationMs: durationMs})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create song (locked): %w", err)
	}
	if err := e.attachSource(ctx, q, db.EntityTypeSong, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	e.search.Upsert(ctx, "songs", search.Document{
		ID: created.ID, EntityType: "song", Name: name, NameNormalized: nameNorm,
		ArtistIDs: artistIDs, ArtistNames: e.artistNames(ctx, artistIDs), AlbumID: albumID, AlbumName: e.albumName(ctx, albumID),
	})
	slog.Info("correlate: song created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

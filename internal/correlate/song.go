package correlate

import (
	"context"
	"fmt"
	"log/slog"

	"Canto/internal/correlate/romanize"
	"Canto/internal/db"
	"Canto/internal/search"
	"Canto/internal/source"
)

// ResolveSong finds or creates the song matching extractedID/name, links it to artistIDs and to albumID, and returns its id and whether this call created it.
func (e *Engine) ResolveSong(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, durationMs *int32, artistIDs []int64, artistNames []string, albumID *int64, trackNumber *int32, matchers []FuzzyMatcher) (int64, bool, error) {
	songID, created, err := e.resolveSong(ctx, name, sourceType, extractedID, rawURL, durationMs, artistIDs, artistNames, albumID, matchers)
	if err != nil {
		return 0, false, err
	}

	// createSongLocked already links every artist and album inside its own transaction when created.
	if !created {
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
	}
	return songID, created, nil
}

// resolveSong finds or creates the song, without linking it to any artist or album.
func (e *Engine) resolveSong(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, durationMs *int32, artistIDs []int64, artistNames []string, albumID *int64, matchers []FuzzyMatcher) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	nameRoman := romanize.Romanize(name)

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeSong, SourceType: string(sourceType), ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: song resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: song source lookup: %w", err)
		}
	}

	q := Query{Names: []string{name}, ArtistIDs: artistIDs, ArtistNames: artistNames, AlbumID: albumID, DurationMs: durationMs}
	dec, err := e.decide(ctx, "song", matchers, sourceType, name, nameNorm, nameRoman, q)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: song candidate scoring: %w", err)
	}

	switch dec.band {
	case bandAutoAccept:
		slog.Debug("correlate: song resolved via fuzzy match", "id", dec.winnerID, "name", name, "score", dec.finalScore)
		finalID, err := e.attachSource(ctx, db.EntityTypeSong, dec.winnerID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		e.recordAlias(ctx, db.EntityTypeSong, finalID, name)
		return finalID, false, nil

	case bandSuggest:
		id, created, err := e.createSongLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID, durationMs, artistIDs, albumID)
		if err != nil {
			return 0, false, err
		}
		if created {
			if err := queueSuggestion(ctx, e.queries, db.EntityTypeSong, id, dec.winnerID, dec.finalScore); err != nil {
				slog.Warn("correlate: queue song merge suggestion failed", "err", err)
			}
		}
		return id, created, nil

	default:
		return e.createSongLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID, durationMs, artistIDs, albumID)
	}
}

// createSongLocked serializes creation of a new song under an advisory lock keyed by source id when known, else by name.
func (e *Engine) createSongLocked(ctx context.Context, name, nameNorm, nameRoman string, sourceType source.SourceType, rawURL, extractedID string, durationMs *int32, artistIDs []int64, albumID *int64) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin song create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if extractedID != "" {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: string(sourceType) + ":" + extractedID, Seed: advisoryLockSeed["song_id"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: song advisory lock: %w", err)
		}
		if id, err := q.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeSong, SourceType: string(sourceType), ExtractedID: &extractedID}); err == nil {
			return id, false, tx.Commit(ctx)
		} else if !isNoRows(err) {
			return 0, false, fmt.Errorf("correlate: song source recheck: %w", err)
		}
	} else {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: nameNorm, Seed: advisoryLockSeed["song"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: song advisory lock: %w", err)
		}
	}

	// DB-only recheck: a configured fuzzy/search-index matcher can lag a concurrent create.
	if id, found, err := exactRecheck(ctx, q, "song", name, artistIDs, albumID); err != nil {
		return 0, false, fmt.Errorf("correlate: song exact recheck: %w", err)
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return 0, false, err
		}
		finalID, err := e.attachSource(ctx, db.EntityTypeSong, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		return finalID, false, nil
	}

	created, err := q.CreateSong(ctx, db.CreateSongParams{Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman, DurationMs: durationMs})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create song (locked): %w", err)
	}
	// Link before commit so a racer's locked recheck (artist/album-scoped) sees it linked.
	for _, artistID := range artistIDs {
		if err := q.LinkSongArtist(ctx, db.LinkSongArtistParams{SongID: created.ID, ArtistID: artistID}); err != nil {
			return 0, false, fmt.Errorf("correlate: link song artist (locked): %w", err)
		}
	}
	if albumID != nil {
		if err := q.LinkSongAlbum(ctx, db.LinkSongAlbumParams{SongID: created.ID, AlbumID: *albumID}); err != nil {
			return 0, false, fmt.Errorf("correlate: link song album (locked): %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}

	finalID, err := e.attachSource(ctx, db.EntityTypeSong, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
	if err != nil {
		return 0, false, err
	}
	if finalID != created.ID {
		slog.Info("correlate: song created then merged via source conflict", "id", created.ID, "into", finalID)
		return finalID, false, nil
	}
	e.search.Upsert(ctx, "songs", search.Document{
		ID: created.ID, EntityType: "song", Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman,
		ArtistIDs: artistIDs, ArtistNames: e.artistNames(ctx, artistIDs), AlbumID: albumID, AlbumName: e.albumName(ctx, albumID),
	})
	e.reconciler.Enqueue(db.EntityTypeSong, created.ID)
	slog.Info("correlate: song created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

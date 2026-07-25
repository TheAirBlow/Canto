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

// ResolveAlbum finds or creates the album matching extractedID/name, links it to every id in artistIDs, and returns its id and whether this call created it.
func (e *Engine) ResolveAlbum(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, artistIDs []int64, artistNames []string, matchers []FuzzyMatcher) (int64, bool, error) {
	albumID, created, err := e.resolveAlbum(ctx, name, sourceType, extractedID, rawURL, artistIDs, artistNames, matchers)
	if err != nil {
		return 0, false, err
	}

	// createAlbumLocked already links every artist inside its own transaction when created.
	if !created {
		for _, artistID := range artistIDs {
			if err := e.queries.LinkAlbumArtist(ctx, db.LinkAlbumArtistParams{AlbumID: albumID, ArtistID: artistID}); err != nil {
				return 0, false, fmt.Errorf("correlate: link album artist: %w", err)
			}
		}
	}

	return albumID, created, nil
}

// resolveAlbum finds or creates the album, without linking it to any artist.
func (e *Engine) resolveAlbum(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, artistIDs []int64, artistNames []string, matchers []FuzzyMatcher) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	nameRoman := romanize.Romanize(name)

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeAlbum, SourceType: string(sourceType), ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: album resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: album source lookup: %w", err)
		}
	}

	q := Query{Names: []string{name}, ArtistIDs: artistIDs, ArtistNames: artistNames}
	dec, err := e.decide(ctx, "album", matchers, sourceType, name, nameNorm, nameRoman, q)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: album candidate scoring: %w", err)
	}

	switch dec.band {
	case bandAutoAccept:
		slog.Debug("correlate: album resolved via fuzzy match", "id", dec.winnerID, "name", name, "score", dec.finalScore)
		finalID, err := e.attachSource(ctx, db.EntityTypeAlbum, dec.winnerID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		e.recordAlias(ctx, db.EntityTypeAlbum, finalID, name)
		return finalID, false, nil

	case bandSuggest:
		id, created, err := e.createAlbumLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID, artistIDs)
		if err != nil {
			return 0, false, err
		}
		if created {
			if err := queueSuggestion(ctx, e.queries, db.EntityTypeAlbum, id, dec.winnerID, dec.finalScore); err != nil {
				slog.Warn("correlate: queue album merge suggestion failed", "err", err)
			}
		}
		return id, created, nil

	default:
		return e.createAlbumLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID, artistIDs)
	}
}

// createAlbumLocked serializes creation of a new album under an advisory lock keyed by source id when known, else by name.
func (e *Engine) createAlbumLocked(ctx context.Context, name, nameNorm, nameRoman string, sourceType source.SourceType, rawURL, extractedID string, artistIDs []int64) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin album create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if extractedID != "" {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: string(sourceType) + ":" + extractedID, Seed: advisoryLockSeed["album_id"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: album advisory lock: %w", err)
		}
		if id, err := q.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeAlbum, SourceType: string(sourceType), ExtractedID: &extractedID}); err == nil {
			return id, false, tx.Commit(ctx)
		} else if !isNoRows(err) {
			return 0, false, fmt.Errorf("correlate: album source recheck: %w", err)
		}
	} else {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: nameNorm, Seed: advisoryLockSeed["album"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: album advisory lock: %w", err)
		}
	}

	// DB-only recheck: a configured fuzzy/search-index matcher can lag a concurrent create.
	if id, found, err := exactRecheck(ctx, q, "album", name, artistIDs, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: album exact recheck: %w", err)
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return 0, false, err
		}
		finalID, err := e.attachSource(ctx, db.EntityTypeAlbum, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		return finalID, false, nil
	}

	created, err := q.CreateAlbum(ctx, db.CreateAlbumParams{Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create album (locked): %w", err)
	}
	// Link before commit so a racer's locked recheck (artist-scoped) sees it linked.
	for _, artistID := range artistIDs {
		if err := q.LinkAlbumArtist(ctx, db.LinkAlbumArtistParams{AlbumID: created.ID, ArtistID: artistID}); err != nil {
			return 0, false, fmt.Errorf("correlate: link album artist (locked): %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}

	finalID, err := e.attachSource(ctx, db.EntityTypeAlbum, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
	if err != nil {
		return 0, false, err
	}
	if finalID != created.ID {
		slog.Info("correlate: album created then merged via source conflict", "id", created.ID, "into", finalID)
		return finalID, false, nil
	}
	e.search.Upsert(ctx, "albums", search.Document{
		ID: created.ID, EntityType: "album", Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman,
		ArtistIDs: artistIDs, ArtistNames: e.artistNames(ctx, artistIDs),
	})
	e.reconciler.Enqueue(db.EntityTypeAlbum, created.ID)
	slog.Info("correlate: album created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

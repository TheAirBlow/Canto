package correlate

import (
	"context"
	"fmt"
	"log/slog"

	"Canto/internal/db"
	"Canto/internal/search"
	"Canto/internal/source"
)

// ResolveAlbum finds or creates the album matching extractedID/name, links it to every id in artistIDs, and returns its id and whether this call created it.
func (e *Engine) ResolveAlbum(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, artistIDs []int64, matchers []FuzzyMatcher, normalize bool) (int64, bool, error) {
	albumID, created, err := e.resolveAlbum(ctx, name, sourceType, extractedID, rawURL, artistIDs, matchers, normalize)
	if err != nil {
		return 0, false, err
	}

	for _, artistID := range artistIDs {
		if err := e.queries.LinkAlbumArtist(ctx, db.LinkAlbumArtistParams{AlbumID: albumID, ArtistID: artistID}); err != nil {
			return 0, false, fmt.Errorf("correlate: link album artist: %w", err)
		}
	}

	return albumID, created, nil
}

// resolveAlbum finds or creates the album, without linking it to any artist.
func (e *Engine) resolveAlbum(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, artistIDs []int64, matchers []FuzzyMatcher, normalize bool) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	matchName := name
	if normalize {
		matchName = nameNorm
	}

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{SourceType: string(sourceType), ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: album resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: album source lookup: %w", err)
		}
	}

	if id, found, err := chainMatch(ctx, matchers, "album", matchName, artistIDs, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: album fuzzy match: %w", err)
	} else if found {
		slog.Debug("correlate: album resolved via fuzzy match", "id", id, "name", name)
		if err := e.attachSource(ctx, e.queries, db.EntityTypeAlbum, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}

	return e.createAlbumLocked(ctx, name, nameNorm, matchName, sourceType, rawURL, extractedID, artistIDs, matchers)
}

// createAlbumLocked serializes creation of a new album under an advisory lock keyed by name.
func (e *Engine) createAlbumLocked(ctx context.Context, name, nameNorm, matchName string, sourceType source.SourceType, rawURL, extractedID string, artistIDs []int64, matchers []FuzzyMatcher) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin album create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if err := q.AdvisoryLockEntityName(ctx, db.AdvisoryLockEntityNameParams{NameNormalized: nameNorm, Seed: advisoryLockSeed["album"]}); err != nil {
		return 0, false, fmt.Errorf("correlate: album advisory lock: %w", err)
	}

	if id, found, err := chainMatch(ctx, matchers, "album", matchName, artistIDs, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: album fuzzy recheck: %w", err)
	} else if found {
		if err := e.attachSource(ctx, q, db.EntityTypeAlbum, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, tx.Commit(ctx)
	}

	created, err := q.CreateAlbum(ctx, db.CreateAlbumParams{Name: name, NameNormalized: nameNorm})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create album (locked): %w", err)
	}
	if err := e.attachSource(ctx, q, db.EntityTypeAlbum, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	e.search.Upsert(ctx, "albums", search.Document{
		ID: created.ID, EntityType: "album", Name: name, NameNormalized: nameNorm,
		ArtistIDs: artistIDs, ArtistNames: e.artistNames(ctx, artistIDs),
	})
	slog.Info("correlate: album created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

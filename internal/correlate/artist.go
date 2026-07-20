package correlate

import (
	"context"
	"fmt"
	"log/slog"

	"Canto/internal/db"
	"Canto/internal/search"
)

// ResolveArtist finds or creates the artist matching extractedID/name, in that priority order, returning its id and whether this call created it.
func (e *Engine) ResolveArtist(ctx context.Context, name string, sourceType db.SourceType, extractedID, rawURL string, matchers []FuzzyMatcher, normalize bool) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	matchName := name
	if normalize {
		matchName = nameNorm
	}

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{SourceType: sourceType, ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: artist resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: artist source lookup: %w", err)
		}
	}

	if id, found, err := chainMatch(ctx, matchers, "artist", matchName, nil, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: artist fuzzy match: %w", err)
	} else if found {
		slog.Debug("correlate: artist resolved via fuzzy match", "id", id, "name", name)
		if err := e.attachSource(ctx, e.queries, db.EntityTypeArtist, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}

	return e.createArtistLocked(ctx, name, nameNorm, matchName, sourceType, rawURL, extractedID, matchers)
}

// createArtistLocked serializes creation of a new artist under an advisory lock keyed by name.
func (e *Engine) createArtistLocked(ctx context.Context, name, nameNorm, matchName string, sourceType db.SourceType, rawURL, extractedID string, matchers []FuzzyMatcher) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin artist create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if err := q.AdvisoryLockEntityName(ctx, db.AdvisoryLockEntityNameParams{NameNormalized: nameNorm, Seed: advisoryLockSeed["artist"]}); err != nil {
		return 0, false, fmt.Errorf("correlate: artist advisory lock: %w", err)
	}

	// Double-check inside the lock: someone else may have created it while we waited.
	if id, found, err := chainMatch(ctx, matchers, "artist", matchName, nil, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: artist fuzzy recheck: %w", err)
	} else if found {
		if err := e.attachSource(ctx, q, db.EntityTypeArtist, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
			return 0, false, err
		}
		return id, false, tx.Commit(ctx)
	}

	created, err := q.CreateArtist(ctx, db.CreateArtistParams{Name: name, NameNormalized: nameNorm})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create artist (locked): %w", err)
	}
	if err := e.attachSource(ctx, q, db.EntityTypeArtist, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	e.search.Upsert(ctx, "artists", search.Document{ID: created.ID, EntityType: "artist", Name: name, NameNormalized: nameNorm})
	slog.Info("correlate: artist created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

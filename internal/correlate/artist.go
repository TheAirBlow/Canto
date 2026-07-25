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

// ResolveArtist finds or creates the artist matching extractedID/name, in that priority order, returning its id and whether this call created it.
func (e *Engine) ResolveArtist(ctx context.Context, name string, sourceType source.SourceType, extractedID, rawURL string, matchers []FuzzyMatcher) (int64, bool, error) {
	nameNorm := NormalizeName(name)
	nameRoman := romanize.Romanize(name)

	if extractedID != "" {
		id, err := e.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeArtist, SourceType: string(sourceType), ExtractedID: &extractedID})
		switch {
		case err == nil:
			slog.Debug("correlate: artist resolved via source id", "id", id, "source", sourceType)
			return id, false, nil
		case !isNoRows(err):
			return 0, false, fmt.Errorf("correlate: artist source lookup: %w", err)
		}
	}

	q := Query{Names: []string{name}}
	dec, err := e.decide(ctx, "artist", matchers, sourceType, name, nameNorm, nameRoman, q)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: artist candidate scoring: %w", err)
	}

	switch dec.band {
	case bandAutoAccept:
		slog.Debug("correlate: artist resolved via fuzzy match", "id", dec.winnerID, "name", name, "score", dec.finalScore)
		finalID, err := e.attachSource(ctx, db.EntityTypeArtist, dec.winnerID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		e.recordAlias(ctx, db.EntityTypeArtist, finalID, name)
		return finalID, false, nil

	case bandSuggest:
		id, created, err := e.createArtistLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID)
		if err != nil {
			return 0, false, err
		}
		if created {
			if err := queueSuggestion(ctx, e.queries, db.EntityTypeArtist, id, dec.winnerID, dec.finalScore); err != nil {
				slog.Warn("correlate: queue artist merge suggestion failed", "err", err)
			}
		}
		return id, created, nil

	default:
		return e.createArtistLocked(ctx, name, nameNorm, nameRoman, sourceType, rawURL, extractedID)
	}
}

// createArtistLocked serializes creation of a new artist under an advisory lock keyed by source id when known, else by name.
func (e *Engine) createArtistLocked(ctx context.Context, name, nameNorm, nameRoman string, sourceType source.SourceType, rawURL, extractedID string) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("correlate: begin artist create tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	if extractedID != "" {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: string(sourceType) + ":" + extractedID, Seed: advisoryLockSeed["artist_id"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: artist advisory lock: %w", err)
		}
		if id, err := q.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeArtist, SourceType: string(sourceType), ExtractedID: &extractedID}); err == nil {
			return id, false, tx.Commit(ctx)
		} else if !isNoRows(err) {
			return 0, false, fmt.Errorf("correlate: artist source recheck: %w", err)
		}
	} else {
		if err := q.AdvisoryLockKey(ctx, db.AdvisoryLockKeyParams{Key: nameNorm, Seed: advisoryLockSeed["artist"]}); err != nil {
			return 0, false, fmt.Errorf("correlate: artist advisory lock: %w", err)
		}
	}

	// DB-only recheck: a configured fuzzy/search-index matcher can lag a concurrent create.
	if id, found, err := exactRecheck(ctx, q, "artist", name, nil, nil); err != nil {
		return 0, false, fmt.Errorf("correlate: artist exact recheck: %w", err)
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return 0, false, err
		}
		finalID, err := e.attachSource(ctx, db.EntityTypeArtist, id, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
		if err != nil {
			return 0, false, err
		}
		return finalID, false, nil
	}

	created, err := q.CreateArtist(ctx, db.CreateArtistParams{Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman})
	if err != nil {
		return 0, false, fmt.Errorf("correlate: create artist (locked): %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}

	finalID, err := e.attachSource(ctx, db.EntityTypeArtist, created.ID, sourceType, rawURL, extractedID, db.CorrelationMethodFuzzyName, nil)
	if err != nil {
		return 0, false, err
	}
	if finalID != created.ID {
		slog.Info("correlate: artist created then merged via source conflict", "id", created.ID, "into", finalID)
		return finalID, false, nil
	}
	e.search.Upsert(ctx, "artists", search.Document{ID: created.ID, EntityType: "artist", Name: name, NameNormalized: nameNorm, NameRomanized: nameRoman})
	e.reconciler.Enqueue(db.EntityTypeArtist, created.ID)
	slog.Info("correlate: artist created", "id", created.ID, "name", name)
	return created.ID, true, nil
}

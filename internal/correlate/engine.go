package correlate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/db"
	"Canto/internal/search"
	"Canto/internal/source"
)

// Engine runs the correlation algorithm against a Postgres pool.
type Engine struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	search     *search.Client
	scoring    ScoringConfig
	reconciler *Reconciler
}

// NewEngine builds an Engine backed by pool; searchClient/reconciler may be nil/disabled.
func NewEngine(pool *pgxpool.Pool, searchClient *search.Client, scoring ScoringConfig, reconciler *Reconciler) *Engine {
	return &Engine{pool: pool, queries: db.New(pool), search: searchClient, scoring: scoring, reconciler: reconciler}
}

// decide scores every candidate, excluding ones that already carry an activeType source, and returns the band decision.
func (e *Engine) decide(ctx context.Context, entityType string, matchers []FuzzyMatcher, activeType source.SourceType, qRaw, qNorm, qRoman string, q Query) (decision, error) {
	candidates, err := collectCandidates(ctx, matchers, entityType, q)
	if err != nil {
		return decision{}, fmt.Errorf("correlate: collect candidates: %w", err)
	}
	if activeType != "" && len(candidates) > 0 {
		candidates, err = e.excludeSameSource(ctx, db.EntityType(entityType), activeType, candidates)
		if err != nil {
			return decision{}, err
		}
	}
	details, err := fetchCandidateDetails(ctx, e.queries, entityType, candidates)
	if err != nil {
		return decision{}, err
	}
	return score(e.scoring, qRaw, qNorm, qRoman, q, details), nil
}

// excludeSameSource drops every candidate that already has a source of activeType attached.
func (e *Engine) excludeSameSource(ctx context.Context, entityType db.EntityType, activeType source.SourceType, candidates []Candidate) ([]Candidate, error) {
	ids := make([]int64, len(candidates))
	for i, c := range candidates {
		ids[i] = c.EntityID
	}
	excluded, err := e.queries.ListEntityIDsWithSourceType(ctx, db.ListEntityIDsWithSourceTypeParams{
		EntityType: entityType, SourceType: string(activeType), EntityIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("correlate: exclude same-source candidates: %w", err)
	}
	if len(excluded) == 0 {
		return candidates, nil
	}

	excludedSet := make(map[int64]bool, len(excluded))
	for _, id := range excluded {
		excludedSet[id] = true
	}
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if !excludedSet[c.EntityID] {
			out = append(out, c)
		}
	}
	return out, nil
}

// recordAlias records name as an alias of entityID, best-effort, and wakes the reconciler.
func (e *Engine) recordAlias(ctx context.Context, entityType db.EntityType, entityID int64, name string) {
	if _, err := e.queries.CreateAlias(ctx, db.CreateAliasParams{EntityType: entityType, EntityID: entityID, Alias: name}); err != nil {
		slog.Warn("correlate: record alias failed", "entity_type", entityType, "id", entityID, "err", err)
		return
	}
	e.reconciler.Enqueue(entityType, entityID)
}

// advisoryLockSeed gives each entity type its own advisory-lock keyspace, name-keyed or id-keyed.
var advisoryLockSeed = map[string]int64{
	"artist": 1, "album": 2, "song": 3,
	"artist_id": 11, "album_id": 12, "song_id": 13,
}

// attachSource inserts a sources row for entityID, merging entityID into the existing owner and returning its id if source_type/extracted_id already belongs to a different entity.
func (e *Engine) attachSource(ctx context.Context, entityType db.EntityType, entityID int64, sourceType source.SourceType, rawURL, extractedID string, method db.CorrelationMethod, confidence *float32) (int64, error) {
	if rawURL == "" && extractedID == "" {
		return entityID, nil // nothing worth recording
	}
	var rawURLPtr, extractedIDPtr *string
	if rawURL != "" {
		rawURLPtr = &rawURL
	}
	if extractedID != "" {
		extractedIDPtr = &extractedID
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("correlate: begin attach source tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := e.queries.WithTx(tx)

	row, err := q.InsertSourceIfAbsent(ctx, db.InsertSourceIfAbsentParams{
		EntityType:        entityType,
		EntityID:          entityID,
		SourceType:        string(sourceType),
		RawUrl:            rawURLPtr,
		ExtractedID:       extractedIDPtr,
		CorrelationMethod: method,
		Confidence:        confidence,
	})
	if err != nil {
		return 0, fmt.Errorf("correlate: attach source: %w", err)
	}

	finalID := entityID
	if row.EntityID != entityID {
		if err := MergeEntity(ctx, q, entityType, entityID, row.EntityID); err != nil {
			return 0, fmt.Errorf("correlate: merge on source conflict: %w", err)
		}
		finalID = row.EntityID
		slog.Info("correlate: merged duplicate entity via source id conflict", "entity_type", entityType, "loser", entityID, "winner", finalID)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("correlate: commit attach source: %w", err)
	}
	if finalID != entityID {
		e.search.Delete(ctx, string(entityType)+"s", entityID)
	}
	return finalID, nil
}

// artistNames looks up artist names for de-normalizing onto a search document.
func (e *Engine) artistNames(ctx context.Context, artistIDs []int64) []string {
	if len(artistIDs) == 0 {
		return nil
	}
	rows, err := e.queries.GetArtistsByIDs(ctx, artistIDs)
	if err != nil {
		slog.Warn("correlate: fetch artist names for search index failed", "err", err)
		return nil
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return names
}

// albumName looks up album name for de-normalizing onto a search document.
func (e *Engine) albumName(ctx context.Context, albumID *int64) string {
	if albumID == nil {
		return ""
	}
	album, err := e.queries.GetAlbumByID(ctx, *albumID)
	if err != nil {
		slog.Warn("correlate: fetch album name for search index failed", "err", err)
		return ""
	}
	return album.Name
}

// isNoRows reports whether err is pgx "no matching row" sentinel.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

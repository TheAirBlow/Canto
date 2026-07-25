package correlate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/db"
	"Canto/internal/search"
)

// reconcileQueueSize bounds how many touched entities Reconciler buffers before dropping new ones.
const reconcileQueueSize = 1024

// touched is one entity a Reconciler should re-check against the rest of the catalog.
type touched struct {
	entityType db.EntityType
	id         int64
}

// Reconciler re-checks touched entities against the full candidate+scorer pipeline, auto-merging clear duplicates and queuing the rest as suggestions.
type Reconciler struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	search   *search.Client
	matchers []FuzzyMatcher
	scoring  ScoringConfig
	queue    chan touched
}

// NewReconciler builds a Reconciler backed by pool/queries, running candidates/scores against matchers/scoring.
func NewReconciler(pool *pgxpool.Pool, queries *db.Queries, searchClient *search.Client, matchers []FuzzyMatcher, scoring ScoringConfig) *Reconciler {
	return &Reconciler{pool: pool, queries: queries, search: searchClient, matchers: matchers, scoring: scoring, queue: make(chan touched, reconcileQueueSize)}
}

// Enqueue schedules entityType/id for reconciliation, blocking until the queue has room.
func (r *Reconciler) Enqueue(entityType db.EntityType, id int64) {
	if r == nil {
		return
	}
	r.queue <- touched{entityType: entityType, id: id}
}

// Run drains queued entities one at a time until ctx is canceled. Call in its own goroutine.
func (r *Reconciler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-r.queue:
			r.reconcile(ctx, t)
		}
	}
}

// reconcile re-scores t against the rest of the catalog, merging or suggesting on a strong enough match.
func (r *Reconciler) reconcile(ctx context.Context, t touched) {
	entityType := string(t.entityType)
	self, ok, err := r.load(ctx, entityType, t.id)
	if err != nil {
		slog.Warn("correlate: reconciler load failed", "entity_type", entityType, "id", t.id, "err", err)
		return
	}
	if !ok || self.pinned {
		return
	}

	q := Query{
		Names: []string{self.detail.name}, ArtistIDs: self.detail.artistIDs, ArtistNames: self.detail.artistNames,
		DurationMs: self.detail.durationMs, TrackIDs: self.detail.trackIDs,
	}
	candidates, err := collectCandidates(ctx, r.matchers, entityType, q)
	if err != nil {
		slog.Warn("correlate: reconciler candidates failed", "entity_type", entityType, "id", t.id, "err", err)
		return
	}
	filtered := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.EntityID != t.id {
			filtered = append(filtered, c)
		}
	}

	details, err := fetchCandidateDetails(ctx, r.queries, entityType, filtered)
	if err != nil {
		slog.Warn("correlate: reconciler fetch details failed", "entity_type", entityType, "id", t.id, "err", err)
		return
	}

	dec := score(r.scoring, self.detail.name, self.detail.nameNormalized, self.detail.nameRomanized, q, details)
	switch dec.band {
	case bandAutoAccept:
		conflict, err := sameSourceConflict(ctx, r.queries, t.entityType, t.id, dec.winnerID)
		if err != nil {
			slog.Warn("correlate: reconciler same-source check failed", "entity_type", entityType, "err", err)
			return
		}
		if conflict {
			if err := queueSuggestion(ctx, r.queries, t.entityType, t.id, dec.winnerID, dec.finalScore); err != nil {
				slog.Warn("correlate: reconciler queue suggestion failed", "entity_type", entityType, "err", err)
			}
			return
		}
		r.merge(ctx, t.entityType, t.id, dec.winnerID)
	case bandSuggest:
		if err := queueSuggestion(ctx, r.queries, t.entityType, t.id, dec.winnerID, dec.finalScore); err != nil {
			slog.Warn("correlate: reconciler queue suggestion failed", "entity_type", entityType, "err", err)
		}
	}
}

// sameSourceConflict reports whether a and b share a source type but with different extracted ids.
func sameSourceConflict(ctx context.Context, queries *db.Queries, entityType db.EntityType, a, b int64) (bool, error) {
	sourcesA, err := queries.ListSourcesForEntity(ctx, db.ListSourcesForEntityParams{EntityType: entityType, EntityID: a})
	if err != nil {
		return false, fmt.Errorf("correlate: list sources for %d: %w", a, err)
	}
	sourcesB, err := queries.ListSourcesForEntity(ctx, db.ListSourcesForEntityParams{EntityType: entityType, EntityID: b})
	if err != nil {
		return false, fmt.Errorf("correlate: list sources for %d: %w", b, err)
	}

	extractedIDByType := make(map[string]string, len(sourcesA))
	for _, s := range sourcesA {
		if s.ExtractedID != nil {
			extractedIDByType[s.SourceType] = *s.ExtractedID
		}
	}
	for _, s := range sourcesB {
		if s.ExtractedID == nil {
			continue
		}
		if existing, ok := extractedIDByType[s.SourceType]; ok && existing != *s.ExtractedID {
			return true, nil
		}
	}
	return false, nil
}

// selfEntity is a reconciled entity's own row data, loaded once per reconcile pass.
type selfEntity struct {
	detail candidateDetail
	pinned bool
}

// load fetches entityID's own name/artists/duration/pinned state.
func (r *Reconciler) load(ctx context.Context, entityType string, entityID int64) (selfEntity, bool, error) {
	switch entityType {
	case "artist":
		row, err := r.queries.GetArtistByID(ctx, entityID)
		if isNoRows(err) {
			return selfEntity{}, false, nil
		}
		if err != nil {
			return selfEntity{}, false, err
		}
		return selfEntity{pinned: row.Pinned, detail: candidateDetail{
			id: row.ID, name: row.Name, nameNormalized: row.NameNormalized, nameRomanized: row.NameRomanized,
		}}, true, nil

	case "album":
		row, err := r.queries.GetAlbumByID(ctx, entityID)
		if isNoRows(err) {
			return selfEntity{}, false, nil
		}
		if err != nil {
			return selfEntity{}, false, err
		}
		artistIDs, artistNames, err := linkedArtists(ctx, r.queries, "album", entityID)
		if err != nil {
			return selfEntity{}, false, err
		}
		trackIDs, err := r.queries.ListSongIDsForAlbum(ctx, entityID)
		if err != nil {
			return selfEntity{}, false, fmt.Errorf("correlate: fetch album track ids: %w", err)
		}
		return selfEntity{pinned: row.Pinned, detail: candidateDetail{
			id: row.ID, name: row.Name, nameNormalized: row.NameNormalized, nameRomanized: row.NameRomanized,
			artistIDs: artistIDs, artistNames: artistNames, trackIDs: trackIDs,
		}}, true, nil

	case "song":
		row, err := r.queries.GetSongByID(ctx, entityID)
		if isNoRows(err) {
			return selfEntity{}, false, nil
		}
		if err != nil {
			return selfEntity{}, false, err
		}
		artistIDs, artistNames, err := linkedArtists(ctx, r.queries, "song", entityID)
		if err != nil {
			return selfEntity{}, false, err
		}
		return selfEntity{pinned: row.Pinned, detail: candidateDetail{
			id: row.ID, name: row.Name, nameNormalized: row.NameNormalized, nameRomanized: row.NameRomanized,
			artistIDs: artistIDs, artistNames: artistNames, durationMs: row.DurationMs,
		}}, true, nil

	default:
		return selfEntity{}, false, nil
	}
}

// isPinned reports whether entityID is pinned, so a merge never overwrites curated data.
func (r *Reconciler) isPinned(ctx context.Context, entityType db.EntityType, entityID int64) (bool, error) {
	switch entityType {
	case db.EntityTypeArtist:
		row, err := r.queries.GetArtistByID(ctx, entityID)
		return row.Pinned, err
	case db.EntityTypeAlbum:
		row, err := r.queries.GetAlbumByID(ctx, entityID)
		return row.Pinned, err
	case db.EntityTypeSong:
		row, err := r.queries.GetSongByID(ctx, entityID)
		return row.Pinned, err
	default:
		return false, nil
	}
}

// merge auto-merges a and b, preferring a pinned side as the winner, refusing entirely if both are pinned.
func (r *Reconciler) merge(ctx context.Context, entityType db.EntityType, a, b int64) {
	aPinned, err := r.isPinned(ctx, entityType, a)
	if err != nil {
		slog.Warn("correlate: reconciler check pinned failed", "id", a, "err", err)
		return
	}
	bPinned, err := r.isPinned(ctx, entityType, b)
	if err != nil {
		slog.Warn("correlate: reconciler check pinned failed", "id", b, "err", err)
		return
	}
	if aPinned && bPinned {
		return
	}

	winner, loser := min(a, b), max(a, b)
	if aPinned {
		winner, loser = a, b
	} else if bPinned {
		winner, loser = b, a
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		slog.Warn("correlate: reconciler begin merge tx failed", "err", err)
		return
	}
	defer tx.Rollback(ctx)
	q := r.queries.WithTx(tx)

	if err := MergeEntity(ctx, q, entityType, loser, winner); err != nil {
		slog.Warn("correlate: reconciler merge failed", "entity_type", entityType, "loser", loser, "winner", winner, "err", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Warn("correlate: reconciler commit merge failed", "err", err)
		return
	}
	r.search.Delete(ctx, string(entityType)+"s", loser)
	slog.Info("correlate: reconciler auto-merged duplicate", "entity_type", entityType, "loser", loser, "winner", winner)
}

// queueSuggestion records a's/b's pair as a merge suggestion, deduplicated by q's unique index.
func queueSuggestion(ctx context.Context, q *db.Queries, entityType db.EntityType, a, b int64, scoreVal float64) error {
	return q.QueueMergeSuggestion(ctx, db.QueueMergeSuggestionParams{
		EntityType: entityType, A: a, B: b, Score: float32(scoreVal),
	})
}

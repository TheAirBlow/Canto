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
	pool    *pgxpool.Pool
	queries *db.Queries
	search  *search.Client
}

// NewEngine builds an Engine backed by pool; searchClient may be disabled, in which case write-through indexing is a no-op.
func NewEngine(pool *pgxpool.Pool, searchClient *search.Client) *Engine {
	return &Engine{pool: pool, queries: db.New(pool), search: searchClient}
}

// advisoryLockSeed gives each entity type its own advisory-lock keyspace.
var advisoryLockSeed = map[string]int64{"artist": 1, "album": 2, "song": 3}

// attachSource inserts a sources row for entityID, tolerating a concurrent duplicate insert for the same source_type and extracted_id pair.
func (e *Engine) attachSource(ctx context.Context, q *db.Queries, entityType db.EntityType, entityID int64, sourceType source.SourceType, rawURL, extractedID string, method db.CorrelationMethod, confidence *float32) error {
	if rawURL == "" && extractedID == "" {
		return nil // nothing worth recording
	}
	var rawURLPtr, extractedIDPtr *string
	if rawURL != "" {
		rawURLPtr = &rawURL
	}
	if extractedID != "" {
		extractedIDPtr = &extractedID
	}
	_, err := q.InsertSourceIfAbsent(ctx, db.InsertSourceIfAbsentParams{
		EntityType:        entityType,
		EntityID:          entityID,
		SourceType:        string(sourceType),
		RawUrl:            rawURLPtr,
		ExtractedID:       extractedIDPtr,
		CorrelationMethod: method,
		Confidence:        confidence,
	})
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("correlate: attach source: %w", err)
	}
	return nil
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

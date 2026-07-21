package stats

import (
	"context"
	"encoding/json"
	"time"

	"Canto/internal/db"
)

// sourceEntry is one source_type's share of listens.
type sourceEntry struct {
	SourceType  string `json:"source_type"`
	ListenCount int64  `json:"listen_count"`
}

// Sources computes userID's listen count broken down by source_type for tf.
func (e *Engine) Sources(ctx context.Context, userID *int64, tf Timeframe) (json.RawMessage, error) {
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceSources, Params: tf}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeSources(ctx, userID, from, to)
	})
}

// computeSources runs SourcesBreakdown and scans its rows.
func (e *Engine) computeSources(ctx context.Context, userID *int64, from, to time.Time) ([]sourceEntry, error) {
	rows, err := e.queries.SourcesBreakdown(ctx, db.SourcesBreakdownParams{UserID: userID, FromTime: ts(from), ToTime: ts(to)})
	if err != nil {
		return nil, err
	}
	entries := make([]sourceEntry, len(rows))
	for i, r := range rows {
		sourceType, _ := r.SourceType.(string)
		entries[i] = sourceEntry{SourceType: sourceType, ListenCount: r.ListenCount}
	}
	return entries, nil
}

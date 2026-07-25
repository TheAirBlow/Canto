package stats

import (
	"context"
	"time"

	"Canto/internal/db"
)

// Engine computes and caches statistics, running its aggregation queries directly against Postgres.
type Engine struct {
	queries       *db.Queries
	regenInterval time.Duration
}

// NewEngine builds an Engine backed by queries.
func NewEngine(queries *db.Queries, regenInterval time.Duration) *Engine {
	return &Engine{queries: queries, regenInterval: regenInterval}
}

// clampToEarliestListen raises from to userID's (or, when nil, the whole catalog's) first listen if that's later, so an all-time request doesn't bucket decades of guaranteed-empty history.
func (e *Engine) clampToEarliestListen(ctx context.Context, userID *int64, from time.Time) (time.Time, error) {
	earliest, err := e.queries.EarliestListenedAt(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	if earliest.Valid && earliest.Time.After(from) {
		return earliest.Time, nil
	}
	return from, nil
}

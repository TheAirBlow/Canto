package stats

import (
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

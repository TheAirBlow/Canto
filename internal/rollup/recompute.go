package rollup

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/db"
)

// RecomputeAll truncates every rollup table and rebuilds it from listens in one transaction.
func RecomputeAll(ctx context.Context, pool *pgxpool.Pool, queries *db.Queries) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rollup: begin recompute tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := queries.WithTx(tx)

	if err := q.TruncateRollupTables(ctx); err != nil {
		return fmt.Errorf("rollup: truncate: %w", err)
	}
	if err := q.RebuildDailySongListens(ctx); err != nil {
		return fmt.Errorf("rollup: rebuild daily song listens: %w", err)
	}
	if err := q.RebuildClockCells(ctx); err != nil {
		return fmt.Errorf("rollup: rebuild clock cells: %w", err)
	}
	if err := q.RebuildEntityFirstListens(ctx); err != nil {
		return fmt.Errorf("rollup: rebuild entity first listens: %w", err)
	}
	if err := q.RebuildEntityGlobalStats(ctx); err != nil {
		return fmt.Errorf("rollup: rebuild entity global stats: %w", err)
	}
	// Must run after RebuildDailySongListens: reuses daily_song_listens for the streak scan.
	if err := q.RebuildUserListenStates(ctx); err != nil {
		return fmt.Errorf("rollup: rebuild user listen states: %w", err)
	}

	return tx.Commit(ctx)
}

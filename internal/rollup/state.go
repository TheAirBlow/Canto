package rollup

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// ReconcileUserState fully recomputes userID's streak state from listens, bypassing the incremental live path.
func ReconcileUserState(ctx context.Context, queries *db.Queries, userID int64) error {
	row, err := queries.ComputeUserStreak(ctx, userID)
	if err != nil {
		return err
	}
	return queries.UpsertUserListenStates(ctx, db.UpsertUserListenStatesParams{
		UserIds:         []int64{userID},
		LastListenedAts: []pgtype.Timestamptz{row.LastListenedAt},
		CurrentStreaks:  []int32{int32(row.CurrentStreak)},
		LongestStreaks:  []int32{int32(row.LongestStreak)},
	})
}

package stats

import (
	"context"
	"encoding/json"
	"time"

	"Canto/internal/db"
)

// summaryResult is stats.summary's response payload.
type summaryResult struct {
	ListenCount        int64   `json:"listen_count"`
	UniqueTracks       int64   `json:"unique_tracks"`
	UniqueAlbums       int64   `json:"unique_albums"`
	UniqueArtists      int64   `json:"unique_artists"`
	MinutesListened    float64 `json:"minutes_listened"`
	DaysActive         int64   `json:"days_active"`
	LongestStreak      int64   `json:"longest_streak"`
	CurrentStreak      int64   `json:"current_streak"`
	AvgDailyPlays      float64 `json:"avg_daily_plays"`
	AvgSessionLengthMs float64 `json:"avg_session_length_ms"`
	TracksPerArtist    float64 `json:"tracks_per_artist"`
	AlbumsPerArtist    float64 `json:"albums_per_artist"`
}

// Summary computes userID's overall listening summary for tf, or every user's when userID is nil.
func (e *Engine) Summary(ctx context.Context, userID *int64, tf Timeframe) (json.RawMessage, error) {
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceSummary, Params: tf}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeSummary(ctx, userID, from, to)
	})
}

// computeSummary runs RollupSummaryStats; streak/session fields stay zero when userID is nil.
func (e *Engine) computeSummary(ctx context.Context, userID *int64, from, to time.Time) (summaryResult, error) {
	row, err := e.queries.RollupSummaryStats(ctx, db.RollupSummaryStatsParams{UserID: userID, FromDay: day(from), ToDay: day(to)})
	if err != nil {
		return summaryResult{}, err
	}
	r := summaryResult{
		ListenCount: row.ListenCount, UniqueTracks: row.UniqueTracks, UniqueAlbums: row.UniqueAlbums,
		UniqueArtists: row.UniqueArtists, MinutesListened: row.MinutesListened, DaysActive: row.DaysActive,
	}
	r.AvgDailyPlays = ratio(r.ListenCount, r.DaysActive)
	r.TracksPerArtist = ratio(r.UniqueTracks, r.UniqueArtists)
	r.AlbumsPerArtist = ratio(r.UniqueAlbums, r.UniqueArtists)

	if userID == nil {
		return r, nil
	}

	longest, err := e.queries.RollupLongestStreak(ctx, db.RollupLongestStreakParams{UserID: *userID, FromDay: day(from), ToDay: day(to)})
	if err != nil {
		return summaryResult{}, err
	}
	r.LongestStreak = longest

	states, err := e.queries.GetUserListenStates(ctx, []int64{*userID})
	if err != nil {
		return summaryResult{}, err
	}
	if len(states) > 0 {
		r.CurrentStreak = int64(states[0].CurrentStreak)
	}

	r.AvgSessionLengthMs, err = e.queries.RollupAvgSessionLengthMs(ctx, db.RollupAvgSessionLengthMsParams{UserID: *userID, FromTime: ts(from), ToTime: ts(to)})
	if err != nil {
		return summaryResult{}, err
	}
	return r, nil
}

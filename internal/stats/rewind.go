package stats

import (
	"context"
	"encoding/json"
	"time"

	"Canto/internal/db"
)

// rewindTopLimit bounds each of Rewind's three leaderboards.
const rewindTopLimit = 5

// rewindEntry is one leaderboard entry plus its delta against the prior equal-length period.
type rewindEntry struct {
	topEntry
	Delta int64 `json:"delta"`
}

// rewindResult is stats.rewind's response payload.
type rewindResult struct {
	TopArtists      []rewindEntry `json:"top_artists"`
	TopAlbums       []rewindEntry `json:"top_albums"`
	TopTracks       []rewindEntry `json:"top_tracks"`
	MinutesListened float64       `json:"minutes_listened"`
	Plays           int64         `json:"plays"`
	NewTracks       int64         `json:"new_tracks"`
	NewAlbums       int64         `json:"new_albums"`
	NewArtists      int64         `json:"new_artists"`
	TopDay          *time.Time    `json:"top_day"`
	LongestStreak   int64         `json:"longest_streak"`
}

// Rewind computes userID's Spotify-Wrapped-style bundle for tf.
func (e *Engine) Rewind(ctx context.Context, userID int64, tf Timeframe) (json.RawMessage, error) {
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	key := cacheKey{UserID: &userID, Resource: db.StatsResourceRewind, Params: tf}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeRewind(ctx, userID, from, to)
	})
}

// computeRewind assembles Rewind's bundle from Summary/Top plus a few dedicated queries.
func (e *Engine) computeRewind(ctx context.Context, userID int64, from, to time.Time) (rewindResult, error) {
	priorFrom := from.Add(-to.Sub(from))

	summary, err := e.computeSummary(ctx, &userID, from, to)
	if err != nil {
		return rewindResult{}, err
	}

	artists, err := e.rewindLeaderboard(ctx, userID, TopArtists, from, to, priorFrom, from)
	if err != nil {
		return rewindResult{}, err
	}
	albums, err := e.rewindLeaderboard(ctx, userID, TopAlbums, from, to, priorFrom, from)
	if err != nil {
		return rewindResult{}, err
	}
	tracks, err := e.rewindLeaderboard(ctx, userID, TopTracks, from, to, priorFrom, from)
	if err != nil {
		return rewindResult{}, err
	}

	newCounts, err := e.queries.RollupNewEntityCounts(ctx, db.RollupNewEntityCountsParams{UserID: userID, FromTime: ts(from), ToTime: ts(to)})
	if err != nil {
		return rewindResult{}, err
	}

	var topDay *time.Time
	if row, err := e.queries.RollupTopDay(ctx, db.RollupTopDayParams{UserID: userID, FromDay: day(from), ToDay: day(to)}); err == nil {
		t := row.Day.Time
		topDay = &t
	}

	return rewindResult{
		TopArtists: artists, TopAlbums: albums, TopTracks: tracks,
		MinutesListened: summary.MinutesListened, Plays: summary.ListenCount,
		NewTracks: newCounts.NewTracks, NewAlbums: newCounts.NewAlbums, NewArtists: newCounts.NewArtists,
		TopDay: topDay, LongestStreak: summary.LongestStreak,
	}, nil
}

// rewindLeaderboard fetches kind's top rewindTopLimit entries for [from, to) and their prior-period deltas.
func (e *Engine) rewindLeaderboard(ctx context.Context, userID int64, kind TopKind, from, to, priorFrom, priorTo time.Time) ([]rewindEntry, error) {
	current, err := e.computeTop(ctx, &userID, kind, from, to, nil, nil, 1, rewindTopLimit)
	if err != nil {
		return nil, err
	}
	prior, err := e.computeTop(ctx, &userID, kind, priorFrom, priorTo, nil, nil, 1, 1000)
	if err != nil {
		return nil, err
	}
	priorByID := make(map[int64]int64, len(prior))
	for _, p := range prior {
		priorByID[p.ID] = p.ListenCount
	}

	entries := make([]rewindEntry, len(current))
	for i, c := range current {
		entries[i] = rewindEntry{topEntry: c, Delta: c.ListenCount - priorByID[c.ID]}
	}
	return entries, nil
}

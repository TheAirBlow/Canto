package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// activityParams is stats.activity's cache-key params: Timeframe plus step and an optional entity scope.
type activityParams struct {
	Timeframe
	Step       string `json:"step"` // day|week|month|year
	EntityType string `json:"entity_type,omitempty"`
	EntityID   *int64 `json:"entity_id,omitempty"`
}

// activityBucket is one zero-filled time bucket's listen count.
type activityBucket struct {
	Bucket          time.Time `json:"bucket"`
	ListenCount     int64     `json:"listen_count"`
	MinutesListened float64   `json:"minutes_listened"`
}

// activityResult is stats.activity's response payload.
type activityResult struct {
	Buckets       []activityBucket `json:"buckets"`
	LongestStreak int64            `json:"longest_streak"`
	CurrentStreak int64            `json:"current_streak"`
}

// Activity computes userID's zero-filled listen-count buckets for tf at step, optionally scoped to one entity.
func (e *Engine) Activity(ctx context.Context, userID *int64, tf Timeframe, step, entityType string, entityID *int64) (json.RawMessage, error) {
	if _, ok := stepIntervals[step]; !ok {
		return nil, fmt.Errorf("stats: invalid step %q", step)
	}
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	from, err = e.clampToEarliestListen(ctx, userID, from)
	if err != nil {
		return nil, err
	}
	params := activityParams{Timeframe: tf, Step: step, EntityType: entityType, EntityID: entityID}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceActivity, Params: params}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeActivity(ctx, userID, from, to, stepIntervals[step], entityType, entityID)
	})
}

// computeActivity runs RollupActivityBuckets; streak fields stay zero when userID is nil.
func (e *Engine) computeActivity(ctx context.Context, userID *int64, from, to time.Time, step pgtype.Interval, entityType string, entityID *int64) (activityResult, error) {
	params := db.RollupActivityBucketsParams{FromTime: ts(from), ToTime: ts(to), Step: step, UserID: userID}
	switch entityType {
	case "artist":
		params.ArtistID = entityID
	case "album":
		params.AlbumID = entityID
	case "song":
		params.SongID = entityID
	}

	rows, err := e.queries.RollupActivityBuckets(ctx, params)
	if err != nil {
		return activityResult{}, err
	}

	buckets := make([]activityBucket, len(rows))
	for i, r := range rows {
		t, _ := r.Bucket.(time.Time)
		buckets[i] = activityBucket{Bucket: t, ListenCount: r.ListenCount, MinutesListened: r.MinutesListened}
	}

	result := activityResult{Buckets: buckets}
	if userID != nil {
		longest, err := e.queries.RollupLongestStreak(ctx, db.RollupLongestStreakParams{UserID: *userID, FromDay: day(from), ToDay: day(to)})
		if err != nil {
			return activityResult{}, err
		}
		result.LongestStreak = longest

		states, err := e.queries.GetUserListenStates(ctx, []int64{*userID})
		if err != nil {
			return activityResult{}, err
		}
		if len(states) > 0 {
			result.CurrentStreak = int64(states[0].CurrentStreak)
		}
	}
	return result, nil
}

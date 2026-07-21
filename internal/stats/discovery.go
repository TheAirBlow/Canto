package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// discoveryParams is stats.discovery's cache-key params: Timeframe plus bucket step.
type discoveryParams struct {
	Timeframe
	Step string `json:"step"`
}

// discoveryBucket is one bucket's new-vs-repeat listen split.
type discoveryBucket struct {
	Bucket        time.Time `json:"bucket"`
	Total         int64     `json:"total"`
	Discoveries   int64     `json:"discoveries"`
	DiscoveryRate float64   `json:"discovery_rate"`
}

// Discovery computes userID's new-vs-repeat listen trend for tf, bucketed by step.
func (e *Engine) Discovery(ctx context.Context, userID *int64, tf Timeframe, step string) (json.RawMessage, error) {
	interval, ok := stepIntervals[step]
	if !ok {
		return nil, fmt.Errorf("stats: invalid step %q", step)
	}
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceDiscovery, Params: discoveryParams{Timeframe: tf, Step: step}}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeDiscovery(ctx, userID, from, to, interval)
	})
}

// computeDiscovery runs RollupDiscoveryBuckets and derives each bucket's discovery rate.
func (e *Engine) computeDiscovery(ctx context.Context, userID *int64, from, to time.Time, step pgtype.Interval) ([]discoveryBucket, error) {
	rows, err := e.queries.RollupDiscoveryBuckets(ctx, db.RollupDiscoveryBucketsParams{UserID: userID, FromTime: ts(from), ToTime: ts(to), Step: step})
	if err != nil {
		return nil, err
	}

	buckets := make([]discoveryBucket, len(rows))
	for i, r := range rows {
		t, _ := r.Bucket.(time.Time)
		buckets[i] = discoveryBucket{Bucket: t, Total: r.Total, Discoveries: r.Discoveries, DiscoveryRate: ratio(r.Discoveries, r.Total)}
	}
	return buckets, nil
}

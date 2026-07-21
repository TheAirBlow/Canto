package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Canto/internal/db"
)

// interestBucketCount is how many evenly-spaced buckets Interest spans an entity's history into.
const interestBucketCount = 20

// interestParams is stats.interest's cache-key params.
type interestParams struct {
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
}

// interestBucket is one evenly-spaced bucket's listen count.
type interestBucket struct {
	Bucket      time.Time `json:"bucket"`
	ListenCount int64     `json:"listen_count"`
}

// Interest computes userID's decay/growth-of-interest graph for one entity, from their first listen to now.
func (e *Engine) Interest(ctx context.Context, userID *int64, entityType string, entityID int64) (json.RawMessage, error) {
	if entityType != "artist" && entityType != "album" && entityType != "song" {
		return nil, fmt.Errorf("stats: invalid entity type %q", entityType)
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceInterest, Params: interestParams{EntityType: entityType, EntityID: entityID}}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeInterest(ctx, userID, entityType, entityID)
	})
}

// computeInterest fetches every scoped listen timestamp and buckets it into interestBucketCount even spans.
func (e *Engine) computeInterest(ctx context.Context, userID *int64, entityType string, entityID int64) ([]interestBucket, error) {
	params := db.InterestHistoryParams{UserID: userID}
	switch entityType {
	case "artist":
		params.ArtistID = &entityID
	case "album":
		params.AlbumID = &entityID
	case "song":
		params.SongID = &entityID
	}

	timestamps, err := e.queries.InterestHistory(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(timestamps) == 0 {
		return []interestBucket{}, nil
	}

	first := timestamps[0].Time
	now := time.Now()
	width := now.Sub(first) / interestBucketCount
	if width <= 0 {
		width = time.Second
	}

	buckets := make([]interestBucket, interestBucketCount)
	for i := range buckets {
		buckets[i].Bucket = first.Add(time.Duration(i) * width)
	}
	for _, at := range timestamps {
		idx := int(at.Time.Sub(first) / width)
		if idx >= interestBucketCount {
			idx = interestBucketCount - 1
		}
		buckets[idx].ListenCount++
	}
	return buckets, nil
}

package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Canto/internal/db"
)

// interestParams is stats.interest's cache-key params.
type interestParams struct {
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	Step       string `json:"step"`
}

// interestBucket is one calendar-aligned bucket's listen count.
type interestBucket struct {
	Bucket      time.Time `json:"bucket"`
	ListenCount int64     `json:"listen_count"`
}

// Interest computes userID's decay/growth-of-interest graph for one entity, from their first listen to now.
func (e *Engine) Interest(ctx context.Context, userID *int64, entityType string, entityID int64, step string) (json.RawMessage, error) {
	if entityType != "artist" && entityType != "album" && entityType != "song" {
		return nil, fmt.Errorf("stats: invalid entity type %q", entityType)
	}
	if _, ok := stepIntervals[step]; !ok {
		return nil, fmt.Errorf("stats: invalid step %q", step)
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceInterest, Params: interestParams{EntityType: entityType, EntityID: entityID, Step: step}}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeInterest(ctx, userID, entityType, entityID, step)
	})
}

// computeInterest fetches every scoped listen timestamp and buckets it into calendar-aligned steps.
func (e *Engine) computeInterest(ctx context.Context, userID *int64, entityType string, entityID int64, step string) ([]interestBucket, error) {
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

	first := truncateToStep(timestamps[0].Time, step)
	last := truncateToStep(time.Now(), step)

	var starts []time.Time
	for t := first; !t.After(last); t = advanceStep(t, step) {
		starts = append(starts, t)
	}

	buckets := make([]interestBucket, len(starts))
	for i, t := range starts {
		buckets[i].Bucket = t
	}
	for _, at := range timestamps {
		idx := stepsBetween(first, truncateToStep(at.Time, step), step)
		if idx >= len(buckets) {
			idx = len(buckets) - 1
		}
		buckets[idx].ListenCount++
	}
	return buckets, nil
}

// truncateToStep floors t (in UTC) to the start of its day/week/month bucket. Week starts Monday.
func truncateToStep(t time.Time, step string) time.Time {
	t = t.UTC()
	switch step {
	case "week":
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		offset := (int(d.Weekday()) + 6) % 7
		return d.AddDate(0, 0, -offset)
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // "day"
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// advanceStep returns t plus one step (day/week/month).
func advanceStep(t time.Time, step string) time.Time {
	switch step {
	case "week":
		return t.AddDate(0, 0, 7)
	case "month":
		return t.AddDate(0, 1, 0)
	default: // "day"
		return t.AddDate(0, 0, 1)
	}
}

// stepsBetween counts how many steps (day/week/month) separate from and to, both already step-truncated.
func stepsBetween(from, to time.Time, step string) int {
	switch step {
	case "week":
		return int(to.Sub(from).Hours() / (24 * 7))
	case "month":
		return (to.Year()-from.Year())*12 + int(to.Month()) - int(from.Month())
	default: // "day"
		return int(to.Sub(from).Hours() / 24)
	}
}

package stats

import (
	"context"
	"encoding/json"
	"time"

	"Canto/internal/db"
)

// recomputeEntity dispatches an entity-scoped row (globally, or for userID alone) to its resource's compute function.
func (e *Engine) recomputeEntity(ctx context.Context, userID *int64, entityType string, entityID int64, resource db.StatsResource) (any, error) {
	switch resource {
	case db.StatsResourceSummary:
		return e.computeEntitySummary(ctx, userID, entityType, entityID)
	default:
		return nil, nil
	}
}

// recompute dispatches to resource's compute function, decoding params; userID is nil for a global row.
func (e *Engine) recompute(ctx context.Context, userID *int64, resource db.StatsResource, params []byte) (any, error) {
	switch resource {
	case db.StatsResourceSummary:
		var tf Timeframe
		if err := json.Unmarshal(params, &tf); err != nil {
			return nil, err
		}
		from, to, err := tf.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeSummary(ctx, userID, from, to)

	case db.StatsResourceTopArtists, db.StatsResourceTopAlbums, db.StatsResourceTopTracks:
		var p topParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		from, to, err := p.Timeframe.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeTop(ctx, userID, p.Kind, from, to, p.ArtistID, p.AlbumID, p.Page, p.PerPage, p.SortBy)

	case db.StatsResourceActivity:
		var p activityParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		from, to, err := p.Timeframe.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		from, err = e.clampToEarliestListen(ctx, userID, from)
		if err != nil {
			return nil, err
		}
		var entityID *int64
		switch p.EntityType {
		case "artist", "album", "song":
			entityID = p.EntityID
		}
		return e.computeActivity(ctx, userID, from, to, stepIntervals[p.Step], p.EntityType, entityID)

	case db.StatsResourceInterest:
		var p interestParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return e.computeInterest(ctx, userID, p.EntityType, p.EntityID, p.Step)

	case db.StatsResourceClock:
		var tf Timeframe
		if err := json.Unmarshal(params, &tf); err != nil {
			return nil, err
		}
		from, to, err := tf.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeClock(ctx, userID, from, to, tf.TZOrUTC())

	case db.StatsResourceSources:
		var tf Timeframe
		if err := json.Unmarshal(params, &tf); err != nil {
			return nil, err
		}
		from, to, err := tf.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeSources(ctx, userID, from, to)

	case db.StatsResourceDiscovery:
		var p discoveryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		from, to, err := p.Timeframe.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		from, err = e.clampToEarliestListen(ctx, userID, from)
		if err != nil {
			return nil, err
		}
		return e.computeDiscovery(ctx, userID, from, to, stepIntervals[p.Step])

	case db.StatsResourceRewind:
		if userID == nil {
			return nil, nil // Rewind is per-user only; a global row shouldn't exist
		}
		var tf Timeframe
		if err := json.Unmarshal(params, &tf); err != nil {
			return nil, err
		}
		from, to, err := tf.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeRewind(ctx, *userID, from, to)

	default:
		return nil, nil
	}
}

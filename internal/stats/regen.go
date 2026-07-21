package stats

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"Canto/internal/db"
)

// Run recomputes every existing stats_cache row on a regenInterval ticker until ctx is canceled.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.regenInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.regenAll(ctx)
		}
	}
}

// regenAll recomputes and re-caches every row currently in stats_cache, logging and skipping any failure.
func (e *Engine) regenAll(ctx context.Context) {
	rows, err := e.queries.ListStatsCache(ctx)
	if err != nil {
		slog.Warn("stats: list cache for regen failed", "err", err)
		return
	}
	for _, row := range rows {
		if err := e.regenRow(ctx, row); err != nil {
			slog.Warn("stats: regen row failed", "id", row.ID, "resource", row.Resource, "err", err)
		}
	}
}

// regenRow recomputes one stats_cache row's value and upserts it, bypassing the freshness check.
func (e *Engine) regenRow(ctx context.Context, row db.StatsCache) error {
	var result any
	var err error
	switch {
	case row.SongID != nil:
		result, err = e.recomputeEntity(ctx, "song", *row.SongID, row.Resource)
	case row.AlbumID != nil:
		result, err = e.recomputeEntity(ctx, "album", *row.AlbumID, row.Resource)
	case row.ArtistID != nil:
		result, err = e.recomputeEntity(ctx, "artist", *row.ArtistID, row.Resource)
	default:
		result, err = e.recompute(ctx, row.UserID, row.Resource, row.Params)
	}
	if err != nil {
		return err
	}
	_, err = e.store(ctx, row.UserID, row.ArtistID, row.AlbumID, row.SongID, row.Resource, row.Params, result)
	return err
}

// recomputeEntity dispatches an entity-scoped (no user_id) row to its resource's compute function.
func (e *Engine) recomputeEntity(ctx context.Context, entityType string, entityID int64, resource db.StatsResource) (any, error) {
	switch resource {
	case db.StatsResourceSummary:
		return e.computeEntitySummary(ctx, entityType, entityID)
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
		return e.computeTop(ctx, userID, p.Kind, from, to, p.ArtistID, p.AlbumID, p.Page, p.PerPage)

	case db.StatsResourceActivity:
		var p activityParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		from, to, err := p.Timeframe.Resolve(time.Now())
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
		return e.computeInterest(ctx, userID, p.EntityType, p.EntityID)

	case db.StatsResourceClock:
		var tf Timeframe
		if err := json.Unmarshal(params, &tf); err != nil {
			return nil, err
		}
		from, to, err := tf.Resolve(time.Now())
		if err != nil {
			return nil, err
		}
		return e.computeClock(ctx, userID, from, to)

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

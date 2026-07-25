package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// staleCacheAge is how long a stats_cache row can go without a successful regen before pruneStale drops it.
const staleCacheAge = 12 * time.Hour

// Run recomputes every existing stats_cache row and prunes stale ones on a regenInterval ticker until ctx is canceled.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.regenInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pruneStale(ctx)
			e.regenAll(ctx)
		}
	}
}

// pruneStale deletes stats_cache rows older than staleCacheAge — only rows whose regen keeps failing outright ever get that old.
func (e *Engine) pruneStale(ctx context.Context) {
	n, err := e.queries.PruneStaleStatsCache(ctx, pgtype.Timestamptz{Time: time.Now().Add(-staleCacheAge), Valid: true})
	if err != nil {
		slog.Warn("stats: prune stale cache failed", "err", err)
		return
	}
	if n > 0 {
		slog.Info("stats: pruned stale cache rows", "count", n)
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

// regenRow recomputes one stats_cache row's value via the resource's dispatch function and upserts it, bypassing the freshness check.
func (e *Engine) regenRow(ctx context.Context, row db.StatsCache) error {
	var result any
	var err error
	switch {
	case row.SongID != nil:
		result, err = e.recomputeEntity(ctx, row.UserID, "song", *row.SongID, row.Resource)
	case row.AlbumID != nil:
		result, err = e.recomputeEntity(ctx, row.UserID, "album", *row.AlbumID, row.Resource)
	case row.ArtistID != nil:
		result, err = e.recomputeEntity(ctx, row.UserID, "artist", *row.ArtistID, row.Resource)
	default:
		result, err = e.recompute(ctx, row.UserID, row.Resource, row.Params)
	}
	if err != nil {
		return err
	}
	_, err = e.store(ctx, row.UserID, row.ArtistID, row.AlbumID, row.SongID, row.Resource, row.Params, result)
	return err
}

// cacheKey identifies one stats_cache row: scope (at most one of UserID/ArtistID/AlbumID/SongID) + Resource + Params.
type cacheKey struct {
	UserID   *int64
	ArtistID *int64
	AlbumID  *int64
	SongID   *int64
	Resource db.StatsResource
	Params   any
}

// cached returns key's cached data, computing and storing it via compute on a cold or stale miss.
func (e *Engine) cached(ctx context.Context, key cacheKey, compute func(ctx context.Context) (any, error)) (json.RawMessage, error) {
	paramsJSON, err := json.Marshal(key.Params)
	if err != nil {
		return nil, fmt.Errorf("stats: marshal params: %w", err)
	}

	row, err := e.queries.GetStatsCache(ctx, db.GetStatsCacheParams{
		UserID: key.UserID, ArtistID: key.ArtistID, AlbumID: key.AlbumID, SongID: key.SongID,
		Resource: key.Resource, Params: paramsJSON,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("stats: get cache: %w", err)
	}
	if err == nil && time.Since(row.ComputedAt.Time) <= 2*e.regenInterval {
		return row.Data, nil
	}

	result, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	return e.store(ctx, key.UserID, key.ArtistID, key.AlbumID, key.SongID, key.Resource, paramsJSON, result)
}

// InvalidateAll deletes every cached stats row, forcing the next read of each to recompute from scratch.
func (e *Engine) InvalidateAll(ctx context.Context) error {
	return e.queries.DeleteStatsCache(ctx)
}

// InvalidateUser deletes every cached row scoped to userID or global, forcing the next read of each to recompute from scratch.
func (e *Engine) InvalidateUser(ctx context.Context, userID int64) error {
	return e.queries.DeleteStatsCacheForUser(ctx, userID)
}

// store marshals result and upserts it into stats_cache under the given scope/resource/paramsJSON key.
func (e *Engine) store(ctx context.Context, userID, artistID, albumID, songID *int64, resource db.StatsResource, paramsJSON []byte, result any) (json.RawMessage, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("stats: marshal result: %w", err)
	}
	stored, err := e.queries.UpsertStatsCache(ctx, db.UpsertStatsCacheParams{
		UserID: userID, ArtistID: artistID, AlbumID: albumID, SongID: songID, Resource: resource, Params: paramsJSON, Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf("stats: upsert cache: %w", err)
	}
	return stored.Data, nil
}

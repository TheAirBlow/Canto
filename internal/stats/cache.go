package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"Canto/internal/db"
)

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

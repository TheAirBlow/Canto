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

// entitySummaryResult is stats.entity_summary's response payload: a catalog entity's stats across every user.
type entitySummaryResult struct {
	Plays           int64      `json:"plays"`
	UniqueListeners int64      `json:"unique_listeners"`
	FirstListenedAt *time.Time `json:"first_listened_at,omitempty"`
}

// EntitySummary computes one catalog entity's global play count and unique listener count across every user.
func (e *Engine) EntitySummary(ctx context.Context, entityType string, entityID int64) (json.RawMessage, error) {
	key := cacheKey{Resource: db.StatsResourceSummary, Params: struct{}{}}
	switch entityType {
	case "artist":
		key.ArtistID = &entityID
	case "album":
		key.AlbumID = &entityID
	case "song":
		key.SongID = &entityID
	default:
		return nil, fmt.Errorf("stats: invalid entity type %q", entityType)
	}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeEntitySummary(ctx, entityType, entityID)
	})
}

// computeEntitySummary runs RollupEntitySummary for entityType.
func (e *Engine) computeEntitySummary(ctx context.Context, entityType string, entityID int64) (entitySummaryResult, error) {
	var dbType db.EntityType
	switch entityType {
	case "artist":
		dbType = db.EntityTypeArtist
	case "album":
		dbType = db.EntityTypeAlbum
	case "song":
		dbType = db.EntityTypeSong
	default:
		return entitySummaryResult{}, fmt.Errorf("stats: invalid entity type %q", entityType)
	}

	row, err := e.queries.RollupEntitySummary(ctx, db.RollupEntitySummaryParams{EntityType: dbType, EntityID: entityID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return entitySummaryResult{}, err
	}

	result := entitySummaryResult{Plays: int64(row.Plays), UniqueListeners: int64(row.UniqueListeners)}
	if row.FirstListenedAt.Valid {
		result.FirstListenedAt = &row.FirstListenedAt.Time
	}
	return result, nil
}

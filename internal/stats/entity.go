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

// entitySummaryResult is stats.entity_summary's response payload: a catalog entity's stats, globally or for one user.
type entitySummaryResult struct {
	Plays           int64      `json:"plays"`
	UniqueListeners *int64     `json:"unique_listeners,omitempty"`
	MinutesListened float64    `json:"minutes_listened"`
	FirstListenedAt *time.Time `json:"first_listened_at,omitempty"`
}

// EntitySummary computes one catalog entity's play count and unique listener count, globally or for userID alone.
func (e *Engine) EntitySummary(ctx context.Context, userID *int64, entityType string, entityID int64) (json.RawMessage, error) {
	key := cacheKey{UserID: userID, Resource: db.StatsResourceSummary, Params: struct{}{}}
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
		return e.computeEntitySummary(ctx, userID, entityType, entityID)
	})
}

// computeEntitySummary runs RollupEntitySummary for global scope, or a live per-user query for a scoped user.
func (e *Engine) computeEntitySummary(ctx context.Context, userID *int64, entityType string, entityID int64) (entitySummaryResult, error) {
	if userID != nil {
		return e.computeEntityUserSummary(ctx, *userID, entityType, entityID)
	}

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

	uniqueListeners := int64(row.UniqueListeners)
	result := entitySummaryResult{Plays: int64(row.Plays), UniqueListeners: &uniqueListeners, MinutesListened: row.MinutesListened}
	if row.FirstListenedAt.Valid {
		result.FirstListenedAt = &row.FirstListenedAt.Time
	}
	return result, nil
}

// computeEntityUserSummary runs EntityUserSummary for one user's listens of entityType/entityID.
func (e *Engine) computeEntityUserSummary(ctx context.Context, userID int64, entityType string, entityID int64) (entitySummaryResult, error) {
	params := db.EntityUserSummaryParams{UserID: userID}
	switch entityType {
	case "artist":
		params.ArtistID = &entityID
	case "album":
		params.AlbumID = &entityID
	case "song":
		params.SongID = &entityID
	default:
		return entitySummaryResult{}, fmt.Errorf("stats: invalid entity type %q", entityType)
	}

	row, err := e.queries.EntityUserSummary(ctx, params)
	if err != nil {
		return entitySummaryResult{}, err
	}

	result := entitySummaryResult{Plays: row.Plays, MinutesListened: row.MinutesListened}
	if row.FirstListenedAt.Valid {
		result.FirstListenedAt = &row.FirstListenedAt.Time
	}
	return result, nil
}

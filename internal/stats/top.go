package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// entityImageURL renders id as the same "/api/images/{uuid}/medium" path the entity-response builders use, or nil when unset.
func entityImageURL(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	url := "/api/images/" + uuid.UUID(id.Bytes).String() + "/medium"
	return &url
}

// TopKind selects which entity type stats.Top ranks.
type TopKind string

const (
	TopArtists TopKind = "artists"
	TopAlbums  TopKind = "albums"
	TopTracks  TopKind = "tracks"
)

// topParams is stats.top's cache-key params: Timeframe plus scope/pagination.
type topParams struct {
	Timeframe
	Kind     TopKind `json:"kind"`
	ArtistID *int64  `json:"artist_id,omitempty"`
	AlbumID  *int64  `json:"album_id,omitempty"`
	Page     int     `json:"page"`
	PerPage  int     `json:"per_page"`
	SortBy   string  `json:"sort_by"`
}

// topEntry is one leaderboard row; ArtistName is the primary artist (omitted for artist rows) and AlbumName is track-only.
type topEntry struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	ArtistID        int64   `json:"artist_id,omitempty"`
	ArtistName      string  `json:"artist_name,omitempty"`
	AlbumID         int64   `json:"album_id,omitempty"`
	AlbumName       string  `json:"album_name,omitempty"`
	ImageURL        *string `json:"image_url,omitempty"`
	ListenCount     int64   `json:"listen_count"`
	MinutesListened float64 `json:"minutes_listened"`
	Blacklisted     bool    `json:"blacklisted"`
}

// Top computes userID's leaderboard for kind within tf and sortBy, scoped to artistID/albumID when given (tracks only).
func (e *Engine) Top(ctx context.Context, userID *int64, kind TopKind, tf Timeframe, artistID, albumID *int64, page, perPage int, sortBy string) (json.RawMessage, error) {
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	params := topParams{Timeframe: tf, Kind: kind, ArtistID: artistID, AlbumID: albumID, Page: page, PerPage: perPage, SortBy: sortBy}
	key := cacheKey{UserID: userID, Resource: topResource(kind), Params: params}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeTop(ctx, userID, kind, from, to, artistID, albumID, page, perPage, sortBy)
	})
}

// topResource maps a TopKind to its stats_cache resource.
func topResource(kind TopKind) db.StatsResource {
	switch kind {
	case TopAlbums:
		return db.StatsResourceTopAlbums
	case TopTracks:
		return db.StatsResourceTopTracks
	default:
		return db.StatsResourceTopArtists
	}
}

// computeTop runs the query for kind and returns its leaderboard rows.
func (e *Engine) computeTop(ctx context.Context, userID *int64, kind TopKind, from, to time.Time, artistID, albumID *int64, page, perPage int, sortBy string) ([]topEntry, error) {
	offset := int32((page - 1) * perPage)
	limit := int32(perPage)

	switch kind {
	case TopArtists:
		rows, err := e.queries.RollupTopArtists(ctx, db.RollupTopArtistsParams{
			UserID: userID, FromDay: day(from), ToDay: day(to), MaxRows: limit, RowOffset: offset, SortBy: sortBy,
		})
		if err != nil {
			return nil, err
		}
		entries := make([]topEntry, len(rows))
		for i, r := range rows {
			entries[i] = topEntry{
				ID: r.ArtistID, Name: r.Name, ImageURL: entityImageURL(r.ImageID),
				ListenCount: r.ListenCount, MinutesListened: r.MinutesListened, Blacklisted: r.Blacklisted,
			}
		}
		return entries, nil
	case TopAlbums:
		rows, err := e.queries.RollupTopAlbums(ctx, db.RollupTopAlbumsParams{
			UserID: userID, FromDay: day(from), ToDay: day(to), MaxRows: limit, RowOffset: offset, SortBy: sortBy,
		})
		if err != nil {
			return nil, err
		}
		entries := make([]topEntry, len(rows))
		for i, r := range rows {
			entries[i] = topEntry{
				ID: r.AlbumID, Name: r.Name, ArtistID: r.ArtistID, ArtistName: r.ArtistName, ImageURL: entityImageURL(r.ImageID),
				ListenCount: r.ListenCount, MinutesListened: r.MinutesListened, Blacklisted: r.Blacklisted,
			}
		}
		return entries, nil
	case TopTracks:
		rows, err := e.queries.RollupTopTracks(ctx, db.RollupTopTracksParams{
			UserID: userID, FromDay: day(from), ToDay: day(to), ArtistID: artistID, AlbumID: albumID, MaxRows: limit, RowOffset: offset, SortBy: sortBy,
		})
		if err != nil {
			return nil, err
		}
		entries := make([]topEntry, len(rows))
		for i, r := range rows {
			entries[i] = topEntry{
				ID: r.SongID, Name: r.Name, ArtistID: r.ArtistID, ArtistName: r.ArtistName, AlbumID: r.AlbumID, AlbumName: r.AlbumName, ImageURL: entityImageURL(r.ImageID),
				ListenCount: r.ListenCount, MinutesListened: r.MinutesListened, Blacklisted: r.Blacklisted,
			}
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("stats: invalid top kind %q", kind)
	}
}

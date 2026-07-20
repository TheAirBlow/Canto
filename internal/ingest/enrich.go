package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/images"
)

// enrichTimeout bounds each background enrichment call, independent of the request that triggered it.
const enrichTimeout = 30 * time.Second

// enrichArtist fills in a newly created artist's description/thumbnail via its attached sources, best-effort in the background.
func (s *Service) enrichArtist(artistID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
		defer cancel()

		full, ok := s.lookup.Artist(ctx, artistID)
		if !ok {
			return
		}

		imageID := s.downloadThumbnail(full.ThumbnailURL)
		if full.Description == "" && imageID == nil {
			return
		}
		if err := s.queries.UpdateArtistMetadata(ctx, db.UpdateArtistMetadataParams{
			ID: artistID, Description: nilIfEmpty(full.Description), ImageID: uuidParam(imageID),
		}); err != nil {
			slog.Warn("update artist metadata failed", "id", artistID, "err", err)
		}
	}()
}

// enrichAlbum fills in a newly created album's description/thumbnail via its attached sources and correlates its full track listing into the catalog.
func (s *Service) enrichAlbum(albumID int64, matchers []correlate.FuzzyMatcher, normalize bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
		defer cancel()

		full, processor, ok := s.lookup.Album(ctx, albumID)
		if !ok {
			return
		}

		imageID := s.downloadThumbnail(full.ThumbnailURL)
		if full.Description != "" || imageID != nil {
			if err := s.queries.UpdateAlbumMetadata(ctx, db.UpdateAlbumMetadataParams{
				ID: albumID, Description: nilIfEmpty(full.Description), ImageID: uuidParam(imageID),
			}); err != nil {
				slog.Warn("update album metadata failed", "id", albumID, "err", err)
			}
		}

		sourceType := processor.Type()
		for _, track := range full.Songs {
			trackArtistIDs := make([]int64, 0, len(track.Artists))
			for _, artistMeta := range track.Artists {
				artistID, created, err := s.engine.ResolveArtist(ctx, artistMeta.Name, sourceType, artistMeta.ExtractedID, "", matchers, normalize)
				if err != nil {
					slog.Warn("enrich album: resolve track artist failed", "album", albumID, "err", err)
					continue
				}
				trackArtistIDs = append(trackArtistIDs, artistID)
				if created {
					s.enrichArtist(artistID)
				}
			}

			var trackDurationMs *int32
			if track.DurationMs > 0 {
				trackDurationMs = &track.DurationMs
			}
			trackNumber := track.TrackNumber
			songID, created, err := s.engine.ResolveSong(ctx, track.Name, sourceType, track.ExtractedID, "", trackDurationMs, trackArtistIDs, &albumID, &trackNumber, matchers, normalize)
			if err != nil {
				slog.Warn("enrich album: resolve track failed", "album", albumID, "track", track.Name, "err", err)
				continue
			}
			if created && track.ThumbnailURL != "" {
				s.enrichSongThumbnail(songID, track.ThumbnailURL)
			}
		}
	}()
}

// enrichSongThumbnail downloads and stores a newly created song's thumbnail, best-effort in the background.
func (s *Service) enrichSongThumbnail(songID int64, thumbnailURL string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
		defer cancel()

		imageID := s.downloadThumbnail(thumbnailURL)
		if imageID == nil {
			return
		}
		if err := s.queries.UpdateSongThumbnail(ctx, db.UpdateSongThumbnailParams{ID: songID, ImageID: uuidParam(imageID)}); err != nil {
			slog.Warn("update song thumbnail failed", "id", songID, "err", err)
		}
	}()
}

// downloadThumbnail downloads url into a freshly minted image cache entry, returning its id, or nil on an empty url or failure.
func (s *Service) downloadThumbnail(url string) *uuid.UUID {
	if url == "" {
		return nil
	}
	id := uuid.New()
	if err := images.Download(id, url); err != nil {
		slog.Warn("thumbnail download failed", "url", url, "err", err)
		return nil
	}
	return &id
}

// nilIfEmpty returns nil for an empty string, else a pointer to s.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// uuidParam converts an optional uuid.UUID into a nullable pgtype.UUID.
func uuidParam(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

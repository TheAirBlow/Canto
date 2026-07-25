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
	"Canto/internal/source"
)

// enrichTimeout bounds a backgrounded (non-imported) enrichment call, independent of the request that triggered it.
const enrichTimeout = 30 * time.Second

// runEnrichment runs fn now on the calling goroutine when imported is true, so a bulk import job's own worker blocks on it and its progress/completion actually reflects the work; otherwise fn runs in the background under a fresh bounded ctx and enrichSem.
func (s *Service) runEnrichment(ctx context.Context, imported bool, fn func(ctx context.Context)) {
	if imported {
		fn(ctx)
		return
	}
	go func() {
		s.enrichSem <- struct{}{}
		defer func() { <-s.enrichSem }()

		ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// enrichArtist fills in a newly created artist's description/thumbnail via its attached sources, best-effort.
func (s *Service) enrichArtist(ctx context.Context, artistID int64, imported bool) {
	s.runEnrichment(ctx, imported, func(ctx context.Context) {
		full, ok := s.lookup.Artist(ctx, artistID)
		if !ok {
			return
		}

		imageID := s.downloadThumbnail(ctx, full.ThumbnailURL)
		if full.Description == "" && imageID == nil {
			return
		}
		if err := s.queries.UpdateArtistMetadata(ctx, db.UpdateArtistMetadataParams{
			ID: artistID, Description: nilIfEmpty(full.Description), ImageID: uuidParam(imageID),
		}); err != nil {
			slog.Warn("update artist metadata failed", "id", artistID, "err", err)
		}
	})
}

// enrichAlbum fills in a newly created album's description/thumbnail via its attached sources and correlates its full track listing into the catalog, best-effort.
func (s *Service) enrichAlbum(ctx context.Context, albumID int64, matchers []correlate.FuzzyMatcher, imported bool) {
	s.runEnrichment(ctx, imported, func(ctx context.Context) {
		full, processor, ok := s.lookup.Album(ctx, albumID)
		if !ok {
			return
		}

		imageID := s.downloadThumbnail(ctx, full.ThumbnailURL)
		if full.Description != "" || imageID != nil {
			if err := s.queries.UpdateAlbumMetadata(ctx, db.UpdateAlbumMetadataParams{
				ID: albumID, Description: nilIfEmpty(full.Description), ImageID: uuidParam(imageID),
			}); err != nil {
				slog.Warn("update album metadata failed", "id", albumID, "err", err)
			}
		}

		sourceType := processor.Type()
		for _, track := range full.Songs {
			trackArtistNames := source.Names(track.Artists)
			trackArtistIDs := make([]int64, 0, len(track.Artists))
			for _, artistMeta := range track.Artists {
				artistID, created, err := s.engine.ResolveArtist(ctx, artistMeta.Name, sourceType, artistMeta.ExtractedID, "", matchers)
				if err != nil {
					slog.Warn("enrich album: resolve track artist failed", "album", albumID, "err", err)
					continue
				}
				trackArtistIDs = append(trackArtistIDs, artistID)
				if created {
					s.enrichArtist(ctx, artistID, imported)
				}
			}

			var trackDurationMs *int32
			if track.DurationMs > 0 {
				trackDurationMs = &track.DurationMs
			}
			trackNumber := track.TrackNumber
			songID, created, err := s.engine.ResolveSong(ctx, track.Name, sourceType, track.ExtractedID, "", trackDurationMs, trackArtistIDs, trackArtistNames, &albumID, &trackNumber, matchers)
			if err != nil {
				slog.Warn("enrich album: resolve track failed", "album", albumID, "track", track.Name, "err", err)
				continue
			}
			if created && track.ThumbnailURL != "" {
				s.enrichSongThumbnail(ctx, songID, track.ThumbnailURL, imported)
			}
		}
	})
}

// enrichSongThumbnail downloads and stores a newly created song's thumbnail, best-effort.
func (s *Service) enrichSongThumbnail(ctx context.Context, songID int64, thumbnailURL string, imported bool) {
	s.runEnrichment(ctx, imported, func(ctx context.Context) {
		imageID := s.downloadThumbnail(ctx, thumbnailURL)
		if imageID == nil {
			return
		}
		if err := s.queries.UpdateSongThumbnail(ctx, db.UpdateSongThumbnailParams{ID: songID, ImageID: uuidParam(imageID)}); err != nil {
			slog.Warn("update song thumbnail failed", "id", songID, "err", err)
		}
	})
}

// downloadThumbnail downloads url into a freshly minted image cache entry, returning its id, or nil on an empty url or failure.
func (s *Service) downloadThumbnail(ctx context.Context, url string) *uuid.UUID {
	if url == "" {
		return nil
	}
	id := uuid.New()
	if err := images.Download(ctx, id, url); err != nil {
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

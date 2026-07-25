package refresh

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/enrich"
	"Canto/internal/images"
	"Canto/internal/source"
)

// scanInterval is how often the worker checks for stale rows, independent of Config.Interval.
const scanInterval = time.Hour

// batchSize bounds how many stale rows of one entity type are refreshed per scan.
const batchSize = 50

// Config controls which entity types the worker refreshes and how stale they must be first.
type Config struct {
	Interval time.Duration
	Entities []string
}

// Worker periodically re-fetches metadata for artists/albums/songs whose data has gone stale.
type Worker struct {
	cfg      Config
	queries  *db.Queries
	engine   *correlate.Engine
	lookup   *enrich.Lookup
	matchers []correlate.FuzzyMatcher
}

// NewWorker builds a Worker. matchers seed any catalog backfill triggered by a refreshed album.
func NewWorker(cfg Config, queries *db.Queries, registry *source.Registry, engine *correlate.Engine, matchers []correlate.FuzzyMatcher) *Worker {
	return &Worker{cfg: cfg, queries: queries, engine: engine, lookup: enrich.NewLookup(queries, registry), matchers: matchers}
}

// Enabled reports whether any entity type is configured for refresh.
func (w *Worker) Enabled() bool {
	return len(w.cfg.Entities) > 0
}

// Run scans for and refreshes stale entities every scanInterval until ctx is canceled. Blocks; call it in a goroutine.
func (w *Worker) Run(ctx context.Context) {
	if !w.Enabled() {
		slog.Info("refresh worker disabled, no entity types configured")
		return
	}

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	w.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan(ctx)
		}
	}
}

// scan refreshes one batch of stale rows for each configured entity type.
func (w *Worker) scan(ctx context.Context) {
	before := pgtype.Timestamptz{Time: time.Now().Add(-w.cfg.Interval), Valid: true}

	if slices.Contains(w.cfg.Entities, "artist") {
		artists, err := w.queries.ListStaleArtists(ctx, db.ListStaleArtistsParams{Before: before, MaxRows: batchSize})
		if err != nil {
			slog.Warn("refresh: list stale artists failed", "err", err)
		}
		for _, artist := range artists {
			w.refreshArtist(ctx, artist)
		}
	}
	if slices.Contains(w.cfg.Entities, "album") {
		albums, err := w.queries.ListStaleAlbums(ctx, db.ListStaleAlbumsParams{Before: before, MaxRows: batchSize})
		if err != nil {
			slog.Warn("refresh: list stale albums failed", "err", err)
		}
		for _, album := range albums {
			w.refreshAlbum(ctx, album)
		}
	}
	if slices.Contains(w.cfg.Entities, "song") {
		songs, err := w.queries.ListStaleSongs(ctx, db.ListStaleSongsParams{Before: before, MaxRows: batchSize})
		if err != nil {
			slog.Warn("refresh: list stale songs failed", "err", err)
		}
		for _, song := range songs {
			w.refreshSong(ctx, song)
		}
	}
}

// refreshArtist re-fetches artist's profile via its attached sources, if any, preserving existing fields the fetch didn't improve on.
func (w *Worker) refreshArtist(ctx context.Context, artist db.Artist) {
	description, imageID := artist.Description, artist.ImageID

	if fetched, ok := w.lookup.Artist(ctx, artist.ID); ok {
		if fetched.Description != "" {
			description = &fetched.Description
		}
		if newID := w.downloadThumbnail(ctx, fetched.ThumbnailURL); newID != nil {
			imageID = uuidParam(newID)
		}
	}

	if err := w.queries.UpdateArtistMetadata(ctx, db.UpdateArtistMetadataParams{ID: artist.ID, Description: description, ImageID: imageID}); err != nil {
		slog.Warn("refresh: update artist failed", "id", artist.ID, "err", err)
		return
	}
	if imageID != artist.ImageID {
		images.DeleteIfSet(artist.ImageID)
	}
}

// refreshAlbum re-fetches album's profile and track listing via its attached sources, if any.
func (w *Worker) refreshAlbum(ctx context.Context, album db.Album) {
	description, imageID := album.Description, album.ImageID

	fetched, processor, ok := w.lookup.Album(ctx, album.ID)
	if ok {
		if fetched.Description != "" {
			description = &fetched.Description
		}
		if newID := w.downloadThumbnail(ctx, fetched.ThumbnailURL); newID != nil {
			imageID = uuidParam(newID)
		}
	}

	if err := w.queries.UpdateAlbumMetadata(ctx, db.UpdateAlbumMetadataParams{ID: album.ID, Description: description, ImageID: imageID}); err != nil {
		slog.Warn("refresh: update album failed", "id", album.ID, "err", err)
	} else if imageID != album.ImageID {
		images.DeleteIfSet(album.ImageID)
	}
	if ok {
		w.backfillTracks(ctx, processor, album.ID, fetched.Songs)
	}
}

// refreshSong re-fetches song's thumbnail via its attached sources, if any.
func (w *Worker) refreshSong(ctx context.Context, song db.Song) {
	imageID := song.ImageID

	if fetched, ok := w.lookup.Song(ctx, song.ID); ok {
		if newID := w.downloadThumbnail(ctx, fetched.ThumbnailURL); newID != nil {
			imageID = uuidParam(newID)
		}
	}

	if err := w.queries.UpdateSongThumbnail(ctx, db.UpdateSongThumbnailParams{ID: song.ID, ImageID: imageID}); err != nil {
		slog.Warn("refresh: update song failed", "id", song.ID, "err", err)
		return
	}
	if imageID != song.ImageID {
		images.DeleteIfSet(song.ImageID)
	}
}

// backfillTracks correlates an album's full track listing into the catalog, same as at album-creation time.
func (w *Worker) backfillTracks(ctx context.Context, processor source.Processor, albumID int64, tracks []source.AlbumTrack) {
	sourceType := processor.Type()
	for _, track := range tracks {
		artistNames := source.Names(track.Artists)
		artistIDs := make([]int64, 0, len(track.Artists))
		for _, artistMeta := range track.Artists {
			artistID, _, err := w.engine.ResolveArtist(ctx, artistMeta.Name, sourceType, artistMeta.ExtractedID, "", w.matchers)
			if err != nil {
				slog.Warn("refresh: resolve track artist failed", "album", albumID, "err", err)
				continue
			}
			artistIDs = append(artistIDs, artistID)
		}

		var durationMs *int32
		if track.DurationMs > 0 {
			durationMs = &track.DurationMs
		}
		trackNumber := track.TrackNumber
		songID, created, err := w.engine.ResolveSong(ctx, track.Name, sourceType, track.ExtractedID, "", durationMs, artistIDs, artistNames, &albumID, &trackNumber, w.matchers)
		if err != nil {
			slog.Warn("refresh: resolve track failed", "album", albumID, "track", track.Name, "err", err)
			continue
		}
		if created && track.ThumbnailURL != "" {
			if imageID := w.downloadThumbnail(ctx, track.ThumbnailURL); imageID != nil {
				if err := w.queries.UpdateSongThumbnail(ctx, db.UpdateSongThumbnailParams{ID: songID, ImageID: uuidParam(imageID)}); err != nil {
					slog.Warn("refresh: update track thumbnail failed", "song", songID, "err", err)
				}
			}
		}
	}
}

// downloadThumbnail downloads url into a freshly minted image cache entry, returning its id, or nil on an empty url or failure.
func (w *Worker) downloadThumbnail(ctx context.Context, url string) *uuid.UUID {
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

// uuidParam converts an optional uuid.UUID into a nullable pgtype.UUID.
func uuidParam(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

package enrich

import (
	"context"
	"log/slog"

	"Canto/internal/db"
	"Canto/internal/source"
)

// Lookup resolves an entity's full metadata by trying each of its attached sources in turn.
type Lookup struct {
	queries  *db.Queries
	registry *source.Registry
}

// NewLookup builds a Lookup backed by queries and registry.
func NewLookup(queries *db.Queries, registry *source.Registry) *Lookup {
	return &Lookup{queries: queries, registry: registry}
}

// Artist tries artistID's attached sources in order, returning the first one that yields full artist metadata.
func (l *Lookup) Artist(ctx context.Context, artistID int64) (source.ArtistMetadata, bool) {
	for processor, extractedID := range l.sources(ctx, db.EntityTypeArtist, artistID) {
		if !processor.State(ctx).CanFetchArtist {
			continue
		}
		if a, ok := processor.(source.Availabler); ok && !a.Available() {
			continue
		}
		meta, err := processor.FetchArtist(ctx, extractedID)
		if err != nil {
			slog.Warn("enrich: fetch artist failed", "processor", processor.ID(), "id", extractedID, "err", err)
			continue
		}
		return meta, true
	}
	return source.ArtistMetadata{}, false
}

// Album tries albumID's attached sources in order, returning the first one that yields full album metadata, along with the processor that provided it.
func (l *Lookup) Album(ctx context.Context, albumID int64) (source.AlbumMetadata, source.Processor, bool) {
	for processor, extractedID := range l.sources(ctx, db.EntityTypeAlbum, albumID) {
		if !processor.State(ctx).CanFetchAlbum {
			continue
		}
		meta, err := processor.FetchAlbum(ctx, extractedID)
		if err != nil {
			slog.Warn("enrich: fetch album failed", "processor", processor.ID(), "id", extractedID, "err", err)
			continue
		}
		return meta, processor, true
	}
	return source.AlbumMetadata{}, nil, false
}

// Song tries songID's attached sources in order, returning the first one that yields refreshed song metadata.
func (l *Lookup) Song(ctx context.Context, songID int64) (source.Metadata, bool) {
	for processor, extractedID := range l.sources(ctx, db.EntityTypeSong, songID) {
		if !processor.State(ctx).CanDetect {
			continue
		}
		meta, _, err := processor.FetchMetadata(ctx, extractedID)
		if err != nil {
			slog.Warn("enrich: fetch song failed", "processor", processor.ID(), "id", extractedID, "err", err)
			continue
		}
		return meta, true
	}
	return source.Metadata{}, false
}

// sources yields processor and extractedID for entityID's attached sources, in attachment order, for every source_type with a registered processor.
func (l *Lookup) sources(ctx context.Context, entityType db.EntityType, entityID int64) func(func(source.Processor, string) bool) {
	return func(yield func(source.Processor, string) bool) {
		rows, err := l.queries.ListSourcesForEntity(ctx, db.ListSourcesForEntityParams{EntityType: entityType, EntityID: entityID})
		if err != nil {
			slog.Warn("enrich: list sources failed", "entity", entityType, "id", entityID, "err", err)
			return
		}
		for _, row := range rows {
			if row.ExtractedID == nil {
				continue
			}
			processor, ok := l.registry.ByType(source.SourceType(row.SourceType))
			if !ok {
				continue
			}
			if !yield(processor, *row.ExtractedID) {
				return
			}
		}
	}
}

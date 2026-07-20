// Package export produces Canto's own listen-history export format, which doubles as an import format.
package export

import (
	"context"
	"fmt"
	"time"

	"Canto/internal/db"
)

// Format/Version identify Canto's own export shape for importers.
const (
	Format  = "canto_export"
	Version = 1
)

// pageSize bounds how many listens Export fetches per round trip.
const pageSize = 1000

// Source is one attached source on an artist/album/song.
type Source struct {
	SourceType        string   `json:"source_type"`
	RawURL            *string  `json:"raw_url,omitempty"`
	ExtractedID       *string  `json:"extracted_id,omitempty"`
	CorrelationMethod string   `json:"correlation_method"`
	Confidence        *float32 `json:"confidence,omitempty"`
}

// Artist is a full artist entity, keyed by id in Export.Artists.
type Artist struct {
	Name           string    `json:"name"`
	NameNormalized string    `json:"name_normalized"`
	Description    *string   `json:"description,omitempty"`
	Sources        []Source  `json:"sources,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Album is a full album entity, keyed by id in Export.Albums.
type Album struct {
	Name           string    `json:"name"`
	NameNormalized string    `json:"name_normalized"`
	ReleaseDate    *string   `json:"release_date,omitempty"`
	Description    *string   `json:"description,omitempty"`
	ArtistIDs      []int64   `json:"artist_ids,omitempty"`
	Sources        []Source  `json:"sources,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Song is a full song entity, keyed by id in Export.Songs.
type Song struct {
	Name           string    `json:"name"`
	NameNormalized string    `json:"name_normalized"`
	DurationMs     *int32    `json:"duration_ms,omitempty"`
	ArtistIDs      []int64   `json:"artist_ids,omitempty"`
	AlbumID        *int64    `json:"album_id,omitempty"`
	Sources        []Source  `json:"sources,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Listen is one exported listen, referencing its song by id into Export.Songs.
type Listen struct {
	SongID     int64     `json:"song_id"`
	ListenedAt time.Time `json:"listened_at"`
	Client     string    `json:"client,omitempty"`
}

// Export is a user's full listen history: deduplicated Artists/Albums/Songs maps keyed by id, referenced by id elsewhere.
type Export struct {
	Format  string           `json:"format"`
	Version int              `json:"version"`
	Artists map[int64]Artist `json:"artists"`
	Albums  map[int64]Album  `json:"albums"`
	Songs   map[int64]Song   `json:"songs"`
	Listens []Listen         `json:"listens"`
}

// Service builds a user's export.
type Service struct {
	queries *db.Queries
}

// NewService builds a Service backed by queries.
func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

// Export builds userID's full listen-history export, paginating internally.
func (s *Service) Export(ctx context.Context, userID int64) (Export, error) {
	out := Export{
		Format: Format, Version: Version,
		Artists: make(map[int64]Artist), Albums: make(map[int64]Album), Songs: make(map[int64]Song),
	}

	var offset int32
	for {
		rows, err := s.queries.ListListensForUser(ctx, db.ListListensForUserParams{
			UserID: userID, MaxRows: pageSize, RowOffset: offset,
		})
		if err != nil {
			return Export{}, fmt.Errorf("export: list listens: %w", err)
		}
		for _, row := range rows {
			if err := s.addSong(ctx, &out, row.SongID); err != nil {
				return Export{}, err
			}
			listen := Listen{SongID: row.SongID, ListenedAt: row.ListenedAt.Time}
			if row.Client != nil {
				listen.Client = *row.Client
			}
			out.Listens = append(out.Listens, listen)
		}
		if len(rows) < pageSize {
			break
		}
		offset += pageSize
	}

	return out, nil
}

// addSong adds songID (and its artists/album) to out if not already present.
func (s *Service) addSong(ctx context.Context, out *Export, songID int64) error {
	if _, ok := out.Songs[songID]; ok {
		return nil
	}
	row, err := s.queries.GetSongByID(ctx, songID)
	if err != nil {
		return fmt.Errorf("export: song %d: %w", songID, err)
	}

	artistIDs, err := s.addArtistsForSong(ctx, out, songID)
	if err != nil {
		return err
	}

	var albumID *int64
	if albumRow, err := s.queries.GetAlbumForSong(ctx, songID); err == nil {
		albumID = &albumRow.ID
		album := db.Album{
			ID: albumRow.ID, Name: albumRow.Name, NameNormalized: albumRow.NameNormalized,
			ReleaseDate: albumRow.ReleaseDate, Description: albumRow.Description,
			CreatedAt: albumRow.CreatedAt, UpdatedAt: albumRow.UpdatedAt,
		}
		if err := s.addAlbum(ctx, out, album); err != nil {
			return err
		}
	}

	sources, err := s.sourcesFor(ctx, db.EntityTypeSong, songID)
	if err != nil {
		return err
	}
	out.Songs[songID] = Song{
		Name: row.Name, NameNormalized: row.NameNormalized, DurationMs: row.DurationMs,
		ArtistIDs: artistIDs, AlbumID: albumID, Sources: sources,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	return nil
}

// addAlbum adds album (and its artists) to out if not already present.
func (s *Service) addAlbum(ctx context.Context, out *Export, album db.Album) error {
	if _, ok := out.Albums[album.ID]; ok {
		return nil
	}

	artistIDs, err := s.addArtistsForAlbum(ctx, out, album.ID)
	if err != nil {
		return err
	}
	sources, err := s.sourcesFor(ctx, db.EntityTypeAlbum, album.ID)
	if err != nil {
		return err
	}
	var releaseDate *string
	if album.ReleaseDate.Valid {
		formatted := album.ReleaseDate.Time.Format("2006-01-02")
		releaseDate = &formatted
	}
	out.Albums[album.ID] = Album{
		Name: album.Name, NameNormalized: album.NameNormalized, ReleaseDate: releaseDate,
		Description: album.Description, ArtistIDs: artistIDs, Sources: sources,
		CreatedAt: album.CreatedAt.Time, UpdatedAt: album.UpdatedAt.Time,
	}
	return nil
}

// addArtist adds artist to out if not already present.
func (s *Service) addArtist(ctx context.Context, out *Export, artist db.Artist) error {
	if _, ok := out.Artists[artist.ID]; ok {
		return nil
	}
	sources, err := s.sourcesFor(ctx, db.EntityTypeArtist, artist.ID)
	if err != nil {
		return err
	}
	out.Artists[artist.ID] = Artist{
		Name: artist.Name, NameNormalized: artist.NameNormalized, Description: artist.Description,
		Sources: sources, CreatedAt: artist.CreatedAt.Time, UpdatedAt: artist.UpdatedAt.Time,
	}
	return nil
}

// addArtistsForSong adds songID's linked artists to out, returning their ids.
func (s *Service) addArtistsForSong(ctx context.Context, out *Export, songID int64) ([]int64, error) {
	rows, err := s.queries.ListArtistsForSong(ctx, songID)
	if err != nil {
		return nil, fmt.Errorf("export: song %d artists: %w", songID, err)
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		if err := s.addArtist(ctx, out, row); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// addArtistsForAlbum adds albumID's linked artists to out, returning their ids.
func (s *Service) addArtistsForAlbum(ctx context.Context, out *Export, albumID int64) ([]int64, error) {
	rows, err := s.queries.ListArtistsForAlbum(ctx, albumID)
	if err != nil {
		return nil, fmt.Errorf("export: album %d artists: %w", albumID, err)
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		if err := s.addArtist(ctx, out, row); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// sourcesFor returns id's attached sources for entityType.
func (s *Service) sourcesFor(ctx context.Context, entityType db.EntityType, id int64) ([]Source, error) {
	rows, err := s.queries.ListSourcesForEntity(ctx, db.ListSourcesForEntityParams{EntityType: entityType, EntityID: id})
	if err != nil {
		return nil, fmt.Errorf("export: sources for %s %d: %w", entityType, id, err)
	}
	sources := make([]Source, len(rows))
	for i, row := range rows {
		sources[i] = Source{
			SourceType: string(row.SourceType), RawURL: row.RawUrl, ExtractedID: row.ExtractedID,
			CorrelationMethod: string(row.CorrelationMethod), Confidence: row.Confidence,
		}
	}
	return sources, nil
}

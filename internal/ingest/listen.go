package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/enrich"
	"Canto/internal/source"
)

// ListenInput is the normalized shape every ingest path produces before submitting a listen.
type ListenInput struct {
	UserID           int64     // ID of the user
	OriginURL        string    // URL the listen originated from, if any
	ArtistNames      []string  // Fallback artist names
	SongName         string    // Fallback song name
	AlbumName        string    // Fallback album name
	ListenedAt       time.Time // Time when the listen was submitted
	DurationMs       int32     // Fallback song duration
	DurationPlayedMs int32     // How long this listen actually played, falls back to the song's duration

	SubmissionClient         string // Name of the submission client
	SubmissionClientVersion  string // Version of the submission client
	OriginalSubmissionClient string // Original submission client (if an importer was used)
	MediaPlayer              string // Name of the media player
	MediaPlayerVersion       string // Version of the media player
	MusicService             string // Name of the music service
	MusicServiceName         string // Version of the music service
}

// InferOriginURL derives an origin_url from a recording MBID or a Spotify track id/URI, when no origin_url was given directly.
func InferOriginURL(mbid, spotifyID string) string {
	switch {
	case mbid != "":
		return "https://musicbrainz.org/recording/" + mbid
	case strings.Contains(spotifyID, "://"):
		return spotifyID
	case spotifyID != "":
		return "https://open.spotify.com/track/" + spotifyID
	default:
		return ""
	}
}

// ProcessorSettings is one user's resolved processor/matcher configuration.
type ProcessorSettings struct {
	LinkOrder     []string
	FallbackOrder []string
	MatcherOrder  []string
	Normalize     bool
}

// Service ties the source-processor registry and correlation engine together to resolve and record a listen.
type Service struct {
	registry *source.Registry
	matchers *correlate.MatcherRegistry
	engine   *correlate.Engine
	queries  *db.Queries
	lookup   *enrich.Lookup
}

// NewService builds a Service.
func NewService(registry *source.Registry, matchers *correlate.MatcherRegistry, engine *correlate.Engine, queries *db.Queries) *Service {
	return &Service{registry: registry, matchers: matchers, engine: engine, queries: queries, lookup: enrich.NewLookup(queries, registry)}
}

// SubmitListen resolves in's source, artists, album, and song under settings, then records the result.
func (s *Service) SubmitListen(ctx context.Context, in ListenInput, settings ProcessorSettings, nowPlaying bool) (db.Listen, error) {
	slog.Debug("ingest: submit listen", "user", in.UserID, "origin_url", in.OriginURL, "song", in.SongName, "now_playing", nowPlaying)

	meta := source.Metadata{SongName: in.SongName, DurationMs: in.DurationMs}
	for _, name := range in.ArtistNames {
		meta.Artists = append(meta.Artists, source.ArtistMetadata{Name: name})
	}
	if in.AlbumName != "" {
		meta.Album = &source.AlbumMetadata{Name: in.AlbumName}
	}

	var activeType db.SourceType
	var extractedID, rawURL string

	linkProcessors := s.registry.OrderedLink(ctx, settings.LinkOrder)
	for _, p := range linkProcessors {
		if in.OriginURL == "" || !p.Detect(in.OriginURL) {
			continue
		}
		id, err := p.ExtractID(in.OriginURL)
		if err != nil {
			slog.Warn("extract id failed", "processor", p.ID(), "err", err)
			continue
		}
		fetched, confident, err := p.FetchMetadata(ctx, id)
		if err != nil {
			slog.Warn("link metadata fetch failed", "processor", p.ID(), "id", id, "err", err)
			continue
		}
		meta = mergeMetadata(meta, fetched)
		activeType, extractedID, rawURL = p.Type(), id, in.OriginURL
		if confident {
			slog.Debug("ingest: link metadata resolved", "processor", p.ID(), "id", id, "song", meta.SongName)
			break
		}
		slog.Warn("processor not confident in link metadata, falling through", "processor", p.ID(), "id", id)
		activeType, extractedID, rawURL = "", "", "" // don't attach a low-confidence source row
	}

	if meta.SongName == "" || activeType == "" || meta.DurationMs == 0 {
		query := source.Query{Song: meta.SongName, Artist: firstArtistName(meta.Artists), Album: albumName(meta.Album)}
		for _, p := range s.registry.OrderedFallback(ctx, settings.FallbackOrder) {
			fetched, confident, err := p.FetchMetadataByQuery(ctx, query)
			if err != nil {
				slog.Warn("fallback metadata lookup failed", "processor", p.ID(), "err", err)
				continue
			}
			meta = mergeMetadata(meta, fetched)
			if confident {
				activeType, extractedID, rawURL = p.Type(), fetched.ExtractedID, ""
				break
			}
		}
	}

	if activeType == "" {
		meta = clearMetaSourceIDs(meta)
	}

	matchers := s.matchers.Ordered(settings.MatcherOrder)

	artistIDs := make([]int64, 0, len(meta.Artists))
	for _, artistMeta := range meta.Artists {
		artistID, created, err := s.engine.ResolveArtist(ctx, artistMeta.Name, activeType, artistMeta.ExtractedID, rawURL, matchers, settings.Normalize)
		if err != nil {
			return db.Listen{}, fmt.Errorf("ingest: resolve artist %q: %w", artistMeta.Name, err)
		}
		artistIDs = append(artistIDs, artistID)
		if created {
			s.enrichArtist(artistID)
		}
	}

	var albumID *int64
	if meta.Album != nil {
		id, created, err := s.engine.ResolveAlbum(ctx, meta.Album.Name, activeType, meta.Album.ExtractedID, rawURL, artistIDs, matchers, settings.Normalize)
		if err != nil {
			return db.Listen{}, fmt.Errorf("ingest: resolve album %q: %w", meta.Album.Name, err)
		}
		albumID = &id
		if created {
			s.enrichAlbum(id, matchers, settings.Normalize)
		}
	}

	var durationMs *int32
	if meta.DurationMs > 0 {
		durationMs = &meta.DurationMs
	}
	songID, created, err := s.engine.ResolveSong(ctx, meta.SongName, activeType, extractedID, rawURL, durationMs, artistIDs, albumID, nil, matchers, settings.Normalize)
	if err != nil {
		return db.Listen{}, fmt.Errorf("ingest: resolve song %q: %w", meta.SongName, err)
	}
	if created && meta.ThumbnailURL != "" {
		s.enrichSongThumbnail(songID, meta.ThumbnailURL)
	}

	playedMs := in.DurationPlayedMs
	if playedMs == 0 {
		playedMs = meta.DurationMs
	}

	if nowPlaying {
		if err := s.queries.UpsertNowPlaying(ctx, db.UpsertNowPlayingParams{
			UserID: in.UserID, SongID: songID, DurationMs: meta.DurationMs,
		}); err != nil {
			return db.Listen{}, fmt.Errorf("ingest: upsert now playing: %w", err)
		}
		return db.Listen{}, nil
	}

	var clientPtr *string
	if in.SubmissionClient != "" {
		clientPtr = &in.SubmissionClient
	}
	var playedMsPtr *int32
	if playedMs > 0 {
		playedMsPtr = &playedMs
	}
	extra, err := json.Marshal(listenExtra{
		SubmissionClientVersion:  in.SubmissionClientVersion,
		OriginalSubmissionClient: in.OriginalSubmissionClient,
		MediaPlayer:              in.MediaPlayer,
		MediaPlayerVersion:       in.MediaPlayerVersion,
		MusicService:             in.MusicService,
		MusicServiceName:         in.MusicServiceName,
	})
	if err != nil {
		return db.Listen{}, fmt.Errorf("ingest: marshal listen extra: %w", err)
	}

	listen, err := s.queries.CreateListen(ctx, db.CreateListenParams{
		UserID:           in.UserID,
		SongID:           songID,
		ListenedAt:       pgtype.Timestamptz{Time: in.ListenedAt, Valid: true},
		Client:           clientPtr,
		DurationPlayedMs: playedMsPtr,
		Extra:            extra,
	})
	if err != nil {
		return db.Listen{}, fmt.Errorf("ingest: create listen: %w", err)
	}
	return listen, nil
}

// listenExtra is the client/device metadata a listen carries beyond the correlated song itself.
type listenExtra struct {
	SubmissionClientVersion  string `json:"submission_client_version,omitempty"`
	OriginalSubmissionClient string `json:"original_submission_client,omitempty"`
	MediaPlayer              string `json:"media_player,omitempty"`
	MediaPlayerVersion       string `json:"media_player_version,omitempty"`
	MusicService             string `json:"music_service,omitempty"`
	MusicServiceName         string `json:"music_service_name,omitempty"`
}

// mergeMetadata overlays add's non-empty fields onto base, keeping base's values where add is empty.
func mergeMetadata(base, add source.Metadata) source.Metadata {
	if add.SongName != "" {
		base.SongName = add.SongName
	}
	if add.ExtractedID != "" {
		base.ExtractedID = add.ExtractedID
	}
	if add.ThumbnailURL != "" {
		base.ThumbnailURL = add.ThumbnailURL
	}
	if len(add.Artists) > 0 {
		base.Artists = add.Artists
	}
	if add.Album != nil {
		base.Album = add.Album
	}
	if add.DurationMs > 0 {
		base.DurationMs = add.DurationMs
	}
	return base
}

// clearMetaSourceIDs strips every per-entity ExtractedID off meta, keeping names/other fields, so ids from
// a source no longer considered active can't be paired with an empty source type during resolution.
func clearMetaSourceIDs(meta source.Metadata) source.Metadata {
	meta.ExtractedID = ""
	if len(meta.Artists) > 0 {
		artists := make([]source.ArtistMetadata, len(meta.Artists))
		for i, a := range meta.Artists {
			a.ExtractedID = ""
			artists[i] = a
		}
		meta.Artists = artists
	}
	if meta.Album != nil {
		album := *meta.Album
		album.ExtractedID = ""
		meta.Album = &album
	}
	return meta
}

// firstArtistName returns artists' first name, or "" if artists is empty.
func firstArtistName(artists []source.ArtistMetadata) string {
	if len(artists) > 0 {
		return artists[0].Name
	}
	return ""
}

// albumName returns album's name, or "" if album is nil.
func albumName(album *source.AlbumMetadata) string {
	if album != nil {
		return album.Name
	}
	return ""
}

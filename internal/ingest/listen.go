package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/correlate"
	"Canto/internal/db"
	"Canto/internal/enrich"
	"Canto/internal/rollup"
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
}

// Service ties the source-processor registry and correlation engine together to resolve and record a listen.
type Service struct {
	registry  *source.Registry
	matchers  *correlate.MatcherRegistry
	engine    *correlate.Engine
	queries   *db.Queries
	lookup    *enrich.Lookup
	rollup    *rollup.Writer
	enrichSem chan struct{}
}

// NewService builds a Service, backgrounding at most enrichConcurrency enrichment goroutines at once.
func NewService(registry *source.Registry, matchers *correlate.MatcherRegistry, engine *correlate.Engine, queries *db.Queries, rollupWriter *rollup.Writer, enrichConcurrency int) *Service {
	return &Service{
		registry: registry, matchers: matchers, engine: engine, queries: queries,
		lookup: enrich.NewLookup(queries, registry), rollup: rollupWriter,
		enrichSem: make(chan struct{}, enrichConcurrency),
	}
}

// SubmitListen resolves in's source, artists, album, and song under settings, then records the result; imported marks in as historical, skipping live streak reconciliation.
func (s *Service) SubmitListen(ctx context.Context, in ListenInput, settings ProcessorSettings, nowPlaying, imported bool) (db.Listen, error) {
	slog.Debug("ingest: submit listen", "user", in.UserID, "origin_url", in.OriginURL, "song", in.SongName, "now_playing", nowPlaying)

	songID, artistIDs, albumID, songDurationMs, err := s.resolveListen(ctx, in, settings, imported)
	if err != nil {
		return db.Listen{}, err
	}

	playedMs := in.DurationPlayedMs
	if playedMs == 0 {
		playedMs = songDurationMs
	}

	if nowPlaying {
		if err := s.queries.UpsertNowPlaying(ctx, db.UpsertNowPlayingParams{
			UserID: in.UserID, SongID: songID, DurationMs: songDurationMs,
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
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Debug("ingest: duplicate listen ignored", "user", in.UserID, "song", songID, "listened_at", in.ListenedAt)
		return db.Listen{}, nil
	}
	if err != nil {
		return db.Listen{}, fmt.Errorf("ingest: create listen: %w", err)
	}

	s.rollup.Record(rollup.ListenEvent{
		UserID: in.UserID, SongID: songID, ArtistIDs: artistIDs, AlbumID: albumID,
		ListenedAt: in.ListenedAt, PlayedMs: playedMs, SongDurationMs: songDurationMs,
		Imported: imported,
	})
	return listen, nil
}

// resolveListen resolves in's song, artists, and album under settings, skipping a source fetch entirely when its origin_url is already a known, linked song.
func (s *Service) resolveListen(ctx context.Context, in ListenInput, settings ProcessorSettings, imported bool) (songID int64, artistIDs []int64, albumID *int64, durationMs int32, err error) {
	meta := source.Metadata{SongName: in.SongName, DurationMs: in.DurationMs}
	for _, name := range in.ArtistNames {
		meta.Artists = append(meta.Artists, source.ArtistMetadata{Name: name})
	}
	if in.AlbumName != "" {
		meta.Album = &source.AlbumMetadata{Name: in.AlbumName}
	}

	var activeType source.SourceType
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
		if knownID, knownArtists, knownAlbum, knownDuration, ok := s.knownSong(ctx, p.Type(), id); ok {
			return knownID, knownArtists, knownAlbum, knownDuration, nil
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

	artistNames := source.Names(meta.Artists)
	artistIDs = make([]int64, 0, len(meta.Artists))
	for _, artistMeta := range meta.Artists {
		artistID, created, err := s.engine.ResolveArtist(ctx, artistMeta.Name, activeType, artistMeta.ExtractedID, rawURL, matchers)
		if err != nil {
			return 0, nil, nil, 0, fmt.Errorf("ingest: resolve artist %q: %w", artistMeta.Name, err)
		}
		artistIDs = append(artistIDs, artistID)
		if created {
			s.enrichArtist(ctx, artistID, imported)
		}
	}

	if meta.Album != nil {
		id, created, err := s.engine.ResolveAlbum(ctx, meta.Album.Name, activeType, meta.Album.ExtractedID, rawURL, artistIDs, artistNames, matchers)
		if err != nil {
			return 0, nil, nil, 0, fmt.Errorf("ingest: resolve album %q: %w", meta.Album.Name, err)
		}
		albumID = &id
		if created {
			s.enrichAlbum(ctx, id, matchers, imported)
		}
	}

	var declaredDurationMs *int32
	if meta.DurationMs > 0 {
		declaredDurationMs = &meta.DurationMs
	}
	songID, created, err := s.engine.ResolveSong(ctx, meta.SongName, activeType, extractedID, rawURL, declaredDurationMs, artistIDs, artistNames, albumID, nil, matchers)
	if err != nil {
		return 0, nil, nil, 0, fmt.Errorf("ingest: resolve song %q: %w", meta.SongName, err)
	}
	if created && meta.ThumbnailURL != "" {
		s.enrichSongThumbnail(ctx, songID, meta.ThumbnailURL, imported)
	}

	// durationMs prefers the song's already-known DB duration over this listen's own possibly-empty fetch.
	durationMs = meta.DurationMs
	if song, err := s.queries.GetSongByID(ctx, songID); err == nil && song.DurationMs != nil && *song.DurationMs > 0 {
		durationMs = *song.DurationMs
	}
	return songID, artistIDs, albumID, durationMs, nil
}

// knownSong reports whether sourceType+id already names a fully linked song, so its caller can skip a source fetch entirely.
func (s *Service) knownSong(ctx context.Context, sourceType source.SourceType, id string) (songID int64, artistIDs []int64, albumID *int64, durationMs int32, ok bool) {
	songID, err := s.queries.GetSourceEntityID(ctx, db.GetSourceEntityIDParams{EntityType: db.EntityTypeSong, SourceType: string(sourceType), ExtractedID: &id})
	if err != nil {
		return 0, nil, nil, 0, false
	}
	song, err := s.queries.GetSongByID(ctx, songID)
	if err != nil {
		return 0, nil, nil, 0, false
	}
	artists, err := s.queries.ListArtistsForSong(ctx, songID)
	if err != nil {
		return 0, nil, nil, 0, false
	}
	artistIDs = make([]int64, len(artists))
	for i, a := range artists {
		artistIDs[i] = a.ID
	}
	if album, err := s.queries.GetAlbumForSong(ctx, songID); err == nil {
		albumID = &album.ID
	}
	if song.DurationMs != nil {
		durationMs = *song.DurationMs
	}
	return songID, artistIDs, albumID, durationMs, true
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

// clearMetaSourceIDs strips every per-entity ExtractedID off meta, so a no-longer-active source's ids can't leak into resolution.
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

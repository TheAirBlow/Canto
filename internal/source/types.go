package source

import (
	"context"
)

// SourceType discriminates which platform a source record was correlated against.
type SourceType string

const (
	SourceTypeYtmusic     SourceType = "ytmusic"
	SourceTypeSpotify     SourceType = "spotify"
	SourceTypeBandcamp    SourceType = "bandcamp"
	SourceTypeMusicbrainz SourceType = "musicbrainz"
	SourceTypeLastfm      SourceType = "lastfm"
	SourceTypeDeezer      SourceType = "deezer"
	SourceTypeSubsonic    SourceType = "subsonic"
	SourceTypeUnknown     SourceType = "unknown"
)

// ArtistMetadata describes one artist, either as a song/album credit or from a full FetchArtist lookup.
type ArtistMetadata struct {
	Name         string
	ExtractedID  string
	ThumbnailURL string
	Description  string
}

// Names returns artists' names, in order.
func Names(artists []ArtistMetadata) []string {
	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}
	return names
}

// AlbumTrack is one track in an album's listing, as returned by FetchAlbum.
type AlbumTrack struct {
	Name         string
	ExtractedID  string
	ThumbnailURL string
	DurationMs   int32
	TrackNumber  int32
	Artists      []ArtistMetadata
}

// AlbumMetadata describes an album. Description/Songs are only populated by FetchAlbum, not a song lookup.
type AlbumMetadata struct {
	Name         string
	ExtractedID  string
	ThumbnailURL string
	Description  string
	Songs        []AlbumTrack
}

// Metadata is what a processor's FetchMetadata/FetchMetadataByQuery call returns for one song.
type Metadata struct {
	SongName     string
	ExtractedID  string
	ThumbnailURL string
	DurationMs   int32
	Artists      []ArtistMetadata
	Album        *AlbumMetadata
}

// Query is the artist/album/song text seed a fallback lookup is searched with.
type Query struct {
	Artist string
	Album  string
	Song   string
}

// State reports what a processor is currently able to do.
type State struct {
	CanDetect      bool // Can recognize and resolve an origin URL via FetchMetadata
	CanLookup      bool // Can search by artist/album/song text via FetchMetadataByQuery
	CanFetchAlbum  bool // Can resolve an album id into full metadata via FetchAlbum
	CanFetchArtist bool // Can resolve an artist id into full metadata via FetchArtist
}

// Availabler is implemented by processors whose availability can change at runtime.
type Availabler interface {
	Available() bool
}

// Processor detects, extracts, and enriches listens originating from one platform.
type Processor interface {
	// ID is this processor's stable identifier.
	ID() string

	// Detect reports whether url belongs to this source.
	Detect(url string) bool

	// ExtractID pulls the platform-native id out of url.
	ExtractID(url string) (id string, err error)

	// FetchMetadata resolves song id into metadata.
	FetchMetadata(ctx context.Context, id string) (meta Metadata, confident bool, err error)

	// FetchMetadataByQuery searches by artist/album/song text.
	FetchMetadataByQuery(ctx context.Context, q Query) (meta Metadata, confident bool, err error)

	// FetchAlbum resolves album id into its full metadata, including track listing.
	FetchAlbum(ctx context.Context, id string) (AlbumMetadata, error)

	// FetchArtist resolves artist id into its full metadata.
	FetchArtist(ctx context.Context, id string) (ArtistMetadata, error)

	// State reports this processor's current capabilities.
	State(ctx context.Context) State

	// Type returns this processor's source type discriminator.
	Type() SourceType
}

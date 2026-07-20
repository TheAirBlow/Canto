package importer

import (
	"io"
	"strings"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// spotifyFormat parses Spotify's extended streaming history export: a JSON array of play entries.
type spotifyFormat struct{}

// newSpotifyFormat builds a spotifyFormat.
func newSpotifyFormat() Format { return spotifyFormat{} }

// ID identifies this format's import_service discriminator.
func (spotifyFormat) ID() db.ImportService { return db.ImportServiceSpotify }

// spotifyEntry is one entry in a Spotify extended-streaming-history export.
type spotifyEntry struct {
	Timestamp    string `json:"ts"`
	MsPlayed     int32  `json:"ms_played"`
	TrackName    string `json:"master_metadata_track_name"`
	ArtistName   string `json:"master_metadata_album_artist_name"`
	AlbumName    string `json:"master_metadata_album_album_name"`
	SpotifyTrack string `json:"spotify_track_uri"`
}

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (spotifyFormat) CountEntries(r io.Reader) (int, error) {
	return countJSONArray(r)
}

// Parse streams r, calling emit once per raw top-level entry.
func (spotifyFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	return decodeJSONArray(r, func(e spotifyEntry) {
		listenedAt, _ := time.Parse(time.RFC3339, e.Timestamp)

		var artists []string
		if e.ArtistName != "" {
			artists = []string{e.ArtistName}
		}
		var originURL string
		if id := strings.TrimPrefix(e.SpotifyTrack, "spotify:track:"); id != "" {
			originURL = "https://open.spotify.com/track/" + id
		}

		emit(ingest.ListenInput{
			OriginalSubmissionClient: "spotify_export",
			OriginURL:                originURL,
			ArtistNames:              artists,
			SongName:                 e.TrackName,
			AlbumName:                e.AlbumName,
			ListenedAt:               listenedAt,
			DurationPlayedMs:         e.MsPlayed,
		})
	})
}

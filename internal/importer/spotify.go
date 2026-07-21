package importer

import (
	"context"
	"io"
	"strings"
	"time"

	"Canto/internal/ingest"
)

// spotifyFormat parses Spotify's extended streaming history export: a JSON array of play entries.
type spotifyFormat struct{}

// newSpotifyFormat builds a spotifyFormat.
func newSpotifyFormat() Format { return spotifyFormat{} }

// ID identifies this format's import_service discriminator.
func (spotifyFormat) ID() ImportService { return ImportServiceSpotify }

// spotifyEntry is one entry in a Spotify extended-streaming-history export.
type spotifyEntry struct {
	Timestamp    string `json:"ts"`
	MsPlayed     int32  `json:"ms_played"`
	TrackName    string `json:"master_metadata_track_name"`
	ArtistName   string `json:"master_metadata_album_artist_name"`
	AlbumName    string `json:"master_metadata_album_album_name"`
	SpotifyTrack string `json:"spotify_track_uri"`
}

// Parse returns r's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (spotifyFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	return parseJSONArray(ctx, r, out, startIdx, func(e spotifyEntry) ingest.ListenInput {
		listenedAt, _ := time.Parse(time.RFC3339, e.Timestamp)

		var artists []string
		if e.ArtistName != "" {
			artists = []string{e.ArtistName}
		}
		var originURL string
		if id := strings.TrimPrefix(e.SpotifyTrack, "spotify:track:"); id != "" {
			originURL = "https://open.spotify.com/track/" + id
		}

		return ingest.ListenInput{
			OriginalSubmissionClient: "spotify_export",
			OriginURL:                originURL,
			ArtistNames:              artists,
			SongName:                 e.TrackName,
			AlbumName:                e.AlbumName,
			ListenedAt:               listenedAt,
			DurationPlayedMs:         e.MsPlayed,
		}
	})
}

package importer

import (
	"io"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// listenBrainzFormat parses a ListenBrainz export: a JSON array of listen entries.
type listenBrainzFormat struct{}

// newListenBrainzFormat builds a listenBrainzFormat.
func newListenBrainzFormat() Format { return listenBrainzFormat{} }

// ID identifies this format's import_service discriminator.
func (listenBrainzFormat) ID() db.ImportService { return db.ImportServiceListenbrainz }

// lbExportEntry is one entry in a ListenBrainz export.
type lbExportEntry struct {
	ListenedAt    int64 `json:"listened_at"`
	TrackMetadata struct {
		ArtistName     string `json:"artist_name"`
		TrackName      string `json:"track_name"`
		ReleaseName    string `json:"release_name"`
		AdditionalInfo struct {
			OriginURL     string `json:"origin_url"`
			RecordingMBID string `json:"recording_mbid"`
			SpotifyID     string `json:"spotify_id"`
			DurationMs    *int32 `json:"duration_ms"`
		} `json:"additional_info"`
	} `json:"track_metadata"`
}

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (listenBrainzFormat) CountEntries(r io.Reader) (int, error) {
	return countJSONArray(r)
}

// Parse streams r, calling emit once per raw top-level entry.
func (listenBrainzFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	return decodeJSONArray(r, func(e lbExportEntry) {
		info := e.TrackMetadata.AdditionalInfo
		originURL := info.OriginURL
		if originURL == "" {
			originURL = ingest.InferOriginURL(info.RecordingMBID, info.SpotifyID)
		}
		var durationMs int32
		if info.DurationMs != nil {
			durationMs = *info.DurationMs
		}

		emit(ingest.ListenInput{
			OriginalSubmissionClient: "listenbrainz_export",
			OriginURL:                originURL,
			ArtistNames:              []string{e.TrackMetadata.ArtistName},
			SongName:                 e.TrackMetadata.TrackName,
			AlbumName:                e.TrackMetadata.ReleaseName,
			ListenedAt:               time.Unix(e.ListenedAt, 0).UTC(),
			DurationMs:               durationMs,
		})
	})
}

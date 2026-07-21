package importer

import (
	"context"
	"io"
	"time"

	"Canto/internal/ingest"
)

// listenBrainzFormat parses a ListenBrainz export: a JSON array of listen entries.
type listenBrainzFormat struct{}

// newListenBrainzFormat builds a listenBrainzFormat.
func newListenBrainzFormat() Format { return listenBrainzFormat{} }

// ID identifies this format's import_service discriminator.
func (listenBrainzFormat) ID() ImportService { return ImportServiceListenbrainz }

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

// Parse returns r's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (listenBrainzFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	return parseJSONArray(ctx, r, out, startIdx, func(e lbExportEntry) ingest.ListenInput {
		info := e.TrackMetadata.AdditionalInfo
		originURL := info.OriginURL
		if originURL == "" {
			originURL = ingest.InferOriginURL(info.RecordingMBID, info.SpotifyID)
		}
		var durationMs int32
		if info.DurationMs != nil {
			durationMs = *info.DurationMs
		}

		return ingest.ListenInput{
			OriginalSubmissionClient: "listenbrainz_export",
			OriginURL:                originURL,
			ArtistNames:              []string{e.TrackMetadata.ArtistName},
			SongName:                 e.TrackMetadata.TrackName,
			AlbumName:                e.TrackMetadata.ReleaseName,
			ListenedAt:               time.Unix(e.ListenedAt, 0).UTC(),
			DurationMs:               durationMs,
		}
	})
}

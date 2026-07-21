package importer

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"time"

	"Canto/internal/ingest"
)

// lastFMFormat parses a ghan.nl-style Last.fm scrobble export: CSV rows of artist,album,track,timestamp.
type lastFMFormat struct{}

// newLastFMFormat builds a lastFMFormat.
func newLastFMFormat() Format { return lastFMFormat{} }

// ID identifies this format's import_service discriminator.
func (lastFMFormat) ID() ImportService { return ImportServiceLastfm }

// Parse returns r's total row count immediately, then streams entries from startIdx onward to out in the background.
func (f lastFMFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		return 0, err
	}

	go func() {
		defer close(out)
		for i := startIdx; i < len(records); i++ {
			record := records[i]
			var entry ingest.ListenInput
			if len(record) < 4 {
				entry = ingest.ListenInput{OriginalSubmissionClient: "lastfm_export"}
			} else {
				artist, album, track, timestamp := record[0], record[1], record[2], record[3]
				var listenedAt time.Time
				if unix, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
					listenedAt = time.Unix(unix, 0).UTC()
				}
				var artists []string
				if artist != "" {
					artists = []string{artist}
				}
				entry = ingest.ListenInput{
					OriginalSubmissionClient: "lastfm_export",
					ArtistNames:              artists,
					SongName:                 track,
					AlbumName:                album,
					ListenedAt:               listenedAt,
				}
			}

			select {
			case out <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()
	return len(records), nil
}

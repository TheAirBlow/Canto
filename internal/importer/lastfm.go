package importer

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// lastFMFormat parses a ghan.nl-style Last.fm scrobble export: CSV rows of artist,album,track,timestamp.
type lastFMFormat struct{}

// newLastFMFormat builds a lastFMFormat.
func newLastFMFormat() Format { return lastFMFormat{} }

// ID identifies this format's import_service discriminator.
func (lastFMFormat) ID() db.ImportService { return db.ImportServiceLastfm }

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (f lastFMFormat) CountEntries(r io.Reader) (int, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// Parse streams r, calling emit once per raw top-level entry.
func (f lastFMFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1

	for {
		record, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(record) < 4 {
			emit(ingest.ListenInput{OriginalSubmissionClient: "lastfm_export"})
			continue
		}

		artist, album, track, timestamp := record[0], record[1], record[2], record[3]
		var listenedAt time.Time
		if unix, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			listenedAt = time.Unix(unix, 0).UTC()
		}

		var artists []string
		if artist != "" {
			artists = []string{artist}
		}
		emit(ingest.ListenInput{
			OriginalSubmissionClient: "lastfm_export",
			ArtistNames:              artists,
			SongName:                 track,
			AlbumName:                album,
			ListenedAt:               listenedAt,
		})
	}
}

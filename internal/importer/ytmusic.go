package importer

import (
	"io"
	"strings"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// ytmusicFormat parses a YouTube Music Takeout watch-history.json export.
type ytmusicFormat struct{}

// newYTMusicFormat builds a ytmusicFormat.
func newYTMusicFormat() Format { return ytmusicFormat{} }

// ID identifies this format's import_service discriminator.
func (ytmusicFormat) ID() db.ImportService { return db.ImportServiceYtmusic }

// ytmusicEntry is one entry in a YouTube Music Takeout watch-history export.
type ytmusicEntry struct {
	Title    string `json:"title"`
	TitleURL string `json:"titleUrl"`
	Time     string `json:"time"`
}

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (ytmusicFormat) CountEntries(r io.Reader) (int, error) {
	return countJSONArray(r)
}

// Parse streams r, calling emit once per raw top-level entry.
func (ytmusicFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	return decodeJSONArray(r, func(e ytmusicEntry) {
		if e.TitleURL == "" {
			emit(ingest.ListenInput{OriginalSubmissionClient: "ytmusic_takeout"})
			return
		}
		listenedAt, _ := time.Parse(time.RFC3339, e.Time)

		emit(ingest.ListenInput{
			OriginalSubmissionClient: "ytmusic_takeout",
			OriginURL:                e.TitleURL,
			SongName:                 strings.TrimPrefix(e.Title, "Watched "),
			ListenedAt:               listenedAt,
		})
	})
}

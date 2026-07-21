package importer

import (
	"context"
	"io"
	"strings"
	"time"

	"Canto/internal/ingest"
)

// ytmusicFormat parses a YouTube Music Takeout watch-history.json export.
type ytmusicFormat struct{}

// newYTMusicFormat builds a ytmusicFormat.
func newYTMusicFormat() Format { return ytmusicFormat{} }

// ID identifies this format's import_service discriminator.
func (ytmusicFormat) ID() ImportService { return ImportServiceYtmusic }

// ytmusicEntry is one entry in a YouTube Takeout watch-history export.
type ytmusicEntry struct {
	Header   string `json:"header"`
	Title    string `json:"title"`
	TitleURL string `json:"titleUrl"`
	Time     string `json:"time"`
}

// Parse returns r's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (ytmusicFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	return parseJSONArray(ctx, r, out, startIdx, func(e ytmusicEntry) ingest.ListenInput {
		if e.TitleURL == "" || e.Header == "YouTube" {
			return ingest.ListenInput{OriginalSubmissionClient: "ytmusic_takeout"}
		}
		listenedAt, _ := time.Parse(time.RFC3339, e.Time)

		return ingest.ListenInput{
			OriginalSubmissionClient: "ytmusic_takeout",
			OriginURL:                e.TitleURL,
			SongName:                 strings.TrimPrefix(e.Title, "Watched "),
			ListenedAt:               listenedAt,
		}
	})
}

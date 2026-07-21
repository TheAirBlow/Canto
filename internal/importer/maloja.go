package importer

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"Canto/internal/ingest"
)

// malojaFormat parses a Maloja scrobble export: {"scrobbles": [...]}.
type malojaFormat struct{}

// newMalojaFormat builds a malojaFormat.
func newMalojaFormat() Format { return malojaFormat{} }

// ID identifies this format's import_service discriminator.
func (malojaFormat) ID() ImportService { return ImportServiceMaloja }

// malojaExport is a Maloja scrobble export document.
type malojaExport struct {
	Scrobbles []struct {
		Time  int64 `json:"time"`
		Track struct {
			Artists []string `json:"artists"`
			Title   string   `json:"title"`
			Album   string   `json:"album"`
			Length  int32    `json:"length"`
		} `json:"track"`
	} `json:"scrobbles"`
}

// Parse returns doc's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (f malojaFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	doc, err := f.decode(r)
	if err != nil {
		return 0, err
	}

	go func() {
		defer close(out)
		for i := startIdx; i < len(doc.Scrobbles); i++ {
			s := doc.Scrobbles[i]
			entry := ingest.ListenInput{
				OriginalSubmissionClient: "maloja_export",
				ArtistNames:              s.Track.Artists,
				SongName:                 s.Track.Title,
				AlbumName:                s.Track.Album,
				ListenedAt:               time.Unix(s.Time, 0).UTC(),
				DurationMs:               s.Track.Length * 1000,
			}
			select {
			case out <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()
	return len(doc.Scrobbles), nil
}

// decode fully decodes r as a malojaExport document.
func (malojaFormat) decode(r io.Reader) (malojaExport, error) {
	var doc malojaExport
	err := json.NewDecoder(r).Decode(&doc)
	return doc, err
}

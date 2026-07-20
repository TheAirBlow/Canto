package importer

import (
	"encoding/json"
	"io"
	"time"

	"Canto/internal/db"
	"Canto/internal/ingest"
)

// malojaFormat parses a Maloja scrobble export: {"scrobbles": [...]}.
type malojaFormat struct{}

// newMalojaFormat builds a malojaFormat.
func newMalojaFormat() Format { return malojaFormat{} }

// ID identifies this format's import_service discriminator.
func (malojaFormat) ID() db.ImportService { return db.ImportServiceMaloja }

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

// CountEntries does a cheap structural scan of r for the raw top-level entry count.
func (f malojaFormat) CountEntries(r io.Reader) (int, error) {
	doc, err := f.decode(r)
	if err != nil {
		return 0, err
	}
	return len(doc.Scrobbles), nil
}

// Parse streams r, calling emit once per raw top-level entry.
func (f malojaFormat) Parse(r io.Reader, emit func(ingest.ListenInput)) error {
	doc, err := f.decode(r)
	if err != nil {
		return err
	}
	for _, s := range doc.Scrobbles {
		emit(ingest.ListenInput{
			OriginalSubmissionClient: "maloja_export",
			ArtistNames:              s.Track.Artists,
			SongName:                 s.Track.Title,
			AlbumName:                s.Track.Album,
			ListenedAt:               time.Unix(s.Time, 0).UTC(),
			DurationMs:               s.Track.Length * 1000,
		})
	}
	return nil
}

// decode fully decodes r as a malojaExport document.
func (malojaFormat) decode(r io.Reader) (malojaExport, error) {
	var doc malojaExport
	err := json.NewDecoder(r).Decode(&doc)
	return doc, err
}

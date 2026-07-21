package importer

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"Canto/internal/ingest"
)

// koitoFormat parses a Koito export: {"version": "1", "listens": [...]}.
type koitoFormat struct{}

// newKoitoFormat builds a koitoFormat.
func newKoitoFormat() Format { return koitoFormat{} }

// ID identifies this format's import_service discriminator.
func (koitoFormat) ID() ImportService { return ImportServiceKoito }

// koitoAlias is one name Koito recorded for an artist/album/track, alongside whether it's the canonical one.
type koitoAlias struct {
	Alias   string `json:"alias"`
	Primary bool   `json:"is_primary"`
}

// koitoExport is a Koito export document.
type koitoExport struct {
	Version string        `json:"version"`
	Listens []koitoListen `json:"listens"`
}

type koitoListen struct {
	ListenedAt time.Time     `json:"listened_at"`
	Client     string        `json:"client"`
	Track      koitoTrack    `json:"track"`
	Album      koitoAlbum    `json:"album"`
	Artists    []koitoArtist `json:"artists"`
}

type koitoTrack struct {
	MBID     *string      `json:"mbid"`
	Duration int32        `json:"duration"` // seconds
	Aliases  []koitoAlias `json:"aliases"`
}

type koitoAlbum struct {
	Aliases []koitoAlias `json:"aliases"`
}

type koitoArtist struct {
	Aliases []koitoAlias `json:"aliases"`
}

// Parse returns doc's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (f koitoFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	doc, err := f.decode(r)
	if err != nil {
		return 0, err
	}

	go func() {
		defer close(out)
		for i := startIdx; i < len(doc.Listens); i++ {
			l := doc.Listens[i]

			var mbid string
			if l.Track.MBID != nil {
				mbid = *l.Track.MBID
			}
			artistNames := make([]string, 0, len(l.Artists))
			for _, a := range l.Artists {
				if name := primaryAlias(a.Aliases); name != "" {
					artistNames = append(artistNames, name)
				}
			}

			entry := ingest.ListenInput{
				OriginalSubmissionClient: "koito_export",
				SubmissionClient:         l.Client,
				OriginURL:                ingest.InferOriginURL(mbid, ""),
				ArtistNames:              artistNames,
				SongName:                 primaryAlias(l.Track.Aliases),
				AlbumName:                primaryAlias(l.Album.Aliases),
				DurationMs:               l.Track.Duration * 1000,
				ListenedAt:               l.ListenedAt,
			}

			select {
			case out <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()
	return len(doc.Listens), nil
}

// decode fully decodes r as a koitoExport document.
func (koitoFormat) decode(r io.Reader) (koitoExport, error) {
	var doc koitoExport
	err := json.NewDecoder(r).Decode(&doc)
	return doc, err
}

// primaryAlias returns aliases' canonical name, or "" if none is marked primary.
func primaryAlias(aliases []koitoAlias) string {
	for _, a := range aliases {
		if a.Primary {
			return a.Alias
		}
	}
	return ""
}

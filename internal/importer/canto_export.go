package importer

import (
	"context"
	"encoding/json"
	"io"

	"Canto/internal/export"
	"Canto/internal/ingest"
)

// cantoExportFormat parses Canto's own export format, produced by internal/export.
type cantoExportFormat struct{}

// newCantoExportFormat builds a cantoExportFormat.
func newCantoExportFormat() Format { return cantoExportFormat{} }

// ID identifies this format's import_service discriminator.
func (cantoExportFormat) ID() ImportService { return ImportServiceCantoExport }

// Parse returns doc's total entry count immediately, then streams entries from startIdx onward to out in the background.
func (f cantoExportFormat) Parse(ctx context.Context, r io.Reader, out chan<- ingest.ListenInput, startIdx int) (int, error) {
	doc, err := f.decode(r)
	if err != nil {
		return 0, err
	}

	go func() {
		defer close(out)
		for i := startIdx; i < len(doc.Listens); i++ {
			l := doc.Listens[i]
			entry := ingest.ListenInput{OriginalSubmissionClient: "canto_export"}

			if song, ok := doc.Songs[l.SongID]; ok {
				artistNames := make([]string, 0, len(song.ArtistIDs))
				for _, artistID := range song.ArtistIDs {
					if artist, ok := doc.Artists[artistID]; ok {
						artistNames = append(artistNames, artist.Name)
					}
				}
				var albumName string
				if song.AlbumID != nil {
					if album, ok := doc.Albums[*song.AlbumID]; ok {
						albumName = album.Name
					}
				}
				var durationMs int32
				if song.DurationMs != nil {
					durationMs = *song.DurationMs
				}
				var originURL string
				for _, s := range song.Sources {
					if s.RawURL != nil && *s.RawURL != "" {
						originURL = *s.RawURL
						break
					}
				}

				entry.SubmissionClient = l.Client
				entry.OriginURL = originURL
				entry.ArtistNames = artistNames
				entry.SongName = song.Name
				entry.AlbumName = albumName
				entry.DurationMs = durationMs
				entry.ListenedAt = l.ListenedAt
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

// decode fully decodes r as an export.Export document.
func (cantoExportFormat) decode(r io.Reader) (export.Export, error) {
	var doc export.Export
	err := json.NewDecoder(r).Decode(&doc)
	return doc, err
}
